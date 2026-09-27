package library

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

// UnknownArtist is the placeholder the extractors store for a file with no
// artist tag. It is not a real artist, so taste profiles and similarity scores
// should treat it (and a blank artist) as "no information".
const UnknownArtist = "Unknown Artist"

// LibraryIndex is a sorted slice of Track values maintained in artist-then-title
// order. Tracks with artist "Unknown Artist" sort below all named artists.
// The soft upper bound is 50,000 tracks.
type LibraryIndex struct {
	tracks []Track
	byHash map[string]int // non-empty hash -> likely index in tracks slice
}

// rebuildHashMap rebuilds the byHash map from the current tracks slice.
func (idx *LibraryIndex) rebuildHashMap() {
	idx.byHash = make(map[string]int, len(idx.tracks))
	for i, t := range idx.tracks {
		if t.Hash != "" {
			idx.byHash[t.Hash] = i
		}
	}
}

// Len returns the number of tracks in the index.
func (idx *LibraryIndex) Len() int { return len(idx.tracks) }

// Tracks returns a snapshot of the sorted track slice.
func (idx *LibraryIndex) Tracks() []Track {
	tracks := make([]Track, len(idx.tracks))
	copy(tracks, idx.tracks)
	return tracks
}

// IsUnknownArtist reports whether the artist field represents an unknown artist
// (either the sentinel "Unknown Artist" or an empty string).
func IsUnknownArtist(artist string) bool {
	return artist == UnknownArtist || artist == ""
}

// less reports whether track i should sort before track j.
// Tracks with unknown artists sort after all named artists; within the same
// artist, unknown albums sort after named albums; within the same album,
// unknown titles sort after named titles; otherwise alphabetical, ignoring
// case (see compareText).
func less(a, b Track) bool {
	aUnknown := IsUnknownArtist(a.Artist)
	bUnknown := IsUnknownArtist(b.Artist)
	if aUnknown != bUnknown {
		return bUnknown // named artist before unknown
	}
	if c := compareText(a.Artist, b.Artist); c != 0 {
		return c < 0
	}
	aAlbumUnknown := a.Album == ""
	bAlbumUnknown := b.Album == ""
	if aAlbumUnknown != bAlbumUnknown {
		return bAlbumUnknown // named album before unknown album
	}
	if c := compareText(a.Album, b.Album); c != 0 {
		return c < 0
	}
	aTitleUnknown := a.Title == ""
	bTitleUnknown := b.Title == ""
	if aTitleUnknown != bTitleUnknown {
		return bTitleUnknown // named title before unknown title
	}
	return compareText(a.Title, b.Title) < 0
}

// compareText orders two tag values case-insensitively, matching the rest of
// the app (column sorts lowercase, SQL uses COLLATE NOCASE), so "alt-J" does
// not land after "Zappa". Values that differ only in case fall back to a raw
// byte comparison, keeping the order total and every exact spelling contiguous.
func compareText(a, b string) int {
	if c := foldCompare(a, b); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// foldCompare compares a and b rune by rune after lowercasing each rune. It
// does not allocate, since BulkLoad runs it O(N log N) times over the library.
func foldCompare(a, b string) int {
	for a != "" && b != "" {
		ra, na := utf8.DecodeRuneInString(a)
		rb, nb := utf8.DecodeRuneInString(b)
		a, b = a[na:], b[nb:]
		if ra == rb {
			continue
		}
		if la, lb := unicode.ToLower(ra), unicode.ToLower(rb); la != lb {
			if la < lb {
				return -1
			}
			return 1
		}
	}
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return -1
	default:
		return 1
	}
}

// findIndexByHash returns the current slice position for hash. The hash map is
// maintained as a hint cache: slice insertions/removals can make unrelated
// positions stale, so callers validate the hinted slot and fall back to a scan.
func (idx *LibraryIndex) findIndexByHash(hash string) (int, bool) {
	if hash == "" {
		return 0, false
	}
	if i, ok := idx.byHash[hash]; ok && i >= 0 && i < len(idx.tracks) && idx.tracks[i].Hash == hash {
		return i, true
	}
	for i, t := range idx.tracks {
		if t.Hash == hash {
			if idx.byHash == nil {
				idx.byHash = make(map[string]int, 1)
			}
			idx.byHash[hash] = i
			return i, true
		}
	}
	if idx.byHash != nil {
		delete(idx.byHash, hash)
	}
	return 0, false
}

// recordHashPosition records the inserted hash's current position. Positions
// for unrelated hashes are refreshed lazily by findIndexByHash.
func (idx *LibraryIndex) recordHashPosition(pos int, hash string) {
	if hash != "" {
		if idx.byHash == nil {
			idx.byHash = make(map[string]int, 1)
		}
		idx.byHash[hash] = pos
	}
}

// removeHashPosition deletes the removed hash's hint. Positions for unrelated
// hashes are refreshed lazily by findIndexByHash.
func (idx *LibraryIndex) removeHashPosition(hash string) {
	if hash != "" {
		delete(idx.byHash, hash)
	}
}

// Insert adds t into the index in sorted order using binary search.
// If a track with the same non-empty hash already exists it is replaced
// (removed then re-inserted) to prevent orphaned duplicates and an inflated
// Len(). Tracks with an empty hash are always inserted without dedup.
func (idx *LibraryIndex) Insert(t Track) {
	// Remove any existing entry with the same hash to avoid duplicates.
	if t.Hash != "" {
		if oldPos, ok := idx.findIndexByHash(t.Hash); ok {
			idx.tracks = append(idx.tracks[:oldPos], idx.tracks[oldPos+1:]...)
			idx.removeHashPosition(t.Hash)
		}
	}
	pos := sort.Search(len(idx.tracks), func(i int) bool {
		return !less(idx.tracks[i], t)
	})
	idx.tracks = append(idx.tracks, Track{})
	copy(idx.tracks[pos+1:], idx.tracks[pos:])
	idx.tracks[pos] = t
	idx.recordHashPosition(pos, t.Hash)
}

// BulkLoad replaces the index contents with the given tracks and sorts once.
// It is O(N log N) vs N binary-search inserts, making it suitable for
// pre-populating the index from the database cache on startup.
func (idx *LibraryIndex) BulkLoad(tracks []Track) {
	// Deduplicate non-empty hashes with "last one wins" semantics to mirror
	// sequential Insert behavior and avoid orphaned duplicates in idx.tracks.
	// Empty hashes are intentionally not deduplicated.
	deduped := make([]Track, 0, len(tracks))
	byHash := make(map[string]int, len(tracks))
	for _, t := range tracks {
		if t.Hash == "" {
			deduped = append(deduped, t)
			continue
		}
		if i, ok := byHash[t.Hash]; ok {
			deduped[i] = t
			continue
		}
		byHash[t.Hash] = len(deduped)
		deduped = append(deduped, t)
	}

	idx.tracks = make([]Track, len(deduped))
	copy(idx.tracks, deduped)
	// Stable sort so tracks with identical Artist/Album/Title preserve input
	// order; otherwise BulkLoad after a rescan would reshuffle equal-keyed
	// rows nondeterministically, causing visible UI flicker.
	sort.SliceStable(idx.tracks, func(i, j int) bool {
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
			idx.removeHashPosition(hash)
			return true
		}
	}
	return false
}

// UpsertByPath removes any existing track with the given path then inserts t,
// ensuring modified files replace their stale entries without creating duplicates.
func (idx *LibraryIndex) UpsertByPath(path string, t Track) {
	// Inline remove so the stale path entry cannot coexist with the replacement.
	for i, old := range idx.tracks {
		if old.Path == path {
			idx.tracks = append(idx.tracks[:i], idx.tracks[i+1:]...)
			idx.removeHashPosition(old.Hash)
			break
		}
	}
	// Remove any existing entry with the same hash to avoid duplicates,
	// mirroring the dedup logic in Insert.
	if t.Hash != "" {
		if oldPos, ok := idx.findIndexByHash(t.Hash); ok {
			idx.tracks = append(idx.tracks[:oldPos], idx.tracks[oldPos+1:]...)
			idx.removeHashPosition(t.Hash)
		}
	}
	// Inline insert.
	pos := sort.Search(len(idx.tracks), func(i int) bool {
		return !less(idx.tracks[i], t)
	})
	idx.tracks = append(idx.tracks, Track{})
	copy(idx.tracks[pos+1:], idx.tracks[pos:])
	idx.tracks[pos] = t
	idx.recordHashPosition(pos, t.Hash)
}

// FindByHash returns the track whose Hash equals hash.
// Returns the track and true if found, or a zero Track and false otherwise.
func (idx *LibraryIndex) FindByHash(hash string) (Track, bool) {
	if i, ok := idx.findIndexByHash(hash); ok {
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

// IncrementPlayCount bumps PlayCount by 1 and advances LastPlayedAt for the
// track identified by hash. Clamps with the existing LastPlayedAt so a
// backwards-going clock (NTP correction, manual adjust) cannot rewind the
// timestamp and reorder the History view. Returns true if the track was found.
func (idx *LibraryIndex) IncrementPlayCount(hash string, playedAt int64) bool {
	if i, ok := idx.findIndexByHash(hash); ok {
		idx.tracks[i].PlayCount++
		if playedAt > idx.tracks[i].LastPlayedAt {
			idx.tracks[i].LastPlayedAt = playedAt
		}
		return true
	}
	return false
}

// UpdateDuration finds the track by hash and updates its Duration field in
// place. No re-sort is needed because Duration is not part of the sort key.
// Returns true if the track was found.
func (idx *LibraryIndex) UpdateDuration(hash string, duration float64) bool {
	if i, ok := idx.findIndexByHash(hash); ok {
		idx.tracks[i].Duration = duration
		return true
	}
	return false
}

// UpdateTrackPosition finds the track by hash and updates its TrackNumber and
// DiscNumber in place. No re-sort is needed because neither is part of the sort
// key. Returns true if the track was found.
func (idx *LibraryIndex) UpdateTrackPosition(hash string, track, disc int) bool {
	if i, ok := idx.findIndexByHash(hash); ok {
		idx.tracks[i].TrackNumber = track
		idx.tracks[i].DiscNumber = disc
		return true
	}
	return false
}

// UpdateMetadata finds the track by hash, updates its Title, Artist, Album, and
// Genre, then re-inserts it to maintain sort order (since the sort key includes
// Artist and Title, the position may change). Returns true if the track was
// found and updated.
func (idx *LibraryIndex) UpdateMetadata(hash, title, artist, album, genre string) bool {
	if hash == "" {
		return false
	}
	for i, t := range idx.tracks {
		if t.Hash == hash {
			// Remove from the current position before changing sort keys.
			idx.tracks = append(idx.tracks[:i], idx.tracks[i+1:]...)
			idx.removeHashPosition(hash)
			// Update fields and re-insert in sorted order.
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
			idx.recordHashPosition(pos, hash)
			return true
		}
	}
	return false
}
