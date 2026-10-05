package library

import (
	"encoding/json"
	"testing"
)

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
