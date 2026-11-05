package tui

import (
	"fmt"
	"log/slog"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/recap"
)

// The listening recap ("Your year in muse") is a full-screen story built
// from the play history Insights reads: one card per page, a page indicator,
// and a period switcher over every year with plays plus the rolling twelve
// months. All of its numbers come from internal/recap; this file only loads
// the history, keeps the reader's place and saves the playlist.

// recapPage is one card of the story.
type recapPage int

const (
	recapPageEmpty recapPage = iota
	recapPageTotals
	recapPageArtists
	recapPageTracks
	recapPageAlbum
	recapPageClock
	recapPageStreak
	recapPageDiscovery
	recapPageRepeat
	recapPageGenres
	recapPageSummary
)

// recapOverlay is the open recap. It is replaced rather than mutated, so a
// copy of the Model never sees another copy's page turn.
type recapOverlay struct {
	token   int64 // identifies the load this overlay is waiting for
	rolling bool  // open on the last 12 months instead of a year
	loading bool

	plays    []insights.PlayRow // all-time counted history
	loc      *time.Location
	genresOf map[string][]string

	periods   []recap.Period // every year with plays, oldest first, then the rolling period
	periodIdx int
	data      recap.Recap
	pages     []recapPage
	page      int

	// shownAt is when the current page appeared, which its entrance and the
	// count-up are measured from; zero when motion is off.
	shownAt time.Time
}

// openRecapMsg opens the recap from the palette's rolling-period entry.
type openRecapMsg struct{ rolling bool }

// recapLoadedMsg delivers the history a recap was opened for.
type recapLoadedMsg struct {
	token    int64
	plays    []insights.PlayRow
	genresOf map[string][]string
	err      error
}

// WithRecapClock pins the clock the recap picks its default year and its
// rolling window from. Fixtures use it so their recap does not change with
// the calendar.
func WithRecapClock(now func() time.Time) Option {
	return func(m *Model) { m.recapClock = now }
}

func (m Model) recapNow() time.Time {
	if m.recapClock != nil {
		return m.recapClock()
	}
	return time.Now()
}

// recapShown reports whether the recap is what the screen shows.
func (m Model) recapShown() bool {
	return m.overlay.recap != nil && m.ActiveOverlayName() == "recap"
}

// openRecap opens the recap and reads the whole history for it off the UI
// goroutine. rolling opens it on the last 12 months instead of a year.
func (m Model) openRecap(rolling bool) (Model, tea.Cmd) {
	r := m.insightsReader
	if r == nil {
		m.statusBar.Notify("Listening history is not available")
		return m, nil
	}
	token := time.Now().UnixNano()
	m.overlay.recap = &recapOverlay{token: token, rolling: rolling, loading: true}
	tracks := m.library.Index.Tracks()
	return m, func() tea.Msg {
		plays, err := r.ReadInsightPlays(0)
		if err != nil {
			return recapLoadedMsg{token: token, err: err}
		}
		genresOf := make(map[string][]string, len(tracks))
		for _, t := range tracks {
			if g := library.SplitGenres(t.Genre); len(g) > 0 {
				genresOf[t.Hash] = g
			}
		}
		return recapLoadedMsg{token: token, plays: plays, genresOf: genresOf}
	}
}

func (m Model) closeRecap() Model {
	m.overlay.recap = nil
	return m
}

// handleRecapLoaded installs the history and opens the default period: the
// current year when anything was played in it, the latest year with plays
// otherwise, or the rolling 12 months when that is what was asked for.
func (m Model) handleRecapLoaded(msg recapLoadedMsg) (Model, tea.Cmd) {
	o := m.overlay.recap
	if o == nil || o.token != msg.token {
		return m, nil
	}
	if msg.err != nil {
		slog.Warn("recap: load", "err", msg.err)
		m.statusBar.NotifyError("Recap: could not read your listening history")
		return m.closeRecap(), nil
	}
	next := *o
	next.loading = false
	next.plays = msg.plays
	next.genresOf = msg.genresOf
	next.loc = time.Local
	now := m.recapNow()
	years := recap.YearsWithPlays(msg.plays, next.loc)
	next.periods = make([]recap.Period, 0, len(years)+1)
	for _, y := range years {
		next.periods = append(next.periods, recap.YearPeriod(y, next.loc))
	}
	next.periods = append(next.periods, recap.RollingPeriod(now, next.loc))
	next.periodIdx = recapDefaultPeriod(next.periods, now.In(next.loc).Year(), next.rolling)
	m.overlay.recap = &next
	return m.recapSelectPeriod(next.periodIdx, recapPageTotals)
}

// recapDefaultPeriod picks the period the recap opens on. periods always
// ends with the rolling one.
func recapDefaultPeriod(periods []recap.Period, year int, rolling bool) int {
	last := len(periods) - 1
	if rolling || last == 0 {
		return last
	}
	for i, p := range periods[:last] {
		if p.Year == year {
			return i
		}
	}
	return last - 1
}

// recapSelectPeriod computes the recap for periods[idx] and turns to the
// page showing the same card as before, or the first one when that card has
// nothing to show in the new period.
func (m Model) recapSelectPeriod(idx int, keep recapPage) (Model, tea.Cmd) {
	o := m.overlay.recap
	if o == nil || idx < 0 || idx >= len(o.periods) {
		return m, nil
	}
	next := *o
	next.periodIdx = idx
	genresOf := next.genresOf
	next.data = recap.Compute(recap.Input{
		Plays:    next.plays,
		Loc:      next.loc,
		Period:   next.periods[idx],
		GenresOf: func(h string) []string { return genresOf[h] },
	})
	next.pages = recapPagesFor(next.data)
	next.page = 0
	for i, p := range next.pages {
		if p == keep {
			next.page = i
		}
	}
	next.shownAt = m.recapEntrance()
	m.overlay.recap = &next
	return m.requestRecapArt()
}

// recapPagesFor lists the cards that have something to say, in story order.
func recapPagesFor(r recap.Recap) []recapPage {
	if r.Empty() {
		return []recapPage{recapPageEmpty}
	}
	pages := []recapPage{recapPageTotals}
	add := func(p recapPage, ok bool) {
		if ok {
			pages = append(pages, p)
		}
	}
	add(recapPageArtists, len(r.TopArtists) > 0)
	add(recapPageTracks, len(r.TopTracks) > 0)
	add(recapPageAlbum, r.TopAlbum.Plays > 0)
	add(recapPageClock, true)
	add(recapPageStreak, r.Streak.Days > 0)
	add(recapPageDiscovery, r.Discovery.Hash != "")
	add(recapPageRepeat, r.RepeatDay.Hash != "")
	add(recapPageGenres, len(r.Genres) > 0)
	return append(pages, recapPageSummary)
}

// recapEntrance is the start of a page's entrance, or zero when nothing
// animates.
func (m Model) recapEntrance() time.Time {
	if !m.motion.enabled {
		return time.Time{}
	}
	return time.Now()
}

// recapTurn moves delta pages, stopping at either end.
func (m Model) recapTurn(delta int) Model {
	o := m.overlay.recap
	if o == nil || len(o.pages) == 0 {
		return m
	}
	page := max(0, min(len(o.pages)-1, o.page+delta))
	if page == o.page {
		return m
	}
	next := *o
	next.page = page
	next.shownAt = m.recapEntrance()
	m.overlay.recap = &next
	return m
}

// recapShiftPeriod moves to an older (-1) or newer (+1) period.
func (m Model) recapShiftPeriod(delta int) (Model, tea.Cmd) {
	o := m.overlay.recap
	if o == nil || o.loading {
		return m, nil
	}
	idx := o.periodIdx + delta
	if idx < 0 || idx >= len(o.periods) {
		return m, nil
	}
	return m.recapSelectPeriod(idx, o.currentPage())
}

func (o *recapOverlay) currentPage() recapPage {
	if o == nil || o.page < 0 || o.page >= len(o.pages) {
		return recapPageEmpty
	}
	return o.pages[o.page]
}

// handleRecapKeyPress owns the keyboard while the recap is open.
func (m Model) handleRecapKeyPress(msg tea.KeyPressMsg) (Model, tea.Cmd, bool) {
	if m.overlay.recap == nil {
		return m, nil, false
	}
	key := msg.String()
	if delta, ok := recapPageKeys(len(m.overlay.recap.pages))[key]; ok {
		return m.recapTurn(delta), nil, true
	}
	switch key {
	case "ctrl+c":
		return m.closeRecap().markQuitting(), tea.Quit, true
	case "esc", "Y":
		return m.closeRecap(), nil, true
	case "]", "[":
		delta := 1
		if key == "[" {
			delta = -1
		}
		next, cmd := m.recapShiftPeriod(delta)
		return next, cmd, true
	case "S":
		next, cmd := m.saveRecapPlaylist()
		return next, cmd, true
	}
	return m, nil, true
}

// recapPageKeys maps the page-turning keys to how far each one turns.
func recapPageKeys(pages int) map[string]int {
	return map[string]int{
		"space": 1, "right": 1, "l": 1, "enter": 1,
		"left": -1, "h": -1, "backspace": -1,
		"g": -pages, "home": -pages,
		"G": pages, "end": pages,
	}
}

// recapCoverTracks lists the library tracks whose covers the recap draws.
func (m Model) recapCoverTracks() []library.Track {
	o := m.overlay.recap
	if o == nil {
		return nil
	}
	var hashes []string
	for _, a := range o.data.TopArtists {
		hashes = append(hashes, a.Hash)
	}
	hashes = append(hashes, o.data.TopAlbum.Hash, o.data.Discovery.Hash, o.data.RepeatDay.Hash)
	var out []library.Track
	for _, h := range hashes {
		if h == "" {
			continue
		}
		if t, ok := m.library.Index.FindByHash(h); ok {
			out = append(out, t)
		}
	}
	return out
}

// requestRecapArt starts the cover lookups the recap's pages draw.
func (m Model) requestRecapArt() (Model, tea.Cmd) {
	if m.art == nil {
		return m, nil
	}
	var cmds []tea.Cmd
	for _, t := range m.recapCoverTracks() {
		var cmd tea.Cmd
		m, cmd = m.requestArt(t)
		cmds = append(cmds, cmd)
	}
	return m, batchNonNil(cmds...)
}

// recapPlaylistName names the saved recap after its period.
func recapPlaylistName(p recap.Period, now time.Time) string {
	if p.Rolling() {
		return "Muse recap: 12 months to " + now.Format("Jan 2, 2006")
	}
	return fmt.Sprintf("Muse recap: %d", p.Year)
}

// saveRecapPlaylist saves the period's top tracks as a static playlist
// through the same path Insights and Mixes save theirs.
func (m Model) saveRecapPlaylist() (Model, tea.Cmd) {
	hashes := m.recapPlaylistHashes()
	if len(hashes) == 0 {
		m.statusBar.Notify("No tracks to save")
		return m, nil
	}
	if m.playlistWriter == nil {
		m.statusBar.Notify("Playlist writer not available")
		return m, nil
	}
	name := recapPlaylistName(m.overlay.recap.data.Period, m.recapNow())
	if playlistNameExists(m.playlists.Playlists, name, 0) {
		m.statusBar.Notify(fmt.Sprintf("A playlist named %q already exists", name))
		return m, nil
	}
	return m, savePlaylistCmd(m.playlistWriter, m.playlistAdder, name, hashes)
}

// recapPlaylistHashes is the recap's playlist, less any track deleted since
// it was played, which a playlist could not hold.
func (m Model) recapPlaylistHashes() []string {
	o := m.overlay.recap
	if o == nil || o.loading {
		return nil
	}
	var hashes []string
	for _, h := range o.data.Playlist {
		if _, ok := m.library.Index.FindByHash(h); ok {
			hashes = append(hashes, h)
		}
	}
	return hashes
}

// savePlaylistCmd creates the playlist name holding hashes and reports back
// with the messages the Mixes save uses.
func savePlaylistCmd(pw playlistCreator, pa playlistTrackAdder, name string, hashes []string) tea.Cmd {
	return func() tea.Msg {
		id, err := pw.CreatePlaylist(name)
		if err != nil {
			return mixSaveAsPlaylistErrorMsg{err: err}
		}
		if pa != nil {
			if err := pa.AddTracksToPlaylist(id, hashes); err != nil {
				return mixSaveAsPlaylistErrorMsg{err: err}
			}
		}
		rows, err := pw.ReadPlaylists()
		if err != nil {
			return mixSaveAsPlaylistErrorMsg{err: err}
		}
		return mixSavedAsPlaylistMsg{playlists: rows, playlistName: name, count: len(hashes)}
	}
}
