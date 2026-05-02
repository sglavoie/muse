package library

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func makeAudioFiles(tb testing.TB, dir string, n int) []string {
	tb.Helper()
	paths := make([]string, n)
	for i := range n {
		p := filepath.Join(dir, fmt.Sprintf("track_%04d.mp3", i))
		if err := os.WriteFile(p, []byte("ID3"), 0o644); err != nil {
			tb.Fatalf("write file: %v", err)
		}
		paths[i] = p
	}
	return paths
}

func TestPathsToChan_ClosesWhenDone(t *testing.T) {
	paths := []string{"/a.mp3", "/b.mp3", "/c.mp3"}

	ch := pathsToChan(context.Background(), paths)

	var got []string
	for p := range ch {
		got = append(got, p)
	}

	if len(got) != len(paths) {
		t.Errorf("got %d paths, want %d", len(got), len(paths))
	}
}

func TestPathsToChan_RespectsCtxCancellation(t *testing.T) {
	paths := make([]string, 1000)
	for i := range paths {
		paths[i] = fmt.Sprintf("/track_%04d.mp3", i)
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch := pathsToChan(ctx, paths)

	<-ch
	cancel()

	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("channel was not closed after context cancellation; possible goroutine leak")
	}
}
