package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestToggleFavorite_TagsTrackLevelSingle(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "ht1", Title: "Tag Song A"},
		{Hash: "ht2", Title: "Tag Song B"},
	}
	m.tagsView.cursor = 1

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewTags at TrackLevel")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.typ != "track" {
		t.Errorf("expected typ='track', got %q", msg.typ)
	}
	if msg.value != "ht2" {
		t.Errorf("expected value='ht2', got %q", msg.value)
	}
}

func TestToggleFavorite_TagsTrackLevelBulk(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{"track": {}}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TrackLevel
	m.tagsView.tracks = []db.PlaylistTrackRow{
		{Hash: "ht1", Title: "Tag Song A"},
		{Hash: "ht2", Title: "Tag Song B"},
	}
	m.tagsView.selectedHashes = map[string]struct{}{"ht1": {}, "ht2": {}}

	nm, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should handle viewTags at TrackLevel with bulk selection")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if nm.tagsView.selectedHashes != nil {
		t.Fatal("selectedHashes should be nil after bulk toggle")
	}
	msg := cmd().(favoriteToggledMsg)
	if msg.count != 2 {
		t.Errorf("expected count=2, got %d", msg.count)
	}
}

func TestToggleFavorite_TagsTagLevelNoop(t *testing.T) {
	w := &stubFavoriteWriter{}
	r := &stubFavoriteReader{favorites: map[string][]string{}}
	m := New(context.Background(), WithFavoriteReader(r), WithFavoriteWriter(w))
	m.activeView = viewTags
	m.tagsView.level = tagtui.TagLevel

	_, cmd, handled := m.handleToggleFavorite()
	if !handled {
		t.Fatal("handleToggleFavorite should return handled=true")
	}
	if cmd != nil {
		t.Fatal("expected nil cmd at TagLevel")
	}
}
