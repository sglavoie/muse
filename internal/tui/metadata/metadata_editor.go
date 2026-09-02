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
	FieldTitle Field = iota
	FieldArtist
	FieldAlbum
	FieldGenre
	FieldDuration
	FieldCount
)

// Action is the result of a key press in the metadata editor.
type Action int

const (
	ActionNone   Action = iota
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
	switch len(parts) {
	case 2:
		m, err := strconv.Atoi(parts[0])
		if err != nil || m < 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		sec, err := strconv.Atoi(parts[1])
		if err != nil || sec < 0 || sec > 59 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return float64(m*60 + sec), nil
	case 3:
		h, err := strconv.Atoi(parts[0])
		if err != nil || h < 0 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		m, err := strconv.Atoi(parts[1])
		if err != nil || m < 0 || m > 59 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		sec, err := strconv.Atoi(parts[2])
		if err != nil || sec < 0 || sec > 59 {
			return 0, fmt.Errorf("invalid duration %q", s)
		}
		return float64(h*3600 + m*60 + sec), nil
	default:
		return 0, fmt.Errorf("invalid duration %q", s)
	}
}

// Editor is a fullscreen overlay for editing track metadata.
type Editor struct {
	hash          string
	hashes        []string // nil for single-track, populated for bulk
	Fields        [FieldCount]textinput.Model
	focused       Field
	initialValues [FieldCount]string // snapshot at open time
	Dirty         [FieldCount]bool   // true when field differs from initial
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
		initialValues: [FieldCount]string{title, artist, album, genre, dur},
		Fields: [FieldCount]textinput.Model{
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
		initialValues: [FieldCount]string{commonTitle, commonArtist, commonAlbum, commonGenre, commonDuration},
		Fields: [FieldCount]textinput.Model{
			newField(commonTitle, true),
			newField(commonArtist, false),
			newField(commonAlbum, false),
			newField(commonGenre, false),
			newField(commonDuration, false),
		},
	}
}

// NewArtistRenameEditor returns a MetadataEditor that shows only the Artist
// field, pre-populated with artist. Saving applies the rename to all hashes.
func NewArtistRenameEditor(hashes []string, artist string) Editor {
	return Editor{
		mode:          ModeRenameArtist,
		renameTitle:   " ✎ Rename Artist ",
		hashes:        hashes,
		focused:       FieldArtist,
		initialValues: [FieldCount]string{"", artist, "", "", ""},
		Fields: [FieldCount]textinput.Model{
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
		focused:       FieldAlbum,
		initialValues: [FieldCount]string{"", "", album, "", ""},
		Fields: [FieldCount]textinput.Model{
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

// TrackCount returns the number of tracks being edited.
func (e Editor) TrackCount() int { return len(e.hashes) }

// Hashes returns the hashes slice (nil for single-track editors).
func (e Editor) Hashes() []string { return e.hashes }

// DirtyFields returns pointers to the new values for fields that were modified.
// Non-dirty fields return nil.
func (e Editor) DirtyFields() (title, artist, album, genre, duration *string) {
	if e.Dirty[FieldTitle] {
		v := strings.TrimSpace(e.Fields[FieldTitle].Value())
		title = &v
	}
	if e.Dirty[FieldArtist] {
		v := strings.TrimSpace(e.Fields[FieldArtist].Value())
		artist = &v
	}
	if e.Dirty[FieldAlbum] {
		v := strings.TrimSpace(e.Fields[FieldAlbum].Value())
		album = &v
	}
	if e.Dirty[FieldGenre] {
		v := strings.TrimSpace(e.Fields[FieldGenre].Value())
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
	case "tab":
		if e.mode != ModeDefault {
			return e, ActionNone, nil
		}
		e.Fields[e.focused].Blur()
		e.focused = (e.focused + 1) % FieldCount
		return e, ActionNone, e.Fields[e.focused].Focus()
	case "shift+tab":
		if e.mode != ModeDefault {
			return e, ActionNone, nil
		}
		e.Fields[e.focused].Blur()
		e.focused = (e.focused + FieldCount - 1) % FieldCount
		return e, ActionNone, e.Fields[e.focused].Focus()
	}

	// Forward to the focused textinput.
	var cmd tea.Cmd
	e.Fields[e.focused], cmd = e.Fields[e.focused].Update(msg)
	e.Dirty[e.focused] = e.Fields[e.focused].Value() != e.initialValues[e.focused]
	return e, ActionNone, cmd
}

// ForwardMsg routes a generic tea.Msg to the focused text input field.
// This enables paste and other multi-step textinput commands.
func (e Editor) ForwardMsg(msg tea.Msg) (Editor, tea.Cmd) {
	var cmd tea.Cmd
	e.Fields[e.focused], cmd = e.Fields[e.focused].Update(msg)
	e.Dirty[e.focused] = e.Fields[e.focused].Value() != e.initialValues[e.focused]
	return e, cmd
}

// Title returns the current title value.
func (e Editor) Title() string { return e.Fields[FieldTitle].Value() }

// Artist returns the current artist value.
func (e Editor) Artist() string { return e.Fields[FieldArtist].Value() }

// Album returns the current album value.
func (e Editor) Album() string { return e.Fields[FieldAlbum].Value() }

// Genre returns the current genre value.
func (e Editor) Genre() string { return e.Fields[FieldGenre].Value() }

// Duration returns the current duration string value (e.g. "3:45").
func (e Editor) Duration() string { return e.Fields[FieldDuration].Value() }

// Hash returns the track hash this editor was opened for.
func (e Editor) Hash() string { return e.hash }

// Render produces a centered box with five labelled, editable fields.
func (e Editor) Render(termWidth, termHeight int, s styles.Styles) string {
	labels := [FieldCount]string{"Title:", "Artist:", "Album:", "Genre:", "Duration:"}
	if e.IsBulk() {
		for i := range FieldCount {
			if e.Dirty[i] {
				labels[i] = "*" + labels[i]
			}
		}
	}
	maxLabelW := 10 // len("*Duration:")

	// Box width: use most of the terminal width, capped at 80.
	boxInner := max(min(termWidth-8, 80), overlay.MinInnerWidth)

	contentWidth := boxInner - overlay.HPadding
	valueWidth := contentWidth - maxLabelW - 1

	title := " ✎ Edit Metadata "
	if e.renameTitle != "" {
		title = e.renameTitle
	} else if e.IsBulk() {
		title = fmt.Sprintf(" ✎ Edit Metadata (%d tracks) ", e.TrackCount())
	}

	// Determine which field indices to render.
	renderFields := []int{int(FieldTitle), int(FieldArtist), int(FieldAlbum), int(FieldGenre), int(FieldDuration)}
	switch e.mode {
	case ModeRenameArtist:
		renderFields = []int{int(FieldArtist)}
	case ModeRenameAlbum:
		renderFields = []int{int(FieldAlbum)}
	}

	var lines []string

	// Top border.
	lines = append(lines, overlay.CornerTL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerTR)

	// Title row (centered).
	titlePad := max(0, boxInner-ansi.StringWidth(title))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, overlay.BorderV+strings.Repeat(" ", leftTP)+s.Heading.Render(title)+strings.Repeat(" ", rightTP)+overlay.BorderV)

	// Separator.
	lines = append(lines, overlay.CrossL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CrossR)

	// Field rows.
	remainingContinuationLines := metadataEditorContinuationBudget(termHeight, len(renderFields))
	for _, i := range renderFields {
		if Field(i) == e.focused {
			// View() renders at Width+1 visible chars (cursor adds 1),
			// so subtract 1 to fit exactly in valueWidth.
			e.Fields[i].SetWidth(valueWidth - 1)
			pos := e.Fields[i].Position()
			e.Fields[i].CursorEnd()    // force viewport recomputation with new width
			e.Fields[i].SetCursor(pos) // restore cursor; viewport is now correctly sized
			padded := e.Fields[i].View()
			row := fmt.Sprintf("  %*s %s  ", maxLabelW, labels[i], padded)
			lines = append(lines, overlay.BorderV+s.Accent.Render(row)+overlay.BorderV)
		} else {
			value := e.Fields[i].Value()
			wrapLines := metadataEditorValueLines(value, valueWidth, remainingContinuationLines)
			remainingContinuationLines -= max(len(wrapLines)-1, 0)
			for j, wl := range wrapLines {
				visLen := ansi.StringWidth(wl)
				if visLen < valueWidth {
					wl += strings.Repeat(" ", valueWidth-visLen)
				}
				var row string
				if j == 0 {
					row = fmt.Sprintf("  %*s %s  ", maxLabelW, labels[i], wl)
				} else {
					row = fmt.Sprintf("  %*s %s  ", maxLabelW, "", wl)
				}
				lines = append(lines, overlay.BorderV+row+overlay.BorderV)
			}
		}
	}

	// Bottom border.
	lines = append(lines, overlay.CornerBL+strings.Repeat(overlay.BorderH, boxInner)+overlay.CornerBR)

	// Hint.
	var hint string
	if e.mode != ModeDefault {
		hint = "  enter save · esc cancel"
	} else {
		hint = "  tab/shift+tab fields · enter save · esc cancel"
		if e.IsBulk() {
			anyDirty := e.Dirty[0] || e.Dirty[1] || e.Dirty[2] || e.Dirty[3] || e.Dirty[4]
			if anyDirty {
				hint += " · * = will be applied"
			}
		}
	}
	lines = append(lines, hint)

	// Center the box.
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
	maxLines := 1 + max(continuationBudget, 0)
	if len(lines) > maxLines {
		tail := strings.Join(lines[maxLines-1:], " ")
		lines = lines[:maxLines]
		lines[maxLines-1] = text.Truncate(tail, width)
	}
	return lines
}
