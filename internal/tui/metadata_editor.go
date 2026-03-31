package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/styles"
)

// metadataField identifies which field is focused in the metadata editor.
type metadataField int

const (
	fieldTitle metadataField = iota
	fieldArtist
	fieldAlbum
	fieldGenre
	fieldDuration
	metadataFieldCount
)

// editorAction is the result of a key press in the metadata editor.
type editorAction int

const (
	editorNone   editorAction = iota
	editorSave                // Enter was pressed
	editorCancel              // Esc was pressed
)

// editorMode controls which fields are shown and whether tab cycles between them.
type editorMode int

const (
	editorModeDefault      editorMode = iota
	editorModeRenameArtist            // single Artist field, bulk rename
	editorModeRenameAlbum             // single Album field, bulk rename
)

// formatDurationForEditor converts seconds to "M:SS" or "H:MM:SS" for display
// in the editor. Returns "" when secs is 0 (unknown/missing duration).
func formatDurationForEditor(secs float64) string {
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

// parseDurationFromEditor parses "M:SS", "H:MM:SS", or "" back to seconds.
// Returns 0, nil for an empty string. Returns an error for invalid input.
func parseDurationFromEditor(s string) (float64, error) {
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

// MetadataEditor is a fullscreen overlay for editing track metadata.
type MetadataEditor struct {
	hash          string
	hashes        []string // nil for single-track, populated for bulk
	fields        [metadataFieldCount]textinput.Model
	focused       metadataField
	initialValues [metadataFieldCount]string // snapshot at open time
	dirty         [metadataFieldCount]bool   // true when field differs from initial
	mode          editorMode
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

// NewMetadataEditor returns a MetadataEditor pre-populated with the given values.
func NewMetadataEditor(hash, title, artist, album, genre string, duration float64) MetadataEditor {
	dur := formatDurationForEditor(duration)
	return MetadataEditor{
		hash:          hash,
		initialValues: [metadataFieldCount]string{title, artist, album, genre, dur},
		fields: [metadataFieldCount]textinput.Model{
			newField(title, true),
			newField(artist, false),
			newField(album, false),
			newField(genre, false),
			newField(dur, false),
		},
	}
}

// NewBulkMetadataEditor returns a MetadataEditor for editing multiple tracks.
// Common values are pre-populated; differing fields are left empty.
func NewBulkMetadataEditor(hashes []string, commonTitle, commonArtist, commonAlbum, commonGenre, commonDuration string) MetadataEditor {
	return MetadataEditor{
		hashes:        hashes,
		initialValues: [metadataFieldCount]string{commonTitle, commonArtist, commonAlbum, commonGenre, commonDuration},
		fields: [metadataFieldCount]textinput.Model{
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
func NewArtistRenameEditor(hashes []string, artist string) MetadataEditor {
	return MetadataEditor{
		mode:          editorModeRenameArtist,
		renameTitle:   " ✎ Rename Artist ",
		hashes:        hashes,
		focused:       fieldArtist,
		initialValues: [metadataFieldCount]string{"", artist, "", "", ""},
		fields: [metadataFieldCount]textinput.Model{
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
func NewAlbumRenameEditor(hashes []string, album string) MetadataEditor {
	return MetadataEditor{
		mode:          editorModeRenameAlbum,
		renameTitle:   " ✎ Rename Album ",
		hashes:        hashes,
		focused:       fieldAlbum,
		initialValues: [metadataFieldCount]string{"", "", album, "", ""},
		fields: [metadataFieldCount]textinput.Model{
			newField("", false),
			newField("", false),
			newField(album, true),
			newField("", false),
			newField("", false),
		},
	}
}

// Mode returns the editor mode.
func (e MetadataEditor) Mode() editorMode { return e.mode }

// IsBulk reports whether this editor is in bulk mode (multiple tracks).
func (e MetadataEditor) IsBulk() bool { return len(e.hashes) > 1 }

// TrackCount returns the number of tracks being edited.
func (e MetadataEditor) TrackCount() int { return len(e.hashes) }

// Hashes returns the hashes slice (nil for single-track editors).
func (e MetadataEditor) Hashes() []string { return e.hashes }

// DirtyFields returns pointers to the new values for fields that were modified.
// Non-dirty fields return nil.
func (e MetadataEditor) DirtyFields() (title, artist, album, genre, duration *string) {
	if e.dirty[fieldTitle] {
		v := strings.TrimSpace(e.fields[fieldTitle].Value())
		title = &v
	}
	if e.dirty[fieldArtist] {
		v := strings.TrimSpace(e.fields[fieldArtist].Value())
		artist = &v
	}
	if e.dirty[fieldAlbum] {
		v := strings.TrimSpace(e.fields[fieldAlbum].Value())
		album = &v
	}
	if e.dirty[fieldGenre] {
		v := strings.TrimSpace(e.fields[fieldGenre].Value())
		genre = &v
	}
	if e.dirty[fieldDuration] {
		v := strings.TrimSpace(e.fields[fieldDuration].Value())
		duration = &v
	}
	return
}

// HandleKey processes a single key press and returns the updated editor and
// the resulting action. Keys that the editor handles (tab, enter, esc) are
// intercepted; everything else is forwarded to the focused Bubbles textinput.
func (e MetadataEditor) HandleKey(msg tea.KeyPressMsg) (MetadataEditor, editorAction, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return e, editorSave, nil
	case "esc":
		return e, editorCancel, nil
	case "tab":
		if e.mode != editorModeDefault {
			return e, editorNone, nil
		}
		e.fields[e.focused].Blur()
		e.focused = (e.focused + 1) % metadataFieldCount
		return e, editorNone, e.fields[e.focused].Focus()
	case "shift+tab":
		if e.mode != editorModeDefault {
			return e, editorNone, nil
		}
		e.fields[e.focused].Blur()
		e.focused = (e.focused + metadataFieldCount - 1) % metadataFieldCount
		return e, editorNone, e.fields[e.focused].Focus()
	}

	// Forward to the focused textinput.
	var cmd tea.Cmd
	e.fields[e.focused], cmd = e.fields[e.focused].Update(msg)
	e.dirty[e.focused] = e.fields[e.focused].Value() != e.initialValues[e.focused]
	return e, editorNone, cmd
}

// Title returns the current title value.
func (e MetadataEditor) Title() string { return e.fields[fieldTitle].Value() }

// Artist returns the current artist value.
func (e MetadataEditor) Artist() string { return e.fields[fieldArtist].Value() }

// Album returns the current album value.
func (e MetadataEditor) Album() string { return e.fields[fieldAlbum].Value() }

// Genre returns the current genre value.
func (e MetadataEditor) Genre() string { return e.fields[fieldGenre].Value() }

// Duration returns the current duration string value (e.g. "3:45").
func (e MetadataEditor) Duration() string { return e.fields[fieldDuration].Value() }

// Hash returns the track hash this editor was opened for.
func (e MetadataEditor) Hash() string { return e.hash }

// Render produces a centered box with five labelled, editable fields.
func (e MetadataEditor) Render(termWidth, termHeight int, s styles.Styles) string {
	labels := [metadataFieldCount]string{"Title:", "Artist:", "Album:", "Genre:", "Duration:"}
	if e.IsBulk() {
		for i := range metadataFieldCount {
			if e.dirty[i] {
				labels[i] = "*" + labels[i]
			}
		}
	}
	maxLabelW := 10 // len("*Duration:")

	// Box width: use most of the terminal width, capped at 80.
	boxInner := max(min(termWidth-8, 80), OverlayMinInnerWidth)

	contentWidth := boxInner - OverlayHPadding
	valueWidth := contentWidth - maxLabelW - 1

	title := " ✎ Edit Metadata "
	if e.renameTitle != "" {
		title = e.renameTitle
	} else if e.IsBulk() {
		title = fmt.Sprintf(" ✎ Edit Metadata (%d tracks) ", e.TrackCount())
	}

	// Determine which field indices to render.
	renderFields := []int{int(fieldTitle), int(fieldArtist), int(fieldAlbum), int(fieldGenre), int(fieldDuration)}
	switch e.mode {
	case editorModeRenameArtist:
		renderFields = []int{int(fieldArtist)}
	case editorModeRenameAlbum:
		renderFields = []int{int(fieldAlbum)}
	}

	var lines []string

	// Top border.
	lines = append(lines, cornerTL+strings.Repeat(borderH, boxInner)+cornerTR)

	// Title row (centered).
	titlePad := max(0, boxInner-ansi.StringWidth(title))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, borderV+strings.Repeat(" ", leftTP)+s.Heading.Render(title)+strings.Repeat(" ", rightTP)+borderV)

	// Separator.
	lines = append(lines, crossL+strings.Repeat(borderH, boxInner)+crossR)

	// Field rows.
	for _, i := range renderFields {
		if metadataField(i) == e.focused {
			// View() renders at Width+1 visible chars (cursor adds 1),
			// so subtract 1 to fit exactly in valueWidth.
			e.fields[i].SetWidth(valueWidth - 1)
			pos := e.fields[i].Position()
			e.fields[i].CursorEnd()    // force viewport recomputation with new width
			e.fields[i].SetCursor(pos) // restore cursor; viewport is now correctly sized
			padded := e.fields[i].View()
			row := fmt.Sprintf("  %*s %s  ", maxLabelW, labels[i], padded)
			lines = append(lines, borderV+s.Accent.Render(row)+borderV)
		} else {
			value := e.fields[i].Value()
			wrapped := ansi.Wrap(value, valueWidth, " ")
			wrapLines := strings.Split(wrapped, "\n")
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
				lines = append(lines, borderV+row+borderV)
			}
		}
	}

	// Bottom border.
	lines = append(lines, cornerBL+strings.Repeat(borderH, boxInner)+cornerBR)

	// Hint.
	var hint string
	if e.mode != editorModeDefault {
		hint = "  enter save · esc cancel"
	} else {
		hint = "  tab/shift+tab fields · enter save · esc cancel"
		if e.IsBulk() {
			anyDirty := e.dirty[0] || e.dirty[1] || e.dirty[2] || e.dirty[3] || e.dirty[4]
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
