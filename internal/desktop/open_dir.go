package desktop

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"
)

// OpenDir returns a tea.Cmd that opens the given directory in the OS file
// manager. On macOS it uses "open", on Linux it uses "xdg-open". An empty or
// whitespace-only directory path is rejected so the OS handler does not receive
// a "" argument
// (which on macOS opens the current working directory and on Linux may surface
// a confusing error dialog).
func OpenDir(ctx context.Context, dir string) tea.Cmd {
	return func() tea.Msg {
		name, args, ok := openDirCommand(runtime.GOOS, dir)
		if !ok {
			return nil
		}
		c := exec.CommandContext(ctx, name, args...)
		if err := c.Start(); err != nil {
			slog.Warn("open directory failed", "cmd", name, "dir", dir, "err", err)
			return nil
		}
		// Per os/exec docs, Wait must be called to release associated
		// system resources (FDs, zombie PID). The handler returns
		// quickly; reap it asynchronously so the TUI is not blocked.
		go func() { _ = c.Wait() }()
		return nil
	}
}

// RevealFile returns a tea.Cmd that reveals the given file in the OS file
// manager. On macOS it uses "open -R", on Linux it opens the parent directory
// with "xdg-open". An empty or whitespace-only path is rejected so the OS
// handler does not receive a "" argument.
func RevealFile(ctx context.Context, path string) tea.Cmd {
	return func() tea.Msg {
		name, args, ok := revealFileCommand(runtime.GOOS, path)
		if !ok {
			return nil
		}
		c := exec.CommandContext(ctx, name, args...)
		if err := c.Start(); err != nil {
			slog.Warn("reveal file failed", "cmd", name, "path", path, "err", err)
			return nil
		}
		// See OpenDir: reap the child so its FDs/PID are released
		// instead of lingering as a zombie until process exit.
		go func() { _ = c.Wait() }()
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
