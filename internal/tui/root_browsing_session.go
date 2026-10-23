package tui

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"github.com/sglavoie/muse/internal/fsutil"
	"github.com/sglavoie/muse/internal/tui/browse"
	"github.com/sglavoie/muse/internal/tui/filter"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	libtui "github.com/sglavoie/muse/internal/tui/library"
	"github.com/sglavoie/muse/internal/tui/sorting"
)

const browsingSessionFile = "browsing_session.json"

// Only navigation is persisted: no selections, open editors or playback actions.
// Steps run from the root collection to the current list and use durable row keys.
type browsingStep struct {
	Level    string            `json:"level"`
	Position filter.Position   `json:"position"`
	TopKey   string            `json:"top_key,omitempty"`
	Query    string            `json:"query,omitempty"`
	Origin   filter.Position   `json:"origin,omitempty"`
	Sort     sorting.SortState `json:"sort"`
}

type browsingSession struct {
	Version     int                `json:"version"`
	View        viewID             `json:"view"`
	LibraryMode libtui.LibraryMode `json:"library_mode,omitempty"`
	BrowseMode  browse.BrowseMode  `json:"browse_mode,omitempty"`
	Panel       instui.Panel       `json:"panel,omitempty"`
	Window      int                `json:"window,omitempty"`
	Resolution  int                `json:"resolution,omitempty"`
	TagFilter   *int64             `json:"tag_filter,omitempty"`
	Steps       []browsingStep     `json:"steps"`
}

type browsingRestore struct {
	Session browsingSession
	Started bool
	Next    int
}

func (m Model) captureBrowsingSession() browsingSession {
	s := browsingSession{Version: 1, View: m.ActiveView, LibraryMode: m.library.Mode,
		BrowseMode: m.browse.Mode, Panel: m.insights.Panel, Window: m.insights.WindowIdx,
		Resolution: m.insights.ResolutionIndex(), TagFilter: m.activeTagFilter}
	for {
		step := browsingStep{Level: m.browsingLevel(), Position: m.browsingPosition()}
		rows := m.browsingKeys()
		if step.Position.Scroll >= 0 && step.Position.Scroll < len(rows) {
			step.TopKey = rows[step.Position.Scroll]
		}
		if f := m.browsingFilter(); f != nil {
			step.Query, step.Origin = f.Text, f.Origin
		}
		if sort, _, _ := m.sortPreferenceTarget(); sort != nil {
			step.Sort = *sort
		}
		s.Steps = append(s.Steps, step)
		parent, ok := m.browsingBack()
		if !ok {
			break
		}
		m = parent
	}
	slices.Reverse(s.Steps)
	return s
}

// SaveBrowsingSession runs after Bubble Tea exits, including signal shutdown,
// before the database writer closes. Atomic replacement preserves the previous
// session if saving fails. Tests and fixture sessions without a data dir do no IO.
func (m Model) SaveBrowsingSession() error {
	if m.dataDir == "" || m.fixtureSafeCommands {
		return nil
	}
	saved := m.captureBrowsingSession()
	if m.browsingRestore != nil {
		saved = m.browsingRestore.Session
	}
	return fsutil.WriteAtomically(filepath.Join(m.dataDir, browsingSessionFile), 0o600, func(f *os.File) error {
		return json.NewEncoder(f).Encode(saved)
	})
}

func (m Model) loadBrowsingSession() Model {
	if m.dataDir == "" || m.fixtureSafeCommands {
		return m
	}
	data, err := os.ReadFile(filepath.Join(m.dataDir, browsingSessionFile))
	if errors.Is(err, fs.ErrNotExist) {
		return m
	}
	var saved browsingSession
	if err == nil {
		err = json.Unmarshal(data, &saved)
	}
	if err != nil {
		slog.Warn("browsing session: load failed", "err", err)
		return m
	}
	if saved.Version != 1 || !slices.Contains(viewOrder, saved.View) || len(saved.Steps) == 0 || len(saved.Steps) > 4 {
		return m
	}
	saved.LibraryMode = min(saved.LibraryMode, libtui.LibraryModeHistory)
	saved.BrowseMode = browse.BrowseMode(max(0, min(int(saved.BrowseMode), 2)))
	m.browsingRestore = &browsingRestore{Session: saved}
	return m
}
