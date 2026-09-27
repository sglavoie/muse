package main

import (
	"bytes"
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

// TestHelpFlagPrintsUsageToStdout covers explicit `muse --help`: the text goes
// to stdout (it is the requested output, not an error), and the Flags section
// lists the global flags instead of being empty, since none of them is
// registered with the flag package.
func TestHelpFlagPrintsUsageToStdout(t *testing.T) {
	var stdout bytes.Buffer
	if !runGlobalFlag(globalFlagHelp, &stdout) {
		t.Fatal("runGlobalFlag(help) = false, want true")
	}
	usage := stdout.String()

	if !strings.HasPrefix(usage, "Usage: muse [command]\n") {
		t.Errorf("usage line should not advertise [flags] for commands:\n%s", usage)
	}
	_, flags, ok := strings.Cut(usage, "\nFlags:\n")
	if !ok {
		t.Fatalf("usage has no Flags section:\n%s", usage)
	}
	for _, want := range []string{"-h, --help", "--version"} {
		if !strings.Contains(flags, want) {
			t.Errorf("Flags section missing %q:\n%s", want, flags)
		}
	}
}

func TestRunGlobalFlagIgnoresNone(t *testing.T) {
	var stdout bytes.Buffer
	if runGlobalFlag(globalFlagNone, &stdout) {
		t.Fatal("runGlobalFlag(none) = true, want false")
	}
	if stdout.Len() != 0 {
		t.Errorf("runGlobalFlag(none) wrote %q, want nothing", stdout.String())
	}
}
