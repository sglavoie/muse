package config

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/notify"
)

// CheckPrereqs verifies that mpv and ExifTool are available and functional.
// It runs a test invocation of each binary to catch broken installs (e.g.
// ExifTool missing its Perl runtime). Returns an error with actionable
// installation instructions if either required check fails.
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
	return warnings, checkExifTool(ctx)
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

func checkExifTool(ctx context.Context) error {
	if _, err := exec.LookPath("exiftool"); err != nil {
		return fmt.Errorf(
			"ExifTool not found on $PATH\n" +
				"ExifTool requires Perl and can be installed via:\n" +
				"  macOS:  brew install exiftool\n" +
				"  Debian: apt install libimage-exiftool-perl\n" +
				"  Fedora: dnf install perl-Image-ExifTool",
		)
	}

	out, err := exec.CommandContext(ctx, "exiftool", "-ver").Output()
	if err != nil {
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
