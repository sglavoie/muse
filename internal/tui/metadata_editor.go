package tui

import (
	"fmt"
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
func NewMetadataEditor(hash, title, artist, album string) MetadataEditor {
	return MetadataEditor{
		hash:          hash,
		initialValues: [metadataFieldCount]string{title, artist, album},
		fields: [metadataFieldCount]textinput.Model{
			newField(title, true),
			newField(artist, false),
			newField(album, false),
		},
	}
}

// NewBulkMetadataEditor returns a MetadataEditor for editing multiple tracks.
// Common values are pre-populated; differing fields are left empty.
func NewBulkMetadataEditor(hashes []string, commonTitle, commonArtist, commonAlbum string) MetadataEditor {
	return MetadataEditor{
		hashes:        hashes,
		initialValues: [metadataFieldCount]string{commonTitle, commonArtist, commonAlbum},
		fields: [metadataFieldCount]textinput.Model{
			newField(commonTitle, true),
			newField(commonArtist, false),
			newField(commonAlbum, false),
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
		initialValues: [metadataFieldCount]string{"", artist, ""},
		fields: [metadataFieldCount]textinput.Model{
			newField("", false),
			newField(artist, true),
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
		initialValues: [metadataFieldCount]string{"", "", album},
		fields: [metadataFieldCount]textinput.Model{
			newField("", false),
			newField("", false),
			newField(album, true),
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
func (e MetadataEditor) DirtyFields() (title, artist, album *string) {
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

// Hash returns the track hash this editor was opened for.
func (e MetadataEditor) Hash() string { return e.hash }

// Render produces a centered box with three labelled, editable fields.
func (e MetadataEditor) Render(termWidth, termHeight int, s styles.Styles) string {
	labels := [metadataFieldCount]string{"Title:", "Artist:", "Album:"}
	if e.IsBulk() {
		for i := range metadataFieldCount {
			if e.dirty[i] {
				labels[i] = "*" + labels[i]
			}
		}
	}
	maxLabelW := 8 // len("*Artist:")

	// Box width: use most of the terminal width, capped at 80.
	boxInner := termWidth - 8
	if boxInner > 80 {
		boxInner = 80
	}
	if boxInner < OverlayMinInnerWidth {
		boxInner = OverlayMinInnerWidth
	}

	contentWidth := boxInner - OverlayHPadding
	valueWidth := contentWidth - maxLabelW - 1

	title := " ✎ Edit Metadata "
	if e.renameTitle != "" {
		title = e.renameTitle
	} else if e.IsBulk() {
		title = fmt.Sprintf(" ✎ Edit Metadata (%d tracks) ", e.TrackCount())
	}

	// Determine which field indices to render.
	renderFields := []int{int(fieldTitle), int(fieldArtist), int(fieldAlbum)}
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
		var padded string
		if metadataField(i) == e.focused {
			// View() renders at Width+1 visible chars (cursor adds 1),
			// so subtract 1 to fit exactly in valueWidth.
			e.fields[i].SetWidth(valueWidth - 1)
			padded = e.fields[i].View()
		} else {
			value := e.fields[i].Value()
			visLen := ansi.StringWidth(value)
			if visLen < valueWidth {
				padded = value + strings.Repeat(" ", valueWidth-visLen)
			} else {
				padded = value
			}
		}

		row := fmt.Sprintf("  %*s %s  ", maxLabelW, labels[i], padded)
		if metadataField(i) == e.focused {
			lines = append(lines, borderV+s.Accent.Render(row)+borderV)
		} else {
			lines = append(lines, borderV+row+borderV)
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
			anyDirty := e.dirty[0] || e.dirty[1] || e.dirty[2]
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
