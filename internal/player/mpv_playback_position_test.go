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

	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) >= 2 {
			prop, _ := cmd[1].(string)
			switch prop {
			case "time-pos":
				return map[string]any{
					"error":      "success",
					"data":       42.5,
					"request_id": req["request_id"],
				}
			case "duration":
				return map[string]any{
					"error":      "success",
					"data":       180.0,
					"request_id": req["request_id"],
				}
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

	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) >= 2 {
			prop, _ := cmd[1].(string)
			switch prop {
			case propTimePos, propDuration:
				return map[string]any{
					"error":      "success",
					"data":       nil,
					"request_id": req["request_id"],
				}
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

	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) >= 2 {
			prop, _ := cmd[1].(string)
			switch prop {
			case propTimePos:
				return map[string]any{
					"error":      "success",
					"data":       "42.5",
					"request_id": req["request_id"],
				}
			case propDuration:
				return map[string]any{
					"error":      "success",
					"data":       180.0,
					"request_id": req["request_id"],
				}
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

	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) >= 2 {
			prop, _ := cmd[1].(string)
			switch prop {
			case "path":
				return map[string]any{
					"error":      "success",
					"data":       "/music/track.flac",
					"request_id": req["request_id"],
				}
			case "time-pos":
				return map[string]any{
					"error":      "success",
					"data":       55.3,
					"request_id": req["request_id"],
				}
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
