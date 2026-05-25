package albums

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestStripFilterActive(t *testing.T) {
	v := View{
		Level:        AlbumLevel,
		Albums:       []string{"a"},
		FilterActive: true,
		FilterText:   "test",
		FilterView:   "test",
		Width:        80,
		Styles:       styles.NewStyles(true),
	}

	out := ansi.Strip(v.renderStateStrip(1, 1))
	if !strings.Contains(out, `filter "test"`) {
		t.Errorf("expected filter chunk, got: %q", out)
	}
}

func TestStripFilterCommitted(t *testing.T) {
	v := View{
		Level:      AlbumLevel,
		Albums:     []string{"a"},
		FilterText: "test",
		Width:      80,
		Styles:     styles.NewStyles(true),
	}

	out := ansi.Strip(v.renderStateStrip(1, 1))
	if !strings.Contains(out, `filter "test"`) {
		t.Errorf("expected committed filter chunk, got: %q", out)
	}
}
