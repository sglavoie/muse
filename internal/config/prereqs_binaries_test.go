package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
