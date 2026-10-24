package insights

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// TestSessionRowFillsWidthWithMultibyteNote verifies that the "·" in a
// session note is measured by display width, so the row fills the panel
// instead of coming up short by the note's extra bytes.
func TestSessionRowFillsWidthWithMultibyteNote(t *testing.T) {
	v := sessionsFixtureView()
	v.SetSize(80, 20)

	row := v.renderSessionRow(styles.NewStyles(false), v.Sessions[0], false)
	if got := ansi.StringWidth(row); got != 80 {
		t.Errorf("session row width = %d, want 80: %q", got, ansi.Strip(row))
	}
}
