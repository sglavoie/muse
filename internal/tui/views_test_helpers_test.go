package tui

import "github.com/sglavoie/muse/internal/library"

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

// makeQueue returns a QueueView pre-loaded with the given entries.
func makeQueue(width int, entries []QueueEntry) QueueView {
	return QueueView{
		entries: entries,
		width:   width,
	}
}