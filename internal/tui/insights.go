package tui

import (
	"time"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
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

// insightsResolutions is the w / W cycle the classResolution panels get instead
// of the window one. Their span is always all-time, so a trailing duration would
// be a label with nothing behind it; the bucket size is the thing that actually
// changes what Momentum and Discovery draw. Monthly leads so the default reading
// is the one Coverage projects with.
var insightsResolutions = []insights.Resolution{
	insights.ResolutionMonthly,
	insights.ResolutionWeekly,
	insights.ResolutionDaily,
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
	// key is the local hour, weekday number, folded genre or trend-bucket key
	// the row stands for.
	drillHour
	drillWeekday
	drillRhythmGenre
	drillTrendBucket
	drillFirstPlaysBucket
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
	snap  insights.Snapshot
	panel insightsPanel
	// windowIdx and resolutionIdx are separate controls sharing one key: w
	// moves the window on windowed panels and the bucket size on the
	// resolution ones, so neither cycle ever has a press that changes nothing.
	windowIdx     int
	resolutionIdx int

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
	// stale marks the memo as superseded by a play, a rescan or a metadata
	// edit. The previous snapshot stays on screen until the replacement lands,
	// so an invalidation never flashes an empty view.
	stale bool
	// loadErr is the aggregate of the readers that failed on the last load. It
	// keeps the automatic refresh from retrying a broken read every tick, so
	// only an entry to the view or a change to the data clears it.
	loadErr error
	// gen increments on every load so a slow reply for an abandoned window
	// cannot overwrite a newer one.
	gen int

	width, height int
}

func (v *InsightsView) setSize(w, h int) { v.width = w; v.height = h }

func (v InsightsView) window() insightsWindow { return insightsWindows[v.windowIdx] }

func (v InsightsView) resolution() insights.Resolution {
	return insightsResolutions[v.resolutionIdx]
}

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
