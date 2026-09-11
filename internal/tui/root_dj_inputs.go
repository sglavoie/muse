package tui

import (
	"errors"
	"fmt"
	"time"

	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// djHydrationPerGroup is how many candidates from *each* pool have their
// behavioral data loaded before the final score.
//
// Per group, not overall. A single global top-N hands every hydration slot to
// whichever pool ranks higher on metadata alone, so the other pool is scored
// with no completion rates and no co-occurrence and loses every close contest —
// which is how an 80% Explore target quietly turns into a familiar batch.
// radio.Pools.HydrationTargets takes this per group for that reason.
const djHydrationPerGroup = 3 * radioBatchSize

// errDJTagCatalogUnavailable reports that the tag catalog could not be read, so
// no verdict about tag-filtered eligibility is available.
var errDJTagCatalogUnavailable = errors.New("tui: dj tag catalog unavailable")

// djDataState is the Model's freshness bookkeeping for Auto-DJ inputs.
type djDataState struct {
	// dataRev counts changes to anything the DJ plans from: counted listens,
	// scan and watcher updates, missing-file reconciliation, metadata edits,
	// imports and their undos, favorites, dismissals, and tag changes.
	//
	// It is a revision, not a dirty bit, because several results may be in
	// flight at once and each has to be judged against the exact data it was
	// computed from.
	dataRev uint64
}

// djInvalidate records that something the Auto-DJ reads has changed.
//
// Planning runs off the update loop against a snapshot, so a result computed
// before the change describes a library that no longer exists: advancing the
// revision is what lets the acceptance path recognize and refuse it instead of
// committing a batch whose classification or filtering is already wrong.
//
// The revision alone only serves the paths that re-check it before accepting,
// which the refill does and the Apply refresh cannot: the refresh captured its
// snapshot once and is already off the update loop, past every re-check. Raising
// its withdrawal flag is how the same change reaches it, and the flag is an
// atomic precisely so this can be done from here without the refresh reading any
// of the maps the revision describes.
//
// Pointer receiver for the same reason insightsInvalidate has one: most callers
// are handlers holding a value copy of the Model, and the bump has to land on
// the copy they return.
func (m *Model) djInvalidate() {
	m.djData.dataRev++
	m.djRefresh.withdraw.invalidate()
}

// djDataRevision returns the revision a captured snapshot must still match to
// be accepted.
func (m Model) djDataRevision() uint64 { return m.djData.dataRev }

// djInputs is an immutable snapshot of every Model-owned input to one Auto-DJ
// planning attempt.
//
// Immutability is load-bearing rather than decorative. Planning happens on a
// goroutine while the update loop keeps mutating the live maps — applyTagToggle
// rewrites trackTags, a dismissal writes dismissedHashes, a favorite toggle
// writes favoriteHashes — so every map here is a private copy taken on the
// update loop before the goroutine starts, and nothing writes to a captured
// snapshot afterwards. The fields are unexported so even a caller inside this
// package has to go through PlanInput rather than reach in and edit one.
type djInputs struct {
	dataRev uint64
	// queueRev reduces the protected-hash set to one comparable token. It is
	// tracked apart from dataRev because the two move for unrelated reasons and
	// at wildly different rates: dataRev follows library and metadata writes,
	// while queueRev follows playback and every queue mutation.
	queueRev   uint64
	capturedAt time.Time
	tracks     []library.Track
	trackTags  map[string]map[int64]struct{}
	favorites  map[string]struct{}
	recent     map[string]struct{}
	protected  map[string]struct{}
	dismissed  map[string]struct{}
	// catalog is nil when the tag cache could not be trusted; catalogErr then
	// says why. An empty non-nil catalog is a real answer: it means the library
	// has no tags, so every tag reference in a filter is unresolved.
	catalog    radio.TagIDSet
	catalogErr error
}

// DataRev returns the data revision this snapshot was taken at.
func (in djInputs) DataRev() uint64 { return in.dataRev }

// QueueRev returns the queue revision this snapshot was taken at.
func (in djInputs) QueueRev() uint64 { return in.queueRev }

// CapturedAt returns the instant the snapshot was taken.
func (in djInputs) CapturedAt() time.Time { return in.capturedAt }

// Candidates returns how many library tracks the snapshot carries.
func (in djInputs) Candidates() int { return len(in.tracks) }

// captureDJInputs copies everything one planning attempt reads out of the
// Model. Call it on the update loop; the result is safe to hand to a goroutine.
func (m Model) captureDJInputs(now time.Time) djInputs {
	in := djInputs{
		dataRev:    m.djData.dataRev,
		capturedAt: now,
		// LibraryIndex.Tracks already returns a fresh slice, so no second copy
		// is needed here. The nested tag maps are not copied by anything, which
		// is what snapshotTrackTags is for.
		tracks:     m.library.Index.Tracks(),
		trackTags:  snapshotTrackTags(m.trackTags),
		favorites:  snapshotStringSet(m.favoriteHashes),
		recent:     m.radioRecentSet(),
		protected:  m.djProtectedHashes(),
		dismissed:  snapshotStringSet(m.dismissedHashes),
		catalogErr: m.tagCacheErr,
	}
	in.queueRev = djQueueRevisionOf(in.protected)
	if m.tagCacheErr == nil {
		in.catalog = m.djTagCatalog()
	}
	return in
}

// djTagCatalog projects the in-memory tag cache onto the catalog the planner
// resolves filter references against.
//
// The cache is loaded at startup and rebuilt by every tag mutation, so this
// costs no database round trip on the update loop. The result is always
// non-nil: "the library has no tags" and "we do not know what tags exist" are
// different answers, and only the second one may skip the unresolved check.
func (m Model) djTagCatalog() radio.TagIDSet {
	set := make(radio.TagIDSet, len(m.tags))
	for _, t := range m.tags {
		set[t.ID] = struct{}{}
	}
	return set
}

// djProtectedHashes returns the hashes a batch must not select again: the row
// playing now plus every row still to be visited.
//
// The player owns traversal order, so its snapshot is the answer whenever one
// exists — under shuffle the visible position of the playing row says nothing
// about which rows are still ahead. Rows already played are deliberately left
// out: protecting the whole persisted queue is what would shrink continuous
// radio's reachable library a little more with every batch it enqueues.
func (m Model) djProtectedHashes() map[string]struct{} {
	protected := make(map[string]struct{}, radioBatchSize)
	if h := m.playback.currentHash; h != "" {
		protected[h] = struct{}{}
	}
	t, ok := m.playerTraversal()
	if !ok {
		// Fallback for a player that exposes no traversal: persisted queue
		// order, which is only correct without shuffle. Same degraded
		// assumption radioRemainingCount already documents.
		for i := max(m.playback.currentIndex, 0); i < len(m.queue.Entries); i++ {
			m.protectQueueIndex(protected, i)
		}
		return protected
	}
	// A negative position means nothing is current, so the whole cycle is
	// still ahead.
	start := max(t.CurrentPosition, 0)
	for _, idx := range t.Order[min(start, len(t.Order)):] {
		m.protectQueueIndex(protected, idx)
	}
	return protected
}

// protectQueueIndex adds the hash at a queue index to protected. An index the
// traversal carries but the queue no longer holds, and a row with no hash, are
// both skipped: neither can be matched against a candidate anyway.
func (m Model) protectQueueIndex(protected map[string]struct{}, idx int) {
	if idx < 0 || idx >= len(m.queue.Entries) {
		return
	}
	if h := m.queue.Entries[idx].Hash; h != "" {
		protected[h] = struct{}{}
	}
}

// StaleAt reports whether this snapshot may still be planned from.
//
// Three things retire it. A data revision that no longer matches means a writer
// changed the library, metadata, favorites, dismissals, or tags it copied. A
// queue revision that no longer matches means the rows a batch must not select
// again are no longer the rows it avoided. A change of local hour means the
// inputs no writer announces have moved: the time-of-day affinity bonus is keyed
// by the hour, and the rolling 90-day discovery cutoff has slid with it. Without
// the last check a Muse left running overnight would keep scoring and
// classifying against the hour it was started in.
//
// This is the strict predicate, for deciding whether a snapshot may be planned
// FROM. Deciding whether a finished plan may still be accepted is a different
// and deliberately more lenient question — see radioQueueAdmissible, which
// tolerates the queue draining because that is the refill's whole purpose.
func (in djInputs) StaleAt(now time.Time, dataRev, queueRev uint64) bool {
	return dataRev != in.dataRev || queueRev != in.queueRev || djHourChanged(in.capturedAt, now)
}

// djHourChanged reports whether two instants fall in different local hours.
func djHourChanged(a, b time.Time) bool {
	ay, am, ad := a.Date()
	by, bm, bd := b.Date()
	return a.Hour() != b.Hour() || ad != bd || am != bm || ay != by
}

// eligibilityErr reports data without which the plan cannot be evaluated at
// all. The settings decide whether the missing catalog matters.
//
// A failure here must never degrade into a plan. With no catalog the planner
// skips its unresolved-tag check, and a tag deleted between runs then silently
// widens an exclusion or narrows an inclusion: the user would receive a batch
// that quietly ignores filters they set. Optional behavioral evidence is the
// opposite case and is handled by djSignals — a batch scored without completion
// rates is still the batch the user asked for.
func (in djInputs) eligibilityErr(s radio.Settings) error {
	if in.catalogErr == nil {
		return nil
	}
	if len(s.Include.TagIDs) == 0 && len(s.Exclude.TagIDs) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %w", errDJTagCatalogUnavailable, in.catalogErr)
}

// PlanInput builds the planner's metadata-only input. Behavioral evidence is
// layered on afterwards by djSignals.applyTo, because which hashes are worth
// loading is decided by partitioning this very input.
func (in djInputs) PlanInput(req radio.PlanRequest, alloc radio.Allocation) radio.PlanInput {
	pi := radio.PlanInput{
		Request:         req,
		Allocation:      alloc,
		Candidates:      in.tracks,
		TrackTags:       in.trackTags,
		FavoriteHashes:  in.favorites,
		RecentHashes:    in.recent,
		ProtectedHashes: in.protected,
		DismissedHashes: in.dismissed,
	}
	if in.catalog != nil {
		pi.TagCatalog = in.catalog
	}
	return pi
}

// djSignalFailure names one optional behavioral query that could not be read.
type djSignalFailure struct {
	Signal string
	Err    error
}

// djSignals is the optional behavioral evidence one plan was scored with, plus
// the record of what could not be read.
//
// Every signal here is optional by construction: the scorer treats an absent
// completion rate as no evidence rather than as a bad one, so a reader that
// fails costs the batch some precision and nothing more. That is exactly why
// these are kept apart from djInputs, whose absence blocks a plan outright.
type djSignals struct {
	completionRates map[string]float64
	coOccurrences   map[string]int
	timeGenreBonus  map[string]float64
	// hour is the local hour timeGenreBonus was built for.
	hour int
	// seeded records whether a seed hash was available to ask for
	// co-occurrence. An unseeded start has no co-occurrence to miss, so it is
	// not a degradation.
	seeded   bool
	failures []djSignalFailure
}

// Degraded reports whether any optional query failed, so scoring ran on less
// evidence than the database holds.
func (s djSignals) Degraded() bool { return len(s.failures) > 0 }

// Failures returns a copy of the optional queries that failed.
func (s djSignals) Failures() []djSignalFailure {
	if len(s.failures) == 0 {
		return nil
	}
	out := make([]djSignalFailure, len(s.failures))
	copy(out, s.failures)
	return out
}

// Seeded reports whether co-occurrence was asked for at all.
func (s djSignals) Seeded() bool { return s.seeded }

// Hour returns the local hour the time-of-day bonus describes.
func (s djSignals) Hour() int { return s.hour }

// StaleAt reports whether the time-of-day bonus now describes the wrong hour.
func (s djSignals) StaleAt(now time.Time) bool { return now.Hour() != s.hour }

// applyTo layers the behavioral maps onto a metadata-only planner input.
func (s djSignals) applyTo(in radio.PlanInput) radio.PlanInput {
	in.CompletionRates = s.completionRates
	in.CoOccurrences = s.coOccurrences
	in.TimeGenreBonus = s.timeGenreBonus
	return in
}

// djLoadSignals reads the behavioral evidence for one batch. It runs off the
// update loop.
//
// Three reads, never more, whatever the size of the library: one IN query
// covering the hydration targets of *both* pools, one hour-affinity query, and
// one co-occurrence query. Asking per candidate instead would issue a query per
// track and hold the goroutine for the length of the candidate set.
//
// The hour affinity is read whether or not there is a seed. It describes what
// the user listens to at this time of day, which is just as true of a synthetic
// seed, a collection start, or an unseeded whole-library session; gating it on a
// real seed hash — as the pre-DJ radio path did — threw the signal away for
// precisely the starts with the least other evidence to go on. Only
// co-occurrence is genuinely seed-relative.
func djLoadSignals(reader recommendationReader, hashes []string, seedHash string, now time.Time) djSignals {
	s := djSignals{
		completionRates: map[string]float64{},
		coOccurrences:   map[string]int{},
		timeGenreBonus:  map[string]float64{},
		hour:            now.Hour(),
		seeded:          seedHash != "",
	}
	if reader == nil {
		return s
	}
	fail := func(signal string, err error) {
		s.failures = append(s.failures, djSignalFailure{Signal: signal, Err: err})
	}

	if len(hashes) > 0 {
		rates, err := reader.ReadCompletionRates(hashes)
		switch {
		case err != nil:
			fail("completion rates", err)
		default:
			for h, r := range rates {
				s.completionRates[h] = r
			}
		}
	}

	if affinities, err := reader.ReadHourGenreAffinities(); err != nil {
		fail("time-of-day affinity", err)
	} else {
		s.timeGenreBonus = buildTimeGenreBonus(affinities, s.hour)
	}

	if s.seeded {
		coOccs, err := reader.ReadSessionCoOccurrences(seedHash, djCoOccurrenceLimit)
		if err != nil {
			fail("session co-occurrence", err)
		}
		for _, c := range coOccs {
			s.coOccurrences[c.Hash] = c.Count
		}
	}
	return s
}

// djCoOccurrenceLimit caps how many co-played tracks are pulled for a seed.
const djCoOccurrenceLimit = 100

// djPlanBatch runs one two-pass plan against a captured snapshot: partition on
// metadata alone, hydrate the top of *both* pools from one batched read each,
// then score and select.
//
// The error result is reserved for eligibility data that is unavailable. A
// behavioral read that failed comes back inside the returned djSignals with the
// plan intact, because degrading the score is an acceptable answer where
// guessing at eligibility is not.
func djPlanBatch(in djInputs, req radio.PlanRequest, alloc radio.Allocation, reader recommendationReader, now time.Time) (radio.PlanResult, djSignals, error) {
	if err := in.eligibilityErr(req.Settings); err != nil {
		return radio.PlanResult{}, djSignals{}, err
	}
	pi := in.PlanInput(req, alloc)
	pools := radio.Partition(pi)
	targets := pools.HydrationTargets(pi, djHydrationPerGroup)
	// CoOccurrenceHash rather than TrackHash: a collection or whole-library seed
	// carries a synthetic hash that matches no play-history row, so asking for
	// it would spend a query to learn nothing and would report the session as
	// seeded when its one seed-relative signal is unavailable by construction.
	signals := djLoadSignals(reader, targets, req.Seed.CoOccurrenceHash(), now)
	return radio.PlanFrom(signals.applyTo(pi), pools), signals, nil
}
