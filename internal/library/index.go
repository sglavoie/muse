package library

import "sort"

const unknownArtist = "Unknown Artist"

// LibraryIndex is a sorted slice of Track values maintained in artist-then-title
// order. Tracks with artist "Unknown Artist" sort below all named artists.
// The soft upper bound is 50,000 tracks.
type LibraryIndex struct {
	tracks []Track
	byHash map[string]int // hash → index in tracks slice
}

// rebuildHashMap rebuilds the byHash map from the current tracks slice.
func (idx *LibraryIndex) rebuildHashMap() {
	idx.byHash = make(map[string]int, len(idx.tracks))
	for i, t := range idx.tracks {
		idx.byHash[t.Hash] = i
	}
}

// Len returns the number of tracks in the index.
func (idx *LibraryIndex) Len() int { return len(idx.tracks) }

// Tracks returns a read-only view of the sorted track slice.
func (idx *LibraryIndex) Tracks() []Track { return idx.tracks }

// isUnknownArtist reports whether the artist field represents an unknown artist
// (either the sentinel "Unknown Artist" or an empty string).
func isUnknownArtist(artist string) bool {
	return artist == unknownArtist || artist == ""
}

// less reports whether track i should sort before track j.
// Tracks with unknown artists sort after all named artists; within the same
// artist, unknown albums sort after named albums; within the same album,
// unknown titles sort after named titles; otherwise alphabetical.
func less(a, b Track) bool {
	aUnknown := isUnknownArtist(a.Artist)
	bUnknown := isUnknownArtist(b.Artist)
	if aUnknown != bUnknown {
		return bUnknown // named artist before unknown
	}
	if a.Artist != b.Artist {
		return a.Artist < b.Artist
	}
	aAlbumUnknown := a.Album == ""
	bAlbumUnknown := b.Album == ""
	if aAlbumUnknown != bAlbumUnknown {
		return bAlbumUnknown // named album before unknown album
	}
	if a.Album != b.Album {
		return a.Album < b.Album
	}
	aTitleUnknown := a.Title == ""
	bTitleUnknown := b.Title == ""
	if aTitleUnknown != bTitleUnknown {
		return bTitleUnknown // named title before unknown title
	}
	return a.Title < b.Title
}

// adjustIndicesAfterInsert increments all hash-map entries at positions >= pos
// by one, then records the new entry at pos. Call after splicing t into the
// tracks slice but before returning from the mutating operation.
func (idx *LibraryIndex) adjustIndicesAfterInsert(pos int, hash string) {
	for h, i := range idx.byHash {
		if i >= pos {
			idx.byHash[h] = i + 1
		}
	}
	idx.byHash[hash] = pos
}

// adjustIndicesAfterRemove deletes the removed hash and decrements all
// hash-map entries at positions > pos by one. Call after removing the element
// at pos from the tracks slice.
func (idx *LibraryIndex) adjustIndicesAfterRemove(pos int, hash string) {
	delete(idx.byHash, hash)
	for h, i := range idx.byHash {
		if i > pos {
			idx.byHash[h] = i - 1
		}
	}
}

// Insert adds t into the index in sorted order using binary search.
// If a track with the same non-empty hash already exists it is replaced
// (removed then re-inserted) to prevent orphaned duplicates and an inflated
// Len(). Tracks with an empty hash are always inserted without dedup.
func (idx *LibraryIndex) Insert(t Track) {
	// Remove any existing entry with the same hash to avoid duplicates.
	if t.Hash != "" {
		if oldPos, ok := idx.byHash[t.Hash]; ok && oldPos < len(idx.tracks) {
			idx.tracks = append(idx.tracks[:oldPos], idx.tracks[oldPos+1:]...)
			idx.adjustIndicesAfterRemove(oldPos, t.Hash)
		}
	}
	pos := sort.Search(len(idx.tracks), func(i int) bool {
		return !less(idx.tracks[i], t)
	})
	idx.tracks = append(idx.tracks, Track{})
	copy(idx.tracks[pos+1:], idx.tracks[pos:])
	idx.tracks[pos] = t
	if idx.byHash == nil {
		idx.byHash = make(map[string]int, 1)
	}
	idx.adjustIndicesAfterInsert(pos, t.Hash)
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
	idx.rebuildHashMap()
}

// RemoveByPath removes the first track whose Path equals path.
// Returns true if a track was found and removed, false otherwise.
func (idx *LibraryIndex) RemoveByPath(path string) bool {
	for i, t := range idx.tracks {
		if t.Path == path {
			hash := t.Hash
			idx.tracks = append(idx.tracks[:i], idx.tracks[i+1:]...)
			idx.adjustIndicesAfterRemove(i, hash)
			return true
		}
	}
	return false
}

// UpsertByPath removes any existing track with the given path then inserts t,
// ensuring modified files replace their stale entries without creating duplicates.
func (idx *LibraryIndex) UpsertByPath(path string, t Track) {
	// Inline remove so we only do one incremental adjust per operation.
	for i, old := range idx.tracks {
		if old.Path == path {
			idx.tracks = append(idx.tracks[:i], idx.tracks[i+1:]...)
			idx.adjustIndicesAfterRemove(i, old.Hash)
			break
		}
	}
	// Remove any existing entry with the same hash to avoid duplicates,
	// mirroring the dedup logic in Insert.
	if t.Hash != "" {
		if oldPos, ok := idx.byHash[t.Hash]; ok && oldPos < len(idx.tracks) {
			idx.tracks = append(idx.tracks[:oldPos], idx.tracks[oldPos+1:]...)
			idx.adjustIndicesAfterRemove(oldPos, t.Hash)
		}
	}
	// Inline insert.
	pos := sort.Search(len(idx.tracks), func(i int) bool {
		return !less(idx.tracks[i], t)
	})
	idx.tracks = append(idx.tracks, Track{})
	copy(idx.tracks[pos+1:], idx.tracks[pos:])
	idx.tracks[pos] = t
	if idx.byHash == nil {
		idx.byHash = make(map[string]int, 1)
	}
	idx.adjustIndicesAfterInsert(pos, t.Hash)
}

// FindByHash returns the track whose Hash equals hash.
// Returns the track and true if found, or a zero Track and false otherwise.
func (idx *LibraryIndex) FindByHash(hash string) (Track, bool) {
	if i, ok := idx.byHash[hash]; ok && i < len(idx.tracks) {
		return idx.tracks[i], true
	}
	return Track{}, false
}

// FindByPath returns the first track whose Path equals path.
// Returns the track and true if found, or a zero Track and false otherwise.
func (idx *LibraryIndex) FindByPath(path string) (Track, bool) {
	for _, t := range idx.tracks {
		if t.Path == path {
			return t, true
		}
	}
	return Track{}, false
}

// IncrementPlayCount bumps PlayCount by 1 and sets LastPlayedAt for the
// track identified by hash. Returns true if the track was found.
func (idx *LibraryIndex) IncrementPlayCount(hash string, playedAt int64) bool {
	if i, ok := idx.byHash[hash]; ok && i < len(idx.tracks) {
		idx.tracks[i].PlayCount++
		idx.tracks[i].LastPlayedAt = playedAt
		return true
	}
	return false
}

// UpdateDuration finds the track by hash and updates its Duration field in
// place. No re-sort is needed because Duration is not part of the sort key.
// Returns true if the track was found.
func (idx *LibraryIndex) UpdateDuration(hash string, duration float64) bool {
	if i, ok := idx.byHash[hash]; ok && i < len(idx.tracks) {
		idx.tracks[i].Duration = duration
		return true
	}
	return false
}

// UpdateMetadata finds the track by hash, updates its Title, Artist, Album, and
// Genre, then re-inserts it to maintain sort order (since the sort key includes
// Artist and Title, the position may change). Returns true if the track was
// found and updated.
func (idx *LibraryIndex) UpdateMetadata(hash, title, artist, album, genre string) bool {
	for i, t := range idx.tracks {
		if t.Hash == hash {
			// Remove from current position with one incremental adjust.
			idx.tracks = append(idx.tracks[:i], idx.tracks[i+1:]...)
			idx.adjustIndicesAfterRemove(i, hash)
			// Update fields and re-insert in sorted order with one incremental adjust.
			t.Title = title
			t.Artist = artist
			t.Album = album
			t.Genre = genre
			pos := sort.Search(len(idx.tracks), func(j int) bool {
				return !less(idx.tracks[j], t)
			})
			idx.tracks = append(idx.tracks, Track{})
			copy(idx.tracks[pos+1:], idx.tracks[pos:])
			idx.tracks[pos] = t
			idx.adjustIndicesAfterInsert(pos, hash)
			return true
		}
	}
	return false
}
