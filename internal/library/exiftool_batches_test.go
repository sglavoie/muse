package library

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// TestExtractBatches_SingleBatch verifies that fewer than exiftoolBatchSize files are
// dispatched in one Extract call and all Tracks reach the output channel.
func TestExtractBatches_SingleBatch(t *testing.T) {
	const n = 10
	fe := &fakeExtractor{}

	in := make(chan HashedFile, n)
	for i := range n {
		in <- HashedFile{Path: fmt.Sprintf("/music/track%02d.flac", i), Hash: fmt.Sprintf("hash%02d", i)}
	}
	close(in)

	var got []Track
	for tr := range ExtractBatches(context.Background(), fe, in) {
		got = append(got, tr)
	}

	if len(got) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(got))
	}
	if len(fe.batches) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(fe.batches))
	}
}

// TestExtractBatches_FullBatch verifies that exactly exiftoolBatchSize files trigger a
// flush mid-stream, producing at least one batch of exiftoolBatchSize.
func TestExtractBatches_FullBatch(t *testing.T) {
	const n = exiftoolBatchSize + 3
	fe := &fakeExtractor{}

	in := make(chan HashedFile, n)
	for i := range n {
		in <- HashedFile{Path: fmt.Sprintf("/music/track%04d.flac", i), Hash: fmt.Sprintf("h%04d", i)}
	}
	close(in)

	var got []Track
	for tr := range ExtractBatches(context.Background(), fe, in) {
		got = append(got, tr)
	}

	if len(got) != n {
		t.Fatalf("expected %d tracks, got %d", n, len(got))
	}
	if len(fe.batches) < 2 {
		t.Fatalf("expected at least 2 batches for %d files, got %d", n, len(fe.batches))
	}
	if len(fe.batches[0]) != exiftoolBatchSize {
		t.Fatalf("first batch size: want %d, got %d", exiftoolBatchSize, len(fe.batches[0]))
	}
}

// TestExtractBatches_EmptyInput verifies that an already-closed empty channel
// produces no Tracks and closes the output channel without hanging.
func TestExtractBatches_EmptyInput(t *testing.T) {
	fe := &fakeExtractor{}
	in := make(chan HashedFile)
	close(in)

	count := 0
	for range ExtractBatches(context.Background(), fe, in) {
		count++
	}
	if count != 0 {
		t.Fatalf("expected 0 tracks for empty input, got %d", count)
	}
	if len(fe.batches) != 0 {
		t.Fatalf("expected no Extract calls for empty input, got %d", len(fe.batches))
	}
}

// TestExtractBatches_OutputClosedAfterDrain verifies the output channel is
// closed (range completes) after all input is consumed.
func TestExtractBatches_OutputClosedAfterDrain(t *testing.T) {
	fe := &fakeExtractor{}
	in := make(chan HashedFile, 5)
	for i := range 5 {
		in <- HashedFile{Path: fmt.Sprintf("/x/%d.mp3", i), Hash: fmt.Sprintf("hx%d", i)}
	}
	close(in)

	for range ExtractBatches(context.Background(), fe, in) {
	}
}

// TestExtractBatches_BinaryRetryIntegration verifies that ExtractBatches
// propagates binary-search retry through the pipeline: with one poison file
// in a batch, the output contains all other tracks.
func TestExtractBatches_BinaryRetryIntegration(t *testing.T) {
	const n = 8
	const poisonIdx = 3

	pe := &poisonExtractor{poisonIdx: poisonIdx}

	in := make(chan HashedFile, n)
	for i := range n {
		in <- HashedFile{
			Path: fmt.Sprintf("/music/track%02d.flac", i),
			Hash: fmt.Sprintf("hash%02d", i),
		}
	}
	close(in)

	var got []Track
	for tr := range ExtractBatches(context.Background(), pe, in) {
		got = append(got, tr)
	}

	if len(got) != n {
		t.Fatalf("expected %d tracks (including fallback for corrupt file), got %d", n, len(got))
	}
	corruptPath := fmt.Sprintf("/music/track%02d.flac", poisonIdx)
	found := false
	for _, tr := range got {
		if tr.Path == corruptPath {
			found = true
			if tr.Artist != "Unknown Artist" {
				t.Errorf("corrupt track artist: want %q, got %q", "Unknown Artist", tr.Artist)
			}
		}
	}
	if !found {
		t.Errorf("corrupt path %q missing from output; should be present with fallback metadata", corruptPath)
	}
}

// TestExtractBatches_HashPreserved verifies that Track.Hash matches the
// HashedFile.Hash that entered the pipeline.
func TestExtractBatches_HashPreserved(t *testing.T) {
	fe := &fakeExtractor{}
	in := make(chan HashedFile, 3)
	inputs := []HashedFile{
		{Path: "/a.flac", Hash: "aaa"},
		{Path: "/b.flac", Hash: "bbb"},
		{Path: "/c.flac", Hash: "ccc"},
	}
	for _, hf := range inputs {
		in <- hf
	}
	close(in)

	byPath := make(map[string]string)
	for tr := range ExtractBatches(context.Background(), fe, in) {
		byPath[tr.Path] = tr.Hash
	}

	for _, hf := range inputs {
		if byPath[hf.Path] != hf.Hash {
			t.Errorf("path %q: want hash %q, got %q", hf.Path, hf.Hash, byPath[hf.Path])
		}
	}
}

// TestExtractBatches_CancelledContextDrainsInputWithoutDeadlock verifies that
// when the consumer cancels the context, ExtractBatches still drains its
// input channel so the upstream HashWorkers / Walk goroutines can exit. Prior
// to the fix, ExtractBatches would block on `out <- t` once the consumer
// stopped reading, leaking the entire pipeline.
func TestExtractBatches_CancelledContextDrainsInputWithoutDeadlock(t *testing.T) {
	in := make(chan HashedFile, 2000)
	for i := 0; i < 2000; i++ {
		in <- HashedFile{Path: "/music/x.flac", Hash: "h"}
	}
	close(in)

	ctx, cancel := context.WithCancel(context.Background())
	out := ExtractBatches(ctx, &fakeExtractor{}, in)

	<-out
	cancel()

	done := make(chan struct{})
	go func() {
		for range out {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("ExtractBatches did not exit after ctx cancellation; producer leak")
	}
}

// TestExtractBatches_CancelledMidStreamDrainsUpstream verifies that when
// ExtractBatches' context is cancelled while upstream is still producing,
// ExtractBatches drains its input channel so the upstream HashWorkers
// goroutines (which send unconditionally on their output) can exit. Without
// the drain, the hash workers block on `out <- HashedFile{...}` forever and
// the pipeline leaks. Distinct from
// TestExtractBatches_CancelledContextDrainsInputWithoutDeadlock above in
// that the upstream HashWorkers goroutines are actually running here, so a
// missing drain would leak them indefinitely.
func TestExtractBatches_CancelledMidStreamDrainsUpstream(t *testing.T) {
	pathCh := make(chan string)
	hashFn := func(p string) (string, error) { return "h", nil }
	hashOut, _ := hashWorkersN(4, pathCh, hashFn)

	dir := t.TempDir()
	realPath := filepath.Join(dir, "song.mp3")
	writeFile(t, realPath, []byte("x"))

	ctx, cancel := context.WithCancel(context.Background())
	out := ExtractBatches(ctx, &fakeExtractor{}, hashOut)

	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		defer close(pathCh)
		for i := 0; i < 5000; i++ {
			select {
			case pathCh <- realPath:
			case <-ctx.Done():
				return
			}
		}
	}()

	for i := 0; i < 3; i++ {
		select {
		case <-out:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatalf("did not receive expected output before cancel")
		}
	}
	cancel()

	drained := make(chan struct{})
	go func() {
		for range out {
		}
		close(drained)
	}()

	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("ExtractBatches output channel never closed; upstream pipeline leaked")
	}
	select {
	case <-producerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("path producer goroutine never returned; HashWorkers must have leaked")
	}
}
