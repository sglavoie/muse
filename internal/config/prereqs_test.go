package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stubBinary writes a tiny shell script that acts as a fake binary and returns
// its path. exitCode controls whether it exits successfully or not.
func stubBinary(t *testing.T, name string, exitCode int) string {
	t.Helper()
	dir := t.TempDir()
	script := filepath.Join(dir, name)
	content := "#!/bin/sh\n"
	if exitCode != 0 {
		content += "exit 1\n"
	}
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("stubBinary: %v", err)
	}
	return dir
}

// prependPATH returns a PATH string with dir prepended to the current PATH.
func prependPATH(dir string) string {
	return dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

func TestCheckMPV_RunsVersionFlag(t *testing.T) {
	// Verify that checkMPV calls "mpv --version" by using a stub that records
	// its arguments via exit 0 only when called correctly.
	dir := t.TempDir()
	script := filepath.Join(dir, "mpv")
	// The stub succeeds regardless of args; we just verify it can be found and run.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := checkMPV(context.Background()); err != nil {
		t.Errorf("expected no error with stub mpv, got: %v", err)
	}
}

func TestCheckMPV_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir — no mpv present

	err := checkMPV(context.Background())
	if err == nil {
		t.Fatal("expected error when mpv is missing")
	}
	msg := err.Error()
	if !strings.Contains(msg, "mpv") {
		t.Errorf("error should mention mpv, got: %s", msg)
	}
	// Must include installation instructions.
	if !strings.Contains(msg, "install") && !strings.Contains(msg, "Install") {
		t.Errorf("error should include installation instructions, got: %s", msg)
	}
}

func TestCheckMPV_BrokenBinary(t *testing.T) {
	dir := stubBinary(t, "mpv", 1) // exits non-zero
	t.Setenv("PATH", prependPATH(dir))

	err := checkMPV(context.Background())
	if err == nil {
		t.Fatal("expected error when mpv exits non-zero")
	}
	if !strings.Contains(err.Error(), "mpv --version") {
		t.Errorf("error should mention 'mpv --version', got: %s", err.Error())
	}
}

func TestCheckExifTool_RunsVersionFlag(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	content := "#!/bin/sh\nprintf '12.00\\n'\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := checkExifTool(context.Background()); err != nil {
		t.Errorf("expected no error with stub exiftool emitting valid version, got: %v", err)
	}
}

func TestCheckExifTool_ValidVersionOutput(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	content := "#!/bin/sh\nprintf '12.00\\n'\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := checkExifTool(context.Background()); err != nil {
		t.Errorf("stub emitting valid version should succeed, got: %v", err)
	}
}

func TestCheckExifTool_InvalidVersionOutput(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	content := "#!/bin/sh\nprintf 'not json'\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	err := checkExifTool(context.Background())
	if err == nil {
		t.Fatal("expected error when exiftool outputs garbage instead of a version")
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "version") {
		t.Errorf("error should mention version, got: %s", msg)
	}
}

func TestCheckExifTool_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := checkExifTool(context.Background())
	if err == nil {
		t.Fatal("expected error when exiftool is missing")
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "perl") {
		t.Errorf("error should mention Perl dependency, got: %s", msg)
	}
	if !strings.Contains(msg, "libimage-exiftool-perl") {
		t.Errorf("error should suggest 'apt install libimage-exiftool-perl', got: %s", msg)
	}
}

func TestCheckExifTool_BrokenBinary(t *testing.T) {
	dir := stubBinary(t, "exiftool", 1)
	t.Setenv("PATH", prependPATH(dir))

	err := checkExifTool(context.Background())
	if err == nil {
		t.Fatal("expected error when exiftool exits non-zero")
	}
	msg := err.Error()
	if !strings.Contains(msg, "exiftool") {
		t.Errorf("error should mention exiftool, got: %s", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "perl") {
		t.Errorf("broken exiftool error should mention Perl, got: %s", msg)
	}
}

func TestCheckPrereqs_BothPresent(t *testing.T) {
	dir := t.TempDir()
	stubs := map[string]string{
		"mpv":      "#!/bin/sh\nexit 0\n",
		"exiftool": "#!/bin/sh\nprintf '12.00\\n'\nexit 0\n",
	}
	for name, content := range stubs {
		script := filepath.Join(dir, name)
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", prependPATH(dir))

	// PATH holds only the two stubs, so on platforms whose notification
	// backend is resolved through PATH the notifier check fires. It must be
	// advisory: a warning, never an error.
	warnings, err := CheckPrereqs(context.Background())
	if err != nil {
		t.Errorf("expected no error when both binaries are present, got: %v", err)
	}
	for _, w := range warnings {
		if !strings.Contains(w, "notification") {
			t.Errorf("unexpected advisory warning %q, want one about notifications", w)
		}
	}
}

// TestCheckNotifierIsAdvisory pins the non-fatal contract: whatever the host
// notification backend situation is, checkNotifier reports it as a warning
// string and never as an error.
func TestCheckNotifierIsAdvisory(t *testing.T) {
	warnings := checkNotifier()
	if len(warnings) > 1 {
		t.Fatalf("checkNotifier returned %d warnings, want at most 1: %v", len(warnings), warnings)
	}
	for _, w := range warnings {
		if !strings.Contains(w, "desktop notifications are unavailable") {
			t.Errorf("warning %q does not explain the problem", w)
		}
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

// TestCheckMPV_InvokesVersionFlag verifies the actual subprocess args by
// checking that a stub which only succeeds when passed "--version" works.
func TestCheckMPV_InvokesVersionFlag_Strict(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "mpv")
	// Exit 0 only when first arg is --version.
	content := `#!/bin/sh
if [ "$1" = "--version" ]; then exit 0; fi
exit 1
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := checkMPV(context.Background()); err != nil {
		t.Errorf("checkMPV should invoke 'mpv --version', got error: %v", err)
	}
}

// TestCheckExifTool_InvokesVersionFlag verifies the subprocess is called with -ver.
func TestCheckExifTool_InvokesVersionFlag_Strict(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	// Emit a valid version and exit 0 only when first arg is -ver.
	content := `#!/bin/sh
if [ "$1" = "-ver" ]; then printf '12.00'; exit 0; fi
exit 1
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := checkExifTool(context.Background()); err != nil {
		t.Errorf("checkExifTool should invoke 'exiftool -ver', got error: %v", err)
	}
}
