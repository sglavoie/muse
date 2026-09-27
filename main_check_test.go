package main

import (
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateXDG points every muse directory at temp dirs and gives the process a
// home with no ~/Music, so runCheck sees a pristine first-run environment.
// It returns the temporary home so callers can populate it without reading the
// environment back. The one-shot commands replace slog's default logger, so the
// logging state is restored when the test ends too.
func isolateXDG(t *testing.T) string {
	t.Helper()
	restoreLogging(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, ".data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(home, ".state"))
	t.Setenv("XDG_RUNTIME_DIR", filepath.Join(home, ".run"))
	return home
}

// captureOutput runs fn with os.Stdout and os.Stderr redirected, returning
// everything both wrote.
func captureOutput(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origOut, origErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w

	done := make(chan string, 1)
	go func() {
		out, _ := io.ReadAll(r)
		done <- string(out)
	}()

	fn()

	os.Stdout, os.Stderr = origOut, origErr
	w.Close()
	out := <-done
	r.Close()
	return out
}

// `muse check` is the explicit pre-flight and stays strict: the TUI softening
// (an invalid music directory becomes a warning) must not leak into it.
func TestRunCheck_InvalidMusicDirExitsOne(t *testing.T) {
	_ = isolateXDG(t)

	var code int
	out := captureOutput(t, func() { code = runCheck() })

	if code != 1 {
		t.Fatalf("runCheck = %d, want 1 with no music directory\noutput:\n%s", code, out)
	}
	if !strings.Contains(out, "music directory") {
		t.Errorf("output did not report the music directory problem:\n%s", out)
	}
	if !strings.Contains(out, "music_directories = [") {
		t.Errorf("output did not include the actionable config hint:\n%s", out)
	}
	if strings.Contains(out, "\nok\n") {
		t.Errorf("runCheck printed ok despite failing:\n%s", out)
	}
}

// With neither mpv nor a metadata backend installed, `muse check` must report
// both in one run, each on its own "error:" line.
func TestRunCheck_ReportsEveryMissingPrereqOnItsOwnLine(t *testing.T) {
	_ = isolateXDG(t)
	t.Setenv("PATH", t.TempDir())

	var code int
	out := captureOutput(t, func() { code = runCheck() })

	if code != 1 {
		t.Fatalf("runCheck = %d, want 1 with no prerequisites installed\noutput:\n%s", code, out)
	}
	for _, want := range []string{"error: mpv not found", "error: ExifTool not found"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing a line starting %q:\n%s", want, out)
		}
	}
}

// The same environment plus a populated music directory must not fail on the
// music-directory check, proving the exit code above is caused by that check
// and not by an unrelated prerequisite.
func TestRunCheck_ValidMusicDirDoesNotReportDirError(t *testing.T) {
	musicDir := filepath.Join(isolateXDG(t), "Music")
	if err := os.MkdirAll(musicDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(musicDir, "track.mp3"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	out := captureOutput(t, func() { runCheck() })

	if strings.Contains(out, "music directory") {
		t.Errorf("unexpected music directory error for a populated ~/Music:\n%s", out)
	}
}

// restoreLogging snapshots slog's default logger and the log package state
// that slog.SetDefault rewires, and restores both when the test ends.
func restoreLogging(t *testing.T) {
	t.Helper()
	origSlog, origOut, origFlags := slog.Default(), log.Writer(), log.Flags()
	t.Cleanup(func() {
		slog.SetDefault(origSlog)
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})
}

// writeOutOfRangeVolumeConfig gives the isolated home a config whose
// default_volume triggers exactly one validation warning.
func writeOutOfRangeVolumeConfig(t *testing.T, home string) {
	t.Helper()
	dir := filepath.Join(home, ".config", "muse")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("default_volume = 500\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// captureWithLog is captureOutput that also captures the log package, which
// slog's default handler writes through; its writer was bound to the real
// stderr at init, so swapping os.Stderr alone would miss it. Callers must have
// called isolateXDG, which restores the log package afterwards.
func captureWithLog(t *testing.T, fn func()) string {
	t.Helper()
	return captureOutput(t, func() {
		log.SetOutput(os.Stderr)
		fn()
	})
}

// The one-shot commands never call InitLogging, so config warnings used to
// appear twice: once through slog's default handler and once as "warning:".
func TestRunCheck_PrintsConfigWarningOnce(t *testing.T) {
	home := isolateXDG(t)
	writeOutOfRangeVolumeConfig(t, home)

	out := captureWithLog(t, func() { runCheck() })

	if n := strings.Count(out, "default_volume"); n != 1 {
		t.Errorf("default_volume warning printed %d times, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "warning: default_volume 500 is out of range") {
		t.Errorf("output missing the warning line:\n%s", out)
	}
}

func TestRunNowPlaying_PrintsConfigWarningOnce(t *testing.T) {
	home := isolateXDG(t)
	writeOutOfRangeVolumeConfig(t, home)
	_ = stubNotifications(t)

	out := captureWithLog(t, func() { runNowPlaying() })

	if n := strings.Count(out, "default_volume"); n != 1 {
		t.Errorf("default_volume warning printed %d times, want 1:\n%s", n, out)
	}
	if !strings.Contains(out, "warning: default_volume 500 is out of range") {
		t.Errorf("output missing the warning line:\n%s", out)
	}
}
