package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestDurationAuditBatchUpdatesIndex verifies that a DurationAuditBatchMsg
// updates the library index and matching queue entries.
func TestDurationAuditBatchUpdatesIndex(t *testing.T) {
	m := New(context.Background())
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 200.0})
	m.queue.entries = []QueueEntry{
		{ID: 1, Hash: "h1", Path: "/a.mp3", DurationMs: 120000},
		{ID: 2, Hash: "h2", Path: "/b.mp3", DurationMs: 200000},
	}

	next, _ := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{
			{Hash: "h1", DurationMs: 240000},
		},
	})
	nm := next.(Model)

	// Library index should reflect the corrected duration.
	t1, ok := nm.library.index.FindByHash("h1")
	if !ok {
		t.Fatal("h1 not found in index")
	}
	if t1.Duration != 240.0 {
		t.Errorf("library index duration for h1: got %f, want 240.0", t1.Duration)
	}

	// Queue entry for h1 should be updated.
	if nm.queue.entries[0].DurationMs != 240000 {
		t.Errorf("queue entry h1 DurationMs: got %d, want 240000", nm.queue.entries[0].DurationMs)
	}

	// Queue entry for h2 should be unchanged.
	if nm.queue.entries[1].DurationMs != 200000 {
		t.Errorf("queue entry h2 DurationMs: got %d, want 200000 (unmodified)", nm.queue.entries[1].DurationMs)
	}
}

// TestDurationAuditBatchChainsNextRead verifies that handling a
// DurationAuditBatchMsg returns a cmd to read the next batch (non-nil cmd).
func TestDurationAuditBatchChainsNextRead(t *testing.T) {
	ch := make(chan library.DurationFix)
	m := New(context.Background(), WithDurationAuditChannel(ch))
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})

	_, cmd := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	if cmd == nil {
		t.Error("expected non-nil cmd to chain next audit batch read, got nil")
	}
}

// TestDurationAuditBatchNilChannelReturnsNilCmd verifies that if auditCh
// is nil when a batch arrives, no follow-up cmd is scheduled.
func TestDurationAuditBatchNilChannelReturnsNilCmd(t *testing.T) {
	m := New(context.Background()) // no audit channel
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})

	_, cmd := m.Update(library.DurationAuditBatchMsg{
		Fixes: []library.DurationFix{{Hash: "h1", DurationMs: 240000}},
	})
	if cmd != nil {
		t.Error("expected nil cmd when auditCh is nil, got non-nil")
	}
}

// TestDurationAuditDoneShowsNotification verifies that when corrected > 0 a
// status bar notification is set.
func TestDurationAuditDoneShowsNotification(t *testing.T) {
	ch := make(chan library.DurationFix)
	m := New(context.Background(), WithDurationAuditChannel(ch))

	next, _ := m.Update(library.DurationAuditDoneMsg{Checked: 10, Corrected: 3})
	nm := next.(Model)

	if nm.statusBar.notification.msg == "" {
		t.Error("expected a non-empty notification after audit with corrections, got empty")
	}
	if nm.auditCh != nil {
		t.Error("expected auditCh to be nil after DurationAuditDoneMsg")
	}
}

// TestDurationAuditDoneSilentWhenNone verifies that when corrected == 0 no
// notification is shown.
func TestDurationAuditDoneSilentWhenNone(t *testing.T) {
	ch := make(chan library.DurationFix)
	m := New(context.Background(), WithDurationAuditChannel(ch))

	next, _ := m.Update(library.DurationAuditDoneMsg{Checked: 5, Corrected: 0})
	nm := next.(Model)

	if nm.statusBar.notification.msg != "" {
		t.Errorf("expected no notification when corrected=0, got %q", nm.statusBar.notification.msg)
	}
	if nm.auditCh != nil {
		t.Error("expected auditCh to be nil after DurationAuditDoneMsg")
	}
}

// TestDurationAuditNilChannelSkipsInit verifies that a model without an audit
// channel leaves auditCh nil (Init will not schedule an audit cmd).
func TestDurationAuditNilChannelSkipsInit(t *testing.T) {
	m := New(context.Background()) // no WithDurationAuditChannel option
	if m.auditCh != nil {
		t.Error("expected auditCh to be nil when no audit channel option is provided")
	}
}
