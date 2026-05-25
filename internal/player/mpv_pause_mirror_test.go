package player

import (
	"context"
	"sync"
	"testing"
)

// TestManager_Play_MirrorsPauseState verifies that Play updates lastPaused to
// false so a restart firing before mpv's property-change event arrives resumes
// in the playing (unpaused) state.
func TestManager_Play_MirrorsPauseState(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "play-mirror", &mu, &received)

	m.mu.Lock()
	m.lastPaused = true // simulate previously paused state
	m.mu.Unlock()

	if err := m.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if got {
		t.Errorf("lastPaused = true after Play; want false")
	}
}

// TestManager_Pause_MirrorsPauseState verifies that Pause updates lastPaused
// to true.
func TestManager_Pause_MirrorsPauseState(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "pause-mirror", &mu, &received)

	if err := m.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if !got {
		t.Errorf("lastPaused = false after Pause; want true")
	}
}

// TestManager_Stop_MirrorsPauseState verifies that Stop sets lastPaused to
// true after pausing the playback.
func TestManager_Stop_MirrorsPauseState(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "stop-mirror", &mu, &received)

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if !got {
		t.Errorf("lastPaused = false after Stop; want true")
	}
}

// TestManager_Stop_MirrorsPauseStateEvenWhenSeekFails verifies that Stop
// mirrors lastPaused=true after the pause command succeeds, even if the
// follow-up seek fails. Without the mirror happening between the two
// commands, an auto-restart firing after pause-applied-but-seek-failed
// would resume the track in the playing state and break user expectations.
func TestManager_Stop_MirrorsPauseStateEvenWhenSeekFails(t *testing.T) {
	sock := shortSockPath(t, "stop-mirror-seekfail")

	var mu sync.Mutex
	var received []ipcTestCommand

	// Custom handler: succeed for set_property pause=true, but error for seek.
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		cmd := req.Command
		if len(cmd) > 0 {
			mu.Lock()
			received = append(received, cmd)
			mu.Unlock()
		}
		if len(cmd) > 0 {
			if cmd.stringIs(0, "seek") {
				return errorResponse(req, "property unavailable")
			}
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, prefetchedIndex: -1}
	t.Cleanup(func() { m.Close() })

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.lastPaused = false // start from playing
	m.mu.Unlock()

	if err := m.Stop(); err == nil {
		t.Fatal("Stop with failing seek: expected error, got nil")
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if !got {
		t.Errorf("lastPaused = false after Stop when pause succeeded but seek failed; want true (pause was applied)")
	}
}

// TestManager_LoadFilePaused_MirrorsPauseState verifies that LoadFilePaused
// updates lastPaused to true so an auto-restart preserves the paused state.
func TestManager_LoadFilePaused_MirrorsPauseState(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "lfp-mirror", &mu, &received)

	m.mu.Lock()
	m.lastPaused = false // start from non-paused
	m.mu.Unlock()

	if err := m.LoadFilePaused("/music/song.flac", 5000); err != nil {
		t.Fatalf("LoadFilePaused: %v", err)
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if !got {
		t.Errorf("lastPaused = false after LoadFilePaused; want true")
	}
}

// TestManager_PlayQueueEntry_MirrorsPauseState verifies that PlayQueueEntry
// clears lastPaused (sets it to false) so a restart after starting a fresh
// track does not resume it paused.
func TestManager_PlayQueueEntry_MirrorsPauseState(t *testing.T) {
	var mu sync.Mutex
	var received []ipcTestCommand
	m := newConnectedManager(t, "pqe-mirror", &mu, &received)

	// Use a real on-disk file so PlayQueueEntry passes the os.Stat check.
	path := tempFile(t, "song*.flac")

	m.mu.Lock()
	m.lastPaused = true // simulate previously paused state
	m.mu.Unlock()

	entry := QueueTrack{Path: path, Hash: "h0", QueueID: 1}
	if err := m.PlayQueueEntry(entry, noopMarker{}); err != nil {
		t.Fatalf("PlayQueueEntry: %v", err)
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if got {
		t.Errorf("lastPaused = true after PlayQueueEntry; want false (pause=no leaks)")
	}
}

// TestManager_PlayQueueEntry_MissingFile_PreservesPauseState verifies that
// PlayQueueEntry does NOT clear lastPaused when the file is missing — the
// pause state should only change when we actually issue a loadfile command.
func TestManager_PlayQueueEntry_MissingFile_PreservesPauseState(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	m.mu.Lock()
	m.lastPaused = true
	m.mu.Unlock()

	entry := QueueTrack{Path: "/nonexistent-file.flac", Hash: "h0", QueueID: 1}
	if err := m.PlayQueueEntry(entry, noopMarker{}); err == nil {
		t.Fatal("PlayQueueEntry on missing file: expected error, got nil")
	}

	m.mu.RLock()
	got := m.lastPaused
	m.mu.RUnlock()
	if !got {
		t.Errorf("lastPaused changed on missing file path; want true (unchanged)")
	}
}
