package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
)

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
