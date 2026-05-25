package player

import (
	"context"
	"errors"
	"os"
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

// TestManager_PlayQueueEntry_InaccessibleFile verifies that PlayQueueEntry
// returns ErrMissingFile when os.Stat fails with a non-"not exist" error
// (e.g. permission denied), rather than falling through to load the file.
func TestManager_PlayQueueEntry_InaccessibleFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root to trigger permission errors")
	}

	sock := shortSockPath(t, "pqe-noperm")

	// Create a file inside a directory, then revoke the directory's execute
	// permission so os.Stat on the file returns a permission error.
	dir := t.TempDir()
	filePath := dir + "/track.flac"
	if err := os.WriteFile(filePath, []byte("data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) }) //nolint:gosec // restoring dir permissions requires 0o700

	// Verify that the file is genuinely inaccessible (permission error, not
	// "not exist") before testing the player code path.
	if _, err := os.Stat(filePath); err == nil {
		t.Fatal("expected os.Stat to fail on inaccessible file")
	} else if os.IsNotExist(err) {
		t.Fatalf("expected permission error, got not-exist: %v", err)
	}

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
		Path:    filePath,
		Hash:    "perm-denied",
		QueueID: 99,
	}

	gotErr := m.PlayQueueEntry(entry, marker)
	if !errors.Is(gotErr, ErrMissingFile) {
		t.Fatalf("PlayQueueEntry: want ErrMissingFile for inaccessible file, got %v", gotErr)
	}
	if marker.trackHash != "perm-denied" {
		t.Errorf("MarkTrackMissing called with %q, want %q", marker.trackHash, "perm-denied")
	}
	if marker.queueID != 99 {
		t.Errorf("MarkQueueEntryMissing called with %d, want 99", marker.queueID)
	}
	if statusMsg == "" {
		t.Error("expected a status message for the inaccessible file, got none")
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
	var received [][]any
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
			if name, _ := args[0].(string); name == "loadfile" {
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
