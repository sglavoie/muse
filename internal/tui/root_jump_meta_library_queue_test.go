package tui

import (
	"context"
	"testing"
)

// TestFocusedTrackMeta_Library verifies that the library cursor drives the
// returned artist and album values.
func TestFocusedTrackMeta_Library(t *testing.T) {
	m := newModelWithTracks()
	// Default active view is viewLibrary, cursor=0 -> Alice/Alpha.
	artist, album, _ := m.focusedTrackMeta()
	if artist != "Alice" {
		t.Errorf("artist: want %q, got %q", "Alice", artist)
	}
	if album != "Alpha" {
		t.Errorf("album: want %q, got %q", "Alpha", album)
	}
}

// TestFocusedTrackMeta_Library_SecondTrack verifies the cursor can be advanced
// to return a different track's metadata.
func TestFocusedTrackMeta_Library_SecondTrack(t *testing.T) {
	m := newModelWithTracks()
	m.library.cursor = 3 // Bob/Gamma/Track G1
	artist, album, _ := m.focusedTrackMeta()
	if artist != "Bob" {
		t.Errorf("artist: want %q, got %q", "Bob", artist)
	}
	if album != "Gamma" {
		t.Errorf("album: want %q, got %q", "Gamma", album)
	}
}

// TestFocusedTrackMeta_Queue verifies that the queue cursor drives the
// returned artist and album values.
func TestFocusedTrackMeta_Queue(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Artist: "Bob", Album: "Gamma", Title: "Track G1"},
		{Artist: "Alice", Album: "Alpha", Title: "Track A1"},
	}
	m.queue.cursor = 0

	artist, album, _ := m.focusedTrackMeta()
	if artist != "Bob" {
		t.Errorf("artist: want %q, got %q", "Bob", artist)
	}
	if album != "Gamma" {
		t.Errorf("album: want %q, got %q", "Gamma", album)
	}
}

// TestFocusedTrackMeta_Queue_SecondEntry advances the cursor to a different entry.
func TestFocusedTrackMeta_Queue_SecondEntry(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	m.queue.entries = []QueueEntry{
		{Artist: "Bob", Album: "Gamma", Title: "Track G1"},
		{Artist: "Alice", Album: "Alpha", Title: "Track A1"},
	}
	m.queue.cursor = 1

	artist, album, _ := m.focusedTrackMeta()
	if artist != "Alice" {
		t.Errorf("artist: want %q, got %q", "Alice", artist)
	}
	if album != "Alpha" {
		t.Errorf("album: want %q, got %q", "Alpha", album)
	}
}

// TestFocusedTrackMeta_EmptyView verifies that an empty library returns ("","",false).
func TestFocusedTrackMeta_EmptyView(t *testing.T) {
	m := New(context.Background())
	artist, album, found := m.focusedTrackMeta()
	if artist != "" || album != "" {
		t.Errorf("empty library: want (%q,%q), got (%q,%q)", "", "", artist, album)
	}
	if found {
		t.Error("empty library: expected found=false")
	}
}

// TestFocusedTrackMeta_QueueEmpty verifies that an empty queue returns ("","",false).
func TestFocusedTrackMeta_QueueEmpty(t *testing.T) {
	m := newModelWithTracks()
	m.activeView = viewQueue
	artist, album, found := m.focusedTrackMeta()
	if artist != "" || album != "" {
		t.Errorf("empty queue: want (%q,%q), got (%q,%q)", "", "", artist, album)
	}
	if found {
		t.Error("empty queue: expected found=false")
	}
}
