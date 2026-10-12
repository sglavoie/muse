package tui

import (
	"errors"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// djPersistMaxRetries bounds the automatic reconciliation of a lifecycle write
// the database refused. Three attempts on the one-second tick covers the
// transient lock the retry exists for; past that the row is genuinely
// unwritable, and asking again every second for the rest of the run would only
// bury the notice that says so.
const djPersistMaxRetries = 3

// errDJUnsaved heads every report of a session the database would not take. It
// is worded for the Tune panel, which renders the error verbatim.
var errDJUnsaved = errors.New("applied for now, not saved")

// djSessionState is the Model's view of the persisted Auto-DJ session.
//
// Every field here survives a restart, which is why the transitions below are
// the only writers. A handler that flipped `active` on its own, or replaced the
// seed without going through djReseed, would leave the database describing a
// session that is no longer the one running.
type djSessionState struct {
	// active reports whether a DJ session is running.
	active bool

	// settings is the applied snapshot in force. It survives a stop so the
	// Tune tab reopens on the user's last filters rather than stock defaults.
	settings radio.Settings

	// seed is the captured starting context. Under AnchorSeed it is frozen for
	// the life of the session; under AnchorFollow it moves on confirmed track
	// changes only.
	seed radio.SeedSnapshot

	// allocation is the discovery rounding carry. It is adopted only when a
	// batch is accepted, reset by a deliberate new start, and restored intact
	// across a restart so a long session's discovery share stays honest.
	allocation radio.Allocation

	// restoreErr records persisted state this build refused to use. It is kept
	// rather than repaired: stock defaults in place of unreadable filters would
	// silently turn a restrictive session into an unrestricted one.
	//
	// It also bars every automatic route into the queue while it is set — see
	// djRestoreBlocked — because keeping the settings honest in the Tune tab is
	// worth nothing if the refill goes on adding tracks against the defaults
	// that stood in for them.
	restoreErr error

	// restored reports that the current seed came off disk rather than from a
	// deliberate start in this run. It is what tells a restart ("resume the
	// session that was running") apart from a fresh start ("capture a new
	// seed"), and it is cleared by the first deliberate transition.
	restored bool

	// unsaved is a lifecycle write the database refused. The transition itself
	// is kept, so this is the record that the running session and the saved one
	// disagree — reconciled by djReconcilePersist and reported until they agree.
	unsaved djUnsavedWrite

	// staleRefs is a merge whose durable reference repair the database refused.
	// The merge itself succeeded, so this is not an unsaved session: it is the
	// record that some stored filter — the session's, a recipe's — may still name
	// the merged-away tag, reconciled by djReconcileTagRefs.
	staleRefs djStaleTagRefs
}

// djTagMerge is one merge's source and destination tag, kept for a repair that
// has not landed yet.
type djTagMerge struct{ src, dest int64 }

// djStaleTagRefs is the durable reference repair one or more merges are still
// owed.
//
// The merges are held in the order they happened and replayed in that order: a
// destination tag can be merged away in turn, and re-pointing the older merge
// after the newer one would move its references onto a tag that is itself gone.
type djStaleTagRefs struct {
	// merges is the repairs still outstanding, oldest first.
	merges []djTagMerge
	// err is the refusal, kept for the surfaces that report it.
	err error
	// attempts counts the automatic retries spent against djPersistMaxRetries.
	// A later merge starts a fresh budget, for the same reason a deliberate
	// gesture does.
	attempts int
}

// pending reports whether some stored filter may still name a merged-away tag.
func (s djStaleTagRefs) pending() bool { return len(s.merges) > 0 }

// exhausted reports that the bounded reconciliation gave up.
func (s djStaleTagRefs) exhausted() bool { return s.attempts >= djPersistMaxRetries }

// djUnsavedWrite is a lifecycle transition the database would not take.
//
// The Model keeps such a transition rather than rolling it back, because
// refusing it would leave the user looking at a DJ that ignored their keypress.
// What it must not do is pretend the transition is durable: the row on disk
// still describes the previous session, and a restart would restore it. This is
// what remembers that until a later write reconciles it.
type djUnsavedWrite struct {
	// err is the refusal, kept for the surfaces that report it.
	err error

	// attempts counts the automatic retries already spent against
	// djPersistMaxRetries. A deliberate gesture starts a fresh budget, because a
	// user who applies again is owed as many tries as the first time.
	attempts int

	// snapshot reports that the whole row is stale, which a stop cannot
	// reconcile on its own: StopDJSession clears the seed and the carry and
	// leaves the settings columns exactly as they were.
	snapshot bool
}

// pending reports whether the saved session is behind the running one.
func (u djUnsavedWrite) pending() bool { return u.err != nil }

// exhausted reports that the bounded reconciliation gave up.
func (u djUnsavedWrite) exhausted() bool { return u.attempts >= djPersistMaxRetries }

// djSessionWriter persists Auto-DJ session transitions. Implemented by
// *db.Writer; nil in tests and in any build without a database.
type djSessionWriter interface {
	WriteDJSession(session db.DJSession) error
	StopDJSession() error
	// RemapDJTagRefs re-points the stored filter and seed references a merged
	// tag left behind. It is here rather than on the recipe writer because it
	// rewrites every DJ-owned reference at once — the session's and every
	// recipe's — which is exactly what a merge means.
	RemapDJTagRefs(sourceID, destID int64) error
}

// defaultDJSettings is the settings a Model starts with before anything is
// restored: Balanced, anchored, unfiltered.
func defaultDJSettings() radio.Settings { return radio.DefaultSettings() }

// DJActive reports whether an Auto-DJ session is running.
func (m Model) DJActive() bool { return m.dj.active }

// DJSeed returns the seed the running session was started from.
func (m Model) DJSeed() radio.SeedSnapshot { return m.dj.seed }

// DJSettings returns the applied settings snapshot.
func (m Model) DJSettings() radio.Settings { return m.dj.settings }

// DJAllocation returns the discovery rounding carry.
func (m Model) DJAllocation() radio.Allocation { return m.dj.allocation }

// DJRestoreErr returns the persisted state this build refused, or nil.
func (m Model) DJRestoreErr() error { return m.dj.restoreErr }

// djRestoreBlocked reports whether refused persisted state bars automatic work.
//
// The settings standing in for state that could not be read are stock defaults:
// unfiltered, Balanced, anchored. They are what the Tune tab has to open on —
// there is nothing else to show — but they are emphatically not what the user
// asked for, and the saved row they replaced may have excluded most of the
// library. So every route that adds tracks without being asked is refused while
// this holds: planning, the hidden preparation it writes, the acceptance that
// publishes it, and the immediate refresh. What is not refused is anything the
// user drives — manual playback, the queue they build by hand, and the
// deliberate start or Apply that establishes settings and clears this.
func (m Model) djRestoreBlocked() bool { return m.dj.restoreErr != nil }

// djSessionUnset reports a restored session that describes nothing at all.
//
// "Nothing was saved" and "what was saved could not be read" look similar and
// mean opposite things. A first launch, a database the DJ migration has only
// just seeded, and a startup that restored no session hand back the zero value
// with no error; there are no filters to lose there, so the DJ comes up on stock
// defaults and the refill runs. Only state that exists and cannot be used is
// refused. Storage never returns unreadable rows with a nil error, so the
// Validate below stays reachable for a caller that built a session by hand.
func djSessionUnset(session db.DJSession) bool {
	s := session.Settings
	return !session.Active && session.Seed.IsZero() && s.Anchor == "" &&
		s.DiscoveryPercent == 0 && s.Include.IsEmpty() && s.Exclude.IsEmpty()
}

// djSnapshot renders the current lifecycle state as the row storage holds.
func (m Model) djSnapshot() db.DJSession {
	return db.DJSession{
		Active:     m.dj.active,
		Settings:   m.dj.settings,
		Seed:       m.dj.seed,
		Allocation: m.dj.allocation,
	}
}

// djRestore adopts persisted Auto-DJ state at startup.
//
// A restored session keeps its seed: "restarting Muse restores the active
// session's seed" is the whole difference between a restart and a new start,
// and re-capturing here would silently re-point a session at whatever happened
// to be playing when it was closed. Settings are adopted even when the session
// was stopped, so the Tune tab reopens where the user left it.
//
// restoreErr is state that could not be read. The DJ then starts stopped on
// stock settings, because the alternative — guessing at the missing filters —
// is the one outcome the shared contract forbids.
func (m Model) djRestore(session db.DJSession, restoreErr error) Model {
	if restoreErr != nil {
		m.dj = djSessionState{settings: defaultDJSettings(), restoreErr: restoreErr}
		return m
	}
	if djSessionUnset(session) {
		m.dj = djSessionState{settings: defaultDJSettings()}
		return m
	}
	settings := session.Settings
	if err := settings.Validate(); err != nil {
		// Defence in depth: storage already refuses invalid settings, so this
		// only fires for a caller that built a DJSession by hand.
		m.dj = djSessionState{settings: defaultDJSettings(), restoreErr: err}
		return m
	}
	m.dj = djSessionState{
		active:     session.Active && !session.Seed.IsZero(),
		settings:   settings.Normalized(),
		seed:       session.Seed,
		allocation: session.Allocation.Normalized(),
		restored:   !session.Seed.IsZero(),
	}
	return m
}

// djStart begins a deliberate new session from a freshly captured seed.
//
// The allocation is reset because the rounding carry belongs to the session
// that earned it: carrying a previous session's remainder into a new one would
// bias its first batch for reasons the user cannot see. A zero seed is refused
// rather than started, since the planner would reject every batch it produced.
func (m Model) djStart(seed radio.SeedSnapshot) Model {
	if seed.IsZero() {
		return m
	}
	m.dj.active = true
	m.dj.seed = seed
	m.dj.allocation = radio.Allocation{}
	m.dj.restored = false
	// A deliberate start is the user establishing what the DJ runs on, which is
	// what lifts the refusal a failed restore put in place: the settings in force
	// are the ones this transition is about to persist over the row that could
	// not be read. Leaving it up here would make the start gesture a key that
	// enables Radio and then never plays anything.
	m.dj.restoreErr = nil
	// A refill already in flight was planned about the previous subject, so it
	// is disowned here rather than left for the acceptance check: under Follow
	// anchoring a moved seed is ordinary drift, and only the request identity
	// can tell that one apart from the user deliberately starting elsewhere.
	return m.supersedeRadioBatch().djPersist()
}

// djStartFor begins a session seeded by the given request and returns the label
// describing what it was seeded from.
//
// This is the single entry point every "turn the DJ on" keypress goes through.
// Keeping resolution and the transition together is what stops one entry point
// from starting a session the persistence layer never hears about, or from
// capturing a seed the lifecycle then discards.
func (m Model) djStartFor(req djSeedRequest, now time.Time) (Model, string) {
	seed, label := m.djResolveSeed(req, now)
	return m.djStart(seed), label
}

// djStartFromTracks begins a session seeded from a collection the caller
// resolved itself — the open mix, the Insights drilldown — where the view's own
// notion of "the targeted collection" is richer than the generic visible-track
// list. An explicit bulk selection still outranks it, and an empty collection
// falls back to the current track.
func (m Model) djStartFromTracks(label string, tracks []library.Track, now time.Time) (Model, string) {
	if seed, selLabel, ok := m.djSelectionSeed(now); ok {
		return m.djStart(seed), selLabel
	}
	if len(tracks) == 0 {
		return m.djStartFor(djSeedCurrent, now)
	}
	seed, seedLabel := m.djSeedFromTracks(label, tracks, now)
	return m.djStart(seed), seedLabel
}

// djStop ends the session, keeping the settings and dropping the seed.
//
// Dropping the seed is what makes the next start a genuinely new one: a stopped
// session with a seed still on disk would be resumed by the next launch as if
// the user had never stopped it.
func (m Model) djStop() Model {
	m.dj.active = false
	m.dj.seed = radio.SeedSnapshot{}
	m.dj.allocation = radio.Allocation{}
	m.dj.restored = false
	return m.supersedeRadioBatch().djPersistStop()
}

// djResume re-activates a restored-but-stopped session against its own seed.
// The second result is false when there is no seed to resume, which is the
// caller's cue to capture a fresh one instead.
func (m Model) djResume() (Model, bool) {
	if m.dj.seed.IsZero() {
		return m, false
	}
	m.dj.active = true
	return m.djPersist(), true
}

// djReseed replaces the seed of a running session, leaving the allocation
// alone: the session continues, so its rounding carry continues with it.
// Follow can reseed from an unsolicited player report after a deliberate stop;
// invalidating the old seed's work must preserve that playback suspension.
func (m Model) djReseed(seed radio.SeedSnapshot) Model {
	if !m.dj.active || seed.IsZero() || seed.TrackHash == m.dj.seed.TrackHash {
		return m
	}
	m.dj.seed = seed
	m.dj.restored = false
	return m.invalidateRadioBatch().djPersist()
}

// djApplySettings adopts a new settings snapshot.
//
// The allocation is reset because the discovery target it was accumulated
// against no longer applies; keeping it would spend a carry earned at 40% on a
// batch planned at 80%. Invalid settings are refused outright rather than
// normalized into something the user did not ask for.
func (m Model) djApplySettings(settings radio.Settings) (Model, error) {
	if err := settings.Validate(); err != nil {
		return m, err
	}
	m.dj.settings = settings.Normalized()
	m.dj.allocation = radio.Allocation{}
	m.dj.restoreErr = nil
	return m.supersedeRadioBatch().djPersist(), nil
}

// djAdoptAllocation records the rounding carry of an accepted batch. Only
// accepted work consumes allocation, so this is deliberately separate from the
// transitions above and writes the one column that moved.
func (m Model) djAdoptAllocation(alloc radio.Allocation) Model {
	if !m.dj.active {
		return m
	}
	m.dj.allocation = alloc.Normalized()
	return m.djPersist()
}
