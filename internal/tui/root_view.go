package tui

import "github.com/sglavoie/muse/internal/tui/styles"

// applyNowPlayingPulse returns a copy of s with Reverse(true) applied to every
// lipgloss.Style used by per-view renders to mark the now-playing row.
func applyNowPlayingPulse(s styles.Styles) styles.Styles {
	s.NowPlaying = s.NowPlaying.Reverse(true)
	s.NowPlayingFavorite = s.NowPlayingFavorite.Reverse(true)
	s.NowPlayingFavArtist = s.NowPlayingFavArtist.Reverse(true)
	s.NowPlayingFavAlbum = s.NowPlayingFavAlbum.Reverse(true)
	s.NowPlayingTitle = s.NowPlayingTitle.Reverse(true)
	s.NowPlayingRow = s.NowPlayingRow.Reverse(true)
	s.NowPlayingRowFavorite = s.NowPlayingRowFavorite.Reverse(true)
	s.NowPlayingRowFavArtist = s.NowPlayingRowFavArtist.Reverse(true)
	s.NowPlayingRowFavAlbum = s.NowPlayingRowFavAlbum.Reverse(true)
	return s
}

// hasActiveOverlay reports whether any overlay or the help screen is
// currently displayed.
func (m Model) hasActiveOverlay() bool {
	return m.welcomeVisible() ||
		m.helpVisible ||
		m.debugVisible ||
		m.overlay.settings != nil ||
		m.overlay.nowPlaying != nil ||
		m.overlay.queueSummary != nil ||
		m.overlay.messages != nil ||
		m.overlay.modesLegend != nil ||
		m.overlay.trackInfo != nil ||
		m.overlay.preview != nil ||
		m.overlay.metadataEditor != nil ||
		m.overlay.metadataImport != nil ||
		m.overlay.destPicker != nil ||
		m.overlay.goTo != nil ||
		m.overlay.smartRuleEditor != nil ||
		m.overlay.commandPalette != nil ||
		m.overlay.tagPicker != nil ||
		m.overlay.tagMergePicker != nil ||
		m.overlay.tagFilterPicker != nil ||
		m.overlay.favoriteTargetPicker != nil ||
		m.overlay.autoDJ != nil
}
