package config

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

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
