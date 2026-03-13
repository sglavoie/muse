//go:build darwin

package library

import (
	"syscall"
	"testing"
)

// TestRaiseFDLimit_Darwin verifies that raiseFDLimit actually increases the
// soft fd limit toward the hard maximum. The test artificially lowers the soft
// limit first so a raise is always observable.
func TestRaiseFDLimit_Darwin(t *testing.T) {
	var before syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &before); err != nil {
		t.Fatalf("Getrlimit: %v", err)
	}
	if before.Max == 0 {
		t.Skip("RLIMIT_NOFILE hard limit is 0; cannot test")
	}

	// Lower the soft limit to a small value so raiseFDLimit has something to do.
	lowered := before
	lowered.Cur = 64
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &lowered); err != nil {
		t.Skipf("cannot lower fd limit (SIP or insufficient permissions): %v", err)
	}
	// Restore the original limit when the test ends.
	t.Cleanup(func() { _ = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &before) })

	raiseFDLimit()

	var after syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &after); err != nil {
		t.Fatalf("Getrlimit after raise: %v", err)
	}
	if after.Cur <= 64 {
		t.Errorf("raiseFDLimit: soft limit after raise is %d; want > 64", after.Cur)
	}
}
