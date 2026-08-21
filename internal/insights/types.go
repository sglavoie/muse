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
	DeepCutsCandidates int // non-missing tracks with 1..5 plays
	DeepCutsQualifying int // ...of which average completion clears the threshold
}

// Input bundles everything Compute needs. Now and Loc are injected: Compute
// never calls time.Now and never reads time.Local.
type Input struct {
	Now         time.Time
	Loc         *time.Location // nil is treated as UTC
	Since       int64          // 0 = all time
	WindowLabel string
	Plays       []PlayRow // pre-filtered to StartAt >= Since
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

// MonthBucket aggregates one local calendar month.
type MonthBucket struct {
	Year         int
	Month        time.Month
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
// finding new music" (Discovery). Both ignore Since.
type Trends struct {
	Months               []MonthBucket // contiguous, oldest first
	Current              MonthBucket
	AvgMonthlySecs       float64 // excludes the current, still-incomplete month
	AvgMonthlyFirstPlays float64 // likewise; the rate Coverage projects with
	CurrentVsAvg         float64 // 0 when there is no completed month to compare to
	DrySpells            []DrySpell
	LongestDry           DrySpell
	FirstPlaysMonth      int     // first-ever plays in the current month
	RepeatShare          float64 // share of the current month's plays that were repeats
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
