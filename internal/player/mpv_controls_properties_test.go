package player

import (
	"context"
	"math"
	"sync"
	"testing"
)

func TestManager_SetVolumeAbsolute_Connected(t *testing.T) {
	sock := shortSockPath(t, "vol-abs")

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

	if err := m.SetVolumeAbsolute(75); err != nil {
		t.Fatalf("SetVolumeAbsolute: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "volume") {
					found = true
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property volume not sent; got %v", received)
	}
}

// TestManager_SetVolumeAbsolute_Clamped verifies that SetVolumeAbsolute
// clamps values outside the documented 0-100 range before forwarding to mpv.
// Out-of-range values cause inconsistent behavior across mpv versions and
// audio backends (some clip silently, some reject), so the Manager normalizes
// at the API boundary.
func TestManager_SetVolumeAbsolute_Clamped(t *testing.T) {
	cases := []struct {
		name  string
		input float64
		want  float64
	}{
		{"negative", -25, 0},
		{"over_max", 150, 100},
		{"nan", math.NaN(), 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sock := shortSockPath(t, "vol-clamp-"+tc.name)

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

			if err := m.SetVolumeAbsolute(tc.input); err != nil {
				t.Fatalf("SetVolumeAbsolute(%v): %v", tc.input, err)
			}

			mu.Lock()
			defer mu.Unlock()
			found := false
			for _, args := range received {
				if len(args) >= 3 && args.stringIs(0, "set_property") && args.stringIs(1, "volume") {
					val, ok := args.float64At(2)
					if !ok {
						t.Fatalf("volume arg not a float64: %v", args)
					}
					if val != tc.want {
						t.Errorf("SetVolumeAbsolute(%v) sent volume=%v; want %v", tc.input, val, tc.want)
					}
					found = true
				}
			}
			if !found {
				t.Errorf("set_property volume not sent; got %v", received)
			}
		})
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

// TestManager_SetShuffle_Connected verifies that SetShuffle is Go-owned: it
// updates the manager's traversal state and never sends mpv's shuffle property,
// even when connected.
func TestManager_SetShuffle_Connected(t *testing.T) {
	sock := shortSockPath(t, "shuffle")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, prefetchedIndex: -1, shufflePos: -1}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.queue = []QueueTrack{
		{Path: "/a.flac", Hash: "h0", QueueID: 1},
		{Path: "/b.flac", Hash: "h1", QueueID: 2},
	}
	m.currentIndex = 0
	m.client = client
	m.mu.Unlock()

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}

	m.mu.RLock()
	enabled := m.shuffleEnabled
	histLen := len(m.shuffleHistory)
	unseenLen := len(m.shuffleUnseen)
	m.mu.RUnlock()
	if !enabled {
		t.Error("shuffleEnabled = false, want true after SetShuffle(true)")
	}
	if histLen != 1 || unseenLen != 1 {
		t.Errorf("history/unseen = %d/%d, want 1/1 (anchor current + 1 other)", histLen, unseenLen)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, args := range received {
		if args.stringIs(0, "set_property") && args.stringIs(1, "shuffle") {
			t.Errorf("set_property shuffle must not be sent; got %v", received)
		}
	}
}

// TestManager_SetShuffle_NotConnectedQueues verifies that SetShuffle works
// without a connection (and without enqueueing an IPC command): the traversal
// state is updated purely in memory so it functions before connect and during
// reconnect.
func TestManager_SetShuffle_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}, prefetchedIndex: -1, shufflePos: -1}
	defer m.Close()

	if err := m.SetShuffle(true); err != nil {
		t.Fatalf("SetShuffle(true): %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	enabled := m.shuffleEnabled
	m.mu.RUnlock()
	if n != 0 {
		t.Errorf("cmdQueue len = %d, want 0 (SetShuffle must not enqueue IPC)", n)
	}
	if !enabled {
		t.Error("shuffleEnabled = false, want true")
	}
}

func TestManager_SetRepeat_AllModes(t *testing.T) {
	for _, mode := range []string{"all", "one", "none"} {
		t.Run(mode, func(t *testing.T) {
			sock := shortSockPath(t, "repeat-"+mode)

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

			if err := m.SetRepeat(mode); err != nil {
				t.Fatalf("SetRepeat(%s): %v", mode, err)
			}

			mu.Lock()
			defer mu.Unlock()
			propCount := 0
			for _, args := range received {
				if len(args) >= 1 {
					if args.stringIs(0, "set_property") {
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
