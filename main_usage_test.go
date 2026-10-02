package main

import (
	"bytes"
	"strings"
	"testing"
)

// TestPrintUsage_ControlLine pins the --control help line. muse.app's --help
// probe takes the first token of a line to be the flag name, so the line must
// start with "--control" followed by its PATH placeholder.
func TestPrintUsage_ControlLine(t *testing.T) {
	var out bytes.Buffer
	printUsage(&out)

	for line := range strings.SplitSeq(out.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "--control" {
			continue
		}
		if !strings.HasPrefix(line, "  --control PATH ") {
			t.Fatalf("--control help line = %q, want prefix %q", line, "  --control PATH ")
		}
		return
	}
	t.Fatalf("help text has no --control line:\n%s", out.String())
}

// TestPrintUsage_HidesBuildInfo keeps the build-info subcommand out of help.
func TestPrintUsage_HidesBuildInfo(t *testing.T) {
	var out bytes.Buffer
	printUsage(&out)
	if strings.Contains(out.String(), "build-info") {
		t.Fatalf("help text must not list build-info:\n%s", out.String())
	}
}
