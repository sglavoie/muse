// Package insights turns raw listening history into the aggregates the
// Insights view renders. It is pure: no database, no TUI, no clock. Every
// time-dependent input (the current instant and the display timezone) is
// injected through Input so the whole package is deterministically testable,
// mirroring internal/radio.
//
// One rule governs every timestamp in this package: play_history.played_at is
// recorded when playback *ends*, so a play's start is played_at -
// listened_secs. Callers must hand over already-corrected start timestamps;
// nothing here re-derives them.
package insights

import "time"

// PlayRow is one play, already corrected to its start instant.
type PlayRow struct {
	Hash           string
	StartAt        int64 // Unix seconds; played_at - listened_secs
	ListenedSecs   int
	CompletionRate float64
	SessionID      string
	Artist         string
	Album          string
	Title          string
}

// GenreRow is a per-genre aggregate read through the track_genres junction, so
// a track tagged "rock,pop" contributes to both "rock" and "pop" instead of a
// single composite bucket. Genre is the raw stored string, collisions and all.
type GenreRow struct {
	Genre        string
	Plays        int
	ListenedSecs int
	Tracks       int
}

// CoverageRow is one library track. Coverage is always all-time and always
// unfiltered by the active window, because "never played" only means anything
// against the whole history.
type CoverageRow struct {
	Path         string
	AlbumArtist  string
	Hash         string
	Artist       string
	Album        string
	DurationSecs float64
	PlayCount    int
	AddedAt      int64
	LastPlayedAt int64
}

// SessionRow is one listening session derived from play_history.session_id.
// StartedAt is start-corrected like every other timestamp here; the
// listening_sessions table stores an end-based started_at and is deliberately
// not read.
type SessionRow struct {
	ID              string
	StartedAt       int64
	EndedAt         int64
	Plays           int
	ListenedSecs    int
	DistinctArtists int
	OpenerArtist    string
	OpenerTitle     string
	CloserArtist    string
	CloserTitle     string
}

// TimelineRow is one all-time play reduced to what the calendar, momentum and
// discovery panels need. These panels ignore Input.Since — the window only
// picks their resolution — so they read this all-time slice instead of Plays.
type TimelineRow struct {
	Hash         string
	StartAt      int64
	ListenedSecs int
	FirstEver    bool // first recorded play of its track
}

// QualityCounts holds the scalar library-health counts that are cheaper to get
// from SQL than to recompute in Go. All figures are all-time.
type QualityCounts struct {
	TracksTotal        int // non-missing
	TracksMissing      int
	TracksWithoutGenre int
	PlaysTotal         int
	PlaysWithoutGenre  int
	SkipsRecorded      int // uncounted listens on record; below DeepCutsMinSkips the share below is not yet measurable
	DeepCutsCandidates int // non-missing tracks with 1..5 plays
	DeepCutsQualifying int // ...of which average completion clears the threshold
}

// Resolution is the bucket size Momentum and Discovery aggregate at. It is the
// only thing their control changes: the source span stays all-time either way,
// so a coarser bucket is a coarser reading of the same history rather than a
// shorter one.
type Resolution int

// The bucket sizes. Monthly is the zero value so a caller that sets no
// resolution gets the calendar-month series the rest of the package projects
// with.
const (
	ResolutionMonthly Resolution = iota
	ResolutionWeekly
	ResolutionDaily
)

// Unit names the bucket size as an adverb, for "resolution: weekly".
func (r Resolution) Unit() string {
	switch r {
	case ResolutionDaily:
		return "daily"
	case ResolutionWeekly:
		return "weekly"
	default:
		return "monthly"
	}
}

// Current names the bucket the present falls in, so a headline reads as a
// sentence at every bucket size rather than saying "this day".
func (r Resolution) Current() string {
	switch r {
	case ResolutionDaily:
		return "today"
	case ResolutionWeekly:
		return "this week"
	default:
		return "this month"
	}
}

// Period names one bucket as a noun, for "replays a month in order".
func (r Resolution) Period() string {
	switch r {
	case ResolutionDaily:
		return "day"
	case ResolutionWeekly:
		return "week"
	default:
		return "month"
	}
}

// BucketStart normalizes an instant to the start of the bucket holding it.
// Every boundary is rebuilt from local calendar fields rather than reached by
// subtracting a fixed duration, so the 23- and 25-hour days a DST transition
// produces cannot slide a bucket off local midnight.
func (r Resolution) BucketStart(t time.Time) time.Time {
	loc := t.Location()
	switch r {
	case ResolutionDaily:
		return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
	case ResolutionWeekly:
		// Weeks start on Monday: this listening history tracks the work week,
		// and a Sunday start would cut it in half.
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc)
		w := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
		return time.Date(w.Year(), w.Month(), w.Day(), 0, 0, 0, 0, loc)
	default:
		return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, loc)
	}
}

// next advances one whole bucket and re-normalizes to local midnight.
func (r Resolution) next(start time.Time) time.Time {
	var n time.Time
	switch r {
	case ResolutionDaily:
		n = start.AddDate(0, 0, 1)
	case ResolutionWeekly:
		n = start.AddDate(0, 0, 7)
	default:
		n = start.AddDate(0, 1, 0)
	}
	return r.BucketStart(n)
}

// between counts whole buckets from a to b. Both must be bucket starts.
func (r Resolution) between(a, b time.Time) int {
	switch r {
	case ResolutionDaily:
		return dayDiff(a, b)
	case ResolutionWeekly:
		return dayDiff(a, b) / 7
	default:
		return (b.Year()-a.Year())*12 + int(b.Month()) - int(a.Month())
	}
}

// trailingCap bounds how far past the last play a series is extended toward the
// present, so a skewed system clock cannot generate an unbounded run of empty
// buckets. Each cap is about two years of that bucket size.
func (r Resolution) trailingCap() int {
	switch r {
	case ResolutionDaily:
		return 732
	case ResolutionWeekly:
		return 105
	default:
		return 24
	}
}

// Input bundles everything Compute needs. Now and Loc are injected: Compute
// never calls time.Now and never reads time.Local.
type Input struct {
	Now         time.Time
	Loc         *time.Location // nil is treated as UTC
	Since       int64          // 0 = all time
	WindowLabel string
	Resolution  Resolution // bucket size for Momentum and Discovery
	Plays       []PlayRow  // pre-filtered to StartAt >= Since
	Genres      []GenreRow
	Sessions    []SessionRow
	Coverage    []CoverageRow // ALWAYS all-time
	Timeline    []TimelineRow // ALWAYS all-time
	Quality     QualityCounts
}

// Snapshot is the computed state of every Insights panel. Compute(Input{})
// returns the zero Snapshot, so a caller that has not loaded anything yet can
// render the same struct it will render later.
type Snapshot struct {
	WindowLabel  string
	Since        int64
	TotalPlays   int
	TotalSecs    int
	FirstPlayAt  int64
	LastPlayAt   int64
	SpanDays     int
	Rhythm       Rhythm
	Calendar     Calendar
	Trends       Trends
	Rankings     Rankings
	SessionStats SessionStats
	Coverage     Coverage
	Quality      Quality
}

// HourBucket counts the plays whose *start* falls in a local hour.
type HourBucket struct {
	Hour         int
	Plays        int
	ListenedSecs int
}

// WeekdayBucket counts the plays whose *start* falls on a local weekday.
type WeekdayBucket struct {
	Weekday      time.Weekday
	Plays        int
	ListenedSecs int
	Share        float64
}

// Rhythm answers "when do I listen".
type Rhythm struct {
	Hours          [24]HourBucket
	Weekdays       [7]WeekdayBucket
	PeakHour       int
	PeakHourPlays  int
	QuietHours     []int // local hours with zero plays, ascending
	TopWeekday     WeekdayBucket
	BottomWeekday  WeekdayBucket
	Genres         []GenreGroup // folded; shown behind the Data Health banner
	GenresUnclean  bool         // at least one genre group has multiple variants
	TotalPlays     int
	TotalSecs      int
	DistinctTracks int
}

// CalendarDay is one local day of the all-time heatmap.
type CalendarDay struct {
	Date         time.Time // local midnight in Input.Loc
	Plays        int
	ListenedSecs int
}

// Calendar is the contiguous day-by-day history, gaps included.
type Calendar struct {
	Days     []CalendarDay
	MaxPlays int
	Weeks    int
}

// TrendBucket aggregates one bucket of the all-time timeline at the active
// Resolution. Start is the local instant the bucket opens at and is the only
// identity it carries: labels and drill keys are formatted from it, so the row
// a caller draws and the query behind that row cannot disagree.
type TrendBucket struct {
	Start        time.Time
	Plays        int
	ListenedSecs int
	FirstPlays   int // plays that were a track's first ever
}

// DrySpell is a run of consecutive days with no plays at all.
type DrySpell struct {
	Start time.Time
	End   time.Time
	Days  int
}

// Trends answers "growing, shrinking or bursty" (Momentum) and "am I still
// finding new music" (Discovery). Both ignore Since and read the whole history
// at Resolution.
type Trends struct {
	Resolution        Resolution
	Buckets           []TrendBucket // contiguous, oldest first
	Current           TrendBucket   // the bucket today falls in; still filling up
	AvgSecs           float64       // per bucket, excluding the current one
	AvgFirstPlays     float64       // likewise
	CurrentVsAvg      float64       // 0 when there is no completed bucket to compare to
	DrySpells         []DrySpell
	LongestDry        DrySpell
	CurrentFirstPlays int     // first-ever plays in the current bucket
	RepeatShare       float64 // share of the current bucket's plays that were repeats
	// AvgMonthlyFirstPlays is always a per-month rate whatever the Resolution:
	// Coverage projects a finish date in months and must not be re-scaled by a
	// control that only governs how Momentum and Discovery are drawn.
	AvgMonthlyFirstPlays float64
}

// EntityStat is one artist (or album) ranked within a window.
type EntityStat struct {
	Name         string
	Plays        int
	ListenedSecs int
	Rank         int // 1-based
}

// RankDelta records how far an entity moves between the count ranking and the
// hours ranking. Delta is PlayRank - HoursRank, so a positive Delta means the
// entity ranks *higher* (nearer 1) when measured by time than by play count.
type RankDelta struct {
	Name      string
	PlayRank  int
	HoursRank int
	Delta     int
}

// Ladder is the repertoire ladder: how few artists account for each share of
// total listening time.
type Ladder struct {
	TotalArtists int
	Artists25    int
	Artists50    int
	Artists75    int
	Artists90    int
}

// Rankings feeds both the Concentration and the Time vs Plays panels.
type Rankings struct {
	ArtistsByPlays []EntityStat
	ArtistsByHours []EntityStat
	Deltas         []RankDelta
	MovedCount     int // entries in Deltas whose Delta is non-zero
	Ladder         Ladder
	TopFiveShare   float64 // share of listening time held by the top 5 artists
}

// SessionStats answers "what does a session look like".
type SessionStats struct {
	Count             int
	MedianSecs        int
	LongestSecs       int
	TotalSecs         int
	SinglePlay        int
	SingleArtistCount int
	SingleArtistShare float64
	AvgPlays          float64
	Durations         []int        // per-session wall-clock seconds, ascending
	TopOpeners        []EntityStat // by artist
	TopClosers        []EntityStat
}

// ShelfStat is one artist or album shelf measured by how much of it is unheard.
type ShelfStat struct {
	Name        string
	Tracks      int
	Unheard     int
	UnheardSecs float64
}

// Coverage is the centerpiece: how much of the library has never been heard.
// Always all-time regardless of the active window.
type Coverage struct {
	TotalTracks    int
	HeardTracks    int
	UnheardTracks  int
	HeardShare     float64
	UnheardSecs    float64
	ListenedSecs   float64 // all-time listening time, for the size comparison
	UnheardVsHeard float64 // UnheardSecs / ListenedSecs; 0 when nothing was heard
	MonthsToFinish float64 // at the current first-play rate; 0 when the rate is 0
	DustyArtists   []ShelfStat
	DustyAlbums    []ShelfStat
}

// GenreGroup folds every case/whitespace variant of a genre into one entry.
// Variants keeps the raw stored strings so Data Health can merge them exactly
// as written.
type GenreGroup struct {
	Canonical    string
	Variants     []string
	Plays        int
	ListenedSecs int
	Tracks       int
}

// IssueKind identifies a Data Health finding.
type IssueKind string

// The Data Health findings Compute can report.
const (
	IssueGenreCollision   IssueKind = "genre-collision"
	IssuePlaceholderGenre IssueKind = "placeholder-genre"
	IssueGenreMissing     IssueKind = "genre-missing"
	IssueDeepCutsFlat     IssueKind = "deep-cuts-flat"
	IssueMissingFiles     IssueKind = "missing-files"
)

// HealthIssue is one row of the Data Health panel.
type HealthIssue struct {
	Kind     IssueKind
	Title    string
	Detail   string
	Affected int  // plays or tracks distorted, whichever the issue is about
	Fixable  bool // true when the panel offers an in-app fix
}

// Quality answers "what is lying to me above".
type Quality struct {
	Genres           []GenreGroup // every folded group, most played first
	Collisions       []GenreGroup // only the groups with more than one variant
	CollisionPlays   int
	Placeholders     []GenreGroup // groups whose name stands in for a real genre
	PlaceholderPlays int
	Issues           []HealthIssue
	DeepCutsShare    float64 // qualifying / candidates; ~1.0 means the filter is inert
	Counts           QualityCounts
}
