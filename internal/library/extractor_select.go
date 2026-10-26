package library

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Backend names returned by SelectExtractor. They are stable strings meant for
// logs, `muse check` output and startup notices — not for parsing.
const (
	BackendExifTool = "exiftool"
	BackendFFProbe  = "ffprobe"
)

// backendProbeTimeout bounds every metadata-backend usability probe. The probes
// run from long-lived contexts — app startup and `muse check` both pass a
// context with no deadline of their own — so without a deadline of its own a
// single wedged executable would hang the whole process before the TUI ever
// appears.
//
// The deadline exists only to bound a *wedged* executable, so it is sized for
// the worst plausible healthy case rather than the typical one. A warm
// `exiftool -ver` / `ffprobe -version` answers in ~10ms, but merely starting a
// cold executable is not free: on a loaded machine — a parallel test suite, a
// busy laptop, a network-mounted or scanned-on-first-exec binary — a process
// that has not been exec'd before can take several seconds just to reach its
// first instruction. An earlier three-second bound was inside that range and
// rejected demonstrably healthy tools under load, which is much worse than a
// slow startup: it silently downgrades the backend, or reports that no metadata
// backend exists at all. Ten seconds keeps the wedged case bounded while
// leaving roughly a 1000x margin over a healthy probe.
//
// It is a var so tests can shrink it; nothing outside tests reassigns it.
var backendProbeTimeout = 10 * time.Second

// probeWaitDelay is the grace a killed probe gets to release the stdout pipe
// before exec closes it out from under whatever still holds it. Killing the
// probe kills only the process exec started, so a descendant that inherited
// that pipe keeps it open and Wait would otherwise block forever.
//
// It is deliberately much smaller than backendProbeTimeout, and deliberately a
// separate quantity rather than a second use of it. The two bound different
// things: the deadline is sized for a *healthy* cold start on a loaded machine,
// while this only has to outlast the kernel tearing down an already-killed
// process group's file descriptors. Spending a second full probe deadline here
// bought nothing and doubled every probe's worst case, which the prerequisite
// budget then had to pay for twice over.
//
// It is a var for the same reason backendProbeTimeout is: tests shrink both
// together through SetBackendProbeBounds.
var probeWaitDelay = 2 * time.Second

// BackendProbeTimeout reports the deadline applied to a single metadata-backend
// usability probe. Callers that impose their own budget over a sequence of
// probes (config.CheckPrereqs) size it from this rather than from a duplicated
// literal, so a change here cannot leave an outer budget truncating a probe
// that has not yet timed out.
func BackendProbeTimeout() time.Duration { return backendProbeTimeout }

// BackendProbeWorstCase reports the longest a single probe can take to return —
// its deadline plus the pipe-cleanup grace that a timed-out probe may then
// spend in Wait. This, not BackendProbeTimeout, is what an outer budget over a
// sequence of probes has to pay per probe: a budget built from the deadline
// alone expires while an earlier wedged probe is still unwinding, and the
// healthy fallback behind it is then reported unusable without ever being run.
func BackendProbeWorstCase() time.Duration { return backendProbeTimeout + probeWaitDelay }

// SetBackendProbeBounds overrides both probe bounds and returns a func
// restoring them. It is the seam through which tests in other packages —
// config, which builds the prerequisite budget on top of these — exercise the
// timeout and pipe-cleanup paths without paying the real multi-second bounds.
// Nothing outside tests calls it.
func SetBackendProbeBounds(timeout, waitDelay time.Duration) (restore func()) {
	prevTimeout, prevWaitDelay := backendProbeTimeout, probeWaitDelay
	backendProbeTimeout, probeWaitDelay = timeout, waitDelay
	return func() { backendProbeTimeout, probeWaitDelay = prevTimeout, prevWaitDelay }
}

// errProbeTimedOut marks the deadline case so callers can explain a hang
// differently from an outright failure — "did not respond" and "exited with
// error" send the user to very different fixes.
var errProbeTimedOut = errors.New("probe timed out")

// runBackendProbe runs a bounded liveness probe for a backend binary. The
// deadline is applied here rather than relying on the caller's context, which
// is what makes the probe safe to call from startup and from `muse check`.
func runBackendProbe(ctx context.Context, name string, args ...string) ([]byte, error) {
	probeCtx, cancel := context.WithTimeout(ctx, backendProbeTimeout)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, name, args...)
	// A killed probe must not then block in Wait because a grandchild still
	// holds the inherited stdout pipe; WaitDelay closes the pipes after a
	// bounded grace period, so the worst case stays BackendProbeWorstCase
	// instead of being unbounded.
	cmd.WaitDelay = probeWaitDelay

	out, err := cmd.Output()
	if err != nil && ctx.Err() == nil && probeCtx.Err() != nil {
		return out, fmt.Errorf("%w after %s: %w", errProbeTimedOut, backendProbeTimeout, err)
	}
	return out, err
}

// usableBinaries remembers the executables whose usability probe succeeded, so
// SelectExtractor and the startup check share one probe per binary, and the
// track-info overlay (ExtractAllTags) stops paying a process launch on every
// open. The key includes the binary's mtime and size, so reinstalling or
// replacing it is probed afresh. Failures are never cached: a missing or broken
// tool is re-probed on every call, so fixing it takes effect without a restart.
var usableBinaries sync.Map // map[usableKey]struct{}

type usableKey struct {
	path    string
	modTime int64
	size    int64
}

// cachedUsable runs probe for the executable name resolves to unless that same
// file has already passed it in this process.
func cachedUsable(name string, probe func() error) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return probe() // the probe owns the not-found message
	}
	info, err := os.Stat(path)
	if err != nil {
		return probe()
	}
	key := usableKey{path: path, modTime: info.ModTime().UnixNano(), size: info.Size()}
	if _, ok := usableBinaries.Load(key); ok {
		return nil
	}
	if err := probe(); err != nil {
		return err
	}
	usableBinaries.Store(key, struct{}{})
	return nil
}

// ExifToolUsable reports whether exiftool can actually be used as a metadata
// backend. Presence on $PATH is not enough: exiftool is a Perl script, so a
// package with a missing or broken Perl runtime resolves through LookPath and
// then fails on every invocation. The `-ver` probe (and the check that its
// output really is a version number) is what separates "installed" from
// "usable"; a nil return means metadata extraction will work.
//
// This is the single probe behind both gates — config.CheckPrereqs and
// SelectExtractor — so the startup check and the extractor that is actually
// constructed can never disagree about whether exiftool is viable.
func ExifToolUsable(ctx context.Context) error {
	return cachedUsable("exiftool", func() error { return probeExifTool(ctx) })
}

func probeExifTool(ctx context.Context) error {
	if _, err := exec.LookPath("exiftool"); err != nil {
		return fmt.Errorf(
			"ExifTool not found on $PATH\n" +
				"ExifTool requires Perl and can be installed via:\n" +
				"  macOS:  brew install exiftool\n" +
				"  Debian: apt install libimage-exiftool-perl\n" +
				"  Fedora: dnf install perl-Image-ExifTool",
		)
	}

	out, err := runBackendProbe(ctx, "exiftool", "-ver")
	if err != nil {
		if errors.Is(err, errProbeTimedOut) {
			return fmt.Errorf(
				"ExifTool is installed but did not respond within %s ('exiftool -ver' was still running): %w\n"+
					"Ensure Perl and required modules are installed (apt install libimage-exiftool-perl)",
				backendProbeTimeout, err,
			)
		}
		return fmt.Errorf(
			"ExifTool is installed but failed to run ('exiftool -ver' exited with error): %w\n"+
				"Ensure Perl and required modules are installed (apt install libimage-exiftool-perl)",
			err,
		)
	}

	version := strings.TrimSpace(string(out))
	if _, err := strconv.ParseFloat(version, 64); version == "" || err != nil {
		return fmt.Errorf(
			"ExifTool ran but 'exiftool -ver' did not produce a valid version (got: %q)\n"+
				"Ensure Perl and required modules are installed (apt install libimage-exiftool-perl)",
			version,
		)
	}

	return nil
}

// ffprobeInstallHint is the install advice shared by every message that has to
// tell the user ffprobe is missing or broken.
const ffprobeInstallHint = "ffprobe ships with FFmpeg and can be installed via:\n" +
	"  macOS:  brew install ffmpeg\n" +
	"  Debian: apt install ffmpeg\n" +
	"  Fedora: dnf install ffmpeg"

// FFProbeUsable reports whether ffprobe can actually be used as a metadata
// backend. As with ExifTool, presence on $PATH is not enough: an ffprobe whose
// shared libraries are missing, or which is a stale symlink into an uninstalled
// FFmpeg, resolves through LookPath and then fails on every invocation. Since
// ffprobe is the fallback backend, accepting a broken one is worse than
// rejecting it — every track would scan into filename-derived placeholder
// metadata with no error anywhere.
//
// A zero exit status from `ffprobe -version` is the whole criterion; the
// version string itself is not parsed, because FFmpeg distributions spell it in
// too many ways (`n6.1`, `4.4.2-0ubuntu0.22.04.1`, git hashes) for a format
// check to distinguish a broken build from an unusual packaging.
//
// This is the single probe behind both gates — config.CheckPrereqs and
// SelectExtractor — for the same reason ExifToolUsable is.
func FFProbeUsable(ctx context.Context) error {
	return cachedUsable("ffprobe", func() error { return probeFFProbe(ctx) })
}

func probeFFProbe(ctx context.Context) error {
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return fmt.Errorf("ffprobe not found on $PATH\n%s", ffprobeInstallHint)
	}

	if _, err := runBackendProbe(ctx, "ffprobe", "-version"); err != nil {
		if errors.Is(err, errProbeTimedOut) {
			return fmt.Errorf(
				"ffprobe is installed but did not respond within %s ('ffprobe -version' was still running): %w\n%s",
				backendProbeTimeout, err, ffprobeInstallHint,
			)
		}
		return fmt.Errorf(
			"ffprobe is installed but failed to run ('ffprobe -version' exited with error): %w\n%s",
			err, ffprobeInstallHint,
		)
	}

	return nil
}

// SelectExtractor chooses the metadata backend for library scans and returns
// it together with its name for logging. ExifTool wins when it is usable — it
// reads a whole batch in one process and is roughly 2.4x faster than ffprobe on
// a real library — and ffprobe is the fallback that makes the Perl runtime
// optional. Only when neither is available is there no way to read metadata at
// all, which is the one fatal case.
func SelectExtractor(ctx context.Context) (MetadataExtractor, string, error) {
	if err := ExifToolUsable(ctx); err == nil {
		return ExifToolExtractor{}, BackendExifTool, nil
	}

	if err := FFProbeUsable(ctx); err == nil {
		return FFProbeExtractor{}, BackendFFProbe, nil
	}

	return nil, "", fmt.Errorf(
		"no metadata backend available: neither exiftool nor ffprobe was found on $PATH\n" +
			"Install ExifTool (recommended, fastest scans; requires Perl):\n" +
			"  macOS:  brew install exiftool\n" +
			"  Debian: apt install libimage-exiftool-perl\n" +
			"  Fedora: dnf install perl-Image-ExifTool\n" +
			"or ffprobe, which ships with FFmpeg:\n" +
			"  macOS:  brew install ffmpeg\n" +
			"  Debian: apt install ffmpeg\n" +
			"  Fedora: dnf install ffmpeg",
	)
}
