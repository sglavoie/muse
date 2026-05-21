package player

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestManager_SetStereoWide_Enabled(t *testing.T) {
	sock := shortSockPath(t, "sw-on")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.SetStereoWide(true, 20); err != nil {
		t.Fatalf("SetStereoWide(true, 20): %v", err)
	}

	wantAF := "lavfi=[extrastereo=m=2.0]"

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if verb, _ := args[0].(string); verb == "set_property" {
				if prop, _ := args[1].(string); prop == "af" {
					if val, _ := args[2].(string); val == wantAF {
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

func TestManager_SetStereoWide_Disabled(t *testing.T) {
	sock := shortSockPath(t, "sw-off")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	defer m.Close()

	// Pre-populate the filter so disabling actually removes it.
	m.audioFilters["stereowide"] = "extrastereo=m=2.0"

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.mu.Unlock()

	if err := m.SetStereoWide(false, 0); err != nil {
		t.Fatalf("SetStereoWide(false): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if verb, _ := args[0].(string); verb == "set_property" {
				if prop, _ := args[1].(string); prop == "af" {
					if val, _ := args[2].(string); val == "" {
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

func TestManager_SetStereoWide_BeforeStartLeavesFiltersUnchanged(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		m := NewManager(shortSockPath(t, "sw-pre-on"))
		defer m.Close()

		m.mu.Lock()
		m.audioFilters["loudnorm"] = "loudnorm"
		m.mu.Unlock()

		if err := m.SetStereoWide(true, 15); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("SetStereoWide(true, 15): want ErrNotConnected, got %v", err)
		}

		assertAudioFilters(t, m, map[string]string{"loudnorm": "loudnorm"})
	})

	t.Run("disable", func(t *testing.T) {
		m := NewManager(shortSockPath(t, "sw-pre-off"))
		defer m.Close()

		want := map[string]string{
			"loudnorm":   "loudnorm",
			"stereowide": "extrastereo=m=1.5",
		}

		m.mu.Lock()
		for name, expr := range want {
			m.audioFilters[name] = expr
		}
		m.mu.Unlock()

		if err := m.SetStereoWide(false, 0); !errors.Is(err, ErrNotConnected) {
			t.Fatalf("SetStereoWide(false): want ErrNotConnected, got %v", err)
		}

		assertAudioFilters(t, m, want)
	})
}

func TestManager_FilterChain_StereoWideWithOthers(t *testing.T) {
	sock := shortSockPath(t, "sw-chain")

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
	m := &Manager{socketPath: sock, ctx: ctx, cancel: cancel, audioFilters: make(map[string]string)}
	defer m.Close()

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	m.mu.Lock()
	m.client = client
	m.audioFilters["loudnorm"] = "loudnorm"
	m.mu.Unlock()

	if err := m.SetStereoWide(true, 15); err != nil {
		t.Fatalf("SetStereoWide(true, 15): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if verb, _ := args[0].(string); verb == "set_property" {
				if prop, _ := args[1].(string); prop == "af" {
					val, _ := args[2].(string)
					if val == "lavfi=[loudnorm,extrastereo=m=1.5]" || val == "lavfi=[extrastereo=m=1.5,loudnorm]" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("expected af containing both loudnorm and extrastereo=m=1.5 not sent; got %v", received)
	}
}
