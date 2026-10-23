package library

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

const albumKeyPrefix = "muse:album:1:"

// AlbumIdentity separates an album's title from the release it belongs to.
// Owner is an Album Artist tag or, when absent, the containing directory.
type AlbumIdentity struct {
	Title string
	Kind  string
	Owner string
}

// AlbumKey returns a durable, unambiguous collection/favorite key. Audio tags
// keep their original values; this key must never be written into Album.
func AlbumKey(t Track) string {
	a := AlbumIdentity{Title: t.Album, Kind: "artist", Owner: strings.TrimSpace(t.AlbumArtist)}
	if a.Title == "(unknown album)" {
		a.Title = ""
	}
	if a.Owner == "" {
		a.Kind = "folder"
		a.Owner = filepath.Dir(t.Path)
		if t.Path == "" {
			// Production tracks have paths. Keep synthetic/orphan rows usable
			// without treating all pathless albums as one folder.
			a.Kind, a.Owner = "artist", t.Artist
		}
	}
	data, _ := json.Marshal(a)
	return albumKeyPrefix + string(data)
}

// ParseAlbumKey distinguishes collection identities from legacy title-only
// favorites. Unknown versions remain legacy values, never guessed identities.
func ParseAlbumKey(key string) (AlbumIdentity, bool) {
	var a AlbumIdentity
	if !strings.HasPrefix(key, albumKeyPrefix) {
		return a, false
	}
	if json.Unmarshal([]byte(strings.TrimPrefix(key, albumKeyPrefix)), &a) != nil {
		return a, false
	}
	return a, a.Kind == "artist" || a.Kind == "folder"
}

// AlbumTitle returns raw editable metadata, not the disambiguated UI label.
func AlbumTitle(key string) string {
	if a, ok := ParseAlbumKey(key); ok {
		return a.Title
	}
	if key == "(unknown album)" {
		return ""
	}
	return key
}

// AlbumLabel is for display/search only. Raw legacy titles pass through.
func AlbumLabel(key string) string {
	a, ok := ParseAlbumKey(key)
	if !ok {
		return key
	}
	title := a.Title
	if title == "" {
		title = "(unknown album)"
	}
	if a.Owner != "" {
		return title + " — " + a.Owner
	}
	return title
}

// AlbumDisplayLabel uses the same disambiguation as the parent collection.
// A removed or legacy key still gets a readable title without exposing paths.
func AlbumDisplayLabel(key string, siblings []string) string {
	if label, ok := AlbumDisplayLabels(siblings)[key]; ok {
		return label
	}
	return AlbumDisplayLabels([]string{key})[key]
}

// AlbumMatches supports old title-only references while new UI paths always
// pass an identity. This also lets unresolved legacy favorites remain visible.
func AlbumMatches(t Track, key string) bool {
	if _, ok := ParseAlbumKey(key); ok {
		return AlbumKey(t) == key
	}
	return AlbumTitle(t.Album) == AlbumTitle(key)
}

// AlbumKeys derives separate releases, ordered by displayed title and owner.
func AlbumKeys(tracks []Track) []string {
	seen := make(map[string]struct{})
	var keys []string
	for _, t := range tracks {
		key := AlbumKey(t)
		if _, ok := seen[key]; !ok {
			keys = append(keys, key)
			seen[key] = struct{}{}
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := AlbumTitle(keys[i]), AlbumTitle(keys[j])
		if (a == "") != (b == "") {
			return a != ""
		}
		return AlbumLabel(keys[i]) < AlbumLabel(keys[j])
	})
	return keys
}
