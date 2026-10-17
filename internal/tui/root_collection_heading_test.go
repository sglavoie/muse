package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/keys"
)

func TestCollectionHeadingKeepsFilteredLastRowVisible(t *testing.T) {
	for _, view := range []viewID{viewTags, viewPlaylists} {
		m := newRichFixture()
		m.termWidth, m.termHeight = 60, 15
		m.setViewDimensions(60, 15)
		m.ActiveView = view
		m.playlists.Playlists = nil
		m.tagsView.Tags = nil
		m.tagsView.TagCounts = map[int64]int{}
		for i := range 30 {
			name := fmt.Sprintf("Collection %02d", i)
			id := int64(i + 1)
			m.playlists.Playlists = append(m.playlists.Playlists, db.PlaylistRow{ID: id, Name: name, TrackCount: 1})
			m.tagsView.Tags = append(m.tagsView.Tags, db.TagRow{ID: id, Name: name})
			m.tagsView.TagCounts[id] = 1
		}
		m.playlists.Filter = filter.State{Text: "Collection"}
		m.tagsView.Filter = filter.State{Text: "Collection"}
		next, _ := m.handleKeyAction(keys.ActionGoToBottom)
		m = next.(Model)
		m.clampAllScrollOffsets()
		screen := ansi.Strip(m.View().Content)
		for _, want := range []string{ViewName(view), "Collection 29", "Filter:"} {
			if !strings.Contains(screen, want) {
				t.Fatalf("view %v missing %q:\n%s", view, want, screen)
			}
		}
		lines := strings.Split(screen, "\n")
		if len(lines) > 15 {
			t.Fatalf("view %v too tall:\n%s", view, screen)
		}
		for _, line := range lines {
			if ansi.StringWidth(line) > 60 {
				t.Fatalf("view %v too wide: %s", view, line)
			}
		}
	}
}
