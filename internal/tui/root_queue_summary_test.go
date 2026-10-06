package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/tui/queue"
)

func TestQueueSummaryCountsAlbumReleases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		entries []queue.Entry
		want    int
	}{
		{"same title, different album artists", []queue.Entry{
			{Album: "Greatest Hits", AlbumArtist: "Alice"},
			{Album: "Greatest Hits", AlbumArtist: "Bob"},
		}, 2},
		{"compilation across discs", []queue.Entry{
			{Album: "Collection", AlbumArtist: "Various Artists", Artist: "Alice", Path: "/disc1/a.flac"},
			{Album: "Collection", AlbumArtist: "Various Artists", Artist: "Bob", Path: "/disc2/b.flac"},
		}, 1},
		{"folder fallback", []queue.Entry{
			{Album: "Greatest Hits", Path: "/alice/a.flac"},
			{Album: "Greatest Hits", Path: "/alice/b.flac"},
			{Album: "Greatest Hits", Path: "/bob/a.flac"},
		}, 2},
		{"duplicate entries", []queue.Entry{
			{Album: "Alpha", AlbumArtist: "Alice", Path: "/a.flac"},
			{Album: "Alpha", AlbumArtist: "Alice", Path: "/a.flac"},
		}, 1},
		{"missing album metadata", []queue.Entry{{Path: "/a.flac"}, {Path: "/other/b.flac"}}, 0},
		{"empty queue", nil, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newModelWithTracks()
			m.queue.Entries = tc.entries
			summary := m.buildQueueSummary()
			if summary.Albums != tc.want || summary.Tracks != len(tc.entries) {
				t.Fatalf("summary = %+v, want %d albums and %d tracks", summary, tc.want, len(tc.entries))
			}
		})
	}
}
