package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	"github.com/sglavoie/muse/internal/tui/palette"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/statusbar"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestViewSwitchPreservesSortOrderAndFocus(t *testing.T) {
	for _, view := range []viewID{viewLibrary, viewQueue, viewPlaylists, viewBrowse, viewFavorites, viewTags} {
		t.Run(ViewName(view), func(t *testing.T) {
			m := applyResize(t, newRichFixture(), 100, 30).switchToView(view)
			m.followNowPlaying = false
			if view == viewTags {
				m.tagsView.Level = tagtui.TrackLevel
				m.tagsView.Tracks = richPlaylistTracks()[1]
			}
			if view == viewFavorites {
				m.favorites.Level = favtui.ListLevel
				m.favorites.Category = favtui.CategoryTracks
				m.favorites.Tracks = richLibraryTracks()
			}
			fields, _ := m.activeSortFields()
			next, _ := m.selectSortField(fields[1])
			m = pressKey(next.(Model), "j")
			_, state := m.activeSortFields()
			rows, focus, cursor := m.filterRowKeys(), m.focusedTrackHash(), m.Snapshot().Cursor
			for _, other := range viewOrder {
				m = m.switchToView(other)
			}
			m = m.switchToView(view)
			_, gotState := m.activeSortFields()
			if gotState != state || !reflect.DeepEqual(rows, m.filterRowKeys()) || m.focusedTrackHash() != focus || m.Snapshot().Cursor != cursor {
				t.Fatalf("switching changed sort, rows, or focus: state=%+v want=%+v", gotState, state)
			}
			next, _ = m.selectSortField(sorting.SortDefault)
			_, reset := next.(Model).activeSortFields()
			if reset.Field != sorting.SortDefault {
				t.Fatal("explicit default-order command failed")
			}
		})
	}
}

func TestJumpToCollectionPreservesOtherViewSorts(t *testing.T) {
	m := newRichFixture()
	m.library.SortState = sorting.SortState{Field: sorting.SortDuration, Dir: sorting.SortDesc}
	m.library = m.library.RefreshView()
	want := m.library.SortState
	m = m.jumpToArtist("Alice").jumpToAlbum("Alpha").jumpToGenre("Rock")
	m = m.jumpToPlaylist(m.playlists.Playlists[0].Name)
	if m.library.SortState != want {
		t.Fatal("collection navigation reset the library sort")
	}
}

func TestQueueSummaryRendersLiveState(t *testing.T) {
	m := applyResize(t, newModelWithTracks(), 100, 30)
	m.queue.Entries = []queue.Entry{{ID: 1, DurationMs: 60000}, {ID: 2, DurationMs: 120000}, {ID: 3}}
	m.playback.state = playback.StatePlaying
	m.playback.currentIndex = 0
	m.playback.totalSecs = 60
	m.overlay.queueSummary = m.buildQueueSummary()
	for _, tc := range []struct {
		index          int
		elapsed, total float64
		remaining      string
	}{{0, 20, 60, "2:40+"}, {0, 35, 60, "2:25+"}, {1, 10, 120, "1:50+"}} {
		m.playback.currentIndex, m.playback.elapsedSecs, m.playback.totalSecs = tc.index, tc.elapsed, tc.total
		body, _ := m.renderOverlayBody()
		out := ansi.Strip(body)
		if !strings.Contains(out, tc.remaining) || !strings.Contains(out, "3:00+") {
			t.Fatalf("summary did not reflect live position or unknown total:\n%s", out)
		}
	}
	m.queue.Entries[2].DurationMs = 30000
	body, _ := m.renderOverlayBody()
	out := ansi.Strip(body)
	if !strings.Contains(out, "3:30") || !strings.Contains(out, "2:20") || strings.Contains(out, "+") {
		t.Fatalf("summary did not reflect corrected duration:\n%s", out)
	}
}

func TestCopyNowPlayingUsesPlaybackNotFocus(t *testing.T) {
	m := newModelWithTracks()
	m.playback.currentHash, m.playback.state = "h2", playback.StatePlaying
	m.ActiveView, m.browse.Mode = viewBrowse, browse.BrowseModeArtists
	command := polishCommand(t, m, "playback.copy.info")
	if !command.IsEnabled() || command.Detail != "Alice – Track A2" {
		t.Fatalf("wrong target at collection level: %+v", command)
	}
	m.playback.state = playback.StatePaused
	if !polishCommand(t, m, command.ID).IsEnabled() {
		t.Fatal("paused track cannot be copied")
	}
	for _, state := range []playback.State{playback.StateIdle, playback.StateStopped} {
		m.playback.state = state
		if polishCommand(t, m, command.ID).IsEnabled() {
			t.Fatal("copy enabled without active playback")
		}
	}
	m.playback.state = playback.StatePlaying
	m.fixtureSafeCommands = true
	_, cmd := m.Update(palette.CommandPalettePickedMsg{Command: polishCommand(t, m, command.ID)})
	if cmd == nil {
		t.Fatal("missing fixture guard notification")
	}
	msg, ok := cmd().(statusbar.TransientNotifyMsg)
	if !ok || !strings.Contains(string(msg), "blocked in fixture mode") {
		t.Fatal("fixture attempted a real clipboard command")
	}
}

func TestNowPlayingCopyMissingMetadata(t *testing.T) {
	for _, tc := range []struct{ artist, title, want string }{
		{"Alice", "", "Alice"}, {"", "Song", "Song"}, {"", "", "/a1.flac"},
	} {
		m := newModelWithTracks()
		m.playback.currentHash = "h1"
		m.library.Index.UpdateMetadata("h1", tc.title, tc.artist, "", "")
		if got := m.nowPlayingCopyText(); got != tc.want {
			t.Fatalf("copy = %q, want %q", got, tc.want)
		}
	}
}

func TestBulkClearMixedMetadataSaveAndUndo(t *testing.T) {
	m := precisionMetadataModel()
	m.library.Index.Insert(library.Track{Hash: "h2", Path: "/b.flac", Title: "Other", Artist: "Artist", Duration: 200.9})
	writer := newImportWriterSpy()
	m.metadataWriter = writer
	ed := m.bulkEditorForHashes([]string{"h1", "h2"})
	m.overlay.metadataEditor = &ed
	next, _ := m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	m = next.(Model)
	next, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = next.(Model)
	if cmd == nil {
		t.Fatal("clear did not save")
	}
	if msg := cmd().(metadataEditedMsg); msg.err != nil {
		t.Fatal(msg.err)
	}
	meta, _ := writer.snapshot()
	for _, hash := range []string{"h1", "h2"} {
		track, _ := m.library.Index.FindByHash(hash)
		row, ok := meta[hash]
		if track.Title != "" || track.Artist != "Artist" || !ok || row[0] != "" {
			t.Fatalf("clear was not applied and persisted: %+v, %v", track, meta)
		}
	}
	m.metadataWriter = nil
	m = m.handleUndo()
	for hash, title := range map[string]string{"h1": "Song", "h2": "Other"} {
		track, _ := m.library.Index.FindByHash(hash)
		if track.Title != title {
			t.Fatalf("undo failed for %s: %+v", hash, track)
		}
	}
	track, _ := m.library.Index.FindByHash("h1")
	if track.Duration != 183.7 {
		t.Fatal("clear or undo lost duration precision")
	}
}
