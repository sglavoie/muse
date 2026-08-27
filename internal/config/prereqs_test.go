package config

import (
	"context"
	"errors"
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

// stubNotifier forces the notification-backend lookup to return err (nil for
// "a backend is available") for the duration of the test. Without it the
// notifier outcome depends on the host: alerter is resolved from absolute
// Homebrew paths on macOS and notify-send from $PATH on Linux, so no
// combination of PATH juggling makes either branch reachable everywhere.
func stubNotifier(t *testing.T, err error) {
	t.Helper()
	original := notifierAvailable
	t.Cleanup(func() { notifierAvailable = original })
	notifierAvailable = func() error { return err }
}

// stubWorkingPrereqBinaries writes passing mpv and exiftool stubs into a fresh
// temp dir and makes it the entire PATH, so the required checks succeed while
// nothing else on the host is reachable.
func stubWorkingPrereqBinaries(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	stubs := map[string]string{
		"mpv":      "#!/bin/sh\nexit 0\n",
		"exiftool": "#!/bin/sh\nprintf '12.00\\n'\nexit 0\n",
	}
	for name, content := range stubs {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
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
