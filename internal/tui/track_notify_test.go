package tui

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/settings"
)

// fakeTrackNotifier records every NotifyTrack call. drainCmd runs the
// notification cmd on the calling goroutine, but the mutex keeps it honest
// under -race.
type fakeTrackNotifier struct {
	mu    sync.Mutex
	calls [][3]string
	err   error
}

func (f *fakeTrackNotifier) NotifyTrack(title, artist, album string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, [3]string{title, artist, album})
	return f.err
}

func (f *fakeTrackNotifier) recorded() [][3]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([][3]string(nil), f.calls...)
}

// notifyModel builds a model with one queued track, the notifier wired, and
// notifications enabled unless disabled by the caller.
func notifyModel(t *testing.T, notifier trackNotifier, enabled bool) Model {
	t.Helper()
	m := New(context.Background(), WithTrackNotifier(notifier))
	m.notifyOnTrackChange = enabled
	m.queue.Entries = []queue.Entry{{ID: 1, Hash: "h2", Path: "/b.flac"}}
	m.library.Index.BulkLoad([]library.Track{
		{Hash: "h2", Path: "/b.flac", Title: "T2", Artist: "B", Album: "Album B"},
	})
	return m
}

func TestTrackChangedNotifiesOnNewHash(t *testing.T) {
	f := &fakeTrackNotifier{}
	m := notifyModel(t, f, true)
	m.playback.currentHash = "h1"

	_, cmd := m.handleTrackChangedMsg(playback.TrackChangedMsg{Hash: "h2", Index: 0, Artist: "B", Title: "T2"})
	drainCmd(cmd)

	got := f.recorded()
	if len(got) != 1 {
		t.Fatalf("expected exactly 1 notification, got %d: %v", len(got), got)
	}
	if want := [3]string{"T2", "B", "Album B"}; got[0] != want {
		t.Errorf("notification = %v, want %v", got[0], want)
	}
}

func TestTrackChangedDoesNotNotifyWhenDisabled(t *testing.T) {
	f := &fakeTrackNotifier{}
	m := notifyModel(t, f, false)
	m.playback.currentHash = "h1"

	_, cmd := m.handleTrackChangedMsg(playback.TrackChangedMsg{Hash: "h2", Index: 0, Artist: "B", Title: "T2"})
	drainCmd(cmd)

	if got := f.recorded(); len(got) != 0 {
		t.Fatalf("notifications posted with the setting off: %v", got)
	}
}

// TestTrackChangedDoesNotNotifyOnSameHash covers the mpv reconnect / session
// restore path, where the event reader re-emits the track already showing.
func TestTrackChangedDoesNotNotifyOnSameHash(t *testing.T) {
	f := &fakeTrackNotifier{}
	m := notifyModel(t, f, true)
	m.playback.currentHash = "h2"

	_, cmd := m.handleTrackChangedMsg(playback.TrackChangedMsg{Hash: "h2", Index: 0, Artist: "B", Title: "T2"})
	drainCmd(cmd)

	if got := f.recorded(); len(got) != 0 {
		t.Fatalf("notification posted for an unchanged hash: %v", got)
	}
}

func TestTrackChangedSurvivesNotifierError(t *testing.T) {
	f := &fakeTrackNotifier{err: errors.New("alerter not found")}
	m := notifyModel(t, f, true)
	m.playback.currentHash = "h1"

	next, cmd := m.handleTrackChangedMsg(playback.TrackChangedMsg{Hash: "h2", Index: 0, Artist: "B", Title: "T2"})
	drainCmd(cmd)

	if next.playback.currentHash != "h2" {
		t.Errorf("currentHash = %q, want %q", next.playback.currentHash, "h2")
	}
	if next.playback.state != playback.StatePlaying {
		t.Errorf("state = %v, want PlaybackPlaying", next.playback.state)
	}
	if len(f.recorded()) != 1 {
		t.Errorf("expected the failing notifier to still be called once")
	}
}

func TestToggleTrackNotifyFlipsState(t *testing.T) {
	m := Model{}
	m2, _ := m.handleToggleTrackNotifyMsg()
	if !m2.notifyOnTrackChange {
		t.Error("after toggle, notifyOnTrackChange should be true")
	}
	if !m2.cfg.NotifyOnTrackChange {
		t.Error("after toggle, cfg.NotifyOnTrackChange should be true")
	}
	m3, _ := m2.handleToggleTrackNotifyMsg()
	if m3.notifyOnTrackChange {
		t.Error("after second toggle, notifyOnTrackChange should be false again")
	}
}

// TestSettingsSaveSyncsTrackNotifyMirror guards the runtime mirror against
// going stale when the setting is flipped from the Display tab rather than the
// command palette.
func TestSettingsSaveSyncsTrackNotifyMirror(t *testing.T) {
	m := New(context.Background())
	o := settings.NewSettingsOverlay(m.cfg, m.cfg.MusicDirectories)
	o.Fields[settings.TabDisplay][2].Value = 1
	m.overlay.settings = &o

	next, _ := m.handleSettingsSave()

	if !next.cfg.NotifyOnTrackChange {
		t.Fatal("cfg.NotifyOnTrackChange = false after saving the overlay, want true")
	}
	if !next.notifyOnTrackChange {
		t.Error("notifyOnTrackChange mirror = false, want it synced to the saved config")
	}
}
