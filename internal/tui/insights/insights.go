package insights

import (
	"time"

	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	libtui "github.com/sglavoie/muse/internal/tui/library"
)

// insightsCompactWidth is the terminal width below which the Insights view
// drops every bar and renders labelled key/value rows instead.
const insightsCompactWidth = 70

// insightsBriefingMinHeight is the view height below which the briefing block
// collapses to the active panel's headline alone, so the panel body still has
// room to say something.
const insightsBriefingMinHeight = 26

// insightsMinBodyRows is how many scrollable rows a panel header must leave
// behind it, and insightsMinHeaderLines is the floor the header keeps even when
// that is impossible — enough for its blank line and its headline.
const (
	insightsMinBodyRows    = 4
	insightsMinHeaderLines = 2
)

// Panel identifies one row of the level-0 briefing.
type Panel int

const (
	PanelCoverage Panel = iota
	panelRhythm
	PanelMomentum
	PanelConcentration
	PanelTimeVsPlays
	PanelSessions
	PanelDiscovery
	PanelDataHealth
)

// PanelClass says how a panel relates to the active time window. The state
// strip renders it so the window control visibly refuses instead of no-opping.
type PanelClass int

const (
	// ClassWindowed recomputes with the window.
	ClassWindowed PanelClass = iota
	// ClassFixed ignores the window entirely: "never played" and "is this
	// data lying to me" are only meaningful against the whole history.
	ClassFixed
	// ClassResolution keeps its all-time span; the window picks the bucket
	// size instead.
	ClassResolution
)

type PanelSpec struct {
	id    Panel
	Name  string
	Class PanelClass
}

var insightsPanelSpecs = []PanelSpec{
	{PanelCoverage, "Coverage", ClassFixed},
	{panelRhythm, "Rhythm", ClassWindowed},
	{PanelMomentum, "Momentum", ClassResolution},
	{PanelConcentration, "Concentration", ClassWindowed},
	{PanelTimeVsPlays, "Time vs Plays", ClassWindowed},
	{PanelSessions, "Sessions", ClassWindowed},
	{PanelDiscovery, "Discovery", ClassResolution},
	{PanelDataHealth, "Data Health", ClassFixed},
}

// Window is one entry of the w / W cycle. Days is the trailing span;
// 0 means all time and -1 means "since the first of the current local month".
type Window struct {
	Label string
	days  int
}

var Windows = []Window{
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
var insightsResolutions = []insightsdata.Resolution{
	insightsdata.ResolutionMonthly,
	insightsdata.ResolutionWeekly,
	insightsdata.ResolutionDaily,
}

// WindowSince converts a window index into the Unix second a play must
// have *started* at or after to fall inside it. Zero means all time.
func WindowSince(idx int, now time.Time, loc *time.Location) int64 {
	if loc == nil {
		loc = time.UTC
	}
	w := Windows[((idx%len(Windows))+len(Windows))%len(Windows)]
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
	LevelBriefing = iota
	LevelTracks
	LevelSessions
	LevelSessionTracks
)

// DrillKind names the track set behind a briefing row. Every drillable
// row resolves to a concrete set of hashes: a bar is a query with a shape, not
// a picture.
type DrillKind int

const (
	DrillNone DrillKind = iota
	DrillUnheardAll
	DrillUnheardArtist
	DrillUnheardAlbum
	DrillUnheardGenre
	DrillDormant
	// key is the local hour, weekday number, folded genre or trend-bucket key
	// the row stands for.
	DrillHour
	DrillWeekday
	DrillRhythmGenre
	DrillTrendBucket
	DrillFirstPlaysBucket
	DrillAddedNeverPlayed
	// DrillArtistPlays keys on an artist name, drillTopArtists on how many of
	// the top artists by time the row stands for.
	DrillArtistPlays
	DrillTopArtists
	// DrillHealth keys on the index of the Data Health finding, whose affected
	// hashes were resolved when the panel was built.
	DrillHealth
	// The session kinds open a session list rather than a track list. key is
	// the duration-bucket index, the shape name or the opener/closer artist.
	DrillSessionsAll
	drillSessionsBucket
	drillSessionsShape
	DrillSessionsOpener
	DrillSessionsCloser
)

// OpensSessions reports whether a row drills into the session list instead of
// straight into tracks.
func (k DrillKind) OpensSessions() bool {
	switch k {
	case DrillSessionsAll, drillSessionsBucket, drillSessionsShape,
		DrillSessionsOpener, DrillSessionsCloser:
		return true
	}
	return false
}

// Row is one cursor-selectable row of a panel body.
type Row struct {
	// Section groups consecutive rows; a header is emitted when it changes.
	Section  string
	Label    string
	note     string
	fraction float64
	Drill    DrillKind
	// Key is the artist, album or genre the drill filters on. Empty for
	// drills that need no Key.
	Key string
}

// MergePlan is a genre merge waiting on the confirmation bar.
type MergePlan struct {
	Canonical string
	Variants  []string
	Hashes    []string
}

// View renders the Insights briefing (level 0) and the track list a
// briefing row drills into (level 1, an embedded library.View so sort, filter,
// visual select and enqueue all come for free).
type View struct {
	Snap  insightsdata.Snapshot
	Panel Panel
	// WindowIdx and resolutionIdx are separate controls sharing one key: w
	// moves the window on windowed panels and the bucket size on the
	// resolution ones, so neither cycle ever has a press that changes nothing.
	WindowIdx     int
	resolutionIdx int

	Cursor       int
	ScrollOffset int

	Level      int // one of the insightsLevel* constants
	TrackView  libtui.View
	DrillLabel string

	// Sessions is every session in the window; sessionList is the subset a
	// Sessions row drilled into, with its own cursor because a session list is
	// not a track list and cannot borrow library.View's.
	Sessions      []insightsdata.SessionRow
	SessionList   []insightsdata.SessionRow
	sessionCursor int
	sessionScroll int
	SessionLabel  string

	// GenreCoverage and dormant are derived from the in-memory library rather
	// than from insights.Snapshot: CoverageRow carries no genre, and the
	// snapshot keeps no per-track last-played detail.
	GenreCoverage []insightsdata.ShelfStat
	Dormant       []library.Track

	// Plays and timeline are the raw history the time-series panels resolve
	// their drills against. Snapshot aggregates track identity away, so without
	// these an hour column or a month bar would be a picture with no query
	// behind it.
	Plays    []insightsdata.PlayRow
	Timeline []insightsdata.TimelineRow
	// GenreHours and backlog need the library's genres and add dates, neither
	// of which reaches insights.Snapshot, so they are folded at load time.
	GenreHours []GenreHours
	Backlog    []BacklogMonth
	// Health is the Data Health panel, findings and affected hashes together.
	Health []HealthRow
	// PendingMerge is the genre merge the confirmation bar is asking about.
	PendingMerge *MergePlan
	// Loc is the zone the snapshot was bucketed in, so a row's own bucketing
	// cannot disagree with the aggregate it sits under.
	Loc *time.Location

	Loaded  bool
	Loading bool
	// Stale marks the memo as superseded by a play, a rescan or a metadata
	// edit. The previous snapshot stays on screen until the replacement lands,
	// so an invalidation never flashes an empty view.
	Stale bool
	// LoadErr is the aggregate of the readers that failed on the last load. It
	// keeps the automatic refresh from retrying a broken read every tick, so
	// only an entry to the view or a change to the data clears it.
	LoadErr error
	// Gen increments on every load so a slow reply for an abandoned window
	// cannot overwrite a newer one.
	Gen int

	width, Height int
}

func (v *View) SetSize(w, h int) { v.width = w; v.Height = h }

func (v View) window() Window { return Windows[v.WindowIdx] }

func (v View) Resolution() insightsdata.Resolution {
	return insightsResolutions[v.resolutionIdx]
}

func (v View) Spec() PanelSpec {
	for _, spec := range insightsPanelSpecs {
		if spec.id == v.Panel {
			return spec
		}
	}
	return insightsPanelSpecs[0]
}

func (v View) compact() bool { return v.width > 0 && v.width < insightsCompactWidth }

// Empty reports whether there is nothing at all to show: no plays recorded and
// no library to measure them against.
func (v View) Empty() bool {
	return v.Snap.TotalPlays == 0 && v.Snap.Coverage.TotalTracks == 0
}

// Rows returns the cursor-selectable Rows of the active panel. Panels whose
// bodies have not landed yet return none; their headlines are live regardless.
func (v View) Rows() []Row {
	switch v.Panel {
	case PanelCoverage:
		return v.coverageRows()
	case panelRhythm:
		return v.rhythmRows()
	case PanelMomentum:
		return v.momentumRows()
	case PanelConcentration:
		return v.concentrationRows()
	case PanelTimeVsPlays:
		return v.timeVsPlaysRows()
	case PanelSessions:
		return v.sessionRows()
	case PanelDiscovery:
		return v.discoveryRows()
	case PanelDataHealth:
		return v.healthRows()
	}
	return nil
}
