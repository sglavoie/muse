package library

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
)

// TestHashWorkers_DrainAndClose verifies that HashWorkers drains every path
// from the input channel and closes the output channel on completion.
func TestHashWorkers_DrainAndClose(t *testing.T) {
	dir := t.TempDir()

	const n = 8
	paths := make([]string, n)
	for i := range paths {
		p := filepath.Join(dir, "track"+string(rune('0'+i))+".flac")
		writeFile(t, p, []byte("audio content for track"))
		paths[i] = p
	}

	in := make(chan string, n)
	for _, p := range paths {
		in <- p
	}
	close(in)

	var got []HashedFile
	for hf := range HashWorkers(in) {
		got = append(got, hf)
	}

	if len(got) != n {
		t.Fatalf("expected %d HashedFile values, got %d", n, len(got))
	}
	for _, hf := range got {
		if hf.Hash == "" {
			t.Errorf("empty hash for path %q", hf.Path)
		}
	}
}

// TestHashWorkers_SkipsErrors verifies that non-existent paths are silently
// skipped and do not block the pipeline or reduce valid results below the
// count of files that actually exist.
func TestHashWorkers_SkipsErrors(t *testing.T) {
	dir := t.TempDir()

	validPaths := []string{
		filepath.Join(dir, "a.flac"),
		filepath.Join(dir, "b.flac"),
	}
	for _, p := range validPaths {
		writeFile(t, p, []byte("real audio"))
	}

	in := make(chan string, 4)
	in <- filepath.Join(dir, "does_not_exist.flac")
	for _, p := range validPaths {
		in <- p
	}
	in <- filepath.Join(dir, "also_missing.flac")
	close(in)

	var got []HashedFile
	for hf := range HashWorkers(in) {
		got = append(got, hf)
	}

	if len(got) != len(validPaths) {
		t.Fatalf("expected %d results (valid files only), got %d", len(validPaths), len(got))
	}
}

// TestHashWorkers_ManyFiles verifies that HashWorkers correctly processes a
// larger batch of files concurrently and closes the output channel afterwards.
func TestHashWorkers_ManyFiles(t *testing.T) {
	dir := t.TempDir()

	const n = 50
	in := make(chan string, n)
	expected := make(map[string]struct{}, n)
	for i := range n {
		p := filepath.Join(dir, "track_many_"+string(rune('A'+i%26))+string(rune('0'+i/26))+".flac")
		writeFile(t, p, makeBytes(1024+i, byte(i)))
		in <- p
		expected[p] = struct{}{}
	}
	close(in)

	seen := make(map[string]struct{}, n)
	for hf := range HashWorkers(in) {
		if hf.Hash == "" {
			t.Errorf("empty hash for %q", hf.Path)
		}
		seen[hf.Path] = struct{}{}
	}

	if len(seen) != n {
		t.Fatalf("expected %d results, got %d", n, len(seen))
	}
	for p := range expected {
		if _, ok := seen[p]; !ok {
			t.Errorf("path %q missing from HashWorkers output", p)
		}
	}
}

// TestHashWorkers_OutputClosedOnEmptyInput verifies that HashWorkers closes
// the output channel immediately when the input channel is empty and closed.
func TestHashWorkers_OutputClosedOnEmptyInput(t *testing.T) {
	in := make(chan string)
	close(in)

	count := 0
	for range HashWorkers(in) {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 results for empty input, got %d", count)
	}
}

// TestHashWorkers_WorkerCountEqualsNumCPU verifies that hashWorkersN launches
// exactly n goroutines running concurrently. Each injected hashFn signals a
// ready channel before blocking on a gate channel. The test drains the ready
// channel n times (ensuring all n workers are simultaneously at the gate),
// then closes the gate to release them. An atomic peak counter tracks the
// maximum observed concurrency.
func TestHashWorkers_WorkerCountEqualsNumCPU(t *testing.T) {
	n := runtime.NumCPU()

	// ready is signalled by each worker once it has incremented the active
	// counter and is about to block. This lets the test know all n workers
	// are concurrently in-flight before releasing the gate.
	ready := make(chan struct{}, n)
	// gate blocks each worker until all are ready.
	gate := make(chan struct{})

	var active atomic.Int64
	var peak atomic.Int64

	hashFn := func(_ string) (string, error) {
		cur := active.Add(1)
		// Update peak via a compare-and-swap loop.
		for {
			old := peak.Load()
			if cur <= old {
				break
			}
			if peak.CompareAndSwap(old, cur) {
				break
			}
		}
		ready <- struct{}{} // signal: this worker is at the gate
		<-gate              // wait for release
		active.Add(-1)
		return "fakehash", nil
	}

	// Feed exactly n paths so every worker gets exactly one job.
	in := make(chan string, n)
	for i := range n {
		in <- fmt.Sprintf("fakepath_%d", i)
	}
	close(in)

	out := hashWorkersN(n, in, hashFn)

	// Wait until all n workers have signalled ready (all are at the gate).
	for range n {
		<-ready
	}
	// Release all workers simultaneously.
	close(gate)

	// Drain output to let all goroutines finish.
	for range out {
	}

	if got := int(peak.Load()); got != n {
		t.Fatalf("peak concurrent workers = %d, want %d (runtime.NumCPU())", got, n)
	}
}