package player

import (
	"context"
	"os/exec"
	"testing"
)

// newTestPreviewPlayer creates a PreviewPlayer backed by a fake mpv server.
func newTestPreviewPlayer(t *testing.T) *PreviewPlayer {
	t.Helper()
	sock := shortSockPath(t, "preview")
	fakeServer(t, sock)

	p := NewPreviewPlayer(sock)
	// Replace launchFn with a no-op since the fake server is already listening.
	p.launchFn = func(ctx context.Context, socketPath string) (*exec.Cmd, error) {
		return nil, nil
	}
	if err := p.Start(); err != nil {
		t.Fatalf("PreviewPlayer.Start: %v", err)
	}
	t.Cleanup(func() { p.Close() })
	return p
}

func TestPreviewPlayer_Start(t *testing.T) {
	p := newTestPreviewPlayer(t)
	if p.client == nil {
		t.Fatal("expected client to be connected after Start")
	}
}

func TestPreviewPlayer_LoadAndPlay(t *testing.T) {
	p := newTestPreviewPlayer(t)
	// The fake server returns success for any command.
	if err := p.LoadAndPlay("/tmp/test.mp3"); err != nil {
		t.Fatalf("LoadAndPlay: %v", err)
	}
}

func TestPreviewPlayer_LoadAndPlay_NotConnected(t *testing.T) {
	p := NewPreviewPlayer("/tmp/nonexistent.sock")
	err := p.LoadAndPlay("/tmp/test.mp3")
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

func TestPreviewPlayer_ElapsedPosition(t *testing.T) {
	sock := shortSockPath(t, "preview-elapsed")
	// Use a handler that returns float64 data for get_property requests.
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

	p := NewPreviewPlayer(sock)
	p.launchFn = func(ctx context.Context, socketPath string) (*exec.Cmd, error) {
		return nil, nil
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	elapsed, total, err := p.ElapsedPosition()
	if err != nil {
		t.Fatalf("ElapsedPosition: %v", err)
	}
	if elapsed != 42.5 {
		t.Errorf("elapsed = %v, want 42.5", elapsed)
	}
	if total != 180.0 {
		t.Errorf("total = %v, want 180.0", total)
	}
}

func TestPreviewPlayer_ElapsedPosition_NotConnected(t *testing.T) {
	p := NewPreviewPlayer("/tmp/nonexistent.sock")
	_, _, err := p.ElapsedPosition()
	if err != ErrNotConnected {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

func TestPreviewPlayer_Close(t *testing.T) {
	p := newTestPreviewPlayer(t)
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// After close, client should be nil.
	if p.client != nil {
		t.Error("expected client to be nil after Close")
	}
}
