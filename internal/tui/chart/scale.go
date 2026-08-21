package chart

import (
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Bar and annotation glyphs. All of them are single-cell so a bar of N glyphs
// occupies exactly N columns.
const (
	glyphFull  = "█"
	glyphTrack = "░"
	glyphGap   = "·"
	glyphUp    = "▲"
	glyphDown  = "▼"
	glyphFlat  = "·"
)

// eighthBlocks indexes partial fill glyphs by eighths of a cell; index 0 is
// the empty string so a fill with no remainder appends nothing.
var eighthBlocks = [8]string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}

// sparkLevels indexes sparkline glyphs from lowest to highest.
var sparkLevels = [8]string{"▁", "▂", "▃", "▄", "▅", "▆", "▇", "█"}

// displayWidth reports the rendered column count of s, ignoring any ANSI
// sequences. All width arithmetic in this package goes through it so wide and
// ambiguous-width runes are accounted for consistently.
func displayWidth(s string) int {
	return ansi.StringWidth(s)
}

func truncateTo(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if displayWidth(s) <= w {
		return s
	}
	return ansi.Truncate(s, w, "")
}

func padRight(s string, w int) string {
	return s + SafeRepeat(" ", w-displayWidth(s))
}

func padLeft(s string, w int) string {
	return SafeRepeat(" ", w-displayWidth(s)) + s
}

// fillSegs splits a bar of width cells into its filled prefix and its track
// remainder. Partial eighth-blocks keep sub-cell precision, and a non-zero
// fraction always renders at least one eighth so a small value is visible
// rather than silently rounding away.
func fillSegs(fraction float64, width int) (fill, track string) {
	width = SafeWidth(width)
	if width == 0 {
		return "", ""
	}
	f := SafeFraction(fraction)
	eighths := int(math.Round(f * float64(width) * 8))
	if eighths == 0 && f > 0 {
		eighths = 1
	}
	full := eighths / 8
	if full >= width {
		return SafeRepeat(glyphFull, width), ""
	}
	fill = SafeRepeat(glyphFull, full)
	if rem := eighths % 8; rem > 0 {
		fill += eighthBlocks[rem]
		full++
	}
	return fill, SafeRepeat(glyphTrack, width-full)
}

// formatPercent renders a fraction as a one-decimal percentage, e.g. "61.5%".
func formatPercent(fraction float64) string {
	return strconv.FormatFloat(SafeFraction(fraction)*100, 'f', 1, 64) + "%"
}

// Scale renders an axis tick row: the first label sits flush left, the last
// flush right, and any middle labels are spaced evenly between them. The two
// endpoints carry the axis, so when the row is too crowded for every label to
// keep a space of clearance the middle ones are what get dropped.
func Scale(labels []string, width int, p Palette) string {
	return styleLine(scaleSegs(labels, width), p)
}

func scalePlain(labels []string, width int) string {
	return plainLine(scaleSegs(labels, width))
}

type scaleTick struct {
	start int
	text  string
}

func scaleSegs(labels []string, width int) []seg {
	width = SafeWidth(width)
	clean := make([]string, 0, len(labels))
	for _, l := range labels {
		if t := truncateTo(SafeText(l), width); t != "" {
			clean = append(clean, t)
		}
	}
	if width == 0 || len(clean) == 0 {
		return nil
	}
	segs := make([]seg, 0, len(clean)*2)
	pos := 0
	for _, t := range scaleTicks(clean, width) {
		if gap := t.start - pos; gap > 0 {
			segs = append(segs, seg{SafeRepeat(" ", gap), rolePlain})
		}
		segs = append(segs, seg{t.text, roleLabel})
		pos = t.start + displayWidth(t.text)
	}
	return segs
}

func scaleTicks(clean []string, width int) []scaleTick {
	ticks := make([]scaleTick, 0, len(clean))
	ticks = append(ticks, scaleTick{0, clean[0]})
	cur := displayWidth(clean[0])

	limit := width
	tail := scaleTick{}
	if last := len(clean) - 1; last > 0 {
		if start := width - displayWidth(clean[last]); start > cur {
			tail = scaleTick{start, clean[last]}
			limit = start - 1
		}
	}

	for i := 1; i < len(clean)-1; i++ {
		w := displayWidth(clean[i])
		ideal := int(math.Round(float64(width-1)*float64(i)/float64(len(clean)-1))) - w/2
		start := max(ideal, cur+1)
		if start+w > limit {
			continue
		}
		ticks = append(ticks, scaleTick{start, clean[i]})
		cur = start + w
	}
	if tail.text != "" {
		ticks = append(ticks, tail)
	}
	return ticks
}

// LegendItem is one entry of a Legend.
type LegendItem struct {
	// Glyph is the swatch drawn before the label. An empty Glyph omits the
	// swatch and its trailing space.
	Glyph string
	Label string
	// Muted draws the entry in the muted role, for the "absent" or "unheard"
	// half of a two-part chart.
	Muted bool
}

// Legend renders items separated by " · ", dropping trailing entries that do
// not fit within width.
func Legend(items []LegendItem, width int, p Palette) string {
	return styleLine(legendSegs(items, width), p)
}

func legendPlain(items []LegendItem, width int) string {
	return plainLine(legendSegs(items, width))
}

const legendSep = " · "

func legendSegs(items []LegendItem, width int) []seg {
	width = SafeWidth(width)
	if width == 0 {
		return nil
	}
	segs := make([]seg, 0, len(items)*4)
	cur := 0
	for _, it := range items {
		glyph := SafeText(it.Glyph)
		label := SafeText(it.Label)
		if glyph == "" && label == "" {
			continue
		}
		need := displayWidth(glyph) + displayWidth(label)
		if glyph != "" && label != "" {
			need++
		}
		sep := 0
		if cur > 0 {
			sep = displayWidth(legendSep)
		}
		if cur+sep+need > width {
			break
		}
		textRole, glyphRole := roleLabel, roleBar
		if it.Muted {
			textRole, glyphRole = roleMuted, roleMuted
		}
		if sep > 0 {
			segs = append(segs, seg{legendSep, roleMuted})
		}
		if glyph != "" {
			segs = append(segs, seg{glyph, glyphRole})
			if label != "" {
				segs = append(segs, seg{" ", rolePlain})
			}
		}
		if label != "" {
			segs = append(segs, seg{label, textRole})
		}
		cur += sep + need
	}
	return segs
}

// Delta renders a signed change as an arrow plus magnitude ("▲4", "▼2") or a
// neutral dot when change is zero.
func Delta(change int, p Palette) string {
	return styleLine(deltaSegs(change), p)
}

func deltaPlain(change int) string {
	return plainLine(deltaSegs(change))
}

func deltaSegs(change int) []seg {
	switch {
	case change > 0:
		return []seg{{glyphUp + strconv.Itoa(change), roleUp}}
	case change < 0:
		// TrimPrefix rather than negation: -math.MinInt overflows.
		return []seg{{glyphDown + strings.TrimPrefix(strconv.Itoa(change), "-"), roleDown}}
	default:
		return []seg{{glyphFlat, roleFlat}}
	}
}
