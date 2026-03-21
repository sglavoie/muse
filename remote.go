package main

import (
	"bufio"
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
	ctrlSocketName    = "muse.ctrl"
	remoteDialTimeout = 2 * time.Second
)

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

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				// Listener closed during shutdown.
				return
			}
			go handleRemoteConn(p, conn)
		}
	}()

	return ln, nil
}

// handleRemoteConn reads newline-delimited action names from a single
// connection and dispatches each one into the TUI.
func handleRemoteConn(p *tea.Program, conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		action := strings.TrimSpace(scanner.Text())
		if action == "" {
			continue
		}
		p.Send(tui.RemoteActionMsg{ActionName: action})
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		slog.Warn("remote connection read error", "err", err)
	}
}

// runRemoteAction connects to the running muse instance's control socket and
// sends the action name. The TUI's listener goroutine reads it and dispatches
// the action through handlePlaybackKeyAction, keeping all playback logic
// (including Go-side queue indexing and TUI state sync) in one place.
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
	return nil
}
