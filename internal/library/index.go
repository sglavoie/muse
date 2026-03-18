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

// BulkLoad replaces the index contents with the given tracks and sorts once.
// It is O(N log N) vs N binary-search inserts, making it suitable for
// pre-populating the index from the database cache on startup.
func (idx *LibraryIndex) BulkLoad(tracks []Track) {
	idx.tracks = make([]Track, len(tracks))
	copy(idx.tracks, tracks)
	sort.Slice(idx.tracks, func(i, j int) bool {
		return less(idx.tracks[i], idx.tracks[j])
	})
}

// RemoveByPath removes the first track whose Path equals path.
// Returns true if a track was found and removed, false otherwise.
func (idx *LibraryIndex) RemoveByPath(path string) bool {
	for i, t := range idx.tracks {
		if t.Path == path {
			idx.tracks = append(idx.tracks[:i], idx.tracks[i+1:]...)
			return true
		}
	}
	return false
}

// UpsertByPath removes any existing track with the given path then inserts t,
// ensuring modified files replace their stale entries without creating duplicates.
func (idx *LibraryIndex) UpsertByPath(path string, t Track) {
	idx.RemoveByPath(path)
	idx.Insert(t)
}

// FindByHash returns the first track whose Hash equals hash.
// Returns the track and true if found, or a zero Track and false otherwise.
func (idx *LibraryIndex) FindByHash(hash string) (Track, bool) {
	for _, t := range idx.tracks {
		if t.Hash == hash {
			return t, true
		}
	}
	return Track{}, false
}

// UpdateMetadata finds the track by hash, updates its Title, Artist, and Album,
// then re-inserts it to maintain sort order (since the sort key includes Artist
// and Title, the position may change). Returns true if the track was found and
// updated.
func (idx *LibraryIndex) UpdateMetadata(hash, title, artist, album string) bool {
	for i, t := range idx.tracks {
		if t.Hash == hash {
			// Remove from current position.
			idx.tracks = append(idx.tracks[:i], idx.tracks[i+1:]...)
			// Update fields and re-insert in sorted order.
			t.Title = title
			t.Artist = artist
			t.Album = album
			idx.Insert(t)
			return true
		}
	}
	return false
}
