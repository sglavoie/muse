package player

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestManager_SetEqualizer_Enabled(t *testing.T) {
	sock := shortSockPath(t, "eq-on")

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

	bands := [10]int{6, 5, 4, 2, 0, 0, 0, 0, 0, 0}
	if err := m.SetEqualizer(true, bands); err != nil {
		t.Fatalf("SetEqualizer(true): %v", err)
	}

	wantAF := "lavfi=[firequalizer=gain_entry='entry(31,6);entry(62,5);entry(125,4);entry(250,2);entry(500,0);entry(1000,0);entry(2000,0);entry(4000,0);entry(8000,0);entry(16000,0)']"

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "af") {
					if val, _ := args.stringAt(2); val == wantAF {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("expected af %q not sent; got %v", wantAF, received)
	}
}

func TestManager_SetEqualizer_Disabled(t *testing.T) {
	sock := shortSockPath(t, "eq-off")

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

	// Pre-populate the filter so disabling actually removes it.
	m.audioFilters["equalizer"] = "firequalizer=gain_entry='entry(31,6)'"

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.SetEqualizer(false, [10]int{}); err != nil {
		t.Fatalf("SetEqualizer(false): %v", err)
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
		t.Errorf("expected set_property af \"\" not sent; got %v", received)
	}
}

func TestManager_SetEqualizer_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}, audioFilters: make(map[string]string)}
	defer m.Close()

	if err := m.SetEqualizer(true, [10]int{}); err != nil {
		t.Fatalf("SetEqualizer queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}

func TestManager_SetEqualizer_BeforeStartLeavesFiltersUnchanged(t *testing.T) {
	bands := [10]int{6, 5, 4, 2, 0, 0, 0, 0, 0, 0}

	t.Run("enable", func(t *testing.T) {
		m := NewManager(shortSockPath(t, "eq-pre-on"))
		defer m.Close()

		m.mu.Lock()
		m.audioFilters["loudnorm"] = "loudnorm"
		m.mu.Unlock()

		if err := m.SetEqualizer(true, bands); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("SetEqualizer(true): want ErrNotConnected, got %v", err)
		}

		assertAudioFilters(t, m, map[string]string{"loudnorm": "loudnorm"})
	})

	t.Run("disable", func(t *testing.T) {
		m := NewManager(shortSockPath(t, "eq-pre-off"))
		defer m.Close()

		want := map[string]string{
			"equalizer": "firequalizer=gain_entry='entry(31,6)'",
			"loudnorm":  "loudnorm",
		}

		m.mu.Lock()
		for name, expr := range want {
			m.audioFilters[name] = expr
		}
		m.mu.Unlock()

		if err := m.SetEqualizer(false, [10]int{}); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("SetEqualizer(false): want ErrNotConnected, got %v", err)
		}

		assertAudioFilters(t, m, want)
	})
}
