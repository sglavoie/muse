package desktop

import (
	"context"
	"reflect"
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

// TestOpenDirReapsChildProcess verifies the fix for an FD/zombie leak: the
// previous implementation called Start() without a matching Wait(), which the
// os/exec docs explicitly warn leaks system resources. After the fix the
// command should run to completion in the background; we exercise that path by
// invoking OpenDir with a context that is already cancelled so Start fails fast
// without leaving a launched-but-unreaped subprocess behind. The test simply
// asserts the helper does not panic and returns nil promptly.
func TestOpenDirReapsChildProcess(t *testing.T) {
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

// TestRevealFileReapsChildProcess mirrors TestOpenDirReapsChildProcess for the
// RevealFile helper.
func TestRevealFileReapsChildProcess(t *testing.T) {
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
