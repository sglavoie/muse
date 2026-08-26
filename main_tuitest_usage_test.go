package main

import (
	"flag"
	"io"
	"os"
	"strings"
	"testing"
)

// TestGlobalUsageOmitsTUITest guards the hidden-command contract: normal
// `muse --help` usage must not advertise the tui-test harness. configureUsage
// writes directly to os.Stderr, so the test captures it through a pipe.
func TestGlobalUsageOmitsTUITest(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	savedStderr := os.Stderr
	savedUsage := flag.Usage
	os.Stderr = w
	t.Cleanup(func() {
		os.Stderr = savedStderr
		flag.Usage = savedUsage
	})

	configureUsage()
	flag.Usage()
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}

	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read usage: %v", err)
	}
	usage := string(out)
	if strings.Contains(usage, "tui-test") {
		t.Errorf("normal usage must not mention tui-test:\n%s", usage)
	}
	// Sanity: the usage should still list the real commands.
	for _, want := range []string{"check", "now-playing", "toggle-play"} {
		if !strings.Contains(usage, want) {
			t.Errorf("usage unexpectedly missing %q:\n%s", want, usage)
		}
	}
}
