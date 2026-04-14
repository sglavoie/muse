package tui

import (
	"context"
	"errors"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
)

// errStubPlay is a sentinel error returned by stubPlayer.PlayFromIndex when
// playFromIndexErr is set, used in tests that exercise play-failure paths.
var errStubPlay = errors.New("stub play error")

// stubFavoriteWriter records calls to AddFavorite, RemoveFavorite, and
// RemoveFavoritesByTypeAndValues for assertion in undo-favorite tests.
type stubFavoriteWriter struct {
	added   []struct{ typ, value string }
	removed []struct{ typ, value string }
}

func (s *stubFavoriteWriter) AddFavorite(typ, value string) error {
	s.added = append(s.added, struct{ typ, value string }{typ, value})
	return nil
}

func (s *stubFavoriteWriter) RemoveFavorite(typ, value string) error {
	s.removed = append(s.removed, struct{ typ, value string }{typ, value})
	return nil
}

func (s *stubFavoriteWriter) RemoveFavoritesByTypeAndValues(typ string, values []string) error {
	for _, v := range values {
		s.removed = append(s.removed, struct{ typ, value string }{typ, v})
	}
	return nil
}

// stubFavoriteReader returns canned favorites data for undo-favorite tests.
type stubFavoriteReader struct {
	favorites map[string][]string // typ -> values
}

func (s *stubFavoriteReader) ReadFavoritesByType(typ string) ([]db.FavoriteRow, error) {
	var result []db.FavoriteRow
	for i, v := range s.favorites[typ] {
		result = append(result, db.FavoriteRow{ID: int64(i + 1), Type: typ, Value: v})
	}
	return result, nil
}

func (s *stubFavoriteReader) ReadFavoriteCounts() (int, int, int, error) {
	return len(s.favorites["artist"]), len(s.favorites["album"]), len(s.favorites["track"]), nil
}

func (s *stubFavoriteReader) IsFavorite(typ, value string) (bool, error) {
	for _, v := range s.favorites[typ] {
		if v == value {
			return true, nil
		}
	}
	return false, nil
}

// stubExtractor is a no-op MetadataExtractor used in rescan tests to avoid
// a real exiftool dependency.
type stubExtractor struct{}

func (stubExtractor) Extract(_ context.Context, files []library.HashedFile) ([]library.Track, error) {
	return nil, nil
}

// stubPlayer records Play, Pause, and PlayFromIndex calls for assertion in tests.
type stubPlayer struct {
	playCalls          int
	pauseCalls         int
	playFromIndexCalls []int
	playFromIndexErr   error // when non-nil, PlayFromIndex returns this error
}

func (s *stubPlayer) Play() error {
	s.playCalls++
	return nil
}

func (s *stubPlayer) Pause() error {
	s.pauseCalls++
	return nil
}

func (s *stubPlayer) PlayFromIndex(idx int) error {
	s.playFromIndexCalls = append(s.playFromIndexCalls, idx)
	return s.playFromIndexErr
}

// stubCommandPlayer extends stubPlayer with the commandPlayer interface so that
// tests can exercise the s/n/p/seek/volume key handlers.
type stubCommandPlayer struct {
	stubPlayer
	stopCalls int
}

func (s *stubCommandPlayer) Stop() error {
	s.stopCalls++
	return nil
}

func (s *stubCommandPlayer) SeekRelative(float64) error      { return nil }
func (s *stubCommandPlayer) SetVolumeRelative(float64) error { return nil }
func (s *stubCommandPlayer) Next() error                     { return nil }
func (s *stubCommandPlayer) Previous() error                 { return nil }

// stubElapsedReader extends stubPlayer with the elapsedReader interface for
// testing the tickMsg handler.
type stubElapsedReader struct {
	stubPlayer
	elapsed float64
	total   float64
	err     error
}

func (s *stubElapsedReader) ElapsedPosition() (float64, float64, error) {
	return s.elapsed, s.total, s.err
}

// stubVolumePlayer extends stubCommandPlayer to capture SetVolumeRelative calls.
type stubVolumePlayer struct {
	stubCommandPlayer
	volumeDelta float64
}

func (s *stubVolumePlayer) SetVolumeRelative(delta float64) error {
	s.volumeDelta = delta
	return nil
}

// stubSessionWriter records WriteRepeatMode, WriteShuffle,
// UpdateSessionPosition, and WriteVolume calls for assertion in tests.
type stubSessionWriter struct {
	calls         []string
	shuffleCalls  []bool
	positionCalls []struct {
		hash       string
		positionMs int64
	}
	volumeCalls []int
}

func (s *stubSessionWriter) WriteRepeatMode(mode string) error {
	s.calls = append(s.calls, mode)
	return nil
}

func (s *stubSessionWriter) WriteShuffle(enabled bool) error {
	s.shuffleCalls = append(s.shuffleCalls, enabled)
	return nil
}

func (s *stubSessionWriter) UpdateSessionPosition(hash string, positionMs int64) error {
	s.positionCalls = append(s.positionCalls, struct {
		hash       string
		positionMs int64
	}{hash, positionMs})
	return nil
}

func (s *stubSessionWriter) WriteVolume(volume int) error {
	s.volumeCalls = append(s.volumeCalls, volume)
	return nil
}

// stubPlaylistCreator is a minimal playlistCreator used in create-flow tests.
type stubPlaylistCreator struct {
	createErr error
	readErr   error
	rows      []db.PlaylistRow
	nextID    int64
}

func (s *stubPlaylistCreator) CreatePlaylist(name string) (int64, error) {
	if s.createErr != nil {
		return 0, s.createErr
	}
	id := s.nextID
	s.rows = append(s.rows, db.PlaylistRow{ID: id, Name: name})
	return id, nil
}

func (s *stubPlaylistCreator) ReadPlaylists() ([]db.PlaylistRow, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}
	return s.rows, nil
}

// stubPlaylistRenamer is a minimal playlistRenamer used in rename-flow tests.
type stubPlaylistRenamer struct {
	rows []db.PlaylistRow
}

func (s *stubPlaylistRenamer) RenamePlaylist(_ int64, _ string) error {
	return nil
}

func (s *stubPlaylistRenamer) ReadPlaylists() ([]db.PlaylistRow, error) {
	return s.rows, nil
}

// errPlaylistCreate is a simple error type used in create-flow error tests.
type errPlaylistCreate struct{ msg string }

func (e *errPlaylistCreate) Error() string { return e.msg }

// stubSeekPlayer extends stubPlayer with tracking for SeekRelative, Next, and
// Previous calls so that seek and navigation key tests can assert correct deltas
// and call counts without a real mpv backend.
type stubSeekPlayer struct {
	stubPlayer
	seekDeltas []float64
	nextCalls  int
	prevCalls  int
}

func (s *stubSeekPlayer) Stop() error { return nil }
func (s *stubSeekPlayer) SeekRelative(d float64) error {
	s.seekDeltas = append(s.seekDeltas, d)
	return nil
}
func (s *stubSeekPlayer) SetVolumeRelative(float64) error { return nil }
func (s *stubSeekPlayer) Next() error                     { s.nextCalls++; return nil }
func (s *stubSeekPlayer) Previous() error                 { s.prevCalls++; return nil }

// stubQueueAppender is a minimal libtui.QueueAppender for root-level tests.
type stubQueueAppender struct {
	tracks []library.Track
}

func (s *stubQueueAppender) AppendTrack(t library.Track, _ string) (int64, error) {
	s.tracks = append(s.tracks, t)
	return int64(len(s.tracks)), nil
}

// stubPlaylistAdder records calls to AddTracksToPlaylist for test assertions.
type stubPlaylistAdder struct {
	calls []struct {
		playlistID int64
		hashes     []string
	}
}

func (s *stubPlaylistAdder) AddTracksToPlaylist(playlistID int64, hashes []string) error {
	s.calls = append(s.calls, struct {
		playlistID int64
		hashes     []string
	}{playlistID, hashes})
	return nil
}

// stubQueueDequeuer records calls to soft-delete, undo, and purge methods for
// use in undo-slot unit tests.
type stubQueueDequeuer struct {
	dequeueTrackSoftCalls []int64
	undoDequeueCallIDs    []int64
	clearQueueSoftResult  []int64
	undoClearQueueCalls   [][]int64
	purgeBeforeCalls      []int64
}

func (s *stubQueueDequeuer) DequeueTrackSoft(id int64) error {
	s.dequeueTrackSoftCalls = append(s.dequeueTrackSoftCalls, id)
	return nil
}

func (s *stubQueueDequeuer) UndoDequeue(id int64) error {
	s.undoDequeueCallIDs = append(s.undoDequeueCallIDs, id)
	return nil
}

func (s *stubQueueDequeuer) ClearQueueSoft() ([]int64, error) {
	return s.clearQueueSoftResult, nil
}

func (s *stubQueueDequeuer) UndoClearQueue(ids []int64) error {
	s.undoClearQueueCalls = append(s.undoClearQueueCalls, ids)
	return nil
}

func (s *stubQueueDequeuer) PurgeDeletedQueueEntries(before int64) error {
	s.purgeBeforeCalls = append(s.purgeBeforeCalls, before)
	return nil
}

// stubQueueReorderer records calls to ReorderQueueTrack for use in
// move-track unit tests.
type stubQueueReorderer struct {
	calls []struct {
		id        int64
		direction int
	}
}

func (s *stubQueueReorderer) ReorderQueueTrack(id int64, direction int) error {
	s.calls = append(s.calls, struct {
		id        int64
		direction int
	}{id, direction})
	return nil
}

// stubPlaylistTrackReader implements playlistTrackReader for tests.
type stubPlaylistTrackReader struct {
	rows []db.PlaylistTrackRow
}

func (s *stubPlaylistTrackReader) ReadPlaylistTracks(_ int64) ([]db.PlaylistTrackRow, error) {
	return s.rows, nil
}

// stubPlayerQueueRemover extends stubCommandPlayer with playerQueueRemover so
// that DequeueTrackMsg handler tests can assert RemoveFromQueue is called with
// the correct index and that Stop is never invoked.
type stubPlayerQueueRemover struct {
	stubCommandPlayer
	removeFromQueueCalls []int
}

func (s *stubPlayerQueueRemover) RemoveFromQueue(index int) error {
	s.removeFromQueueCalls = append(s.removeFromQueueCalls, index)
	return nil
}

// stubPlayerQueueSyncer extends stubPlayer with playerQueueSetter and
// playerQueueRemover so undo tests can verify the player queue is rebuilt
// after undoing a dequeue.
type stubPlayerQueueSyncer struct {
	stubPlayer
	setQueueCalls        [][]player.QueueTrack
	removeFromQueueCalls []int
}

func (s *stubPlayerQueueSyncer) SetQueue(tracks []player.QueueTrack) {
	cp := make([]player.QueueTrack, len(tracks))
	copy(cp, tracks)
	s.setQueueCalls = append(s.setQueueCalls, cp)
}

func (s *stubPlayerQueueSyncer) RemoveFromQueue(index int) error {
	s.removeFromQueueCalls = append(s.removeFromQueueCalls, index)
	return nil
}

// stubPlaylistDeleter records calls to playlist/track soft-delete, undo, and
// purge methods for use in playlist deletion and undo tests.
type stubPlaylistDeleter struct {
	deletePlaylistSoftCalls         []int64
	undoDeletePlaylistCalls         []int64
	purgeDeletedPlaylistsCalls      []int64
	removePlaylistTrackSoftCalls    []int64
	undoRemovePlaylistTrackCalls    []int64
	purgeDeletedPlaylistTracksCalls []int64
}

func (s *stubPlaylistDeleter) DeletePlaylistSoft(id int64) error {
	s.deletePlaylistSoftCalls = append(s.deletePlaylistSoftCalls, id)
	return nil
}
func (s *stubPlaylistDeleter) UndoDeletePlaylist(id int64) error {
	s.undoDeletePlaylistCalls = append(s.undoDeletePlaylistCalls, id)
	return nil
}
func (s *stubPlaylistDeleter) PurgeDeletedPlaylists(before int64) error {
	s.purgeDeletedPlaylistsCalls = append(s.purgeDeletedPlaylistsCalls, before)
	return nil
}
func (s *stubPlaylistDeleter) RemovePlaylistTrackSoft(id int64) error {
	s.removePlaylistTrackSoftCalls = append(s.removePlaylistTrackSoftCalls, id)
	return nil
}
func (s *stubPlaylistDeleter) UndoRemovePlaylistTrack(id int64) error {
	s.undoRemovePlaylistTrackCalls = append(s.undoRemovePlaylistTrackCalls, id)
	return nil
}
func (s *stubPlaylistDeleter) PurgeDeletedPlaylistTracks(before int64) error {
	s.purgeDeletedPlaylistTracksCalls = append(s.purgeDeletedPlaylistTracksCalls, before)
	return nil
}

// stubBatchEnqueuer records calls to AppendTracks and DequeueTracksSoft for
// use in batch-enqueue undo tests.
type stubBatchEnqueuer struct {
	appendTracksCalls    [][]library.Track
	dequeueTracksSoftIDs [][]int64
}

func (s *stubBatchEnqueuer) AppendTracks(tracks []library.Track, _ string) ([]int64, error) {
	s.appendTracksCalls = append(s.appendTracksCalls, tracks)
	ids := make([]int64, len(tracks))
	for i := range tracks {
		ids[i] = int64(100 + i)
	}
	return ids, nil
}

func (s *stubBatchEnqueuer) DequeueTracksSoft(ids []int64) error {
	cp := make([]int64, len(ids))
	copy(cp, ids)
	s.dequeueTracksSoftIDs = append(s.dequeueTracksSoftIDs, cp)
	return nil
}

// stubFailingVolumePlayer extends stubCommandPlayer to return errors from
// SetVolumeRelative, used to test volume-drift prevention.
type stubFailingVolumePlayer struct {
	stubCommandPlayer
	volumeErr error
}

func (s *stubFailingVolumePlayer) SetVolumeRelative(float64) error {
	return s.volumeErr
}

// stubFailingPlayer returns errors from Play() to test error paths.
type stubFailingPlayer struct {
	stubPlayer
	playErr error
}

func (s *stubFailingPlayer) Play() error {
	s.playCalls++
	return s.playErr
}

// stubHistoryRecorder records RecordPlay calls for assertion in tests.
type stubHistoryRecorder struct {
	calls []struct {
		hash       string
		playedAt   int64
		listenedSs int
	}
}

func (s *stubHistoryRecorder) RecordPlay(hash string, playedAt int64, listenedSecs int) error {
	s.calls = append(s.calls, struct {
		hash       string
		playedAt   int64
		listenedSs int
	}{hash, playedAt, listenedSecs})
	return nil
}

type stubDurationWriter struct {
	calls []struct {
		hash       string
		durationMs int64
	}
}

func (s *stubDurationWriter) UpdateTrackDuration(hash string, durationMs int64) error {
	s.calls = append(s.calls, struct {
		hash       string
		durationMs int64
	}{hash, durationMs})
	return nil
}

// stubTagReader returns canned tag data for tag-picker tests.
type stubTagReader struct {
	tags       []db.TagRow
	trackTags  map[string][]db.TagRow // hash → tags
	tagCounts  map[int64]int
	tracksByID map[int64][]db.PlaylistTrackRow
	hashesByID map[int64][]string
}

func (s *stubTagReader) ReadTags() ([]db.TagRow, error) {
	return s.tags, nil
}
func (s *stubTagReader) ReadTagsForTrack(hash string) ([]db.TagRow, error) {
	return s.trackTags[hash], nil
}
func (s *stubTagReader) ReadTrackHashesByTag(tagID int64) ([]string, error) {
	return s.hashesByID[tagID], nil
}
func (s *stubTagReader) ReadTracksByTag(tagID int64) ([]db.PlaylistTrackRow, error) {
	return s.tracksByID[tagID], nil
}
func (s *stubTagReader) ReadTagCounts() (map[int64]int, error) {
	return s.tagCounts, nil
}

// stubTagWriter records tag mutation calls for assertion in tests.
type stubTagWriter struct {
	nextID         int64
	createCalls    []string
	addTrackCalls  []struct{ hash string; tagID int64 }
	addBulkCalls   []struct{ hashes []string; tagID int64 }
}

func (s *stubTagWriter) CreateTag(name, _ string) (int64, error) {
	s.createCalls = append(s.createCalls, name)
	s.nextID++
	return s.nextID, nil
}
func (s *stubTagWriter) RenameTag(_ int64, _ string) error          { return nil }
func (s *stubTagWriter) UpdateTagColor(_ int64, _ string) error     { return nil }
func (s *stubTagWriter) DeleteTagSoft(_ int64) error                { return nil }
func (s *stubTagWriter) DeleteTagIfEmpty(_ int64) (bool, error)     { return false, nil }
func (s *stubTagWriter) PurgeDeletedTags(_ int64) error             { return nil }
func (s *stubTagWriter) UndoDeleteTag(_ int64) error                { return nil }
func (s *stubTagWriter) AddTrackTag(hash string, tagID int64) error {
	s.addTrackCalls = append(s.addTrackCalls, struct{ hash string; tagID int64 }{hash, tagID})
	return nil
}
func (s *stubTagWriter) RemoveTrackTag(_ string, _ int64) error { return nil }
func (s *stubTagWriter) AddTrackTagBulk(hashes []string, tagID int64) error {
	s.addBulkCalls = append(s.addBulkCalls, struct{ hashes []string; tagID int64 }{hashes, tagID})
	return nil
}
func (s *stubTagWriter) RemoveTrackTagBulk(_ []string, _ int64) error { return nil }
func (s *stubTagWriter) MergeTags(_, _ int64) error                   { return nil }
