package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// isolateXDG points every muse directory at temp dirs and gives the process a
// home with no ~/Music, so runCheck sees a pristine first-run environment.
// It returns the temporary home so callers can populate it without reading the
// environment back.
func isolateXDG(t *testing.T) string {
	t.Helper()
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
