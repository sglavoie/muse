// Package notify posts macOS banner notifications through the alerter binary.
// It is shared by the `muse now-playing` CLI subcommand and the TUI's
// automatic on-track-change notification.
package notify

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// alerterPath is the preferred absolute location of the alerter binary.
// Declared as a var so tests can point it at a stub.
var alerterPath = "/opt/homebrew/bin/alerter"

const (
	alerterGroup   = "com.sglavoie.muse.now-playing"
	alerterSender  = "com.apple.Terminal"
	alerterTimeout = "5"
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

// alerterArgs builds the argv passed to alerter. --group makes repeated
// notifications replace the existing banner instead of stacking, and --timeout
// auto-dismisses it (alerter otherwise blocks waiting for interaction).
func (t Track) alerterArgs() []string {
	args := []string{"--title", t.Title}
	// An empty --subtitle renders as a blank line, so omit the flag entirely.
	if t.Artist != "" {
		args = append(args, "--subtitle", t.Artist)
	}
	return append(args,
		"--message", t.Message(),
		"--group", alerterGroup,
		"--sender", alerterSender,
		"--timeout", alerterTimeout,
	)
}

// Send launches alerter fire-and-forget: Start then Release, never Wait, so
// the caller (Karabiner's shell_command, or a tea.Cmd goroutine) returns
// immediately. Stdin/Stdout/Stderr are left nil, which wires them to
// /dev/null.
func Send(t Track) error {
	bin, err := resolveAlerter()
	if err != nil {
		return err
	}
	cmd := exec.Command(bin, t.alerterArgs()...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start %s: %w", bin, err)
	}
	if err := cmd.Process.Release(); err != nil {
		return fmt.Errorf("release %s: %w", bin, err)
	}
	return nil
}

// resolveAlerter prefers the hardcoded absolute path because Karabiner's
// shell_command runs with a minimal PATH, falling back to a PATH lookup.
func resolveAlerter() (string, error) {
	if _, err := os.Stat(alerterPath); err == nil {
		return alerterPath, nil
	}
	bin, err := exec.LookPath("alerter")
	if err != nil {
		return "", fmt.Errorf("alerter not found at %s or on PATH", alerterPath)
	}
	return bin, nil
}
