package strip

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func newTestStyles() styles.Styles {
	return styles.NewStyles(true)
}

func requireRenderWidth(t *testing.T, got string, want int) string {
	t.Helper()

	plain := ansi.Strip(got)
	if ansi.StringWidth(got) != want {
		t.Fatalf("width = %d, want %d (plain=%q)", ansi.StringWidth(got), want, plain)
	}

	return plain
}
