package playlists

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
	"github.com/sglavoie/muse/internal/tui/styles"
)

func (v PlaylistsView) Render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	pv := View{
		Level:            v.Level,
		totalPlaylists:   len(v.Playlists),
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
	return v.renderFiltered(pv)
}

func (v PlaylistsView) renderFiltered(pv View) string {
	body := pv.Render()
	if v.hasNoMatches(pv) {
		body = ansi.Truncate(filter.NoMatchesMessage(true, false), v.Width, "…")
		if v.AtListLevel() {
			body = pv.listHeading() + "\n" + body
		}
		if !v.AtListLevel() {
			body = pv.renderTrackHeading() + "\n" + pv.renderTrackSummary() + "\n" + body
		}
	}
	if pv.FilterVisible {
		filterLine := ansi.Truncate("Filter: "+v.Filter.View(), v.Width, "…")
		if v.AtListLevel() {
			heading := pv.listHeading() + "\n"
			return heading + filterLine + "\n" + strings.TrimPrefix(body, heading)
		}
		return filterLine + "\n" + body
	}
	return body
}

func (v PlaylistsView) hasNoMatches(pv View) bool {
	if v.Filter.Text == "" {
		return false
	}
	if v.AtListLevel() {
		return len(pv.Playlists) == 0
	}
	return len(pv.Tracks) == 0
}
