package db

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriter_ExecBlockedOnFullQueueReturnsAfterCancel(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	ctx, cancel := context.WithCancel(context.Background())
	w := NewWriter(ctx, d.DB)

	writerBusy := make(chan struct{})
	gate := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- w.Exec(func(*sql.DB) error {
			close(writerBusy)
			<-gate
			return nil
		})
	}()

	<-writerBusy
	defer func() {
		close(gate)
		if err := <-firstDone; err != nil {
			t.Errorf("in-flight Exec: %v", err)
		}
		w.Wait()
	}()

	for range writerChannelBuffer {
		w.cmds <- writeCmd{
			fn:   func(*sql.DB) error { return nil },
			resp: make(chan error, 1),
		}
	}

	blockedDone := make(chan error, 1)
	go func() {
		blockedDone <- w.Exec(func(*sql.DB) error { return nil })
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-blockedDone:
		if err == nil {
			t.Fatal("blocked Exec returned nil after cancellation; want stopped error")
		}
		if !strings.Contains(err.Error(), "stopped") {
			t.Fatalf("blocked Exec error = %q, want stopped error", err.Error())
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("blocked Exec did not return after cancellation")
	}
}
