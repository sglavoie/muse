package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func TestToggleFavorite_TagsTrackLevelSingleOpensPicker(t *testing.T) {
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

	nm := openFavoritePickerForTest(t, m)
	if got := nm.overlay.favoriteTargetPicker.tracks[0].Hash; got != "ht2" {
		t.Errorf("picker track hash = %q, want ht2", got)
	}
}

func TestToggleFavorite_TagsTrackLevelBulkOpensPicker(t *testing.T) {
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

	nm := openFavoritePickerForTest(t, m)
	if len(nm.tagsView.selectedHashes) != 2 {
		t.Fatal("opening picker should preserve selectedHashes")
	}
	if got := len(nm.overlay.favoriteTargetPicker.tracks); got != 2 {
		t.Errorf("picker tracks = %d, want 2", got)
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
