package playlists

import (
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func (v PlaylistsView) Render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	pv := View{
		Level:            v.Level,
		allTracks:        v.Tracks,
		Cursor:           v.Cursor,
		ScrollOffset:     v.ScrollOffset,
		Playlists:        v.EffectivePlaylists(),
		FilterVisible:    v.Filter.Active || v.Filter.Text != "",
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
	body := pv.Render()
	if v.Filter.Text != "" && ((v.AtListLevel() && len(pv.Playlists) == 0) || (!v.AtListLevel() && len(pv.Tracks) == 0)) {
		body = ansi.Truncate(filter.NoMatchesMessage(true, false), v.Width, "…")
		if !v.AtListLevel() {
			body = pv.renderTrackHeading() + "\n" + pv.renderTrackSummary() + "\n" + body
		}
	}
	if pv.FilterVisible {
		return ansi.Truncate("Filter: "+v.Filter.View(), v.Width, "…") + "\n" + body
	}
	return body
}
