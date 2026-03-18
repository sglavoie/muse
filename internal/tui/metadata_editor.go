package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/bubbles/v2/textinput"

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

// MetadataEditor is a fullscreen overlay for editing track metadata.
type MetadataEditor struct {
	hash    string
	fields  [metadataFieldCount]textinput.Model
	focused metadataField
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
		hash: hash,
		fields: [metadataFieldCount]textinput.Model{
			newField(title, true),
			newField(artist, false),
			newField(album, false),
		},
	}
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
		e.fields[e.focused].Blur()
		e.focused = (e.focused + 1) % metadataFieldCount
		return e, editorNone, e.fields[e.focused].Focus()
	case "shift+tab":
		e.fields[e.focused].Blur()
		e.focused = (e.focused + metadataFieldCount - 1) % metadataFieldCount
		return e, editorNone, e.fields[e.focused].Focus()
	}

	// Forward to the focused textinput.
	var cmd tea.Cmd
	e.fields[e.focused], cmd = e.fields[e.focused].Update(msg)
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
	const (
		borderH  = "─"
		borderV  = "│"
		cornerTL = "╭"
		cornerTR = "╮"
		cornerBL = "╰"
		cornerBR = "╯"
		crossL   = "├"
		crossR   = "┤"
	)

	labels := [metadataFieldCount]string{"Title:", "Artist:", "Album:"}
	maxLabelW := 7 // len("Artist:")

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

	var lines []string

	// Top border.
	lines = append(lines, cornerTL+strings.Repeat(borderH, boxInner)+cornerTR)

	// Title row (centered).
	titlePad := boxInner - len([]rune(title))
	leftTP := titlePad / 2
	rightTP := titlePad - leftTP
	lines = append(lines, borderV+strings.Repeat(" ", leftTP)+s.Heading.Render(title)+strings.Repeat(" ", rightTP)+borderV)

	// Separator.
	lines = append(lines, crossL+strings.Repeat(borderH, boxInner)+crossR)

	// Field rows.
	for i := range metadataFieldCount {
		var padded string
		if metadataField(i) == e.focused {
			// View() renders at Width+1 visible chars (cursor adds 1),
			// so subtract 1 to fit exactly in valueWidth.
			e.fields[i].SetWidth(valueWidth - 1)
			padded = e.fields[i].View()
		} else {
			value := e.fields[i].Value()
			visLen := len([]rune(value))
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
	lines = append(lines, "  tab/shift+tab fields · enter save · esc cancel")

	// Center the box.
	boxW := len([]rune(lines[0]))
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
