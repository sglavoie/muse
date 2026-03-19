package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"
)

// CheckPrereqs verifies that mpv and ExifTool are available and functional.
// It runs a test invocation of each binary to catch broken installs (e.g.
// ExifTool missing its Perl runtime). Returns an error with actionable
// installation instructions if either check fails.
func CheckPrereqs(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := checkMPV(ctx); err != nil {
		return err
	}
	return checkExifTool(ctx)
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

	out, err := exec.CommandContext(ctx, "exiftool", "-json", "-ver").Output()
	if err != nil {
		return fmt.Errorf(
			"ExifTool is installed but failed to run ('exiftool -json -ver' exited with error): %w\n"+
				"Ensure Perl and required modules are installed (apt install libimage-exiftool-perl)",
			err,
		)
	}

	if !json.Valid(out) {
		return fmt.Errorf(
			"ExifTool ran but 'exiftool -json -ver' did not produce valid JSON (got: %q)\n"+
				"Ensure Perl and required modules are installed (apt install libimage-exiftool-perl)",
			out,
		)
	}

	return nil
}
