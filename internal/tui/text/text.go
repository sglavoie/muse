// Package text provides display-width-aware string formatting utilities
// for terminal rendering. All width calculations use ansi.StringWidth
// to correctly handle wide characters (CJK, emoji) and ANSI escape codes.
package text

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// Truncate shortens s to at most maxLen display columns, appending "..." when
// truncated. Returns an empty string when maxLen is zero or negative.
func Truncate(s string, maxLen int) string {
	if maxLen <= 0 {
		return ""
	}
	s = singleLine(s)
	if ansi.StringWidth(s) <= maxLen {
		return s
	}
	return ansi.Truncate(s, maxLen, "…")
}

// Pad truncates or right-pads s with spaces to exactly width display columns.
func Pad(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = Truncate(s, width)
	n := ansi.StringWidth(s)
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}

// FormatDurationSecs converts seconds to M:SS or H:MM:SS format.
// Returns "--:--" when secs is zero, negative, NaN, or infinite.
func FormatDurationSecs(secs float64) string {
	if secs <= 0 || math.IsNaN(secs) || math.IsInf(secs, 0) {
		return "--:--"
	}
	total := int(secs)
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// FormatPositionSecs converts a duration in seconds to M:SS or H:MM:SS format.
// Unlike FormatDurationSecs, zero is treated as a valid position (start of
// playback) and returns "0:00". NaN and infinite values are clamped to zero.
func FormatPositionSecs(secs float64) string {
	if secs < 0 || math.IsNaN(secs) || math.IsInf(secs, 0) {
		secs = 0
	}
	total := int(secs)
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%d:%02d", minutes, seconds)
}

// FormatDurationMs converts milliseconds to M:SS or H:MM:SS format.
// Returns "--:--" when ms is zero or negative.
func FormatDurationMs(ms int64) string {
	if ms <= 0 {
		return "--:--"
	}
	return FormatDurationSecs(float64(ms) / 1000)
}

// PadLeft truncates or left-pads s with spaces to exactly width display columns.
func PadLeft(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = Truncate(s, width)
	n := ansi.StringWidth(s)
	if n < width {
		return strings.Repeat(" ", width-n) + s
	}
	return s
}

// FormatTrackPos returns the display string for a track's position column.
// When trackNum is non-positive (unknown/invalid), it falls back to the
// sequential fallback index.
// When discNum > 1, it formats as "D.TT" (e.g. "2.03" for disc 2, track 3).
// Otherwise it returns the bare track number.
func FormatTrackPos(trackNum, discNum, fallback int) string {
	if trackNum <= 0 {
		return strconv.Itoa(fallback)
	}
	if discNum > 1 {
		return fmt.Sprintf("%d.%02d", discNum, trackNum)
	}
	return strconv.Itoa(trackNum)
}

// FavPrefix returns s padded to maxWidth, with "★ " prepended when isFav,
// reducing the available text width by 2 to maintain column alignment.
func FavPrefix(s string, maxWidth int, isFav bool) string {
	if !isFav {
		return Pad(s, maxWidth)
	}
	if maxWidth <= 2 {
		return Pad("★", maxWidth)
	}
	return "★ " + Pad(s, maxWidth-2)
}

// CountColWidth is the fixed display width reserved for the right-aligned
// count zone in Browse views. Five digits covers libraries up to 99,999
// tracks per artist/album/genre — comfortably above any realistic value.
const CountColWidth = 5

// MinLabelWithCount is the smallest label-zone width that still feels usable
// next to a count column. Below this threshold (label + space + count zone),
// the count is dropped so the label keeps its full width.
const MinLabelWithCount = 6

// FormatCountSuffix returns the trailing count zone for a Browse list row:
// one leading space, followed by the count right-aligned in CountColWidth.
// When count <= 0 the digit area is rendered as blank spaces (no "0") so
// missing-count rows visually omit the value while still occupying the
// column width. The returned string has exactly CountColWidth+1 display
// columns. Counts wider than CountColWidth are shown as a capped overflow
// marker (e.g. "9999+") instead of a text ellipsis, keeping the column
// numeric and fixed-width.
func FormatCountSuffix(count int) string {
	if count <= 0 {
		return " " + strings.Repeat(" ", CountColWidth)
	}
	digits := strconv.Itoa(count)
	if ansi.StringWidth(digits) > CountColWidth {
		digits = strings.Repeat("9", CountColWidth-1) + "+"
	}
	return " " + PadLeft(digits, CountColWidth)
}

// RightPadWithCount shapes a Browse row with a right-aligned count.
// It returns the label portion padded to fit, and the trailing count portion
// so callers can dim-style the count separately from the label before
// composing the row.
//
// When count <= 0, the count zone is blank-padded (no "0" digit). When width
// is too narrow to host both a usable label and the count zone, the count
// drops out entirely (label is padded to the full width and countPart is "").
// Width must equal the total row width including any future count column.
func RightPadWithCount(label string, count int, width int) (string, string) {
	if width <= 0 {
		return "", ""
	}
	if width < MinLabelWithCount+1+CountColWidth {
		return Pad(label, width), ""
	}
	labelWidth := width - 1 - CountColWidth
	return Pad(label, labelWidth), FormatCountSuffix(count)
}

// DimRepeat returns a continuation marker when value repeats from the
// previous row, or value padded to width otherwise. A repeated cell becomes
// a "·" middle-dot glyph followed by spaces filling the column, signalling
// visual chunking when consecutive rows share metadata. The returned string
// has exactly width display columns; callers apply a dim style to repeated
// cells. Returns "" when width <= 0; treats an empty prev as no previous row
// so the first row of a run always renders as its full value.
func DimRepeat(value, prev string, width int) string {
	if width <= 0 {
		return ""
	}
	if prev == "" || value != prev {
		return Pad(value, width)
	}
	if width == 1 {
		return "·"
	}
	return "·" + strings.Repeat(" ", width-1)
}

// FormatTrackPosSimple returns the display string for a track's position column
// when all visible tracks are single-disc (no disc prefix needed).
func FormatTrackPosSimple(trackNum, fallback int) string {
	if trackNum <= 0 {
		return strconv.Itoa(fallback)
	}
	return strconv.Itoa(trackNum)
}

// RowMarker keeps focus/selection in the first column and playback in the
// second. A ringed dot identifies a focused selection without relying on color.
func RowMarker(isSelected, isFocused, isNowPlaying bool, nowPlayingIcon string) string {
	marker := " "
	switch {
	case isSelected && isFocused:
		marker = "◉"
	case isSelected:
		marker = "●"
	case isFocused:
		marker = "▎"
	}
	if !isNowPlaying {
		return marker + " "
	}
	icon := strings.TrimSpace(normalizeMarkerIcon(nowPlayingIcon))
	if ansi.StringWidth(icon) != 1 {
		icon = "♪"
	}
	return marker + icon
}

func normalizeMarkerIcon(icon string) string {
	icon = singleLine(icon)
	if strings.TrimSpace(icon) == "" {
		icon = "♪"
	}
	if ansi.StringWidth(icon) > 2 {
		icon = ansi.Truncate(icon, 2, "")
	}
	switch ansi.StringWidth(icon) {
	case 1:
		return icon + " "
	case 2:
		return icon
	default:
		return "♪ "
	}
}

// BuildFilterSeparator returns the separator string rendered between a section
// heading and its item list. leadingNewline=true is used when the separator
// appears below a heading; false when at the top level where the title bar
// already provides spacing.
//
// sortIndicator is a pre-formatted sort label (e.g. "Sort: Name ▲") or empty.
// filterActive is true while the user is typing in the filter bar.
// filterText is the committed filter query; filterView is the pre-rendered
// textinput string to display while editing; renderedHint is the
// already-styled placeholder string shown when filterActive is true but
// filterText is empty (e.g. st.Dim.Render(v.FilterHint)).
// width is used to truncate the filter line to fit the terminal.
func BuildFilterSeparator(sortIndicator string, filterActive bool, filterText, filterView, renderedHint string, width int, leadingNewline bool) string {
	sortIndicator = singleLine(sortIndicator)
	filterText = singleLine(filterText)
	filterView = singleLine(filterView)
	renderedHint = singleLine(renderedHint)

	prefix := sortPrefix(sortIndicator, leadingNewline)
	sep := ""
	if leadingNewline {
		sep = "\n"
	}
	if filterActive {
		return prefix + sep + clampLine("Filter: "+activeFilterBody(filterText, filterView, renderedHint), width) + "\n"
	}
	if filterText != "" {
		return prefix + sep + committedFilterLine(filterText, width) + "\n"
	}
	if leadingNewline {
		return prefix + "\n\n"
	}
	return prefix
}

// sortPrefix places the sort indicator on its own line, above the filter line
// when there is a heading above it and below it otherwise.
func sortPrefix(sortIndicator string, leadingNewline bool) string {
	if sortIndicator == "" {
		return ""
	}
	if leadingNewline {
		return "\n" + sortIndicator
	}
	return sortIndicator + "\n"
}

// activeFilterBody is what follows "Filter: " while the user is typing: the
// placeholder hint on an empty query, otherwise the live input view.
func activeFilterBody(filterText, filterView, renderedHint string) string {
	if filterText == "" && renderedHint != "" {
		return renderedHint
	}
	if filterView == "" {
		return filterText
	}
	return filterView
}

// committedFilterLine renders a committed query, eliding the query itself
// before falling back to a hard clamp of the whole line.
func committedFilterLine(filterText string, width int) string {
	ft := filterText
	maxLen := width - ansi.StringWidth("Filter: ")
	if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
		ft = ansi.Truncate(ft, maxLen, "…")
	}
	line := "Filter: " + ft
	if ansi.StringWidth(line) > width {
		return clampLine(line, width)
	}
	return line
}

func clampLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = singleLine(line)
	if ansi.StringWidth(line) > width {
		return ansi.Truncate(line, width, "")
	}
	return line
}

func singleLine(s string) string {
	if !strings.ContainsAny(s, "\r\n\t") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.ReplaceAll(s, "\t", " ")
}

// RowLabelWidthAndCount computes the label zone width and formatted count suffix
// for a single Browse list row. When counts is nil/empty or width is too narrow
// to fit both a usable label and the count column, it returns the full width
// and an empty count string (caller renders the full row as label-only).
func RowLabelWidthAndCount(width int, counts map[string]int, name string) (int, string) {
	if width <= 0 {
		return 0, ""
	}
	count := " " + FormatCount(counts[name]) + " " + Pluralize(counts[name], "track", "tracks")
	if len(counts) == 0 || width < MinLabelWithCount+ansi.StringWidth(count) {
		return width, ""
	}
	return width - ansi.StringWidth(count), count
}

// Pluralize returns singular when n == 1, plural otherwise.
func Pluralize(n int, singular, plural string) string {
	if n == 1 {
		return singular
	}
	return plural
}

// FormatCount formats n with comma separators (e.g. 1234 → "1,234").
func FormatCount(n int) string {
	raw := fmt.Sprintf("%d", n)
	sign := ""
	digits := raw
	if len(raw) > 0 && raw[0] == '-' {
		sign = "-"
		digits = raw[1:]
	}
	if len(digits) <= 3 {
		return sign + digits
	}
	offset := len(digits) % 3
	var result []byte
	for i := range len(digits) {
		if i > 0 && (i-offset)%3 == 0 {
			result = append(result, ',')
		}
		result = append(result, digits[i])
	}
	return sign + string(result)
}
