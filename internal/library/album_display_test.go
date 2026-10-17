package library

import (
	"strings"
	"testing"
)

func TestAlbumDisplayLabelsDisambiguateWithoutChangingIdentity(t *testing.T) {
	tracks := []Track{
		{Album: "Unique", Path: "/music/alice/unique/1.flac"},
		{Album: "Live", Path: "/music/alice/live/1.flac"},
		{Album: "Live", Path: "/music/bob/live/1.flac"},
		{Album: "Live", AlbumArtist: "Charlie", Path: "/music/charlie/live/1.flac"},
		{Path: "/music/untagged/1.flac"},
	}
	keys := AlbumKeys(tracks)
	labels := AlbumDisplayLabels(append(keys, keys[0]))
	want := []string{"Unique", "Live — alice/live", "Live — bob/live", "Live — Charlie", "(unknown album)"}
	for i, track := range tracks {
		key := AlbumKey(track)
		if labels[key] != want[i] {
			t.Errorf("%s: got %q, want %q", track.Path, labels[key], want[i])
		}
		if !AlbumMatches(track, key) {
			t.Errorf("display labels changed release identity: %s", track.Path)
		}
	}
	if !strings.Contains(AlbumLabel(AlbumKey(tracks[1])), "/music/alice/live") {
		t.Fatal("full folder must remain available to search and details")
	}
}

func TestAlbumDisplayLabelsResolveSameFolderNameAcrossRoots(t *testing.T) {
	a := AlbumKey(Track{Album: "Live", Path: "/one/alice/live/1.flac"})
	b := AlbumKey(Track{Album: "Live", Path: "/two/alice/live/1.flac"})
	c := AlbumKey(Track{Album: "Live", AlbumArtist: "one/alice/live"})
	labels := AlbumDisplayLabels([]string{a, b, c, "Legacy"})
	if labels[a] != "Live — /one/alice/live" || labels[b] != "Live — two/alice/live" {
		t.Fatalf("ambiguous folder qualifiers: %v", labels)
	}
	if labels["Legacy"] != "Legacy" || labels[c] != "Live — one/alice/live" {
		t.Fatalf("artist and legacy labels changed: %v", labels)
	}
}
