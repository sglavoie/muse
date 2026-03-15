package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

// stubExtractor is a no-op MetadataExtractor used in rescan tests to avoid
// a real exiftool dependency.
type stubExtractor struct{}

func (stubExtractor) Extract(files []library.HashedFile) ([]library.Track, error) {
	return nil, nil
}

// stubPlayer records Play, Pause, and PlayFromIndex calls for assertion in tests.
type stubPlayer struct {
	playCalls          int
	pauseCalls         int
	playFromIndexCalls []int
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
	return nil
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

// stubSessionWriter records WriteRepeatMode, WriteShuffle, and
// UpdateSessionPosition calls for assertion in tests.
type stubSessionWriter struct {
	calls         []string
	shuffleCalls  []bool
	positionCalls []struct {
		hash       string
		positionMs int64
	}
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

func (s *stubQueueAppender) AppendTrack(t library.Track) (int64, error) {
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
