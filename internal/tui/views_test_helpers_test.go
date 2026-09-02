package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// stubFileExistsTrue replaces the package-level queue.FileExists check with
// one that always returns true for the duration of the test. Tests that
// exercise actionSelect with synthetic paths use this helper so the
// pre-flight stat does not short-circuit Enter → PlayQueueIndexMsg into
// the missing-track removal path.
func stubFileExistsTrue(t *testing.T) {
	t.Helper()
	saved := queue.FileExists
	queue.FileExists = func(string) bool { return true }
	t.Cleanup(func() { queue.FileExists = saved })
}

// fakeQueueAppender records enqueue calls for testing.
type fakeQueueAppender struct {
	tracks []library.Track
	nextID int64
}

func (f *fakeQueueAppender) AppendTrack(t library.Track, _ string) (int64, error) {
	f.nextID++
	f.tracks = append(f.tracks, t)
	return f.nextID, nil
}
