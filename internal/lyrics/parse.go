package lyrics

import (
	"bytes"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

// Parse reads LRC or plain-text lyrics. Lines with [mm:ss.xx] time tags make
// synced lyrics; text without any makes unsynced lyrics. It returns nil when
// there is nothing to show.
//
// It accepts what real files contain: several time tags on one line, times
// with no fraction or with one to three fraction digits, an [offset:] in
// milliseconds, metadata tags such as [ar:] and [ti:] (ignored), enhanced
// per-word times <mm:ss.xx> (dropped), a byte-order mark, CRLF line endings,
// UTF-16, and Windows-1252/Latin-1 files that are not valid UTF-8.
func Parse(data []byte) *Lyrics {
	return ParseString(decode(data))
}

// ParseString is Parse for text that is already decoded, such as a lyrics
// tag read from the audio file.
func ParseString(s string) *Lyrics {
	s = strings.TrimPrefix(s, "\uFEFF")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	var p parser
	for _, raw := range strings.Split(s, "\n") {
		p.line(raw)
	}
	l := p.result()
	if l.Empty() {
		return nil
	}
	return l
}

type parser struct {
	timed   []Line
	plain   []string
	offset  time.Duration
	hasTime bool
}

// line reads one line of the file.
func (p *parser) line(raw string) {
	stamps, rest, meta := splitTags(strings.TrimSpace(raw))
	if meta != nil {
		if strings.EqualFold(meta[0], "offset") {
			if ms, err := strconv.Atoi(strings.TrimPrefix(strings.TrimSpace(meta[1]), "+")); err == nil {
				p.offset = time.Duration(ms) * time.Millisecond
			}
		}
		return
	}
	text := cleanText(rest)
	if len(stamps) == 0 {
		p.plain = append(p.plain, text)
		return
	}
	p.hasTime = true
	for _, at := range stamps {
		p.timed = append(p.timed, Line{At: at, Text: text})
	}
}

// result assembles the lyrics. A positive offset makes the lines show
// earlier, as the LRC format defines it.
func (p *parser) result() *Lyrics {
	if !p.hasTime {
		return &Lyrics{Lines: plainLines(p.plain)}
	}
	for i := range p.timed {
		p.timed[i].At = max(0, p.timed[i].At-p.offset)
	}
	sort.SliceStable(p.timed, func(i, j int) bool { return p.timed[i].At < p.timed[j].At })
	return &Lyrics{Lines: p.timed, Synced: true}
}

// plainLines keeps unsynced text as written, minus leading and trailing
// blank lines, with runs of blank lines folded into one stanza break.
func plainLines(texts []string) []Line {
	var lines []Line
	for _, t := range texts {
		if t == "" && (len(lines) == 0 || lines[len(lines)-1].Text == "") {
			continue
		}
		lines = append(lines, Line{Text: t})
	}
	for len(lines) > 0 && lines[len(lines)-1].Text == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// splitTags reads the [..] tags at the start of a line. It returns the time
// tags, the text after them, and for a metadata line such as [ar:Artist]
// its key and value. A bracket that is neither, like "[Chorus]", is text.
func splitTags(s string) (stamps []time.Duration, rest string, meta []string) {
	for strings.HasPrefix(s, "[") {
		end := strings.IndexByte(s, ']')
		if end < 0 {
			break
		}
		inner := s[1:end]
		if at, ok := parseStamp(inner); ok {
			stamps = append(stamps, at)
		} else if key, value, ok := metaTag(inner); ok && len(stamps) == 0 && strings.TrimSpace(s[end+1:]) == "" {
			return nil, "", []string{key, value}
		} else {
			break
		}
		s = strings.TrimLeft(s[end+1:], " \t")
	}
	return stamps, s, nil
}

// metaTag splits an [key:value] tag whose key is a short word, like ar, ti,
// offset or length.
func metaTag(inner string) (key, value string, ok bool) {
	key, value, ok = strings.Cut(inner, ":")
	if !ok || key == "" || len(key) > 12 {
		return "", "", false
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '#' {
			return "", "", false
		}
	}
	return key, value, true
}

// parseStamp reads a time tag: mm:ss, mm:ss.f to mm:ss.fff, the mm:ss:cc
// variant some tools write, or hh:mm:ss.ff.
func parseStamp(s string) (time.Duration, bool) {
	parts := strings.Split(s, ":")
	if len(parts) < 2 || len(parts) > 3 {
		return 0, false
	}
	frac := ""
	last := parts[len(parts)-1]
	if whole, f, ok := strings.Cut(last, "."); ok {
		parts[len(parts)-1], frac = whole, f
	} else if len(parts) == 3 {
		// mm:ss:cc: the third field is hundredths, not seconds.
		parts, frac = parts[:2], last
	}
	var secs int
	for _, part := range parts {
		n, ok := digits(part)
		if !ok {
			return 0, false
		}
		secs = secs*60 + n
	}
	ms, ok := fraction(frac)
	if !ok {
		return 0, false
	}
	return time.Duration(secs)*time.Second + time.Duration(ms)*time.Millisecond, true
}

// fraction reads the digits after a time's decimal point as milliseconds:
// "5" is 500, "34" is 340, "345" is 345; further digits are ignored.
func fraction(s string) (int, bool) {
	if s == "" {
		return 0, true
	}
	if _, ok := digits(s); !ok {
		return 0, false
	}
	s = (s + "00")[:3]
	n, _ := strconv.Atoi(s)
	return n, true
}

func digits(s string) (int, bool) {
	if s == "" || len(s) > 6 {
		return 0, false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// wordTime matches the per-word times of enhanced LRC, like <00:12.34>.
var wordTime = regexp.MustCompile(`<\d+:\d+(?:[.:]\d+)?>`)

// cleanText drops enhanced word times and the extra spaces they leave.
func cleanText(s string) string {
	if strings.Contains(s, "<") {
		s = wordTime.ReplaceAllString(s, "")
	}
	return strings.Join(strings.Fields(s), " ")
}

var (
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
	utf16LEBOM = []byte{0xFF, 0xFE}
	utf16BEBOM = []byte{0xFE, 0xFF}
)

// decode turns file bytes into text: UTF-8 with or without a byte-order
// mark, UTF-16 with one, and otherwise Windows-1252, a superset of Latin-1
// that older lyric files often use.
func decode(data []byte) string {
	switch {
	case bytes.HasPrefix(data, utf8BOM):
		data = data[len(utf8BOM):]
	case bytes.HasPrefix(data, utf16LEBOM) || bytes.HasPrefix(data, utf16BEBOM):
		// ExpectBOM reads the byte order from the mark and drops it.
		out, err := unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM).NewDecoder().Bytes(data)
		if err == nil {
			return string(out)
		}
	}
	if utf8.Valid(data) {
		return string(data)
	}
	if out, err := charmap.Windows1252.NewDecoder().Bytes(data); err == nil {
		return string(out)
	}
	return strings.ToValidUTF8(string(data), string(utf8.RuneError))
}
