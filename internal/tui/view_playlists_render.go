package tui

import (
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func (v PlaylistsView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	playlists := v.playlists
	if v.sortedPlaylists != nil {
		playlists = v.sortedPlaylists
	}
	pv := pltui.View{
		Level:            v.level,
		Cursor:           v.cursor,
		ScrollOffset:     v.scrollOffset,
		Playlists:        playlists,
		Tracks:           v.displayTracks(),
		Width:            v.width,
		Height:           v.height,
		Creating:         v.creating,
		CreateInput:      v.createInput,
		Renaming:         v.renaming,
		RenameInput:      v.renameInput,
		SelectedTrackIDs: v.selectedTrackIDs,
		IsSmartPlaylist:  v.smartQuery != nil,
	}
	if v.sortState.Field != sorting.SortDefault {
		pv.SortField = sorting.SortFieldLabel(v.sortState.Field)
		pv.SortDir = sorting.SortDirIndicator(v.sortState.Dir)
	}
	pv.Styles = s
	pv.FavoriteHashes = favHashes
	pv.FavoriteArtists = favArtists
	pv.FavoriteAlbums = favAlbums
	pv.NowPlayingHash = nowPlayingHash
	pv.NowPlayingIcon = nowPlayingIcon
	return pv.Render()
}
