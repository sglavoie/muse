package app_test

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/app"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/player"
	"github.com/sglavoie/muse/internal/tui"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// mockPlayer records calls to SessionPlayer methods for verification.
type mockPlayer struct {
	queue        []player.QueueTrack
	repeatMode   string
	shuffle      bool
	currentIndex int
	indexSet     bool
	volume       float64
	volumeSet    bool
}

func (p *mockPlayer) SetQueue(e []player.QueueTrack) { p.queue = e }
func (p *mockPlayer) SetRepeat(mode string) error    { p.repeatMode = mode; return nil }
func (p *mockPlayer) SetShuffle(enabled bool) error  { p.shuffle = enabled; return nil }
func (p *mockPlayer) RestoreShuffleAnchor(i int)     { p.currentIndex = i; p.indexSet = true }
func (p *mockPlayer) SetVolumeAbsolute(v float64) error {
	p.volume = v
	p.volumeSet = true
	return nil
}

// mockShuffleWriter records any WriteShuffle calls so tests can assert restore
// no longer regenerates shuffle_position (WriteShuffle is not part of any
// interface ApplyRestoredSession consumes).
type mockShuffleWriter struct {
	shuffleCalled bool
	shuffleArg    bool
}

func (w *mockShuffleWriter) WriteShuffle(enabled bool) error {
	w.shuffleCalled = true
	w.shuffleArg = enabled
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

	app.ApplyRestoredSession(&model, mgr, restored, pr, nil, 50)

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

func TestApplyRestoredSession_NilPlaylistReaderNoPanic(t *testing.T) {
	model := tui.New(context.Background())
	mgr := &mockPlayer{}

	app.ApplyRestoredSession(&model, mgr, &app.RestoredSession{}, nil, nil, 50)

	if len(mgr.queue) != 0 {
		t.Errorf("player queue = %d, want 0", len(mgr.queue))
	}
}

func TestApplyRestoredSession_ShuffleAppliedToPlayer(t *testing.T) {
	model := tui.New(context.Background())
	mgr := &mockPlayer{}
	pr := &mockPlaylistReader{}
	restored := &app.RestoredSession{
		QueueEntries: []queue.Entry{
			{ID: 1, Hash: "hash-1", Path: "/music/a.mp3"},
		},
		PlayerTracks: []player.QueueTrack{
			{Hash: "hash-1", Path: "/music/a.mp3"},
		},
		Shuffle: true,
	}

	app.ApplyRestoredSession(&model, mgr, restored, pr, nil, 50)

	if len(mgr.queue) != 1 {
		t.Errorf("player queue = %d, want 1", len(mgr.queue))
	}
	if !mgr.shuffle {
		t.Error("player shuffle should be applied on restore")
	}
}

func TestApplyRestoredSession_NilRestoredNoPanic(t *testing.T) {
	model := tui.New(context.Background())
	mgr := &mockPlayer{}
	pr := &mockPlaylistReader{}

	app.ApplyRestoredSession(&model, mgr, nil, pr, nil, 50)

	if len(mgr.queue) != 0 {
		t.Errorf("player queue = %d, want 0", len(mgr.queue))
	}
	if model.Volume() != 50 {
		t.Errorf("volume = %d, want default 50 for nil restored session", model.Volume())
	}
	if !mgr.volumeSet || mgr.volume != 50 {
		t.Errorf("player volume = %v (set=%v), want default 50", mgr.volume, mgr.volumeSet)
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

	qe := []queue.Entry{
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
		RepeatMode:   playback.RepeatAll,
		Shuffle:      true,
		CurrentHash:  "bbb",
		PositionMs:   5000,
	}

	tracks := []library.Track{
		{Hash: "aaa", Path: "/music/a.mp3"},
		{Hash: "bbb", Path: "/music/b.mp3"},
	}

	app.ApplyRestoredSession(&model, mgr, restored, pr, tracks, 75)

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

	// Shuffle order is Go-owned by the player now; restore must NOT regenerate
	// shuffle_position via WriteShuffle. The restored boolean flag is already
	// persisted and the player's cycle is anchored via RestoreShuffleAnchor.
	if sw.shuffleCalled {
		t.Error("WriteShuffle should NOT be called during restore (shuffle order is Go-owned)")
	}

	// Current track index set to the second track (index 1).
	if !mgr.indexSet || mgr.currentIndex != 1 {
		t.Errorf("currentIndex = %d (set=%v), want 1 (set=true)", mgr.currentIndex, mgr.indexSet)
	}

	// TUI model's currentIndex should also be set so FocusNowPlaying works.
	if got := model.PlaybackCurrentIndex(); got != 1 {
		t.Errorf("model PlaybackCurrentIndex = %d, want 1", got)
	}

	// Playback state should remain idle (no track loaded in mpv yet).
	if got := model.PlaybackStateValue(); got != playback.StateIdle {
		t.Errorf("playback state = %v, want PlaybackIdle", got)
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
	pr := &mockPlaylistReader{}

	qe := []queue.Entry{
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

	app.ApplyRestoredSession(&model, mgr, restored, pr, nil, 50)

	if got := model.StatusBarPlaylistSource(); got != "Queue" {
		t.Errorf("status bar source = %q, want %q", got, "Queue")
	}
}

func TestApplyRestoredSession_VolumeRestored(t *testing.T) {
	mgr := &mockPlayer{}
	pr := &mockPlaylistReader{}

	// When restored.Volume > 0 it should be applied instead of defaultVolume.
	model := tui.New(context.Background())
	restored := &app.RestoredSession{Volume: 70}
	app.ApplyRestoredSession(&model, mgr, restored, pr, nil, 50)
	if model.Volume() != 70 {
		t.Errorf("restored volume: got %d, want 70", model.Volume())
	}

	// When restored.Volume == -1 (no session saved) the defaultVolume should be used.
	model2 := tui.New(context.Background())
	restored2 := &app.RestoredSession{Volume: -1}
	app.ApplyRestoredSession(&model2, mgr, restored2, pr, nil, 50)
	if model2.Volume() != 50 {
		t.Errorf("default volume fallback: got %d, want 50", model2.Volume())
	}
}

func TestApplyRestoredSession_MutedVolumePreserved(t *testing.T) {
	mgr := &mockPlayer{}
	pr := &mockPlaylistReader{}

	// Volume 0 (muted) must survive a restart — it should not fall back to default.
	model := tui.New(context.Background())
	restored := &app.RestoredSession{Volume: 0}
	app.ApplyRestoredSession(&model, mgr, restored, pr, nil, 50)
	if model.Volume() != 0 {
		t.Errorf("muted volume: got %d, want 0", model.Volume())
	}
}

func TestApplyRestoredSession_VolumeSentToPlayer(t *testing.T) {
	for _, tc := range []struct {
		name          string
		restoredVol   int
		defaultVol    int
		wantPlayerVol float64
	}{
		{"restored volume sent to player", 70, 50, 70},
		{"default volume used when no session", -1, 50, 50},
		{"muted volume sent to player", 0, 100, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mgr := &mockPlayer{}
			pr := &mockPlaylistReader{}
			model := tui.New(context.Background())
			restored := &app.RestoredSession{Volume: tc.restoredVol}

			app.ApplyRestoredSession(&model, mgr, restored, pr, nil, tc.defaultVol)

			if !mgr.volumeSet {
				t.Fatal("SetVolumeAbsolute was not called on the player")
			}
			if mgr.volume != tc.wantPlayerVol {
				t.Errorf("player volume = %v, want %v", mgr.volume, tc.wantPlayerVol)
			}
		})
	}
}
