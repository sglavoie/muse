package library

import (
	"encoding/json"
	"testing"
)

// TestExifOrdinalAliases pins the per-container spellings exiftool emits for
// track and disc numbers, as observed on real FLAC, Opus, M4A, and MP3 files.
func TestExifOrdinalAliases(t *testing.T) {
	for _, tc := range []struct {
		data        string
		track, disc int
	}{
		{`{"Track":"3/12","PartOfSet":"2/3"}`, 3, 2},
		{`{"TrackNumber":"3/12","Discnumber":"2/3"}`, 3, 2},
		{`{"TrackNumber":"3 of 12","DiskNumber":"2 of 3"}`, 3, 2},
		{`{"Track":5,"DiscNumber":1}`, 5, 1},
		{`{"Track":"","TrackNumber":"7"}`, 7, 0},
	} {
		var record exifRecord
		if err := json.Unmarshal([]byte(tc.data), &record); err != nil {
			t.Fatal(err)
		}
		if got := parseTrackNumber(record.Track); got != tc.track {
			t.Errorf("%s: track %d, want %d", tc.data, got, tc.track)
		}
		if got := parseTrackNumber(record.DiscNumber); got != tc.disc {
			t.Errorf("%s: disc %d, want %d", tc.data, got, tc.disc)
		}
	}
}

func TestExifAlbumArtistAliases(t *testing.T) {
	for _, tc := range []struct{ data, want string }{
		{`{"AlbumArtist":"Release Artist","Artist":"Guest"}`, "Release Artist"},
		{`{"Albumartist":"Various Artists"}`, "Various Artists"},
		{`{"Band":"Release Artist"}`, "Release Artist"},
		{`{"AlbumArtist":"Explicit","Band":"Fallback"}`, "Explicit"},
		{`{"AlbumArtist":"","Band":"Fallback"}`, "Fallback"},
		{`{"AlbumArtist":["One","Two"]}`, "One; Two"},
		{`{"Artist":"Track performer"}`, ""},
	} {
		var record exifRecord
		if err := json.Unmarshal([]byte(tc.data), &record); err != nil {
			t.Fatal(err)
		}
		if record.AlbumArtist != tc.want {
			t.Fatalf("%s: album artist %q, want %q", tc.data, record.AlbumArtist, tc.want)
		}
	}
}
