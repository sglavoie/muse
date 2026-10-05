package tui

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/fsutil"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

const sortPreferencesFile = "sort_preferences.json"

// Version 1 stores the existing view-level and sort enums. Bump the version
// if those identities change; incompatible preferences fall back to defaults.
type savedSortPreferences struct {
	Version int                          `json:"version"`
	Views   map[string]sorting.SortState `json:"views"`
}

// Keep list sorts separate from track sorts, and each Library mode separate
// from the others. Preferences apply to a view level, never a specific album,
// playlist, query, or track identity.
func (m Model) sortPreferenceKey() string {
	switch m.ActiveView {
	case viewLibrary:
		return fmt.Sprintf("library.%d", m.library.Mode)
	case viewQueue:
		return "queue"
	case viewPlaylists:
		return fmt.Sprintf("playlists.%d", m.playlists.Level)
	case viewBrowse:
		return m.browseSortPreferenceKey()
	case viewFavorites:
		return fmt.Sprintf("favorites.%d.%d", m.favorites.Category, m.favorites.Level)
	case viewTags:
		return fmt.Sprintf("tags.%d", m.tagsView.Level)
	case viewMixes:
		return fmt.Sprintf("mixes.%d", m.mixes.Level)
	case viewInsights:
		if m.insights.AtTracks() {
			return "insights.tracks"
		}
	}
	return ""
}

func (m Model) browseSortPreferenceKey() string {
	switch m.browse.Mode {
	case browse.BrowseModeArtists:
		return fmt.Sprintf("browse.artists.%d", m.browse.Artists.Level)
	case browse.BrowseModeAlbums:
		return fmt.Sprintf("browse.albums.%d", m.browse.Albums.Level)
	case browse.BrowseModeGenres:
		return fmt.Sprintf("browse.genres.%d", m.browse.Genres.Level)
	}
	return ""
}

func (m Model) loadSortPreferences() Model {
	if m.dataDir == "" {
		return m
	}
	data, err := os.ReadFile(filepath.Join(m.dataDir, sortPreferencesFile))
	if errors.Is(err, fs.ErrNotExist) {
		return m
	}
	var saved savedSortPreferences
	if err == nil {
		err = json.Unmarshal(data, &saved)
	}
	if err != nil {
		slog.Warn("sort preferences: load failed, using defaults", "err", err)
		return m
	}
	if saved.Version == 1 {
		m.sortPreferences = saved.Views
	}
	return m
}

func rememberSortResult(next tea.Model, cmd tea.Cmd, handled bool) (tea.Model, tea.Cmd, bool) {
	if handled {
		next = next.(Model).rememberSortPreference()
	}
	return next, cmd, handled
}

func (m Model) rememberSortPreference() Model {
	fields, state := m.activeSortFields()
	key := m.sortPreferenceKey()
	if key == "" || !slices.Contains(fields, state.Field) {
		return m
	}
	// Copy on write: older Model values must not observe later preferences.
	m.sortPreferences = maps.Clone(m.sortPreferences)
	if m.sortPreferences == nil {
		m.sortPreferences = make(map[string]sorting.SortState)
	}
	m.sortPreferences[key] = state
	if m.dataDir == "" {
		return m
	}
	// Serialize small atomic writes on the update loop, as with filter history,
	// so rapid direction changes cannot save an older value after a newer one.
	err := fsutil.WriteAtomically(filepath.Join(m.dataDir, sortPreferencesFile), 0o600, func(f *os.File) error {
		return json.NewEncoder(f).Encode(savedSortPreferences{Version: 1, Views: m.sortPreferences})
	})
	if err != nil {
		slog.Warn("sort preferences: save failed", "err", err)
		m.statusBar.NotifyError("Sort changed (not saved — won't survive restart)")
	}
	return m
}

func (m Model) restoreSortPreference() Model {
	saved, ok := m.sortPreferences[m.sortPreferenceKey()]
	fields, current := m.activeSortFields()
	if !ok || saved == current || !slices.Contains(fields, saved.Field) {
		return m
	}
	if saved.Dir != sorting.SortAsc && saved.Dir != sorting.SortDesc {
		return m
	}
	if saved.Field == sorting.SortDefault {
		saved.Dir = sorting.SortAsc
	}
	state, cursor, scroll := m.sortPreferenceTarget()
	if state == nil {
		return m
	}
	position := filter.CapturePosition(m.filterRowKeys(), *cursor, *scroll)
	*state = saved
	m = m.recomputePreferredSort()
	_, cursor, scroll = m.sortPreferenceTarget()
	*cursor, *scroll = position.Restore(m.filterRowKeys(), position.Key)
	m.clampAllScrollOffsets()
	return m
}
