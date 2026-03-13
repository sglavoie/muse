package config

import (
	"fmt"
	"os/exec"
)

// CheckPrereqs verifies that mpv and ExifTool are available and functional.
// It runs a test invocation of each binary to catch broken installs (e.g.
// ExifTool missing its Perl runtime). Returns an error with actionable
// installation instructions if either check fails.
func CheckPrereqs() error {
	if err := checkMPV(); err != nil {
		return err
	}
	return checkExifTool()
}

func checkMPV() error {
	if _, err := exec.LookPath("mpv"); err != nil {
		return fmt.Errorf(
			"mpv not found on $PATH\n" +
				"Install mpv:\n" +
				"  macOS:  brew install mpv\n" +
				"  Debian: apt install mpv\n" +
				"  Fedora: dnf install mpv",
		)
	}

	if err := exec.Command("mpv", "--version").Run(); err != nil {
		return fmt.Errorf("mpv is installed but failed to run ('mpv --version' exited with error): %w", err)
	}

	return nil
}

func checkExifTool() error {
	if _, err := exec.LookPath("exiftool"); err != nil {
		return fmt.Errorf(
			"ExifTool not found on $PATH\n" +
				"ExifTool requires Perl and can be installed via:\n" +
				"  macOS:  brew install exiftool\n" +
				"  Debian: apt install libimage-exiftool-perl\n" +
				"  Fedora: dnf install perl-Image-ExifTool",
		)
	}

	if err := exec.Command("exiftool", "-json", "-ver").Run(); err != nil {
		return fmt.Errorf(
			"ExifTool is installed but failed to run ('exiftool -json -ver' exited with error): %w\n"+
				"Ensure Perl and required modules are installed (apt install libimage-exiftool-perl)",
			err,
		)
	}

	return nil
}
