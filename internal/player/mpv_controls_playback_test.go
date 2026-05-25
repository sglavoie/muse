package player

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestManager_LoadFilePaused_Connected(t *testing.T) {
	sock := shortSockPath(t, "loadpause")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.LoadFilePaused("/music/song.flac", 42500); err != nil {
		t.Fatalf("LoadFilePaused: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, args := range received {
		if len(args) >= 2 {
			if verb, _ := args[0].(string); verb == "loadfile" {
				if path, _ := args[1].(string); path != "/music/song.flac" {
					t.Errorf("loadfile path = %q, want /music/song.flac", path)
				}
				hasOpts := false
				for _, a := range args {
					if s, ok := a.(string); ok && strings.Contains(s, "pause=yes") && strings.Contains(s, "start=42.5") {
						hasOpts = true
					}
				}
				if !hasOpts {
					t.Errorf("loadfile missing pause=yes/start=42.5; args: %v", args)
				}
				return
			}
		}
	}
	t.Errorf("no loadfile command received; got %v", received)
}

func TestManager_LoadFilePaused_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}}
	defer m.Close()

	if err := m.LoadFilePaused("/a.flac", 1000); err != nil {
		t.Fatalf("LoadFilePaused queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}

// TestManager_SeekAbsolute_NegativeClamped verifies that SeekAbsolute clamps a
// negative position to 0 before forwarding to mpv. Without the clamp, mpv
// interprets negative absolute seek values as seeking from the end of the
// track, causing playback to jump to an unintended position.
func TestManager_SeekAbsolute_NegativeClamped(t *testing.T) {
	sock := shortSockPath(t, "seek-abs")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.SeekAbsolute(-10); err != nil {
		t.Fatalf("SeekAbsolute(-10): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, args := range received {
		if len(args) >= 2 {
			if verb, _ := args[0].(string); verb == "seek" {
				pos, _ := args[1].(float64)
				if pos < 0 {
					t.Errorf("SeekAbsolute sent negative position %v to mpv; want >= 0", pos)
				}
			}
		}
	}
}
