package tui

// viewID identifies which primary view is active.
type viewID int

const (
	viewLibrary   viewID = 1
	viewPlaylists viewID = 2
	viewQueue     viewID = 3
	viewBrowse    viewID = 4
	viewFavorites viewID = 5
	viewTags      viewID = 6
	viewMixes     viewID = 7

	// Legacy sub-view IDs — not in viewOrder; kept until all dispatch sites migrate to viewBrowse.
	viewArtists viewID = 10
	viewAlbums  viewID = 11
	viewGenres  viewID = 12
)

// viewOrder defines the tab-cycling order for primary views.
var viewOrder = []viewID{viewLibrary, viewPlaylists, viewQueue, viewBrowse, viewFavorites, viewTags, viewMixes}

// viewName returns the display name for a viewID.
func viewName(id viewID) string {
	switch id {
	case viewLibrary:
		return "Library"
	case viewPlaylists:
		return "Playlists"
	case viewQueue:
		return "Queue"
	case viewBrowse:
		return "Browse"
	case viewArtists:
		return "Artists"
	case viewAlbums:
		return "Albums"
	case viewGenres:
		return "Genres"
	case viewFavorites:
		return "Favorites"
	case viewTags:
		return "Tags"
	case viewMixes:
		return "Mixes"
	default:
		return ""
	}
}

// viewDisplayName returns the mode-aware display name for a viewID.
// For the Library view it includes the active sub-mode suffix when not in All mode.
func (m Model) viewDisplayName(id viewID) string {
	if id == viewLibrary {
		switch m.library.mode {
		case LibraryModeRecent:
			return "Library (Recent)"
		case LibraryModeHistory:
			return "Library (History)"
		}
	}
	if id == viewBrowse {
		switch m.browse.mode {
		case BrowseModeArtists:
			return "Browse (Artists)"
		case BrowseModeAlbums:
			return "Browse (Albums)"
		case BrowseModeGenres:
			return "Browse (Genres)"
		}
	}
	if id == viewQueue && m.queueDirty {
		return "Queue*"
	}
	return viewName(id)
}

// viewToContext maps a viewID to its corresponding bindingContext.
func viewToContext(id viewID) bindingContext {
	switch id {
	case viewLibrary:
		return contextLibrary
	case viewPlaylists:
		return contextPlaylists
	case viewQueue:
		return contextQueue
	case viewBrowse:
		return contextBrowse
	case viewArtists:
		return contextArtists
	case viewAlbums:
		return contextAlbums
	case viewGenres:
		return contextGenres
	case viewFavorites:
		return contextFavorites
	case viewTags:
		return contextTags
	case viewMixes:
		return contextMixes
	default:
		return contextGlobal
	}
}

// prevView returns the previous view in the cycle (wrapping around).
func prevView(id viewID) viewID {
	for i, v := range viewOrder {
		if v == id {
			return viewOrder[(i-1+len(viewOrder))%len(viewOrder)]
		}
	}
	return viewOrder[0]
}

// nextView returns the next view in the cycle (wrapping around).
func nextView(id viewID) viewID {
	for i, v := range viewOrder {
		if v == id {
			return viewOrder[(i+1)%len(viewOrder)]
		}
	}
	return viewOrder[0]
}
