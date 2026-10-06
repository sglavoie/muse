package library

import "testing"

func TestAlbumIdentity(t *testing.T) {
	tests := []struct {
		name string
		a, b Track
		same bool
	}{
		{"same title different artists", Track{Album: "Greatest Hits",
			AlbumArtist: "A"}, Track{Album: "Greatest Hits",
			AlbumArtist: "B"}, false},
		{"tagged compilation", Track{Album: "Mix", Artist: "A", AlbumArtist: "Various Artists", Path: "/one/1.flac"}, Track{Album: "Mix", Artist: "B", AlbumArtist: "Various Artists", Path: "/two/2.flac"}, true},
		{"untagged compilation", Track{Album: "Mix", Artist: "A", Path: "/mix/1.flac"}, Track{Album: "Mix", Artist: "B", Path: "/mix/2.flac"}, true},
		{"separate untagged folders", Track{Album: "Mix", Path: "/one/1.flac"}, Track{Album: "Mix", Path: "/two/2.flac"}, false},
		{"titles within a folder", Track{Album: "One", Path: "/music/1.flac"}, Track{Album: "Two", Path: "/music/2.flac"}, false},
		{"blank album artist", Track{Album: "Mix",
			AlbumArtist: "  ", Path: "/mix/1.flac"}, Track{Album: "Mix", Path: "/mix/2.flac"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := AlbumKey(tt.a), AlbumKey(tt.b)
			if (a == b) != tt.same || AlbumMatches(tt.a, b) != tt.same {
				t.Fatalf("identity mismatch: %s / %s", a, b)
			}
			if AlbumTitle(a) != tt.a.Album {
				t.Fatalf("lost title: %s", a)
			}
		})
	}
}

func TestAlbumKeysSeparateDisplayAndMetadata(t *testing.T) {
	track := Track{Album: `A — B: "C"`,
		AlbumArtist: `D: "E"`}
	key := AlbumKey(track)
	if AlbumTitle(key) != track.Album || AlbumLabel(key) != track.Album+" — "+track.AlbumArtist {
		t.Fatal("identity did not round trip")
	}
	if _, ok := ParseAlbumKey("muse:album:1:broken"); ok {
		t.Fatal("accepted malformed identity")
	}
	keys := AlbumKeys([]Track{{Album: "", Path: "/unknown/t.flac"}, track, track})
	if len(keys) != 2 || keys[0] != key || AlbumTitle(keys[1]) != "" {
		t.Fatalf("unexpected albums: %v", keys)
	}
}
