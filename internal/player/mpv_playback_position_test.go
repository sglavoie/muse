package player

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestManager_SetCurrentPosition(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	m.SetCurrentPosition("/music/track.flac", 99.5)

	m.mu.RLock()
	gotPath := m.lastTrackPath
	gotElapsed := m.lastElapsed
	m.mu.RUnlock()

	if gotPath != "/music/track.flac" {
		t.Errorf("lastTrackPath = %q, want /music/track.flac", gotPath)
	}
	if gotElapsed != 99.5 {
		t.Errorf("lastElapsed = %f, want 99.5", gotElapsed)
	}
}

func TestManager_ElapsedPosition_Connected(t *testing.T) {
	sock := shortSockPath(t, "elapsed")

	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		cmd := req.Command
		if len(cmd) >= 2 {
			prop, _ := cmd.stringAt(1)
			switch prop {
			case "time-pos":
				return successFloatResponse(req, 42.5)
			case "duration":
				return successFloatResponse(req, 180.0)
			}
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

	elapsed, total, err := m.ElapsedPosition()
	if err != nil {
		t.Fatalf("ElapsedPosition: %v", err)
	}
	if elapsed != 42.5 {
		t.Errorf("elapsed = %f, want 42.5", elapsed)
	}
	if total != 180.0 {
		t.Errorf("total = %f, want 180.0", total)
	}
}

func TestManager_ElapsedPosition_NullPropertiesReturnZero(t *testing.T) {
	sock := shortSockPath(t, "elapsed-null")

	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		cmd := req.Command
		if len(cmd) >= 2 {
			prop, _ := cmd.stringAt(1)
			switch prop {
			case propTimePos, propDuration:
				return successHandler(req)
			}
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

	elapsed, total, err := m.ElapsedPosition()
	if err != nil {
		t.Fatalf("ElapsedPosition: %v", err)
	}
	if elapsed != 0 {
		t.Errorf("elapsed = %f, want 0", elapsed)
	}
	if total != 0 {
		t.Errorf("total = %f, want 0", total)
	}
}

func TestManager_ElapsedPosition_InvalidPropertyType(t *testing.T) {
	sock := shortSockPath(t, "elapsed-invalid")

	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		cmd := req.Command
		if len(cmd) >= 2 {
			prop, _ := cmd.stringAt(1)
			switch prop {
			case propTimePos:
				return successStringResponse(req, "42.5")
			case propDuration:
				return successFloatResponse(req, 180.0)
			}
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

	_, _, err = m.ElapsedPosition()
	if err == nil {
		t.Fatal("ElapsedPosition: expected error for string time-pos, got nil")
	}
	if !strings.Contains(err.Error(), "expected float64 or nil") {
		t.Fatalf("ElapsedPosition: unexpected error: %v", err)
	}
}

func TestManager_ElapsedPosition_NotConnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	_, _, err := m.ElapsedPosition()
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("ElapsedPosition: want ErrNotConnected, got %v", err)
	}
}

func TestManager_SyncPosition_Connected(t *testing.T) {
	sock := shortSockPath(t, "syncpos")

	fakeServerWithHandler(t, sock, func(req ipcTestRequest) ipcTestResponse {
		cmd := req.Command
		if len(cmd) >= 2 {
			prop, _ := cmd.stringAt(1)
			switch prop {
			case "path":
				return successStringResponse(req, "/music/track.flac")
			case "time-pos":
				return successFloatResponse(req, 55.3)
			}
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

	if err := m.SyncPosition(); err != nil {
		t.Fatalf("SyncPosition: %v", err)
	}

	m.mu.RLock()
	path := m.lastTrackPath
	elapsed := m.lastElapsed
	m.mu.RUnlock()

	if path != "/music/track.flac" {
		t.Errorf("lastTrackPath = %q, want /music/track.flac", path)
	}
	if elapsed != 55.3 {
		t.Errorf("lastElapsed = %f, want 55.3", elapsed)
	}
}

func TestManager_SyncPosition_NotConnected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{ctx: ctx, cancel: cancel}
	defer m.Close()

	err := m.SyncPosition()
	if !errors.Is(err, ErrNotConnected) {
		t.Errorf("SyncPosition: want ErrNotConnected, got %v", err)
	}
}
