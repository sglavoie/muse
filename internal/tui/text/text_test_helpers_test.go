package text

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func requireDisplayWidth(t *testing.T, got string, want int) {
	t.Helper()

	if width := ansi.StringWidth(got); width != want {
		t.Fatalf("display width = %d, want %d (got %q)", width, want, got)
	}
}

func trimmedLine(out string) string {
	return strings.TrimRight(out, "\n")
}
