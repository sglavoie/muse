package config

import (
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

	if err := checkMPV(); err != nil {
		t.Errorf("expected no error with stub mpv, got: %v", err)
	}
}

func TestCheckMPV_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty dir — no mpv present

	err := checkMPV()
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

	err := checkMPV()
	if err == nil {
		t.Fatal("expected error when mpv exits non-zero")
	}
	if !strings.Contains(err.Error(), "mpv --version") {
		t.Errorf("error should mention 'mpv --version', got: %s", err.Error())
	}
}

func TestCheckExifTool_RunsJSONFlag(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	content := "#!/bin/sh\nprintf '[{\"ExifToolVersion\":\"12.00\"}]'\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := checkExifTool(); err != nil {
		t.Errorf("expected no error with stub exiftool emitting valid JSON, got: %v", err)
	}
}

func TestCheckExifTool_ValidJSONOutput(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	content := "#!/bin/sh\nprintf '[{\"ExifToolVersion\":\"12.00\"}]'\nexit 0\n"
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := checkExifTool(); err != nil {
		t.Errorf("stub emitting valid JSON should succeed, got: %v", err)
	}
}

func TestCheckExifTool_GarbageOutput(t *testing.T) {
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

	err := checkExifTool()
	if err == nil {
		t.Fatal("expected error when exiftool outputs garbage instead of JSON")
	}
	msg := err.Error()
	if !strings.Contains(strings.ToLower(msg), "json") {
		t.Errorf("error should mention JSON, got: %s", msg)
	}
}

func TestCheckExifTool_MissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())

	err := checkExifTool()
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

	err := checkExifTool()
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
		"exiftool": "#!/bin/sh\nprintf '[{\"ExifToolVersion\":\"12.00\"}]'\nexit 0\n",
	}
	for name, content := range stubs {
		script := filepath.Join(dir, name)
		if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := CheckPrereqs(); err != nil {
		t.Errorf("expected no error when both binaries are present, got: %v", err)
	}
}

func TestCheckPrereqs_MPVMissing(t *testing.T) {
	// Only exiftool stub present — use isolated PATH so system mpv is hidden.
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '[{\"ExifToolVersion\":\"12.00\"}]'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	err := CheckPrereqs()
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

	err := CheckPrereqs()
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

	if err := checkMPV(); err != nil {
		t.Errorf("checkMPV should invoke 'mpv --version', got error: %v", err)
	}
}

// TestCheckExifTool_InvokesJSONFlag verifies the subprocess is called with -json.
func TestCheckExifTool_InvokesJSONFlag_Strict(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "exiftool")
	// Emit valid JSON and exit 0 only when first arg is -json.
	content := `#!/bin/sh
if [ "$1" = "-json" ]; then printf '[{"ExifToolVersion":"12.00"}]'; exit 0; fi
exit 1
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", prependPATH(dir))

	if err := checkExifTool(); err != nil {
		t.Errorf("checkExifTool should invoke 'exiftool -json ...', got error: %v", err)
	}
}
