package tui

import (
	"context"
	"os/exec"
	"runtime"

	tea "charm.land/bubbletea/v2"
)

// openDir returns a tea.Cmd that opens the given directory in the OS file
// manager. On macOS it uses "open", on Linux it uses "xdg-open".
func openDir(ctx context.Context, dir string) tea.Cmd {
	return func() tea.Msg {
		name := "xdg-open"
		if runtime.GOOS == "darwin" {
			name = "open"
		}
		//nolint:gosec // dir comes from resolved XDG paths, not user input
		_ = exec.CommandContext(ctx, name, dir).Start()
		return nil
	}
}
