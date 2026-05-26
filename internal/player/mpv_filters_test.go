package player

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func TestManager_SetNormalize_Enabled(t *testing.T) {
	sock := shortSockPath(t, "norm-on")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.SetNormalize(true); err != nil {
		t.Fatalf("SetNormalize(true): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "af") {
					if args.stringIs(2, "lavfi=[loudnorm]") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property af \"lavfi=[loudnorm]\" not sent; got %v", received)
	}
}

func TestManager_SetNormalize_Disabled(t *testing.T) {
	sock := shortSockPath(t, "norm-off")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.SetNormalize(false); err != nil {
		t.Fatalf("SetNormalize(false): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "af") {
					if val, _ := args.stringAt(2); val == "" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property af \"\" not sent; got %v", received)
	}
}

func TestManager_SetNormalize_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}, audioFilters: make(map[string]string)}
	defer m.Close()

	if err := m.SetNormalize(true); err != nil {
		t.Fatalf("SetNormalize queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}

func TestManager_SetNormalize_RevertsStateWhenRebuildFails(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		sock := shortSockPath(t, "norm-fail-on")
		fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
			if len(req.Command) >= 2 && req.Command.stringIs(0, cmdSetProperty) && req.Command.stringIs(1, propAF) {
				return errorResponse(req, "forced failure")
			}
			return successHandler(req)
		})

		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
		defer m.Close()

		client, err := NewIPCClient(sock)
		if err != nil {
			t.Fatalf("NewIPCClient: %v", err)
		}
		m.mu.Lock()
		m.client = client
		m.audioFilters["bass"] = "bass=g=6"
		m.mu.Unlock()

		if err := m.SetNormalize(true); !errors.Is(err, ErrPlaybackFailed) {
			t.Fatalf("SetNormalize(true): want ErrPlaybackFailed, got %v", err)
		}

		assertAudioFilters(t, m, map[string]string{"bass": "bass=g=6"})
	})

	t.Run("disable", func(t *testing.T) {
		sock := shortSockPath(t, "norm-fail-off")
		fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
			if len(req.Command) >= 2 && req.Command.stringIs(0, cmdSetProperty) && req.Command.stringIs(1, propAF) {
				return errorResponse(req, "forced failure")
			}
			return successHandler(req)
		})

		ctx, cancel := context.WithCancel(context.Background())
		m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
		defer m.Close()

		client, err := NewIPCClient(sock)
		if err != nil {
			t.Fatalf("NewIPCClient: %v", err)
		}
		m.mu.Lock()
		m.client = client
		m.audioFilters["bass"] = "bass=g=6"
		m.audioFilters["loudnorm"] = "loudnorm"
		m.mu.Unlock()

		if err := m.SetNormalize(false); !errors.Is(err, ErrPlaybackFailed) {
			t.Fatalf("SetNormalize(false): want ErrPlaybackFailed, got %v", err)
		}

		assertAudioFilters(t, m, map[string]string{
			"bass":     "bass=g=6",
			"loudnorm": "loudnorm",
		})
	})
}

func TestManager_SetNormalize_BeforeStartLeavesFiltersUnchanged(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		m := NewManager(shortSockPath(t, "norm-pre-on"))
		defer m.Close()

		m.mu.Lock()
		m.audioFilters["bass"] = "bass=g=6"
		m.mu.Unlock()

		if err := m.SetNormalize(true); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("SetNormalize(true): want ErrNotConnected, got %v", err)
		}

		assertAudioFilters(t, m, map[string]string{"bass": "bass=g=6"})
	})

	t.Run("disable", func(t *testing.T) {
		m := NewManager(shortSockPath(t, "norm-pre-off"))
		defer m.Close()

		m.mu.Lock()
		m.audioFilters["bass"] = "bass=g=6"
		m.audioFilters["loudnorm"] = "loudnorm"
		m.mu.Unlock()

		if err := m.SetNormalize(false); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("SetNormalize(false): want ErrNotConnected, got %v", err)
		}

		assertAudioFilters(t, m, map[string]string{
			"bass":     "bass=g=6",
			"loudnorm": "loudnorm",
		})
	})
}

func TestManager_FilterChain_MultipleFilters(t *testing.T) {
	sock := shortSockPath(t, "fc-multi")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.audioFilters["bass"] = "bass=g=6"
	m.audioFilters["loudnorm"] = "loudnorm"
	m.mu.Unlock()

	if err := m.rebuildAudioFilters(); err != nil {
		t.Fatalf("rebuildAudioFilters: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "af") {
					if args.stringIs(2, "lavfi=[bass=g=6,loudnorm]") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property af \"lavfi=[bass=g=6,loudnorm]\" not sent; got %v", received)
	}
}

func TestManager_FilterChain_RemoveOne(t *testing.T) {
	sock := shortSockPath(t, "fc-remove")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.audioFilters["bass"] = "bass=g=6"
	m.audioFilters["loudnorm"] = "loudnorm"
	m.mu.Unlock()

	// Remove "bass" filter.
	if err := m.setFilter("bass", "bass=g=6", false); err != nil {
		t.Fatalf("setFilter remove bass: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "af") {
					if args.stringIs(2, "lavfi=[loudnorm]") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property af \"lavfi=[loudnorm]\" not sent after removing bass; got %v", received)
	}
}

func TestManager_FilterChain_Empty(t *testing.T) {
	sock := shortSockPath(t, "fc-empty")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.rebuildAudioFilters(); err != nil {
		t.Fatalf("rebuildAudioFilters (empty): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "af") {
					if val, _ := args.stringAt(2); val == "" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property af \"\" not sent for empty filter chain; got %v", received)
	}
}

// TestManager_RebuildAudioFilters_ConcurrentSetFilter exercises the data-race
// fix in rebuildAudioFilters: one goroutine repeatedly calls setFilter
// (modifying the audioFilters map under m.mu) while another calls
// rebuildAudioFilters (reading the map). Under -race this would panic before
// the snapshot-based fix.
func TestManager_RebuildAudioFilters_ConcurrentSetFilter(t *testing.T) {
	sock := shortSockPath(t, "race-af")
	fakeServer(t, sock)

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	t.Cleanup(func() { client.Close() })

	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		socketPath:     sock,
		ctx:            ctx,
		cancel:         cancel,
		audioFilters:   make(map[string]string),
		callbacksReady: make(chan struct{}),
	}
	defer m.Close()

	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	// Run concurrent setFilter and rebuildAudioFilters calls. Under the
	// race detector this will surface any unsynchronised map access.
	const iterations = 50
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := range iterations {
			name := fmt.Sprintf("filter%d", i%3)
			_ = m.setFilter(name, name+"=expr", i%2 == 0)
		}
	}()

	go func() {
		defer wg.Done()
		for range iterations {
			_ = m.rebuildAudioFilters()
		}
	}()

	wg.Wait()
}
