package tui

import (
	"context"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

type queueWriterReorderer struct {
	nextID int64
}

func (w *queueWriterReorderer) AppendTrack(_ library.Track, _ string) (int64, error) {
	w.nextID++
	return w.nextID, nil
}

func (w *queueWriterReorderer) ReorderQueueTrack(_ int64, _ int) error {
	return nil
}

func TestWithQueueWriterWiresReordererWhenSupported(t *testing.T) {
	w := &queueWriterReorderer{}
	m := New(context.Background(), WithQueueWriter(w))

	if m.queueWriter != w {
		t.Fatal("WithQueueWriter did not set queueWriter")
	}
	if m.reorderer != w {
		t.Fatal("WithQueueWriter should reuse writers that also implement queueReorderer")
	}
}

func TestWithQueueWriterDoesNotOverrideExplicitReorderer(t *testing.T) {
	w := &queueWriterReorderer{}
	r := &stubQueueReorderer{}
	m := New(context.Background(), WithQueueReorderer(r), WithQueueWriter(w))

	if m.queueWriter != w {
		t.Fatal("WithQueueWriter did not set queueWriter")
	}
	if m.reorderer != r {
		t.Fatal("WithQueueWriter should not override an explicitly configured reorderer")
	}
}
