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
func newConnectedManager(t *testing.T, name string, mu *sync.Mutex, received *[]ipcTestCommand) *Manager {
	t.Helper()
	sock := shortSockPath(t, name)
	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		recordCommand(mu, received, req.Command)
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
func waitForCommand(t *testing.T, mu *sync.Mutex, received *[]ipcTestCommand, match func(ipcTestCommand) bool, timeout time.Duration, desc string) {
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

func isLoadfileAppend(cmd ipcTestCommand) bool {
	return len(cmd) >= 3 && cmd.stringIs(0, "loadfile") && cmd.stringIs(2, "append")
}

func isPlaylistRemove(cmd ipcTestCommand) bool {
	return len(cmd) >= 2 && cmd.stringIs(0, "playlist-remove")
}
