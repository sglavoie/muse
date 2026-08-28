package library

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
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

// BackendProbeTimeout reports the deadline applied to a single metadata-backend
// usability probe. Callers that impose their own budget over a sequence of
// probes (config.CheckPrereqs) size it from this rather than from a duplicated
// literal, so a change here cannot leave an outer budget truncating a probe
// that has not yet timed out.
func BackendProbeTimeout() time.Duration { return backendProbeTimeout }

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
	// bounded grace period, so the worst case stays twice the probe timeout
	// instead of being unbounded.
	cmd.WaitDelay = backendProbeTimeout

	out, err := cmd.Output()
	if err != nil && ctx.Err() == nil && probeCtx.Err() != nil {
		return out, fmt.Errorf("%w after %s: %w", errProbeTimedOut, backendProbeTimeout, err)
	}
	return out, err
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
