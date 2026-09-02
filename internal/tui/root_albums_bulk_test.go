package tui

import (
	"context"
	"testing"

	albtui "github.com/sglavoie/muse/internal/tui/albums"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
)

// TestAlbumsBulkActionAtTrackLevel verifies that pressing 'a' at TrackLevel
// opens the destination picker with the focused track.
func TestAlbumsBulkActionAtTrackLevel(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)

	// Drill down: Alpha -> tracks.
	m = pressEnter(m) // -> TrackLevel
	if m.browse.Albums.Level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Albums.Level)
	}

	// Press 'a' to open destination picker.
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

// TestAlbumsBulkActionAtAlbumLevel verifies that pressing 'a' at AlbumLevel
// opens the destination picker with all tracks from the focused album.
func TestAlbumsBulkActionAtAlbumLevel(t *testing.T) {
	m := newModelWithTracks()
	m = switchToAlbumsMode(m)

	// Press 'a' at AlbumLevel (focused on Alpha which has 2 tracks).
	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be non-nil at AlbumLevel")
	}
	if len(m.overlay.destPickerTracks) != 2 {
		t.Fatalf("expected 2 destPickerTracks (Alpha tracks), got %d", len(m.overlay.destPickerTracks))
	}
}

// TestAlbumsSelectedHashesClearedOnDestPickerConfirm verifies that
// albums.selectedHashes is cleared when the destination picker is confirmed
// via DestinationPickedMsg.
func TestAlbumsSelectedHashesClearedOnDestPickerConfirm(t *testing.T) {
	be := &stubBatchEnqueuer{}
	m := New(context.Background(), WithBatchEnqueuer(be), WithLibraryTracks(sampleLibraryTracks()))
	m = switchToAlbumsMode(m) // switch to Albums

	// Drill down: first album -> tracks.
	m = pressEnter(m)
	if m.browse.Albums.Level != albtui.TrackLevel {
		t.Fatalf("expected TrackLevel, got %v", m.browse.Albums.Level)
	}

	// Select the first track.
	m = pressKey(m, "x")
	if len(m.browse.Albums.SelectedHashes) == 0 {
		t.Fatal("expected at least one selected hash after 'x'")
	}

	// Open destination picker.
	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open after 'a'")
	}

	// Simulate DestinationPickedMsg (queue).
	m2, _ := m.Update(pltui.DestinationPickedMsg{IsQueue: true})
	nm := m2.(Model)
	if len(nm.browse.Albums.SelectedHashes) != 0 {
		t.Errorf("expected albums.selectedHashes cleared after DestinationPickedMsg, got %v", nm.browse.Albums.SelectedHashes)
	}
}

// TestAlbumsSelectedHashesClearedOnDestPickerDismiss verifies that
// albums.selectedHashes is cleared when the destination picker is dismissed.
func TestAlbumsSelectedHashesClearedOnDestPickerDismiss(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m = switchToAlbumsMode(m)
	m = pressEnter(m) // tracks

	m = pressKey(m, "x")

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open")
	}

	m2, _ := m.Update(pltui.DestinationPickerDismissedMsg{})
	nm := m2.(Model)
	if len(nm.browse.Albums.SelectedHashes) != 0 {
		t.Errorf("expected albums.selectedHashes cleared after dismiss, got %v", nm.browse.Albums.SelectedHashes)
	}
}

// TestAlbumsSelectedHashesClearedOnDestPickerCreate verifies that
// albums.selectedHashes is cleared when the destination picker "create"
// option is chosen.
func TestAlbumsSelectedHashesClearedOnDestPickerCreate(t *testing.T) {
	m := New(context.Background(), WithLibraryTracks(sampleLibraryTracks()))
	m = switchToAlbumsMode(m)
	m = pressEnter(m) // tracks

	m = pressKey(m, "x")

	m = pressKey(m, "a")
	if m.overlay.destPicker == nil {
		t.Fatal("expected destPicker to be open")
	}

	m2, _ := m.Update(pltui.DestinationPickerCreateMsg{})
	nm := m2.(Model)
	if len(nm.browse.Albums.SelectedHashes) != 0 {
		t.Errorf("expected albums.selectedHashes cleared after create, got %v", nm.browse.Albums.SelectedHashes)
	}
}
