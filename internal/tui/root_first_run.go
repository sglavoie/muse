package tui

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/config"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// firstRunCovers is how many of the newest albums the first-scan screen shows.
const firstRunCovers = 4

// The first-scan covers are small squares: half blocks draw two pixel rows per
// cell, so twice as many columns as rows.
const (
	firstRunCoverRows = 4
	firstRunCoverCols = 2 * firstRunCoverRows
)

// museWordmark is the name drawn in box characters for the first-run screen.
// It is 15 cells wide and three rows tall so it fits the 60×15 minimum.
var museWordmark = []string{
	"┌┬┐ ┬ ┬ ┌─┐ ┌─┐",
	"│││ │ │ └─┐ ├┤",
	"┴ ┴ └─┘ └─┘ └─┘",
}

// firstRunState carries the welcome screen shown before muse has any music,
// and the live progress of the scan that fills an empty library.
type firstRunState struct {
	// suggestion is the platform music folder found at startup. Its Path is
	// empty when there is none. It is probed once, outside rendering.
	suggestion config.MusicDirSuggestion
	// scanning is true while a scan fills a library that started empty.
	scanning bool
	// browsing hides the progress screen once the user chose to look at the
	// tracks found so far.
	browsing bool
	// folder is the folder of the most recently found track.
	folder string
	// recent holds one track per album, newest first, for the cover row.
	recent []library.Track
	// heldWelcome records that the first scan, not anything else, kept the
	// welcome overlay away, so a library that turns out not to be empty can
	// give it back.
	heldWelcome bool
}

// WithMusicDirSuggestion sets the music folder the first-run screen offers to
// add with one key. Pass the result of config.DetectMusicDirectory; the zero
// value offers nothing.
func WithMusicDirSuggestion(s config.MusicDirSuggestion) Option {
	return func(m *Model) { m.firstRun.suggestion = s }
}

// initFirstRun runs once at construction. The first-run screen already says
// what the welcome overlay would, so the overlay stays away while that screen
// is what the user sees: with no folders at all, or while the very first scan
// runs. A folder that failed validation keeps the overlay, which names it.
func (m Model) initFirstRun() Model {
	emptyScan := m.scanning && m.library.Index.Len() == 0
	heldWelcome := false
	if m.musicFoldersUnconfigured() {
		m.welcomeDismissed = true
	} else if emptyScan && m.musicDirHint == "" && !m.welcomeDismissed {
		m.welcomeDismissed, heldWelcome = true, true
	}
	if emptyScan {
		m.firstRun = m.firstRun.begin()
		m.firstRun.heldWelcome = heldWelcome
	}
	return m
}

// noteCachedLibrary revisits initFirstRun's verdict once the cached library
// arrives. muse loads it after building the model, so at construction every
// startup looks like a first scan into an empty library; without this the
// Library would show "Scanning your music…" instead of the tracks it already
// has until the background scan finished.
func (m Model) noteCachedLibrary() Model {
	if !m.firstRun.scanning || m.library.Index.Len() == 0 {
		return m
	}
	if m.firstRun.heldWelcome {
		m.welcomeDismissed = false
	}
	m.firstRun = m.firstRun.finish()
	return m
}

func (s firstRunState) begin() firstRunState {
	return firstRunState{suggestion: s.suggestion, scanning: true}
}

func (s firstRunState) finish() firstRunState {
	return firstRunState{suggestion: s.suggestion}
}

func (m Model) musicFoldersUnconfigured() bool {
	return len(m.cfg.MusicDirectories) == 0 && len(m.libraryRoots) == 0
}

// offeredMusicDir reports the suggested folder when the user has none yet.
func (m Model) offeredMusicDir() (config.MusicDirSuggestion, bool) {
	s := m.firstRun.suggestion
	if s.Path == "" || !m.musicFoldersUnconfigured() || slices.Contains(m.cfg.MusicDirectories, s.Path) {
		return config.MusicDirSuggestion{}, false
	}
	return s, true
}

func (m Model) showsFirstRunWelcome() bool {
	return !m.scanning && m.musicFoldersUnconfigured() && m.showsEmptyLibrary()
}

func (m Model) showsFirstRunProgress() bool {
	return m.firstRun.scanning && m.scanning && !m.firstRun.browsing && m.ActiveView == viewLibrary &&
		!m.library.Filter.Active && m.library.Filter.Text == "" && m.library.TagFilterHashes == nil
}

// handleFirstRunKeyPress owns Enter on the first-run screens: it adds the
// suggested folder, or leaves the scan progress for the tracks found so far.
func (m Model) handleFirstRunKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if msg.String() != "enter" || m.hasActiveOverlay() {
		return m, nil, false
	}
	if m.showsFirstRunProgress() {
		m.firstRun.browsing = true
		return m, nil, true
	}
	if s, ok := m.offeredMusicDir(); ok && m.showsFirstRunWelcome() {
		next, cmd := m.adoptMusicDir(s.Path)
		return next, cmd, true
	}
	return m, nil, false
}

// adoptMusicDir adds dir to the music folders and saves and rescans through
// the same path as Settings. Fixtures have no config file and no extractor,
// so there the save and the scan do nothing.
func (m Model) adoptMusicDir(dir string) (Model, tea.Cmd) {
	oldRoots, oldExcluded := m.libraryRoots, m.cfg.ExcludedPaths
	cfg := m.cfg
	cfg.MusicDirectories = append(slices.Clone(cfg.MusicDirectories), dir)
	cfg.MusicDirectory = cfg.MusicDirectories[0]
	m.cfg = cfg
	return m.saveConfigAndRescan(oldRoots, oldExcluded)
}

// noteFirstScanBatch records where the first scan is and looks up covers for
// the newest albums it found, so they appear while the scan continues.
func (m Model) noteFirstScanBatch(tracks []library.Track, next tea.Cmd) (Model, tea.Cmd) {
	if !m.firstRun.scanning || len(tracks) == 0 {
		return m, next
	}
	m.firstRun.folder = filepath.Dir(tracks[len(tracks)-1].Path)
	recent := make([]library.Track, 0, firstRunCovers)
	seen := map[string]bool{}
	for i := len(tracks) - 1; i >= 0 && len(recent) < firstRunCovers; i-- {
		recent = appendNewAlbum(recent, seen, tracks[i])
	}
	for _, t := range m.firstRun.recent {
		if len(recent) == firstRunCovers {
			break
		}
		recent = appendNewAlbum(recent, seen, t)
	}
	m.firstRun.recent = recent
	cmds := []tea.Cmd{next}
	for _, t := range recent {
		var cmd tea.Cmd
		m, cmd = m.requestArt(t)
		cmds = append(cmds, cmd)
	}
	return m, batchNonNil(cmds...)
}

func appendNewAlbum(recent []library.Track, seen map[string]bool, t library.Track) []library.Track {
	if t.Album == "" || t.Path == "" {
		return recent
	}
	key := library.AlbumKey(t)
	if seen[key] {
		return recent
	}
	seen[key] = true
	return append(recent, t)
}

// firstScanSummary is the notice shown when the first scan finishes.
func (m Model) firstScanSummary() string {
	albums := map[string]bool{}
	for _, t := range m.library.Index.ReadOnlyTracks() {
		if t.Album != "" {
			albums[library.AlbumKey(t)] = true
		}
	}
	tracks := m.library.Index.Len()
	return fmt.Sprintf("Found %s %s across %s %s",
		text.FormatCount(tracks), text.Pluralize(tracks, "track", "tracks"),
		text.FormatCount(len(albums)), text.Pluralize(len(albums), "album", "albums"))
}

// firstRunFooter replaces the footer hints on the first-run screens.
func (m Model) firstRunFooter() (string, bool) {
	var hint string
	var keep []string
	switch {
	case m.showsFirstRunProgress():
		hint, keep = "esc cancel scan · enter browse now · ? help", []string{"esc", "enter"}
	case m.showsFirstRunWelcome():
		if s, ok := m.offeredMusicDir(); ok {
			hint, keep = "enter use "+s.Label+" · ctrl+, choose folders · ? help", []string{"enter", "ctrl+,"}
		} else {
			hint, keep = "ctrl+, add music folders · : commands · ? help", []string{"ctrl+,", "?"}
		}
	default:
		return "", false
	}
	return m.styles.Dim.Render(text.CompactHints(hint, m.termWidth, keep...)), true
}

// renderFirstRun draws the welcome or scan-progress screen in the Library.
func (m Model) renderFirstRun(display styles.Styles) (string, bool) {
	switch {
	case m.showsFirstRunProgress():
		return m.renderFirstRunScreen(display, m.firstRunProgressBlock(display)), true
	case m.showsFirstRunWelcome():
		return m.renderFirstRunScreen(display, m.firstRunWelcomeBlock(display)), true
	}
	return "", false
}

// firstRunBlock is a first-run screen's content below the wordmark. Covers
// and blank spacer rows are given up first when the screen is short.
type firstRunBlock struct {
	lines   []firstRunLine
	covers  string
	actions []styles.EmptyAction
}

// firstRunLine is one centered line; a blank Text is a spacer row.
type firstRunLine struct {
	text  string
	style lipgloss.Style
}

func (m Model) firstRunWelcomeBlock(display styles.Styles) firstRunBlock {
	title, _ := m.emptyLibraryGuidance()
	actions := []styles.EmptyAction{{Key: "ctrl+,", Label: "add music folders"}, {Key: "?", Label: "help"}}
	if s, ok := m.offeredMusicDir(); ok {
		actions = []styles.EmptyAction{{Key: "enter", Label: "use " + s.Label}, {Key: "ctrl+,", Label: "choose folders"}, {Key: "?", Label: "help"}}
	}
	return firstRunBlock{
		lines: []firstRunLine{
			{text: "A terminal player for the music in your own folders.", style: display.TextSecondary},
			{text: title, style: display.Bold},
		},
		actions: actions,
	}
}

func (m Model) firstRunProgressBlock(display styles.Styles) firstRunBlock {
	found := fmt.Sprintf("%s %s found", text.FormatCount(m.scanned), text.Pluralize(m.scanned, "track", "tracks"))
	if m.scanTotal > 0 {
		found += fmt.Sprintf(" · %d%%", min(100, m.scanned*100/m.scanTotal))
	}
	prefix, where := "Looking in ", strings.Join(m.libraryRoots, ", ")
	if m.firstRun.folder != "" {
		prefix, where = "", m.firstRunFolderLabel()
	}
	// The deepest folder is the part that changes, so a long path loses its
	// beginning rather than its end.
	room := m.library.Width - lipgloss.Width(prefix)
	if w := lipgloss.Width(where); room > 1 && w > room {
		where = ansi.TruncateLeft(where, w-room+1, "…")
	}
	where = prefix + where
	return firstRunBlock{
		lines: []firstRunLine{
			{text: "Scanning your music…", style: display.Bold},
			{text: found, style: display.TextSecondary},
			{text: where, style: display.Dim},
		},
		covers:  m.firstRunCoverRow(),
		actions: []styles.EmptyAction{{Key: "esc", Label: "cancel scan"}, {Key: "enter", Label: "browse tracks found so far"}},
	}
}

// firstRunFolderLabel names the current folder from its music folder down,
// which keeps the part that changes from track to track.
func (m Model) firstRunFolderLabel() string {
	folder := m.firstRun.folder
	for _, root := range m.libraryRoots {
		if rel, err := filepath.Rel(root, folder); err == nil && !strings.HasPrefix(rel, "..") {
			if rel == "." {
				return filepath.Base(root)
			}
			return filepath.ToSlash(filepath.Join(filepath.Base(root), rel))
		}
	}
	return folder
}

func (m Model) firstRunCoverRow() string {
	var covers []string
	for _, t := range m.firstRun.recent {
		if art := m.renderArt(t, firstRunCoverCols, firstRunCoverRows); art != "" {
			if len(covers) > 0 {
				covers = append(covers, "  ")
			}
			covers = append(covers, art)
		}
	}
	if len(covers) == 0 {
		return ""
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, covers...)
}

// renderFirstRunScreen centers the wordmark and block in the Library area,
// dropping the covers, then the drawn wordmark (for a one-line name), then the
// spacer rows until it fits.
func (m Model) renderFirstRunScreen(display styles.Styles, block firstRunBlock) string {
	width, height := m.library.Width, m.library.Height
	var rows []string
	for _, layout := range []firstRunLayout{{covers: true, wordmark: true, spacers: true}, {wordmark: true, spacers: true}, {spacers: true}, {}} {
		rows = layout.rows(display, block, width)
		if len(rows) <= height {
			break
		}
	}
	if len(rows) > height {
		rows = rows[:max(height, 0)]
	}
	top := max(0, (height-len(rows))/2)
	out := make([]string, 0, height)
	for range top {
		out = append(out, "")
	}
	out = append(out, rows...)
	for len(out) < height {
		out = append(out, "")
	}
	return strings.Join(out, "\n")
}

type firstRunLayout struct{ covers, wordmark, spacers bool }

func (l firstRunLayout) rows(display styles.Styles, block firstRunBlock, width int) []string {
	var rows []string
	spacer := func() {
		if l.spacers {
			rows = append(rows, "")
		}
	}
	if l.wordmark {
		rows = append(rows, centerBlock(display.Accent.Bold(true), museWordmark, width)...)
	} else {
		rows = append(rows, centerBlock(display.Accent.Bold(true), []string{"muse"}, width)...)
	}
	spacer()
	for _, line := range block.lines {
		rows = append(rows, centerBlock(line.style, []string{line.text}, width)...)
	}
	if l.covers && block.covers != "" {
		spacer()
		rows = append(rows, centerBlock(lipgloss.NewStyle(), strings.Split(block.covers, "\n"), width)...)
	}
	spacer()
	return append(rows, centerBlock(lipgloss.NewStyle(), firstRunActionRows(display, block.actions), width)...)
}

// firstRunActionRows lines the action labels up after a common key column.
func firstRunActionRows(display styles.Styles, actions []styles.EmptyAction) []string {
	keyW := 0
	for _, a := range actions {
		keyW = max(keyW, lipgloss.Width(a.Key))
	}
	rows := make([]string, 0, len(actions))
	for _, a := range actions {
		key := a.Key + strings.Repeat(" ", keyW-lipgloss.Width(a.Key))
		rows = append(rows, display.EmptyKey.Render(key)+"  "+display.EmptyHint.Render(a.Label))
	}
	return rows
}

// centerBlock styles lines and indents them as one block centered in width,
// truncating any line wider than the screen.
func centerBlock(style lipgloss.Style, lines []string, width int) []string {
	widest := 0
	for _, line := range lines {
		widest = max(widest, lipgloss.Width(line))
	}
	widest = min(widest, width)
	pad := strings.Repeat(" ", max(0, (width-widest)/2))
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = pad + style.Render(ansi.Truncate(line, width, "…"))
	}
	return out
}
