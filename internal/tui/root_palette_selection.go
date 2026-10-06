package tui

import "fmt"

// Count rows rather than hashes: queue and playlist duplicates are separate selections.
func (m Model) clearSelectionDetail() string {
	total := m.activeViewSelectionCount()
	visible := 0
	switch m.ActiveView {
	case viewQueue:
		for _, row := range m.queue.FilteredEntries() {
			if row.Selected {
				visible++
			}
		}
	case viewPlaylists:
		for _, row := range m.playlists.DisplayTracks() {
			if _, ok := m.playlists.SelectedTrackIDs[row.ID]; ok {
				visible++
			}
		}
	case viewFavorites:
		visible = m.visibleFavoriteSelectionCount()
	default:
		visible = m.visibleSelectedTrackCount()
	}

	detail := fmt.Sprintf("Clear %d selected rows", total)
	if hidden := total - visible; hidden > 0 {
		detail += fmt.Sprintf(", including %d hidden", hidden)
	}
	return detail
}

func (m Model) visibleSelectedTrackCount() int {
	selected := make(map[string]bool)
	for _, hash := range m.bulkSelectedHashes() {
		selected[hash] = true
	}
	rows, _, _ := m.visibleTracks()
	count := 0
	for _, row := range rows {
		if selected[row.Hash] {
			count++
			delete(selected, row.Hash)
		}
	}
	return count
}

func (m Model) visibleFavoriteSelectionCount() int {
	count := 0
	for _, key := range m.favorites.SelectionKeys() {
		if _, ok := m.favorites.SelectedItems[key]; ok {
			count++
		}
	}
	return count
}
