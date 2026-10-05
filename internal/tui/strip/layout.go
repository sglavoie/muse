package strip

import (
	"fmt"

	"github.com/charmbracelet/x/ansi"
)

const (
	filterPrefix = `filter "`
	filterSuffix = `"`
)

type stripLayout struct {
	width       int
	breadcrumb  []string
	crumbWidths []int

	sort      SortChunk
	filter    FilterChunk
	counts    CountsChunk
	selection *SelectionChunk
	hint      string

	sortPlain         string
	filterValue       string
	countsPlain       string
	selectionPlain    string
	selectionDuration string

	sepW          int
	chevronW      int
	sortW         int
	filterPrefixW int
	filterSuffixW int
	filterValueW  int
	countsW       int
	hintW         int
	selectionW    int

	includeSort      bool
	includeFilter    bool
	includeSelection bool
	includeCounts    bool
	includeHint      bool
}

func newStripLayout(
	width int,
	breadcrumb []string,
	sort SortChunk,
	filter FilterChunk,
	counts CountsChunk,
	selection *SelectionChunk,
	hint string,
) *stripLayout {
	layout := &stripLayout{
		width:      width,
		breadcrumb: breadcrumb,
		sort:       sort,
		filter:     filter,
		counts:     counts,
		selection:  selection,
		hint:       hint,

		includeSort:      sort.Label != "",
		includeFilter:    filter.Active || filter.Text != "",
		includeSelection: selection != nil && selection.Count > 0,
	}

	if layout.includeSort {
		layout.sortPlain = "sort " + sort.Label + sort.Indicator
	}
	layout.filterValue = buildFilterValue(filter)
	layout.countsPlain = buildCountsPlain(counts)
	if layout.includeSelection {
		layout.selectionPlain = fmt.Sprintf("◉ %d selected", selection.Count)
		layout.selectionDuration = plainLine(selection.Duration)
		if layout.selectionDuration != "" {
			layout.selectionPlain += Separator + layout.selectionDuration
		}
	}

	layout.sepW = ansi.StringWidth(Separator)
	layout.chevronW = ansi.StringWidth(Chevron)
	layout.sortW = ansi.StringWidth(layout.sortPlain)
	layout.filterPrefixW = ansi.StringWidth(filterPrefix)
	layout.filterSuffixW = ansi.StringWidth(filterSuffix)
	layout.filterValueW = ansi.StringWidth(layout.filterValue)
	layout.countsW = ansi.StringWidth(layout.countsPlain)
	layout.hintW = ansi.StringWidth(hint)
	layout.selectionW = ansi.StringWidth(layout.selectionPlain)
	layout.includeCounts = layout.countsPlain != ""
	layout.includeHint = hint != ""
	layout.crumbWidths = make([]int, len(breadcrumb))
	for i, crumb := range breadcrumb {
		layout.crumbWidths[i] = ansi.StringWidth(crumb)
	}

	return layout
}

func buildFilterValue(filter FilterChunk) string {
	switch {
	case filter.Active && filter.Text == "":
		if filter.Hint != "" {
			return filter.Hint
		}
		return filter.InputView
	case filter.Active && filter.Text != "":
		if filter.InputView != "" {
			return filter.InputView
		}
		return filter.Text
	case filter.Text != "":
		return filter.Text
	default:
		return ""
	}
}

func buildCountsPlain(counts CountsChunk) string {
	switch {
	case counts.Total <= 0:
		return ""
	case counts.Filtering:
		filtered := counts.Filtered
		if filtered < 0 {
			filtered = 0
		}
		if filtered > counts.Total {
			filtered = counts.Total
		}
		return fmt.Sprintf("%d/%d", filtered, counts.Total)
	default:
		return fmt.Sprintf("%d", counts.Total)
	}
}

func (l *stripLayout) totalWidth() int {
	return l.leftWidth() + l.gap() + l.rightWidth()
}

func (l *stripLayout) breadcrumbWidth() int {
	w := 0
	for i, crumbWidth := range l.crumbWidths {
		if i > 0 {
			w += l.chevronW
		}
		w += crumbWidth
	}
	return w
}

func (l *stripLayout) leftWidth() int {
	w := l.breadcrumbWidth()
	if l.includeSort {
		w += l.sepW + l.sortW
	}
	if l.includeFilter {
		w += l.sepW + l.filterPrefixW + l.filterValueW + l.filterSuffixW
	}
	if l.includeSelection {
		w += l.sepW + l.selectionW
	}
	return w
}

func (l *stripLayout) rightWidth() int {
	w := 0
	if l.includeCounts {
		w += l.countsW
	}
	if l.includeHint {
		if w > 0 {
			w++
		}
		w += l.hintW
	}
	return w
}

func (l *stripLayout) gap() int {
	if !(l.includeCounts || l.includeHint) {
		return 0
	}
	if l.includeSort || l.includeFilter || l.includeSelection {
		return 2
	}
	return 1
}

// applyOverflow surrenders chunks until the strip fits, in the order the hint
// is least missed and the breadcrumb most: selection duration, hint, counts, sort, filter (which
// truncates before it disappears), then selection. The breadcrumb is never
// dropped, only truncated, and only once nothing else is left to give.
func (l *stripLayout) applyOverflow() {
	l.dropUntilFits(l.dropSelectionDuration, l.dropHint, l.dropCounts, l.dropSort, l.fitFilter, l.dropSelection)
	if l.totalWidth() > l.width {
		l.fitBreadcrumb()
	}
}

// dropUntilFits runs each step in turn, stopping as soon as the strip fits.
// Every step is a no-op for a chunk that is already absent, so a step is never
// skipped on account of its chunk.
func (l *stripLayout) dropUntilFits(steps ...func()) {
	for _, step := range steps {
		if l.totalWidth() <= l.width {
			return
		}
		step()
	}
}

func (l *stripLayout) dropHint() {
	l.includeHint = false
	l.hintW = 0
}

func (l *stripLayout) dropSelectionDuration() {
	if l.selectionDuration == "" {
		return
	}
	l.selectionDuration = ""
	l.selectionPlain = fmt.Sprintf("◉ %d selected", l.selection.Count)
	l.selectionW = ansi.StringWidth(l.selectionPlain)
}

func (l *stripLayout) dropCounts() {
	l.includeCounts = false
	l.countsW = 0
}

func (l *stripLayout) dropSort() {
	l.includeSort = false
	l.sortW = 0
}

func (l *stripLayout) dropSelection() {
	l.includeSelection = false
	l.selectionW = 0
}

func (l *stripLayout) fitFilter() {
	budget := l.width - l.breadcrumbWidth() - l.sepW - l.filterPrefixW - l.filterSuffixW - l.rightWidth() - l.gap()
	if l.includeSort {
		budget -= l.sepW + l.sortW
	}
	if l.includeSelection {
		budget -= l.sepW + l.selectionW
	}

	switch {
	case budget <= 0:
		l.includeFilter = false
	case budget < l.filterValueW:
		l.filterValue = ansi.Truncate(l.filterValue, budget, "…")
		l.filterValueW = ansi.StringWidth(l.filterValue)
	}
}

func (l *stripLayout) fitBreadcrumb() {
	room := l.width - l.gap() - l.rightWidth()
	if room < 1 {
		room = l.width
		l.includeCounts = false
		l.includeHint = false
		l.countsW = 0
		l.hintW = 0
	}

	fixed := 0
	for i := 0; i < len(l.breadcrumb)-1; i++ {
		fixed += l.crumbWidths[i] + l.chevronW
	}

	lastBudget := room - fixed
	if lastBudget < 1 {
		last := l.breadcrumb[len(l.breadcrumb)-1]
		if ansi.StringWidth(last) > room {
			last = ansi.Truncate(last, room, "…")
		}
		l.breadcrumb = []string{last}
		l.crumbWidths = []int{ansi.StringWidth(last)}
		return
	}

	lastIndex := len(l.crumbWidths) - 1
	if lastBudget < l.crumbWidths[lastIndex] {
		last := ansi.Truncate(l.breadcrumb[lastIndex], lastBudget, "…")
		bc := make([]string, len(l.breadcrumb))
		copy(bc, l.breadcrumb)
		bc[lastIndex] = last
		l.breadcrumb = bc
		l.crumbWidths[lastIndex] = ansi.StringWidth(last)
	}
}
