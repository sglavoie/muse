// Package metaexport implements the muse metadata round-trip file format: a
// tab-separated export of library rows that survives being pasted through an
// LLM chat, plus a strict parser that turns the corrected paste back into a
// diff against the library.
//
// The format is documented by Encode; the leniencies the parser allows are
// documented by Parse.
package metaexport

import (
	"io"
	"strconv"
	"strings"
	"time"
)

// Version is the major version of the file format this package reads and
// writes. A file declaring any other major version is rejected.
const Version = 1

// magicPrefix opens line 1 of every export; the major version follows it.
const magicPrefix = "# muse-export v"

// Column names. Every one of them is required on import; the parser matches
// them by name so an LLM may reorder them freely.
const (
	ColHash   = "hash"
	ColRoot   = "root"
	ColPath   = "path"
	ColArtist = "artist"
	ColTitle  = "title"
	ColAlbum  = "album"
	ColGenre  = "genre"
	ColTrack  = "track"
	ColDisc   = "disc"
)

// Columns lists every column an export writes, in write order.
var Columns = []string{
	ColHash, ColRoot, ColPath,
	ColArtist, ColTitle, ColAlbum, ColGenre,
	ColTrack, ColDisc,
}

// Row is one exported track. Hash is the only join key on import: RootID and
// RelPath are context for the LLM and are ignored when applying edits.
type Row struct {
	Hash    string
	RootID  int // 1-based id of a declared root; 0 means RelPath is absolute
	RelPath string
	Artist  string
	Title   string
	Album   string
	Genre   string
	Track   int // 0 = unknown
	Disc    int // 0 = unknown
	Line    int // 1-based physical line the row was parsed from; 0 when encoding
}

// Document is a whole export: the music directories the rows live under (root
// id is index+1), the provenance preamble, and the rows themselves.
type Document struct {
	Roots       []string
	GeneratedAt time.Time
	ViewLabel   string
	Rows        []Row
}

// Encode writes d as UTF-8 TSV with a trailing newline:
//
//	# muse-export v1
//	# generated	2026-08-21T14:03:11Z
//	# view	Browse > Albums > Most Relaxing Classical
//	# count	73
//	# root	1	/Users/me/Music
//	hash	root	path	artist	title	album	genre	track	disc
//	a3f91c	1	Classical/01 Sheep.mp3		01. Sheep	Most Relaxing		0	0
//
// Every text value is escaped so one record is always one physical line, which
// is what survives a chat round-trip. Values are written verbatim otherwise:
// never Unicode-normalize them, because macOS paths are NFD and normalizing
// breaks the rejoin. Encode issues many small writes; pass a buffered writer.
func Encode(w io.Writer, d Document) error {
	ew := &errWriter{w: w}
	ew.line(magicPrefix + strconv.Itoa(Version))
	ew.line("# generated\t" + d.GeneratedAt.UTC().Format(time.RFC3339))
	ew.line("# view\t" + escapeField(d.ViewLabel))
	ew.line("# count\t" + strconv.Itoa(len(d.Rows)))
	for i, root := range d.Roots {
		ew.line("# root\t" + strconv.Itoa(i+1) + "\t" + escapeField(root))
	}
	ew.line(strings.Join(Columns, "\t"))
	for _, r := range d.Rows {
		ew.line(strings.Join([]string{
			escapeField(r.Hash),
			strconv.Itoa(r.RootID),
			escapeField(r.RelPath),
			escapeField(r.Artist),
			escapeField(r.Title),
			escapeField(r.Album),
			escapeField(r.Genre),
			strconv.Itoa(r.Track),
			strconv.Itoa(r.Disc),
		}, "\t"))
	}
	return ew.err
}

type errWriter struct {
	w   io.Writer
	err error
}

func (e *errWriter) line(s string) {
	if e.err != nil {
		return
	}
	if _, e.err = io.WriteString(e.w, s); e.err != nil {
		return
	}
	_, e.err = io.WriteString(e.w, "\n")
}
