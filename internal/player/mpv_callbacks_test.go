package player

import (
	"context"
	"testing"
)

func TestManager_SetEventCallbacks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, callbacksReady: make(chan struct{})}
	defer m.Close()

	var missingCalled bool
	var endedCalled bool
	var gotHash, gotArtist, gotTitle, gotSource string
	var gotQueueID int64

	m.SetEventCallbacks(EventCallbacks{
		OnMissingFile:   func(_ string) { missingCalled = true },
		OnPlaybackEnded: func() { endedCalled = true },
		OnTrackChanged: func(h, a, tl, src string, _ int, qid int64) {
			gotHash, gotArtist, gotTitle, gotSource, gotQueueID = h, a, tl, src, qid
		},
	})

	m.mu.RLock()
	cb := m.callbacks
	m.mu.RUnlock()

	if cb.OnMissingFile == nil {
		t.Fatal("OnMissingFile not set")
	}
	cb.OnMissingFile("/test")
	if !missingCalled {
		t.Error("OnMissingFile was not invoked")
	}

	if cb.OnPlaybackEnded == nil {
		t.Fatal("OnPlaybackEnded not set")
	}
	cb.OnPlaybackEnded()
	if !endedCalled {
		t.Error("OnPlaybackEnded was not invoked")
	}

	if cb.OnTrackChanged == nil {
		t.Fatal("OnTrackChanged not set")
	}
	cb.OnTrackChanged("h1", "Artist", "Title", "My Playlist", 0, 42)
	if gotHash != "h1" || gotArtist != "Artist" || gotTitle != "Title" || gotSource != "My Playlist" {
		t.Errorf("OnTrackChanged args: got (%q, %q, %q, %q)", gotHash, gotArtist, gotTitle, gotSource)
	}
	if gotQueueID != 42 {
		t.Errorf("OnTrackChanged queue id = %d, want 42", gotQueueID)
	}

	select {
	case <-m.callbacksReady:
	default:
		t.Error("callbacksReady was not closed by SetEventCallbacks")
	}
}

func TestSetEventCallbacks_Idempotent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, callbacksReady: make(chan struct{})}
	defer m.Close()

	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(string) {}})
}
