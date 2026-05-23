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
	{"volume-up", "Increase volume by 5%"},
	{"volume-down", "Decrease volume by 5%"},
}

type unixSocketListener struct {
	net.Listener
	path string
}

func (l *unixSocketListener) Close() error {
	closeErr := l.Listener.Close()
	removeErr := os.Remove(l.path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	return errors.Join(closeErr, removeErr)
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
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return nil, fmt.Errorf("create runtime dir: %w", err)
	}

	sockPath := filepath.Join(runtimeDir, ctrlSocketName)

	// Remove any stale socket left by a previous crash.
	if err := removeStaleControlSocket(sockPath); err != nil {
		return nil, fmt.Errorf("remove stale control socket: %w", err)
	}

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		return nil, fmt.Errorf("listen on control socket: %w", err)
	}
	// Restrict the socket to the owner so a permissive umask cannot let
	// other users on a multi-user host dispatch playback commands.
	if err := os.Chmod(sockPath, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(sockPath)
		return nil, fmt.Errorf("restrict control socket permissions: %w", err)
	}
	ln = &unixSocketListener{Listener: ln, path: sockPath}

	go acceptLoop(p, ln)

	return ln, nil
}

func removeStaleControlSocket(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("%s exists and is not a socket", path)
	}
	return os.Remove(path)
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
				// Close the listener so the socket file is removed and future
				// `muse <action>` dials fail fast with a clear error instead of
				// connecting to a dead listener and hanging until timeout.
				if closeErr := ln.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
					slog.Warn("control socket: close after circuit breaker", "err", closeErr)
				}
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
// sender knows the command was received. The read deadline is refreshed after
// each successful command so a connection that pauses briefly between commands
// is not cut off by the original connection-wide timeout — the deadline acts
// as a per-command idle timeout instead.
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
			// Refresh the idle deadline so a stream of empty lines does not
			// burn through the timeout window without legitimate work.
			if err := conn.SetReadDeadline(time.Now().Add(remoteReadTimeout)); err != nil {
				slog.Warn("remote: refresh read deadline", "err", err)
				return
			}
			continue
		}
		if !isKnownAction(action) {
			slog.Warn("unknown remote action", "action", action)
			if err := conn.SetWriteDeadline(time.Now().Add(remoteReadTimeout)); err == nil {
				_, _ = fmt.Fprintf(conn, "error: unknown action %q\n", action) // best-effort
			}
			// Push the read deadline forward so the rejection of one bad
			// command does not invalidate subsequent valid commands on the
			// same connection.
			if err := conn.SetReadDeadline(time.Now().Add(remoteReadTimeout)); err != nil {
				slog.Warn("remote: refresh read deadline", "err", err)
				return
			}
			continue
		}
		p.Send(tui.RemoteActionMsg{ActionName: action})
		if err := conn.SetWriteDeadline(time.Now().Add(remoteReadTimeout)); err == nil {
			_, _ = fmt.Fprintln(conn, "ok") // best-effort acknowledgment
		}
		// Refresh the read deadline after each successful command so a
		// long-lived client can keep streaming commands without the original
		// connection-wide timeout cutting it off.
		if err := conn.SetReadDeadline(time.Now().Add(remoteReadTimeout)); err != nil {
			slog.Warn("remote: refresh read deadline", "err", err)
			return
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
	if !isKnownAction(action) {
		return fmt.Errorf("unknown action %q", action)
	}

	runtimeDir := config.ResolveRuntimeDir()
	sockPath := filepath.Join(runtimeDir, ctrlSocketName)

	conn, err := net.DialTimeout("unix", sockPath, remoteDialTimeout)
	if err != nil {
		return fmt.Errorf("connect to muse: %w (is muse running?)", err)
	}
	defer conn.Close()

	if err := conn.SetWriteDeadline(time.Now().Add(remoteReadTimeout)); err != nil {
		return fmt.Errorf("prepare command write: %w", err)
	}
	if _, err := fmt.Fprintln(conn, action); err != nil {
		return fmt.Errorf("send command: %w", err)
	}

	// Read acknowledgment from the running instance so callers fail fast if the
	// listener closes early or never confirms receipt.
	if err := conn.SetReadDeadline(time.Now().Add(remoteReadTimeout)); err != nil {
		return fmt.Errorf("prepare acknowledgment read: %w", err)
	}
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return fmt.Errorf("read acknowledgment: %w", err)
		}
		return errors.New("remote: no acknowledgment received")
	}
	if resp := strings.TrimSpace(scanner.Text()); resp != "ok" {
		return fmt.Errorf("remote: %s", resp)
	}
	return nil
}
