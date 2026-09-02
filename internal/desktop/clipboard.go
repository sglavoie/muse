// Package desktop provides OS-level desktop integration: opening a directory
// or revealing a file in the system file manager, and copying text to the
// system clipboard. It is independent of the TUI and imports no internal/tui
// package.
package desktop

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"

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
		cmd.Stdin = bytes.NewBufferString(text)
		err := cmd.Run()
		return ClipboardResultMsg{Err: err, Text: text}
	}
}

// lookPath is a thin wrapper kept as a var so tests can stub it without
// requiring the actual binaries to be installed on the test host.
var lookPath = func(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
