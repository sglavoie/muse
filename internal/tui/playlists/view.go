package playlists

import (
	"charm.land/bubbles/v2/textinput"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// Level identifies the active navigation level inside the playlist panel.
type Level int

const (
	ListLevel  Level = iota // showing the flat list of playlists
	TrackLevel              // showing the tracks inside a selected playlist
)

// Compile-time assertion: Styles must remain a value type on View.
// This line fails to compile if View.Styles is ever changed to *styles.Styles.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the playlists panel. All fields are set
// by the caller (PlaylistsView in the tui package) before each Render call.
type View struct {
	Level            Level
	Cursor           int
	ListCursor       int // saved list cursor; restored when BackToList is called
	ListScrollOffset int // saved list scroll offset; restored when BackToList is called
	ScrollOffset     int // first visible row index
	Playlists        []db.PlaylistRow
	Tracks           []db.PlaylistTrackRow // populated when Level == TrackLevel
	Width            int
	Height           int
	Creating         bool               // true while the inline create input is active
	CreateInput      textinput.Model    // current value of the inline create field
	Renaming         bool               // true while the inline rename input is active
	RenameInput      textinput.Model    // current value of the inline rename field
	SelectedTrackIDs map[int64]struct{} // bulk-selected playlist track row IDs
	FavoriteHashes   map[string]struct{}
	FavoriteArtists  map[string]struct{}
	FavoriteAlbums   map[string]struct{}
	Styles           styles.Styles
	IsSmartPlaylist  bool // true when the open playlist is a smart playlist (TrackLevel)
	SortField        string
	SortDir          string
	NowPlayingHash   string // hash of the currently playing track; empty when nothing is playing
	NowPlayingIcon   string // marker glyph (e.g. "▶", "⏸", "■"); empty falls back to "♪"
	playlistName     string // name of the open playlist, shown in the TrackLevel heading
}

// visibleRows returns the maximum number of rows that fit on screen.
// Reserve 1 line for the list level plus 1 extra when a sort indicator is
// shown, or 4 for the track level (heading + blank line + header
// row + 1 padding).
func (v View) visibleRows() int {
	reserve := 1
	if v.Level == ListLevel && v.SortField != "" {
		reserve++
	}
	if v.Level == TrackLevel {
		reserve = 4
	}
	return layout.VisibleRows(v.Height, reserve)
}

func halfPageStep(visibleRows int) int {
	step := visibleRows / 2
	if step < 1 {
		return 1
	}
	return step
}

// CursorUp returns a copy of v with the cursor moved up by one row,
// stopping at the first entry. The scroll offset is adjusted so the
// cursor stays visible.
func (v View) CursorUp() View {
	return v.moveCursor(-1)
}

// CursorDown returns a copy of v with the cursor moved down by one row,
// stopping at the last entry. The upper bound depends on the active level:
// TrackLevel uses the track slice length; ListLevel uses the playlist slice.
// The scroll offset is adjusted so the cursor stays visible.
func (v View) CursorDown() View {
	return v.moveCursor(1)
}

// HalfPageDown moves the cursor half a visible page down.
func (v View) HalfPageDown() View {
	return v.moveCursor(halfPageStep(v.visibleRows()))
}

// HalfPageUp moves the cursor half a visible page up.
func (v View) HalfPageUp() View {
	return v.moveCursor(-halfPageStep(v.visibleRows()))
}

// FullPageDown moves the cursor a full visible page down.
func (v View) FullPageDown() View {
	return v.moveCursor(v.visibleRows())
}

// FullPageUp moves the cursor a full visible page up.
func (v View) FullPageUp() View {
	return v.moveCursor(-v.visibleRows())
}

// GoToTop moves the cursor to the first row.
func (v View) GoToTop() View {
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// GoToBottom moves the cursor to the last row.
func (v View) GoToBottom() View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor = n - 1
	maxRows := v.visibleRows()
	if v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	// Clamp ScrollOffset down when the list shrank below a previously
	// scrolled position: otherwise the cursor renders above the visible
	// window after a delete reduces itemCount.
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}

// itemCount returns the number of items in the current level.
func (v View) itemCount() int {
	if v.Level == TrackLevel {
		return len(v.Tracks)
	}
	return len(v.Playlists)
}

// moveCursor shifts the cursor by delta rows, clamping to [0, itemCount-1],
// and adjusts the scroll offset so the cursor stays visible.
func (v View) moveCursor(delta int) View {
	n := v.itemCount()
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	maxRows := v.visibleRows()
	if maxRows <= 0 {
		v.ScrollOffset = v.Cursor
		if v.ScrollOffset < 0 {
			v.ScrollOffset = 0
		}
		return v
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	return v
}

// OpenTracks returns a copy of v transitioned to TrackLevel with tracks
// loaded. The current list cursor is saved to ListCursor so it can be
// restored by BackToList. The cursor is reset to 0 for track-level navigation.
// Any stale bulk track selection from a previous playlist is cleared.
func (v View) OpenTracks(tracks []db.PlaylistTrackRow) View {
	v.ListCursor = v.Cursor
	v.ListScrollOffset = v.ScrollOffset
	v.Tracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.SelectedTrackIDs = nil
	return v
}

// BackToList returns a copy of v transitioned back to ListLevel, restoring
// the cursor to the position saved by the preceding OpenTracks call and
// clearing the track slice. When Playlists is non-nil, the restored cursor
// and scroll offset are clamped against it so that a playlist deleted during
// the drill-down does not leave the cursor pointing past the new end. Callers
// that defer clamping to an outer wrapper (and pass a nil Playlists into the
// temporary View used purely for cursor translation) are unaffected.
func (v View) BackToList() View {
	v.Level = ListLevel
	v.Cursor = v.ListCursor
	v.ScrollOffset = v.ListScrollOffset
	v.Tracks = nil
	v.SelectedTrackIDs = nil
	if v.Playlists == nil {
		return v
	}
	n := len(v.Playlists)
	if n == 0 {
		v.Cursor = 0
		v.ScrollOffset = 0
		return v
	}
	if v.Cursor < 0 {
		v.Cursor = 0
	}
	if v.Cursor > n-1 {
		v.Cursor = n - 1
	}
	if v.ScrollOffset > v.Cursor {
		v.ScrollOffset = v.Cursor
	}
	if v.ScrollOffset < 0 {
		v.ScrollOffset = 0
	}
	// Keep the cursor inside the visible window when the saved ListScrollOffset
	// trailed far behind a (now clamped) Cursor — otherwise the focused row
	// would render below the bottom of the panel after BackToList.
	maxRows := v.visibleRows()
	if maxRows > 0 && v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// NewInlineInput creates a focused textinput suitable for inline row editing.
func NewInlineInput(initial string) textinput.Model {
	ti := textinput.New()
	ti.SetValue(initial)
	ti.CursorEnd()
	ti.Prompt = ""
	ti.CharLimit = 0
	ti.Focus()
	return ti
}

// StartCreate returns a copy of v with the inline create input activated and
// its buffer cleared.
func (v View) StartCreate() View {
	v.Creating = true
	v.CreateInput = NewInlineInput("")
	v.Renaming = false
	v.RenameInput = textinput.Model{}
	return v
}

// CancelCreate returns a copy of v with the inline create input dismissed.
func (v View) CancelCreate() View {
	v.Creating = false
	v.CreateInput = textinput.Model{}
	return v
}

// StartRename returns a copy of v with the inline rename input activated,
// pre-populated with currentName.
func (v View) StartRename(currentName string) View {
	v.Creating = false
	v.CreateInput = textinput.Model{}
	v.Renaming = true
	v.RenameInput = NewInlineInput(currentName)
	return v
}

// CancelRename returns a copy of v with the inline rename input dismissed.
func (v View) CancelRename() View {
	v.Renaming = false
	v.RenameInput = textinput.Model{}
	return v
}

// ToggleBulkSelection toggles id in the bulk-selected set. No-op when id is 0.
// The map is lazily initialised on first toggle.
func (v View) ToggleBulkSelection(id int64) View {
	if id == 0 {
		return v
	}
	if v.SelectedTrackIDs == nil {
		v.SelectedTrackIDs = make(map[int64]struct{})
	} else {
		selected := make(map[int64]struct{}, len(v.SelectedTrackIDs))
		for trackID := range v.SelectedTrackIDs {
			selected[trackID] = struct{}{}
		}
		v.SelectedTrackIDs = selected
	}
	if _, ok := v.SelectedTrackIDs[id]; ok {
		delete(v.SelectedTrackIDs, id)
	} else {
		v.SelectedTrackIDs[id] = struct{}{}
	}
	return v
}

// ClearBulkSelection removes all bulk-selection markers.
func (v View) ClearBulkSelection() View {
	v.SelectedTrackIDs = nil
	return v
}
