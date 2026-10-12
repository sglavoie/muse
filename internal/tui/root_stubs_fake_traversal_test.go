package tui

import (
	"errors"
	"fmt"

	"github.com/sglavoie/muse/internal/player"
)

// errFakeTraversalExhausted stands in for the player's end-of-traversal error:
// the extension added nothing this cycle can still play.
var errFakeTraversalExhausted = errors.New("fake player: traversal exhausted")

// errFakeNothingToReplay stands in for the Manager's "no current entry to
// replay" refusal.
var errFakeNothingToReplay = errors.New("fake player: nothing to replay")

// fakeTraversalPlayer stands in for player.Manager's ownership of the playback
// timeline: it holds a traversal order over queue rows that is independent of
// the visible queue order, remembers where the cycle sits, and resumes only into
// rows the cycle has not visited. It exists so the root handlers can be driven
// through a completed shuffle cycle and its refill without an mpv process.
//
// A fake that was never told the queue models tail growth only — appends and
// pastes at the end. Once SetQueue has seeded tracks it reconciles by identity
// instead, so a row inserted before the cursor shifts the queue without renaming
// the row the cycle is on. missing is keyed by queue index, so a test that
// inserts rows must set it after the insertion.
type fakeTraversalPlayer struct {
	order    []int        // queue indices in traversal order
	pos      int          // cursor into order; -1 when nothing is current
	missing  map[int]bool // queue indices whose load fails
	shuffled bool
	tracks   []player.QueueTrack // last queue SetQueue was given; nil until seeded

	loaded             []int // queue indices actually loaded, in order
	playFromIndexCalls []int
	// playFromIndexErrs is consumed one entry per PlayFromIndex call, so a test
	// can make two requests naming the same queue row resolve differently. A
	// nil entry, or an exhausted slice, means that call succeeds.
	playFromIndexErrs    []error
	setCurrentIndexCalls []int
	resumeCalls          int
	resumeErr            error
	replayCalls          int
	replayErr            error
	prefetched           int   // queue index mpv holds as the gapless successor; -1 when none
	prefetchLog          []int // every successor resolved, in order
}

// newFakeTraversalPlayer builds a player whose traversal visits order and whose
// cursor sits at position pos within it.
func newFakeTraversalPlayer(order []int, pos int) *fakeTraversalPlayer {
	f := &fakeTraversalPlayer{
		order:    append([]int(nil), order...),
		pos:      pos,
		missing:  map[int]bool{},
		shuffled: true,
	}
	f.prefetched = -1
	return f
}

// resolvePrefetch models the gapless successor the Manager appends to mpv's
// playlist after every load: the next row the traversal will visit, or none once
// the cycle has nothing left. Every operation that moves or re-asserts the
// cursor resolves it again, which is what makes a replay observably restore the
// same successor rather than a re-permuted one.
func (f *fakeTraversalPlayer) resolvePrefetch() {
	f.prefetched = -1
	for i := f.pos + 1; i < len(f.order); i++ {
		if f.missing[f.order[i]] {
			continue
		}
		f.prefetched = f.order[i]
		f.prefetchLog = append(f.prefetchLog, f.prefetched)
		return
	}
}

// finishCurrent models an end-of-file that promotes the prefetched successor.
// It reports the queue index that started, or false when nothing was prefetched
// — the exhaustion the TUI sees as PlaybackEndedMsg.
func (f *fakeTraversalPlayer) finishCurrent() (int, bool) {
	if f.prefetched < 0 {
		return -1, false
	}
	idx := f.prefetched
	for i := f.pos + 1; i < len(f.order); i++ {
		if f.order[i] == idx {
			f.pos = i
			break
		}
	}
	f.loaded = append(f.loaded, idx)
	f.resolvePrefetch()
	return idx, true
}

// ReplayCurrent restarts the row under the cursor without moving it: the order,
// the cursor, and therefore the successor are all exactly what they were. Tests
// assert that against PlayFromIndex, which reanchors.
func (f *fakeTraversalPlayer) ReplayCurrent() error {
	f.replayCalls++
	if f.replayErr != nil {
		return f.replayErr
	}
	if f.pos < 0 || f.pos >= len(f.order) {
		return errFakeNothingToReplay
	}
	f.loaded = append(f.loaded, f.order[f.pos])
	f.resolvePrefetch()
	return nil
}

func (f *fakeTraversalPlayer) Play() error  { return nil }
func (f *fakeTraversalPlayer) Pause() error { return nil }

// PlayFromIndex is the reanchoring selection: it moves the cursor onto idx
// wherever that sits in the traversal. Tests assert it is *not* called on the
// resume paths, because reanchoring is what replays a finished cycle.
func (f *fakeTraversalPlayer) PlayFromIndex(idx int) error {
	f.playFromIndexCalls = append(f.playFromIndexCalls, idx)
	if len(f.playFromIndexErrs) > 0 {
		err := f.playFromIndexErrs[0]
		f.playFromIndexErrs = f.playFromIndexErrs[1:]
		if err != nil {
			return err
		}
	}
	f.loaded = append(f.loaded, idx)
	for i, queueIdx := range f.order {
		if queueIdx == idx {
			f.pos = i
		}
	}
	f.resolvePrefetch()
	return nil
}

// AppendToQueue extends the traversal with the appended row, which lands in the
// unseen tail exactly as reconciliation puts it there. A seeded fake records the
// row too, so a later SetQueue does not read it as brand new.
func (f *fakeTraversalPlayer) AppendToQueue(track player.QueueTrack) {
	f.order = append(f.order, len(f.order))
	if f.tracks != nil {
		f.tracks = append(f.tracks, track)
	}
}

// SetQueue absorbs a rebuilt queue. A fake that has never been given a queue has
// no identities to compare against, so it can only reason positionally: it
// extends the traversal to cover rows added at the tail. Once seeded it
// reconciles by identity the way the Manager does, which is what makes a
// mid-queue insertion observable.
func (f *fakeTraversalPlayer) SetQueue(tracks []player.QueueTrack) {
	prev := f.tracks
	f.tracks = append([]player.QueueTrack(nil), tracks...)
	if prev == nil {
		for i := len(f.order); i < len(tracks); i++ {
			f.order = append(f.order, i)
		}
		return
	}
	f.reconcileByIdentity(prev, tracks)
}

// reconcileByIdentity rewrites the traversal order through the identity map
// between the previous queue and the new one: surviving rows keep their place in
// the cycle under their new queue index, rows that left drop out, and the cursor
// follows the row it named rather than the position it held. Brand-new rows
// become the next thing the traversal visits wherever they sit in the queue —
// the unseen pool with shuffle on, the rows recorded as unreached with shuffle
// off — which is the behaviour a resume after a mid-queue insertion depends on.
func (f *fakeTraversalPlayer) reconcileByIdentity(prev, next []player.QueueTrack) {
	newIndex := make(map[string]int, len(next))
	for i, track := range next {
		newIndex[fakeQueueIdentity(track)] = i
	}

	known := make(map[string]bool, len(prev))
	order := make([]int, 0, len(next))
	pos := -1
	for cyclePos, queueIdx := range f.order {
		if queueIdx < 0 || queueIdx >= len(prev) {
			continue
		}
		id := fakeQueueIdentity(prev[queueIdx])
		known[id] = true
		idx, live := newIndex[id]
		if !live {
			continue
		}
		order = append(order, idx)
		if cyclePos == f.pos {
			pos = len(order) - 1
		}
	}

	var additions []int
	for i, track := range next {
		if !known[fakeQueueIdentity(track)] {
			additions = append(additions, i)
		}
	}
	at := pos + 1
	merged := make([]int, 0, len(order)+len(additions))
	merged = append(merged, order[:at]...)
	merged = append(merged, additions...)
	merged = append(merged, order[at:]...)

	f.order = merged
	f.pos = pos
}

// fakeQueueIdentity names a queue row the way the Manager does: by queue ID when
// the row has one, by content otherwise. It drops the Manager's occurrence
// suffix because no test queues the same row twice.
func fakeQueueIdentity(track player.QueueTrack) string {
	if track.QueueID != 0 {
		return fmt.Sprintf("q:%d", track.QueueID)
	}
	return "f:" + track.Hash + "|" + track.Path
}

// SetCurrentIndex records the index the TUI pushed. The real Manager clamps a
// negative index to the head of the queue, which is why the TUI must not push
// one; recording it lets a test prove it does not.
func (f *fakeTraversalPlayer) SetCurrentIndex(index int) {
	f.setCurrentIndexCalls = append(f.setCurrentIndexCalls, index)
}

func (f *fakeTraversalPlayer) QueueTraversal() player.QueueTraversal {
	remaining := 0
	for i := f.pos + 1; i < len(f.order); i++ {
		if !f.missing[f.order[i]] {
			remaining++
		}
	}
	return player.QueueTraversal{
		Order:           append([]int(nil), f.order...),
		CurrentPosition: f.pos,
		Remaining:       remaining,
		Shuffled:        f.shuffled,
	}
}

// ResumeAfterQueueExtension continues the cycle: it walks forward from the
// cursor to the first row that is not missing, so rows already behind the cursor
// are never revisited and the index it returns is the row that actually played.
func (f *fakeTraversalPlayer) ResumeAfterQueueExtension() (int, error) {
	f.resumeCalls++
	if f.resumeErr != nil {
		return -1, f.resumeErr
	}
	for i := f.pos + 1; i < len(f.order); i++ {
		if f.missing[f.order[i]] {
			continue
		}
		f.pos = i
		f.loaded = append(f.loaded, f.order[i])
		f.resolvePrefetch()
		return f.order[i], nil
	}
	return -1, errFakeTraversalExhausted
}
