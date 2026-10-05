package metadata

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/overlay"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Field identifies which field is focused in the metadata editor.
type Field int

const (
	fieldTitle Field = iota
	fieldArtist
	fieldAlbum
	fieldGenre
	FieldDuration
	fieldCount
)

// Action is the result of a key press in the metadata editor.
type Action int

const (
	actionNone   Action = iota
	ActionSave          // Enter was pressed
	ActionCancel        // Esc was pressed
)

// Mode controls which fields are shown and whether tab cycles between them.
type Mode int

const (
	ModeDefault      Mode = iota
	ModeRenameArtist      // single Artist field, bulk rename
	ModeRenameAlbum       // single Album field, bulk rename
)

// FormatDuration converts seconds to "M:SS" or "H:MM:SS" for display
// in the editor. Returns "" when secs is 0 (unknown/missing duration).
func FormatDuration(secs float64) string {
	if secs <= 0 {
		return ""
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

// ParseDuration parses "M:SS", "H:MM:SS", or "" back to seconds.
// Returns 0, nil for an empty string. Returns an error for invalid input.
func ParseDuration(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	parts := strings.Split(s, ":")
	// Seconds-per-unit for each field, most significant first: "M:SS" is
	// minutes and seconds; "H:MM:SS" adds hours in front.
	var multipliers []int
	switch len(parts) {
	case 2:
		multipliers = []int{60, 1}
	case 3:
		multipliers = []int{3600, 60, 1}
	default:
		return 0, fmt.Errorf("invalid duration %q", s)
	}

	total := 0
	for i, part := range parts {
		// Only the leading field is unbounded; the rest must stay below the
		// unit above them.
		n, ok := parseDurationField(part, i > 0)
		if !ok {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		total += n * multipliers[i]
	}
	return float64(total), nil
}

// parseDurationField parses one colon-separated duration field. It must be a
// non-negative integer, and at most 59 when bounded.
func parseDurationField(part string, bounded bool) (int, bool) {
	n, err := strconv.Atoi(part)
	if err != nil || n < 0 {
		return 0, false
	}
	if bounded && n > 59 {
		return 0, false
	}
	return n, true
}

// Editor is a fullscreen overlay for editing track metadata.
type Editor struct {
	hash          string
	hashes        []string // nil for single-track, populated for bulk
	Fields        [fieldCount]textinput.Model
	focused       Field
	initialValues [fieldCount]string // snapshot at open time
	Dirty         [fieldCount]bool   // changed input or an explicit bulk clear
	cleared       [fieldCount]bool   // explicit bulk clear, distinct from an untouched empty input
	mixed         [fieldCount]bool   // distinct opening values in a bulk field
	mode          Mode
	renameTitle   string // custom dialog title, e.g. " ✎ Rename Artist "
}

// newField creates a focused textinput with the given initial value.
func newField(value string, focused bool) textinput.Model {
	ti := textinput.New()
	ti.SetValue(value)
	ti.CursorEnd()
	ti.Prompt = ""
	ti.CharLimit = 0 // unlimited
	if focused {
		ti.Focus()
	} else {
		ti.Blur()
	}
	return ti
}

// NewEditor returns a MetadataEditor pre-populated with the given values.
func NewEditor(hash, title, artist, album, genre string, duration float64) Editor {
	dur := FormatDuration(duration)
	return Editor{
		hash:          hash,
		initialValues: [fieldCount]string{title, artist, album, genre, dur},
		Fields: [fieldCount]textinput.Model{
			newField(title, true),
			newField(artist, false),
			newField(album, false),
			newField(genre, false),
			newField(dur, false),
		},
	}
}

// NewBulkEditor returns a MetadataEditor for editing multiple tracks.
// Common values are pre-populated; differing fields are left empty.
func NewBulkEditor(hashes []string, commonTitle, commonArtist, commonAlbum, commonGenre, commonDuration string) Editor {
	return Editor{
		hashes:        hashes,
		initialValues: [fieldCount]string{commonTitle, commonArtist, commonAlbum, commonGenre, commonDuration},
		Fields: [fieldCount]textinput.Model{
			newField(commonTitle, true),
			newField(commonArtist, false),
			newField(commonAlbum, false),
			newField(commonGenre, false),
			newField(commonDuration, false),
		},
	}
}

const mixedValuePlaceholder = "Multiple values — unchanged"

// WithMixedValues marks bulk fields whose original values differ. Values are
// ordered title, artist, album, genre, duration, just like NewBulkEditor.
// The placeholder never becomes an input value or an edit to persist.
func (e Editor) WithMixedValues(values ...[]string) Editor {
	for i, group := range values {
		if i >= int(fieldCount) || len(group) == 0 {
			continue
		}
		for _, value := range group[1:] {
			if value != group[0] {
				e.mixed[i] = true
				e.Fields[i].Placeholder = mixedValuePlaceholder
				break
			}
		}
	}
	return e
}

// NewArtistRenameEditor returns a MetadataEditor that shows only the Artist
// field, pre-populated with artist. Saving applies the rename to all hashes.
func NewArtistRenameEditor(hashes []string, artist string) Editor {
	return Editor{
		mode:          ModeRenameArtist,
		renameTitle:   " ✎ Rename Artist ",
		hashes:        hashes,
		focused:       fieldArtist,
		initialValues: [fieldCount]string{"", artist, "", "", ""},
		Fields: [fieldCount]textinput.Model{
			newField("", false),
			newField(artist, true),
			newField("", false),
			newField("", false),
			newField("", false),
		},
	}
}

// NewAlbumRenameEditor returns a MetadataEditor that shows only the Album
// field, pre-populated with album. Saving applies the rename to all hashes.
func NewAlbumRenameEditor(hashes []string, album string) Editor {
	return Editor{
		mode:          ModeRenameAlbum,
		renameTitle:   " ✎ Rename Album ",
		hashes:        hashes,
		focused:       fieldAlbum,
		initialValues: [fieldCount]string{"", "", album, "", ""},
		Fields: [fieldCount]textinput.Model{
			newField("", false),
			newField("", false),
			newField(album, true),
			newField("", false),
			newField("", false),
		},
	}
}

// Mode returns the editor mode.
func (e Editor) Mode() Mode { return e.mode }

// IsBulk reports whether this editor is in bulk mode (multiple tracks).
func (e Editor) IsBulk() bool { return len(e.hashes) > 1 }

// trackCount returns the number of tracks being edited.
func (e Editor) trackCount() int { return len(e.hashes) }

// Hashes returns the hashes slice (nil for single-track editors).
func (e Editor) Hashes() []string { return e.hashes }

// DirtyFields returns pointers to the new values for fields that were modified.
// Non-dirty fields return nil.
func (e Editor) DirtyFields() (title, artist, album, genre, duration *string) {
	if e.Dirty[fieldTitle] {
		v := strings.TrimSpace(e.Fields[fieldTitle].Value())
		title = &v
	}
	if e.Dirty[fieldArtist] {
		v := strings.TrimSpace(e.Fields[fieldArtist].Value())
		artist = &v
	}
	if e.Dirty[fieldAlbum] {
		v := strings.TrimSpace(e.Fields[fieldAlbum].Value())
		album = &v
	}
	if e.Dirty[fieldGenre] {
		v := strings.TrimSpace(e.Fields[fieldGenre].Value())
		genre = &v
	}
	if e.Dirty[FieldDuration] {
		v := strings.TrimSpace(e.Fields[FieldDuration].Value())
		duration = &v
	}
	return
}

// HandleKey processes a single key press and returns the updated editor and
// the resulting action. Keys that the editor handles (tab, enter, esc) are
// intercepted; everything else is forwarded to the focused Bubbles textinput.
func (e Editor) HandleKey(msg tea.KeyPressMsg) (Editor, Action, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return e, ActionSave, nil
	case "esc":
		return e, ActionCancel, nil
	case "ctrl+r":
		e.setFieldCleared(false)
		return e, actionNone, nil
	case "ctrl+d":
		if e.IsBulk() {
			e.setFieldCleared(true)
			return e, actionNone, nil
		}
	case "tab":
		if e.mode != ModeDefault {
			return e, actionNone, nil
		}
		e.Fields[e.focused].Blur()
		e.focused = (e.focused + 1) % fieldCount
		return e, actionNone, e.Fields[e.focused].Focus()
	case "shift+tab":
		if e.mode != ModeDefault {
			return e, actionNone, nil
		}
		e.Fields[e.focused].Blur()
		e.focused = (e.focused + fieldCount - 1) % fieldCount
		return e, actionNone, e.Fields[e.focused].Focus()
	}

	// Forward to the focused textinput.
	var cmd tea.Cmd
	e.Fields[e.focused], cmd = e.Fields[e.focused].Update(msg)
	e.refreshFocusedEdit()
	return e, actionNone, cmd
}

// ForwardMsg routes a generic tea.Msg to the focused text input field.
// This enables paste and other multi-step textinput commands.
func (e Editor) ForwardMsg(msg tea.Msg) (Editor, tea.Cmd) {
	var cmd tea.Cmd
	e.Fields[e.focused], cmd = e.Fields[e.focused].Update(msg)
	e.refreshFocusedEdit()
	return e, cmd
}

// setFieldCleared either explicitly clears a bulk field or restores its opening value.
func (e *Editor) setFieldCleared(clearField bool) {
	value := e.initialValues[e.focused]
	if clearField {
		value = ""
	}
	e.Fields[e.focused].SetValue(value)
	e.cleared[e.focused] = clearField
	e.refreshFocusedEdit()
}

// refreshFocusedEdit keeps explicit clearing separate from ordinary input edits.
func (e *Editor) refreshFocusedEdit() {
	i := e.focused
	if e.Fields[i].Value() != "" {
		e.cleared[i] = false
	}
	e.Dirty[i] = e.cleared[i] || e.Fields[i].Value() != e.initialValues[i]
	e.Fields[i].Placeholder = ""
	if e.cleared[i] {
		e.Fields[i].Placeholder = e.clearPlaceholder()
	} else if e.mixed[i] {
		e.Fields[i].Placeholder = mixedValuePlaceholder
	}
}

func (e Editor) clearPlaceholder() string {
	return fmt.Sprintf("Will clear for %d tracks", e.trackCount())
}

// Title returns the current title value.
func (e Editor) Title() string { return e.Fields[fieldTitle].Value() }

// Artist returns the current artist value.
func (e Editor) Artist() string { return e.Fields[fieldArtist].Value() }

// Album returns the current album value.
func (e Editor) Album() string { return e.Fields[fieldAlbum].Value() }

// Genre returns the current genre value.
func (e Editor) Genre() string { return e.Fields[fieldGenre].Value() }

// Duration returns the current duration string value (e.g. "3:45").
func (e Editor) Duration() string { return e.Fields[FieldDuration].Value() }

// DurationChanged compares the displayed input to its opening value so an
// untouched whole-second display never overwrites a more precise duration.
func (e Editor) DurationChanged() bool {
	return strings.TrimSpace(e.Duration()) != e.initialValues[FieldDuration]
}

// Hash returns the track hash this editor was opened for.
func (e Editor) Hash() string { return e.hash }

// maxLabelW is the column width of the field labels: len("*Duration:").
const maxLabelW = 10

// Render produces a centered box with five labelled, editable fields.
func (e Editor) Render(termWidth, termHeight int, s styles.Styles) string {
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

	// Field rows.
	lines = append(lines, e.fieldRowLines(renderFields, valueWidth, termHeight, s)...)

	// Bottom border.
	lines = append(lines, overlay.CornerBL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerBR)

	// Hint.
	lines = append(lines, strings.Split(e.hintLine(), "\n")...)

	return centerBoxLines(lines, termWidth, termHeight)
}

// fieldLabels returns the field labels, marking every edited field that
// differs from its opening value with a leading "*".
func (e Editor) fieldLabels() [fieldCount]string {
	labels := [fieldCount]string{"Title:", "Artist:", "Album:", "Genre:", "Duration:"}
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
		return []int{int(fieldTitle), int(fieldArtist), int(fieldAlbum), int(fieldGenre), int(FieldDuration)}
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
	// View() renders at Width+1 visible chars (cursor adds 1),
	// so subtract 1 to fit exactly in valueWidth.
	e.Fields[i].SetWidth(valueWidth - 1)
	pos := e.Fields[i].Position()
	e.Fields[i].CursorEnd()    // force viewport recomputation with new width
	e.Fields[i].SetCursor(pos) // restore cursor; viewport is now correctly sized
	return fmt.Sprintf("  %*s %s  ", maxLabelW, label, e.Fields[i].View())
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

// centerBoxLines pads the assembled box so it sits centered in the terminal.
func centerBoxLines(lines []string, termWidth, termHeight int) string {
	boxW := ansi.StringWidth(lines[0])
	boxH := len(lines)

	leftPad := 0
	if termWidth > boxW {
		leftPad = (termWidth - boxW) / 2
	}
	topPad := 0
	if termHeight > boxH {
		topPad = (termHeight - boxH) / 2
	}

	prefix := strings.Repeat(" ", leftPad)
	var b strings.Builder
	for range topPad {
		b.WriteByte('\n')
	}
	for _, line := range lines {
		b.WriteString(prefix + line + "\n")
	}
	return b.String()
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
