package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

func newCollectionTagModel(tracks []library.Track) (Model, *stubTagWriter) {
	tw := &stubTagWriter{}
	tr := &stubTagReader{tags: []db.TagRow{{ID: 7, Name: "chill"}}}
	m := New(context.Background(), WithLibraryTracks(tracks), WithTagReader(tr), WithTagWriter(tw))
	m.tags = []db.TagRow{{ID: 7, Name: "chill"}}
	return m, tw
}

func tagKeyEnter() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }

func TestTagPickerOnArtistRowTargetsAllArtistTracks(t *testing.T) {
	m, _ := newCollectionTagModel(sampleLibraryTracks())
	m = pressKey(m, "4")
	if m.browse.Artists.Level != arttui.ArtistLevel {
		t.Fatalf("expected ArtistLevel, got %v", m.browse.Artists.Level)
	}

	m = pressKey(m, "t")
	if m.overlay.tagPicker == nil {
		t.Fatal("t on an artist row should open the tag picker")
	}
	if got, want := m.overlay.tagPicker.Hashes(), []string{"h1", "h2", "h3"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picker hashes = %v, want %v", got, want)
	}
	if !m.overlay.tagPicker.IsBulk() {
		t.Error("an artist with several tracks should open the bulk picker")
	}
	if got := m.overlay.tagPicker.Scope(); got != "Artist: Alice · 3 tracks" {
		t.Errorf("scope = %q", got)
	}
	if out := m.overlay.tagPicker.Render(80, 30, *m.styles); !strings.Contains(out, "Artist: Alice · 3 tracks") {
		t.Errorf("rendered picker should name the artist scope:\n%s", out)
	}
}

func TestTagPickerOnArtistRowAssignsTagToEveryTrack(t *testing.T) {
	m, tw := newCollectionTagModel(sampleLibraryTracks())
	m = pressKey(m, "4")
	m = pressKey(m, "t")
	if m.overlay.tagPicker == nil {
		t.Fatal("tag picker did not open")
	}

	picker := *m.overlay.tagPicker
	_, cmd := picker.HandleKey("enter", tagKeyEnter())
	toggle := cmd().(tagtui.TagPickerToggleMsg)
	if !toggle.Add {
		t.Fatal("first toggle on untagged tracks should add")
	}
	m, write := m.handleTagPickerToggle(toggle)
	if msg := write().(tagToggledMsg); msg.err != nil {
		t.Fatalf("toggle failed: %v", msg.err)
	}

	if len(tw.addBulkCalls) != 1 || !reflect.DeepEqual(tw.addBulkCalls[0].hashes, []string{"h1", "h2", "h3"}) {
		t.Fatalf("addBulkCalls = %+v, want one call for h1,h2,h3", tw.addBulkCalls)
	}
	for _, h := range []string{"h1", "h2", "h3"} {
		if _, ok := m.trackTags[h][7]; !ok {
			t.Errorf("track %s should carry the tag in the cache", h)
		}
	}
	if _, ok := m.trackTags["h4"][7]; ok {
		t.Error("tracks by other artists must not be tagged")
	}
}

func TestTagPickerOnArtistAlbumRowTargetsAlbumTracks(t *testing.T) {
	m, _ := newCollectionTagModel(sampleLibraryTracks())
	m = pressKey(m, "4")
	m = pressEnter(m) // Alice → her albums
	if m.browse.Artists.Level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.Artists.Level)
	}

	m = pressKey(m, "t")
	if m.overlay.tagPicker == nil {
		t.Fatal("t on an album row should open the tag picker")
	}
	if got, want := m.overlay.tagPicker.Hashes(), []string{"h1", "h2"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("picker hashes = %v, want %v", got, want)
	}
	if got := m.overlay.tagPicker.Scope(); got != "Album: Alpha · 2 tracks" {
		t.Errorf("scope = %q", got)
	}
}

func TestTagPickerOnAlbumsModeRowTargetsAlbumTracks(t *testing.T) {
	m, _ := newCollectionTagModel(sampleLibraryTracks())
	m = switchToAlbumsMode(m)
	if m.browse.Albums.Level != albtui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.Albums.Level)
	}
	album, ok := m.browse.Albums.FocusedAlbum()
	if !ok {
		t.Fatal("expected a focused album")
	}
	want := uniqueTrackHashes(albtui.DeriveAlbumTracks(m.library.Index.Tracks(), album))

	m = pressKey(m, "t")
	if m.overlay.tagPicker == nil {
		t.Fatal("t on an album row should open the tag picker")
	}
	if got := m.overlay.tagPicker.Hashes(); !reflect.DeepEqual(got, want) || len(got) == 0 {
		t.Fatalf("picker hashes = %v, want %v", got, want)
	}
	if !strings.HasPrefix(m.overlay.tagPicker.Scope(), "Album: ") {
		t.Errorf("scope = %q, want an album label", m.overlay.tagPicker.Scope())
	}
}

func TestTagPickerOnArtistRowShowsPartialStateAndRemovesFromAll(t *testing.T) {
	m, _ := newCollectionTagModel(sampleLibraryTracks())
	m = m.applyTagToggle(7, []string{"h1"}, true) // only one Alice track tagged
	m = pressKey(m, "4")
	m = pressKey(m, "t")
	if m.overlay.tagPicker == nil {
		t.Fatal("tag picker did not open")
	}
	// Partial → add to all; a second toggle then removes from every track.
	picker := *m.overlay.tagPicker
	_, cmd := picker.HandleKey("enter", tagKeyEnter())
	toggle := cmd().(tagtui.TagPickerToggleMsg)
	if !toggle.Add {
		t.Fatal("toggling a partially applied tag should add it to every track")
	}
	m, _ = m.handleTagPickerToggle(toggle)

	picker = *m.overlay.tagPicker
	_, cmd = picker.HandleKey("enter", tagKeyEnter())
	toggle = cmd().(tagtui.TagPickerToggleMsg)
	if toggle.Add {
		t.Fatal("toggling a fully applied tag should remove it")
	}
	if !reflect.DeepEqual(toggle.Hashes, []string{"h1", "h2", "h3"}) {
		t.Fatalf("remove hashes = %v", toggle.Hashes)
	}
	m, _ = m.handleTagPickerToggle(toggle)
	for _, h := range []string{"h1", "h2", "h3"} {
		if _, ok := m.trackTags[h][7]; ok {
			t.Errorf("track %s should no longer carry the tag", h)
		}
	}
}

func TestTagPickerOnSingleTrackArtistUsesSingleMode(t *testing.T) {
	m, _ := newCollectionTagModel([]library.Track{
		{Artist: "Solo", Album: "One", Title: "Only", Hash: "s1", Path: "/s1.flac"},
	})
	m = pressKey(m, "4")
	m = pressKey(m, "t")
	if m.overlay.tagPicker == nil {
		t.Fatal("tag picker did not open")
	}
	if m.overlay.tagPicker.IsBulk() {
		t.Error("a one-track artist should use the single-track picker")
	}
	if got := m.overlay.tagPicker.Scope(); got != "Artist: Solo · 1 track" {
		t.Errorf("scope = %q", got)
	}
}

func TestTagPickerAtTrackLevelHasNoScope(t *testing.T) {
	m, _ := newCollectionTagModel(sampleLibraryTracks())
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m) // Alpha → tracks
	m = pressKey(m, "t")
	if m.overlay.tagPicker == nil {
		t.Fatal("tag picker did not open")
	}
	if got := m.overlay.tagPicker.Hashes(); !reflect.DeepEqual(got, []string{"h1"}) {
		t.Fatalf("track level should target the focused track, got %v", got)
	}
	if s := m.overlay.tagPicker.Scope(); s != "" {
		t.Errorf("track level should have no scope, got %q", s)
	}
}

func TestTagPickerOnEmptyArtistsListNotifies(t *testing.T) {
	m, _ := newCollectionTagModel(nil)
	m = pressKey(m, "4")
	m = pressKey(m, "t")
	if m.overlay.tagPicker != nil {
		t.Fatal("no picker should open without any tracks")
	}
}

func TestUndoPartialTagOnArtistKeepsPreviouslyTaggedTracks(t *testing.T) {
	m, _ := newCollectionTagModel(sampleLibraryTracks())
	m = m.applyTagToggle(7, []string{"h1"}, true) // h1 already tagged
	m = pressKey(m, "4")
	m = pressKey(m, "t")
	if m.overlay.tagPicker == nil {
		t.Fatal("tag picker did not open")
	}

	picker := *m.overlay.tagPicker
	_, cmd := picker.HandleKey("enter", tagKeyEnter())
	toggle := cmd().(tagtui.TagPickerToggleMsg)
	m, _ = m.handleTagPickerToggle(toggle)

	slot, ok := m.undo.slot.action.(undoTagToggleAction)
	if !ok {
		t.Fatalf("undo slot = %T, want undoTagToggleAction", m.undo.slot.action)
	}
	if !reflect.DeepEqual(slot.hashes, []string{"h2", "h3"}) {
		t.Fatalf("undo hashes = %v, want only the newly tagged h2,h3", slot.hashes)
	}
	m, err := slot.perform(m)
	if err != nil {
		t.Fatalf("undo failed: %v", err)
	}
	if _, ok := m.trackTags["h1"][7]; !ok {
		t.Error("undo must keep the tag on h1, which had it before")
	}
	for _, h := range []string{"h2", "h3"} {
		if _, ok := m.trackTags[h][7]; ok {
			t.Errorf("undo should remove the tag from %s", h)
		}
	}
}
