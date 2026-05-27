package artists

import (
	"sort"

	"github.com/sglavoie/muse/internal/library"
)

const libraryUnknownArtist = "Unknown Artist"

// DisplayArtistName returns the display name for an artist, mapping any
// unknown-artist sentinel value to the canonical UnknownArtist display string.
func DisplayArtistName(artist string) string {
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

// ArtistMatches reports whether trackArtist belongs to the requested artist,
// treating all unknown-artist sentinel values as equivalent.
func ArtistMatches(trackArtist, artist string) bool {
	if IsUnknownArtist(artist) {
		return IsUnknownArtist(trackArtist)
	}
	return trackArtist == artist
}

// AlbumMatches reports whether trackAlbum belongs to the requested album,
// treating empty string and UnknownAlbum sentinel as equivalent on both sides
// so callers may pass either form without first normalizing via RawName.
func AlbumMatches(trackAlbum, album string) bool {
	if album == "" || album == UnknownAlbum {
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
	var artists []string
	seen := make(map[string]struct{})
	for _, track := range tracks {
		name := DisplayArtistName(track.Artist)
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		artists = append(artists, name)
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
		counts[DisplayArtistName(t.Artist)]++
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
		if !ArtistMatches(track.Artist, artist) {
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
		if ArtistMatches(track.Artist, artist) && AlbumMatches(track.Album, album) {
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
		if ArtistMatches(track.Artist, rawArtist) {
			result = append(result, track)
		}
	}
	return result
}
