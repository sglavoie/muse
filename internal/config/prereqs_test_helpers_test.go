package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
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

// stubMetadataPATH makes a fresh dir the entire PATH with a working mpv plus
// whatever metadata binaries the case needs, and silences the notifier so the
// only warning that can appear is the metadata one under test.
func stubMetadataPATH(t *testing.T, stubs map[string]string) {
	t.Helper()
	dir := t.TempDir()
	all := map[string]string{"mpv": "#!/bin/sh\nexit 0\n"}
	for name, body := range stubs {
		all[name] = body
	}
	for name, body := range all {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		// The first exec of a newly written executable costs hundreds of
		// milliseconds idle and several seconds while the whole suite runs
		// concurrently; a second exec of the same file is ~4ms. Pay it here so
		// it is never charged against a bounded metadata-backend probe, which
		// would turn a healthy stub into a spurious readiness failure.
		_ = exec.Command(path).Run()
	}
	t.Setenv("PATH", dir)
	stubNotifier(t, nil)
}

// stubWarmupArg lets a stub that would otherwise block for seconds recognise
// the warming exec in stubTimingPATH and exit immediately, so the first-exec
// cost is still paid outside every bounded probe.
const stubWarmupArg = "-muse-stub-warmup"

// stubTimingPATH is the timing-sensitive sibling of stubMetadataPATH: it writes
// exactly the stubs it is given (no mpv is supplied for free, because these
// cases care how long mpv takes), warms each one through stubWarmupArg so no
// probe is charged for a first exec, and silences the notifier.
func stubTimingPATH(t *testing.T, stubs map[string]string) {
	t.Helper()
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	for name, body := range stubs {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
		_ = exec.Command(path, stubWarmupArg).Run()
	}
	t.Setenv("PATH", dir)
	stubNotifier(t, nil)
}

// absSleep resolves sleep before $PATH is replaced. Every stub below spells it
// absolutely for the reason library's own hanging stub does: under an emptied
// PATH a bare `sleep` exits 127, which would make a stub meant to hang look
// merely broken and quietly turn a timing assertion into a no-op.
func absSleep(t *testing.T) string {
	t.Helper()
	sleep, err := exec.LookPath("sleep")
	if err != nil {
		t.Skip("sleep not available")
	}
	return sleep
}

// shortenProbeBounds shrinks every bound the prerequisite pass is built from so
// a case that has to actually reach a deadline runs in seconds rather than the
// production minute.
//
// The pipe-cleanup grace is set equal to the probe deadline rather than to the
// production ratio, because that is the worst case the budget has to survive
// and it is the whole quantity the old budget omitted. Making the three bounds
// equal is also what leaves the timings below room to breathe: the margin by
// which a deadline-priced budget falls short is exactly one grace, so a larger
// grace is a larger margin.
func shortenProbeBounds(t *testing.T) {
	t.Helper()
	prevMPV := mpvProbeTimeout
	t.Cleanup(func() { mpvProbeTimeout = prevMPV })
	mpvProbeTimeout = 2 * time.Second
	t.Cleanup(library.SetBackendProbeBounds(2*time.Second, 2*time.Second))
}
