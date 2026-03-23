package app_test

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
)

// mockPlayer records calls to SessionPlayer methods for verification.
type mockPlayer struct {
	queue        []player.QueueTrack
	repeatMode   string
	shuffle      bool
	currentIndex int
	indexSet     bool
}

func (p *mockPlayer) SetQueue(e []player.QueueTrack) { p.queue = e }
func (p *mockPlayer) SetRepeat(mode string) error    { p.repeatMode = mode; return nil }
func (p *mockPlayer) SetShuffle(enabled bool) error  { p.shuffle = enabled; return nil }
func (p *mockPlayer) SetCurrentIndex(i int)          { p.currentIndex = i; p.indexSet = true }

// mockShuffleWriter records calls to ShuffleWriter.
type mockShuffleWriter struct {
	shuffleCalled bool
	shuffleArg    bool
	missingHashes []string
}

func (w *mockShuffleWriter) WriteShuffle(enabled bool) error {
	w.shuffleCalled = true
	w.shuffleArg = enabled
	return nil
}

func (w *mockShuffleWriter) MarkTrackMissing(hash string) error {
	w.missingHashes = append(w.missingHashes, hash)
	return nil
}

// mockPlaylistReader returns fixed playlists.
type mockPlaylistReader struct {
	playlists []db.PlaylistRow
}

func (r *mockPlaylistReader) ReadPlaylists() ([]db.PlaylistRow, error) {
	return r.playlists, nil
}

func TestApplyRestoredSession_EmptySession(t *testing.T) {
	model := tui.New(context.Background())
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{}
	restored := &app.RestoredSession{}

	app.ApplyRestoredSession(&model, mgr, restored, sw, pr, nil, nil, nil, 50)

	if sw.shuffleCalled {
		t.Error("WriteShuffle should not be called for empty session")
	}
	if mgr.indexSet {
		t.Error("SetCurrentIndex should not be called for empty session")
	}
	if len(mgr.queue) != 0 {
		t.Errorf("player queue = %d, want 0", len(mgr.queue))
	}
}

func TestApplyRestoredSession_FullSession(t *testing.T) {
	model := tui.New(context.Background())
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{
		playlists: []db.PlaylistRow{
			{ID: 1, Name: "Favorites"},
		},
	}

	qe := []tui.QueueEntry{
		{ID: 1, Hash: "aaa", Path: "/music/a.mp3", Artist: "Artist A", Title: "Song A", SourcePlaylist: "Rock"},
		{ID: 2, Hash: "bbb", Path: "/music/b.mp3", Artist: "Artist B", Title: "Song B", SourcePlaylist: "Jazz"},
	}
	pt := []player.QueueTrack{
		{Hash: "aaa", Path: "/music/a.mp3"},
		{Hash: "bbb", Path: "/music/b.mp3"},
	}

	restored := &app.RestoredSession{
		QueueEntries: qe,
		PlayerTracks: pt,
		RepeatMode:   tui.RepeatAll,
		Shuffle:      true,
		CurrentHash:  "bbb",
		PositionMs:   5000,
	}

	tracks := []library.Track{
		{Hash: "aaa", Path: "/music/a.mp3"},
		{Hash: "bbb", Path: "/music/b.mp3"},
	}

	app.ApplyRestoredSession(&model, mgr, restored, sw, pr, tracks, nil, nil, 75)

	// Player queue synced.
	if len(mgr.queue) != 2 {
		t.Errorf("player queue = %d, want 2", len(mgr.queue))
	}

	// Repeat/shuffle propagated to player.
	if mgr.repeatMode != "all" {
		t.Errorf("player repeatMode = %q, want %q", mgr.repeatMode, "all")
	}
	if !mgr.shuffle {
		t.Error("player shuffle should be true")
	}

	// Shuffle write called because shuffle=true and queue non-empty.
	if !sw.shuffleCalled || !sw.shuffleArg {
		t.Error("WriteShuffle(true) should have been called")
	}

	// Current track index set to the second track (index 1).
	if !mgr.indexSet || mgr.currentIndex != 1 {
		t.Errorf("currentIndex = %d (set=%v), want 1 (set=true)", mgr.currentIndex, mgr.indexSet)
	}

	// Status bar populated from the current track's queue entry.
	if got := model.StatusBarArtist(); got != "Artist B" {
		t.Errorf("status bar artist = %q, want %q", got, "Artist B")
	}
	if got := model.StatusBarTitle(); got != "Song B" {
		t.Errorf("status bar title = %q, want %q", got, "Song B")
	}
	if got := model.StatusBarPlaylistSource(); got != "Jazz" {
		t.Errorf("status bar source = %q, want %q", got, "Jazz")
	}
}

func TestApplyRestoredSession_StatusBarFallbackSource(t *testing.T) {
	model := tui.New(context.Background())
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{}

	qe := []tui.QueueEntry{
		{ID: 1, Hash: "aaa", Path: "/music/a.mp3", Artist: "Art", Title: "Song"},
	}
	pt := []player.QueueTrack{
		{Hash: "aaa", Path: "/music/a.mp3"},
	}
	restored := &app.RestoredSession{
		QueueEntries: qe,
		PlayerTracks: pt,
		CurrentHash:  "aaa",
	}

	app.ApplyRestoredSession(&model, mgr, restored, sw, pr, nil, nil, nil, 50)

	if got := model.StatusBarPlaylistSource(); got != "Queue" {
		t.Errorf("status bar source = %q, want %q", got, "Queue")
	}
}

func TestApplyRestoredSession_DeletedPathsEvicted(t *testing.T) {
	model := tui.New(context.Background())
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{}
	restored := &app.RestoredSession{}

	deletedPaths := []string{"/music/gone.mp3", "/music/also-gone.mp3"}
	hashByPath := map[string]string{
		"/music/gone.mp3":      "hash1",
		"/music/also-gone.mp3": "hash2",
	}

	app.ApplyRestoredSession(&model, mgr, restored, sw, pr, nil, deletedPaths, hashByPath, 50)

	// Both deleted paths should be tracked as missing.
	if len(sw.missingHashes) != 2 {
		t.Fatalf("missing hashes = %d, want 2", len(sw.missingHashes))
	}
	gotHashes := map[string]bool{}
	for _, h := range sw.missingHashes {
		gotHashes[h] = true
	}
	for _, want := range []string{"hash1", "hash2"} {
		if !gotHashes[want] {
			t.Errorf("expected missing hash %q, got %v", want, sw.missingHashes)
		}
	}
}

func TestApplyRestoredSession_VolumeRestored(t *testing.T) {
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{}

	// When restored.Volume > 0 it should be applied instead of defaultVolume.
	model := tui.New(context.Background())
	restored := &app.RestoredSession{Volume: 70}
	app.ApplyRestoredSession(&model, mgr, restored, sw, pr, nil, nil, nil, 50)
	if model.Volume() != 70 {
		t.Errorf("restored volume: got %d, want 70", model.Volume())
	}

	// When restored.Volume == 0 the defaultVolume should be used.
	model2 := tui.New(context.Background())
	restored2 := &app.RestoredSession{Volume: 0}
	app.ApplyRestoredSession(&model2, mgr, restored2, sw, pr, nil, nil, nil, 50)
	if model2.Volume() != 50 {
		t.Errorf("default volume fallback: got %d, want 50", model2.Volume())
	}
}
