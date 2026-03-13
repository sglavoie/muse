package library

import "sort"

const unknownArtist = "Unknown Artist"

// LibraryIndex is a sorted slice of Track values maintained in artist-then-title
// order. Tracks with artist "Unknown Artist" sort below all named artists.
// The soft upper bound is 50,000 tracks.
type LibraryIndex struct {
	tracks []Track
}

// Len returns the number of tracks in the index.
func (idx *LibraryIndex) Len() int { return len(idx.tracks) }

// Tracks returns a read-only view of the sorted track slice.
func (idx *LibraryIndex) Tracks() []Track { return idx.tracks }

// less reports whether track i should sort before track j.
// Unknown Artist sorts after all named artists; within the same artist,
// tracks are ordered by title.
func less(a, b Track) bool {
	aUnknown := a.Artist == unknownArtist
	bUnknown := b.Artist == unknownArtist
	if aUnknown != bUnknown {
		return bUnknown // named artist before Unknown Artist
	}
	if a.Artist != b.Artist {
		return a.Artist < b.Artist
	}
	return a.Title < b.Title
}

// Insert adds t into the index in sorted order using binary search.
func (idx *LibraryIndex) Insert(t Track) {
	pos := sort.Search(len(idx.tracks), func(i int) bool {
		return !less(idx.tracks[i], t)
	})
	idx.tracks = append(idx.tracks, Track{})
	copy(idx.tracks[pos+1:], idx.tracks[pos:])
	idx.tracks[pos] = t
}
