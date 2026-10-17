package tui

import (
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	"github.com/sglavoie/muse/internal/tui/browse"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	"github.com/sglavoie/muse/internal/tui/keys"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// hasFooterTarget checks the displayed rows without fetching collection contents.
func (m Model) hasFooterTarget() bool {
	switch m.ActiveView {
	case viewQueue:
		return len(m.queue.FilteredEntries()) > 0
	case viewPlaylists:
		return m.playlistsHasFooterTarget()
	case viewTags:
		if _, ok := m.tagsView.FocusedTag(); ok {
			return true
		}
	case viewBrowse:
		return m.browseHasFooterTarget()
	case viewFavorites:
		return m.favoritesHasFooterTarget()
	case viewMixes:
		return m.mixesHasFooterTarget()
	case viewInsights:
		if !m.insights.AtTracks() {
			return m.library.Index.Len() > 0
		}
	}
	_, ok := m.focusedTrackForPreview()
	return ok
}

func (m Model) browseHasFooterTarget() bool {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		if _, ok := m.browse.Artists.FocusedArtist(); ok {
			return true
		}
		if _, ok := m.browse.Artists.FocusedAlbum(); ok {
			return true
		}
	case browse.BrowseModeAlbums:
		if _, ok := m.browse.Albums.FocusedAlbum(); ok {
			return true
		}
	case browse.BrowseModeGenres:
		if _, ok := m.browse.Genres.FocusedGenre(); ok {
			return true
		}
		if _, ok := m.browse.Genres.FocusedArtist(); ok {
			return true
		}
	}
	_, ok := m.focusedTrackForPreview()
	return ok
}

func (m Model) emptyHintMode() keys.HintMode {
	if m.hasCommittedFilter() || m.activeTagFilter != nil {
		return keys.HintModeNoMatches
	}
	if m.emptyCollectionOpen() {
		return keys.HintModeEmptyTracks
	}
	return keys.HintModeEmpty
}

// Empty library-dependent views share the same actionable card as other views.
func (m Model) renderEmptyPrimary(display styles.Styles) (string, bool) {
	title := ""
	action := styles.EmptyAction{Key: "1", Label: "go to library"}
	switch {
	case m.ActiveView == viewBrowse && m.library.Index.Len() == 0:
		title = "No music to browse"
		action = styles.EmptyAction{Key: "ctrl+,", Label: "set music directories"}
	case m.ActiveView == viewMixes && m.library.Index.Len() == 0:
		title = "Add music to build your mixes"
		action = styles.EmptyAction{Key: "ctrl+,", Label: "set music directories"}
	case m.ActiveView == viewFavorites && m.favorites.Level == favtui.CategoryLevel && m.favorites.ArtistCount+m.favorites.AlbumCount+m.favorites.TrackCount == 0:
		title = "No favorites yet"
		action.Label = "open library, then F to favorite"
	default:
		return "", false
	}
	height := max(m.termHeight-5, 1)
	heading := m.emptyPrimaryHeading(display)
	if heading != "" {
		height--
	}
	return heading + display.RenderEmptyCard(m.termWidth, height, title, []styles.EmptyAction{action, {Key: "?", Label: "help"}}), true
}

func (m Model) footerOpensCollectionOnly() bool {
	if m.ActiveView == viewFavorites && m.favorites.Level == favtui.CategoryLevel {
		return true
	}
	if m.ActiveView == viewMixes && m.mixes.Level == 0 && m.mixes.SelectedMix >= 0 && m.mixes.SelectedMix < len(m.mixes.Mixes) {
		return m.mixes.Mixes[m.mixes.SelectedMix].TrackCount == 0
	}
	return false
}

func (m Model) favoritesHasFooterTarget() bool {
	if m.favorites.Level == favtui.CategoryLevel {
		return m.favorites.ArtistCount+m.favorites.AlbumCount+m.favorites.TrackCount > 0
	}
	if _, ok := m.favorites.FocusedArtist(); ok {
		return true
	}
	if _, ok := m.favorites.FocusedAlbum(); ok {
		return true
	}
	_, ok := m.favorites.FocusedTrack()
	return ok
}
func (m Model) mixesHasFooterTarget() bool {
	if m.library.Index.Len() == 0 {
		return false
	}
	if m.mixes.Level == 0 {
		return len(m.mixes.Mixes) > 0
	}
	return m.mixes.FocusedTrack() != nil
}

func (m Model) emptyCollectionOpen() bool {
	switch m.ActiveView {
	case viewPlaylists:
		return !m.playlists.AtListLevel()
	case viewMixes:
		return m.mixes.Level == 1
	case viewTags:
		return m.tagsView.Level == tagtui.TrackLevel
	case viewFavorites:
		return m.favorites.Level != favtui.CategoryLevel
	case viewBrowse:
		return m.library.Index.Len() > 0
	}
	return false
}

func (m Model) playlistsHasFooterTarget() bool {
	if m.playlists.AtListLevel() {
		return len(m.playlists.EffectivePlaylists()) > 0
	}
	_, ok := m.playlists.FocusedTrack()
	return ok
}

// atCollectionTrackLevel distinguishes collection navigation from the
// enqueue performed by Enter at track level.
func (m Model) atCollectionTrackLevel() bool {
	switch m.ActiveView {
	case viewPlaylists:
		return !m.playlists.AtListLevel()
	case viewTags:
		return m.tagsView.Level == tagtui.TrackLevel
	case viewFavorites:
		return m.favorites.Level == favtui.TrackLevel ||
			(m.favorites.Level == favtui.ListLevel && m.favorites.Category == favtui.CategoryTracks)
	case viewBrowse:
		switch m.browse.Mode {
		case browse.BrowseModeArtists:
			return m.browse.Artists.Level == arttui.TrackLevel
		case browse.BrowseModeAlbums:
			return m.browse.Albums.Level == albtui.TrackLevel
		case browse.BrowseModeGenres:
			return m.browse.Genres.Level == gentui.TrackLevel
		}
	}
	return false
}

func (m Model) emptyPrimaryHeading(display styles.Styles) string {
	heading := ""
	switch m.ActiveView {
	case viewFavorites:
		heading = strip.Collection(m.termWidth, "Favorites", "category", 3, 3, display) + "\n"
	case viewMixes:
		heading = strip.Collection(m.termWidth, "Mixes", "mix", 0, 0, display) + "\n"
	}
	return heading
}
