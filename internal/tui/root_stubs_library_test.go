package tui

import (
	"context"

	"github.com/sglavoie/muse/internal/library"
)

// stubExtractor is a no-op MetadataExtractor used in rescan tests to avoid
// a real exiftool dependency.
type stubExtractor struct{}

func (stubExtractor) Extract(_ context.Context, files []library.HashedFile) ([]library.Track, error) {
	return nil, nil
}

// stubQueueAppender is a minimal libtui.QueueAppender for root-level tests.
type stubQueueAppender struct {
	tracks []library.Track
}

func (s *stubQueueAppender) AppendTrack(t library.Track, _ string) (int64, error) {
	s.tracks = append(s.tracks, t)
	return int64(len(s.tracks)), nil
}
