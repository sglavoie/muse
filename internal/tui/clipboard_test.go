package tui

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

// TestCopyToClipboardNoToolReturnsError verifies that when no clipboard tool
// is available on the host, copyToClipboard returns a clipboardResultMsg
// carrying errNoClipboardTool rather than panicking on a nil *exec.Cmd.
// Stubs lookPath to simulate an environment with no tools installed. Only
// meaningful on non-darwin/non-windows hosts (those branches hardcode their
// tool); on macOS we still verify the cmd is invokable without panic.
func TestCopyToClipboardNoToolReturnsError(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("darwin/windows hardcode pbcopy/clip; lookPath stub does not affect them")
	}
	orig := lookPath
	defer func() { lookPath = orig }()
	lookPath = func(string) bool { return false }

	cmd := copyToClipboard(context.Background(), "hello")
	if cmd == nil {
		t.Fatal("copyToClipboard returned nil cmd")
	}
	msg := cmd()
	res, ok := msg.(clipboardResultMsg)
	if !ok {
		t.Fatalf("expected clipboardResultMsg, got %T", msg)
	}
	if !errors.Is(res.err, errNoClipboardTool) {
		t.Errorf("expected errNoClipboardTool, got %v", res.err)
	}
	if res.text != "hello" {
		t.Errorf("expected text 'hello', got %q", res.text)
	}
}

// TestCopyToClipboardAlwaysReturnsClipboardResultMsg ensures that, regardless
// of host OS and tool availability, invoking the returned cmd produces a
// clipboardResultMsg rather than panicking. The earlier implementation could
// panic on a nil *exec.Cmd when running on unsupported platforms.
func TestCopyToClipboardAlwaysReturnsClipboardResultMsg(t *testing.T) {
	cmd := copyToClipboard(context.Background(), "x")
	if cmd == nil {
		t.Fatal("copyToClipboard returned nil cmd")
	}
	// Invocation should not panic and must return clipboardResultMsg.
	msg := cmd()
	res, ok := msg.(clipboardResultMsg)
	if !ok {
		t.Fatalf("expected clipboardResultMsg, got %T", msg)
	}
	if res.text != "x" {
		t.Errorf("expected text 'x', got %q", res.text)
	}
}
