package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/palette"
)

func TestPaletteSearchSynonyms(t *testing.T) {
	m := newModelWithTracks()
	for _, tc := range []struct{ query, id string }{
		{"refresh library", "library.rescan"},
		{"scan music", "library.rescan"},
		{"eq", "audio.toggle.eq"},
		{"auto dj", "radio.toggle"},
		{"toggle radio mode", "radio.toggle"},
		{"start radio from mix", "mix.radio.seed"},
		{"start radio from insight row", "insights.radio.seed"},
		{"stats", "nav.insights"},
		{"autodj", "radio.toggle"},
		{"tune radio", "autodj.tune"},
		{"errors", "messages.show"},
		{"restart song", "playback.seek.start"},
		{"locate file", "track.reveal"},
	} {
		t.Run(tc.query, func(t *testing.T) {
			cp, _ := palette.NewCommandPalette(m.BuildPaletteCommands(), nil)
			cp.Filter.Text = tc.query
			cp.Refilter()
			for _, command := range cp.Filtered {
				if command.ID == tc.id {
					return
				}
			}
			t.Fatalf("%q did not find %s", tc.query, tc.id)
		})
	}
}
