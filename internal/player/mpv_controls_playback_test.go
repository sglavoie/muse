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
			if args.stringIs(0, "loadfile") {
				if path, _ := args.stringAt(1); path != "/music/song.flac" {
					t.Errorf("loadfile path = %q, want /music/song.flac", path)
				}
				if len(args) < 5 {
					t.Errorf("loadfile has too few args: %v", args)
					return
				}
				opts, _ := args.stringAt(4)
				if !strings.Contains(opts, "pause=yes") || !strings.Contains(opts, "start=42.5") {
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

// TestManager_LoadFilePaused_NegativePositionClamped verifies that
// LoadFilePaused clamps a negative positionMs to 0 before forwarding to mpv.
// A stale or corrupt session row could store a negative resume position; we
// must not pass a negative start= option to mpv, which would seek to an
// unintended offset (mpv's start= is documented as relative to the start of
// the file but mpv treats negative values inconsistently).
func TestManager_LoadFilePaused_NegativePositionClamped(t *testing.T) {
	sock := shortSockPath(t, "loadpause-neg")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.LoadFilePaused("/music/song.flac", -5000); err != nil {
		t.Fatalf("LoadFilePaused(-5000): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, args := range received {
		if len(args) >= 5 && args.stringIs(0, "loadfile") {
			opts, _ := args.stringAt(4)
			if strings.Contains(opts, "start=-") {
				t.Errorf("LoadFilePaused sent negative start= option to mpv; args: %v", args)
			}
			if !strings.Contains(opts, "start=0") {
				t.Errorf("LoadFilePaused with negative positionMs did not clamp to start=0; opts=%q", opts)
			}
			return
		}
	}
	t.Errorf("no loadfile command received; got %v", received)
}

// TestManager_SeekAbsolute_NegativeClamped verifies that SeekAbsolute clamps a
// negative position to 0 before forwarding to mpv. Without the clamp, mpv
// interprets negative absolute seek values as seeking from the end of the
// track, causing playback to jump to an unintended position.
func TestManager_SeekAbsolute_NegativeClamped(t *testing.T) {
	sock := shortSockPath(t, "seek-abs")

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
			if args.stringIs(0, "seek") {
				pos, _ := args.float64At(1)
				if pos < 0 {
					t.Errorf("SeekAbsolute sent negative position %v to mpv; want >= 0", pos)
				}
			}
		}
	}
}
