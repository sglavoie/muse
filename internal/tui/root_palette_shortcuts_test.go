package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/keys"
)

func TestPaletteOnlyAdvertisesShortcutsThatWorkHere(t *testing.T) {
	for _, id := range viewOrder {
		m := newRichFixture().switchToView(id)
		for _, entry := range paletteCatalog {
			shortcut := m.paletteShortcutFor(entry)
			if shortcut != "" && m.resolveKey(shortcut) != entry.act {
				t.Fatalf("view %d: misleading key %q for %s", id, shortcut, entry.id)
			}
		}
	}
	m := newRichFixture()
	for _, entry := range paletteCatalog {
		if entry.act == keys.ActionTagCreate || entry.act == keys.ActionPlaylistCreate {
			if m.paletteShortcutFor(entry) != "" {
				t.Fatal("foreign view key advertised in Library")
			}
		}
	}
}

func TestPaletteTagFilterShortcutNamesItsCurrentAction(t *testing.T) {
	m := newRichFixture()
	for _, active := range []bool{false, true} {
		m.activeTagFilter = nil
		if active {
			id := m.tags[0].ID
			m.activeTagFilter = &id
		}
		for _, cmd := range m.BuildPaletteCommands() {
			if cmd.ID != "tag.filter.open" && cmd.ID != "tag.filter.clear" {
				continue
			}
			want := ""
			if (cmd.ID == "tag.filter.clear") == active {
				want = "ctrl+t"
			}
			if cmd.Shortcut != want {
				t.Fatalf("active=%v %s shortcut=%q want %q", active, cmd.ID, cmd.Shortcut, want)
			}
		}
	}
}
