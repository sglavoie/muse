package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

// TestWatchStatusMsg_UnavailableMarksDegraded verifies that an asynchronous
// watcher-unavailable report (graceful fd-exhaustion fallback, Err == nil)
// marks the status bar degraded and notifies the user to use manual rescan.
func TestWatchStatusMsg_UnavailableMarksDegraded(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(library.WatchStatusMsg{Available: false})
	nm := next.(Model)

	if !nm.watchDegraded {
		t.Error("watchDegraded = false, want true after an unavailable watcher status")
	}
	if msg := nm.statusBar.TopNotification().Msg; !strings.Contains(msg, "unavailable") {
		t.Errorf("notification = %q, want it to mention the watcher is unavailable", msg)
	}
}

// TestWatchStatusMsg_ErrorMarksDegraded verifies that a watcher setup failure
// (Err != nil) is surfaced with a distinct "failed to start" notification.
func TestWatchStatusMsg_ErrorMarksDegraded(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(library.WatchStatusMsg{Available: false, Err: errors.New("boom")})
	nm := next.(Model)

	if !nm.watchDegraded {
		t.Error("watchDegraded = false, want true after a failed watcher status")
	}
	if msg := nm.statusBar.TopNotification().Msg; !strings.Contains(msg, "failed to start") {
		t.Errorf("notification = %q, want it to mention the watcher failed to start", msg)
	}
}

// TestWatchStatusMsg_AvailableLeavesHealthy verifies that a healthy watcher
// report neither degrades the status bar nor shows a notification.
func TestWatchStatusMsg_AvailableLeavesHealthy(t *testing.T) {
	m := New(context.Background())

	next, _ := m.Update(library.WatchStatusMsg{Available: true})
	nm := next.(Model)

	if nm.watchDegraded {
		t.Error("watchDegraded = true, want false after a healthy watcher status")
	}
	if msg := nm.statusBar.TopNotification().Msg; msg != "" {
		t.Errorf("notification = %q, want none for a healthy watcher", msg)
	}
}

// TestWaitForWatchStatus_DeliversSingleMessage verifies the Cmd blocks for the
// single status message and returns nil once the channel closes.
func TestWaitForWatchStatus_DeliversSingleMessage(t *testing.T) {
	ch := make(chan library.WatchStatusMsg, 1)
	ch <- library.WatchStatusMsg{Available: false, Err: errors.New("x")}

	cmd := waitForWatchStatus(ch)
	msg := cmd()
	status, ok := msg.(library.WatchStatusMsg)
	if !ok {
		t.Fatalf("expected library.WatchStatusMsg, got %T", msg)
	}
	if status.Available || status.Err == nil {
		t.Errorf("status = %+v, want unavailable with error", status)
	}

	// After close, the Cmd returns nil so it is not re-scheduled. A receive on a
	// closed channel returns immediately, so guard against a regression that
	// would block here with a deadline.
	close(ch)
	blank := make(chan tea.Msg, 1)
	go func() { blank <- waitForWatchStatus(ch)() }()
	select {
	case got := <-blank:
		if got != nil {
			t.Errorf("closed channel Cmd = %v, want nil", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waitForWatchStatus blocked on a closed channel")
	}
}
