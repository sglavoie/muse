package tags

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/layout"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// Level identifies the active navigation level inside the tags panel.
type Level int

const (
	TagLevel   Level = iota // showing the flat list of tags
	TrackLevel              // showing tracks for a selected tag
)

// Compile-time assertion: Styles must remain a value type on View.
var _ styles.Styles = View{}.Styles

// View is the pure render struct for the tags panel with 2-level drill-down:
// Tags -> Tracks. All fields are set by the caller before each Render call.
type View struct {
	Level        Level
	Cursor       int
	ScrollOffset int

	Tags            []db.TagRow
	TagCounts       map[int64]int // tag ID → track count
	Tracks          []db.PlaylistTrackRow
	SelectedTag     db.TagRow
	TagCursor       int // saved tag cursor for restoring on Back
	TagScrollOffset int // saved tag scroll offset for restoring on Back

	Width  int
	Height int
	Styles styles.Styles

	FilterActive bool
	FilterText   string
	FilterView   string // pre-rendered textinput view for active filter display

	SortIndicator  string // e.g. "Sort: Name ▲"
	FavoriteHashes map[string]struct{}
	SelectedHashes map[string]struct{}
	NowPlayingHash string // hash of the currently playing track; empty when nothing is playing

	// Inline create/rename input state (passed through from TagsView).
	Creating    bool
	CreateInput string // current value of the inline create field
	Renaming    bool
	RenameInput string // current value of the inline rename field
}

// EffectiveTags returns the tag list filtered by FilterText, excluding tags
// with no associated tracks.
func (v View) EffectiveTags() []db.TagRow {
	q := strings.ToLower(v.FilterText)
	var result []db.TagRow
	for _, t := range v.Tags {
		if v.TagCounts != nil && v.TagCounts[t.ID] == 0 {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(t.Name), q) {
			continue
		}
		result = append(result, t)
	}
	return result
}

// EffectiveTracks returns the track list filtered by FilterText.
func (v View) EffectiveTracks() []db.PlaylistTrackRow {
	if v.FilterText == "" {
		return v.Tracks
	}
	q := strings.ToLower(v.FilterText)
	var result []db.PlaylistTrackRow
	for _, t := range v.Tracks {
		if strings.Contains(strings.ToLower(t.Title), q) ||
			strings.Contains(strings.ToLower(t.Artist), q) ||
			strings.Contains(strings.ToLower(t.Album), q) {
			result = append(result, t)
		}
	}
	return result
}

// visibleRows returns the maximum number of data rows that fit on screen.
func (v View) visibleRows() int {
	var reserve int
	switch v.Level {
	case TagLevel:
		if v.FilterActive || v.FilterText != "" {
			reserve = 1
		}
	default: // TrackLevel
		reserve = 3 // heading + separator + column header
	}
	if v.SortIndicator != "" {
		reserve++
	}
	return layout.VisibleRows(v.Height, reserve)
}

// itemCount returns the number of items in the current level.
func (v View) itemCount() int {
	if v.Level == TrackLevel {
		return len(v.EffectiveTracks())
	}
	return len(v.EffectiveTags())
}

// CursorUp returns a copy of v with the cursor moved up by one row.
func (v View) CursorUp() View {
	if v.Cursor > 0 {
		v.Cursor--
	}
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	}
	return v
}

// CursorDown returns a copy of v with the cursor moved down by one row.
func (v View) CursorDown() View {
	limit := v.itemCount() - 1
	if v.Cursor < limit {
		v.Cursor++
	}
	maxRows := v.visibleRows()
	if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// HalfPageDown moves the cursor half a visible page down.
func (v View) HalfPageDown() View { return v.moveCursor(v.visibleRows() / 2) }

// HalfPageUp moves the cursor half a visible page up.
func (v View) HalfPageUp() View { return v.moveCursor(-v.visibleRows() / 2) }

// FullPageDown moves the cursor a full visible page down.
func (v View) FullPageDown() View { return v.moveCursor(v.visibleRows()) }

// FullPageUp moves the cursor a full visible page up.
func (v View) FullPageUp() View { return v.moveCursor(-v.visibleRows()) }

// GoToTop moves the cursor to the first row.
func (v View) GoToTop() View {
	v.Cursor = 0
	v.ScrollOffset = 0
	return v
}

// GoToBottom moves the cursor to the last row.
func (v View) GoToBottom() View {
	n := v.itemCount()
	if n > 0 {
		v.Cursor = n - 1
	}
	maxRows := v.visibleRows()
	if v.Cursor >= maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

func (v View) moveCursor(delta int) View {
	n := v.itemCount()
	if n == 0 {
		return v
	}
	v.Cursor += delta
	v.Cursor = max(0, min(v.Cursor, n-1))
	maxRows := v.visibleRows()
	if v.Cursor < v.ScrollOffset {
		v.ScrollOffset = v.Cursor
	} else if v.Cursor >= v.ScrollOffset+maxRows {
		v.ScrollOffset = v.Cursor - maxRows + 1
	}
	return v
}

// OpenTracks returns a copy of v transitioned to TrackLevel with tracks
// loaded. The current tag cursor and scroll offset are saved for restoration
// by Back.
func (v View) OpenTracks(tag db.TagRow, tracks []db.PlaylistTrackRow) View {
	v.TagCursor = v.Cursor
	v.TagScrollOffset = v.ScrollOffset
	v.SelectedTag = tag
	v.Tracks = tracks
	v.Level = TrackLevel
	v.Cursor = 0
	v.ScrollOffset = 0
	v.FilterActive = false
	v.FilterText = ""
	return v
}

// Back returns a copy of v navigated one level up. At TrackLevel it returns to
// TagLevel. At TagLevel it is a no-op.
func (v View) Back() View {
	if v.Level == TrackLevel {
		v.Level = TagLevel
		v.Cursor = v.TagCursor
		v.ScrollOffset = v.TagScrollOffset
		v.Tracks = nil
		v.FilterActive = false
		v.FilterText = ""
	}
	return v
}

// ---------- Rendering ----------

// Render returns the string representation of the tags panel.
func (v View) Render() string {
	if v.Level == TrackLevel {
		return v.renderTracks()
	}
	return v.renderTags()
}

func (v View) renderTags() string {
	items := v.EffectiveTags()
	if len(v.Tags) == 0 && !v.Creating {
		return "(no tags — press c to create)"
	}

	maxRows := v.visibleRows()

	// When inline create is active, show the input as an extra row at bottom.
	var createRow string
	if v.Creating {
		maxRows-- // reserve one row for the input
		label := text.Pad("+ "+v.CreateInput+"_", v.Width)
		createRow = "\n" + v.Styles.Accent.Render(label)
	}

	if len(items) == 0 && !v.Creating {
		return v.topFilterSeparator() + "(no matches)"
	}

	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, tag := range items[start:end] {
		absIdx := start + i
		count := 0
		if v.TagCounts != nil {
			count = v.TagCounts[tag.ID]
		}
		dot := styles.ColoredDot(tag.Color)
		name := fmt.Sprintf("%s %s (%d)", dot, tag.Name, count)
		display := text.Pad(name, v.Width)
		isOdd := absIdx%2 == 1
		if absIdx == v.Cursor {
			if v.Renaming {
				// Show inline rename for the focused row.
				renamed := text.Pad("r "+v.RenameInput+"_", v.Width)
				display = v.Styles.Accent.Render(renamed)
			} else {
				display = v.Styles.Accent.Render(display)
			}
		} else if isOdd {
			display = lipgloss.NewStyle().Background(v.Styles.ZebraBg).Render(display)
		}
		lines[i] = display
	}
	return v.topFilterSeparator() + strings.Join(lines, "\n") + createRow
}

func (v View) renderTracks() string {
	heading := v.Styles.Heading.Render("Tags — " + v.SelectedTag.Name)
	items := v.EffectiveTracks()
	if len(v.Tracks) == 0 {
		return heading + "\n\n(no tracks)"
	}
	if len(items) == 0 {
		return heading + v.filterSeparator() + "(no matches)"
	}

	cols := ComputeTrack(v.Width)

	header := v.Styles.ColHead.Render(
		strings.Repeat(" ", cols.SelectionMarker) +
			fmt.Sprintf("%-*s %-*s %-*s %*s",
				cols.Title, "Title",
				cols.Artist, "Artist",
				cols.Album, "Album",
				cols.Duration, "Time"))

	maxRows := v.visibleRows()
	start := min(v.ScrollOffset, len(items))
	end := min(len(items), start+maxRows)

	lines := make([]string, end-start)
	for i, t := range items[start:end] {
		absIdx := start + i
		isFocused := absIdx == v.Cursor
		isNowPlaying := v.NowPlayingHash != "" && t.Hash == v.NowPlayingHash
		_, isSelected := v.SelectedHashes[t.Hash]
		_, isTrackFav := v.FavoriteHashes[t.Hash]

		var markerStr string
		switch {
		case isSelected && isFocused:
			markerStr = "● "
		case isSelected:
			markerStr = v.Styles.AccentMark.Render("●") + " "
		case isFocused && isNowPlaying:
			markerStr = "♪ "
		case isFocused:
			markerStr = "▎ "
		case isNowPlaying:
			markerStr = "♪ "
		default:
			markerStr = "  "
		}

		titleStr := text.Pad(t.Title, cols.Title)
		artistStr := text.Pad(t.Artist, cols.Artist)
		albumStr := text.Pad(t.Album, cols.Album)
		dur := text.FormatDurationSecs(float64(t.DurationMs) / 1000)
		durStr := fmt.Sprintf("%*s", cols.Duration, dur)

		if isFocused {
			a := v.Styles.Accent
			sep := a.Render(" ")
			titleStr = styles.AccentFavRender(a, v.Styles.FavoriteAccent, isTrackFav, titleStr)
			lines[i] = a.Render(markerStr) + titleStr + sep + a.Render(artistStr) + sep + a.Render(albumStr) + sep + a.Render(durStr)
		} else if isNowPlaying {
			np := v.Styles.NowPlayingRow
			sep := np.Render(" ")
			titleStr = v.Styles.NowPlayingTitle.Render(titleStr)
			artistStr = v.Styles.TextPrimary.Background(v.Styles.NowPlayingBg).Render(artistStr)
			albumStr = v.Styles.TextSecondary.Background(v.Styles.NowPlayingBg).Render(albumStr)
			lines[i] = np.Render(markerStr) + titleStr + sep + artistStr + sep + albumStr + sep + np.Render(durStr)
		} else {
			isOdd := absIdx%2 == 1
			if isTrackFav {
				titleStr = styles.WithZebraBg(v.Styles.Favorite, v.Styles.ZebraBg, isOdd).Render(titleStr)
			}
			if isOdd {
				zb := lipgloss.NewStyle().Background(v.Styles.ZebraBg)
				if !isTrackFav {
					titleStr = styles.WithZebraBg(v.Styles.TextPrimary, v.Styles.ZebraBg, true).Render(titleStr)
				}
				markerPart := zb.Render(markerStr)
				artistPart := styles.WithZebraBg(v.Styles.TextPrimary, v.Styles.ZebraBg, true).Render(artistStr)
				albumPart := styles.WithZebraBg(v.Styles.TextSecondary, v.Styles.ZebraBg, true).Render(albumStr)
				durPart := styles.WithZebraBg(v.Styles.TextTertiary, v.Styles.ZebraBg, isOdd).Render(durStr)
				sep := zb.Render(" ")
				lines[i] = markerPart + titleStr + sep + artistPart + sep + albumPart + sep + durPart
			} else {
				if !isTrackFav {
					titleStr = v.Styles.TextPrimary.Render(titleStr)
				}
				lines[i] = markerStr + titleStr + " " + v.Styles.TextPrimary.Render(artistStr) + " " + v.Styles.TextSecondary.Render(albumStr) + " " + v.Styles.TextTertiary.Render(durStr)
			}
		}
	}
	return heading + v.filterSeparator() + header + "\n" + strings.Join(lines, "\n")
}

// filterSeparator returns the line between the heading and the item list.
func (v View) filterSeparator() string {
	return v.buildFilterSeparator(true)
}

// topFilterSeparator is like filterSeparator but without a leading newline.
func (v View) topFilterSeparator() string {
	return v.buildFilterSeparator(false)
}

func (v View) buildFilterSeparator(leadingNewline bool) string {
	var prefix string
	if v.SortIndicator != "" {
		if leadingNewline {
			prefix = "\n" + v.SortIndicator
		} else {
			prefix = v.SortIndicator + "\n"
		}
	}
	if v.FilterActive {
		sep := "\n"
		if !leadingNewline {
			sep = ""
		}
		fv := v.FilterView
		if fv == "" {
			fv = v.FilterText
		}
		return prefix + sep + "Filter: " + fv + "\n"
	}
	if v.FilterText != "" {
		sep := "\n"
		if !leadingNewline {
			sep = ""
		}
		ft := v.FilterText
		maxLen := v.Width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		return prefix + sep + "Filter: " + ft + "\n"
	}
	if leadingNewline {
		return prefix + "\n\n"
	}
	return prefix
}
