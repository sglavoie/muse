package library

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// gatedExtractor is a MetadataExtractor that echoes each file as a Track and
// records the peak number of Extract calls in flight. Each call holds for
// delay so overlapping calls are observable.
type gatedExtractor struct {
	delay    time.Duration
	inFlight atomic.Int64
	peak     atomic.Int64
	panicOn  string

	mu    sync.Mutex
	calls int
}

func (g *gatedExtractor) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	g.mu.Lock()
	g.calls++
	g.mu.Unlock()
	n := g.inFlight.Add(1)
	defer g.inFlight.Add(-1)
	for {
		p := g.peak.Load()
		if n <= p || g.peak.CompareAndSwap(p, n) {
			break
		}
	}
	time.Sleep(g.delay)
	tracks := make([]Track, len(files))
	for i, f := range files {
		if f.Path == g.panicOn {
			panic("extractor blew up")
		}
		tracks[i] = Track{Path: f.Path, Hash: f.Hash}
	}
	return tracks, nil
}

func hashedInput(n int) <-chan HashedFile {
	in := make(chan HashedFile, n)
	for i := range n {
		in <- HashedFile{Path: fmt.Sprintf("/music/%04d.flac", i), Hash: fmt.Sprintf("h%04d", i)}
	}
	close(in)
	return in
}

// TestExtractBatchesN_RunsBatchesConcurrently verifies that several workers
// extract batches side by side and that every input is emitted exactly once.
func TestExtractBatchesN_RunsBatchesConcurrently(t *testing.T) {
	const n = 95
	ge := &gatedExtractor{delay: 50 * time.Millisecond}
	seen := make(map[string]int, n)
	for tr := range extractBatchesN(context.Background(), ge, hashedInput(n), 4, 10) {
		seen[tr.Path]++
	}
	if len(seen) != n {
		t.Fatalf("want %d distinct tracks, got %d", n, len(seen))
	}
	for path, count := range seen {
		if count != 1 {
			t.Errorf("track %s emitted %d times", path, count)
		}
	}
	if ge.calls != 10 {
		t.Errorf("want 10 batches of at most 10, got %d calls", ge.calls)
	}
	if peak := ge.peak.Load(); peak < 2 || peak > 4 {
		t.Errorf("peak concurrent Extract calls: want 2..4, got %d", peak)
	}
}

// TestExtractBatches_ConcurrentExtractorUsesSmallBatches verifies that an
// extractor advertising BatchConcurrency is driven in concurrentBatchSize
// batches rather than one exiftoolBatchSize batch.
func TestExtractBatches_ConcurrentExtractorUsesSmallBatches(t *testing.T) {
	ce := &concurrentFake{}
	count := 0
	for range ExtractBatches(context.Background(), ce, hashedInput(concurrentBatchSize*3)) {
		count++
	}
	if count != concurrentBatchSize*3 {
		t.Fatalf("want %d tracks, got %d", concurrentBatchSize*3, count)
	}
	if got := ce.calls.Load(); got != 3 {
		t.Errorf("want 3 batches, got %d", got)
	}
}

type concurrentFake struct{ calls atomic.Int64 }

func (c *concurrentFake) BatchConcurrency() int { return 4 }

func (c *concurrentFake) Extract(_ context.Context, files []HashedFile) ([]Track, error) {
	c.calls.Add(1)
	tracks := make([]Track, len(files))
	for i, f := range files {
		tracks[i] = Track{Path: f.Path, Hash: f.Hash}
	}
	return tracks, nil
}

// TestExtractBatchesN_WorkerPanicKeepsOtherBatches verifies that a panic in one
// batch loses only that batch: the remaining workers still extract the rest and
// the output channel still closes.
func TestExtractBatchesN_WorkerPanicKeepsOtherBatches(t *testing.T) {
	ge := &gatedExtractor{panicOn: "/music/0005.flac"}
	count := 0
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range extractBatchesN(context.Background(), ge, hashedInput(50), 3, 10) {
			count++
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("output never closed after a worker panic")
	}
	if count != 40 {
		t.Errorf("want the 40 tracks outside the panicking batch, got %d", count)
	}
}

// TestExtractBatchesN_AllWorkersPanicStillDrainsInput verifies that when every
// worker dies, the last one drains the batch channel so the batcher keeps
// consuming the input and nothing upstream is left blocked.
func TestExtractBatchesN_AllWorkersPanicStillDrainsInput(t *testing.T) {
	in := make(chan HashedFile)
	ge := &gatedExtractor{panicOn: "/music/x.flac"}
	out := extractBatchesN(context.Background(), ge, in, 2, 1)

	sent := make(chan struct{})
	go func() {
		defer close(sent)
		defer close(in)
		for range 100 {
			in <- HashedFile{Path: "/music/x.flac", Hash: "h"}
		}
	}()
	select {
	case <-sent:
	case <-time.After(5 * time.Second):
		t.Fatal("producer blocked after every extract worker panicked")
	}
	for range out {
	}
}
