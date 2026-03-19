package app_test

import (
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
)

// mockModel records calls to SessionModel methods for verification.
type mockModel struct {
	queueEntries  []tui.QueueEntry
	libraryTracks []library.Track
	removedPaths  []string
	playlists     []db.PlaylistRow
	repeatMode    tui.RepeatMode
	shuffle       bool
	volume        int
	playbackState tui.PlaybackState
	currentHash   string
}

func (m *mockModel) SetQueueEntries(e []tui.QueueEntry)   { m.queueEntries = e }
func (m *mockModel) SetLibraryTracks(t []library.Track)   { m.libraryTracks = t }
func (m *mockModel) RemoveLibraryTrackByPath(p string)    { m.removedPaths = append(m.removedPaths, p) }
func (m *mockModel) SetPlaylists(r []db.PlaylistRow)      { m.playlists = r }
func (m *mockModel) SetRepeatMode(mode tui.RepeatMode)    { m.repeatMode = mode }
func (m *mockModel) SetShuffle(enabled bool)              { m.shuffle = enabled }
func (m *mockModel) SetVolume(v int)                      { m.volume = v }
func (m *mockModel) SetPlaybackState(s tui.PlaybackState) { m.playbackState = s }
func (m *mockModel) SetCurrentTrackHash(h string)         { m.currentHash = h }

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
	model := &mockModel{}
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{}
	restored := &app.RestoredSession{}

	app.ApplyRestoredSession(model, mgr, restored, sw, pr, nil, nil, nil, 50)

	if model.volume != 50 {
		t.Errorf("volume = %d, want 50", model.volume)
	}
	if model.repeatMode != tui.RepeatOff {
		t.Errorf("repeatMode = %v, want RepeatOff", model.repeatMode)
	}
	if model.shuffle {
		t.Error("shuffle should be false")
	}
	if model.playbackState != 0 {
		t.Errorf("playbackState should be zero value, got %v", model.playbackState)
	}
	if model.currentHash != "" {
		t.Errorf("currentHash = %q, want empty", model.currentHash)
	}
	if sw.shuffleCalled {
		t.Error("WriteShuffle should not be called for empty session")
	}
	if mgr.indexSet {
		t.Error("SetCurrentIndex should not be called for empty session")
	}
}

func TestApplyRestoredSession_FullSession(t *testing.T) {
	model := &mockModel{}
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{
		playlists: []db.PlaylistRow{
			{ID: 1, Name: "Favorites"},
		},
	}

	qe := []tui.QueueEntry{
		{ID: 1, Hash: "aaa", Path: "/music/a.mp3"},
		{ID: 2, Hash: "bbb", Path: "/music/b.mp3"},
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

	app.ApplyRestoredSession(model, mgr, restored, sw, pr, tracks, nil, nil, 75)

	// Queue synced.
	if len(model.queueEntries) != 2 {
		t.Errorf("model queue entries = %d, want 2", len(model.queueEntries))
	}
	if len(mgr.queue) != 2 {
		t.Errorf("player queue = %d, want 2", len(mgr.queue))
	}

	// Library tracks set.
	if len(model.libraryTracks) != 2 {
		t.Errorf("library tracks = %d, want 2", len(model.libraryTracks))
	}

	// Playlists loaded.
	if len(model.playlists) != 1 {
		t.Errorf("playlists = %d, want 1", len(model.playlists))
	}

	// Repeat/shuffle propagated to both model and player.
	if model.repeatMode != tui.RepeatAll {
		t.Errorf("repeatMode = %v, want RepeatAll", model.repeatMode)
	}
	if mgr.repeatMode != "all" {
		t.Errorf("player repeatMode = %q, want %q", mgr.repeatMode, "all")
	}
	if !model.shuffle {
		t.Error("model shuffle should be true")
	}
	if !mgr.shuffle {
		t.Error("player shuffle should be true")
	}

	// Shuffle write called because shuffle=true and queue non-empty.
	if !sw.shuffleCalled || !sw.shuffleArg {
		t.Error("WriteShuffle(true) should have been called")
	}

	// Volume.
	if model.volume != 75 {
		t.Errorf("volume = %d, want 75", model.volume)
	}

	// Playback state paused (queue non-empty).
	if model.playbackState != tui.PlaybackPaused {
		t.Errorf("playbackState = %v, want PlaybackPaused", model.playbackState)
	}

	// Current track + index.
	if model.currentHash != "bbb" {
		t.Errorf("currentHash = %q, want %q", model.currentHash, "bbb")
	}
	if !mgr.indexSet || mgr.currentIndex != 1 {
		t.Errorf("currentIndex = %d (set=%v), want 1 (set=true)", mgr.currentIndex, mgr.indexSet)
	}
}

func TestApplyRestoredSession_DeletedPathsEvicted(t *testing.T) {
	model := &mockModel{}
	mgr := &mockPlayer{}
	sw := &mockShuffleWriter{}
	pr := &mockPlaylistReader{}
	restored := &app.RestoredSession{}

	deletedPaths := []string{"/music/gone.mp3", "/music/also-gone.mp3"}
	hashByPath := map[string]string{
		"/music/gone.mp3":      "hash1",
		"/music/also-gone.mp3": "hash2",
	}

	app.ApplyRestoredSession(model, mgr, restored, sw, pr, nil, deletedPaths, hashByPath, 50)

	if len(model.removedPaths) != 2 {
		t.Fatalf("removed paths = %d, want 2", len(model.removedPaths))
	}
	if len(sw.missingHashes) != 2 {
		t.Fatalf("missing hashes = %d, want 2", len(sw.missingHashes))
	}
	if sw.missingHashes[0] != "hash1" || sw.missingHashes[1] != "hash2" {
		t.Errorf("missing hashes = %v, want [hash1 hash2]", sw.missingHashes)
	}
}
