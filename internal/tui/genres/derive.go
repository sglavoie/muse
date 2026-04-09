package genres

import (
	"slices"
	"sort"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

// DeriveGenres extracts unique genre names from all tracks. Multi-genre tags
// are split via library.SplitGenres. Tracks with no genre contribute to
// UnknownGenre. Results are sorted alphabetically with unknowns last.
func DeriveGenres(tracks []library.Track) []string {
	if len(tracks) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var genres []string
	for _, t := range tracks {
		gs := library.SplitGenres(t.Genre)
		if len(gs) == 0 {
			if _, ok := seen[UnknownGenre]; !ok {
				seen[UnknownGenre] = struct{}{}
				genres = append(genres, UnknownGenre)
			}
			continue
		}
		for _, g := range gs {
			if _, ok := seen[g]; ok {
				continue
			}
			seen[g] = struct{}{}
			genres = append(genres, g)
		}
	}
	sort.Slice(genres, func(i, j int) bool {
		if genres[i] == UnknownGenre {
			return false
		}
		if genres[j] == UnknownGenre {
			return true
		}
		return genres[i] < genres[j]
	})
	return genres
}

// DeriveGenreArtists returns unique artist names for tracks that belong to the
// given genre. Tracks with no artist use the arttui.UnknownArtist sentinel.
// Results are sorted alphabetically with unknowns last.
func DeriveGenreArtists(tracks []library.Track, genre string) []string {
	raw := RawGenre(genre)
	seen := make(map[string]struct{})
	var artists []string
	for _, t := range tracks {
		if !trackMatchesGenre(t, raw) {
			continue
		}
		name := t.Artist
		if name == "" {
			name = arttui.UnknownArtist
		}
		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			artists = append(artists, name)
		}
	}
	sort.Slice(artists, func(i, j int) bool {
		if artists[i] == arttui.UnknownArtist {
			return false
		}
		if artists[j] == arttui.UnknownArtist {
			return true
		}
		return artists[i] < artists[j]
	})
	return artists
}

// DeriveGenreArtistTracks returns all tracks matching both the given genre and
// artist.
func DeriveGenreArtistTracks(tracks []library.Track, genre, artist string) []library.Track {
	rawGenre := RawGenre(genre)
	rawArtist := arttui.RawName(artist)
	var result []library.Track
	for _, t := range tracks {
		if !trackMatchesGenre(t, rawGenre) {
			continue
		}
		if t.Artist == rawArtist {
			result = append(result, t)
		}
	}
	return result
}

// DeriveGenreTracks returns all tracks for a genre regardless of artist.
// Used for bulk enqueue at the genre level.
func DeriveGenreTracks(tracks []library.Track, genre string) []library.Track {
	raw := RawGenre(genre)
	var result []library.Track
	for _, t := range tracks {
		if trackMatchesGenre(t, raw) {
			result = append(result, t)
		}
	}
	return result
}

// trackMatchesGenre reports whether t belongs to the genre identified by raw
// (the result of RawGenre on the display genre string). When raw is empty the
// function matches tracks that have no recognised genre.
func trackMatchesGenre(t library.Track, raw string) bool {
	if raw == "" {
		return len(library.SplitGenres(t.Genre)) == 0
	}
	return slices.Contains(library.SplitGenres(t.Genre), raw)
}
