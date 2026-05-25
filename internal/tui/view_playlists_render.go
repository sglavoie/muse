package tui

import (
	"github.com/sglavoie/muse/internal/db"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func (v PlaylistsView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	playlists := v.playlists
	if v.sortedPlaylists != nil {
		playlists = v.sortedPlaylists
	}
	tracks := v.tracks
	if v.sortedTrackOrder != nil && len(v.sortedTrackOrder) == len(v.tracks) {
		// Build a reordered track slice for rendering. Guard against stale
		// sortedTrackOrder (length mismatch or out-of-range indices) by
		// falling back to natural order — rendering a partially-filled or
		// out-of-range slice would show empty rows or panic.
		reordered := make([]db.PlaylistTrackRow, len(v.tracks))
		safe := true
		for i, idx := range v.sortedTrackOrder {
			if idx < 0 || idx >= len(v.tracks) {
				safe = false
				break
			}
			reordered[i] = v.tracks[idx]
		}
		if safe {
			tracks = reordered
		}
	}
	pv := pltui.View{
		Level:            v.level,
		Cursor:           v.cursor,
		ScrollOffset:     v.scrollOffset,
		Playlists:        playlists,
		Tracks:           tracks,
		Width:            v.width,
		Height:           v.height,
		Creating:         v.creating,
		CreateInput:      v.createInput,
		Renaming:         v.renaming,
		RenameInput:      v.renameInput,
		SelectedTrackIDs: v.selectedTrackIDs,
		IsSmartPlaylist:  v.smartQuery != nil,
	}
	if v.sortState.Field != SortDefault {
		pv.SortField = SortFieldLabel(v.sortState.Field)
		pv.SortDir = SortDirIndicator(v.sortState.Dir)
	}
	pv.Styles = s
	pv.FavoriteHashes = favHashes
	pv.FavoriteArtists = favArtists
	pv.FavoriteAlbums = favAlbums
	pv.NowPlayingHash = nowPlayingHash
	pv.NowPlayingIcon = nowPlayingIcon
	return pv.Render()
}
