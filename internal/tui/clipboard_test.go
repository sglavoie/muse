package tui

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

// TestCopyToClipboardPrefersWaylandWhenAvailable verifies that on a Wayland
// session (WAYLAND_DISPLAY set) wl-copy is preferred over xclip/xsel, since
// xclip targets the X selection and silently fails on Wayland-only sessions.
func TestCopyToClipboardPrefersWaylandWhenAvailable(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("Linux/BSD-only behavior")
	}
	origLook, origEnv := lookPath, getEnv
	defer func() { lookPath, getEnv = origLook, origEnv }()

	getEnv = func(key string) string {
		if key == "WAYLAND_DISPLAY" {
			return "wayland-0"
		}
		return ""
	}
	var order []string
	lookPath = func(name string) bool {
		order = append(order, name)
		// Pretend both xclip and wl-copy exist on the host.
		return name == "xclip" || name == "wl-copy"
	}

	_ = copyToClipboard(context.Background(), "hi")()
	if len(order) == 0 || order[0] != "wl-copy" {
		t.Fatalf("expected wl-copy to be checked first under Wayland, got %v", order)
	}
}

// TestCopyToClipboardDetectsWaylandViaSessionType verifies the fix for an
// adjacent bug: some display managers (GDM/sddm) expose XDG_SESSION_TYPE=wayland
// to the user shell but do not propagate WAYLAND_DISPLAY, so relying solely on
// WAYLAND_DISPLAY would incorrectly prefer xclip/xsel and silently fail to
// populate the Wayland clipboard.
func TestCopyToClipboardDetectsWaylandViaSessionType(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("Linux/BSD-only behavior")
	}
	origLook, origEnv := lookPath, getEnv
	defer func() { lookPath, getEnv = origLook, origEnv }()

	getEnv = func(key string) string {
		if key == "XDG_SESSION_TYPE" {
			return "wayland"
		}
		return ""
	}
	var order []string
	lookPath = func(name string) bool {
		order = append(order, name)
		return name == "xclip" || name == "wl-copy"
	}

	_ = copyToClipboard(context.Background(), "hi")()
	if len(order) == 0 || order[0] != "wl-copy" {
		t.Fatalf("expected wl-copy to be checked first when XDG_SESSION_TYPE=wayland, got %v", order)
	}
}

// TestCopyToClipboardFallsBackToXclipWithoutWayland verifies that with no
// WAYLAND_DISPLAY set the X11 tools are tried first (xclip before wl-copy)
// so installed but-non-running wl-copy daemons don't intercept copies.
func TestCopyToClipboardFallsBackToXclipWithoutWayland(t *testing.T) {
	if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
		t.Skip("Linux/BSD-only behavior")
	}
	origLook, origEnv := lookPath, getEnv
	defer func() { lookPath, getEnv = origLook, origEnv }()

	getEnv = func(string) string { return "" }
	var order []string
	lookPath = func(name string) bool {
		order = append(order, name)
		return name == "xclip" || name == "wl-copy"
	}

	_ = copyToClipboard(context.Background(), "hi")()
	if len(order) == 0 || order[0] != "xclip" {
		t.Fatalf("expected xclip first without Wayland, got %v", order)
	}
}

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
