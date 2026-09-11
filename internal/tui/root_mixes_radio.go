package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// buildMixSeed computes a synthetic radio.SeedTrack from the aggregate metadata
// of the given tracks. It uses the most-common artist, union of genres, median
// duration, and union of tag IDs. The returned SeedTrack has no Hash (it is not
// a real library track), so no candidate is excluded from scoring by hash match.
func buildMixSeed(tracks []library.Track, trackTags map[string]map[int64]struct{}) radio.SeedTrack {
	// radio.AggregateSeed owns the aggregation so the legacy radio path and the
	// DJ's collection seed cannot drift apart. It also deduplicates by content
	// hash and breaks the artist tally's ties on the name, where the version
	// that lived here counted a repeated track twice and let Go's map iteration
	// order decide between two equally common artists — which made the same mix
	// produce a different seed from one press to the next.
	return radio.AggregateSeed(tracks, trackTags)
}

// handleMixesRadioSeed starts radio mode seeded from the focused mix's
// aggregate metadata (top artist, union of genres, median duration, union of
// tags). If radio is already enabled, it delegates to radioToggle to disable
// it. If the mix cache is empty it also falls back to radioToggle.
func (m Model) handleMixesRadioSeed() (Model, tea.Cmd) {
	if m.radio.enabled {
		tm, cmd, _ := m.radioToggle()
		return tm.(Model), cmd
	}
	tracks, ok := m.mixes.CachedMixes[m.mixes.ActiveMix]
	if !ok || len(tracks) == 0 {
		tm, cmd, _ := m.radioToggle()
		return tm.(Model), cmd
	}
	var mixName string
	for _, info := range m.mixes.Mixes {
		if info.ID == m.mixes.ActiveMix {
			mixName = info.Name
			break
		}
	}
	// A mix is an explicitly targeted collection, so it outranks the row under
	// the cursor; only a bulk selection inside the mix is more specific, which
	// the shared start checks for.
	return m.djStartFromCollection("Mixes ▸ "+mixName, tracks, time.Now())
}
