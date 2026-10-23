package palette

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestPaletteAliasesRankAfterNames(t *testing.T) {
	usage := NewUsageStore("")
	for range 100 {
		usage.Bump("alias")
	}
	commands := []PaletteCommand{
		{ID: "help", Name: "Help", Pinned: PinTop},
		{ID: "name", Name: "Refresh library"},
		{ID: "alias", Name: "Rescan music", Aliases: []string{"refresh library", "refresh library"}},
		{ID: "other", Name: "Pause playback"},
		{ID: "quit", Name: "Quit", Pinned: PinBottom},
	}
	cp, _ := NewCommandPalette(commands, usage)
	if cp.Filtered[0].ID != "alias" {
		t.Fatal("empty query should retain usage ranking")
	}
	cp.Filter.Text = "  REFRESH LIBRARY  "
	cp.Refilter()
	want := []string{"name", "alias"}
	if len(cp.Filtered) != len(want) {
		t.Fatalf("results = %v", cp.Filtered)
	}
	for i, id := range want {
		if cp.Filtered[i].ID != id {
			t.Fatalf("result %d = %s, want %s", i, cp.Filtered[i].ID, id)
		}
	}
	if cp.cursor != 0 {
		t.Fatal("query should focus the first name match")
	}
}

func TestPaletteAliasOnlyMatchPreservesCommandAndAvailability(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		usage := NewUsageStore("")
		command := PaletteCommand{ID: "rescan", Name: "Rescan library", Aliases: []string{"refresh collection"}, Enabled: func() bool { return enabled }, DisabledReason: "Wait for scan"}
		cp, _ := NewCommandPalette([]PaletteCommand{command}, usage)
		cp.Filter.Text = "refresh collection"
		cp.Refilter()
		if len(cp.Filtered) != 1 || cp.Filtered[0].Name != command.Name || cp.Filtered[0].ID != command.ID {
			t.Fatal("alias match should retain the original command")
		}
		_, cmd := cp.HandleKey("enter", tea.KeyPressMsg{})
		if !enabled {
			if cmd != nil || usage.Get("rescan").Count != 0 || cp.Filtered[0].detailText() != "Wait for scan" {
				t.Fatal("disabled alias result should keep its reason and refuse execution")
			}
			continue
		}
		if cmd == nil || cmd().(CommandPalettePickedMsg).Command.ID != "rescan" || usage.Get("rescan").Count != 1 {
			t.Fatal("alias selection should dispatch and track usage by the original ID")
		}
	}
}

func TestPaletteExactAliasImprovesFuzzyNameHit(t *testing.T) {
	cp, _ := NewCommandPalette([]PaletteCommand{
		{ID: "queue", Name: "Clear queue"},
		{ID: "equalizer", Name: "Toggle equalizer", Aliases: []string{"EQ"}},
		{ID: "exact", Name: "EQ"},
	}, nil)
	cp.Filter.Text = "eq"
	cp.Refilter()
	for i, want := range []string{"exact", "equalizer", "queue"} {
		if cp.Filtered[i].ID != want {
			t.Fatalf("result %d = %s, want %s", i, cp.Filtered[i].ID, want)
		}
	}
}
