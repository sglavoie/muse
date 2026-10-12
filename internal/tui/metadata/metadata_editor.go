package metadata

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/tui/overlay"
)

// Field identifies which field is focused in the metadata editor.
type Field int

const (
	fieldTitle Field = iota
	fieldArtist
	fieldAlbum
	fieldGenre
	FieldDuration
	FieldAlbumArtist
	fieldCount
)

// Action is the result of a key press in the metadata editor.
type Action int

const (
	actionNone   Action = iota
	ActionSave          // Enter was pressed
	ActionCancel        // unchanged form closed or edited draft explicitly discarded
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

// ParseDuration accepts the same time formats as seeking, plus an empty value
// for unknown duration.
func ParseDuration(s string) (float64, error) {
	if strings.TrimSpace(s) == "" {
		return 0, nil
	}
	return overlay.ParseSeekTime(s)
}

// Editor is a fullscreen overlay for editing track metadata.
type Editor struct {
	discard       overlay.DiscardPrompt
	hash          string
	filename      string   // opening filename, independent of edits to the title
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
			newField("", false),
		},
	}
}

// WithPath adds the original filename as read-only context for a single track.
func (e Editor) WithPath(path string) Editor {
	if path != "" {
		e.filename = filepath.Base(path)
	}
	return e
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
			newField("", false),
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
	if e.discard.Active {
		var discard bool
		e.discard, discard = e.discard.HandleKey(msg.String())
		if discard {
			return e, ActionCancel, nil
		}
		return e, actionNone, nil
	}
	return e.handleFormKey(msg)
}

func (e Editor) handleFormKey(msg tea.KeyPressMsg) (Editor, Action, tea.Cmd) {
	switch msg.String() {
	case "enter":
		return e, ActionSave, nil
	case "esc":
		return e.requestCancel()
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
		fields := e.renderFieldIndices()
		e.focused = Field(fields[(slices.Index(fields, int(e.focused))+1)%len(fields)])
		return e, actionNone, e.Fields[e.focused].Focus()
	case "shift+tab":
		if e.mode != ModeDefault {
			return e, actionNone, nil
		}
		e.Fields[e.focused].Blur()
		fields := e.renderFieldIndices()
		e.focused = Field(fields[(slices.Index(fields, int(e.focused))+len(fields)-1)%len(fields)])
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

// maxLabelW accommodates the longest changed label, "*Album Artist:".
const maxLabelW = 14
