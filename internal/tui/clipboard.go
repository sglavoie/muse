package tui

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// clipboardResultMsg is dispatched after a copy-to-clipboard operation.
type clipboardResultMsg struct {
	err  error
	text string
}

// copyToClipboard returns a tea.Cmd that writes text to the system clipboard.
// On macOS it pipes to pbcopy; on Linux it tries xclip then xsel.
func copyToClipboard(ctx context.Context, text string) tea.Cmd {
	return func() tea.Msg {
		var cmd *exec.Cmd
		if runtime.GOOS == "darwin" {
			cmd = exec.CommandContext(ctx, "pbcopy") //nolint:gosec
		} else {
			if _, err := exec.LookPath("xclip"); err == nil {
				cmd = exec.CommandContext(ctx, "xclip", "-selection", "clipboard") //nolint:gosec
			} else {
				cmd = exec.CommandContext(ctx, "xsel", "--clipboard", "--input") //nolint:gosec
			}
		}
		cmd.Stdin = bytes.NewBufferString(text)
		err := cmd.Run()
		return clipboardResultMsg{err: err, text: text}
	}
}
