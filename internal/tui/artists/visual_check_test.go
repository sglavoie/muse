package artists

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestVisualCheck(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	v := View{
		Level:        ArtistLevel,
		Width:        50,
		Cursor:       1,
		Artists:      []string{"Pink Floyd", "The Beatles", "Mystery Band"},
		ArtistCounts: map[string]int{"Pink Floyd": 23, "The Beatles": 4, "Mystery Band": 0},
		Styles:       styles.NewStyles(true),
	}
	out := v.Render()
	for _, line := range strings.Split(out, "\n") {
		stripped := ansi.Strip(line)
		fmt.Printf("[%2d] %q\n", ansi.StringWidth(stripped), stripped)
	}
}
