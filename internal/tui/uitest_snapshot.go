package tui

// This file derives a compact UITestSnapshot from a live Model for the hidden
// `muse tui-test` layout harness. Snapshots expose only stable, high-signal
// state (active view/overlay, level/mode, cursor/scroll, visible and selection
// counts, focused item) — never full track structs, playlist rows, or rendered
// tables. All reads are pure: Snapshot never mutates the model or triggers I/O.

import (
	"fmt"

	"github.com/sglavoie/muse/internal/library"
	albtui "github.com/sglavoie/muse/internal/tui/albums"
	arttui "github.com/sglavoie/muse/internal/tui/artists"
	favtui "github.com/sglavoie/muse/internal/tui/favorites"
	gentui "github.com/sglavoie/muse/internal/tui/genres"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	pltui "github.com/sglavoie/muse/internal/tui/playlists"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// Snapshot returns a compact, high-signal digest of the model's current state
// for a UITestObservation. It reads only stable fields and performs no side
// effects.
func (m Model) Snapshot() *UITestSnapshot {
	snap := &UITestSnapshot{
		PrimaryView:  viewName(m.activeView),
		Overlay:      m.activeOverlayName(),
		QueueLength:  len(m.queue.entries),
		CurrentIndex: m.playback.currentIndex,
	}
	switch m.activeView {
	case viewLibrary:
		m.fillLibrarySnapshot(snap)
	case viewQueue:
		m.fillQueueSnapshot(snap)
	case viewBrowse:
		m.fillBrowseSnapshot(snap)
	case viewFavorites:
		m.fillFavoritesSnapshot(snap)
	case viewTags:
		m.fillTagsSnapshot(snap)
	case viewPlaylists:
		m.fillPlaylistsSnapshot(snap)
	case viewMixes:
		m.fillMixesSnapshot(snap)
	case viewInsights:
		m.fillInsightsSnapshot(snap)
	}
	return snap
}

// activeOverlayName returns a stable name for the currently displayed overlay,
// checked in the same priority order as renderOverlayBody. It returns "" when
// no overlay is active.
func (m Model) activeOverlayName() string {
	switch {
	case m.welcomeVisible():
		return "welcome"
	case m.helpVisible:
		return "help"
	case m.debugVisible:
		return "debug"
	case m.overlay.settings != nil:
		return "settings"
	case m.overlay.nowPlaying != nil:
		return "now_playing"
	case m.overlay.queueSummary != nil:
		return "queue_summary"
	case m.overlay.modesLegend != nil:
		return "modes_legend"
	case m.overlay.trackInfo != nil:
		return "track_info"
	case m.overlay.preview != nil:
		return "preview"
	case m.overlay.metadataEditor != nil:
		return "metadata_editor"
	case m.overlay.destPicker != nil:
		return "destination_picker"
	case m.overlay.goTo != nil:
		return "goto"
	case m.overlay.smartRuleEditor != nil:
		return "smart_rule_editor"
	case m.overlay.commandPalette != nil:
		return "command_palette"
	case m.overlay.tagPicker != nil:
		return "tag_picker"
	case m.overlay.tagMergePicker != nil:
		return "tag_merge_picker"
	case m.overlay.tagFilterPicker != nil:
		return "tag_filter_picker"
	case m.overlay.favoriteTargetPicker != nil:
		return "favorite_target_picker"
	default:
		return ""
	}
}

func (m Model) fillLibrarySnapshot(snap *UITestSnapshot) {
	lib := m.library
	snap.Mode = libtui.StripLabel(lib.Mode)
	snap.Cursor = lib.Cursor
	snap.ScrollOffset = lib.ScrollOffset
	snap.VisibleCount = lib.TrackCount()
	snap.SelectionCount = len(lib.SelectedHashes)
	if t, ok := lib.Panel().FocusedTrack(); ok {
		snap.FocusedHash = t.Hash
		snap.FocusedLabel = trackLabel(t)
	}
}

func (m Model) fillQueueSnapshot(snap *UITestSnapshot) {
	q := m.queue
	snap.Cursor = q.cursor
	snap.ScrollOffset = q.scrollOffset
	snap.VisibleCount = len(q.entries)
	selected := 0
	for _, e := range q.entries {
		if e.Selected {
			selected++
		}
	}
	snap.SelectionCount = selected
	if q.cursor >= 0 && q.cursor < len(q.entries) {
		e := q.entries[q.cursor]
		snap.FocusedHash = e.Hash
		snap.FocusedLabel = e.Display
	}
}

func (m Model) fillBrowseSnapshot(snap *UITestSnapshot) {
	switch m.browse.mode {
	case BrowseModeArtists:
		v := m.browse.artists
		snap.Mode = "artists"
		snap.Level = artistsLevelName(v.level)
		snap.Cursor = v.cursor
		snap.ScrollOffset = v.scrollOffset
		snap.SelectionCount = len(v.selectedHashes)
	case BrowseModeAlbums:
		v := m.browse.albums
		snap.Mode = "albums"
		snap.Level = albumsLevelName(v.level)
		snap.Cursor = v.cursor
		snap.ScrollOffset = v.scrollOffset
		snap.SelectionCount = len(v.selectedHashes)
	case BrowseModeGenres:
		v := m.browse.genres
		snap.Mode = "genres"
		snap.Level = genresLevelName(v.level)
		snap.Cursor = v.cursor
		snap.ScrollOffset = v.scrollOffset
		snap.SelectionCount = len(v.selectedHashes)
	}
}

func (m Model) fillFavoritesSnapshot(snap *UITestSnapshot) {
	f := m.favorites
	snap.Level = favoritesLevelName(f.Level)
	snap.Cursor = f.Cursor
	snap.ScrollOffset = f.ScrollOffset
	snap.SelectionCount = len(f.SelectedItems)
	switch f.Level {
	case favtui.CategoryLevel:
		snap.VisibleCount = 3
	case favtui.TrackLevel:
		snap.VisibleCount = len(f.DrilldownTracks)
	default:
		// ListLevel: exactly one of the item slices is populated.
		snap.VisibleCount = maxInt(len(f.Artists), len(f.Albums), len(f.Tracks))
	}
}

func (m Model) fillTagsSnapshot(snap *UITestSnapshot) {
	t := m.tagsView
	snap.Level = tagsLevelName(t.Level)
	snap.Cursor = t.Cursor
	snap.ScrollOffset = t.ScrollOffset
	snap.SelectionCount = len(t.SelectedHashes)
	if t.Level == tagtui.TrackLevel {
		snap.VisibleCount = len(t.Tracks)
	} else {
		snap.VisibleCount = len(t.Tags)
	}
}

func (m Model) fillPlaylistsSnapshot(snap *UITestSnapshot) {
	p := m.playlists
	snap.Level = playlistsLevelName(p.Level)
	snap.Cursor = p.Cursor
	snap.ScrollOffset = p.ScrollOffset
	snap.SelectionCount = len(p.SelectedTrackIDs)
	if p.Level == pltui.TrackLevel {
		snap.VisibleCount = len(p.Tracks)
	} else {
		snap.VisibleCount = len(p.Playlists)
	}
}

func (m Model) fillMixesSnapshot(snap *UITestSnapshot) {
	mx := m.mixes
	if mx.level == 0 {
		snap.Level = "list"
		snap.Cursor = mx.selectedMix
		snap.VisibleCount = len(mx.mixes)
		return
	}
	snap.Level = "tracks"
	snap.Cursor = mx.trackView.Cursor
	snap.ScrollOffset = mx.trackView.ScrollOffset
	snap.VisibleCount = mx.trackView.TrackCount()
	snap.SelectionCount = len(mx.trackView.SelectedHashes)
	if t, ok := mx.trackView.Panel().FocusedTrack(); ok {
		snap.FocusedHash = t.Hash
		snap.FocusedLabel = trackLabel(t)
	}
}

func (m Model) fillInsightsSnapshot(snap *UITestSnapshot) {
	iv := m.insights
	if iv.level == 0 {
		snap.Level = "briefing"
		snap.Mode = iv.panelSpec().name
		snap.Cursor = iv.cursor
		snap.ScrollOffset = iv.scrollOffset
		rows := iv.rows()
		snap.VisibleCount = len(rows)
		if iv.cursor >= 0 && iv.cursor < len(rows) {
			snap.FocusedLabel = rows[iv.cursor].label
		}
		return
	}
	snap.Level = "tracks"
	snap.Mode = iv.drillLabel
	snap.Cursor = iv.trackView.Cursor
	snap.ScrollOffset = iv.trackView.ScrollOffset
	snap.VisibleCount = iv.trackView.TrackCount()
	snap.SelectionCount = len(iv.trackView.SelectedHashes)
	if t, ok := iv.trackView.Panel().FocusedTrack(); ok {
		snap.FocusedHash = t.Hash
		snap.FocusedLabel = trackLabel(t)
	}
}

// trackLabel renders a compact "Artist – Title" label for snapshot focus fields.
func trackLabel(t library.Track) string {
	return fmt.Sprintf("%s – %s", t.Artist, t.Title)
}

func maxInt(vals ...int) int {
	m := 0
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

func artistsLevelName(l arttui.Level) string {
	switch l {
	case arttui.ArtistLevel:
		return "artist"
	case arttui.AlbumLevel:
		return "album"
	case arttui.TrackLevel:
		return "track"
	}
	return ""
}

func albumsLevelName(l albtui.Level) string {
	switch l {
	case albtui.AlbumLevel:
		return "album"
	case albtui.TrackLevel:
		return "track"
	}
	return ""
}

func genresLevelName(l gentui.Level) string {
	switch l {
	case gentui.GenreLevel:
		return "genre"
	case gentui.ArtistLevel:
		return "artist"
	case gentui.TrackLevel:
		return "track"
	}
	return ""
}

func favoritesLevelName(l favtui.Level) string {
	switch l {
	case favtui.CategoryLevel:
		return "category"
	case favtui.ListLevel:
		return "list"
	case favtui.TrackLevel:
		return "track"
	}
	return ""
}

func tagsLevelName(l tagtui.Level) string {
	switch l {
	case tagtui.TagLevel:
		return "tag"
	case tagtui.TrackLevel:
		return "track"
	}
	return ""
}

func playlistsLevelName(l pltui.Level) string {
	switch l {
	case pltui.ListLevel:
		return "list"
	case pltui.TrackLevel:
		return "track"
	}
	return ""
}
