package library

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

// TestHashWorkers_PanicReportsDegraded verifies that a hash worker panic is
// visible at the exported boundary rather than being laundered into a clean
// finish. The output channel closes either way, so a caller that reads the
// close as "every file was hashed" would silently act on a short batch — most
// damagingly by concluding the unhashed files had been deleted. The stage
// outcome is what makes the two cases distinguishable.
func TestHashWorkers_PanicReportsDegraded(t *testing.T) {
	prev := hashFileFn
	t.Cleanup(func() { hashFileFn = prev })
	hashFileFn = func(string) (string, error) { panic("blake3 blew up") }

	in := make(chan string, 4)
	for i := range 4 {
		in <- fmt.Sprintf("/music/track_%02d.flac", i)
	}
	close(in)

	out, outcome := HashWorkers(in)

	drained := make(chan int, 1)
	go func() {
		count := 0
		for range out {
			count++
		}
		drained <- count
	}()

	select {
	case count := <-drained:
		if count != 0 {
			t.Errorf("expected no emissions from panicking workers, got %d", count)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("output channel never closed after the workers panicked")
	}

	err := outcome.Err()
	if err == nil {
		t.Fatal("closed output channel reported success after a hash worker panicked")
	}
	if !errors.Is(err, ErrHashWorkerPanicked) {
		t.Errorf("outcome error = %v, want it to wrap ErrHashWorkerPanicked", err)
	}
}

// TestHashWorkers_CleanRunReportsNoFailure is the negative control for
// TestHashWorkers_PanicReportsDegraded: without it the outcome could report a
// failure unconditionally and still satisfy the panic assertions.
func TestHashWorkers_CleanRunReportsNoFailure(t *testing.T) {
	in := make(chan string)
	close(in)

	out, outcome := HashWorkers(in)
	for range out { //nolint:revive // draining an empty stream
	}
	if err := outcome.Err(); err != nil {
		t.Errorf("clean hash run reported a failure: %v", err)
	}
}

// TestHashWorkers_LastWorkerPanicTerminatesPipeline verifies that when the only
// (i.e. the last) hash worker dies to a panic, the pipeline still terminates:
// the walker goroutine upstream must not be left blocked forever on an
// unbuffered send into an input channel nobody reads any more, and the output
// channel must still close so the extraction stage downstream finishes.
func TestHashWorkers_LastWorkerPanicTerminatesPipeline(t *testing.T) {
	const paths = 16

	// Unbuffered, so the producer can only advance while a worker is reading —
	// exactly the shape Walk feeds HashWorkers with.
	in := make(chan string)
	producerDone := make(chan struct{})
	go func() {
		defer close(producerDone)
		defer close(in)
		for i := range paths {
			in <- fmt.Sprintf("/music/track_%02d.flac", i)
		}
	}()

	hashFn := func(string) (string, error) { panic("blake3 blew up") }

	// A single worker is the last worker by construction.
	out, _ := hashWorkersN(1, in, hashFn)

	drained := make(chan int, 1)
	go func() {
		count := 0
		for range out {
			count++
		}
		drained <- count
	}()

	select {
	case count := <-drained:
		if count != 0 {
			t.Errorf("expected no emissions from a panicking worker, got %d", count)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("output channel never closed after the last worker panicked")
	}

	select {
	case <-producerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("producer still blocked on the input channel after the last worker panicked")
	}
}
