package tui

import (
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
)

// favoriteToggledMsg carries the result of a single favorite toggle operation.
type favoriteToggledMsg struct {
	typ   string // "artist", "album", or "track"
	value string
	added bool // true if added, false if removed
	count int  // new total count for the type (for status bar updates)
	err   error

	removedIdx     int             // pre-removal index in unfiltered slice (-1 if N/A)
	removedTrack   library.Track   // the removed Track (for track type)
	removedValues  []string        // all removed values (bulk)
	removedTracks  []library.Track // all removed Track objects (bulk, track type)
	removedIndices []int           // original unfiltered indices (bulk)
	addedValues    []string        // all added values (bulk add, for undo tracking)
	favLevel       favtui.Level
	favCategory    favtui.Category

	fromTargetPicker    bool
	pickerOriginView    viewID
	pickerOriginLevel   favtui.Level
	pickerDrilldownName string
}

// favoritesLoadedMsg carries the favorites loaded for a specific category.
type favoritesLoadedMsg struct {
	category int    // 0=artists, 1=albums, 2=tracks
	token    uint64 // request token; results for a superseded request are dropped
	artists  []string
	albums   []string
	tracks   []library.Track
	err      error
}

// watchBatchMsg carries tracks discovered by the filesystem watcher and paths
// of files deleted/renamed-away.
type watchBatchMsg struct {
	tracks  []library.Track
	deleted []string
}

// smartPlaylistCreatedMsg is dispatched after a successful CreateSmartPlaylist.
type smartPlaylistCreatedMsg struct {
	id        int64
	playlists []db.PlaylistRow
}

// smartPlaylistEditedMsg is dispatched after a successful
// UpdateSmartPlaylistQuery call.
type smartPlaylistEditedMsg struct {
	playlists []db.PlaylistRow
}

// smartPlaylistCountsUpdatedMsg carries actual track counts for smart
// playlists, evaluated in the background by evalSmartPlaylistCountsCmd.
type smartPlaylistCountsUpdatedMsg struct {
	counts map[int64]int
}

// tagToggledMsg is dispatched when a tag toggle (add or remove) completes.
type tagToggledMsg struct {
	tagID       int64
	hashes      []string
	wasAdd      bool
	autoDeleted bool // tag was auto-soft-deleted because it became empty
	err         error
}

// tagCreatedMsg is dispatched when a new tag is successfully created.
type tagCreatedMsg struct {
	id     int64
	name   string
	hashes []string
	err    error
}

// tagsRefreshedMsg carries the full tag list and per-track tag map after any
// tag mutation.
type tagsRefreshedMsg struct {
	tags      []db.TagRow
	trackTags map[string]map[int64]struct{}
	err       error
}

// tagViewTracksLoadedMsg carries the tracks for a tag drill-down.
type tagViewTracksLoadedMsg struct {
	tag    db.TagRow
	tracks []db.PlaylistTrackRow
	counts map[int64]int
	err    error
}

// tagViewCountsLoadedMsg carries refreshed tag counts after a mutation.
type tagViewCountsLoadedMsg struct {
	counts map[int64]int
	err    error
}

// tagRenamedMsg carries the result of a RenameTag call.
type tagRenamedMsg struct {
	id   int64
	name string
	err  error
}

// tagDeletedMsg carries the result of a DeleteTagSoft call.
type tagDeletedMsg struct {
	tag db.TagRow // the tag that was soft-deleted (for undo)
	idx int       // canonical position in the unfiltered list (for undo/error recovery)
	err error
}

// tagMergedMsg carries the result of a MergeTags call.
type tagMergedMsg struct {
	src  db.TagRow
	dest db.TagRow
	err  error
}

// openTagFilterPickerMsg requests opening the tag filter picker overlay.
type openTagFilterPickerMsg struct{}

// clearTagFilterMsg requests clearing the active cross-view tag filter.
type clearTagFilterMsg struct{}

// removeAllTagsFromTrackMsg requests removing all tags from the focused track.
type removeAllTagsFromTrackMsg struct {
	hash string
}

// allTagsRemovedMsg is dispatched after the "remove all tags from track"
// operation completes.
type allTagsRemovedMsg struct {
	hash              string
	autoDeletedTagIDs []int64
}

// emptyTagsPurgedMsg is dispatched after the startup cleanup of empty tags.
type emptyTagsPurgedMsg struct {
	tagIDs []int64
}

// enqueueByTagMsg requests enqueueing all tracks for a specific tag ID.
type enqueueByTagMsg struct {
	tagID int64
	name  string
}
