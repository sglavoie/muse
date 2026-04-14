package player

import (
	"context"
	"errors"
	"os/exec"
	"testing"
)

// newTestPreviewPlayer creates a PreviewPlayer backed by a fake mpv server.
func newTestPreviewPlayer(t *testing.T) *PreviewPlayer {
	t.Helper()
	sock := shortSockPath(t, "preview")
	fakeServer(t, sock)

	p := NewPreviewPlayer(context.Background(), sock)
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

func TestPreviewPlayer_TogglePause(t *testing.T) {
	sock := shortSockPath(t, "preview-toggle-pause")
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) != 2 || cmd[0] != "cycle" || cmd[1] != "pause" {
			t.Fatalf("unexpected command: %#v", cmd)
		}
		return successHandler(req)
	})

	p := NewPreviewPlayer(context.Background(), sock)
	p.launchFn = func(ctx context.Context, socketPath string) (*exec.Cmd, error) {
		return nil, nil
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	if err := p.TogglePause(); err != nil {
		t.Fatalf("TogglePause: %v", err)
	}
}

func TestPreviewPlayer_SeekRelative(t *testing.T) {
	sock := shortSockPath(t, "preview-seek")
	want := -12.5
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) != 3 || cmd[0] != "seek" || cmd[2] != "relative" {
			t.Fatalf("unexpected command: %#v", cmd)
		}
		got, ok := cmd[1].(float64)
		if !ok {
			t.Fatalf("seek delta type = %T, want float64", cmd[1])
		}
		if got != want {
			t.Fatalf("seek delta = %v, want %v", got, want)
		}
		return successHandler(req)
	})

	p := NewPreviewPlayer(context.Background(), sock)
	p.launchFn = func(ctx context.Context, socketPath string) (*exec.Cmd, error) {
		return nil, nil
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	if err := p.SeekRelative(want); err != nil {
		t.Fatalf("SeekRelative: %v", err)
	}
}

func TestPreviewPlayer_SetAudioFilter(t *testing.T) {
	sock := shortSockPath(t, "preview-af")
	fakeServerWithHandler(t, sock, func(req map[string]any) map[string]any {
		cmd, _ := req["command"].([]any)
		if len(cmd) != 3 || cmd[0] != "set_property" || cmd[1] != "af" || cmd[2] != "lavfi=[loudnorm]" {
			t.Fatalf("unexpected command: %#v", cmd)
		}
		return successHandler(req)
	})

	p := NewPreviewPlayer(context.Background(), sock)
	p.launchFn = func(ctx context.Context, socketPath string) (*exec.Cmd, error) {
		return nil, nil
	}
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { p.Close() })

	if err := p.SetAudioFilter("lavfi=[loudnorm]"); err != nil {
		t.Fatalf("SetAudioFilter: %v", err)
	}
}

func TestPreviewPlayer_SetAudioFilter_NotConnected(t *testing.T) {
	p := NewPreviewPlayer(context.Background(), "/tmp/nonexistent.sock")
	err := p.SetAudioFilter("lavfi=[loudnorm]")
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

func TestPreviewPlayer_LoadAndPlay_NotConnected(t *testing.T) {
	p := NewPreviewPlayer(context.Background(), "/tmp/nonexistent.sock")
	err := p.LoadAndPlay("/tmp/test.mp3")
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

func TestPreviewPlayer_TogglePause_NotConnected(t *testing.T) {
	p := NewPreviewPlayer(context.Background(), "/tmp/nonexistent.sock")
	err := p.TogglePause()
	if !errors.Is(err, ErrNotConnected) {
		t.Fatalf("expected ErrNotConnected, got: %v", err)
	}
}

func TestPreviewPlayer_SeekRelative_NotConnected(t *testing.T) {
	p := NewPreviewPlayer(context.Background(), "/tmp/nonexistent.sock")
	err := p.SeekRelative(5)
	if !errors.Is(err, ErrNotConnected) {
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

	p := NewPreviewPlayer(context.Background(), sock)
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
	p := NewPreviewPlayer(context.Background(), "/tmp/nonexistent.sock")
	_, _, err := p.ElapsedPosition()
	if !errors.Is(err, ErrNotConnected) {
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
