package library

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TestWatchLoop_EmitsDebugForRawEvents verifies that watchLoop emits a
// slog.Debug record for every raw fsnotify event received.
func TestWatchLoop_EmitsDebugForRawEvents(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	origLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	root := t.TempDir()
	const debounce = 150 * time.Millisecond

	p := filepath.Join(root, "track.mp3")
	mkfile(t, p)

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, _, err := Watch(ctx, []string{root}, nil, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	if err := os.WriteFile(p, []byte("ID3v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), debounce+400*time.Millisecond)
	defer drainCancel()
	collectTracks(drainCtx, ch)
	cancel()

	log := buf.String()
	if !strings.Contains(log, "fsnotify event") {
		t.Errorf("expected debug log containing 'fsnotify event'; got: %s", log)
	}
	if !strings.Contains(log, "op=") {
		t.Errorf("expected debug log with 'op' field; got: %s", log)
	}
	if !strings.Contains(log, filepath.Base(p)) {
		t.Errorf("expected debug log with path %q; got: %s", p, log)
	}
}

// TestWatch_WatcherFactoryFailure_LogsWarningAndDegrades verifies that when
// the fsnotify watcher cannot be created, Watch logs a warning and degrades.
func TestWatch_WatcherFactoryFailure_LogsWarningAndDegrades(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	origLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	origFactory := watcherFactory
	watcherFactory = func() (*fsnotify.Watcher, error) {
		return nil, errors.New("too many open files")
	}
	t.Cleanup(func() { watcherFactory = origFactory })

	ch, _, err := Watch(t.Context(), []string{t.TempDir()}, nil, &fakeExtractor{}, DefaultDebounce)
	if err != nil {
		t.Fatalf("Watch returned error; want nil for degraded mode: %v", err)
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected pre-closed channel in degraded mode, received a value")
		}
	case <-time.After(time.Second):
		t.Error("channel not closed within 1 s after watcher creation failure")
	}

	if !strings.Contains(buf.String(), "watcher") {
		t.Errorf("expected slog warning mentioning 'watcher'; got: %s", buf.String())
	}
}

// TestWatchLoop_LogsWatcherErrors verifies that watcher errors received from
// the fsnotify Errors channel are logged at Warn level instead of discarded.
func TestWatchLoop_LogsWatcherErrors(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	origLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	t.Cleanup(func() { slog.SetDefault(origLogger) })

	root := t.TempDir()
	const debounce = 100 * time.Millisecond

	fe := &fakeExtractor{}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	ch, _, err := Watch(ctx, []string{root}, nil, fe, debounce)
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	p := filepath.Join(root, "track.mp3")
	mkfile(t, p)
	if err := os.WriteFile(p, []byte("ID3v2"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	drainCtx, drainCancel := context.WithTimeout(context.Background(), debounce+300*time.Millisecond)
	defer drainCancel()
	collectTracks(drainCtx, ch)
	cancel()
}
