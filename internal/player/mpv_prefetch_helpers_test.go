package player

import (
	"context"
	"os"
	"slices"
	"sync"
	"testing"
	"time"
)

// tempFile creates a temporary file and returns its path.
// The file is removed via t.Cleanup.
func tempFile(t *testing.T, name string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), name)
	if err != nil {
		t.Fatalf("tempFile: %v", err)
	}
	f.Close()
	return f.Name()
}

// newConnectedManager creates a Manager with a fake IPC server and connected
// client. The received commands are appended to *received under mu.
func newConnectedManager(t *testing.T, name string, mu *sync.Mutex, received *[][]any) *Manager {
	t.Helper()
	sock := shortSockPath(t, name)
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok {
			mu.Lock()
			*received = append(*received, cmd)
			mu.Unlock()
		}
		return successHandler(req)
	})

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:      sock,
		ctx:             ctx,
		cancel:          cancel,
		prefetchedIndex: -1,
	}
	t.Cleanup(func() { m.Close() })

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()
	return m
}

// waitForCommand waits until a command matching the predicate appears in
// received, or times out.
func waitForCommand(t *testing.T, mu *sync.Mutex, received *[][]any, match func([]any) bool, timeout time.Duration, desc string) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		mu.Lock()
		if slices.ContainsFunc(*received, match) {
			mu.Unlock()
			return
		}
		mu.Unlock()
		select {
		case <-deadline:
			mu.Lock()
			t.Fatalf("timed out waiting for %s; got %v", desc, *received)
			mu.Unlock()
		case <-time.After(5 * time.Millisecond):
		}
	}
}

func isLoadfileAppend(cmd []any) bool {
	if len(cmd) < 3 {
		return false
	}
	verb, _ := cmd[0].(string)
	mode, _ := cmd[2].(string)
	return verb == "loadfile" && mode == "append"
}

func isPlaylistRemove(cmd []any) bool {
	if len(cmd) < 2 {
		return false
	}
	verb, _ := cmd[0].(string)
	return verb == "playlist-remove"
}