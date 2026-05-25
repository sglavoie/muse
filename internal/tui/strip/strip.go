// Package strip renders the unified state strip shown above the column
// header in each primary list view (Library, Queue, Browse). The strip
// carries the view label, sort, filter, and counts on a single line so
// each view can drop its previous 2-line header.
package strip

import "github.com/sglavoie/muse/internal/tui/styles"

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

	sort.Label = plainLine(sort.Label)
	sort.Indicator = plainLine(sort.Indicator)
	filter.Text = plainLine(filter.Text)
	filter.InputView = singleLine(filter.InputView)
	filter.Hint = singleLine(filter.Hint)
	hint = plainHintLine(hint)

	breadcrumb = normalizeBreadcrumb(breadcrumb)
	if len(breadcrumb) == 0 {
		return ""
	}

	layout := newStripLayout(width, breadcrumb, sort, filter, counts, selection, hint)
	layout.applyOverflow()

	return renderStrip(layout, s)
}

// JoinStrip concatenates a rendered strip line and a content body with a
// newline separator. When stripLine is empty, body is returned unchanged.
func JoinStrip(stripLine, body string) string {
if stripLine == "" {
return body
}
return stripLine + "\n" + body
}
