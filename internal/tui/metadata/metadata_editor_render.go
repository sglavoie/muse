package metadata

import (
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Render produces a centered box with six labelled, editable fields.
func (e Editor) Render(termWidth, termHeight int, s styles.Styles) string {
	if e.discard.Active {
		return e.discard.Render(termWidth, termHeight, s)
	}
	// Box width: use most of the terminal width, capped at 80.
	boxInner := max(min(termWidth-8, 80), overlay.MinInnerWidth)
	contentWidth := boxInner - overlay.HPadding
	valueWidth := contentWidth - maxLabelW - 1

	renderFields := e.renderFieldIndices()

	var lines []string

	// Top border.
	lines = append(lines, overlay.CornerTL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerTR)

	// Title row (centered).
	title := e.boxTitle()
	titlePad := max(0, boxInner-ansi.StringWidth(title))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, overlay.BorderV+strings.Repeat(" ", leftTP)+s.Heading.Render(title)+strings.Repeat(" ", rightTP)+overlay.BorderV)

	// Separator.
	lines = append(lines, overlay.CrossL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CrossR)

	// Keep the filename visible while editing metadata when there is a spare row.
	contextRows := 0
	if e.filename != "" && termHeight >= len(renderFields)+5+len(strings.Split(e.hintLine(), "\n")) {
		lines = append(lines, overlay.BorderV+s.Dim.Render(text.Pad("  File: "+e.filename, boxInner))+overlay.BorderV)
		contextRows = 1
	}

	// Field rows.
	lines = append(lines, e.fieldRowLines(renderFields, valueWidth, termHeight-contextRows, s)...)

	// Bottom border.
	lines = append(lines, overlay.CornerBL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerBR)

	// Hint.
	hints := strings.Split(e.hintLine(), "\n")
	if len(lines)+len(hints) > termHeight {
		changed := 0
		for _, i := range renderFields {
			if e.Dirty[i] {
				changed++
			}
		}
		hints = []string{"  tab fields · enter save · esc cancel", fmt.Sprintf("  ctrl+r reset · %d changed", changed)}
		if e.IsBulk() {
			hints[1] += " · ctrl+d clear"
		}
		if e.focused == FieldDuration {
			hints[1] = fmt.Sprintf("  s/m:ss/h:mm:ss · %d changed · ctrl+r reset", changed)
		}
	}
	lines = append(lines, hints...)

	return overlay.RenderTopLines(lines, "", termWidth)
}

// fieldLabels returns the field labels, marking every edited field that
// differs from its opening value with a leading "*".
func (e Editor) fieldLabels() [fieldCount]string {
	labels := [fieldCount]string{"Title:", "Artist:", "Album:", "Genre:", "Duration:", "Album Artist:"}
	for i := range fieldCount {
		if e.Dirty[i] {
			labels[i] = "*" + labels[i]
		}
	}
	return labels
}

// boxTitle returns the box's centered title row text.
func (e Editor) boxTitle() string {
	switch {
	case e.renameTitle != "":
		return e.renameTitle
	case e.IsBulk():
		return fmt.Sprintf(" ✎ Edit Metadata (%d tracks) ", e.trackCount())
	default:
		return " ✎ Edit Metadata "
	}
}

// renderFieldIndices returns which field indices this mode shows: every field
// by default, or the single renamed field in a rename mode.
func (e Editor) renderFieldIndices() []int {
	switch e.mode {
	case ModeRenameArtist:
		return []int{int(fieldArtist)}
	case ModeRenameAlbum:
		return []int{int(fieldAlbum)}
	default:
		return []int{int(fieldTitle), int(fieldArtist), int(FieldAlbumArtist), int(fieldAlbum), int(fieldGenre), int(FieldDuration)}
	}
}

// fieldRowLines renders one entry per visible field: the focused field as a
// live text input, the others as wrapped static values drawing on a shared
// continuation-line budget so the box still fits the terminal height.
func (e Editor) fieldRowLines(renderFields []int, valueWidth, termHeight int, s styles.Styles) []string {
	labels := e.fieldLabels()
	remainingContinuationLines := metadataEditorContinuationBudget(termHeight-strings.Count(e.hintLine(), "\n"), len(renderFields))

	var lines []string
	for _, i := range renderFields {
		if Field(i) == e.focused {
			row := e.focusedFieldRow(i, labels[i], valueWidth)
			lines = append(lines, overlay.BorderV+s.Accent.Render(row)+overlay.BorderV)
			continue
		}
		value := e.Fields[i].Value()
		mixed := e.mixed[i] && !e.Dirty[i] && value == ""
		if e.cleared[i] {
			value = e.clearPlaceholder()
		} else if mixed {
			value = mixedValuePlaceholder
		}
		wrapLines := metadataEditorValueLines(value, valueWidth, remainingContinuationLines)
		remainingContinuationLines -= max(len(wrapLines)-1, 0)
		if mixed {
			for j := range wrapLines {
				wrapLines[j] = s.Dim.Render(wrapLines[j])
			}
		}
		lines = append(lines, valueFieldRows(labels[i], wrapLines, valueWidth)...)
	}
	return lines
}

// focusedFieldRow renders the focused field's row content, resizing its text
// input to the box first.
func (e Editor) focusedFieldRow(i int, label string, valueWidth int) string {
	return fmt.Sprintf("  %*s %s  ", maxLabelW, label, inputWithOverflow(e.Fields[i], valueWidth))
}

// valueFieldRows renders a non-focused field: the label sits on the first row
// and any wrapped remainder continues under a blank label column.
func valueFieldRows(label string, wrapLines []string, valueWidth int) []string {
	rows := make([]string, 0, len(wrapLines))
	for j, wl := range wrapLines {
		if visLen := ansi.StringWidth(wl); visLen < valueWidth {
			wl += strings.Repeat(" ", valueWidth-visLen)
		}
		rowLabel := label
		if j > 0 {
			rowLabel = ""
		}
		rows = append(rows, overlay.BorderV+fmt.Sprintf("  %*s %s  ", maxLabelW, rowLabel, wl)+overlay.BorderV)
	}
	return rows
}

// hintLine returns the key hint printed under the box.
func (e Editor) hintLine() string {
	hint := "  tab/shift+tab fields · enter save · esc cancel"
	if e.mode != ModeDefault {
		hint = "  enter save · esc cancel"
	}
	if e.IsBulk() {
		hint += "\n  ctrl+d clear field · ctrl+r reset · * applies"
	} else {
		hint += "\n  ctrl+r reset field"
	}
	if e.focused == FieldDuration {
		if e.IsBulk() {
			hint = "  tab/shift+tab fields · enter save · esc cancel\n  ctrl+d clear · ctrl+r reset · s/m:ss/h:mm:ss"
		} else {
			hint += " · s/m:ss/h:mm:ss"
		}
	}
	changed := 0
	for _, i := range e.renderFieldIndices() {
		if e.Dirty[i] {
			changed++
		}
	}
	if changed == 1 {
		hint += "\n  * 1 field changed"
	} else if changed > 1 {
		hint += fmt.Sprintf("\n  * %d fields changed", changed)
	}
	return hint
}

func metadataEditorContinuationBudget(termHeight, fieldCount int) int {
	if termHeight <= 0 {
		return int(^uint(0) >> 1)
	}
	const chromeLines = 5 // top border, title, separator, bottom border, hint
	return max(termHeight-chromeLines-fieldCount, 0)
}

func metadataEditorValueLines(value string, width, continuationBudget int) []string {
	wrapped := ansi.Wrap(value, width, " ")
	lines := strings.Split(wrapped, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	// An unknown terminal height yields an effectively unbounded budget, so
	// compare against the line count before computing 1+budget: the sentinel
	// overflows to a negative bound and panics on the slice below.
	budget := max(continuationBudget, 0)
	if budget >= len(lines) {
		return lines
	}
	maxLines := 1 + budget
	if len(lines) > maxLines {
		tail := strings.Join(lines[maxLines-1:], " ")
		lines = lines[:maxLines]
		lines[maxLines-1] = text.Truncate(tail, width)
	}
	return lines
}

// WithAlbumArtists seeds the additional field, including mixed bulk values.
func (e Editor) WithAlbumArtists(values ...string) Editor {
	value := ""
	if len(values) > 0 {
		value = values[0]
	}
	for _, v := range values {
		if v != value {
			e.mixed[FieldAlbumArtist] = true
			break
		}
	}
	if e.mixed[FieldAlbumArtist] {
		value = ""
	}
	e.initialValues[FieldAlbumArtist] = value
	e.Fields[FieldAlbumArtist] = newField(value, false)
	if e.mixed[FieldAlbumArtist] {
		e.Fields[FieldAlbumArtist].Placeholder = mixedValuePlaceholder
	}
	return e
}

func (e Editor) AlbumArtist() string { return e.Fields[FieldAlbumArtist].Value() }

func (e Editor) DirtyAlbumArtist() *string {
	if !e.Dirty[FieldAlbumArtist] {
		return nil
	}
	value := strings.TrimSpace(e.AlbumArtist())
	return &value
}

func (e Editor) requestCancel() (Editor, Action, tea.Cmd) {
	if slices.Contains(e.Dirty[:], true) {
		e.discard = overlay.DiscardPrompt{Active: true}
		return e, actionNone, nil
	}
	return e, ActionCancel, nil
}
