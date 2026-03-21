package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/tui"
)

// startSignalListener listens for SIGUSR1 and dispatches remote playback
// actions into the TUI program. The action name is read from
// $runtimeDir/muse.cmd (written by runRemoteAction); a missing file (bare
// kill -USR1) defaults to play/pause for backward compatibility.
func startSignalListener(p *tea.Program, runtimeDir string) {
	sigusr1 := make(chan os.Signal, 1)
	signal.Notify(sigusr1, syscall.SIGUSR1)
	go func() {
		cmdPath := filepath.Join(runtimeDir, "muse.cmd")
		for range sigusr1 {
			actionName := ""
			if data, err := os.ReadFile(cmdPath); err == nil {
				actionName = strings.TrimSpace(string(data))
				if err := os.Remove(cmdPath); err != nil {
					slog.Warn("remove command file failed", "path", cmdPath, "err", err)
				}
			}
			p.Send(tui.RemoteActionMsg{ActionName: actionName})
		}
	}()
}

// runRemoteAction writes the action name to $RUNTIME/muse.cmd and sends
// SIGUSR1 to the running muse process. The TUI's signal handler reads the
// file and dispatches the action through handlePlaybackKeyAction, keeping
// all playback logic (including Go-side queue indexing and TUI state sync)
// in one place.
func runRemoteAction(action string) error {
	runtimeDir := config.ResolveRuntimeDir()
	cmdPath := filepath.Join(runtimeDir, "muse.cmd")
	pidPath := filepath.Join(runtimeDir, "muse.pid")

	// Atomic write: write to a temp file then rename.
	tmpPath := cmdPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(action+"\n"), 0o600); err != nil {
		return fmt.Errorf("write command file: %w", err)
	}
	if err := os.Rename(tmpPath, cmdPath); err != nil {
		if rmErr := os.Remove(tmpPath); rmErr != nil {
			slog.Warn("remove temp command file failed", "path", tmpPath, "err", rmErr)
		}
		return fmt.Errorf("rename command file: %w", err)
	}

	// Signal muse TUI to read the command file and dispatch the action.
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("read PID file: %w", err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("parse PID: %w", err)
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process: %w", err)
	}
	if err := proc.Signal(syscall.SIGUSR1); err != nil {
		return fmt.Errorf("send SIGUSR1: %w", err)
	}
	return nil
}
