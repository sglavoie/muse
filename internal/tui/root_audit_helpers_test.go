package tui

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
)

func newDurationAuditModel() Model {
	return New(context.Background())
}

func newDurationAuditModelWithChannel() (Model, chan library.DurationFix) {
	ch := make(chan library.DurationFix)
	return New(context.Background(), WithDurationAuditChannel(ch)), ch
}

func durationAuditBatchUpdate(t *testing.T, m Model, fixes ...library.DurationFix) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(library.DurationAuditBatchMsg{Fixes: fixes})
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return nm, cmd
}

func durationAuditDoneUpdate(t *testing.T, m Model) Model {
	t.Helper()
	next, _ := m.Update(library.DurationAuditDoneMsg{})
	nm, ok := next.(Model)
	if !ok {
		t.Fatalf("Update returned %T, want Model", next)
	}
	return nm
}

func requireIndexTrack(t *testing.T, m Model, hash string) library.Track {
	t.Helper()
	track, ok := m.library.index.FindByHash(hash)
	if !ok {
		t.Fatalf("%s not found in index", hash)
	}
	return track
}
