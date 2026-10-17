package library

import (
	"testing"

	musiclib "github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
)

type countingSearcher struct{ calls int }

func (s *countingSearcher) Search(query string, tracks []musiclib.Track) []musiclib.Match {
	s.calls++
	return (musiclib.SahilmFuzzySearcher{}).Search(query, tracks)
}

func TestEmptySearchResultIsCachedAcrossFrames(t *testing.T) {
	search := &countingSearcher{}
	v := View{Fuzzy: search, Width: 80, Height: 20}
	v.Index.BulkLoad([]musiclib.Track{{Hash: "one", Title: "One"}})
	v.Filter.Text = "absent"
	v = v.RebuildFilterCache()
	for range 3 {
		_ = v.Render(styles.NewStyles(true), nil, nil, nil, "", "")
		_, _ = v.Panel().FocusedTrack()
	}
	if search.calls != 1 {
		t.Fatalf("empty results searched %d times", search.calls)
	}
}

func TestVisibleTrackSnapshotsDoNotMutateIndex(t *testing.T) {
	v := View{}
	v.Index.BulkLoad([]musiclib.Track{{Hash: "one", Title: "Original"}})
	for _, snapshot := range [][]musiclib.Track{v.BaseTracks(), v.Panel().VisibleTracks()} {
		snapshot[0].Title = "Changed outside the index"
	}
	track, ok := v.Panel().FocusedTrack()
	if !ok || track.Title != "Original" {
		t.Fatal("borrowed rendering storage escaped", track)
	}
}
