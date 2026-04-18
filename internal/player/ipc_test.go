package player

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestSendCommand_LoadFile(t *testing.T) {
	c := newTestClient(t)
	resp, err := c.SendCommand("loadfile", "/tmp/track.flac", "replace")
	if err != nil {
		t.Fatalf("loadfile: unexpected error: %v", err)
	}
	if resp.Error != "success" {
		t.Fatalf("loadfile: want error=success, got %q", resp.Error)
	}
}

func TestSendCommand_CyclePause(t *testing.T) {
	c := newTestClient(t)
	resp, err := c.SendCommand("cycle", "pause")
	if err != nil {
		t.Fatalf("cycle pause: unexpected error: %v", err)
	}
	if resp.Error != "success" {
		t.Fatalf("cycle pause: want error=success, got %q", resp.Error)
	}
}

func TestSendCommand_Seek(t *testing.T) {
	c := newTestClient(t)

	for _, delta := range []float64{5, -5, 30, -30} {
		resp, err := c.SendCommand("seek", delta, "relative")
		if err != nil {
			t.Fatalf("seek %v: unexpected error: %v", delta, err)
		}
		if resp.Error != "success" {
			t.Fatalf("seek %v: want error=success, got %q", delta, resp.Error)
		}
	}
}

func TestSendCommand_AddVolume(t *testing.T) {
	c := newTestClient(t)

	for _, delta := range []float64{5, -5} {
		resp, err := c.SendCommand("add", "volume", delta)
		if err != nil {
			t.Fatalf("add volume %v: unexpected error: %v", delta, err)
		}
		if resp.Error != "success" {
			t.Fatalf("add volume %v: want error=success, got %q", delta, resp.Error)
		}
	}
}

func TestSendCommand_RequestIDRoundTrip(t *testing.T) {
	c := newTestClient(t)

	// Fire several commands and verify each gets back a valid (success) response.
	commands := [][]any{
		{"loadfile", "/a.flac", "replace"},
		{"cycle", "pause"},
		{"seek", 5.0, "relative"},
		{"add", "volume", 5.0},
	}
	for _, cmd := range commands {
		resp, err := c.SendCommand(cmd...)
		if err != nil {
			t.Errorf("command %v: %v", cmd, err)
			continue
		}
		if resp.Error != "success" {
			t.Errorf("command %v: want success, got %q", cmd, resp.Error)
		}
	}
}

func TestSendCommand_MPVErrorReturnsErrPlaybackFailed(t *testing.T) {
	sock := shortSockPath(t, "mpv-err")
	fakeServerWithHandler(t, sock, errorHandler("invalid parameter"))

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	defer client.Close()

	_, err = client.SendCommand("seek", 999.0, "absolute")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrPlaybackFailed) {
		t.Fatalf("expected ErrPlaybackFailed, got %v", err)
	}
}

func TestSendCommand_DebugLogsIPCWireMessages(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	c := newTestClient(t)
	_, err := c.SendCommand("loadfile", "/tmp/track.flac", "replace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "ipc send") {
		t.Errorf("expected 'ipc send' in debug log, got:\n%s", out)
	}
	if !strings.Contains(out, "ipc recv") {
		t.Errorf("expected 'ipc recv' in debug log, got:\n%s", out)
	}
}

func TestSendCommand_ReturnsTimeoutWhenMPVHangs(t *testing.T) {
	// Use a very short timeout so the test doesn't take 5 seconds.
	prev := ipcReadTimeout
	ipcReadTimeout = 100 * time.Millisecond
	t.Cleanup(func() { ipcReadTimeout = prev })

	sock := shortSockPath(t, "mpv-hang")
	silentServer(t, sock)

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}
	defer client.Close()

	_, err = client.SendCommand("loadfile", "/tmp/track.flac", "replace")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestSendCommand_ClosedConnectionReturnsErrIPCTransport(t *testing.T) {
	c := newTestClient(t)
	c.Close() // marks closed=true and closes the underlying connection

	_, err := c.SendCommand("loadfile", "/tmp/track.flac", "replace")
	if err == nil {
		t.Fatal("expected error for closed connection, got nil")
	}
	if !errors.Is(err, ErrIPCTransport) {
		t.Fatalf("expected ErrIPCTransport, got %v", err)
	}
}

func TestClose_UnblocksSendCommand(t *testing.T) {
	// Use a long read timeout so the test proves Close() unblocks the
	// reader rather than the read deadline expiring first.
	prev := ipcReadTimeout
	ipcReadTimeout = 10 * time.Second
	t.Cleanup(func() { ipcReadTimeout = prev })

	sock := shortSockPath(t, "mpv-close")
	silentServer(t, sock) // accepts but never writes, so SendCommand blocks

	client, err := NewIPCClient(sock)
	if err != nil {
		t.Fatalf("NewIPCClient: %v", err)
	}

	// Start SendCommand in background; it will block in ReadBytes because
	// the server never sends a response.
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		_, _ = client.SendCommand("get_property", "volume")
	}()

	// Give SendCommand time to enter the read loop.
	time.Sleep(50 * time.Millisecond)

	// Close must return quickly by closing the connection first.
	closeDone := make(chan error, 1)
	go func() {
		closeDone <- client.Close()
	}()

	select {
	case <-closeDone:
		// Success: Close returned promptly.
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Close() blocked for >500ms; expected it to return immediately")
	}

	// SendCommand must also unblock once the connection is closed.
	select {
	case <-sendDone:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SendCommand did not unblock after Close()")
	}
}

func TestSendCommand_InfoLevelSuppressesIPCWireMessages(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	prev := slog.Default()
	slog.SetDefault(logger)
	t.Cleanup(func() { slog.SetDefault(prev) })

	c := newTestClient(t)
	_, err := c.SendCommand("loadfile", "/tmp/track.flac", "replace")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "ipc send") {
		t.Errorf("expected 'ipc send' to be suppressed at Info level, got:\n%s", out)
	}
	if strings.Contains(out, "ipc recv") {
		t.Errorf("expected 'ipc recv' to be suppressed at Info level, got:\n%s", out)
	}
}
