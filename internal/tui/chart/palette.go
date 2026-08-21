package chart

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// Palette holds the styles chart primitives render with. Every field must be
// foreground-only: charts are drawn inside zebra-striped and now-playing rows
// whose background belongs to the row, not to the chart. The zero Palette is
// usable and renders everything unstyled.
type Palette struct {
	Bar   lipgloss.Style // filled portion of a bar
	Track lipgloss.Style // unfilled remainder of a bar
	Label lipgloss.Style // row labels and axis ticks
	Value lipgloss.Style // numeric annotations next to a bar
	Muted lipgloss.Style // absent data and other de-emphasized glyphs
	Up    lipgloss.Style // positive delta
	Down  lipgloss.Style // negative delta
	Flat  lipgloss.Style // zero delta
}

// FromStyles maps the shared TUI palette onto the chart roles so charts need
// no color constants of their own. Track deliberately uses Dim rather than
// ProgressEmpty: ProgressEmpty carries a background that would bleed over the
// zebra stripe of the row a chart is drawn in.
func FromStyles(s styles.Styles) Palette {
	return Palette{
		Bar:   s.ProgressFilled,
		Track: s.Dim,
		Label: s.TextPrimary,
		Value: s.TextSecondary,
		Muted: s.Dim,
		Up:    s.Success,
		Down:  s.Error,
		Flat:  s.TextTertiary,
	}
}

// role selects which Palette field a segment is styled with.
type role uint8

const (
	rolePlain role = iota // never styled — spacing between segments
	roleBar
	roleTrack
	roleLabel
	roleValue
	roleMuted
	roleUp
	roleDown
	roleFlat
)

// seg is one styled run of a rendered line. Splitting a line into segments is
// what makes the width invariant structural: the plain and styled renderers
// consume the same segments and differ only in whether they call Render.
type seg struct {
	text string
	role role
}

func (p Palette) styleFor(r role) lipgloss.Style {
	switch r {
	case roleBar:
		return p.Bar
	case roleTrack:
		return p.Track
	case roleLabel:
		return p.Label
	case roleValue:
		return p.Value
	case roleMuted:
		return p.Muted
	case roleUp:
		return p.Up
	case roleDown:
		return p.Down
	case roleFlat:
		return p.Flat
	default:
		return lipgloss.NewStyle()
	}
}

// plainLine concatenates the segment texts with no styling.
func plainLine(segs []seg) string {
	var b strings.Builder
	for _, s := range segs {
		b.WriteString(s.text)
	}
	return b.String()
}

// styleLine renders segs with the palette, coalescing adjacent segments that
// share a role so a run emits a single escape sequence.
func styleLine(segs []seg, p Palette) string {
	var b strings.Builder
	for i := 0; i < len(segs); {
		j := i
		var run strings.Builder
		for j < len(segs) && segs[j].role == segs[i].role {
			run.WriteString(segs[j].text)
			j++
		}
		text := run.String()
		if text != "" {
			if segs[i].role == rolePlain {
				b.WriteString(text)
			} else {
				b.WriteString(p.styleFor(segs[i].role).Render(text))
			}
		}
		i = j
	}
	return b.String()
}

func plainLines(lines [][]seg) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	for i, segs := range lines {
		out[i] = plainLine(segs)
	}
	return out
}

func styleLines(lines [][]seg, p Palette) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, len(lines))
	for i, segs := range lines {
		out[i] = styleLine(segs, p)
	}
	return out
}
