package player

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

const (
	previewPollInterval    = 50 * time.Millisecond
	previewPollTimeout     = 5 * time.Second
	previewShutdownTimeout = 3 * time.Second
)

// Previewer controls a lightweight preview player that plays a single track
// without affecting the main playback queue.
type Previewer interface {
	Start() error
	LoadAndPlay(path string) error
	ElapsedPosition() (elapsed, total float64, err error)
	Close() error
}

// PreviewPlayer manages a second mpv subprocess dedicated to song previews.
// It reuses IPCClient for socket communication but has no queue, no event
// reader, and no health checks — it is intentionally fire-and-forget.
type PreviewPlayer struct {
	socketPath string
	cmd        *exec.Cmd
	client     *IPCClient
	ctx        context.Context
	cancel     context.CancelFunc

	// launchFn starts the mpv subprocess. Tests replace this with a no-op.
	launchFn func(ctx context.Context, socketPath string) (*exec.Cmd, error)
}

// Compile-time check that PreviewPlayer implements Previewer.
var _ Previewer = (*PreviewPlayer)(nil)

// NewPreviewPlayer creates a PreviewPlayer that will use the given socket path.
// parentCtx ties the player's lifecycle to the application: when the parent
// context is cancelled the preview subprocess is also terminated.
func NewPreviewPlayer(parentCtx context.Context, socketPath string) *PreviewPlayer {
	ctx, cancel := context.WithCancel(parentCtx)
	return &PreviewPlayer{
		socketPath: socketPath,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start launches the mpv subprocess in idle mode and blocks until the IPC
// socket is ready or a 5-second timeout elapses.
func (p *PreviewPlayer) Start() error {
	if err := os.MkdirAll(os.TempDir(), 0o700); err != nil {
		return fmt.Errorf("preview: create dir: %w", err)
	}

	launch := p.launchFn
	if launch == nil {
		launch = func(ctx context.Context, sock string) (*exec.Cmd, error) {
			args := append([]string{"--idle=yes", "--input-ipc-server=" + sock}, baseFlags...)
			cmd := exec.CommandContext(ctx, "mpv", args...)
			if err := cmd.Start(); err != nil {
				return nil, fmt.Errorf("preview mpv: start: %w", err)
			}
			return cmd, nil
		}
	}

	cmd, err := launch(p.ctx, p.socketPath)
	if err != nil {
		return err
	}
	p.cmd = cmd

	// Poll until the socket is ready.
	deadline := time.Now().Add(previewPollTimeout)
	for time.Now().Before(deadline) {
		select {
		case <-p.ctx.Done():
			return p.ctx.Err()
		default:
		}

		client, err := NewIPCClient(p.socketPath)
		if err == nil {
			p.client = client
			return nil
		}
		time.Sleep(previewPollInterval)
	}
	return fmt.Errorf("preview: timed out waiting for mpv socket at %s", p.socketPath)
}

// LoadAndPlay loads a file and starts playback immediately.
func (p *PreviewPlayer) LoadAndPlay(path string) error {
	if p.client == nil {
		return ErrNotConnected
	}
	_, err := p.client.SendCommand("loadfile", path, "replace", 0, "pause=no")
	return err
}

// ElapsedPosition queries the preview player for current position and duration.
func (p *PreviewPlayer) ElapsedPosition() (elapsed, total float64, err error) {
	if p.client == nil {
		return 0, 0, ErrNotConnected
	}

	posResp, err := p.client.SendCommand("get_property", "time-pos")
	if err != nil {
		return 0, 0, err
	}
	elapsed, ok := posResp.Data.(float64)
	if !ok {
		return 0, 0, fmt.Errorf("preview: time-pos: expected float64, got %T", posResp.Data)
	}

	durResp, err := p.client.SendCommand("get_property", "duration")
	if err != nil {
		return 0, 0, err
	}
	total, ok = durResp.Data.(float64)
	if !ok {
		return 0, 0, fmt.Errorf("preview: duration: expected float64, got %T", durResp.Data)
	}

	return elapsed, total, nil
}

// Close stops the preview mpv subprocess, closes the IPC connection, and
// removes the socket file.
func (p *PreviewPlayer) Close() error {
	p.cancel()

	if p.client != nil {
		_ = p.client.Close() // best-effort; subprocess is about to be killed
		p.client = nil
	}
	if p.cmd != nil && p.cmd.Process != nil {
		done := make(chan error, 1)
		go func() { done <- p.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(previewShutdownTimeout):
			slog.Warn("preview mpv process did not exit within timeout, killing")
			_ = p.cmd.Process.Kill()
		}
	}
	if err := os.Remove(p.socketPath); err != nil && !os.IsNotExist(err) {
		slog.Warn("preview: remove socket", "path", p.socketPath, "err", err)
	}
	return nil
}
