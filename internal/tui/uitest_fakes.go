package tui

// This file provides in-memory fake dependencies for the hidden `muse tui-test`
// layout harness. They implement the narrow reader/writer interfaces the root
// Model depends on, but perform no real I/O: no filesystem access, no database,
// no mpv, no Finder, and no clipboard. Fixtures (see uitest_fixtures.go) wire
// these fakes so a Model can be constructed and driven entirely in-memory.

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// uitestPlayer is a no-op playPauser used by fixtures so that playback-related
// keys are safe to exercise in-memory without launching mpv.
type uitestPlayer struct{}

func (uitestPlayer) Play() error             { return nil }
func (uitestPlayer) Pause() error            { return nil }
func (uitestPlayer) PlayFromIndex(int) error { return nil }

// uitestQueueStore is an in-memory queue writer/dequeuer/reorderer. It records
// nothing and never persists; AppendTrack hands out monotonically increasing
// IDs so enqueue paths have a plausible row ID to work with.
type uitestQueueStore struct{ nextID int64 }

// AppendTrack satisfies libtui.QueueAppender.
func (s *uitestQueueStore) AppendTrack(_ library.Track, _ string) (int64, error) {
	s.nextID++
	return s.nextID, nil
}

// db.QueueDequeuer surface.
func (s *uitestQueueStore) DequeueTrackSoft(int64) error         { return nil }
func (s *uitestQueueStore) UndoDequeue(int64) error              { return nil }
func (s *uitestQueueStore) ClearQueueSoft() ([]int64, error)     { return nil, nil }
func (s *uitestQueueStore) UndoClearQueue([]int64) error         { return nil }
func (s *uitestQueueStore) PurgeDeletedQueueEntries(int64) error { return nil }

// queueReorderer / queueShuffler surface.
func (s *uitestQueueStore) ReorderQueueTrack(int64, int) error { return nil }
func (s *uitestQueueStore) SetQueueOrder([]int64) error        { return nil }

// uitestFavorites is an in-memory favoriteReader + favoriteWriter. Data is keyed
// by favorite type ("artist", "album", "track"); track values are hashes.
type uitestFavorites struct {
	byType map[string][]string
}

func newUITestFavorites(byType map[string][]string) *uitestFavorites {
	if byType == nil {
		byType = map[string][]string{}
	}
	return &uitestFavorites{byType: byType}
}

// favoriteReader surface.
func (f *uitestFavorites) ReadFavoritesByType(typ string) ([]db.FavoriteRow, error) {
	var rows []db.FavoriteRow
	for i, v := range f.byType[typ] {
		rows = append(rows, db.FavoriteRow{ID: int64(i + 1), Type: typ, Value: v})
	}
	return rows, nil
}

func (f *uitestFavorites) ReadFavoriteCounts() (int, int, int, error) {
	return len(f.byType["artist"]), len(f.byType["album"]), len(f.byType["track"]), nil
}

func (f *uitestFavorites) IsFavorite(typ, value string) (bool, error) {
	for _, v := range f.byType[typ] {
		if v == value {
			return true, nil
		}
	}
	return false, nil
}

// favoriteWriter surface.
func (f *uitestFavorites) AddFavorite(typ, value string) error {
	f.byType[typ] = append(f.byType[typ], value)
	return nil
}

func (f *uitestFavorites) RemoveFavorite(typ, value string) error {
	f.byType[typ] = removeString(f.byType[typ], value)
	return nil
}

func (f *uitestFavorites) RemoveFavoritesByTypeAndValues(typ string, values []string) error {
	for _, v := range values {
		f.byType[typ] = removeString(f.byType[typ], v)
	}
	return nil
}

// uitestTags is an in-memory tagReader + tagWriter. Only the read surface is
// meaningfully populated for fixtures; the mutation surface is no-op.
type uitestTags struct {
	tags        []db.TagRow
	trackTags   map[string][]db.TagRow
	counts      map[int64]int
	tracksByTag map[int64][]db.PlaylistTrackRow
	hashesByTag map[int64][]string
	nextID      int64
}

// tagReader surface.
func (t *uitestTags) ReadTags() ([]db.TagRow, error) { return t.tags, nil }
func (t *uitestTags) ReadTagsForTrack(hash string) ([]db.TagRow, error) {
	return t.trackTags[hash], nil
}
func (t *uitestTags) ReadTrackHashesByTag(tagID int64) ([]string, error) {
	return t.hashesByTag[tagID], nil
}
func (t *uitestTags) ReadTracksByTag(tagID int64) ([]db.PlaylistTrackRow, error) {
	return t.tracksByTag[tagID], nil
}
func (t *uitestTags) ReadTagCounts() (map[int64]int, error) { return t.counts, nil }

// tagWriter surface.
func (t *uitestTags) CreateTag(_, _ string) (int64, error)     { t.nextID++; return t.nextID, nil }
func (t *uitestTags) RenameTag(int64, string) error            { return nil }
func (t *uitestTags) UpdateTagColor(int64, string) error       { return nil }
func (t *uitestTags) DeleteTagSoft(int64) error                { return nil }
func (t *uitestTags) DeleteTagIfEmpty(int64) (bool, error)     { return false, nil }
func (t *uitestTags) PurgeDeletedTags(int64) error             { return nil }
func (t *uitestTags) UndoDeleteTag(int64) error                { return nil }
func (t *uitestTags) AddTrackTag(string, int64) error          { return nil }
func (t *uitestTags) RemoveTrackTag(string, int64) error       { return nil }
func (t *uitestTags) RemoveAllTrackTags(string) error          { return nil }
func (t *uitestTags) AddTrackTagBulk([]string, int64) error    { return nil }
func (t *uitestTags) RemoveTrackTagBulk([]string, int64) error { return nil }
func (t *uitestTags) MergeTags(int64, int64) error             { return nil }

// uitestPlaylistTracks is an in-memory playlistTrackReader keyed by playlist ID.
type uitestPlaylistTracks struct {
	byID map[int64][]db.PlaylistTrackRow
}

func (r *uitestPlaylistTracks) ReadPlaylistTracks(playlistID int64) ([]db.PlaylistTrackRow, error) {
	return r.byID[playlistID], nil
}

// uitestRecs is an in-memory recommendationReader. Every mix query returns the
// same deterministic track slice, which is enough to render the Mixes view.
type uitestRecs struct {
	sufficient bool
	tracks     []library.Track
	progress   map[db.MixID]db.MixProgress
}

func (r *uitestRecs) HasSufficientHistory(int) (bool, error) { return r.sufficient, nil }
func (r *uitestRecs) ReadRediscoverMix(int, int, int) ([]library.Track, error) {
	return r.tracks, nil
}
func (r *uitestRecs) ReadHiddenGemsMix(int, int, int, int) ([]library.Track, error) {
	return r.tracks, nil
}
func (r *uitestRecs) ReadOnRepeatMix(int, int, int) ([]library.Track, error) { return r.tracks, nil }
func (r *uitestRecs) ReadDeepCutsMix(int, int, float64, int) ([]library.Track, error) {
	return r.tracks, nil
}
func (r *uitestRecs) ReadNeverPlayedMix(int) ([]library.Track, error)          { return r.tracks, nil }
func (r *uitestRecs) ReadCompletionRates([]string) (map[string]float64, error) { return nil, nil }
func (r *uitestRecs) ReadSessionCoOccurrences(string, int) ([]db.SessionCoOccurrence, error) {
	return nil, nil
}
func (r *uitestRecs) ReadHourGenreAffinities() ([]db.HourGenreAffinity, error) { return nil, nil }
func (r *uitestRecs) ReadAllMixProgress() (map[db.MixID]db.MixProgress, error) {
	return r.progress, nil
}

// uitestCache is an in-memory cachedMixReader returning a fixed track slice.
type uitestCache struct{ tracks []library.Track }

func (c *uitestCache) ReadCachedMix(int, int) ([]library.Track, error) { return c.tracks, nil }

// uitestDismissals is an in-memory dismissalReader + dismissalWriter.
type uitestDismissals struct{ hashes map[string]struct{} }

func (d *uitestDismissals) ReadDismissedTrackHashes() (map[string]struct{}, error) {
	return d.hashes, nil
}
func (d *uitestDismissals) DismissTrack(string) error   { return nil }
func (d *uitestDismissals) UndismissTrack(string) error { return nil }
