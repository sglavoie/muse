package tui

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

// clipboardResultMsg is dispatched after a copy-to-clipboard operation.
type clipboardResultMsg struct {
	err  error
	text string
}

// copyToClipboard returns a tea.Cmd that writes text to the system clipboard.
// On macOS it pipes to pbcopy; on Linux/BSD it tries xclip, xsel, then wl-copy
// (Wayland). When no tool is available it returns an explanatory error rather
// than letting a nil *exec.Cmd panic.
func copyToClipboard(ctx context.Context, text string) tea.Cmd {
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
			// and silently fail to populate the Wayland clipboard.
			wayland := getEnv("WAYLAND_DISPLAY") != ""
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
			return clipboardResultMsg{err: errNoClipboardTool, text: text}
		}
		cmd.Stdin = bytes.NewBufferString(text)
		err := cmd.Run()
		return clipboardResultMsg{err: err, text: text}
	}
}

// lookPath is a thin wrapper kept as a var so tests can stub it without
// requiring the actual binaries to be installed on the test host.
var lookPath = func(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
