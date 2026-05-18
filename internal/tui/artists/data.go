package artists

import (
	"sort"

	"github.com/sglavoie/muse/internal/library"
)

// DeriveArtists extracts unique artist names from a sorted track slice.
// The input must be sorted by artist (as LibraryIndex.Tracks() provides).
func DeriveArtists(tracks []library.Track) []string {
	if len(tracks) == 0 {
		return nil
	}
	name := func(artist string) string {
		if artist == "" {
			return UnknownArtist
		}
		return artist
	}
	artists := []string{name(tracks[0].Artist)}
	for i := 1; i < len(tracks); i++ {
		if tracks[i].Artist != tracks[i-1].Artist {
			artists = append(artists, name(tracks[i].Artist))
		}
	}
	return artists
}

// DeriveArtistCounts maps each artist name to its track count in tracks.
// Tracks with an empty Artist field count under UnknownArtist, mirroring
// DeriveArtists' display sentinel.
func DeriveArtistCounts(tracks []library.Track) map[string]int {
	if len(tracks) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, t := range tracks {
		name := t.Artist
		if name == "" {
			name = UnknownArtist
		}
		counts[name]++
	}
	return counts
}

// DeriveAlbums returns unique album names for the given artist, sorted
// alphabetically.
func DeriveAlbums(tracks []library.Track, artist string) []string {
	artist = RawName(artist)
	seen := make(map[string]struct{})
	var albums []string
	for _, track := range tracks {
		if track.Artist != artist {
			continue
		}
		album := track.Album
		if album == "" {
			album = UnknownAlbum
		}
		if _, ok := seen[album]; !ok {
			seen[album] = struct{}{}
			albums = append(albums, album)
		}
	}
	sort.Slice(albums, func(i, j int) bool {
		if albums[i] == UnknownAlbum {
			return false
		}
		if albums[j] == UnknownAlbum {
			return true
		}
		return albums[i] < albums[j]
	})
	return albums
}

// DeriveTracks returns all tracks matching the given artist and album.
func DeriveTracks(tracks []library.Track, artist, album string) []library.Track {
	artist = RawName(artist)
	album = RawName(album)
	var result []library.Track
	for _, track := range tracks {
		if track.Artist == artist && track.Album == album {
			result = append(result, track)
		}
	}
	return result
}

// DeriveArtistTracks returns all tracks by the given artist, regardless of album.
func DeriveArtistTracks(tracks []library.Track, artist string) []library.Track {
	rawArtist := RawName(artist)
	var result []library.Track
	for _, track := range tracks {
		if track.Artist == rawArtist {
			result = append(result, track)
		}
	}
	return result
}
