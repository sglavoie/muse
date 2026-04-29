package tui

import (
	"fmt"
	"log/slog"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
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
	if len(m.mixes.mixes) == 0 || m.mixes.playsSinceRefresh >= 5 {
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
	m.mixes.insufficientData = msg.insufficient
	for i := range m.mixes.mixes {
		id := m.mixes.mixes[i].ID
		if !msg.insufficient && msg.counts != nil {
			m.mixes.mixes[i].TrackCount = msg.counts[id]
		}
		if msg.progress != nil {
			m.mixes.mixes[i].Progress = msg.progress[id].Description
		}
	}
	m.mixes.playsSinceRefresh = 0
	return m
}

// handleMixLoadedMsg stores the loaded tracks in the cache and, when the
// user is still on the Mixes view at drill-down level for this mix,
// populates the track view (bug hotspot #42: async message validation).
// Dismissed tracks are filtered out before caching.
func (m Model) handleMixLoadedMsg(msg mixLoadedMsg) Model {
	if m.mixes.cachedMixes == nil {
		m.mixes.cachedMixes = make(map[db.MixID][]library.Track)
	}
	tracks := msg.tracks
	if len(m.dismissedHashes) > 0 {
		filtered := make([]library.Track, 0, len(tracks))
		for _, t := range tracks {
			if _, dismissed := m.dismissedHashes[t.Hash]; !dismissed {
				filtered = append(filtered, t)
			}
		}
		tracks = filtered
	}
	m.mixes.cachedMixes[msg.mixID] = tracks
	if m.activeView == viewMixes && m.mixes.level == 1 && m.mixes.activeMix == msg.mixID {
		m.mixes.openMix(msg.mixID)
	}
	return m
}

// handleMixOpen drills into the track list for the focused mix.
func (m Model) handleMixOpen() (Model, tea.Cmd) {
	if m.activeView != viewMixes || m.mixes.level != 0 {
		return m, nil
	}
	mix := m.mixes.focusedMix()
	if mix == nil {
		return m, nil
	}
	m.mixes.openMix(mix.ID)
	return m, m.loadMixCmd(mix.ID)
}

// handleMixBack returns from the track list to the mix list.
func (m Model) handleMixBack() Model {
	if m.activeView != viewMixes {
		return m
	}
	m.mixes.back()
	return m
}

// mixSavedAsPlaylistMsg is returned when a mix is successfully saved as a
// static playlist. It carries the refreshed playlist list, the new playlist
// name, and the number of tracks saved.
type mixSavedAsPlaylistMsg struct {
	playlists    []db.PlaylistRow
	playlistName string
	count        int
}

// mixSaveAsPlaylistErrorMsg is returned when saving a mix as a playlist fails.
type mixSaveAsPlaylistErrorMsg struct{ err error }

// handleMixesSaveMixAsPlaylist creates a static playlist from the tracks of
// the currently open mix (level 1 only). It is a no-op at the list level or
// when the mix cache is empty.
func (m Model) handleMixesSaveMixAsPlaylist() (Model, tea.Cmd) {
	if m.mixes.level != 1 {
		return m, nil
	}
	tracks, ok := m.mixes.cachedMixes[m.mixes.activeMix]
	if !ok || len(tracks) == 0 {
		m.statusBar.notify("No tracks to save")
		return m, nil
	}
	var mixName string
	for _, info := range m.mixes.mixes {
		if info.ID == m.mixes.activeMix {
			mixName = info.Name
			break
		}
	}
	name := "Mix: " + mixName
	hashes := make([]string, len(tracks))
	for i, t := range tracks {
		hashes[i] = t.Hash
	}
	count := len(hashes)
	pw := m.playlistWriter
	pa := m.playlistAdder
	if pw == nil {
		m.statusBar.notify("Playlist writer not available")
		return m, nil
	}
	return m, func() tea.Msg {
		id, err := pw.CreatePlaylist(name)
		if err != nil {
			return mixSaveAsPlaylistErrorMsg{err: err}
		}
		if pa != nil && len(hashes) > 0 {
			if err := pa.AddTracksToPlaylist(id, hashes); err != nil {
				return mixSaveAsPlaylistErrorMsg{err: err}
			}
		}
		rows, err := pw.ReadPlaylists()
		if err != nil {
			return mixSaveAsPlaylistErrorMsg{err: err}
		}
		return mixSavedAsPlaylistMsg{playlists: rows, playlistName: name, count: count}
	}
}

// handleMixEnqueueAll enqueues all cached tracks from the focused mix (level 0).
func (m Model) handleMixEnqueueAll() (Model, tea.Cmd) {
	if m.activeView != viewMixes || m.mixes.level != 0 {
		return m, nil
	}
	mix := m.mixes.focusedMix()
	if mix == nil {
		return m, nil
	}
	be := m.batchEnqueuer
	if be == nil {
		m.statusBar.notify("Enqueue not available")
		return m, nil
	}
	cached, ok := m.mixes.cachedMixes[mix.ID]
	if !ok || len(cached) == 0 {
		m.statusBar.notify("Load the mix first (press Enter)")
		return m, nil
	}
	wasEmpty := len(m.queue.entries) == 0
	mixName := mix.Name
	tracks := cached
	return m, func() tea.Msg {
		ids, err := be.AppendTracks(tracks, mixName)
		if err != nil {
			return BatchEnqueuedMsg{Err: fmt.Errorf("enqueue mix: %w", err)}
		}
		return BatchEnqueuedMsg{
			Entries:      newQueueEntries(tracks, ids, mixName),
			QueueIDs:     ids,
			WasEmpty:     wasEmpty,
			PlaylistName: mixName,
		}
	}
}

// handleMixSavedAsPlaylistMsg updates the playlist list and shows a success
// notification after a mix is saved as a static playlist.
func (m Model) handleMixSavedAsPlaylistMsg(msg mixSavedAsPlaylistMsg) Model {
	m.playlists.playlists = msg.playlists
	m = m.preservePlaylistsSort()
	m.statusBar.notify(fmt.Sprintf("Saved as %q (%d %s)", msg.playlistName, msg.count, pluralize(msg.count, "track", "tracks")))
	return m
}

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

// loadDismissedSet reads dismissed track hashes from the database and returns
// them as a set. Returns nil when reader is nil or on error.
func loadDismissedSet(reader dismissalReader) map[string]struct{} {
	if reader == nil {
		return nil
	}
	set, err := reader.ReadDismissedTrackHashes()
	if err != nil {
		return nil
	}
	return set
}

// mixDismissedMsg is sent when a track has been successfully dismissed.
type mixDismissedMsg struct{ hash string }

// mixDismissErrorMsg is sent when a dismiss DB write fails.
type mixDismissErrorMsg struct {
	hash string
	err  error
}

// mixUndismissedMsg is sent when a track has been successfully undismissed.
type mixUndismissedMsg struct{ hash string }

// mixUndismissErrorMsg is sent when an undismiss DB write fails.
type mixUndismissErrorMsg struct{ err error }

// handleMixDismiss dismisses the focused track at level 1. It writes to the
// database asynchronously and, on success, removes the track from the cached
// mix and the in-memory dismissedHashes set (bug hotspot #42: async validation).
func (m Model) handleMixDismiss() (Model, tea.Cmd) {
	if m.activeView != viewMixes || m.mixes.level != 1 {
		return m, nil
	}
	t := m.mixes.focusedTrack()
	if t == nil {
		return m, nil
	}
	dw := m.dismissalWriter
	if dw == nil {
		m.statusBar.notify("Dismissal not available")
		return m, nil
	}
	hash := t.Hash
	return m, func() tea.Msg {
		if err := dw.DismissTrack(hash); err != nil {
			return mixDismissErrorMsg{hash: hash, err: err}
		}
		return mixDismissedMsg{hash: hash}
	}
}

// handleMixDismissedMsg updates in-memory state after a successful dismiss:
// adds the hash to dismissedHashes and removes the track from the open mix.
func (m Model) handleMixDismissedMsg(msg mixDismissedMsg) Model {
	if m.dismissedHashes == nil {
		m.dismissedHashes = make(map[string]struct{})
	}
	m.dismissedHashes[msg.hash] = struct{}{}

	if tracks, ok := m.mixes.cachedMixes[m.mixes.activeMix]; ok {
		updated := removeTrackByHash(tracks, msg.hash)
		m.mixes.cachedMixes[m.mixes.activeMix] = updated
		// Refresh the track view if currently open on this mix.
		if m.activeView == viewMixes && m.mixes.level == 1 {
			m.mixes.openMix(m.mixes.activeMix)
		}
	}
	m.statusBar.notify("Dismissed — won't appear in future mixes")
	return m
}

// handleMixUndismiss undismisses the focused track at level 1. The track
// reappears on the next mix load.
func (m Model) handleMixUndismiss() (Model, tea.Cmd) {
	if m.activeView != viewMixes || m.mixes.level != 1 {
		return m, nil
	}
	t := m.mixes.focusedTrack()
	if t == nil {
		return m, nil
	}
	dw := m.dismissalWriter
	if dw == nil {
		m.statusBar.notify("Dismissal not available")
		return m, nil
	}
	hash := t.Hash
	return m, func() tea.Msg {
		if err := dw.UndismissTrack(hash); err != nil {
			return mixUndismissErrorMsg{err: err}
		}
		return mixUndismissedMsg{hash: hash}
	}
}

// handleMixUndismissedMsg removes the hash from in-memory dismissedHashes.
func (m Model) handleMixUndismissedMsg(msg mixUndismissedMsg) Model {
	delete(m.dismissedHashes, msg.hash)
	m.statusBar.notify("Undismissed — will reappear on next mix load")
	return m
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
	m.statusBar.notification = newNotificationFor(
		"Radio: seeded from "+mixName,
		time.Duration(m.cfg.VolumeFeedbackDurationSecs)*time.Second,
	)
	cmd := m.radioMaybeEnqueueCmd()
	if cmd != nil {
		m.radio.enqueuePending = true
	}
	return m, cmd
}
