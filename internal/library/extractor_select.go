package library

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// Backend names returned by SelectExtractor. They are stable strings meant for
// logs, `muse check` output and startup notices — not for parsing.
const (
	BackendExifTool = "exiftool"
	BackendFFProbe  = "ffprobe"
)

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

	if FFProbeAvailable() {
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
