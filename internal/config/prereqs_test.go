package config

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckPrereqs_BothPresent(t *testing.T) {
	stubWorkingPrereqBinaries(t)
	stubNotifier(t, nil)

	warnings, err := CheckPrereqs(context.Background())
	if err != nil {
		t.Errorf("expected no error when both binaries are present, got: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings when every backend resolves, got: %v", warnings)
	}
}

// TestCheckPrereqs_MissingNotifierIsAdvisory is the counterpart: mpv and
// ExifTool resolve, the notification backend does not, and that must surface as
// exactly one warning with no fatal error. The backend lookup is stubbed rather
// than hidden behind an empty PATH so the assertion holds on a host that has a
// notification backend installed.
func TestCheckPrereqs_MissingNotifierIsAdvisory(t *testing.T) {
	stubWorkingPrereqBinaries(t)
	stubNotifier(t, errors.New("stub backend not found on $PATH"))

	warnings, err := CheckPrereqs(context.Background())
	if err != nil {
		t.Fatalf("a missing notification backend must not be fatal, got: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 advisory warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "desktop notifications are unavailable") {
		t.Errorf("warning %q does not name the problem", warnings[0])
	}
	if !strings.Contains(warnings[0], "stub backend not found on $PATH") {
		t.Errorf("warning %q drops the backend's own explanation", warnings[0])
	}
}

// TestCheckNotifierIsAdvisory pins the non-fatal contract at the unit level:
// checkNotifier reports a resolution failure as a warning string and never as
// an error, and stays silent when a backend is available.
func TestCheckNotifierIsAdvisory(t *testing.T) {
	stubNotifier(t, nil)
	if warnings := checkNotifier(); len(warnings) != 0 {
		t.Errorf("available backend should produce no warnings, got: %v", warnings)
	}

	stubNotifier(t, errors.New("no backend here"))
	warnings := checkNotifier()
	if len(warnings) != 1 {
		t.Fatalf("checkNotifier returned %d warnings, want exactly 1: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "desktop notifications are unavailable") {
		t.Errorf("warning %q does not explain the problem", warnings[0])
	}
}

// TestCheckPrereqs_ExifToolMissingFallsBackToFFProbe pins the point of this
// change: ExifTool is no longer an unconditional startup requirement. With
// ffprobe present, its absence is a warning and startup continues.
func TestCheckPrereqs_ExifToolMissingFallsBackToFFProbe(t *testing.T) {
	stubMetadataPATH(t, map[string]string{"ffprobe": "#!/bin/sh\nexit 0\n"})

	warnings, err := CheckPrereqs(context.Background())
	if err != nil {
		t.Fatalf("a missing ExifTool with ffprobe present must not be fatal, got: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "ffprobe") {
		t.Errorf("warning %q does not name the fallback backend", warnings[0])
	}
	if strings.Contains(warnings[0], "\n") {
		t.Errorf("warning must stay single-line for the caller's warning list, got: %q", warnings[0])
	}
}

// TestCheckPrereqs_ExifToolBrokenFallsBackToFFProbe is the subtle half: the
// binary resolves through LookPath but 'exiftool -ver' fails, so the
// broken-Perl detection must demote to the fallback rather than abort startup.
func TestCheckPrereqs_ExifToolBrokenFallsBackToFFProbe(t *testing.T) {
	for name, script := range map[string]string{
		"exits non-zero":   "#!/bin/sh\nexit 1\n",
		"unparseable -ver": "#!/bin/sh\nprintf 'not a version\\n'\nexit 0\n",
	} {
		t.Run(name, func(t *testing.T) {
			stubMetadataPATH(t, map[string]string{
				"exiftool": script,
				"ffprobe":  "#!/bin/sh\nexit 0\n",
			})

			warnings, err := CheckPrereqs(context.Background())
			if err != nil {
				t.Fatalf("a broken ExifTool with ffprobe present must not be fatal, got: %v", err)
			}
			if len(warnings) != 1 {
				t.Fatalf("expected exactly 1 warning, got %d: %v", len(warnings), warnings)
			}
			if !strings.Contains(warnings[0], "ExifTool") {
				t.Errorf("warning %q does not explain what was lost", warnings[0])
			}
		})
	}
}

// TestCheckPrereqs_NoMetadataBackendIsFatal is the remaining fatal case: with
// neither backend there is no way to read metadata at all.
func TestCheckPrereqs_NoMetadataBackendIsFatal(t *testing.T) {
	stubMetadataPATH(t, nil)

	_, err := CheckPrereqs(context.Background())
	if err == nil {
		t.Fatal("expected an error when neither exiftool nor ffprobe is available")
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "exiftool") {
		t.Errorf("error should mention exiftool, got: %s", msg)
	}
	if !strings.Contains(msg, "ffprobe") {
		t.Errorf("error should mention ffprobe and how to install it, got: %s", msg)
	}
}

// TestCheckPrereqs_RejectsBrokenFFProbe is the fallback's own broken-install
// case, and it is the reason readiness runs ffprobe instead of only resolving
// it on $PATH. An ffprobe that resolves but cannot execute — missing shared
// libraries, a stale symlink into an uninstalled FFmpeg — used to satisfy the
// check, so muse started, warned only that ExifTool was unavailable, and then
// scanned the whole library into filename-derived placeholder metadata. With
// neither backend usable this must be the fatal case, naming both binaries.
func TestCheckPrereqs_RejectsBrokenFFProbe(t *testing.T) {
	stubMetadataPATH(t, map[string]string{"ffprobe": "#!/bin/sh\nexit 127\n"})

	warnings, err := CheckPrereqs(context.Background())
	if err == nil {
		t.Fatalf("an ffprobe that cannot run must not satisfy the metadata prereq, warnings: %v", warnings)
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "exiftool") {
		t.Errorf("error should mention exiftool, got: %s", msg)
	}
	if !strings.Contains(msg, "ffprobe is not usable") {
		t.Errorf("error should say ffprobe itself is broken rather than absent, got: %s", msg)
	}
	for _, w := range warnings {
		if strings.Contains(w, "falling back to ffprobe") {
			t.Errorf("a broken ffprobe must not be announced as a working fallback, got: %q", w)
		}
	}
}

// TestCheckPrereqs_FFProbeMissingIsSilentWhenExifToolWorks guards against the
// fallback warning firing on a healthy ExifTool-only install.
func TestCheckPrereqs_FFProbeMissingIsSilentWhenExifToolWorks(t *testing.T) {
	stubMetadataPATH(t, map[string]string{"exiftool": "#!/bin/sh\nprintf '12.00\\n'\nexit 0\n"})

	warnings, err := CheckPrereqs(context.Background())
	if err != nil {
		t.Fatalf("a working ExifTool needs no ffprobe, got: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("expected no warnings when ExifTool works, got: %v", warnings)
	}
}

func TestCheckPrereqs_MPVMissing(t *testing.T) {
	// Only exiftool stub present — use isolated PATH so system mpv is hidden.
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '12.00\\n'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	_, err := CheckPrereqs(context.Background())
	if err == nil {
		t.Fatal("expected error when mpv is missing")
	}
	if !strings.Contains(err.Error(), "mpv") {
		t.Errorf("error should mention mpv, got: %s", err.Error())
	}
}

func TestCheckPrereqs_ExifToolMissing(t *testing.T) {
	// Only mpv stub present — use an isolated PATH so system exiftool is hidden.
	dir := t.TempDir()
	script := filepath.Join(dir, "mpv")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	_, err := CheckPrereqs(context.Background())
	if err == nil {
		t.Fatal("expected error when exiftool is missing")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "exiftool") {
		t.Errorf("error should mention exiftool, got: %s", err.Error())
	}
}
