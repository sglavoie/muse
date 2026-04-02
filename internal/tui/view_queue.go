package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/layout"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// QueueEntry holds a single item in the playback queue.
type QueueEntry struct {
	ID             int64  // database row ID; 0 when not persisted
	Path           string // file path used to identify the entry
	Hash           string // BLAKE3 content hash; used to rebuild player.QueueTrack on undo
	Display        string // human-readable label, e.g. "Artist – Title"
	Missing        bool   // true when the file was not found at playback time
	Artist         string // track artist, used by the status bar persistent left zone
	Title          string // track title, used by the status bar persistent left zone
	Album          string // track album name
	Selected       bool   // true when the entry is part of a bulk selection
	DurationMs     int64  // track duration in milliseconds from the database
	Position       int64  // gapped-integer queue position from the database
	SourcePlaylist string // playlist name this entry was enqueued from; empty for individual enqueues
	origIdx        int    // index in the unfiltered/unsorted entries slice; set by filteredEntries
}

// QueueView renders the playback queue panel.
//
// Key-input routing: all queue actions are dispatched by the root model via
// HandleAction after the keybinding registry resolves the raw key to an action.
// QueueView.Update is therefore a no-op stub kept only so the type satisfies a
// (QueueView, tea.Cmd) call-site contract. Do NOT add key-string handlers to
// Update — that would duplicate registry logic and create a path where registry
// key renames silently stop working.
type QueueView struct {
	entries             []QueueEntry
	cursor              int // index of the currently selected entry
	scrollOffset        int // first visible entry index
	width               int
	height              int
	savingAsPlaylist    bool            // true while the inline save-as-playlist input is active
	savingFromSelection bool            // true when saving only bulk-selected entries (not the full queue)
	saveInput           textinput.Model // current value of the inline save field
	sortState           SortState
	displayOrder        []int // nil when sort inactive (identity mapping)
	filter              filter.State
}

func (v *QueueView) setSize(w, h int) { v.width = w; v.height = h }

// recomputeDisplayOrder rebuilds the sorted display index mapping.
func (v QueueView) recomputeDisplayOrder() QueueView {
	v.displayOrder = SortQueueIndices(v.entries, v.sortState.Field, v.sortState.Dir)
	return v
}

// entryIndex translates a display row index to the underlying entries index.
func (v QueueView) entryIndex(displayIdx int) int {
	if v.displayOrder == nil {
		return displayIdx
	}
	if displayIdx < 0 || displayIdx >= len(v.displayOrder) {
		return displayIdx
	}
	return v.displayOrder[displayIdx]
}

// findEntryDisplayPosition returns the display row for the given underlying
// entry index. Returns 0 if not found.
func (v QueueView) findEntryDisplayPosition(entryIdx int) int {
	if v.displayOrder == nil {
		return entryIdx
	}
	for i, idx := range v.displayOrder {
		if idx == entryIdx {
			return i
		}
	}
	return 0
}

// Update is a no-op stub. See the QueueView godoc for why all key handling is
// done via HandleAction instead.
func (v QueueView) Update(_ tea.Msg) (QueueView, tea.Cmd) {
	return v, nil
}

// HandleAction executes a pre-resolved queue action dispatched by root.Update.
// Root resolves the raw key to an action via the keybinding registry and calls
// this method, keeping key-to-action mapping in one authoritative location.
func (v QueueView) HandleAction(a action) (QueueView, tea.Cmd) {
	filtered := v.filteredEntries()
	n := len(filtered)

	switch a {
	case actionCursorUp:
		if v.cursor > 0 {
			v.cursor--
		}
		if v.cursor < v.scrollOffset {
			v.scrollOffset = v.cursor
		}
	case actionCursorDown:
		if v.cursor < n-1 {
			v.cursor++
		}
		maxRows := v.visibleRows()
		if v.cursor >= v.scrollOffset+maxRows {
			v.scrollOffset = v.cursor - maxRows + 1
		}
	case actionHalfPageDown:
		v = v.moveQueueCursor(v.visibleRows() / 2)
	case actionHalfPageUp:
		v = v.moveQueueCursor(-v.visibleRows() / 2)
	case actionFullPageDown:
		v = v.moveQueueCursor(v.visibleRows())
	case actionFullPageUp:
		v = v.moveQueueCursor(-v.visibleRows())
	case actionGoToTop:
		v.cursor = 0
		v.scrollOffset = 0
	case actionGoToBottom:
		if n > 0 {
			v.cursor = n - 1
			maxRows := v.visibleRows()
			if v.cursor >= maxRows {
				v.scrollOffset = v.cursor - maxRows + 1
			}
		}
	case actionSelect:
		if v.cursor >= 0 && v.cursor < n {
			e := filtered[v.cursor]
			idx := v.realEntryIndex(e)
			if !e.Missing {
				return v, func() tea.Msg { return PlayQueueIndexMsg{Index: idx} }
			}
			return v, func() tea.Msg { return MissingTrackSelectedMsg{} }
		}
	case actionDequeue:
		if v.cursor >= 0 && v.cursor < n {
			e := filtered[v.cursor]
			idx := v.realEntryIndex(e)
			return v, func() tea.Msg { return DequeueTrackMsg{Index: idx} }
		}
	case actionBulkSelectionToggle:
		if v.cursor >= 0 && v.cursor < n {
			e := filtered[v.cursor]
			idx := v.realEntryIndex(e)
			return v, func() tea.Msg { return BulkSelectionToggleMsg{Index: idx} }
		}
	case actionClearQueue:
		return v, func() tea.Msg { return ClearQueueMsg{} }
	case actionMoveTrackUp:
		if v.displayOrder != nil || v.filter.Text != "" {
			// Move is disabled when queue is sorted or filtered.
			return v, func() tea.Msg { return TransientNotifyMsg("Cannot reorder while sorted/filtered") }
		}
		if v.cursor > 0 {
			idx := v.cursor
			return v, func() tea.Msg { return MoveTrackUpMsg{Index: idx} }
		}
	case actionMoveTrackDown:
		if v.displayOrder != nil || v.filter.Text != "" {
			// Move is disabled when queue is sorted or filtered.
			return v, func() tea.Msg { return TransientNotifyMsg("Cannot reorder while sorted/filtered") }
		}
		if v.cursor < n-1 {
			idx := v.cursor
			return v, func() tea.Msg { return MoveTrackDownMsg{Index: idx} }
		}
	}
	return v, nil
}

// realEntryIndex returns the index of entry e in the underlying v.entries slice.
// It matches by Path+Hash to handle entries that may appear multiple times.
func (v QueueView) realEntryIndex(e QueueEntry) int {
	for i, ee := range v.entries {
		if ee.Path == e.Path && ee.Hash == e.Hash && ee.ID == e.ID {
			return i
		}
	}
	return 0
}

// markMissing marks all entries whose Path matches as missing.
func (v *QueueView) markMissing(path string) {
	for i := range v.entries {
		if v.entries[i].Path == path {
			v.entries[i].Missing = true
		}
	}
}

// isSavingAsPlaylist reports whether the inline save-as-playlist input is active.
func (v QueueView) isSavingAsPlaylist() bool {
	return v.savingAsPlaylist
}

// startSaveAsPlaylist activates the inline save-as-playlist input with an empty buffer.
// When fromSelection is true the prompt says "Save selection as playlist" and
// only bulk-selected entries will be persisted.
func (v QueueView) startSaveAsPlaylist(fromSelection bool) QueueView {
	v.savingAsPlaylist = true
	v.savingFromSelection = fromSelection
	v.saveInput = pltui.NewInlineInput("")
	return v
}

// cancelSaveAsPlaylist dismisses the inline save-as-playlist input without persisting.
func (v QueueView) cancelSaveAsPlaylist() QueueView {
	v.savingAsPlaylist = false
	v.savingFromSelection = false
	v.saveInput = textinput.Model{}
	return v
}

// hasSelection reports whether any queue entries are bulk-selected.
func (v QueueView) hasSelection() bool {
	for _, e := range v.entries {
		if e.Selected {
			return true
		}
	}
	return false
}

// selectedHashes returns the hashes of all bulk-selected entries, preserving queue order.
func (v QueueView) selectedHashes() []string {
	var hashes []string
	for _, e := range v.entries {
		if e.Selected {
			hashes = append(hashes, e.Hash)
		}
	}
	return hashes
}

// updateSaveInput forwards a key message to the Bubbles textinput model.
func (v QueueView) updateSaveInput(msg tea.KeyPressMsg) (QueueView, tea.Cmd) {
	var cmd tea.Cmd
	v.saveInput, cmd = v.saveInput.Update(msg)
	return v, cmd
}

// updateSaveInputMsg forwards a generic tea.Msg to the textinput, enabling
// paste and other multi-step commands.
func (v QueueView) updateSaveInputMsg(msg tea.Msg) (QueueView, tea.Cmd) {
	var cmd tea.Cmd
	v.saveInput, cmd = v.saveInput.Update(msg)
	return v, cmd
}

// saveInputValue returns the current save input buffer, trimmed of
// leading/trailing whitespace to prevent names like "  hello  ".
func (v QueueView) saveInputValue() string {
	return strings.TrimSpace(v.saveInput.Value())
}

// saveInputIsEmpty reports whether the trimmed save input buffer is empty.
func (v QueueView) saveInputIsEmpty() bool {
	return strings.TrimSpace(v.saveInput.Value()) == ""
}

// filteredEntries returns queue entries matching the current filter text,
// in display order (respecting sort). When no filter is active, all entries
// are returned in display order.
func (v QueueView) filteredEntries() []QueueEntry {
	n := len(v.entries)
	if n == 0 {
		return nil
	}

	// Build display-ordered slice.
	ordered := make([]QueueEntry, n)
	for i := range n {
		idx := i
		if v.displayOrder != nil {
			idx = v.displayOrder[i]
		}
		ordered[i] = v.entries[idx]
		ordered[i].origIdx = idx
	}

	if v.filter.Text == "" {
		return ordered
	}
	query := strings.ToLower(v.filter.Text)
	var result []QueueEntry
	for _, e := range ordered {
		if strings.Contains(strings.ToLower(e.Title), query) ||
			strings.Contains(strings.ToLower(e.Artist), query) ||
			strings.Contains(strings.ToLower(e.Album), query) {
			result = append(result, e)
		}
	}
	return result
}

// FocusNowPlaying moves the cursor to the entry whose Hash matches
// currentPlayingIndex and adjusts the scroll offset to center it in the viewport.
// Returns v unchanged if no match is found or index is negative.
func (v QueueView) FocusNowPlaying(currentPlayingIndex int) QueueView {
	if currentPlayingIndex < 0 {
		return v
	}
	filtered := v.filteredEntries()
	idx := -1
	for i, e := range filtered {
		if e.origIdx == currentPlayingIndex {
			idx = i
			break
		}
	}
	if idx < 0 {
		return v
	}
	v.cursor = idx
	maxRows := v.visibleRows()
	if v.cursor >= maxRows {
		v.scrollOffset = v.cursor - maxRows/2
	} else {
		v.scrollOffset = 0
	}
	return v
}

// moveQueueCursor shifts the cursor by delta rows, clamping to [0, len-1],
// and adjusts the scroll offset so the cursor stays visible.
func (v QueueView) moveQueueCursor(delta int) QueueView {
	n := len(v.filteredEntries())
	if n == 0 {
		return v
	}
	v.cursor += delta
	v.cursor = max(0, min(v.cursor, n-1))
	maxRows := v.visibleRows()
	if v.cursor < v.scrollOffset {
		v.scrollOffset = v.cursor
	} else if v.cursor >= v.scrollOffset+maxRows {
		v.scrollOffset = v.cursor - maxRows + 1
	}
	return v
}

// visibleRows returns the maximum number of queue entries that fit on screen.
// Reserve 2 lines for the column header and bottom margin, plus 1 extra when
// the filter bar is visible (active or committed with non-empty text), and
// 1 extra when the save-as-playlist input is active.
func (v QueueView) visibleRows() int {
	reserve := 2
	if v.filter.Active || v.filter.Text != "" {
		reserve++
	}
	if v.savingAsPlaylist {
		reserve++
	}
	return layout.VisibleRows(v.height, reserve)
}

// render builds the queue panel string using a 6-column layout:
//
//	Position (~4 chars fixed) | Title (~30% of width) | Artist (~20% of width) | Album (~20% of width) | Source (~10% of width) | Duration (~10 chars fixed)
//
// Proportional columns resize on WindowSizeMsg; fixed-width columns are
// constant. Missing entries are wrapped in ANSI dim escape codes so they
// appear grayed out in the terminal.
func (v QueueView) render(s styles.Styles, currentPlayingIndex int, favHashes, favArtists, favAlbums map[string]struct{}) string {
	entries := v.filteredEntries()
	if len(entries) == 0 {
		if v.filter.Text != "" {
			return "(no matches)"
		}
		return "(empty)"
	}

	const (
		posWidth    = 4  // "   1".."9999"
		durWidth    = 10 // "59:59" or "--:--" right-aligned
		markerWidth = 2  // "● " or "▶ " or "  "
		sepCount    = 5  // spaces between the 6 content columns
	)

	titleWidth := 20 // sensible fallback when width is unknown
	artistWidth := 15
	albumWidth := 15
	sourceWidth := 10
	if v.width > 0 {
		fixed := posWidth + durWidth + markerWidth + sepCount
		flex := v.width - fixed
		flex = max(flex, 4)
		titleWidth = flex * 30 / 80
		artistWidth = flex * 20 / 80
		albumWidth = flex * 20 / 80
		sourceWidth = flex - titleWidth - artistWidth - albumWidth // absorbs remainder
	}
	titleWidth = max(titleWidth, 1)
	artistWidth = max(artistWidth, 1)
	albumWidth = max(albumWidth, 1)
	sourceWidth = max(sourceWidth, 1)

	maxRows := v.visibleRows()
	start := min(v.scrollOffset, len(entries))
	end := min(len(entries), start+maxRows)

	// Column header row (2-char left margin aligns with the selection marker column).
	titleHead, artistHead, albumHead, sourceHead, timeHead := "Title", "Artist", "Album", "Source", "Time"
	if v.sortState.Field != SortDefault {
		ind := SortDirIndicator(v.sortState.Dir)
		switch v.sortState.Field {
		case SortTitle:
			titleHead += ind
		case SortArtist:
			artistHead += ind
		case SortAlbum:
			albumHead += ind
		case SortSource:
			sourceHead += ind
		case SortDuration:
			timeHead += ind
		}
	}
	header := fmt.Sprintf("  %-*s %-*s %-*s %-*s %-*s %*s",
		posWidth, "#", titleWidth, titleHead, artistWidth, artistHead, albumWidth, albumHead, sourceWidth, sourceHead, durWidth, timeHead)

	lines := make([]string, end-start)
	for i := range end - start {
		absIdx := start + i
		e := entries[absIdx]
		isFocused := absIdx == v.cursor

		isNowPlaying := currentPlayingIndex >= 0 && e.origIdx == currentPlayingIndex

		// Selection marker: matches Library and Playlists style.
		// Now-playing track gets a "▶" marker when not bulk-selected.
		var markerStr string
		switch {
		case e.Selected && isFocused:
			markerStr = "● "
		case e.Selected:
			markerStr = s.AccentMark.Render("●") + " "
		case isNowPlaying && isFocused:
			markerStr = "▶ "
		case isNowPlaying:
			markerStr = "▶ "
		default:
			markerStr = "  "
		}

		pos := fmt.Sprintf("%4d", absIdx+1)

		source := padQueue(e.SourcePlaylist, sourceWidth)
		dur := fmt.Sprintf("%*s", durWidth, formatQueueDuration(e.DurationMs))

		// Per-cell favorite lookups.
		_, isTrackFav := favHashes[e.Hash]
		_, isArtistFav := favArtists[e.Artist]
		_, isAlbumFav := favAlbums[e.Album]

		// Style each text cell independently.
		// When the row is focused, apply accent background to each cell
		// individually rather than wrapping the whole row—nested
		// Render calls produce ANSI resets that kill the outer background.
		titleStr := padQueue(e.Title, titleWidth)
		artistStr := padQueue(e.Artist, artistWidth)
		albumStr := padQueue(e.Album, albumWidth)
		if e.Missing { //nolint:gocritic // ifElseChain: branches have fundamentally different rendering logic
			plainLine := markerStr + pos + " " + padQueue(e.Title, titleWidth) + " " + padQueue(e.Artist, artistWidth) + " " + padQueue(e.Album, albumWidth) + " " + source + " " + dur
			lines[i] = s.Dim.Render(plainLine + " [missing]")
		} else if isFocused {
			bg := s.Accent.GetBackground()
			sep := s.Accent.Render(" ")
			if isNowPlaying {
				titleStr = s.NowPlaying.Background(bg).Render(titleStr)
				artistStr = s.NowPlaying.Background(bg).Render(artistStr)
				albumStr = s.NowPlaying.Background(bg).Render(albumStr)
				if isTrackFav {
					titleStr = s.NowPlayingFavorite.Background(bg).Render(padQueue(e.Title, titleWidth))
				}
				if isArtistFav {
					artistStr = s.NowPlayingFavArtist.Background(bg).Render(padQueue(e.Artist, artistWidth))
				}
				if isAlbumFav {
					albumStr = s.NowPlayingFavAlbum.Background(bg).Render(padQueue(e.Album, albumWidth))
				}
			} else {
				if isTrackFav {
					titleStr = s.FavoriteAccent.Render(titleStr)
				} else {
					titleStr = s.Accent.Render(titleStr)
				}
				if isArtistFav {
					artistStr = s.FavArtistAccent.Render(artistStr)
				} else {
					artistStr = s.Accent.Render(artistStr)
				}
				if isAlbumFav {
					albumStr = s.FavAlbumAccent.Render(albumStr)
				} else {
					albumStr = s.Accent.Render(albumStr)
				}
			}
			markerStr = s.Accent.Render(markerStr)
			pos = s.Accent.Render(pos)
			source = s.Accent.Render(source)
			dur = s.Accent.Render(dur)
			line := markerStr + pos + sep + titleStr + sep + artistStr + sep + albumStr + sep + source + sep + dur
			lines[i] = line
		} else if isNowPlaying {
			titleStr = s.NowPlaying.Render(titleStr)
			artistStr = s.NowPlaying.Render(artistStr)
			albumStr = s.NowPlaying.Render(albumStr)
			if isTrackFav {
				titleStr = s.NowPlayingFavorite.Render(padQueue(e.Title, titleWidth))
			}
			if isArtistFav {
				artistStr = s.NowPlayingFavArtist.Render(padQueue(e.Artist, artistWidth))
			}
			if isAlbumFav {
				albumStr = s.NowPlayingFavAlbum.Render(padQueue(e.Album, albumWidth))
			}
			line := markerStr + pos + " " + titleStr + " " + artistStr + " " + albumStr + " " + source + " " + dur
			lines[i] = line
		} else {
			if isTrackFav {
				titleStr = s.Favorite.Render(titleStr)
			}
			if isArtistFav {
				artistStr = s.FavArtist.Render(artistStr)
			}
			if isAlbumFav {
				albumStr = s.FavAlbum.Render(albumStr)
			}
			lines[i] = markerStr + pos + " " + titleStr + " " + artistStr + " " + albumStr + " " + source + " " + dur
		}
	}
	// Filter bar line (above the column header, matching the pattern of other views).
	var filterLine string
	if v.filter.Active {
		filterLine = "Filter: " + v.filter.View() + "\n"
	} else if v.filter.Text != "" {
		ft := v.filter.Text
		maxLen := v.width - len("Filter: ") - 1
		if maxLen > 0 && ansi.StringWidth(ft) > maxLen {
			ft = ansi.Truncate(ft, maxLen-1, "…")
		}
		filterLine = "Filter: " + ft + "\n"
	}

	out := filterLine + s.ColHead.Render(header) + "\n" + strings.Join(lines, "\n")
	if v.savingAsPlaylist {
		prompt := "Save as playlist: "
		if v.savingFromSelection {
			prompt = "Save selection as playlist: "
		}
		inputLine := s.Accent.Render(prompt) + v.saveInput.View()
		out += "\n" + inputLine
	}
	return out
}

// formatQueueDuration converts a duration in milliseconds to a "m:ss" or
// "h:mm:ss" string. Returns "--:--" when ms is zero or negative.
func formatQueueDuration(ms int64) string {
	return FormatDurationMs(ms)
}

// padQueue truncates or right-pads s with spaces to exactly width display columns.
func padQueue(s string, width int) string {
	if width <= 0 {
		return ""
	}
	s = truncateTitle(s, width)
	n := ansi.StringWidth(s)
	if n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s
}
