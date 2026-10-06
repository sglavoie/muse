package tui

import (
	"context"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/keys"
)

type refreshPlaylistEvaluator struct{ calls int }

func (e *refreshPlaylistEvaluator) EvalSmartPlaylist(db.SmartPlaylistQuery) ([]db.PlaylistTrackRow, error) {
	e.calls++
	return nil, nil
}

func TestPlaylistCommandsDescribeAndGateTargets(t *testing.T) {
	m := New(context.Background(), WithPlaylists([]db.PlaylistRow{
		{ID: 1, Name: "Static"}, {ID: 2, Name: "Smart", IsSmart: true},
	}))
	m.ActiveView = viewPlaylists
	if cmd := polishCommand(t, m, "playlist.rename"); !cmd.IsEnabled() || !strings.Contains(cmd.Detail, "Static") {
		t.Fatalf("rename should identify focused playlist: %+v", cmd)
	}
	if polishCommand(t, m, "playlist.smart.edit").IsEnabled() {
		t.Fatal("static playlist offered smart rules")
	}
	m.playlists.Cursor = 1
	if cmd := polishCommand(t, m, "playlist.smart.edit"); !cmd.IsEnabled() || !strings.Contains(cmd.Detail, "Smart") {
		t.Fatalf("smart rule target missing: %+v", cmd)
	}
	m.playlists = m.playlists.OpenTracks([]db.PlaylistTrackRow{{ID: 3, DurationMs: 60000}})
	m.playlists.SmartQuery = &db.SmartPlaylistQuery{}
	eval := &refreshPlaylistEvaluator{}
	m.smartEval = eval
	for _, id := range []string{"playlist.rename", "playlist.delete", "playlist.smart.edit"} {
		if polishCommand(t, m, id).IsEnabled() {
			t.Errorf("%s enabled inside playlist", id)
		}
	}
	cmd := polishCommand(t, m, "playlist.smart.refresh")
	if !cmd.IsEnabled() || !strings.Contains(cmd.Detail, "Smart") {
		t.Fatalf("refresh target missing: %+v", cmd)
	}
	_, action := m.handleCommandPaletteAction(keys.ActionPlaylistRename, 0)
	if action != nil || eval.calls != 0 {
		t.Fatal("rename dispatched refresh")
	}
	_, action = m.handleRefreshSmartPlaylist()
	if action == nil {
		t.Fatal("refresh missing")
	}
	_ = action()
	if eval.calls != 1 {
		t.Fatal("refresh did not evaluate rules")
	}
	m.playlists.Filter.Text = "hidden"
	if cmd := polishCommand(t, m, "playlist.enqueue_all"); !strings.Contains(cmd.Detail, "entire playlist: Smart") {
		t.Fatalf("enqueue should explain whole playlist target despite filter: %+v", cmd)
	}
}

func TestPlaylistCommandsEmptyAndWrongView(t *testing.T) {
	m := New(context.Background())
	for _, view := range []viewID{viewLibrary, viewPlaylists} {
		m.ActiveView = view
		for _, id := range []string{"playlist.rename", "playlist.delete", "playlist.smart.edit", "playlist.enqueue_all", "playlist.smart.refresh"} {
			if polishCommand(t, m, id).IsEnabled() {
				t.Errorf("%s enabled without a playlist target in %v", id, view)
			}
		}
	}
}
