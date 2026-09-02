package tui

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// TestPreviewTickUpdatesProgress verifies that a tickMsg updates the
// preview overlay's elapsed and total values.
func TestPreviewTickUpdatesProgress(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{elapsed: 45.5, total: 200.0}
	m.overlay.preview = &overlay.PreviewOverlay{Artist: "A", Title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.overlay.preview == nil {
		t.Fatal("expected preview to still be active after tick")
	}
	if nm.overlay.preview.Elapsed != 45.5 {
		t.Errorf("expected elapsed=45.5, got %v", nm.overlay.preview.Elapsed)
	}
	if nm.overlay.preview.Total != 200.0 {
		t.Errorf("expected total=200.0, got %v", nm.overlay.preview.Total)
	}
}

// TestPreviewTickAutoDismissOnError verifies that if ElapsedPosition
// returns an error during tick, the preview is auto-dismissed.
func TestPreviewTickAutoDismissOnError(t *testing.T) {
	m := New(context.Background())
	sp := &stubPreviewer{elapsedErr: errTest("eof")}
	m.overlay.preview = &overlay.PreviewOverlay{Artist: "A", Title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if nm.overlay.preview != nil {
		t.Fatal("expected preview to be auto-dismissed on ElapsedPosition error")
	}
}

// TestPreviewTickAutoFillsDurationWhenZero verifies that when a track has
// Duration=0 and the preview player reports a valid total, the duration is
// updated in the library index, queue entries, and the DB writer is called.
func TestPreviewTickAutoFillsDurationWhenZero(t *testing.T) {
	dw := &stubDurationWriter{}
	m := New(context.Background(), WithDurationWriter(dw))
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 0,
	})
	m.queue.Entries = []queue.Entry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 0},
	}
	sp := &stubPreviewer{elapsed: 10.0, total: 225.0}
	m.overlay.preview = &overlay.PreviewOverlay{Hash: "h1", Artist: "A", Title: "T"}
	m.overlay.previewPlayer = sp

	nm, cmd := m.handleTickMsg()

	if t2, ok := nm.library.Index.FindByHash("h1"); !ok || t2.Duration != 225.0 {
		t.Fatalf("expected library duration 225.0, got %v", t2.Duration)
	}
	if nm.queue.Entries[0].DurationMs != 225000 {
		t.Fatalf("expected queue DurationMs 225000, got %d", nm.queue.Entries[0].DurationMs)
	}
	if !nm.overlay.previewDurationFilled {
		t.Fatal("expected previewDurationFilled to be set")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd from tick with duration fill")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected BatchMsg from handleTickMsg")
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		msg := make(chan tea.Msg, 1)
		go func(fn tea.Cmd) { msg <- fn() }(c)
		select {
		case <-msg:
		case <-time.After(50 * time.Millisecond):
		}
	}
	if len(dw.calls) != 1 {
		t.Fatalf("expected 1 DB write call, got %d", len(dw.calls))
	}
	if dw.calls[0].hash != "h1" || dw.calls[0].durationMs != 225000 {
		t.Fatalf("unexpected DB write: %+v", dw.calls[0])
	}
}

// TestPreviewTickCorrectsWrongDuration verifies that when a track has a
// non-zero Duration that differs significantly from mpv's reported total,
// the duration is corrected in the library index, queue, and DB.
func TestPreviewTickCorrectsWrongDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	m := New(context.Background(), WithDurationWriter(dw))
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 180.0,
	})
	m.queue.Entries = []queue.Entry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 180000},
	}
	sp := &stubPreviewer{elapsed: 10.0, total: 225.0}
	m.overlay.preview = &overlay.PreviewOverlay{Hash: "h1", Artist: "A", Title: "T"}
	m.overlay.previewPlayer = sp

	nm, cmd := m.handleTickMsg()

	if t2, ok := nm.library.Index.FindByHash("h1"); !ok || t2.Duration != 225.0 {
		t.Fatalf("expected library duration 225.0, got %v", t2.Duration)
	}
	if nm.queue.Entries[0].DurationMs != 225000 {
		t.Fatalf("expected queue DurationMs 225000, got %d", nm.queue.Entries[0].DurationMs)
	}
	if !nm.overlay.previewDurationFilled {
		t.Fatal("expected previewDurationFilled to be set")
	}
	if cmd == nil {
		t.Fatal("expected non-nil cmd from tick with duration correction")
	}
	batch, ok := cmd().(tea.BatchMsg)
	if !ok {
		t.Fatal("expected BatchMsg from handleTickMsg")
	}
	for _, c := range batch {
		if c == nil {
			continue
		}
		msg := make(chan tea.Msg, 1)
		go func(fn tea.Cmd) { msg <- fn() }(c)
		select {
		case <-msg:
		case <-time.After(50 * time.Millisecond):
		}
	}
	if len(dw.calls) != 1 {
		t.Fatalf("expected 1 DB write call, got %d", len(dw.calls))
	}
	if dw.calls[0].hash != "h1" || dw.calls[0].durationMs != 225000 {
		t.Fatalf("unexpected DB write: %+v", dw.calls[0])
	}
}

// TestPreviewTickSkipsCloseEnoughDuration verifies that when the stored
// duration is within 2 seconds of the mpv-reported total, no correction occurs.
func TestPreviewTickSkipsCloseEnoughDuration(t *testing.T) {
	dw := &stubDurationWriter{}
	m := New(context.Background(), WithDurationWriter(dw))
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 180.5,
	})
	m.queue.Entries = []queue.Entry{
		{Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", DurationMs: 180500},
	}
	sp := &stubPreviewer{elapsed: 10.0, total: 180.0}
	m.overlay.preview = &overlay.PreviewOverlay{Hash: "h1", Artist: "A", Title: "T"}
	m.overlay.previewPlayer = sp

	next, _ := m.Update(tickMsg{})
	nm := next.(Model)

	if t2, ok := nm.library.Index.FindByHash("h1"); !ok || t2.Duration != 180.5 {
		t.Fatalf("expected library duration to stay 180.5, got %v", t2.Duration)
	}
	if nm.queue.Entries[0].DurationMs != 180500 {
		t.Fatalf("expected queue DurationMs to stay 180500, got %d", nm.queue.Entries[0].DurationMs)
	}
	if !nm.overlay.previewDurationFilled {
		t.Fatal("expected previewDurationFilled to be set even when no correction needed")
	}
	if len(dw.calls) != 0 {
		t.Fatalf("expected 0 DB write calls, got %d", len(dw.calls))
	}
}

// TestPreviewTickFlagPreventsRepeatedWrites verifies that once
// previewDurationFilled is true, subsequent ticks do not trigger more writes.
func TestPreviewTickFlagPreventsRepeatedWrites(t *testing.T) {
	dw := &stubDurationWriter{}
	m := New(context.Background(), WithDurationWriter(dw))
	m.library.Index.Insert(library.Track{
		Hash: "h1", Path: "/a.flac", Artist: "A", Title: "T", Duration: 0,
	})
	sp := &stubPreviewer{elapsed: 10.0, total: 225.0}
	m.overlay.preview = &overlay.PreviewOverlay{Hash: "h1", Artist: "A", Title: "T"}
	m.overlay.previewPlayer = sp
	m.overlay.previewDurationFilled = true

	_, _ = m.Update(tickMsg{})

	if len(dw.calls) != 0 {
		t.Fatalf("expected 0 DB write calls on second tick, got %d", len(dw.calls))
	}
}
