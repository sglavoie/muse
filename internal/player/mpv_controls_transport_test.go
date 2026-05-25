package player

import (
	"context"
	"sync"
	"testing"
)

func TestManager_Play_Connected(t *testing.T) {
	sock := shortSockPath(t, "play")

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

	if err := m.Play(); err != nil {
		t.Fatalf("Play: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "pause") {
					if val, ok := args.boolAt(2); ok && !val {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property pause=false not sent; got %v", received)
	}
}

func TestManager_Play_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}}
	defer m.Close()

	if err := m.Play(); err != nil {
		t.Fatalf("Play queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}

func TestManager_Pause_Connected(t *testing.T) {
	sock := shortSockPath(t, "pause")

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

	if err := m.Pause(); err != nil {
		t.Fatalf("Pause: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	found := false
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "pause") {
					if val, ok := args.boolAt(2); ok && val {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Errorf("set_property pause=true not sent; got %v", received)
	}
}

func TestManager_Pause_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}}
	defer m.Close()

	if err := m.Pause(); err != nil {
		t.Fatalf("Pause queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}

func TestManager_Stop_Connected(t *testing.T) {
	sock := shortSockPath(t, "stop")

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

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	var foundPause, foundSeek bool
	for _, args := range received {
		if len(args) >= 3 {
			if args.stringIs(0, "set_property") {
				if args.stringIs(1, "pause") {
					if val, ok := args.boolAt(2); ok && val {
						foundPause = true
					}
				}
			}
		}
		if len(args) >= 2 {
			if args.stringIs(0, "seek") {
				if pos, ok := args.float64At(1); ok && pos == 0 {
					foundSeek = true
				}
			}
		}
	}
	if !foundPause {
		t.Errorf("set_property pause=true not sent; got %v", received)
	}
	if !foundSeek {
		t.Errorf("seek 0 not sent; got %v", received)
	}
}

func TestManager_Stop_NotConnectedQueues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel, cmdQueue: []func() error{}}
	defer m.Close()

	if err := m.Stop(); err != nil {
		t.Fatalf("Stop queued: %v", err)
	}
	m.mu.RLock()
	n := len(m.cmdQueue)
	m.mu.RUnlock()
	if n != 1 {
		t.Errorf("cmdQueue len = %d, want 1", n)
	}
}
