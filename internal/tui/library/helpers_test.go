package library

import (
	"charm.land/lipgloss/v2"
	musiclib "github.com/sglavoie/muse/internal/library"
)

// styledText returns text wrapped with the given lipgloss style, producing
// the same string that production code generates via Style.Render.
func styledText(style lipgloss.Style, text string) string {
	return style.Render(text)
}

// fakeQueueAppender records every AppendTrack call and returns a
// monotonically-increasing fake queue ID. If err is non-nil it is returned
// from every call without recording the track.
type fakeQueueAppender struct {
	tracks []musiclib.Track
	nextID int64
	err    error
}

func (f *fakeQueueAppender) AppendTrack(t musiclib.Track, _ string) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	f.nextID++
	f.tracks = append(f.tracks, t)
	return f.nextID, nil
}

func buildIndex(tracks []musiclib.Track) musiclib.LibraryIndex {
	var idx musiclib.LibraryIndex
	for _, t := range tracks {
		idx.Insert(t)
	}
	return idx
}

var sampleTracks = []musiclib.Track{
	{Artist: "Alice", Title: "Song One", Album: "Debut", Duration: 185},
	{Artist: "Bob", Title: "Track Two", Album: "Second", Duration: 242},
	{Artist: "Unknown Artist", Title: "Mystery", Album: "", Duration: 0},
}

// highlightFake is a test-local FuzzySearcher that returns predetermined results.
type highlightFake struct {
	results []musiclib.Match
}

func (h highlightFake) Search(_ string, _ []musiclib.Track) []musiclib.Match {
	return h.results
}
