package desktop

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

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
		return runFileManager(ctx, name, args, "Open directory", dir)
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
		return runFileManager(ctx, name, args, "Reveal file", path)
	}
}

// FileManagerResultMsg reports file-manager failures to the UI.
type FileManagerResultMsg struct {
	Action string
	Path   string
	Err    error
}

// Bubble Tea runs commands off the update loop. Waiting here reaps the child
// and reports exit failures as well as launch failures without blocking input.
func runFileManager(ctx context.Context, name string, args []string, action, path string) tea.Msg {
	cmd := exec.CommandContext(ctx, name, args...)
	// A launched desktop app may inherit stderr after the opener exits.
	cmd.WaitDelay = time.Second
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil || errors.Is(err, exec.ErrWaitDelay) {
			return nil // shutdown or inherited pipes are not opener failures
		}
		err = toolError(name, err, stderr.String())
		slog.Warn(strings.ToLower(action)+" failed", "path", path, "err", err)
		return FileManagerResultMsg{Action: action, Path: path, Err: err}
	}
	return nil
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
