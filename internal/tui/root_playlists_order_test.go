package tui

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

type orderTestStore struct {
	stubPlaylistDeleter
	err        error
	readErr    error
	playlistID int64
	tracks     []db.PlaylistTrackRow
}

func (s *orderTestStore) ReorderPlaylistTrack(pid, _ int64, _ int) error {
	s.playlistID = pid
	return s.err
}
func (s *orderTestStore) SetPlaylistOrder(pid int64, _ []int64) error {
	s.playlistID = pid
	return s.err
}
func (s *orderTestStore) ReadPlaylistTracks(_ int64) ([]db.PlaylistTrackRow, error) {
	return s.tracks, s.readErr
}

func TestPlaylistOrderSaveFailureRestoresRows(t *testing.T) {
	for _, shuffle := range []bool{false, true} {
		t.Run(map[bool]string{false: "move", true: "shuffle"}[shuffle], func(t *testing.T) {
			store := &orderTestStore{err: errors.New("disk full")}
			m := playlistModelAtTrackLevel(New(context.Background(), WithPlaylistDeleter(store)), samplePlaylistTracks(), 42)
			m.playlists.Cursor = 1
			// The opened playlist ID must win over any stale list cursor.
			m.playlists.Playlists = []db.PlaylistRow{{ID: 99}}
			var result playlistOrderSavedMsg
			if shuffle {
				next, cmd := m.handlePlaylistShuffleMsg()
				m = next
				result = cmd().(playlistOrderSavedMsg)
			} else {
				next, cmd := m.handlePlaylistMoveTrackUp()
				m = next
				result = cmd().(playlistOrderSavedMsg)
			}
			if !m.playlistOrderPending {
				t.Fatal("save should be pending")
			}
			if _, cmd := m.handlePlaylistMoveTrackDown(); cmd != nil {
				t.Fatal("overlapping order writes must wait")
			}
			// Changes arriving during the write must survive recovery.
			for i := range m.playlists.Tracks {
				if m.playlists.Tracks[i].ID == 20 {
					m.playlists.Tracks[i].Title = "Updated"
				}
			}
			m = m.handlePlaylistOrderSaved(result)
			if store.playlistID != 42 || m.playlistOrderPending {
				t.Fatalf("wrong target or pending state: %d", store.playlistID)
			}
			if got := playlistOrderIDs(m.playlists.Tracks); !reflect.DeepEqual(got, []int64{10, 20, 30}) {
				t.Fatalf("order = %v", got)
			}
			if m.playlists.Tracks[1].Title != "Updated" {
				t.Fatal("metadata was overwritten")
			}
			note := m.statusBar.TopNotification()
			if note.Severity != statusbar.SeverityError || !strings.Contains(note.Msg, "disk full") {
				t.Fatalf("missing error: %+v", note)
			}
			history := m.statusBar.RecentMessages()
			if len(history) == 0 || history[len(history)-1].Severity != statusbar.SeverityError {
				t.Fatal("save failure missing from recent messages")
			}
		})
	}
}

func TestPlaylistOrderResultDoesNotChangeAnotherPlaylist(t *testing.T) {
	m := playlistModelAtTrackLevel(New(context.Background()), samplePlaylistTracks(), 20)
	m = m.handlePlaylistOrderSaved(playlistOrderSavedMsg{playlistID: 10, order: []int64{30, 20, 10}, err: errors.New("disk full")})
	if !reflect.DeepEqual(playlistOrderIDs(m.playlists.Tracks), []int64{10, 20, 30}) {
		t.Fatal("changed another playlist")
	}
}

func TestPlaylistOrderRecoveryDoesNotResurrectDeletedRows(t *testing.T) {
	m := playlistModelAtTrackLevel(New(context.Background()), []db.PlaylistTrackRow{{ID: 30}, {ID: 10}, {ID: 40}}, 42)
	m = m.handlePlaylistOrderSaved(playlistOrderSavedMsg{playlistID: 42, order: []int64{10, 20, 30}, err: errors.New("disk full")})
	if !reflect.DeepEqual(playlistOrderIDs(m.playlists.Tracks), []int64{10, 30, 40}) {
		t.Fatalf("rows = %v", m.playlists.Tracks)
	}
}

func TestPlaylistShuffleReportsSavedSuccessOnly(t *testing.T) {
	for _, list := range []bool{false, true} {
		for _, failure := range []string{"", "write", "read"} {
			if !list && failure == "read" {
				continue
			}
			t.Run(map[bool]string{false: "tracks", true: "list"}[list]+failure, func(t *testing.T) {
				store := &orderTestStore{tracks: samplePlaylistTracks()}
				if failure == "write" {
					store.err = errors.New("write failed")
				}
				if failure == "read" {
					store.readErr = errors.New("read failed")
				}
				m := New(context.Background(), WithPlaylistDeleter(store), WithPlaylists([]db.PlaylistRow{{ID: 42, Name: "Mix", TrackCount: 3}}))
				m.playlistReader = store
				m.ActiveView = viewPlaylists
				if !list {
					m = playlistModelAtTrackLevel(m, samplePlaylistTracks(), 42)
				}
				m, cmd := m.handlePlaylistShuffleMsg()
				if strings.Contains(m.statusBar.TopNotification().Msg, "shuffled") {
					t.Fatal("premature success")
				}
				next, _ := m.Update(cmd())
				m = next.(Model)
				note := m.statusBar.TopNotification()
				if (note.Severity == statusbar.SeverityError) != (failure != "") {
					t.Fatalf("unexpected notification: %+v", note)
				}
				if m.playlistOrderPending {
					t.Fatal("pending flag stuck")
				}
			})
		}
	}
}
