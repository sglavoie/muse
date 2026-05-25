package artists

import (
	"sort"

	"github.com/sglavoie/muse/internal/library"
)

const libraryUnknownArtist = "Unknown Artist"

func displayArtistName(artist string) string {
	if IsUnknownArtist(artist) {
		return UnknownArtist
	}
	return artist
}

// IsUnknownArtist reports whether artist represents an unknown or missing
// artist, matching both the library sentinel ("Unknown Artist") and the TUI
// display sentinel ("(unknown artist)") as well as the empty string.
func IsUnknownArtist(artist string) bool {
	return artist == "" || artist == libraryUnknownArtist || artist == UnknownArtist
}

func artistMatches(trackArtist, artist string) bool {
	if IsUnknownArtist(artist) {
		return IsUnknownArtist(trackArtist)
	}
	return trackArtist == artist
}

func albumMatches(trackAlbum, album string) bool {
	if album == "" {
		return trackAlbum == "" || trackAlbum == UnknownAlbum
	}
	return trackAlbum == album
}

// DeriveArtists extracts unique artist names from a sorted track slice.
// The input must be sorted by artist (as LibraryIndex.Tracks() provides).
func DeriveArtists(tracks []library.Track) []string {
	if len(tracks) == 0 {
		return nil
	}
	artists := []string{displayArtistName(tracks[0].Artist)}
	for i := 1; i < len(tracks); i++ {
		name := displayArtistName(tracks[i].Artist)
		if name != artists[len(artists)-1] {
			artists = append(artists, name)
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
		counts[displayArtistName(t.Artist)]++
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
		if !artistMatches(track.Artist, artist) {
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
		if artistMatches(track.Artist, artist) && albumMatches(track.Album, album) {
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
		if artistMatches(track.Artist, rawArtist) {
			result = append(result, track)
		}
	}
	return result
}
