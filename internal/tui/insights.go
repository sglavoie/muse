package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/chart"
	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// insightsCompactWidth is the terminal width below which the Insights view
// drops every bar and renders labelled key/value rows instead.
const insightsCompactWidth = 70

// insightsBriefingMinHeight is the view height below which the briefing block
// collapses to the active panel's headline alone, so the panel body still has
// room to say something.
const insightsBriefingMinHeight = 18

// insightsProseMinHeight is the view height below which Coverage drops its
// cross-reference paragraph. The paragraph wraps to three lines at narrow
// widths, which is most of a short panel's budget.
const insightsProseMinHeight = 14

// insightsMinBodyRows is how many scrollable rows a panel header must leave
// behind it, and insightsMinHeaderLines is the floor the header keeps even when
// that is impossible — enough for its blank line and its headline.
const (
	insightsMinBodyRows    = 4
	insightsMinHeaderLines = 2
)

// insightsPanel identifies one row of the level-0 briefing.
type insightsPanel int

const (
	panelCoverage insightsPanel = iota
	panelRhythm
	panelMomentum
	panelConcentration
	panelTimeVsPlays
	panelSessions
	panelDiscovery
	panelDataHealth
)

// panelClass says how a panel relates to the active time window. The state
// strip renders it so the window control visibly refuses instead of no-opping.
type panelClass int

const (
	// classWindowed recomputes with the window.
	classWindowed panelClass = iota
	// classFixed ignores the window entirely: "never played" and "is this
	// data lying to me" are only meaningful against the whole history.
	classFixed
	// classResolution keeps its all-time span; the window picks the bucket
	// size instead.
	classResolution
)

type insightsPanelSpec struct {
	id    insightsPanel
	name  string
	class panelClass
}

var insightsPanelSpecs = []insightsPanelSpec{
	{panelCoverage, "Coverage", classFixed},
	{panelRhythm, "Rhythm", classWindowed},
	{panelMomentum, "Momentum", classResolution},
	{panelConcentration, "Concentration", classWindowed},
	{panelTimeVsPlays, "Time vs Plays", classWindowed},
	{panelSessions, "Sessions", classWindowed},
	{panelDiscovery, "Discovery", classResolution},
	{panelDataHealth, "Data Health", classFixed},
}

// insightsWindow is one entry of the w / W cycle. Days is the trailing span;
// 0 means all time and -1 means "since the first of the current local month".
type insightsWindow struct {
	label string
	days  int
}

var insightsWindows = []insightsWindow{
	{"All Time", 0},
	{"7 Days", 7},
	{"30 Days", 30},
	{"90 Days", 90},
	{"This Month", -1},
}

// insightsWindowSince converts a window index into the Unix second a play must
// have *started* at or after to fall inside it. Zero means all time.
func insightsWindowSince(idx int, now time.Time, loc *time.Location) int64 {
	if loc == nil {
		loc = time.UTC
	}
	w := insightsWindows[((idx%len(insightsWindows))+len(insightsWindows))%len(insightsWindows)]
	switch {
	case w.days == 0:
		return 0
	case w.days < 0:
		local := now.In(loc)
		return time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, loc).Unix()
	default:
		return now.Add(-time.Duration(w.days) * 24 * time.Hour).Unix()
	}
}

// The drill levels. A briefing row usually opens a track list directly, but a
// Sessions row opens a list of sessions first, because a session is a thing in
// its own right — it has a shape, an opener and an order to replay it in.
const (
	insightsLevelBriefing = iota
	insightsLevelTracks
	insightsLevelSessions
	insightsLevelSessionTracks
)

// insightsDrillKind names the track set behind a briefing row. Every drillable
// row resolves to a concrete set of hashes: a bar is a query with a shape, not
// a picture.
type insightsDrillKind int

const (
	drillNone insightsDrillKind = iota
	drillUnheardAll
	drillUnheardArtist
	drillUnheardAlbum
	drillUnheardGenre
	drillDormant
	// key is the local hour, weekday number, folded genre or "YYYY-MM" month
	// the row stands for.
	drillHour
	drillWeekday
	drillRhythmGenre
	drillMonth
	drillFirstPlaysMonth
	drillAddedNeverPlayed
	// drillArtistPlays keys on an artist name, drillTopArtists on how many of
	// the top artists by time the row stands for.
	drillArtistPlays
	drillTopArtists
	// drillHealth keys on the index of the Data Health finding, whose affected
	// hashes were resolved when the panel was built.
	drillHealth
	// The session kinds open a session list rather than a track list. key is
	// the duration-bucket index, the shape name or the opener/closer artist.
	drillSessionsAll
	drillSessionsBucket
	drillSessionsShape
	drillSessionsOpener
	drillSessionsCloser
)

// opensSessions reports whether a row drills into the session list instead of
// straight into tracks.
func (k insightsDrillKind) opensSessions() bool {
	switch k {
	case drillSessionsAll, drillSessionsBucket, drillSessionsShape,
		drillSessionsOpener, drillSessionsCloser:
		return true
	}
	return false
}

// insightsRow is one cursor-selectable row of a panel body.
type insightsRow struct {
	// section groups consecutive rows; a header is emitted when it changes.
	section  string
	label    string
	note     string
	fraction float64
	drill    insightsDrillKind
	// key is the artist, album or genre the drill filters on. Empty for
	// drills that need no key.
	key string
}

// InsightsView renders the Insights briefing (level 0) and the track list a
// briefing row drills into (level 1, an embedded LibraryView so sort, filter,
// visual select and enqueue all come for free).
type InsightsView struct {
	snap      insights.Snapshot
	panel     insightsPanel
	windowIdx int

	cursor       int
	scrollOffset int

	level      int // one of the insightsLevel* constants
	trackView  LibraryView
	drillLabel string

	// sessions is every session in the window; sessionList is the subset a
	// Sessions row drilled into, with its own cursor because a session list is
	// not a track list and cannot borrow LibraryView's.
	sessions      []insights.SessionRow
	sessionList   []insights.SessionRow
	sessionCursor int
	sessionScroll int
	sessionLabel  string

	// genreCoverage and dormant are derived from the in-memory library rather
	// than from insights.Snapshot: CoverageRow carries no genre, and the
	// snapshot keeps no per-track last-played detail.
	genreCoverage []insights.ShelfStat
	dormant       []library.Track

	// plays and timeline are the raw history the time-series panels resolve
	// their drills against. Snapshot aggregates track identity away, so without
	// these an hour column or a month bar would be a picture with no query
	// behind it.
	plays    []insights.PlayRow
	timeline []insights.TimelineRow
	// genreHours and backlog need the library's genres and add dates, neither
	// of which reaches insights.Snapshot, so they are folded at load time.
	genreHours []insightsGenreHours
	backlog    []insightsBacklogMonth
	// health is the Data Health panel, findings and affected hashes together.
	health []insightsHealthRow
	// pendingMerge is the genre merge the confirmation bar is asking about.
	pendingMerge *insightsMergePlan
	// loc is the zone the snapshot was bucketed in, so a row's own bucketing
	// cannot disagree with the aggregate it sits under.
	loc *time.Location

	loaded  bool
	loading bool
	// gen increments on every load so a slow reply for an abandoned window
	// cannot overwrite a newer one.
	gen int

	width, height int
}

func (v *InsightsView) setSize(w, h int) { v.width = w; v.height = h }

func (v InsightsView) window() insightsWindow { return insightsWindows[v.windowIdx] }

func (v InsightsView) panelSpec() insightsPanelSpec {
	for _, spec := range insightsPanelSpecs {
		if spec.id == v.panel {
			return spec
		}
	}
	return insightsPanelSpecs[0]
}

func (v InsightsView) compact() bool { return v.width > 0 && v.width < insightsCompactWidth }

// empty reports whether there is nothing at all to show: no plays recorded and
// no library to measure them against.
func (v InsightsView) empty() bool {
	return v.snap.TotalPlays == 0 && v.snap.Coverage.TotalTracks == 0
}

// shiftPanel moves the active panel by delta, wrapping, and resets the row
// cursor since the new panel has a different row set.
func (v InsightsView) shiftPanel(delta int) InsightsView {
	n := len(insightsPanelSpecs)
	idx := 0
	for i, spec := range insightsPanelSpecs {
		if spec.id == v.panel {
			idx = i
			break
		}
	}
	v.panel = insightsPanelSpecs[((idx+delta)%n+n)%n].id
	v.cursor = 0
	v.scrollOffset = 0
	return v
}

// shiftWindow moves the active window by delta, wrapping.
func (v InsightsView) shiftWindow(delta int) InsightsView {
	n := len(insightsWindows)
	v.windowIdx = ((v.windowIdx+delta)%n + n) % n
	return v
}

// atTracks reports whether the embedded LibraryView is what is on screen — true
// both for a row's track list and for the tracks of one session.
func (v InsightsView) atTracks() bool {
	return v.level == insightsLevelTracks || v.level == insightsLevelSessionTracks
}

func (v *InsightsView) clampCursor() {
	if v.level == insightsLevelSessions {
		if n := len(v.sessionList); n > 0 {
			v.sessionCursor = min(max(v.sessionCursor, 0), n-1)
			v.sessionScroll = adjustScroll(v.sessionCursor, v.sessionScroll, v.sessionBodyRows())
		} else {
			v.sessionCursor, v.sessionScroll = 0, 0
		}
		return
	}
	rows := v.rows()
	if len(rows) == 0 {
		v.cursor = 0
		v.scrollOffset = 0
		return
	}
	v.cursor = min(max(v.cursor, 0), len(rows)-1)
	v.scrollOffset = min(max(v.scrollOffset, 0), v.cursor)
	// Section headers spend body lines alongside the rows under them, so
	// scrolling by row count alone would let the cursor slide under the bottom
	// edge. Data Health is the panel that reaches six sections and shows it.
	budget := v.bodyRows()
	for v.scrollOffset < v.cursor && panelBodyLines(rows, v.scrollOffset, v.cursor) > budget {
		v.scrollOffset++
	}
}

// panelBodyLines counts the lines rows[from..to] occupy once every section
// header between them is counted too.
func panelBodyLines(rows []insightsRow, from, to int) int {
	lines, section := 0, ""
	for i := from; i <= to; i++ {
		if rows[i].section != section {
			section = rows[i].section
			lines++
		}
		lines++
	}
	return lines
}

// bodyRows is how many panel rows fit under the header block.
func (v InsightsView) bodyRows() int {
	return max(v.height-v.headerHeight(), 1)
}

// focusedRow returns the row under the cursor, or nil when the panel has none.
func (v InsightsView) focusedRow() *insightsRow {
	rows := v.rows()
	if v.cursor < 0 || v.cursor >= len(rows) {
		return nil
	}
	return &rows[v.cursor]
}

// focusedTrack returns the focused track at drill level, or nil otherwise.
func (v *InsightsView) focusedTrack() *library.Track {
	if !v.atTracks() {
		return nil
	}
	track, ok := v.trackView.view().FocusedTrack()
	if !ok {
		return nil
	}
	return &track
}

// focusedSession returns the session under the cursor in the session list.
func (v InsightsView) focusedSession() *insights.SessionRow {
	if v.level != insightsLevelSessions ||
		v.sessionCursor < 0 || v.sessionCursor >= len(v.sessionList) {
		return nil
	}
	return &v.sessionList[v.sessionCursor]
}

// openDrill enters the track level with the given set.
func (v *InsightsView) openDrill(label string, tracks []library.Track) {
	if v.level == insightsLevelSessions {
		v.level = insightsLevelSessionTracks
	} else {
		v.level = insightsLevelTracks
	}
	v.drillLabel = label
	v.trackView = LibraryView{
		sourceTracks: tracks,
		fuzzy:        library.SahilmFuzzySearcher{},
		width:        v.width,
		height:       v.height,
	}
}

// openSessionList enters the session level with the sessions a Sessions row
// stands for.
func (v *InsightsView) openSessionList(label string, rows []insights.SessionRow) {
	v.level = insightsLevelSessions
	v.sessionLabel = label
	v.sessionList = rows
	v.sessionCursor = 0
	v.sessionScroll = 0
}

// back steps out one level: a session's tracks return to the session list,
// anything else returns to the briefing.
func (v *InsightsView) back() {
	if v.level == insightsLevelSessionTracks {
		v.level = insightsLevelSessions
		v.drillLabel = ""
		v.trackView = LibraryView{}
		return
	}
	v.level = insightsLevelBriefing
	v.drillLabel = ""
	v.trackView = LibraryView{}
	v.sessionLabel = ""
	v.sessionList = nil
	v.sessionCursor = 0
	v.sessionScroll = 0
}

func (v InsightsView) handleCursorAction(a action) InsightsView {
	if v.atTracks() {
		v.trackView = v.trackView.HandleAction(a)
		return v
	}
	if v.level == insightsLevelSessions {
		return v.handleSessionCursorAction(a)
	}
	n := len(v.rows())
	if n == 0 {
		return v
	}
	v.cursor = moveInsightsCursor(v.cursor, n, v.bodyRows(), a)
	v.clampCursor()
	return v
}

func (v InsightsView) handleSessionCursorAction(a action) InsightsView {
	n := len(v.sessionList)
	if n == 0 {
		return v
	}
	v.sessionCursor = moveInsightsCursor(v.sessionCursor, n, v.sessionBodyRows(), a)
	v.clampCursor()
	return v
}

// moveInsightsCursor applies one cursor action. The result is left unclamped
// for the caller's clampCursor, which also owns the scroll offset.
func moveInsightsCursor(cursor, n, page int, a action) int {
	switch a {
	case actionCursorUp:
		cursor--
	case actionCursorDown:
		cursor++
	case actionHalfPageUp:
		cursor -= max(page/2, 1)
	case actionHalfPageDown:
		cursor += max(page/2, 1)
	case actionFullPageUp:
		cursor -= page
	case actionFullPageDown:
		cursor += page
	case actionGoToTop:
		cursor = 0
	case actionGoToBottom:
		cursor = n - 1
	}
	return cursor
}

// rows returns the cursor-selectable rows of the active panel. Panels whose
// bodies have not landed yet return none; their headlines are live regardless.
func (v InsightsView) rows() []insightsRow {
	switch v.panel {
	case panelCoverage:
		return v.coverageRows()
	case panelRhythm:
		return v.rhythmRows()
	case panelMomentum:
		return v.momentumRows()
	case panelConcentration:
		return v.concentrationRows()
	case panelTimeVsPlays:
		return v.timeVsPlaysRows()
	case panelSessions:
		return v.sessionRows()
	case panelDiscovery:
		return v.discoveryRows()
	case panelDataHealth:
		return v.healthRows()
	}
	return nil
}

// location is the zone every row buckets in. It falls back to the load path's
// own default rather than to UTC, so a view built without a snapshot still
// files a play under the hour the user heard it.
func (v InsightsView) location() *time.Location {
	if v.loc == nil {
		return time.Local
	}
	return v.loc
}

// coverageRows builds the Coverage body: dusty shelves, albums nobody has ever
// opened, per-genre coverage, and tracks that were heard once and then dropped.
func (v InsightsView) coverageRows() []insightsRow {
	c := v.snap.Coverage
	rows := make([]insightsRow, 0, 40)

	var untouched, dusty []insights.ShelfStat
	for _, a := range c.DustyAlbums {
		if a.Unheard == a.Tracks {
			untouched = append(untouched, a)
		} else {
			dusty = append(dusty, a)
		}
	}

	appendShelves := func(section string, shelves []insights.ShelfStat, kind insightsDrillKind) {
		for _, s := range shelves {
			rows = append(rows, insightsRow{
				section:  section,
				label:    s.Name,
				note:     fmt.Sprintf("%d/%d unheard", s.Unheard, s.Tracks),
				fraction: shelfFraction(s),
				drill:    kind,
				key:      s.Name,
			})
		}
	}

	appendShelves("Dusty artists", c.DustyArtists, drillUnheardArtist)
	appendShelves("Untouched albums", untouched, drillUnheardAlbum)
	appendShelves("Dusty albums", dusty, drillUnheardAlbum)
	appendShelves("Genre coverage", v.genreCoverage, drillUnheardGenre)

	for _, t := range v.dormant {
		rows = append(rows, insightsRow{
			section: "Dormant tracks",
			label:   fmt.Sprintf("%s — %s", t.Artist, t.Title),
			note:    "last " + relativeDayLabel(t.LastPlayedAt, v.snap.LastPlayAt),
			drill:   drillDormant,
			key:     t.Hash,
		})
	}
	return rows
}

func shelfFraction(s insights.ShelfStat) float64 {
	if s.Tracks <= 0 {
		return 0
	}
	return float64(s.Unheard) / float64(s.Tracks)
}

// relativeDayLabel renders how long before ref a timestamp falls. ref is the
// snapshot's most recent play rather than the wall clock so the label stays
// stable between renders.
func relativeDayLabel(at, ref int64) string {
	if at <= 0 {
		return "never"
	}
	if ref <= at {
		return "today"
	}
	days := int((ref - at) / 86400)
	switch {
	case days <= 0:
		return "today"
	case days < 60:
		return fmt.Sprintf("%dd ago", days)
	default:
		return fmt.Sprintf("%dmo ago", days/30)
	}
}

// render draws the Insights view: the briefing plus the active panel at level
// 0, a session list at level 2, and an embedded track list at the two levels
// that end in tracks.
func (v InsightsView) render(s styles.Styles, favHashes, favArtists, favAlbums map[string]struct{}, nowPlayingHash, nowPlayingIcon string) string {
	if v.atTracks() {
		if len(v.trackView.sourceTracks) == 0 {
			return s.Dim.Render("Nothing behind this row.")
		}
		return v.trackView.render(s, favHashes, favArtists, favAlbums, nowPlayingHash, nowPlayingIcon)
	}
	if v.level == insightsLevelSessions {
		return v.renderSessionList(s)
	}

	if !v.loaded {
		if v.loading {
			return s.Dim.Render("Reading your listening history…")
		}
		return s.Dim.Render("Insights are not available: no history reader is wired.")
	}
	if v.empty() {
		return s.RenderEmptyCard(v.width, v.height, "Nothing to measure yet", []styles.EmptyAction{
			{Key: "1", Label: "go to library"},
			{Key: "?", Label: "help"},
		})
	}

	var b strings.Builder
	b.WriteString(v.renderStrip(s))
	b.WriteString("\n")
	if v.briefingFits() {
		b.WriteString(v.renderBriefing(s))
		b.WriteString("\n")
	}
	b.WriteString(v.renderPanel(s))

	// The header block has a floor it cannot shrink past, so at extreme heights
	// it can still outgrow the frame. Clamp rather than let it push the footer
	// off the screen.
	out := strings.Split(b.String(), "\n")
	if v.height > 0 && len(out) > v.height {
		out = out[:v.height]
	}
	return strings.Join(out, "\n")
}

func (v InsightsView) briefingFits() bool {
	return v.height >= insightsBriefingMinHeight
}

// headerHeight counts every line drawn above the panel's scrollable rows.
func (v InsightsView) headerHeight() int {
	h := 1 // state strip
	if v.briefingFits() {
		h += len(insightsPanelSpecs)
	}
	// Rendering the header with a zero Styles yields the same line count as the
	// styled render, so the two can never drift apart.
	h += len(v.panelHeaderLines(styles.Styles{}))
	return h
}

func (v InsightsView) renderStrip(s styles.Styles) string {
	spec := v.panelSpec()
	summary := fmt.Sprintf("%s plays · %s · %d %s",
		formatCount(v.snap.TotalPlays),
		formatListeningTime(v.snap.TotalSecs),
		v.snap.SpanDays, pluralize(v.snap.SpanDays, "day", "days"))
	line := strip.Render(v.width, []string{"Insights", summary},
		strip.SortChunk{}, strip.FilterChunk{}, strip.CountsChunk{}, nil,
		v.windowHint(), s)
	if line == "" {
		return spec.name
	}
	return line
}

// windowHint is the right-edge affordance that names the active window and,
// for panels that do not honour it, says so instead of silently ignoring w.
func (v InsightsView) windowHint() string {
	switch v.panelSpec().class {
	case classFixed:
		return "all-time (fixed)"
	case classResolution:
		return "resolution: " + v.window().label
	default:
		return v.window().label + " ‹w›"
	}
}

func (v InsightsView) renderBriefing(s styles.Styles) string {
	lines := make([]string, 0, len(insightsPanelSpecs))
	labelW := 0
	for _, spec := range insightsPanelSpecs {
		labelW = max(labelW, len(spec.name))
	}
	for _, spec := range insightsPanelSpecs {
		cursor := "  "
		if spec.id == v.panel {
			cursor = "> "
		}
		head := cursor + text.Pad(spec.name, labelW) + "  "
		tail := v.headline(spec.id)
		if v.width > 0 {
			tail = text.Truncate(tail, max(v.width-len(head), 1))
		}
		if spec.id == v.panel {
			lines = append(lines, s.Accent.Render(head+tail))
		} else {
			lines = append(lines, s.TextPrimary.Render(head)+s.TextTertiary.Render(tail))
		}
	}
	return strings.Join(lines, "\n")
}

// headline is the one-line answer a panel gives. Every headline is computed
// from live data even when the panel's expanded body is not built yet.
func (v InsightsView) headline(p insightsPanel) string {
	snap := v.snap
	switch p {
	case panelCoverage:
		c := snap.Coverage
		return fmt.Sprintf("%s of %s tracks never played · %s unheard",
			formatCount(c.UnheardTracks), formatCount(c.TotalTracks),
			formatListeningTime(int(c.UnheardSecs)))
	case panelRhythm:
		r := snap.Rhythm
		if r.TotalPlays == 0 {
			return "no plays in this window"
		}
		return fmt.Sprintf("%ss %.0f%% of plays · peak %02d:00 · %d quiet %s",
			r.TopWeekday.Weekday, r.TopWeekday.Share*100, r.PeakHour,
			len(r.QuietHours), pluralize(len(r.QuietHours), "hour", "hours"))
	case panelMomentum:
		t := snap.Trends
		if t.AvgMonthlySecs <= 0 {
			return "not enough completed months to compare"
		}
		return fmt.Sprintf("%s this month — %.0f%% of your %s monthly average",
			formatListeningTime(t.Current.ListenedSecs), t.CurrentVsAvg*100,
			formatListeningTime(int(t.AvgMonthlySecs)))
	case panelConcentration:
		l := snap.Rankings.Ladder
		if l.TotalArtists == 0 {
			return "no artists in this window"
		}
		return fmt.Sprintf("%d %s = half your listening time · top-5 %.0f%%",
			l.Artists50, pluralize(l.Artists50, "artist", "artists"),
			snap.Rankings.TopFiveShare*100)
	case panelTimeVsPlays:
		r := snap.Rankings
		if len(r.Deltas) == 0 {
			return "no artists in this window"
		}
		// Deltas covers the compared head of the two orderings, not every artist:
		// "moves 9 of your top 15" is only true when 15 is the size of that head.
		return fmt.Sprintf("ranking by hours moves %d of your top %d artists",
			r.MovedCount, len(r.Deltas))
	case panelSessions:
		ss := snap.SessionStats
		if ss.Count == 0 {
			return "no sessions in this window"
		}
		return fmt.Sprintf("%d sessions · median %s · %.0f%% single-artist",
			ss.Count, formatListeningTime(ss.MedianSecs), ss.SingleArtistShare*100)
	case panelDiscovery:
		t := snap.Trends
		return fmt.Sprintf("%d first-time %s this month · %.0f%% are repeats",
			t.FirstPlaysMonth, pluralize(t.FirstPlaysMonth, "track", "tracks"),
			t.RepeatShare*100)
	case panelDataHealth:
		// The count is of rows rather than of snapshot issues: a finding with no
		// track list behind it is stated in the header prose instead, and a
		// headline that counted those would promise rows that are not there.
		switch n := len(v.health); {
		case n == 0 && len(snap.Quality.Issues) == 0:
			return "nothing distorting the panels above"
		case n == 0:
			return snap.Quality.Issues[0].Detail
		default:
			return fmt.Sprintf("%d %s · %s %s affected",
				n, pluralize(n, "finding", "findings"),
				formatCount(v.healthAffectedTracks()),
				pluralize(v.healthAffectedTracks(), "track", "tracks"))
		}
	}
	return ""
}

// panelHeaderLines is the static block a panel draws above its scrollable
// rows, trimmed so it can never crowd the body out of a short terminal.
func (v InsightsView) panelHeaderLines(s styles.Styles) []string {
	var lines []string
	switch v.panel {
	case panelCoverage:
		lines = v.coverageHeaderLines(s)
	case panelRhythm:
		lines = v.rhythmHeaderLines(s)
	case panelMomentum:
		lines = v.momentumHeaderLines(s)
	case panelConcentration:
		lines = v.concentrationHeaderLines(s)
	case panelTimeVsPlays:
		lines = v.timeVsPlaysHeaderLines(s)
	case panelSessions:
		lines = v.sessionsHeaderLines(s)
	case panelDiscovery:
		lines = v.discoveryHeaderLines(s)
	case panelDataHealth:
		lines = v.dataHealthHeaderLines(s)
	default:
		lines = []string{"", s.Dim.Render(v.headline(v.panel))}
	}
	if b := v.headerBudget(); len(lines) > b {
		lines = lines[:b]
	}
	return lines
}

// headerBudget is how many lines a panel header may occupy. A header that eats
// the whole frame turns a drillable panel into a picture, so the body's floor
// is reserved before the header gets anything.
func (v InsightsView) headerBudget() int {
	used := 1 // state strip
	if v.briefingFits() {
		used += len(insightsPanelSpecs)
	}
	return max(v.height-used-insightsMinBodyRows, insightsMinHeaderLines)
}

func (v InsightsView) coverageHeaderLines(s styles.Styles) []string {
	c := v.snap.Coverage
	w := max(v.width, 1)
	pal := chart.FromStyles(s)

	lines := []string{
		"",
		s.TextPrimary.Render(fmt.Sprintf("In %d %s you played %s tracks. You own %s.",
			v.snap.SpanDays, pluralize(v.snap.SpanDays, "day", "days"),
			formatCount(c.HeardTracks), formatCount(c.TotalTracks))),
		"",
	}
	if !v.compact() {
		lines = append(lines, chart.Ratio(float64(c.HeardTracks), float64(c.TotalTracks), w, pal))
	}
	lines = append(lines,
		s.TextSecondary.Render(fmt.Sprintf("heard %s · unheard %s · %s unheard",
			formatCount(c.HeardTracks), formatCount(c.UnheardTracks),
			formatListeningTime(int(c.UnheardSecs)))),
		"",
	)
	if v.height >= insightsProseMinHeight {
		for _, l := range strings.Split(ansi.Wrap(v.coverageReadLine(), w, " "), "\n") {
			lines = append(lines, s.Dim.Render(l))
		}
		lines = append(lines, "")
	}
	return lines
}

// coverageReadLine is the cross-reference that makes Coverage land: the size of
// the unheard part measured against everything actually listened to.
func (v InsightsView) coverageReadLine() string {
	c := v.snap.Coverage
	if c.ListenedSecs <= 0 {
		return "Nothing listened to yet — every track below is still unopened."
	}
	sized := fmt.Sprintf("That unheard part is %.0f%% the size of everything you have listened to.",
		c.UnheardVsHeard*100)
	if c.MonthsToFinish <= 0 {
		return sized + " Not enough first-play history to project a finish date."
	}
	return sized + fmt.Sprintf(" At the current discovery rate you would finish it in %s.",
		formatMonths(c.MonthsToFinish))
}

func (v InsightsView) renderPanel(s styles.Styles) string {
	lines := append([]string(nil), v.panelHeaderLines(s)...)

	rows := v.rows()
	if len(rows) == 0 {
		if v.panel == panelCoverage {
			lines = append(lines, s.Dim.Render("Every shelf in your library has been opened."))
		}
		return strings.Join(lines, "\n")
	}

	// Section headers occupy body lines too, so the budget is spent as lines are
	// emitted rather than sliced off the row list up front.
	budget := v.bodyRows()
	section := ""
	for i := v.scrollOffset; i < len(rows) && budget > 0; i++ {
		row := rows[i]
		if row.section != section {
			section = row.section
			lines = append(lines, s.BucketHeader.Render(section))
			budget--
			if budget == 0 {
				break
			}
		}
		lines = append(lines, v.renderRow(s, row, i == v.cursor))
		budget--
	}
	return strings.Join(lines, "\n")
}

func (v InsightsView) renderRow(s styles.Styles, row insightsRow, focused bool) string {
	cursor := "  "
	if focused {
		cursor = "> "
	}
	width := max(v.width-len(cursor), 1)

	var body string
	if v.compact() || row.drill == drillDormant {
		labelW := max(width-len(row.note)-2, 1)
		body = text.Pad(text.Truncate(row.label, labelW), labelW) + "  " + row.note
	} else {
		bars := chart.BarList([]chart.BarRow{{Label: row.label, Value: row.fraction, Note: row.note}},
			chart.BarListOptions{Width: width, Max: 1}, chart.FromStyles(s))
		if len(bars) > 0 {
			body = bars[0]
		}
	}
	if focused {
		// The bar is already styled; strip it so the focus background covers the
		// whole row instead of being punched through by the bar's own colours.
		return s.Accent.Render(cursor + text.Truncate(ansi.Strip(body), width))
	}
	return s.TextTertiary.Render(cursor) + body
}

// formatMonths renders a fractional month count as years and months.
func formatMonths(months float64) string {
	total := int(months + 0.5)
	if total <= 0 {
		return "under a month"
	}
	years, rest := total/12, total%12
	switch {
	case years > 0 && rest > 0:
		return fmt.Sprintf("%d %s %d %s", years, pluralize(years, "year", "years"), rest, pluralize(rest, "month", "months"))
	case years > 0:
		return fmt.Sprintf("%d %s", years, pluralize(years, "year", "years"))
	default:
		return fmt.Sprintf("%d %s", rest, pluralize(rest, "month", "months"))
	}
}
