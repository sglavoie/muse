package genres

import (
	"slices"
	"sort"

	"github.com/sglavoie/muse/internal/library"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
)

const libraryUnknownArtist = "Unknown Artist"

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

// DeriveGenreCounts maps each genre name to its track count. A track carrying
// multiple genre tags increments each of its tags (consistent with
// library.SplitGenres semantics used elsewhere). Tracks with no recognised
// genre count under UnknownGenre.
func DeriveGenreCounts(tracks []library.Track) map[string]int {
	if len(tracks) == 0 {
		return nil
	}
	counts := make(map[string]int)
	for _, t := range tracks {
		gs := library.SplitGenres(t.Genre)
		if len(gs) == 0 {
			counts[UnknownGenre]++
			continue
		}
		for _, g := range gs {
			counts[g]++
		}
	}
	return counts
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
		name := displayArtistName(t.Artist)
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
		if artistMatches(t.Artist, rawArtist) {
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
// function matches the rendered unknown row, which includes tracks with no
// recognised genre plus any literal UnknownGenre tag collapsed into that row.
func trackMatchesGenre(t library.Track, raw string) bool {
	genres := library.SplitGenres(t.Genre)
	if raw == "" {
		return len(genres) == 0 || slices.Contains(genres, UnknownGenre)
	}
	return slices.Contains(genres, raw)
}

func displayArtistName(artist string) string {
	if isUnknownArtist(artist) {
		return arttui.UnknownArtist
	}
	return artist
}

func artistMatches(trackArtist, artist string) bool {
	if isUnknownArtist(artist) {
		return isUnknownArtist(trackArtist)
	}
	return trackArtist == artist
}

func isUnknownArtist(artist string) bool {
	return artist == "" || artist == libraryUnknownArtist
}
