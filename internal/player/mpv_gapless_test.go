package player

import (
	"context"
	"sync"
	"testing"
)

func TestManager_SetGapless_Enabled(t *testing.T) {
	sock := shortSockPath(t, "gap-on")

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

	if err := m.SetGapless(true); err != nil {
		t.Fatalf("SetGapless(true): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "gapless-audio") {
					if args.stringIs(2, "yes") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property gapless-audio yes not sent; got %v", received)
	}
}

func TestManager_SetGapless_Disabled(t *testing.T) {
	sock := shortSockPath(t, "gap-off")

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

	if err := m.SetGapless(false); err != nil {
		t.Fatalf("SetGapless(false): %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "gapless-audio") {
					if args.stringIs(2, "no") {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property gapless-audio no not sent; got %v", received)
	}
}

func TestManager_SetGapless_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}}
	defer m.Close()

	if err := m.SetGapless(true); err != nil {
		t.Fatalf("SetGapless queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}
