package desktop

import (
	"bytes"
	tea "charm.land/bubbletea/v2"
	"context"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestOpenDirReturnsNonNilCmd(t *testing.T) {
	cmd := OpenDir(context.Background(), t.TempDir())
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}

// TestOpenDirEmptyPathIsNoOp verifies the bug fix where OpenDir would dispatch
// the OS open command with an empty argument, which on macOS opens the current
// working directory and on Linux surfaces a confusing xdg-open failure.
func TestOpenDirEmptyPathIsNoOp(t *testing.T) {
	cmd := OpenDir(context.Background(), "")
	if cmd == nil {
		t.Fatal("expected non-nil cmd even for empty path")
	}
	// The cmd should return nil without panicking or invoking exec.
	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil msg for empty dir, got %#v", msg)
	}
}

func TestOpenDirWhitespacePathIsNoOp(t *testing.T) {
	cmd := OpenDir(context.Background(), "   \t  ")
	if cmd == nil {
		t.Fatal("expected non-nil cmd even for whitespace path")
	}
	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil msg for whitespace dir, got %#v", msg)
	}
}

// TestRevealFileEmptyPathIsNoOp mirrors TestOpenDirEmptyPathIsNoOp for the
// RevealFile helper: an empty path must not be forwarded to the OS.
func TestRevealFileEmptyPathIsNoOp(t *testing.T) {
	cmd := RevealFile(context.Background(), "")
	if cmd == nil {
		t.Fatal("expected non-nil cmd even for empty path")
	}
	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil msg for empty path, got %#v", msg)
	}
}

func TestRevealFileWhitespacePathIsNoOp(t *testing.T) {
	cmd := RevealFile(context.Background(), " \n\t ")
	if cmd == nil {
		t.Fatal("expected non-nil cmd even for whitespace path")
	}
	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil msg for whitespace path, got %#v", msg)
	}
}

// A cancelled app context should not create a failure notification at shutdown.
func TestOpenDirCancelledContextIsQuiet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := OpenDir(ctx, t.TempDir())
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("expected nil msg, got %#v", msg)
	}
}

// A cancelled reveal should stay quiet, just like opening a directory.
func TestRevealFileCancelledContextIsQuiet(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	cmd := RevealFile(ctx, "/tmp/does-not-exist")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	if msg := cmd(); msg != nil {
		t.Errorf("expected nil msg, got %#v", msg)
	}
}

func TestOpenDirCommand_SelectsCommandsByOS(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		dir      string
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{name: "empty", goos: "linux", dir: "", wantOK: false},
		{name: "whitespace", goos: "linux", dir: " \t ", wantOK: false},
		{name: "linux", goos: "linux", dir: "/music", wantName: "xdg-open", wantArgs: []string{"/music"}, wantOK: true},
		{name: "darwin", goos: "darwin", dir: "/music", wantName: "open", wantArgs: []string{"/music"}, wantOK: true},
		{name: "windows", goos: "windows", dir: `C:\Music`, wantName: "explorer", wantArgs: []string{`C:\Music`}, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotArgs, gotOK := openDirCommand(tt.goos, tt.dir)
			if gotOK != tt.wantOK {
				t.Fatalf("openDirCommand() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if !gotOK {
				return
			}
			if gotName != tt.wantName {
				t.Fatalf("openDirCommand() name = %q, want %q", gotName, tt.wantName)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Fatalf("openDirCommand() args = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}

func TestRevealFileCommand_SelectsCommandsByOS(t *testing.T) {
	tests := []struct {
		name     string
		goos     string
		path     string
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{name: "empty", goos: "linux", path: "", wantOK: false},
		{name: "whitespace", goos: "linux", path: "\n\t ", wantOK: false},
		{name: "linux", goos: "linux", path: "/music/song.flac", wantName: "xdg-open", wantArgs: []string{"/music"}, wantOK: true},
		{name: "darwin", goos: "darwin", path: "/music/song.flac", wantName: "open", wantArgs: []string{"-R", "/music/song.flac"}, wantOK: true},
		{name: "windows", goos: "windows", path: `C:\Music\song.flac`, wantName: "explorer", wantArgs: []string{`/select,C:\Music\song.flac`}, wantOK: true},
		{name: "windows with spaces", goos: "windows", path: `C:\My Music\song.flac`, wantName: "explorer", wantArgs: []string{`/select,C:\My Music\song.flac`}, wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotArgs, gotOK := revealFileCommand(tt.goos, tt.path)
			if gotOK != tt.wantOK {
				t.Fatalf("revealFileCommand() ok = %v, want %v", gotOK, tt.wantOK)
			}
			if !gotOK {
				return
			}
			if gotName != tt.wantName {
				t.Fatalf("revealFileCommand() name = %q, want %q", gotName, tt.wantName)
			}
			if !reflect.DeepEqual(gotArgs, tt.wantArgs) {
				t.Fatalf("revealFileCommand() args = %v, want %v", gotArgs, tt.wantArgs)
			}
		})
	}
}

// captureSlog routes slog output to a buffer for the rest of the test and
// restores the previous default logger, and the log package state that
// slog.SetDefault rewires, afterwards.
func captureSlog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	origSlog, origOut, origFlags := slog.Default(), log.Writer(), log.Flags()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	t.Cleanup(func() {
		slog.SetDefault(origSlog)
		log.SetOutput(origOut)
		log.SetFlags(origFlags)
	})
	return &buf
}

// TestOpenDirAndRevealFileLogStartFailure guards against a missing file
// manager (no xdg-open, say) failing silently: the Start error must at least
// reach the log.
func TestOpenDirAndRevealFileLogStartFailure(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	buf := captureSlog(t)

	dir := t.TempDir()
	for _, cmd := range []tea.Cmd{
		OpenDir(context.Background(), dir),
		RevealFile(context.Background(), filepath.Join(dir, "a.flac")),
	} {
		msg, ok := cmd().(FileManagerResultMsg)
		if !ok || msg.Err == nil || msg.Path == "" || msg.Action == "" {
			t.Errorf("expected actionable file-manager failure, got %#v", msg)
		}
	}

	logged := buf.String()
	for _, want := range []string{"open directory failed", "reveal file failed"} {
		if !strings.Contains(logged, want) {
			t.Errorf("log does not contain %q:\n%s", want, logged)
		}
	}
}

func TestFileManagerReportsExitFailure(t *testing.T) {
	for _, action := range []string{"open", "reveal"} {
		t.Run(action, func(t *testing.T) {
			binDir := t.TempDir()
			tool := "xdg-open"
			if runtime.GOOS == "darwin" {
				tool = "open"
			}
			// Exercise the public command with a real child that starts but fails.
			script := "#!/bin/sh\necho 'permission denied' >&2\necho 'extra detail' >&2\nexit 7\n"
			if err := os.WriteFile(filepath.Join(binDir, tool), []byte(script), 0o700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", binDir)
			path := filepath.Join(t.TempDir(), "music with spaces")
			cmd := OpenDir(context.Background(), path)
			if action == "reveal" {
				cmd = RevealFile(context.Background(), path)
			}
			msg, ok := cmd().(FileManagerResultMsg)
			if !ok || msg.Err == nil || msg.Path != path {
				t.Fatalf("expected exit failure for %q, got %#v", path, msg)
			}
			for _, want := range []string{tool, "exit status 7", "permission denied"} {
				if !strings.Contains(msg.Err.Error(), want) {
					t.Errorf("error %q missing %q", msg.Err, want)
				}
			}
			if strings.Contains(msg.Err.Error(), "extra detail") {
				t.Fatal("notification should use only the first stderr line")
			}
		})
	}
}

func TestFileManagerSuccessIsQuiet(t *testing.T) {
	if msg := runFileManager(context.Background(), "/bin/sh", []string{"-c", "exit 0"}, "Open directory", "/music"); msg != nil {
		t.Fatalf("successful open should not notify: %#v", msg)
	}
}
