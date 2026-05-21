package tui

import (
	"context"
	"testing"
)

func TestOpenDirReturnsNonNilCmd(t *testing.T) {
	cmd := openDir(context.Background(), "/tmp")
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
}

// TestOpenDirEmptyPathIsNoOp verifies the bug fix where openDir would dispatch
// the OS open command with an empty argument, which on macOS opens the current
// working directory and on Linux surfaces a confusing xdg-open failure.
func TestOpenDirEmptyPathIsNoOp(t *testing.T) {
	cmd := openDir(context.Background(), "")
	if cmd == nil {
		t.Fatal("expected non-nil cmd even for empty path")
	}
	// The cmd should return nil without panicking or invoking exec.
	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil msg for empty dir, got %#v", msg)
	}
}

// TestRevealFileEmptyPathIsNoOp mirrors TestOpenDirEmptyPathIsNoOp for the
// revealFile helper: an empty path must not be forwarded to the OS.
func TestRevealFileEmptyPathIsNoOp(t *testing.T) {
	cmd := revealFile(context.Background(), "")
	if cmd == nil {
		t.Fatal("expected non-nil cmd even for empty path")
	}
	msg := cmd()
	if msg != nil {
		t.Errorf("expected nil msg for empty path, got %#v", msg)
	}
}
