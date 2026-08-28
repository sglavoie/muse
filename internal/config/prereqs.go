package config

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/notify"
)

// CheckPrereqs verifies that mpv and a metadata backend are available and
// functional. It runs a test invocation of each binary to catch broken installs
// (e.g. ExifTool missing its Perl runtime). Returns an error with actionable
// installation instructions if a required check fails.
//
// mpv is unconditionally required. Metadata extraction is not tied to one
// binary: ExifTool is preferred, ffprobe is an accepted fallback, and only the
// absence of both is fatal.
//
// Advisory problems that must not block startup are returned separately as
// warnings. Warnings are always returned, including alongside a fatal error,
// so callers can surface everything they know in one pass.
func CheckPrereqs(ctx context.Context) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	warnings := checkNotifier()

	if err := checkMPV(ctx); err != nil {
		return warnings, err
	}

	metadataWarnings, err := checkMetadataBackend(ctx)
	return append(warnings, metadataWarnings...), err
}

// checkMetadataBackend applies the same preference order as
// library.SelectExtractor — which is why it goes through the same two probes
// rather than reimplementing either. A usable ExifTool is silent; falling back
// to ffprobe is a warning, since scans still work but are slower; having
// neither is the only fatal outcome.
//
// Both probes actually run their binary and bound themselves, so a wedged
// executable cannot hold startup open and a present-but-broken ffprobe is
// rejected here rather than silently becoming the backend.
//
// checkExifTool's error is deliberately demoted to a fall-through signal here:
// all three of its failure modes (absent, non-zero `-ver`, unparseable version)
// mean "cannot extract metadata with ExifTool", and each must reach ffprobe
// rather than abort startup.
func checkMetadataBackend(ctx context.Context) ([]string, error) {
	exifErr := checkExifTool(ctx)
	if exifErr == nil {
		return nil, nil
	}

	ffprobeErr := library.FFProbeUsable(ctx)
	if ffprobeErr == nil {
		return []string{
			"ExifTool is unavailable, falling back to ffprobe for track metadata " +
				"(scans are roughly 2.4x slower): " + firstLine(exifErr),
		}, nil
	}

	return nil, fmt.Errorf("%w\nffprobe is not usable either: %s\ninstall FFmpeg to use it instead:\n"+
		"  macOS:  brew install ffmpeg\n"+
		"  Debian: apt install ffmpeg\n"+
		"  Fedora: dnf install ffmpeg",
		exifErr, firstLine(ffprobeErr),
	)
}

// firstLine keeps a multi-line install-instruction error readable inside a
// single-line warning; the full text is still reachable via `muse check`.
func firstLine(err error) string {
	msg := err.Error()
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		return msg[:i]
	}
	return msg
}

// notifierAvailable is the seam through which checkNotifier reaches the
// notification backend. Declared as a var so tests can force either outcome:
// the backend is resolved from absolute paths on macOS and from $PATH on Linux,
// so neither presence nor absence can be arranged portably by a test that only
// manipulates the environment.
var notifierAvailable = notify.Available

// checkNotifier is advisory only: notifications are opt-in
// (defaultNotifyOnTrackChange is false), so a missing backend must never block
// startup — it just means `muse now-playing` and the track-change banner are
// silently dead, which is exactly what this warning exists to surface.
func checkNotifier() []string {
	err := notifierAvailable()
	if err == nil {
		return nil
	}
	return []string{"desktop notifications are unavailable: " + err.Error()}
}

func checkMPV(ctx context.Context) error {
	if _, err := exec.LookPath("mpv"); err != nil {
		return fmt.Errorf(
			"mpv not found on $PATH\n" +
				"Install mpv:\n" +
				"  macOS:  brew install mpv\n" +
				"  Debian: apt install mpv\n" +
				"  Fedora: dnf install mpv",
		)
	}

	if err := exec.CommandContext(ctx, "mpv", "--version").Run(); err != nil {
		return fmt.Errorf("mpv is installed but failed to run ('mpv --version' exited with error): %w", err)
	}

	return nil
}

// checkExifTool reports whether ExifTool is usable, including the broken-Perl
// case where the binary resolves but `exiftool -ver` cannot produce a version.
// The probe itself lives in library.ExifToolUsable so that the startup check
// and library.SelectExtractor decide from exactly the same evidence — a broken
// install that warns here must also fall through to ffprobe there, instead of
// handing back an ExifToolExtractor that would scan an empty library.
func checkExifTool(ctx context.Context) error {
	return library.ExifToolUsable(ctx)
}
