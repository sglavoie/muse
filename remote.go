package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui"
)

const (
	ctrlSocketName           = "muse.ctrl"
	remoteDialTimeout        = 2 * time.Second
	remoteReadTimeout        = 5 * time.Second
	acceptBackoff            = 100 * time.Millisecond
	maxConsecutiveAcceptErrs = 1000
)

// remoteAction describes a single control-socket command.
type remoteAction struct {
	Name string
	Desc string
}

// remoteActions is the authoritative list of remote action names accepted by
// the control socket. It is also used by main to validate subcommand arguments
// and to generate help text, so adding a new action here is the only change
// required.
var remoteActions = []remoteAction{
	{"toggle-play", "Toggle play/pause on the running instance"},
	{"next", "Skip to the next track"},
	{"previous", "Skip to the previous track"},
	{"volume-up", "Increase volume"},
	{"volume-down", "Decrease volume"},
}

// isKnownAction reports whether name is a recognised remote action.
func isKnownAction(name string) bool {
	for _, a := range remoteActions {
		if a.Name == name {
			return true
		}
	}
	return false
}

// startCommandListener creates a Unix domain socket at
// $runtimeDir/muse.ctrl and dispatches incoming action names into the TUI
// program. Each connection may send one newline-delimited action per line;
// concurrent connections are handled safely because p.Send is goroutine-safe.
// Returns the listener so the caller can Close it during shutdown.
func startCommandListener(p *tea.Program, runtimeDir string) (net.Listener, error) {
	sockPath := filepath.Join(runtimeDir, ctrlSocketName)

	// Remove any stale socket left by a previous crash.
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale control socket: %w", err)
	}

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listen on control socket: %w", err)
	}

	go acceptLoop(p, ln)

	return ln, nil
}

// acceptLoop runs the accept loop for a command listener. It exits cleanly
// when ln is closed (net.ErrClosed) and continues on transient errors.
// Logging is throttled after 5 consecutive failures to avoid flooding the log
// if the socket file is deleted while muse is running. After
// maxConsecutiveAcceptErrs consecutive failures the loop gives up and returns
// so the goroutine does not spin forever on a permanently broken socket.
func acceptLoop(p *tea.Program, ln net.Listener) {
	acceptLoopWithBackoff(p, ln, acceptBackoff)
}

func acceptLoopWithBackoff(p *tea.Program, ln net.Listener, backoff time.Duration) {
	consecutiveErrs := 0
	for {
		conn, err := ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			consecutiveErrs++
			if consecutiveErrs <= 5 || consecutiveErrs%100 == 0 {
				slog.Warn("control socket accept error",
					"err", err, "consecutive", consecutiveErrs)
			}
			if consecutiveErrs >= maxConsecutiveAcceptErrs {
				slog.Error("control socket: too many consecutive accept errors, disabling remote control",
					"consecutive", consecutiveErrs)
				return
			}
			if backoff > 0 {
				time.Sleep(backoff)
			}
			continue
		}
		consecutiveErrs = 0
		go handleRemoteConn(p, conn)
	}
}

// handleRemoteConn reads newline-delimited action names from a single
// connection, dispatches each one into the TUI, and writes "ok\n" back so the
// sender knows the command was received.
func handleRemoteConn(p *tea.Program, conn net.Conn) {
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(remoteReadTimeout)); err != nil {
		slog.Warn("remote: set read deadline", "err", err)
		return
	}
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		action := strings.TrimSpace(scanner.Text())
		if action == "" {
			continue
		}
		if !isKnownAction(action) {
			slog.Warn("unknown remote action", "action", action)
			if err := conn.SetWriteDeadline(time.Now().Add(remoteReadTimeout)); err == nil {
				_, _ = fmt.Fprintf(conn, "error: unknown action %q\n", action) // best-effort
			}
			continue
		}
		p.Send(tui.RemoteActionMsg{ActionName: action})
		if err := conn.SetWriteDeadline(time.Now().Add(remoteReadTimeout)); err == nil {
			_, _ = fmt.Fprintln(conn, "ok") // best-effort acknowledgment
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		slog.Warn("remote connection read error", "err", err)
	}
}

// runRemoteAction connects to the running muse instance's control socket,
// sends the action name, and reads the acknowledgment. The TUI's listener
// goroutine reads it and dispatches the action through handlePlaybackKeyAction,
// keeping all playback logic (including Go-side queue indexing and TUI state
// sync) in one place.
func runRemoteAction(action string) error {
	runtimeDir := config.ResolveRuntimeDir()
	sockPath := filepath.Join(runtimeDir, ctrlSocketName)

	conn, err := net.DialTimeout("unix", sockPath, remoteDialTimeout)
	if err != nil {
		return fmt.Errorf("connect to muse: %w (is muse running?)", err)
	}
	defer conn.Close()

	if _, err := fmt.Fprintln(conn, action); err != nil {
		return fmt.Errorf("send command: %w", err)
	}

	// Read acknowledgment from the running instance. Use the same dial timeout
	// so callers do not wait indefinitely if the server is stalled.
	if err := conn.SetReadDeadline(time.Now().Add(remoteDialTimeout)); err != nil {
		return nil // command was sent; ack read is best-effort
	}
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		if resp := scanner.Text(); resp != "ok" {
			return fmt.Errorf("remote: %s", resp)
		}
	}
	return nil
}
