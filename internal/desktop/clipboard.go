// Package desktop provides OS-level desktop integration: opening a directory
// or revealing a file in the system file manager, and copying text to the
// system clipboard. It is independent of the TUI and imports no internal/tui
// package.
package desktop

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// getEnv is a var so tests can stub session-detection without mutating the
// real process environment.
var getEnv = os.Getenv

// errNoClipboardTool is returned when no supported clipboard utility is
// available on the host. Callers surface this as a notification rather than a
// panic so the TUI keeps running on systems without pbcopy/xclip/xsel/wl-copy.
var errNoClipboardTool = errors.New("no clipboard utility found (install xclip, xsel, or wl-copy)")

// ClipboardResultMsg is dispatched after a copy-to-clipboard operation.
type ClipboardResultMsg struct {
	Err  error
	Text string
}

// CopyToClipboard returns a tea.Cmd that writes text to the system clipboard.
// On macOS it pipes to pbcopy; on Linux/BSD it tries xclip, xsel, then wl-copy
// (Wayland). When no tool is available it returns an explanatory error rather
// than letting a nil *exec.Cmd panic.
func CopyToClipboard(ctx context.Context, text string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "darwin":
			cmd = exec.CommandContext(ctx, "pbcopy")
		case "windows":
			cmd = exec.CommandContext(ctx, "clip")
		default:
			// Prefer wl-copy when running under Wayland: xclip/xsel may be
			// installed for XWayland compat but they target the X selection
			// and silently fail to populate the Wayland clipboard. Some
			// display managers (e.g. GDM, sddm) expose XDG_SESSION_TYPE to
			// the user shell but do not propagate WAYLAND_DISPLAY, so honor
			// either signal.
			wayland := getEnv("WAYLAND_DISPLAY") != "" ||
				getEnv("XDG_SESSION_TYPE") == "wayland"
			switch {
			case wayland && lookPath("wl-copy"):
				cmd = exec.CommandContext(ctx, "wl-copy")
			case lookPath("xclip"):
				cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
			case lookPath("xsel"):
				cmd = exec.CommandContext(ctx, "xsel", "--clipboard", "--input")
			case lookPath("wl-copy"):
				cmd = exec.CommandContext(ctx, "wl-copy")
			}
		}
		if cmd == nil {
			return ClipboardResultMsg{Err: errNoClipboardTool, Text: text}
		}
		var stderr bytes.Buffer
		cmd.Stdin = bytes.NewBufferString(text)
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return ClipboardResultMsg{Err: toolError(cmd.Args[0], err, stderr.String()), Text: text}
		}
		return ClipboardResultMsg{Text: text}
	}
}

// toolError names the desktop tool that failed and appends the first line of
// its stderr, so the status bar shows something actionable ("xclip: exit
// status 1: Error: Can't open display") instead of a bare "exit status 1".
func toolError(name string, err error, stderr string) error {
	msg, _, _ := strings.Cut(strings.TrimSpace(stderr), "\n")
	if msg = strings.TrimSpace(msg); msg != "" {
		return fmt.Errorf("%s: %w: %s", name, err, msg)
	}
	return fmt.Errorf("%s: %w", name, err)
}

// lookPath is a thin wrapper kept as a var so tests can stub it without
// requiring the actual binaries to be installed on the test host.
var lookPath = func(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
