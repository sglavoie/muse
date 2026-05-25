package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

// TestDurationAuditBatchUpdatesIndex verifies that a DurationAuditBatchMsg
// updates the library index and matching queue entries.
func TestDurationAuditBatchUpdatesIndex(t *testing.T) {
	m := newDurationAuditModel()
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 200.0})
	m.queue.entries = []QueueEntry{{ID: 1, Hash: "h1", Path: "/a.mp3", DurationMs: 120000}, {ID: 2, Hash: "h2", Path: "/b.mp3", DurationMs: 200000}}

	nm, _ := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if got := requireIndexTrack(t, nm, "h1").Duration; got != 240.0 {
		t.Errorf("library index duration for h1: got %f, want 240.0", got)
	}
	if got := nm.queue.entries[0].DurationMs; got != 240000 {
		t.Errorf("queue entry h1 DurationMs: got %d, want 240000", got)
	}
	if got := nm.queue.entries[1].DurationMs; got != 200000 {
		t.Errorf("queue entry h2 DurationMs: got %d, want 200000 (unmodified)", got)
	}
}

func TestDurationAuditBatchChainsNextRead(t *testing.T) {
	m, _ := newDurationAuditModelWithChannel()
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})

	_, cmd := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if cmd == nil {
		t.Error("expected non-nil cmd to chain next audit batch read, got nil")
	}
}

func TestDurationAuditBatchNilChannelReturnsNilCmd(t *testing.T) {
	m := newDurationAuditModel()
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})

	_, cmd := durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 240000})
	if cmd != nil {
		t.Error("expected nil cmd when auditCh is nil, got non-nil")
	}
}

func TestDurationAuditDoneShowsNotification(t *testing.T) {
	m, _ := newDurationAuditModelWithChannel()
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 120.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 200.0})
	m.library.index.Insert(library.Track{Hash: "h3", Path: "/c.mp3", Duration: 300.0})

	m, _ = durationAuditBatchUpdate(t, m,
		library.DurationFix{Hash: "h1", DurationMs: 240000},
		library.DurationFix{Hash: "h2", DurationMs: 250000},
		library.DurationFix{Hash: "h3", DurationMs: 350000},
	)
	nm := durationAuditDoneUpdate(t, m)

	if nm.statusBar.topNotification().msg == "" {
		t.Error("expected a non-empty notification after audit with corrections, got empty")
	}
	if nm.auditCh != nil {
		t.Error("expected auditCh to be nil after DurationAuditDoneMsg")
	}
	if nm.auditCorrected != 0 {
		t.Errorf("expected auditCorrected to be reset to 0, got %d", nm.auditCorrected)
	}
}

func TestDurationAuditCorrectedAccumulatesAcrossBatches(t *testing.T) {
	m, _ := newDurationAuditModelWithChannel()
	m.library.index.Insert(library.Track{Hash: "h1", Path: "/a.mp3", Duration: 100.0})
	m.library.index.Insert(library.Track{Hash: "h2", Path: "/b.mp3", Duration: 200.0})
	m.library.index.Insert(library.Track{Hash: "h3", Path: "/c.mp3", Duration: 300.0})

	m, _ = durationAuditBatchUpdate(t, m, library.DurationFix{Hash: "h1", DurationMs: 150000})
	if m.auditCorrected != 1 {
		t.Fatalf("after batch 1: auditCorrected = %d, want 1", m.auditCorrected)
	}
	m, _ = durationAuditBatchUpdate(t, m,
		library.DurationFix{Hash: "h2", DurationMs: 250000},
		library.DurationFix{Hash: "h3", DurationMs: 350000},
	)
	if m.auditCorrected != 3 {
		t.Fatalf("after batch 2: auditCorrected = %d, want 3", m.auditCorrected)
	}
	if nm := durationAuditDoneUpdate(t, m); nm.statusBar.topNotification().msg == "" {
		t.Error("expected notification after 3 corrections")
	} else if nm.auditCorrected != 0 {
		t.Errorf("auditCorrected should be reset after done, got %d", nm.auditCorrected)
	}
}

func TestDurationAuditDoneSilentWhenNone(t *testing.T) {
	m, _ := newDurationAuditModelWithChannel()
	nm := durationAuditDoneUpdate(t, m)

	if nm.statusBar.topNotification().msg != "" {
		t.Errorf("expected no notification when corrected=0, got %q", nm.statusBar.topNotification().msg)
	}
	if nm.auditCh != nil {
		t.Error("expected auditCh to be nil after DurationAuditDoneMsg")
	}
}

func TestDurationAuditNilChannelSkipsInit(t *testing.T) {
	if m := newDurationAuditModel(); m.auditCh != nil {
		t.Error("expected auditCh to be nil when no audit channel option is provided")
	}
}
