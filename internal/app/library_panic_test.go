package app

import (
	"context"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/library"
)

// TestRunLibraryPipeline_PanicReportsDegraded verifies that a panic inside the
// library pipeline is not laundered into a clean finish. Recovering used to
// close the done channel and nothing else, so LoadLibrary's waiters saw a
// normal completion while the TUI's scan, watch, error and status streams
// stayed open forever with no indication anything had failed.
func TestRunLibraryPipeline_PanicReportsDegraded(t *testing.T) {
	prev := watchFn
	t.Cleanup(func() { watchFn = prev })
	watchFn = func(context.Context, []string, []string, library.MetadataExtractor, time.Duration) (<-chan library.WatchEvent, bool, error) {
		panic("watcher registration blew up")
	}

	scanStream := make(chan library.Track, 1)
	watchStream := make(chan library.WatchEvent, 1)
	scanErrCh := make(chan library.ScanErrorMsg, 32)
	watchStatusCh := make(chan library.WatchStatusMsg, 1)
	done := make(chan struct{})

	go runLibraryPipeline(t.Context(), done, libraryPipeline{
		musicDirs:      []string{t.TempDir()},
		known:          map[string]library.KnownFile{},
		rawScanStream:  scanStream,
		rawWatchStream: watchStream,
		scanErrCh:      scanErrCh,
		watchStatusCh:  watchStatusCh,
	})

	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("pipeline never signalled completion after the panic")
	}

	// The failure is reported as a degraded watcher status...
	var status library.WatchStatusMsg
	select {
	case msg, ok := <-watchStatusCh:
		if !ok {
			t.Fatal("watch status channel closed without reporting the failure")
		}
		status = msg
	case <-time.After(10 * time.Second):
		t.Fatal("no watch status reported after a pipeline panic")
	}
	if status.Available {
		t.Errorf("watch status reported Available after a pipeline panic: %+v", status)
	}
	if status.Err == nil {
		t.Error("watch status carried no error after a pipeline panic")
	}
	if _, ok := <-watchStatusCh; ok {
		t.Error("expected watch status channel to be closed after its single message")
	}

	// ...and as an explicit scan error the TUI already surfaces.
	select {
	case scanErr, ok := <-scanErrCh:
		if !ok {
			t.Fatal("scan error channel closed without reporting the failure")
		}
		if scanErr.Err == nil {
			t.Error("scan error message carried no error")
		}
	default:
		t.Error("no scan error reported for the panicking pipeline")
	}

	// Every stream is closed, so no consumer is left hanging.
	assertClosed(t, "scan error", func() bool { _, ok := <-scanErrCh; return ok })
	assertClosed(t, "scan", func() bool { _, ok := <-scanStream; return ok })
	assertClosed(t, "watch", func() bool { _, ok := <-watchStream; return ok })
}

func assertClosed(t *testing.T, name string, recv func() bool) {
	t.Helper()
	if recv() {
		t.Errorf("%s stream delivered a value where a closed channel was expected", name)
	}
}
