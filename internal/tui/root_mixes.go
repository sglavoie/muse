package tui

import (
	"log/slog"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recommendation"
)

// mixCountsLoadedMsg carries the per-mix track counts returned by
// loadMixCountsCmd. insufficient is true when the database does not yet
// have enough play history to generate meaningful recommendations.
type mixCountsLoadedMsg struct {
	counts       map[db.MixID]int
	progress     map[db.MixID]db.MixProgress
	insufficient bool
}

// mixLoadedMsg carries the tracks for a specific mix loaded on drill-down.
type mixLoadedMsg struct {
	mixID  db.MixID
	tracks []library.Track
}

// recommendationCacheRebuiltMsg is sent when the async cache rebuild completes.
type recommendationCacheRebuiltMsg struct{}

// mixesMaybeLoadCmd returns loadMixCountsCmd when a reload is warranted
// (no mixes loaded yet, or enough plays have accumulated since the last load).
// When a cacheRebuilder is wired it is also batched in so the recommendation
// cache is refreshed alongside the counts.
// Returns nil when recReader is not wired or no reload is needed.
func (m Model) mixesMaybeLoadCmd() tea.Cmd {
	if m.recReader == nil {
		return nil
	}
	if len(m.mixes.Mixes) == 0 || m.mixes.PlaysSinceRefresh >= 5 {
		loadCmd := m.loadMixCountsCmd()
		rebuildCmd := m.rebuildCacheCmd()
		if rebuildCmd != nil {
			return tea.Batch(loadCmd, rebuildCmd)
		}
		return loadCmd
	}
	return nil
}

// rebuildCacheCmd runs every mix query (limit 200), assigns rank-based scores,
// and persists the result to recommendation_cache. Returns nil when either
// recReader or cacheRebuilder is not wired.
func (m Model) rebuildCacheCmd() tea.Cmd {
	rr := m.recReader
	cr := m.cacheRebuilder
	if rr == nil || cr == nil {
		return nil
	}
	mx := m.cfg.Mixes
	return func() tea.Msg {
		const limit = 200
		type mixLoader struct {
			id   db.MixID
			load func() ([]library.Track, error)
		}
		loaders := []mixLoader{
			{db.MixRediscover, func() ([]library.Track, error) {
				return rr.ReadRediscoverMix(mx.RediscoverMinPlays, mx.RediscoverAgeDays, limit)
			}},
			{db.MixHiddenGems, func() ([]library.Track, error) {
				return rr.ReadHiddenGemsMix(mx.HiddenGemsMaxPlays, mx.HiddenGemsTopArtists, mx.HiddenGemsTopGenres, limit)
			}},
			{db.MixOnRepeat, func() ([]library.Track, error) {
				return rr.ReadOnRepeatMix(mx.OnRepeatMinPlays, mx.OnRepeatWindowDays, limit)
			}},
			{db.MixDeepCuts, func() ([]library.Track, error) {
				return rr.ReadDeepCutsMix(mx.DeepCutsMinPlays, mx.DeepCutsMaxPlays, mx.DeepCutsMinCompletion, limit)
			}},
			{db.MixNeverPlayed, func() ([]library.Track, error) {
				return rr.ReadNeverPlayedMix(limit)
			}},
		}

		var entries []db.CacheEntry
		for _, l := range loaders {
			tracks, err := l.load()
			if err != nil {
				slog.Warn("mixes: rebuild cache load", "mixID", l.id, "err", err)
				continue
			}
			n := len(tracks)
			for rank, t := range tracks {
				entries = append(entries, db.CacheEntry{
					MixID:     int(l.id),
					TrackHash: t.Hash,
					Score:     float64(n - rank),
				})
			}
		}

		if err := cr.RebuildRecommendationCache(entries); err != nil {
			slog.Warn("mixes: rebuild cache write", "err", err)
		}
		return recommendationCacheRebuiltMsg{}
	}
}

// loadMixCountsCmd queries each recommendation mix (limit 100) to obtain
// track counts for the list view. Also checks HasSufficientHistory.
func (m Model) loadMixCountsCmd() tea.Cmd {
	rr := m.recReader
	if rr == nil {
		return nil
	}
	mx := m.cfg.Mixes
	return func() tea.Msg {
		sufficient, err := rr.HasSufficientHistory(mx.SufficientHistoryCount)
		if err != nil {
			slog.Warn("mixes: HasSufficientHistory", "err", err)
		}

		progress, perr := rr.ReadAllMixProgress()
		if perr != nil {
			slog.Warn("mixes: ReadAllMixProgress", "err", perr)
		}

		if !sufficient {
			return mixCountsLoadedMsg{insufficient: true, progress: progress}
		}

		counts := make(map[db.MixID]int, len(recommendation.Registry()))
		loaders := []struct {
			id   db.MixID
			load func(int) ([]library.Track, error)
		}{
			{db.MixRediscover, func(limit int) ([]library.Track, error) {
				return rr.ReadRediscoverMix(mx.RediscoverMinPlays, mx.RediscoverAgeDays, limit)
			}},
			{db.MixHiddenGems, func(limit int) ([]library.Track, error) {
				return rr.ReadHiddenGemsMix(mx.HiddenGemsMaxPlays, mx.HiddenGemsTopArtists, mx.HiddenGemsTopGenres, limit)
			}},
			{db.MixOnRepeat, func(limit int) ([]library.Track, error) {
				return rr.ReadOnRepeatMix(mx.OnRepeatMinPlays, mx.OnRepeatWindowDays, limit)
			}},
			{db.MixDeepCuts, func(limit int) ([]library.Track, error) {
				return rr.ReadDeepCutsMix(mx.DeepCutsMinPlays, mx.DeepCutsMaxPlays, mx.DeepCutsMinCompletion, limit)
			}},
			{db.MixNeverPlayed, rr.ReadNeverPlayedMix},
		}
		for _, l := range loaders {
			tracks, lerr := l.load(100)
			if lerr != nil {
				slog.Warn("mixes: load count", "mixID", l.id, "err", lerr)
				counts[l.id] = 0
			} else {
				counts[l.id] = len(tracks)
			}
		}
		return mixCountsLoadedMsg{counts: counts, progress: progress}
	}
}

// loadMixCmd queries the full track list for a specific mix on drill-down.
// It tries the recommendation_cache first (fast path); if the cache is empty
// it falls back to the live query.
func (m Model) loadMixCmd(mixID db.MixID) tea.Cmd {
	rr := m.recReader
	if rr == nil {
		return nil
	}
	cr := m.cacheReader
	mx := m.cfg.Mixes
	return func() tea.Msg {
		const limit = 200

		// Fast path: pre-computed cache.
		if cr != nil {
			cached, err := cr.ReadCachedMix(int(mixID), limit)
			if err == nil && len(cached) > 0 {
				return mixLoadedMsg{mixID: mixID, tracks: cached}
			}
		}

		// Slow path: live query.
		var tracks []library.Track
		var err error
		switch mixID {
		case db.MixRediscover:
			tracks, err = rr.ReadRediscoverMix(mx.RediscoverMinPlays, mx.RediscoverAgeDays, limit)
		case db.MixHiddenGems:
			tracks, err = rr.ReadHiddenGemsMix(mx.HiddenGemsMaxPlays, mx.HiddenGemsTopArtists, mx.HiddenGemsTopGenres, limit)
		case db.MixOnRepeat:
			tracks, err = rr.ReadOnRepeatMix(mx.OnRepeatMinPlays, mx.OnRepeatWindowDays, limit)
		case db.MixDeepCuts:
			tracks, err = rr.ReadDeepCutsMix(mx.DeepCutsMinPlays, mx.DeepCutsMaxPlays, mx.DeepCutsMinCompletion, limit)
		case db.MixNeverPlayed:
			tracks, err = rr.ReadNeverPlayedMix(limit)
		default:
			return nil
		}
		if err != nil {
			slog.Warn("mixes: load mix", "mixID", mixID, "err", err)
			return mixLoadedMsg{mixID: mixID, tracks: nil}
		}
		return mixLoadedMsg{mixID: mixID, tracks: tracks}
	}
}

// handleMixCountsLoadedMsg populates mix track counts and progress strings
// into the list view and resets playsSinceRefresh since the data is now fresh.
func (m Model) handleMixCountsLoadedMsg(msg mixCountsLoadedMsg) Model {
	m.mixes.InsufficientData = msg.insufficient
	for i := range m.mixes.Mixes {
		id := m.mixes.Mixes[i].ID
		if !msg.insufficient && msg.counts != nil {
			m.mixes.Mixes[i].TrackCount = msg.counts[id]
		}
		if msg.progress != nil {
			m.mixes.Mixes[i].Progress = msg.progress[id].Description
		}
	}
	m.mixes.PlaysSinceRefresh = 0
	return m
}

// handleMixLoadedMsg stores the loaded tracks in the cache and, when the
// user is still on the Mixes view at drill-down level for this mix,
// populates the track view (bug hotspot #42: async message validation).
// Dismissed tracks are filtered out before caching.
func (m Model) handleMixLoadedMsg(msg mixLoadedMsg) Model {
	if m.mixes.CachedMixes == nil {
		m.mixes.CachedMixes = make(map[db.MixID][]library.Track)
	}
	tracks := recommendation.FilterSeen(msg.tracks, m.dismissedHashes)
	m.mixes.CachedMixes[msg.mixID] = tracks
	if m.ActiveView == viewMixes && m.mixes.Level == 1 && m.mixes.ActiveMix == msg.mixID {
		if len(m.mixes.TrackView.SourceTracks) == 0 {
			// Initial population: no user state to preserve.
			m.mixes.OpenMix(msg.mixID)
		} else {
			// Re-load while user is already browsing: update tracks
			// in place to preserve cursor, sort, and filter state.
			m.mixes.TrackView.SourceTracks = tracks
			m.mixes.TrackView = m.mixes.TrackView.RefreshView().ClampCursor()
		}
	}
	return m
}
