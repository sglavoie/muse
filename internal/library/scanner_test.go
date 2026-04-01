package library

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// make10kDir creates n minimal audio files in a temp directory and returns the
// directory path.
func make10kDir(tb testing.TB, n int) string {
	tb.Helper()
	dir := tb.TempDir()
	for i := range n {
		path := filepath.Join(dir, fmt.Sprintf("track_%05d.mp3", i))
		if err := os.WriteFile(path, []byte("ID3"), 0o644); err != nil {
			tb.Fatal(err)
		}
	}
	return dir
}

// TestScan10kWallTime verifies that scanning a synthetic 10,000-file library
// with a zero-cost extractor completes in under 30 seconds, confirming the
// pipeline goroutine orchestration does not introduce sequential bottlenecks.
func TestScan10kWallTime(t *testing.T) {
	const n = 10_000
	dir := make10kDir(t, n)

	start := time.Now()
	var count int
	for range Scan(context.Background(), []string{dir}, nil, &fakeExtractor{}) {
		count++
	}
	elapsed := time.Since(start)

	if count != n {
		t.Fatalf("expected %d tracks, got %d", n, count)
	}
	if elapsed > 30*time.Second {
		t.Fatalf("scan took %v, want < 30s", elapsed)
	}
}

// TestScanGoroutineLeak confirms that all pipeline goroutines exit once the
// Track channel returned by Scan is closed (scan-complete signal).
func TestScanGoroutineLeak(t *testing.T) {
	dir := t.TempDir()
	for i := range 20 {
		path := filepath.Join(dir, fmt.Sprintf("track_%02d.flac", i))
		if err := os.WriteFile(path, []byte("fLaC"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	// Establish a goroutine baseline before the scan.
	baseline := runtime.NumGoroutine()

	// Drain the scan channel; its close is the scan-complete signal.
	for range Scan(context.Background(), []string{dir}, nil, &fakeExtractor{}) {
	}

	// Allow any goroutine that closed a channel to finish its own exit.
	time.Sleep(200 * time.Millisecond)

	after := runtime.NumGoroutine()
	// Allow up to 2 extra goroutines for incidental Go runtime activity.
	if after > baseline+2 {
		t.Fatalf("goroutine leak: baseline=%d after scan=%d", baseline, after)
	}
}

// TestScanLogging_DebugPerTrack verifies that Scan emits a slog.Debug record
// with msg="track hashed" for every file in the directory.
func TestScanLogging_DebugPerTrack(t *testing.T) {
	const n = 5
	dir := make10kDir(t, n)

	h := &capturingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	for range Scan(context.Background(), []string{dir}, nil, &fakeExtractor{}) {
	}

	var debugCount int
	for _, r := range h.records {
		if r.Level == slog.LevelDebug && r.Message == "track hashed" {
			debugCount++
		}
	}
	if debugCount != n {
		t.Errorf("expected %d slog.Debug(\"track hashed\") records, got %d", n, debugCount)
	}
}

// TestScanLogging_InfoBatchProgress verifies that Scan emits at least one
// slog.Info record with msg="scan batch complete" and a positive "tracks" count.
func TestScanLogging_InfoBatchProgress(t *testing.T) {
	const n = 5
	dir := make10kDir(t, n)

	h := &capturingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	defer slog.SetDefault(prev)

	for range Scan(context.Background(), []string{dir}, nil, &fakeExtractor{}) {
	}

	found := false
	for _, r := range h.records {
		if r.Level == slog.LevelInfo && r.Message == "scan batch complete" {
			r.Attrs(func(a slog.Attr) bool {
				if a.Key == "tracks" && a.Value.Int64() > 0 {
					found = true
				}
				return true
			})
		}
	}
	if !found {
		t.Error("expected at least one slog.Info(\"scan batch complete\") record with tracks > 0")
	}
}

// BenchmarkScan exercises the pipeline end-to-end with a fake extractor so
// that goroutine orchestration and channel throughput are measured in
// isolation from exiftool invocation time.
func BenchmarkScan(b *testing.B) {
	const n = 10_000
	dir := make10kDir(b, n)

	b.ResetTimer()
	for b.Loop() {
		var count int
		for range Scan(context.Background(), []string{dir}, nil, &fakeExtractor{}) {
			count++
		}
		if count != n {
			b.Fatalf("expected %d tracks, got %d", n, count)
		}
	}
}
