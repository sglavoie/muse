package insights

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func TestPanelSelectorGivesContentRoomAtOrdinaryHeights(t *testing.T) {
	v := rankingsFixtureView(PanelConcentration)
	v.Loaded = true
	v.Snap.TotalPlays = 10
	s := styles.NewStyles(false)
	v.SetSize(80, 19) // 80x24 after root chrome
	out := ansi.Strip(v.Render(s, nil, nil, nil, "", ""))
	if !strings.Contains(out, "‹ Concentration · 4/8 ›") || strings.Contains(out, "Time vs Plays") {
		t.Fatalf("expected compact selector, not the full briefing:\n%s", out)
	}
	if strings.Count(out, "▎ ") != 1 {
		t.Fatalf("expected exactly one focused row:\n%s", out)
	}
	v.SetSize(100, 35)
	out = ansi.Strip(v.Render(s, nil, nil, nil, "", ""))
	if !strings.Contains(out, "[Concentration]") || !strings.Contains(out, "Time vs Plays") {
		t.Fatalf("tall terminal lost the full briefing:\n%s", out)
	}
}

func TestPanelSectionsNeverEndWithAnOrphanHeading(t *testing.T) {
	v := rankingsFixtureView(PanelConcentration)
	s := styles.NewStyles(false)
	rows := v.Rows()
	for height := 10; height <= 35; height++ {
		v.SetSize(80, height)
		for cursor := range rows {
			v.Cursor = cursor
			v.ClampCursor()
			out := ansi.Strip(v.renderPanel(s))
			lines := strings.Split(strings.TrimSpace(out), "\n")
			last := strings.TrimSpace(lines[len(lines)-1])
			for _, row := range rows {
				if last == row.Section {
					t.Fatalf("height %d cursor %d: orphan heading %q", height, cursor, last)
				}
			}
			if !strings.Contains(out, "▎ "+rows[cursor].Label) {
				t.Fatalf("height %d cursor %d lost focus:\n%s", height, cursor, out)
			}
		}
	}
}
