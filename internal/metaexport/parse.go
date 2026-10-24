package metaexport

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Whole-file rejections. Everything else the parser can localize to a single
// row becomes a Warning instead.
var (
	ErrBadMagic           = errors.New("not a muse export: first line must be " + magicPrefix + "1")
	ErrUnsupportedVersion = errors.New("unsupported export version")
	ErrMissingColumn      = errors.New("header is missing a required column")
	ErrNoRows             = errors.New("no data rows")
	ErrMarkdownTable      = errors.New("this looks like a markdown table; re-paste it as plain TSV, not a markdown table")
)

// maxLineBytes bounds a single record. Album and title values are short, but a
// pathological paste should fail loudly rather than allocate without limit.
const maxLineBytes = 1 << 20

// Warning is a problem the parser survived: a skipped row, an ignored column,
// or a malformed preamble line.
type Warning struct {
	Line int
	Msg  string
}

func (w Warning) String() string {
	if w.Line > 0 {
		return fmt.Sprintf("line %d: %s", w.Line, w.Msg)
	}
	return w.Msg
}

// File is a parsed export. Roots maps a root id to the directory it was
// declared with; rows are deduplicated by hash with the last one winning.
type File struct {
	Version int
	Roots   map[int]string
	Rows    []Row
	// Skipped counts dropped data rows only, unlike Warnings, which also
	// covers ignored columns and malformed root declarations.
	Skipped  int
	Warnings []Warning
}

// Parse reads an export that has been round-tripped through a chat.
//
// It is lenient in exactly three places, because LLMs reliably do these:
// it strips ``` fence lines, matches header columns by name so reordering is
// fine (extra columns are ignored with a warning), and unwraps symmetric
// quotes and invisible padding around values. Blank lines and "#" comment
// lines are skipped anywhere, including after the header, since LLMs add
// prose.
//
// It is strict everywhere else: a bad magic line, an unknown major version, a
// header missing a required column, no data rows at all, and any line starting
// with "|" reject the whole file. A row missing only trailing optional cells
// (whitespace stripping eats the tabs of empty ones) is padded with empty
// values. A row with any other wrong field count, an empty hash, an invalid
// escape or an unparsable track/disc is skipped with a warning rather than
// repaired, because every plausible repair loses data.
func Parse(r io.Reader) (*File, error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxLineBytes)

	f := &File{Roots: make(map[int]string)}
	var (
		lineNo int
		cols   map[string]int
		ncols  int
	)
	byHash := make(map[string]int)

	for sc.Scan() {
		lineNo++
		raw := strings.TrimSuffix(sc.Text(), "\r")
		trimmed := strings.Trim(raw, trimCutset)
		if trimmed == "" || strings.HasPrefix(trimmed, "```") {
			continue
		}
		if strings.HasPrefix(trimmed, "|") {
			return nil, ErrMarkdownTable
		}

		if f.Version == 0 {
			v, ok := parseMagicVersion(trimmed)
			if !ok {
				return nil, fmt.Errorf("%w (line %d is %q)", ErrBadMagic, lineNo, clip(trimmed))
			}
			if v != Version {
				return nil, fmt.Errorf("%w: file is v%d, this build reads v%d", ErrUnsupportedVersion, v, Version)
			}
			f.Version = v
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			f.readPreamble(trimmed, lineNo)
			continue
		}

		if cols == nil {
			c, extra, err := parseHeader(raw)
			if err != nil {
				return nil, err
			}
			cols, ncols = c, len(strings.Split(raw, "\t"))
			for _, name := range extra {
				f.warn(lineNo, fmt.Sprintf("ignoring unknown column %q", name))
			}
			continue
		}

		row, ok := f.parseRow(raw, lineNo, cols, ncols)
		if !ok {
			f.Skipped++
			continue
		}
		if idx, dup := byHash[row.Hash]; dup {
			f.warn(lineNo, fmt.Sprintf("duplicate hash %s: this row replaces the earlier one", clip(row.Hash)))
			f.Rows[idx] = row
			continue
		}
		byHash[row.Hash] = len(f.Rows)
		f.Rows = append(f.Rows, row)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if f.Version == 0 {
		return nil, fmt.Errorf("%w (file is empty)", ErrBadMagic)
	}
	if len(f.Rows) == 0 {
		return nil, ErrNoRows
	}
	return f, nil
}

func (f *File) warn(line int, msg string) {
	f.Warnings = append(f.Warnings, Warning{Line: line, Msg: msg})
}

// parseMagicVersion reports the major version declared by line 1. A minor
// suffix is tolerated so a future v1.1 writer stays readable here.
func parseMagicVersion(line string) (int, bool) {
	if !strings.HasPrefix(line, magicPrefix) {
		return 0, false
	}
	rest := strings.Trim(line[len(magicPrefix):], trimCutset)
	if i := strings.IndexByte(rest, '.'); i >= 0 {
		rest = rest[:i]
	}
	n, err := strconv.Atoi(rest)
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

// readPreamble records "# key<TAB>value" lines. Unknown keys are ignored so a
// newer writer can add provenance without breaking this reader.
func (f *File) readPreamble(line string, lineNo int) {
	parts := strings.Split(strings.TrimPrefix(line, "#"), "\t")
	if strings.Trim(parts[0], trimCutset) != "root" {
		return
	}
	if len(parts) < 3 {
		f.warn(lineNo, "ignoring malformed root declaration")
		return
	}
	id, err := strconv.Atoi(strings.Trim(parts[1], trimCutset))
	if err != nil || id < 1 {
		f.warn(lineNo, fmt.Sprintf("ignoring root with invalid id %q", clip(parts[1])))
		return
	}
	dir, err := unescapeField(parts[2])
	if err != nil {
		f.warn(lineNo, fmt.Sprintf("ignoring root %d: %v", id, err))
		return
	}
	f.Roots[id] = dir
}

func parseHeader(raw string) (map[string]int, []string, error) {
	fields := strings.Split(raw, "\t")
	cols := make(map[string]int, len(fields))
	var extra []string
	for i, fld := range fields {
		name, err := unescapeField(fld)
		if err != nil {
			name = fld
		}
		name = strings.ToLower(Cell(name))
		if name == "" {
			continue
		}
		if _, dup := cols[name]; dup {
			continue
		}
		if !isColumn(name) {
			extra = append(extra, name)
			continue
		}
		cols[name] = i
	}
	for _, want := range Columns {
		if want == ColAlbumArtist {
			continue
		}
		if _, ok := cols[want]; !ok {
			return nil, nil, fmt.Errorf("%w: %q", ErrMissingColumn, want)
		}
	}
	return cols, extra, nil
}

func (f *File) parseRow(raw string, line int, cols map[string]int, ncols int) (Row, bool) {
	fields := strings.Split(raw, "\t")
	if len(fields) < ncols && requiredWithin(cols, len(fields)) {
		// The optional album_artist column is written last, so an empty value
		// leaves a trailing tab that chat clients and editors strip. Only
		// empty optional cells can be missing here; restore them as empty.
		fields = append(fields, make([]string, ncols-len(fields))...)
	}
	if len(fields) != ncols {
		f.warn(line, fmt.Sprintf("skipped: %d fields, want %d", len(fields), ncols))
		return Row{}, false
	}

	var bad bool
	get := func(name string) string {
		v, err := unescapeField(fields[cols[name]])
		if err != nil {
			if !bad {
				f.warn(line, fmt.Sprintf("skipped: %s: %v", name, err))
			}
			bad = true
			return ""
		}
		return Cell(v)
	}

	row := Row{
		Hash:    get(ColHash),
		RelPath: get(ColPath),
		Artist:  get(ColArtist),
		Title:   get(ColTitle),
		Album:   get(ColAlbum),
		Genre:   get(ColGenre),
		Line:    line,
	}
	if _, ok := cols[ColAlbumArtist]; ok {
		row.AlbumArtist = get(ColAlbumArtist)
		row.AlbumArtistPresent = true
	}
	rawRoot, rawTrack, rawDisc := get(ColRoot), get(ColTrack), get(ColDisc)
	if bad {
		return Row{}, false
	}
	if row.Hash == "" {
		f.warn(line, "skipped: empty hash")
		return Row{}, false
	}

	track, ok := ParsePosition(rawTrack)
	if !ok {
		f.warn(line, fmt.Sprintf("skipped: track %q is not a number", clip(rawTrack)))
		return Row{}, false
	}
	disc, ok := ParsePosition(rawDisc)
	if !ok {
		f.warn(line, fmt.Sprintf("skipped: disc %q is not a number", clip(rawDisc)))
		return Row{}, false
	}
	row.Track, row.Disc = track, disc

	// root and path are advisory, so a broken root id costs a warning rather
	// than the row.
	if rawRoot != "" {
		id, err := strconv.Atoi(rawRoot)
		if err != nil || id < 0 {
			f.warn(line, fmt.Sprintf("root %q is not a root id; treating the path as absolute", clip(rawRoot)))
		} else {
			row.RootID = id
		}
	}
	return row, true
}

// requiredWithin reports whether every required column sits among the first n
// fields, so a row cut short after them lost only optional or unknown cells.
func requiredWithin(cols map[string]int, n int) bool {
	for _, name := range Columns {
		if name != ColAlbumArtist && cols[name] >= n {
			return false
		}
	}
	return true
}

func isColumn(name string) bool {
	for _, c := range Columns {
		if c == name {
			return true
		}
	}
	return false
}

// clip shortens a value for an error message.
func clip(s string) string {
	const max = 40
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
