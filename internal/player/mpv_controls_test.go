package player

import (
	"context"
	"strings"
	"sync"
	"testing"
)

func TestManager_SetVolumeAbsolute_Connected(t *testing.T) {
	sock := shortSockPath(t, "vol-abs")

	var mu sync.Mutex
	var received [][]any
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok {
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

	if err := m.SetVolumeAbsolute(75); err != nil {
		t.Fatalf("SetVolumeAbsolute: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if verb, _ := args[0].(string); verb == "set_property" {
				if prop, _ := args[1].(string); prop == "volume" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property volume not sent; got %v", received)
	}
}

func TestManager_SetVolumeAbsolute_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}}
	defer m.Close()

	if err := m.SetVolumeAbsolute(50); err != nil {
		t.Fatalf("SetVolumeAbsolute queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}

func TestManager_SetShuffle_Connected(t *testing.T) {
	sock := shortSockPath(t, "shuffle")

	var mu sync.Mutex
	var received [][]any
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok {
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

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if verb, _ := args[0].(string); verb == "set_property" {
				if prop, _ := args[1].(string); prop == "shuffle" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property shuffle not sent; got %v", received)
	}
}

func TestManager_SetShuffle_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}}
	defer m.Close()

	if err := m.SetShuffle(false); err != nil {
		t.Fatalf("SetShuffle queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}

func TestManager_SetRepeat_AllModes(t *testing.T) {
	for _, mode := range []string{"all", "one", "none"} {
		t.Run(mode, func(t *testing.T) {
			sock := shortSockPath(t, "repeat-"+mode)

			var mu sync.Mutex
			var received [][]any
			fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
				if cmd, ok := req["command"].([]any); ok {
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

			if err := m.SetRepeat(mode); err != nil {
				t.Fatalf("SetRepeat(%s): %v", mode, err)
			}

			// At least 2 set_property calls expected (loop-playlist + loop-file)
			mu.Lock()
			defer mu.Unlock()
			propCount := 0
			for _, args := range received {
				if len(args) >= 1 {
					if verb, _ := args[0].(string); verb == "set_property" {
						propCount++
					}
				}
			}
			if propCount < 2 {
				t.Errorf("SetRepeat(%s): expected 2 set_property calls, got %d; commands: %v", mode, propCount, received)
			}
		})
	}
}

func TestManager_SetRepeat_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}}
	defer m.Close()

	if err := m.SetRepeat("all"); err != nil {
		t.Fatalf("SetRepeat queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}

func TestManager_LoadFilePaused_Connected(t *testing.T) {
	sock := shortSockPath(t, "loadpause")

	var mu sync.Mutex
	var received [][]any
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		if cmd, ok := req["command"].([]any); ok {
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
				// Check the path and that pause=yes appears in options
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