package metaexport

import (
	"path/filepath"
	"strings"

	"github.com/sglavoie/muse/internal/library"
)

// Current is the library's present state for one hash, as the diff sees it.
type Current struct {
	Path   string
	Artist string
	Title  string
	Album  string
	Genre  string
	Track  int
	Disc   int
}

// Change is one row that differs from the library. A nil field means the row
// left that field alone; a non-nil one carries the value to write.
type Change struct {
	Hash    string
	Line    int
	Current Current

	Artist *string
	Title  *string
	Album  *string
	Genre  *string
	Track  *int
	Disc   *int

	// PathMoved means the file is no longer where the export said it was.
	// Hashes are content-addressed, so this is a move, not a mismatch.
	PathMoved bool
}

// FieldCounts is how many changes touch each field.
type FieldCounts struct {
	Artist int
	Title  int
	Album  int
	Genre  int
	Track  int
	Disc   int
}

// Plan is everything the confirmation overlay needs to describe an import.
// Warnings starts empty: the caller fills it with the parse warnings, which
// are the only warnings an import produces.
type Plan struct {
	Changes   []Change
	Unknown   []string // hashes the library no longer holds
	Moved     int
	Unchanged int
	Counts    FieldCounts
	Warnings  []Warning
}

// Diff pairs parsed rows with the library state keyed by hash.
//
// The path is advisory only, so a row whose file moved is counted and still
// applied; only an unknown hash is dropped.
func Diff(rows []Row, cur map[string]Current) Plan {
	var p Plan
	for _, row := range rows {
		c, ok := cur[row.Hash]
		if !ok {
			p.Unknown = append(p.Unknown, row.Hash)
			continue
		}

		ch := Change{Hash: row.Hash, Line: row.Line, Current: c, PathMoved: moved(row.RelPath, c.Path)}
		if ch.PathMoved {
			p.Moved++
		}

		if !sameText(row.Artist, c.Artist) {
			ch.Artist = &row.Artist
			p.Counts.Artist++
		}
		if !sameText(row.Title, c.Title) {
			ch.Title = &row.Title
			p.Counts.Title++
		}
		if !sameText(row.Album, c.Album) {
			ch.Album = &row.Album
			p.Counts.Album++
		}
		// Genres compare canonically so reordering or respelling the
		// separators is a no-op, but the raw string is what gets stored.
		if !sameGenre(row.Genre, c.Genre) {
			ch.Genre = &row.Genre
			p.Counts.Genre++
		}
		if row.Track != c.Track {
			ch.Track = &row.Track
			p.Counts.Track++
		}
		if row.Disc != c.Disc {
			ch.Disc = &row.Disc
			p.Counts.Disc++
		}

		if ch.Artist == nil && ch.Title == nil && ch.Album == nil &&
			ch.Genre == nil && ch.Track == nil && ch.Disc == nil {
			p.Unchanged++
			continue
		}
		p.Changes = append(p.Changes, ch)
	}
	return p
}

// sameText reports whether an imported value leaves the library value alone.
// Parse runs every cell through Cell, so a library value with its own
// surrounding quotes or padding (a title like "Heroes", quotes included)
// comes back cleaned up from an untouched export; matching either form keeps
// that from showing up as an edit the user never made.
func sameText(row, cur string) bool {
	return row == cur || row == Cell(cur)
}

// sameGenre is sameText for genres, which also compare canonically so
// reordering or respelling the separators is a no-op.
func sameGenre(row, cur string) bool {
	g := CanonicalGenre(row)
	return g == CanonicalGenre(cur) || g == CanonicalGenre(Cell(cur))
}

// CanonicalGenre is the form a genre string takes once the library has split
// it, so "Rock, Pop" and "Pop; Rock" compare equal. It is also what the user
// should be shown as the stored value.
func CanonicalGenre(s string) string {
	return strings.Join(library.SplitGenres(s), "; ")
}

// moved reports whether the library path still ends in the path the export
// declared. Comparing the suffix rather than a rebuilt absolute path keeps a
// reconfigured music directory from reporting every track as moved.
func moved(relPath, curPath string) bool {
	if relPath == "" {
		return false
	}
	if curPath == relPath {
		return false
	}
	return !strings.HasSuffix(curPath, string(filepath.Separator)+relPath)
}
