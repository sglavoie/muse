package tui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sglavoie/muse/internal/metaexport"
	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// MetadataImportOverlay confirms a parsed import before anything is written.
//
// It slices its own visible window rather than handing the whole plan to
// overlay.Box: overlay.Box.rowsForHeight drops rows from the middle to make a box
// fit, which is invisible on a fixed form and corrupts an ordered list.
type MetadataImportOverlay struct {
	path string
	plan metaexport.Plan
	// skipped is metaexport.File.Skipped: dropped data rows only, which is a
	// smaller number than len(plan.Warnings).
	skipped      int
	cursor       int
	offset       int
	showWarnings bool
}

func newMetadataImportOverlay(path string, plan metaexport.Plan, skipped int) MetadataImportOverlay {
	return MetadataImportOverlay{path: path, plan: plan, skipped: skipped}
}

// importFieldChange is one field of one row, ready to render as `old → new`.
type importFieldChange struct {
	badge string
	name  string
	old   string
	new   string
}

// importChangedFields lists the fields a row rewrites, in export column order.
// The badge letters are a=artist, t=title, b=album, g=genre, #=track, d=disc.
func importChangedFields(ch metaexport.Change) []importFieldChange {
	var out []importFieldChange
	if ch.Artist != nil {
		out = append(out, importFieldChange{"a", "artist", ch.Current.Artist, *ch.Artist})
	}
	if ch.Title != nil {
		out = append(out, importFieldChange{"t", "title", ch.Current.Title, *ch.Title})
	}
	if ch.Album != nil {
		out = append(out, importFieldChange{"b", "album", ch.Current.Album, *ch.Album})
	}
	if ch.Genre != nil {
		// Canonical on both sides, because that is what the library will store:
		// a genre meant literally as "Rock/Pop" becomes two of them.
		out = append(out, importFieldChange{"g", "genre",
			metaexport.CanonicalGenre(ch.Current.Genre), metaexport.CanonicalGenre(*ch.Genre)})
	}
	if ch.Track != nil {
		out = append(out, importFieldChange{"#", "trk", strconv.Itoa(ch.Current.Track), strconv.Itoa(*ch.Track)})
	}
	if ch.Disc != nil {
		out = append(out, importFieldChange{"d", "disc", strconv.Itoa(ch.Current.Disc), strconv.Itoa(*ch.Disc)})
	}
	return out
}

// metadataImportRowBudget is how many content rows fit at this height. It
// mirrors overlay.Box's own chrome accounting so that rowsForHeight has nothing
// left to drop.
func metadataImportRowBudget(termHeight int) int {
	if termHeight <= 0 {
		return 1 << 30
	}
	const chromeLines = 7 // two borders, title, rule, two padding rows, hint
	return max(termHeight-chromeLines, 1)
}

// canApply reports whether there is anything to write. The overlay still opens
// with an empty change list when the parse skipped rows or warned, but then it
// is a report: nothing may be applied from it.
func (o MetadataImportOverlay) canApply() bool {
	return len(o.plan.Changes) > 0
}

func (o MetadataImportOverlay) rowCount() int {
	if o.showWarnings {
		return len(o.plan.Warnings)
	}
	return len(o.plan.Changes)
}

// perPage is how many list entries the window holds, after the summary rows and
// the focused row's expansion have taken their share.
func (o MetadataImportOverlay) perPage(termHeight int) int {
	budget := metadataImportRowBudget(termHeight)
	if o.showWarnings {
		return max(budget, 1)
	}
	budget -= len(o.headerRows())
	if o.cursor >= 0 && o.cursor < len(o.plan.Changes) {
		budget -= len(importChangedFields(o.plan.Changes[o.cursor]))
	}
	return max(budget, 1)
}

// reclamp keeps the cursor inside the list and the offset around the cursor.
// Key handling calls it because Render cannot persist what it computes.
func (o MetadataImportOverlay) reclamp(termHeight int) MetadataImportOverlay {
	n := o.rowCount()
	if n == 0 {
		o.cursor, o.offset = 0, 0
		return o
	}
	o.cursor = clampPaletteInt(o.cursor, 0, n-1)
	o.offset = clampImportOffset(o.offset, o.cursor, o.perPage(termHeight), n)
	return o
}

func clampImportOffset(offset, cursor, per, n int) int {
	if per >= n {
		return 0
	}
	if offset > n-per {
		offset = n - per
	}
	if offset < 0 {
		offset = 0
	}
	if cursor < offset {
		offset = cursor
	}
	if cursor >= offset+per {
		offset = cursor - per + 1
	}
	return offset
}

func (o MetadataImportOverlay) Render(termWidth, termHeight int, s styles.Styles) string {
	title := " ♪ Import metadata "
	rows := o.changeRows(termHeight)
	if o.showWarnings {
		title = " ♪ Import warnings "
		rows = o.warningRows(termHeight)
	}
	// The truncation here is what keeps rowsForHeight a no-op; dropping a
	// trailing row is safe where dropping a middle one is not.
	if budget := metadataImportRowBudget(termHeight); len(rows) > budget {
		rows = rows[:budget]
	}
	return overlay.Box{
		Title:         title,
		Rows:          rows,
		Hint:          o.hint(),
		MaxInnerWidth: max(termWidth-4, overlay.HPadding),
	}.Render(termWidth, termHeight, s)
}

func (o MetadataImportOverlay) headerRows() []overlay.Row {
	p := o.plan
	rows := []overlay.Row{
		{Text: o.path},
		{Text: fmt.Sprintf("%d changed · %d unchanged · %d unknown · %d skipped",
			len(p.Changes), p.Unchanged, len(p.Unknown), o.skipped)},
		{Text: fmt.Sprintf("artist %d · title %d · album %d · genre %d · trk %d · disc %d",
			p.Counts.Artist, p.Counts.Title, p.Counts.Album, p.Counts.Genre, p.Counts.Track, p.Counts.Disc)},
	}
	if n := len(p.Unknown); n > 0 {
		rows = append(rows, overlay.Row{Text: fmt.Sprintf(
			"⚠ %d %s no longer in the library — re-export before importing",
			n, text.Pluralize(n, "hash", "hashes"))})
	}
	if o.skipped > 0 {
		rows = append(rows, overlay.Row{Text: fmt.Sprintf(
			"⚠ %d %s skipped and not imported — w to review",
			o.skipped, text.Pluralize(o.skipped, "row", "rows"))})
	}
	if p.Moved > 0 {
		rows = append(rows, overlay.Row{Text: fmt.Sprintf(
			"%d %s moved since the export · applied anyway", p.Moved, text.Pluralize(p.Moved, "file", "files"))})
	}
	return append(rows, overlay.Row{Blank: true})
}

func (o MetadataImportOverlay) changeRows(termHeight int) []overlay.Row {
	rows := o.headerRows()
	n := len(o.plan.Changes)
	if n == 0 {
		return append(rows, overlay.Row{Text: "Nothing to apply"})
	}
	per := o.perPage(termHeight)
	start := clampImportOffset(o.offset, o.cursor, per, n)
	for i := start; i < min(start+per, n); i++ {
		ch := o.plan.Changes[i]
		fields := importChangedFields(ch)
		rows = append(rows, overlay.Row{Text: importChangeLine(ch, fields), Styled: i == o.cursor})
		if i != o.cursor {
			continue
		}
		for _, f := range fields {
			rows = append(rows, overlay.Row{Text: fmt.Sprintf("    %-6s %s → %s",
				f.name, importValue(f.old), importValue(f.new))})
		}
	}
	return rows
}

func (o MetadataImportOverlay) warningRows(termHeight int) []overlay.Row {
	n := len(o.plan.Warnings)
	if n == 0 {
		return []overlay.Row{{Text: "No warnings"}}
	}
	per := o.perPage(termHeight)
	start := clampImportOffset(o.offset, o.cursor, per, n)
	rows := make([]overlay.Row, 0, per)
	for i := start; i < min(start+per, n); i++ {
		rows = append(rows, overlay.Row{Text: o.plan.Warnings[i].String(), Styled: i == o.cursor})
	}
	return rows
}

func (o MetadataImportOverlay) hint() string {
	tail := " · enter/y apply · esc cancel"
	if !o.canApply() {
		tail = " · esc close"
	}
	if o.showWarnings {
		return "j/k scroll · w back to changes" + tail
	}
	warnings := ""
	if n := len(o.plan.Warnings); n > 0 {
		warnings = fmt.Sprintf(" · w %d %s", n, text.Pluralize(n, "warning", "warnings"))
	}
	return "j/k scroll" + warnings + tail
}

func importChangeLine(ch metaexport.Change, fields []importFieldChange) string {
	badges := make([]string, 0, len(fields))
	for _, f := range fields {
		badges = append(badges, f.badge)
	}
	line := importIdentity(ch.Current.Artist, ch.Artist) + " — " +
		importIdentity(ch.Current.Title, ch.Title) + "  [" + strings.Join(badges, " ") + "]"
	if ch.PathMoved {
		line += " (moved)"
	}
	return line
}

// importIdentity labels a row with what the library holds now, falling back to
// the incoming value: the rip this feature exists for has an empty artist on
// every row, and "(empty) — (empty)" identifies nothing.
func importIdentity(current string, incoming *string) string {
	if current != "" {
		return current
	}
	if incoming != nil && *incoming != "" {
		return *incoming
	}
	return "(empty)"
}

func importValue(s string) string {
	if s == "" {
		return "(empty)"
	}
	return s
}
