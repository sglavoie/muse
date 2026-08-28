package library

import (
	"fmt"
	"testing"
	"time"
)

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
	out := hashWorkersN(1, in, hashFn)

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
