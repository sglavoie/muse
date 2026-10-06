package tui

import (
	"cmp"
	"slices"

	"github.com/sglavoie/muse/internal/db"
)

// order contains the saved row IDs: the original order on failure, or the new
// order on success. Only ordering is restored, retaining metadata and deletions
// received while the write was in flight.
type playlistOrderSavedMsg struct {
	playlistID int64
	order      []int64
	success    string
	err        error
}

func playlistOrderIDs(tracks []db.PlaylistTrackRow) []int64 {
	ids := make([]int64, len(tracks))
	for i, track := range tracks {
		ids[i] = track.ID
	}
	return ids
}

func (m Model) handlePlaylistOrderSaved(msg playlistOrderSavedMsg) Model {
	m.playlistOrderPending = false
	if msg.err != nil {
		m.statusBar.NotifyError("Save playlist order failed: " + msg.err.Error())
	} else if msg.success != "" {
		m.statusBar.Notify(msg.success)
	}
	if m.playlists.AtListLevel() || m.playlists.PendingLoadID != msg.playlistID || msg.order == nil {
		return m
	}
	return m.applyPlaylistRowOrder(msg.order)
}

func (m Model) applyPlaylistRowOrder(order []int64) Model {
	var focusedID int64
	visible := m.playlists.DisplayTracks()
	if i := m.playlists.Cursor; i >= 0 && i < len(visible) {
		focusedID = visible[i].ID
	}
	ranks := make(map[int64]int, len(order))
	for i, id := range order {
		ranks[id] = i + 1
	}
	tracks := slices.Clone(m.playlists.Tracks)
	slices.SortStableFunc(tracks, func(a, b db.PlaylistTrackRow) int {
		return cmp.Compare(orderRank(ranks, a.ID), orderRank(ranks, b.ID))
	})
	m.playlists = m.playlists.RefreshTracks(tracks)
	m = m.preservePlaylistsSort()
	for i, track := range m.playlists.DisplayTracks() {
		if track.ID == focusedID {
			m.playlists.Cursor = i
			break
		}
	}
	m.playlists = m.playlists.EnsureCursorVisible()
	return m
}

func orderRank(ranks map[int64]int, id int64) int {
	if rank, ok := ranks[id]; ok {
		return rank
	}
	return len(ranks) + 1
}
