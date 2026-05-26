package tui

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// openDir returns a tea.Cmd that opens the given directory in the OS file
// manager. On macOS it uses "open", on Linux it uses "xdg-open". An empty or
// whitespace-only directory path is rejected so the OS handler does not receive
// a "" argument
// (which on macOS opens the current working directory and on Linux may surface
// a confusing error dialog).
func openDir(ctx context.Context, dir string) tea.Cmd {
	return func() tea.Msg {
		name, args, ok := openDirCommand(runtime.GOOS, dir)
		if !ok {
			return nil
		}
		_ = exec.CommandContext(ctx, name, args...).Start()
		return nil
	}
}

// revealFile returns a tea.Cmd that reveals the given file in the OS file
// manager. On macOS it uses "open -R", on Linux it opens the parent directory
// with "xdg-open". An empty or whitespace-only path is rejected so the OS
// handler does not receive a "" argument.
func revealFile(ctx context.Context, path string) tea.Cmd {
	return func() tea.Msg {
		name, args, ok := revealFileCommand(runtime.GOOS, path)
		if !ok {
			return nil
		}
		_ = exec.CommandContext(ctx, name, args...).Start()
		return nil
	}
}

func openDirCommand(goos, dir string) (string, []string, bool) {
	if strings.TrimSpace(dir) == "" {
		return "", nil, false
	}
	switch goos {
	case "darwin":
		return "open", []string{dir}, true
	case "windows":
		return "explorer", []string{dir}, true
	default:
		return "xdg-open", []string{dir}, true
	}
}

func revealFileCommand(goos, path string) (string, []string, bool) {
	if strings.TrimSpace(path) == "" {
		return "", nil, false
	}
	switch goos {
	case "darwin":
		return "open", []string{"-R", path}, true
	case "windows":
		return "explorer", []string{"/select," + path}, true
	default:
		return "xdg-open", []string{filepath.Dir(path)}, true
	}
}
