// Package strip renders the unified state strip shown above the column
// header in each primary list view (Library, Queue, Browse). The strip
// carries the view label, sort, filter, and counts on a single line so
// each view can drop its previous 2-line header.
package strip

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// Separator is the divider between adjacent left-zone chunks. It is
// exported so callers can reason about width arithmetic when needed.
const Separator = " · "

// Chevron is the divider between adjacent breadcrumb segments (e.g.
// "Artists › Brian Eno › Heroes"). Rendered in TextTertiary.
const Chevron = " › "

// SortChunk describes the active sort field on the strip. Label is empty
// when the view is at its default (no sort), in which case the chunk is
// not rendered. Indicator is the direction arrow rendered immediately
// after the label (e.g. "▲" or "▼"); it may be empty.
type SortChunk struct {
	Label     string
	Indicator string
}

// FilterChunk describes the filter portion of the strip. The chunk is
// not rendered when Active is false and Text is empty.
//
// While Active is true and Text is empty, Hint is shown in place of the
// value (already styled, e.g. via Dim.Render).
// While Active is true and Text is non-empty, InputView is rendered in
// place of the value (the textinput's pre-rendered output) so the cursor
// displays in place — no extra row is needed for the filter bar.
// When Active is false and Text is non-empty, Text is rendered as a
// committed filter value.
type FilterChunk struct {
	Active    bool
	Text      string
	InputView string
	Hint      string
}

// CountsChunk describes the right-zone counts on the strip. When
// Filtering is true the chunk renders "filtered/total"; otherwise it
// renders the bare total. A zero Total suppresses the chunk entirely.
type CountsChunk struct {
	Filtered  int
	Total     int
	Filtering bool
}

// SelectionChunk describes the optional "selected" chip rendered between
// the breadcrumb and the sort/filter chunks. When Count <= 0 the chip is
// suppressed. The chip is the *last* left-zone chunk to drop under width
// pressure (after counts, sort, and filter) because it anchors the user's
// working memory for an in-progress bulk action. Render accepts a nil
// pointer to mean "no selection chip".
type SelectionChunk struct {
	Count int
}

// Render assembles the single-line state strip and returns a string
// exactly width display columns wide (when width is positive).
//
// breadcrumb carries the view position as one or more crumbs (e.g.
// {"Artists"} for top-level lists, {"Artists", "Brian Eno"} when drilled
// into an artist). A single crumb behaves like the previous viewLabel; a
// multi-crumb path renders with " › " in TextTertiary between segments
// (labels in TextSecondary). An empty slice or zero width returns "".
//
// hint is an optional right-edge affordance (e.g. "esc ←") rendered
// dimmed after counts. It is the first chunk dropped under width
// pressure; remaining drops occur in order counts → sort → filter
// (truncated, then dropped) → selection (dropped last) → breadcrumb
// (last crumb truncated).
func Render(
	width int,
	breadcrumb []string,
	sort SortChunk,
	filter FilterChunk,
	counts CountsChunk,
	selection *SelectionChunk,
	hint string,
	s styles.Styles,
) string {
	if width <= 0 || len(breadcrumb) == 0 {
		return ""
	}

	hasSort := sort.Label != ""
	hasFilter := filter.Active || filter.Text != ""
	hasSelection := selection != nil && selection.Count > 0

	sortPlain := ""
	if hasSort {
		sortPlain = "sort " + sort.Label + sort.Indicator
	}

	filterValue := ""
	switch {
	case filter.Active && filter.Text == "":
		filterValue = filter.Hint
	case filter.Active && filter.Text != "":
		filterValue = filter.InputView
		if filterValue == "" {
			filterValue = filter.Text
		}
	case filter.Text != "":
		filterValue = filter.Text
	}
	const filterPrefix = `filter "`
	const filterSuffix = `"`

	countsPlain := ""
	switch {
	case counts.Total <= 0:
		// A zero total suppresses counts in all modes, including filtering.
		countsPlain = ""
	case counts.Filtering:
		countsPlain = fmt.Sprintf("%d/%d", counts.Filtered, counts.Total)
	default:
		countsPlain = fmt.Sprintf("%d", counts.Total)
	}

	selectionPlain := ""
	if hasSelection {
		selectionPlain = fmt.Sprintf("◉ %d selected", selection.Count)
	}

	sepW := ansi.StringWidth(Separator)
	chevronW := ansi.StringWidth(Chevron)
	sortW := ansi.StringWidth(sortPlain)
	filterPrefixW := ansi.StringWidth(filterPrefix)
	filterSuffixW := ansi.StringWidth(filterSuffix)
	filterValueW := ansi.StringWidth(filterValue)
	countsW := ansi.StringWidth(countsPlain)
	hintW := ansi.StringWidth(hint)
	selectionW := ansi.StringWidth(selectionPlain)

	includeSort := hasSort
	includeFilter := hasFilter
	includeSelection := hasSelection
	includeCounts := countsPlain != ""
	includeHint := hint != ""

	// crumbWidths tracks the per-crumb display width so we can compute
	// the total breadcrumb footprint without re-walking the slice.
	crumbWidths := make([]int, len(breadcrumb))
	for i, c := range breadcrumb {
		crumbWidths[i] = ansi.StringWidth(c)
	}
	breadcrumbW := func() int {
		w := 0
		for i, cw := range crumbWidths {
			if i > 0 {
				w += chevronW
			}
			w += cw
		}
		return w
	}

	leftWidth := func() int {
		w := breadcrumbW()
		if includeSort {
			w += sepW + sortW
		}
		if includeFilter {
			w += sepW + filterPrefixW + filterValueW + filterSuffixW
		}
		if includeSelection {
			w += sepW + selectionW
		}
		return w
	}
	// rightWidth covers counts and the optional hint plus the single
	// space separating them when both are present. Returns 0 when
	// neither is active.
	rightWidth := func() int {
		w := 0
		if includeCounts {
			w += countsW
		}
		if includeHint {
			if w > 0 {
				w++ // space between counts and hint
			}
			w += hintW
		}
		return w
	}
	// gap returns the spacing between the left and right zones. Two
	// spaces visually separate the "state" group (sort/filter/selection)
	// from the "meta" group (counts/hint) per the Proximity / Common
	// Region grouping; a single space is enough when only the breadcrumb
	// is on the left. Returns 0 when the right zone is empty.
	gap := func() int {
		if !(includeCounts || includeHint) {
			return 0
		}
		if includeSort || includeFilter || includeSelection {
			return 2
		}
		return 1
	}

	// Drop order on overflow: hint → counts → sort → filter (truncated
	// then dropped) → selection chip → breadcrumb (last crumb truncated).
	if includeHint && leftWidth()+gap()+rightWidth() > width {
		includeHint = false
		hintW = 0
	}
	if includeCounts && leftWidth()+gap()+rightWidth() > width {
		includeCounts = false
		countsW = 0
	}
	if includeSort && leftWidth()+gap()+rightWidth() > width {
		includeSort = false
	}
	if includeFilter && leftWidth()+gap()+rightWidth() > width {
		budget := width - breadcrumbW() - sepW - filterPrefixW - filterSuffixW - rightWidth() - gap()
		if includeSort {
			budget -= sepW + sortW
		}
		if includeSelection {
			budget -= sepW + selectionW
		}
		switch {
		case budget <= 0:
			includeFilter = false
		case budget < filterValueW:
			filterValue = ansi.Truncate(filterValue, budget, "…")
			filterValueW = ansi.StringWidth(filterValue)
		}
	}
	if includeSelection && leftWidth()+gap()+rightWidth() > width {
		includeSelection = false
		selectionW = 0
	}
	if leftWidth()+gap()+rightWidth() > width {
		// Breadcrumb itself overflows. Truncate the last crumb as a last
		// resort so the strip stays single-line; keep parent crumbs
		// intact (they convey position).
		room := width - gap() - rightWidth()
		if room < 1 {
			room = width
			includeCounts = false
			includeHint = false
			countsW = 0
			hintW = 0
		}
		fixed := 0
		for i := 0; i < len(breadcrumb)-1; i++ {
			fixed += crumbWidths[i] + chevronW
		}
		lastBudget := room - fixed
		if lastBudget < 1 {
			// Even the prefix path won't fit — truncate the whole joined
			// path as a single string.
			joined := strings.Join(breadcrumb, Chevron)
			breadcrumb = []string{ansi.Truncate(joined, room, "…")}
			crumbWidths = []int{ansi.StringWidth(breadcrumb[0])}
		} else if lastBudget < crumbWidths[len(crumbWidths)-1] {
			last := ansi.Truncate(breadcrumb[len(breadcrumb)-1], lastBudget, "…")
			breadcrumb[len(breadcrumb)-1] = last
			crumbWidths[len(crumbWidths)-1] = ansi.StringWidth(last)
		}
	}

	divider := s.Dim.Render(Separator)
	chevronStyled := s.Dim.Render(Chevron)

	var b strings.Builder
	for i, crumb := range breadcrumb {
		if i > 0 {
			b.WriteString(chevronStyled)
		}
		// In a multi-crumb path the final crumb carries the current
		// position and earns TextPrimary; parent crumbs stay at
		// TextSecondary as context. A single crumb is always
		// TextSecondary so the strip never visually "lights up" when the
		// view is quiet.
		style := s.TextSecondary
		if len(breadcrumb) > 1 && i == len(breadcrumb)-1 {
			style = s.TextPrimary
		}
		b.WriteString(style.Render(crumb))
	}
	if includeSort {
		b.WriteString(divider)
		b.WriteString(s.TextTertiary.Render("sort "))
		b.WriteString(s.TextPrimary.Render(sort.Label))
		if sort.Indicator != "" {
			b.WriteString(s.AccentMark.Render(sort.Indicator))
		}
	}
	if includeFilter {
		b.WriteString(divider)
		b.WriteString(s.TextTertiary.Render(filterPrefix))
		switch {
		case filter.Active && filter.Text != "":
			b.WriteString(filterValue) // textinput; preserve its own styling
		case filter.Active && filter.Text == "":
			b.WriteString(filterValue) // caller-styled hint (Zeigarnik)
		default:
			b.WriteString(s.TextPrimary.Render(filterValue))
		}
		b.WriteString(s.TextTertiary.Render(filterSuffix))
	}
	if includeSelection {
		b.WriteString(divider)
		// Combine fg+bg in a single style so the whole chip carries the
		// accent background. A nested `s.TextPrimary.Render("◉")` inside
		// `s.Accent.Render(...)` would emit an ANSI reset after the glyph
		// and drop the background mid-chip — see project_bug_hotspots.md
		// entry #8 ("Nested lipgloss Render calls kill outer background").
		chipStyle := s.Accent.Foreground(s.TextPrimary.GetForeground())
		b.WriteString(chipStyle.Render(fmt.Sprintf("◉ %d selected", selection.Count)))
	}
	leftStr := b.String()
	leftStyledW := ansi.StringWidth(leftStr)

	rightPlainW := rightWidth()
	if rightPlainW == 0 {
		pad := width - leftStyledW
		if pad < 0 {
			pad = 0
		}
		return leftStr + strings.Repeat(" ", pad)
	}

	var right strings.Builder
	if includeCounts {
		countsStyle := s.Dim
		if counts.Filtering {
			countsStyle = s.TextSecondary
		}
		right.WriteString(countsStyle.Render(countsPlain))
	}
	if includeHint {
		if includeCounts {
			right.WriteString(" ")
		}
		right.WriteString(renderKeyLabelPair(hint, s))
	}
	pad := width - leftStyledW - rightPlainW
	if pad < 0 {
		pad = 0
	}
	return leftStr + strings.Repeat(" ", pad) + right.String()
}

// renderKeyLabelPair styles a "key label" hint string by rendering the
// first whitespace-delimited token in HelpKey (bold + accent foreground)
// and the remainder in TextSecondary, joined by a single plain space.
// A hint with no space is treated as a key-only label. Width arithmetic
// upstream uses the unstyled string, so this function never changes
// display width.
func renderKeyLabelPair(hint string, s styles.Styles) string {
	if hint == "" {
		return ""
	}
	idx := strings.IndexByte(hint, ' ')
	if idx < 0 {
		return s.HelpKey.Render(hint)
	}
	key := hint[:idx]
	label := hint[idx+1:]
	if label == "" {
		return s.HelpKey.Render(key)
	}
	return s.HelpKey.Render(key) + " " + s.TextSecondary.Render(label)
}
