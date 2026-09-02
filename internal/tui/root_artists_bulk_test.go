package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/keys"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestArtistsBulkActionAtTrackLevel verifies that pressing 'a' at TrackLevel
// opens the destination picker with the focused track.
func TestArtistsBulkActionAtTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)
	if m.browse.Artists.Level != arttui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Artists.Level)
	}

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil after pressing 'a' at TrackLevel")
	}
	if len(m.overlay.destPickerTracks) != 1 {
		t.Fatalf("expected 1 destPickerTrack, got %d", len(m.overlay.destPickerTracks))
	}
	if m.overlay.destPickerTracks[0].Title != "Track A1" {
		t.Errorf("expected destPickerTrack 'Track A1', got %q", m.overlay.destPickerTracks[0].Title)
	}
}

// TestArtistsBulkActionAtArtistLevel verifies that pressing 'a' at ArtistLevel
// opens the destination picker with all tracks by the focused artist.
func TestArtistsBulkActionAtArtistLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil at ArtistLevel")
	}
	if len(m.overlay.destPickerTracks) != 3 {
		t.Fatalf("expected 3 destPickerTracks (all Alice tracks), got %d", len(m.overlay.destPickerTracks))
	}
	for _, tr := range m.overlay.destPickerTracks {
		if tr.Artist != "Alice" {
			t.Errorf("expected all tracks by Alice, got artist %q", tr.Artist)
		}
	}
}

// TestArtistsBulkActionAtAlbumLevel verifies that pressing 'a' at AlbumLevel
// opens the destination picker with all tracks from the focused album.
func TestArtistsBulkActionAtAlbumLevel(t *testing.T) {
	m := newModelWithTracks()
	m = pressKey(m, "4")
	m = pressEnter(m)
	if m.browse.Artists.Level != arttui.AlbumLevel {
		t.Fatalf("expected AlbumLevel, got %v", m.browse.Artists.Level)
	}

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil at AlbumLevel")
	}
	if len(m.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 destPickerTracks (Alpha tracks), got %d", len(m.overlay.destPickerTracks))
	}
}

// TestBulkActionArtistLevelUnknownArtistCollectsTracks verifies that the
// actionBulkAction handler at ArtistLevel correctly collects tracks for the
// "(unknown artist)" sentinel by converting it to an empty string for matching
// against raw track metadata.
func TestBulkActionArtistLevelUnknownArtistCollectsTracks(t *testing.T) {
	tracks := []library.Track{
		{Artist: "", Album: "NoArtist Album", Title: "Track UA1", Hash: "hu1", Path: "/ua1.flac"},
		{Artist: "", Album: "NoArtist Album", Title: "Track UA2", Hash: "hu2", Path: "/ua2.flac"},
		{Artist: "Alice", Album: "Alpha", Title: "Track A1", Hash: "h1", Path: "/a1.flac"},
	}
	m := New(context.Background(), WithLibraryTracks(tracks))
	m = pressKey(m, "4")
	m.browse.Artists.Width = 80
	m.browse.Artists.Height = 40

	unknownIdx := -1
	for i, a := range m.browse.Artists.Artists {
		if a == arttui.UnknownArtist {
			unknownIdx = i
			break
		}
	}
	if unknownIdx < 0 {
		t.Fatal("expected (unknown artist) in artist list")
	}
	m.browse.Artists.Cursor = unknownIdx

	next, _, handled := m.handleLibraryKeyAction(keys.ActionBulkAction)
	if !handled {
		t.Fatal("actionBulkAction should be handled")
	}
	nm := next.(Model)

	if nm.overlay.destPicker == nil {
		t.Fatal("expected destination picker to be opened for unknown artist tracks")
	}
	if len(nm.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 tracks for unknown artist, got %d", len(nm.overlay.destPickerTracks))
	}
}

// TestArtistsSelectedHashesClearedOnDestPickerConfirm verifies that
// artists.selectedHashes is cleared when the destination picker is confirmed
// via DestinationPickedMsg, preventing stale selection markers.
func TestArtistsSelectedHashesClearedOnDestPickerConfirm(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithLibraryTracks(sampleLibraryTracks()))
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)

	m = pressKey(m, "x")
	if len(m.browse.Artists.SelectedHashes) == 0 {
		t.Fatal("expected at least one selected hash after 'x'")
	}

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	m2, _ := m.Update(pltui.DestinationPickedMsg{IsQueue: true})
	nm := m2.(Model)
	if len(nm.browse.Artists.SelectedHashes) != 0 {
		t.Errorf("expected artists.selectedHashes cleared after DestinationPickedMsg, got %v", nm.browse.Artists.SelectedHashes)
	}
}

// TestArtistsSelectedHashesClearedOnDestPickerDismiss verifies that
// artists.selectedHashes is cleared when the destination picker is dismissed.
func TestArtistsSelectedHashesClearedOnDestPickerDismiss(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)
	m = pressKey(m, "x")

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open")
	}

	m2, _ := m.Update(pltui.DestinationPickerDismissedMsg{})
	nm := m2.(Model)
	if len(nm.browse.Artists.SelectedHashes) != 0 {
		t.Errorf("expected artists.selectedHashes cleared after dismiss, got %v", nm.browse.Artists.SelectedHashes)
	}
}

// TestArtistsSelectedHashesClearedOnDestPickerCreate verifies that
// artists.selectedHashes is cleared when the destination picker "create"
// option is chosen.
func TestArtistsSelectedHashesClearedOnDestPickerCreate(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m = pressKey(m, "4")
	m = pressEnter(m)
	m = pressEnter(m)
	m = pressKey(m, "x")

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open")
	}

	m2, _ := m.Update(pltui.DestinationPickerCreateMsg{})
	nm := m2.(Model)
	if len(nm.browse.Artists.SelectedHashes) != 0 {
		t.Errorf("expected artists.selectedHashes cleared after create, got %v", nm.browse.Artists.SelectedHashes)
	}
}
