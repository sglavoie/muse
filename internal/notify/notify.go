// Package notify posts desktop banner notifications through whichever backend
// the host platform provides (alerter on macOS, notify-send on Linux). It is
// shared by the `muse now-playing` CLI subcommand and the TUI's automatic
// on-track-change notification.
//
// Everything in this file is platform-neutral. The per-platform argv is built
// by backendCommand, implemented once per GOOS in notify_darwin.go,
// notify_linux.go and notify_other.go.
package notify

import (
	"fmt"
	"os/exec"
	"strings"
)

// Track is the resolved notification content for a single banner.
type Track struct {
	Title    string
	Artist   string
	Album    string
	Paused   bool
	Degraded string // non-empty when metadata could not be read from the library
}

// Message returns the notification body, which doubles as the state line.
func (t Track) Message() string {
	body := t.Degraded
	if body == "" {
		body = t.Album
	}
	if body == "" {
		body = "Now playing"
	}
	if t.Paused {
		return "Paused — " + body
	}
	return body
}

// SummaryLine is the single stdout line, so the command is also useful when
// run from a terminal.
func (t Track) SummaryLine() string {
	parts := []string{t.Title}
	if t.Artist != "" {
		parts = append(parts, t.Artist)
	}
	return strings.Join(parts, " — ") + " (" + t.Message() + ")"
}

// Send launches the platform notification backend fire-and-forget: Start then
// Release, never Wait, so the caller (Karabiner's shell_command, or a tea.Cmd
// goroutine) returns immediately. Stdin/Stdout/Stderr are left nil, which
// wires them to /dev/null.
func Send(t Track) error {
	bin, args, err := backendCommand(t)
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", bin, err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release %s: %w", bin, err)
	}
	return nil
}

// Available reports whether a notification backend can be resolved on this
// platform. The returned error carries per-platform install instructions and
// is advisory only: notifications are opt-in, so a missing backend must never
// block startup.
func Available() error {
	_, _, err := backendCommand(Track{Title: "muse"})
	return err
}
