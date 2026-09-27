package playlists

import (
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func (v PlaylistsView) Render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	playlists := v.Playlists
	if v.SortedPlaylists != nil {
		playlists = v.SortedPlaylists
	}
	pv := View{
		Level:            v.Level,
		Cursor:           v.Cursor,
		ScrollOffset:     v.ScrollOffset,
		Playlists:        playlists,
		Tracks:           v.DisplayTracks(),
		Width:            v.Width,
		Height:           v.Height,
		Creating:         v.Creating,
		CreateInput:      v.createInput,
		Renaming:         v.Renaming,
		RenameInput:      v.RenameInput,
		SelectedTrackIDs: v.SelectedTrackIDs,
		IsSmartPlaylist:  v.SmartQuery != nil,
		playlistName:     v.sourcePlaylistName(),
	}
	if v.SortState.Field != sorting.SortDefault {
		pv.SortField = sorting.SortFieldLabel(v.SortState.Field)
		pv.SortDir = sorting.SortDirIndicator(v.SortState.Dir)
	}
	pv.Styles = s
	pv.FavoriteHashes = favHashes
	pv.FavoriteArtists = favArtists
	pv.FavoriteAlbums = favAlbums
	pv.NowPlayingHash = nowPlayingHash
	pv.NowPlayingIcon = nowPlayingIcon
	return pv.Render()
}
