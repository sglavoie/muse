package tui

import (
	"log/slog"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// buildMixSeed computes a synthetic radio.SeedTrack from the aggregate metadata
// of the given tracks. It uses the most-common artist, union of genres, median
// duration, and union of tag IDs. The returned SeedTrack has no Hash (it is not
// a real library track), so no candidate is excluded from scoring by hash match.
func buildMixSeed(tracks []library.Track, trackTags map[string]map[int64]struct{}) radio.SeedTrack {
	// Most-common artist.
	artistCount := make(map[string]int, len(tracks))
	for _, t := range tracks {
		if t.Artist != "" {
			artistCount[t.Artist]++
		}
	}
	var topArtist string
	var maxCount int
	for a, c := range artistCount {
		if c > maxCount {
			maxCount = c
			topArtist = a
		}
	}

	// Union of genres.
	genreSet := make(map[string]struct{})
	for _, t := range tracks {
		for _, g := range library.SplitGenres(t.Genre) {
			genreSet[g] = struct{}{}
		}
	}
	genres := make([]string, 0, len(genreSet))
	for g := range genreSet {
		genres = append(genres, g)
	}

	// Median duration.
	durs := make([]float64, 0, len(tracks))
	for _, t := range tracks {
		if t.Duration > 0 {
			durs = append(durs, t.Duration)
		}
	}
	sort.Float64s(durs)
	var medianDur float64
	if len(durs) > 0 {
		medianDur = durs[len(durs)/2]
	}

	// Union of tag IDs.
	tagSet := make(map[int64]struct{})
	for _, t := range tracks {
		for id := range trackTags[t.Hash] {
			tagSet[id] = struct{}{}
		}
	}

	return radio.SeedTrack{
		Artist:   topArtist,
		Genres:   genres,
		Duration: medianDur,
		TagIDs:   tagSet,
		// Hash intentionally empty: synthetic seed, not a real track.
	}
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
	tracks, ok := m.mixes.cachedMixes[m.mixes.activeMix]
	if !ok || len(tracks) == 0 {
		tm, cmd, _ := m.radioToggle()
		return tm.(Model), cmd
	}
	seed := buildMixSeed(tracks, m.trackTags)
	m.radio.syntheticSeed = &seed
	m.radio.enabled = true
	m.radio.seedHash = ""

	var mixName string
	for _, info := range m.mixes.mixes {
		if info.ID == m.mixes.activeMix {
			mixName = info.Name
			break
		}
	}
	if m.sessionWriter != nil {
		if err := m.sessionWriter.WriteRadioMode(true); err != nil {
			slog.Warn("session: write radio mode", "err", err)
		}
	}
	m.statusBar.PushMsg("Radio: seeded from "+mixName, time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second, statusbar.NotifInfo)
	cmd := m.radioMaybeEnqueueCmd()
	if cmd != nil {
		m.radio.enqueuePending = true
	}
	return m, cmd
}
