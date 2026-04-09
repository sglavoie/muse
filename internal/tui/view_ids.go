package tui

// viewID identifies which primary view is active.
type viewID int

const (
	viewLibrary       viewID = 1
	viewPlaylists     viewID = 2
	viewQueue         viewID = 3
	viewArtists       viewID = 4
	viewAlbums        viewID = 5
	viewFavorites     viewID = 6
	viewGenres        viewID = 7
	viewRecentlyAdded viewID = 8
	viewHistory       viewID = 9
	viewTags          viewID = 10
)

// viewOrder defines the tab-cycling order for primary views.
var viewOrder = []viewID{viewLibrary, viewPlaylists, viewQueue, viewArtists, viewAlbums, viewGenres, viewFavorites, viewRecentlyAdded, viewHistory, viewTags}

// viewName returns the display name for a viewID.
func viewName(id viewID) string {
	switch id {
	case viewLibrary:
		return "Library"
	case viewPlaylists:
		return "Playlists"
	case viewQueue:
		return "Queue"
	case viewArtists:
		return "Artists"
	case viewAlbums:
		return "Albums"
	case viewFavorites:
		return "Favorites"
	case viewGenres:
		return "Genres"
	case viewRecentlyAdded:
		return "Recently Added"
	case viewHistory:
		return "History"
	case viewTags:
		return "Tags"
	default:
		return ""
	}
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
	case viewArtists:
		return contextArtists
	case viewAlbums:
		return contextAlbums
	case viewFavorites:
		return contextFavorites
	case viewGenres:
		return contextGenres
	case viewRecentlyAdded:
		return contextRecentlyAdded
	case viewHistory:
		return contextHistory
	case viewTags:
		return contextTags
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
