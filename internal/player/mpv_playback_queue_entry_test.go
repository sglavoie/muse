package player

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// mockMarker records calls to MarkTrackMissing and MarkQueueEntryMissing.
type mockMarker struct {
	trackHash string
	queueID   int64
}

func (m *mockMarker) MarkTrackMissing(hash string) error {
	m.trackHash = hash
	return nil
}

func (m *mockMarker) MarkQueueEntryMissing(id int64) error {
	m.queueID = id
	return nil
}

func TestManager_PlayQueueEntry_MissingFile(t *testing.T) {
	sock := shortSockPath(t, "pqe-missing")

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		cmdQueue:       []func() error{},
		callbacksReady: make(chan struct{}),
	}
	defer m.Close()

	var statusMsg string
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(msg string) { statusMsg = msg }})

	// Connect the manager to a fake server so ipc() succeeds.
	fakeServer(t, sock)
	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	marker := &mockMarker{}
	entry := QueueTrack{
		Path:    "/nonexistent/path/track.flac",
		Hash:    "abc123",
		QueueID: 42,
	}

	gotErr := m.PlayQueueEntry(entry, marker)
	if !errors.Is(gotErr, ErrMissingFile) {
		t.Fatalf("PlayQueueEntry: want ErrMissingFile, got %v", gotErr)
	}
	if marker.trackHash != "abc123" {
		t.Errorf("MarkTrackMissing called with %q, want %q", marker.trackHash, "abc123")
	}
	if marker.queueID != 42 {
		t.Errorf("MarkQueueEntryMissing called with %d, want 42", marker.queueID)
	}
	if statusMsg == "" {
		t.Error("expected a status message for the missing file, got none")
	}
}

func TestManager_PlayQueueEntry_MissingFileNilMarker(t *testing.T) {
	m := NewManager(shortSockPath(t, "pqe-missing-nil-marker"))
	defer m.Close()

	var statusMsg string
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(msg string) { statusMsg = msg }})

	entry := QueueTrack{
		Path:    "/nonexistent/path/track.flac",
		Hash:    "abc123",
		QueueID: 42,
	}

	gotErr := m.PlayQueueEntry(entry, nil)
	if !errors.Is(gotErr, ErrMissingFile) {
		t.Fatalf("PlayQueueEntry: want ErrMissingFile, got %v", gotErr)
	}
	if statusMsg == "" {
		t.Error("expected a status message for the missing file, got none")
	}
}

// TestManager_PlayQueueEntry_NonNotExistStatError verifies that only a true
// missing-file stat result is converted into ErrMissingFile. Other filesystem
// errors must be surfaced directly so the caller can distinguish corruption or
// permission problems from a deleted track.
func TestManager_PlayQueueEntry_NonNotExistStatError(t *testing.T) {
	m := NewManager(shortSockPath(t, "pqe-stat-err"))
	defer m.Close()

	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	badPath := filepath.Join(blocker, "track.flac")
	if _, err := os.Stat(badPath); err == nil {
		t.Fatal("expected os.Stat to fail for a non-directory path component")
	} else if errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected non-not-exist error, got %v", err)
	}

	var statusMsg string
	m.SetEventCallbacks(EventCallbacks{OnStatus: func(msg string) { statusMsg = msg }})

	marker := &mockMarker{}
	entry := QueueTrack{
		Path:    badPath,
		Hash:    "stat-error",
		QueueID: 99,
	}

	gotErr := m.PlayQueueEntry(entry, marker)
	if gotErr == nil {
		t.Fatal("PlayQueueEntry error = nil, want stat error")
	}
	if errors.Is(gotErr, ErrMissingFile) {
		t.Fatalf("PlayQueueEntry error = %v, want raw stat error", gotErr)
	}
	if marker.trackHash != "" {
		t.Errorf("MarkTrackMissing called with %q, want no call", marker.trackHash)
	}
	if marker.queueID != 0 {
		t.Errorf("MarkQueueEntryMissing called with %d, want no call", marker.queueID)
	}
	if statusMsg != "" {
		t.Errorf("unexpected status message for non-missing stat error: %q", statusMsg)
	}
}

func TestManager_PlayQueueEntry_PresentFile(t *testing.T) {
	sock := shortSockPath(t, "pqe-present")

	// Create a real temporary file to stat.
	tmp, err := os.CreateTemp("", "muse-test-*.flac")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	// Track loadfile commands received by the fake server.
	var mu sync.Mutex
	var received []ipcTestCommand
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		if cmd := req.Command; len(cmd) > 0 {
			mu.Lock()
			received = append(received, cmd)
			mu.Unlock()
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
	}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.cmdQueue = []func() error{}
	m.mu.Unlock()

	marker := &mockMarker{}
	entry := QueueTrack{
		Path:    tmp.Name(),
		Hash:    "deadbeef",
		QueueID: 7,
	}

	if err := m.PlayQueueEntry(entry, marker); err != nil {
		t.Fatalf("PlayQueueEntry: unexpected error: %v", err)
	}

	// Marker must not have been called.
	if marker.trackHash != "" || marker.queueID != 0 {
		t.Errorf("marker was called unexpectedly: hash=%q queueID=%d", marker.trackHash, marker.queueID)
	}

	// loadfile must have been sent to mpv.
	mu.Lock()
	defer mu.Unlock()
	for _, args := range received {
		if len(args) > 0 {
			if args.stringIs(0, "loadfile") {
				return
			}
		}
	}
	t.Errorf("no loadfile command received; got %v", received)
}

func TestManager_PlayQueueEntry_NotConnectedQueues(t *testing.T) {
	sock := shortSockPath(t, "pqe-queue")

	// Create a real temporary file so the stat check passes.
	tmp, err := os.CreateTemp("", "muse-test-*.flac")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	tmp.Close()
	t.Cleanup(func() { os.Remove(tmp.Name()) })

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath: sock,
		ctx:        ctx,
		cancel:     cancel,
		cmdQueue:   []func() error{},
	}
	defer m.Close()

	marker := &mockMarker{}
	entry := QueueTrack{Path: tmp.Name(), Hash: "ff", QueueID: 1}

	// Should queue rather than fail when not yet connected.
	if err := m.PlayQueueEntry(entry, marker); err != nil {
		t.Fatalf("PlayQueueEntry before connect: want nil (queued), got %v", err)
	}

	m.mu.RLock()
	qlen := len(m.cmdQueue)
	m.mu.RUnlock()
	if qlen != 1 {
		t.Errorf("cmdQueue len = %d, want 1", qlen)
	}
}

func TestManager_PlayQueueEntry_MissingFileNoCallbacks(t *testing.T) {
	m := NewManager(shortSockPath(t, "pqe-missing-nocallbacks"))
	defer m.Close()

	entry := QueueTrack{
		Path:    "/nonexistent/path/track.flac",
		Hash:    "abc123",
		QueueID: 42,
	}

	// No callbacks set; should still log a warning (not directly testable, but should not panic or error)
	gotErr := m.PlayQueueEntry(entry, nil)
	if !errors.Is(gotErr, ErrMissingFile) {
		t.Fatalf("PlayQueueEntry: want ErrMissingFile, got %v", gotErr)
	}
}
