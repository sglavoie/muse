package config

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestPrereqBudget_CoversEveryProbeWorstCase pins the arithmetic directly,
// without depending on any wall clock. A probe's worst case is its deadline
// plus the pipe-cleanup grace it may then spend in Wait, and the budget has to
// cover mpv plus *both* backend probes at that price. Budgeting
// BackendProbeTimeout instead was the defect: it looks sufficient, and is short
// by one pipe-cleanup grace per backend probe.
func TestPrereqBudget_CoversEveryProbeWorstCase(t *testing.T) {
	worst := library.BackendProbeWorstCase()

	if worst <= library.BackendProbeTimeout() {
		t.Fatalf("a probe's worst case (%s) must exceed its deadline (%s), or the pipe-cleanup grace is unpriced",
			worst, library.BackendProbeTimeout())
	}
	if got, want := prereqBudget(), mpvProbeTimeout+2*worst; got < want {
		t.Errorf("prereqBudget() = %s, need at least %s (mpv %s + 2 backend probes at %s)",
			got, want, mpvProbeTimeout, worst)
	}
}

// TestCheckPrereqs_PipeHoldingExifToolStillReachesHealthyFFProbe is the
// behavioural half, and the case the earlier hanging stub structurally could
// not reach. library's stub replaces its shell with `exec sleep`, so the kill
// lands on the one process holding the probe's stdout pipe and Wait returns at
// the deadline. A real wedged ExifTool is a Perl script: killing it leaves the
// descendants that inherited its stdout alive, the pipe stays open, and the
// probe only returns once WaitDelay closes it — a full grace *after* the
// deadline it already spent.
//
// So the ordering here is a slow-but-healthy mpv, then an ExifTool that hangs
// while a background child keeps the pipe, then a healthy ffprobe that is
// merely slow to start. Priced at BackendProbeTimeout the parent context is
// already spent by the time ffprobe is reached and startup dies with "no
// metadata backend" despite a working fallback being installed; priced at
// BackendProbeWorstCase the fallback is reached and the outcome is the warning.
//
// With every bound at 2s the two budgets are 6s (deadline-priced) and 10s
// (worst-case-priced) against 6.8s of real work, so the case fails by 0.8s
// under the old arithmetic and passes with 3.2s to spare under the new one.
// Both slow stubs stay 0.6s inside their own deadlines, which is the headroom
// left for exec overhead when the whole suite runs concurrently.
func TestCheckPrereqs_PipeHoldingExifToolStillReachesHealthyFFProbe(t *testing.T) {
	shortenProbeBounds(t)
	sleep := absSleep(t)
	guard := "[ \"$1\" = \"" + stubWarmupArg + "\" ] && exit 0\n"

	stubTimingPATH(t, map[string]string{
		// Consumes most of its 2s share and succeeds, which is all "slow mpv"
		// needs to mean: the budget the backend probes inherit is already
		// reduced by real work before either of them runs.
		"mpv": "#!/bin/sh\n" + guard + sleep + " 1.4\nexit 0\n",
		// Hangs past its 2s deadline while a backgrounded child holds the
		// inherited stdout pipe, so the probe cannot return until the 2s
		// WaitDelay closes it: 4s consumed, not 2s. The child is bounded so a
		// failing run leaks nothing beyond the test binary's lifetime.
		"exiftool": "#!/bin/sh\n" + guard + sleep + " 30 &\n" + sleep + " 30\n",
		// Healthy and comfortably inside its own 2s deadline, but slower than
		// the deadline-priced budget has left by the time it is reached.
		"ffprobe": "#!/bin/sh\n" + guard + sleep + " 1.4\nexit 0\n",
	})

	warnings, err := CheckPrereqs(context.Background())
	if err != nil {
		t.Fatalf("a healthy ffprobe behind a pipe-holding ExifTool must still be reached, got: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 fallback warning, got %d: %v", len(warnings), warnings)
	}
	if !strings.Contains(warnings[0], "ffprobe") {
		t.Errorf("warning %q does not name the fallback that was reached", warnings[0])
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
