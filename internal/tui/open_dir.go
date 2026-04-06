package tui

import (
	"context"
	"os/exec"
	"path/filepath"
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
		_ = exec.CommandContext(ctx, name, dir).Start()
		return nil
	}
}

// revealFile returns a tea.Cmd that reveals the given file in the OS file
// manager. On macOS it uses "open -R", on Linux it opens the parent directory
// with "xdg-open".
func revealFile(ctx context.Context, path string) tea.Cmd {
	return func() tea.Msg {
		if runtime.GOOS == "darwin" {
			_ = exec.CommandContext(ctx, "open", "-R", path).Start()
		} else {
			dir := filepath.Dir(path)
			_ = exec.CommandContext(ctx, "xdg-open", dir).Start()
		}
		return nil
	}
}
