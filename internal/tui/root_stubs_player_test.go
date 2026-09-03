package tui

import (
	"errors"

	"github.com/sglavoie/muse/internal/player"
)

// errStubPlay is a sentinel error returned by stubPlayer.PlayFromIndex when
// playFromIndexErr is set, used in tests that exercise play-failure paths.
var errStubPlay = errors.New("stub play error")

// stubPlayer records Play, Pause, and PlayFromIndex calls for assertion in tests.
type stubPlayer struct {
	playCalls          int
	pauseCalls         int
	playFromIndexCalls []int
	playFromIndexErr   error // when non-nil, PlayFromIndex returns this error
}

func (s *stubPlayer) Play() error {
	s.playCalls++
	return nil
}

func (s *stubPlayer) Pause() error {
	s.pauseCalls++
	return nil
}

func (s *stubPlayer) PlayFromIndex(idx int) error {
	s.playFromIndexCalls = append(s.playFromIndexCalls, idx)
	return s.playFromIndexErr
}

// stubCommandPlayer extends stubPlayer with the commandPlayer interface so that
// tests can exercise the s/n/p/seek/volume key handlers.
type stubCommandPlayer struct {
	stubPlayer
	stopCalls int
}

func (s *stubCommandPlayer) Stop() error {
	s.stopCalls++
	return nil
}

func (s *stubCommandPlayer) SeekAbsolute(float64) error      { return nil }
func (s *stubCommandPlayer) SeekRelative(float64) error      { return nil }
func (s *stubCommandPlayer) SetVolumeRelative(float64) error { return nil }
func (s *stubCommandPlayer) Next() error                     { return nil }
func (s *stubCommandPlayer) Previous() error                 { return nil }

// stubElapsedReader extends stubPlayer with the elapsedReader interface for
// testing the tickMsg handler.
type stubElapsedReader struct {
	stubPlayer
	elapsed float64
	total   float64
	err     error
}

func (s *stubElapsedReader) ElapsedPosition() (float64, float64, error) {
	return s.elapsed, s.total, s.err
}

// stubVolumePlayer extends stubCommandPlayer to capture SetVolumeRelative calls.
type stubVolumePlayer struct {
	stubCommandPlayer
	volumeDelta float64
}

func (s *stubVolumePlayer) SetVolumeRelative(delta float64) error {
	s.volumeDelta = delta
	return nil
}

// stubSeekPlayer extends stubPlayer with tracking for SeekRelative, Next, and
// Previous calls so that seek and navigation key tests can assert correct deltas
// and call counts without a real mpv backend.
type stubSeekPlayer struct {
	stubPlayer
	seekDeltas []float64
	nextCalls  int
	prevCalls  int
}

func (s *stubSeekPlayer) Stop() error                { return nil }
func (s *stubSeekPlayer) SeekAbsolute(float64) error { return nil }
func (s *stubSeekPlayer) SeekRelative(d float64) error {
	s.seekDeltas = append(s.seekDeltas, d)
	return nil
}
func (s *stubSeekPlayer) SetVolumeRelative(float64) error { return nil }
func (s *stubSeekPlayer) Next() error                     { s.nextCalls++; return nil }
func (s *stubSeekPlayer) Previous() error                 { s.prevCalls++; return nil }

// stubPlayerQueueRemover extends stubCommandPlayer with playerQueueRemover so
// that DequeueTrackMsg handler tests can assert RemoveFromQueue is called with
// the correct index and that Stop is never invoked.
type stubPlayerQueueRemover struct {
	stubCommandPlayer
	removeFromQueueCalls []int
}

func (s *stubPlayerQueueRemover) RemoveFromQueue(index int) error {
	s.removeFromQueueCalls = append(s.removeFromQueueCalls, index)
	return nil
}

// stubPlayerQueueSyncer extends stubPlayer with playerQueueSetter,
// playerQueueRemover, and playerIndexSetter so undo tests can verify the
// player queue and current index are rebuilt after undoing a dequeue.
type stubPlayerQueueSyncer struct {
	stubPlayer
	setQueueCalls        [][]player.QueueTrack
	removeFromQueueCalls []int
	setCurrentIndexCalls []int
}

func (s *stubPlayerQueueSyncer) SetQueue(tracks []player.QueueTrack) {
	cp := make([]player.QueueTrack, len(tracks))
	copy(cp, tracks)
	s.setQueueCalls = append(s.setQueueCalls, cp)
}

func (s *stubPlayerQueueSyncer) RemoveFromQueue(index int) error {
	s.removeFromQueueCalls = append(s.removeFromQueueCalls, index)
	return nil
}

func (s *stubPlayerQueueSyncer) SetCurrentIndex(index int) {
	s.setCurrentIndexCalls = append(s.setCurrentIndexCalls, index)
}

// stubQueueLifecyclePlayer records the full clear-and-enqueue player sequence
// so integration-style root tests can assert both queue sync and playback.
type stubQueueLifecyclePlayer struct {
	stubCommandPlayer
	setQueueCalls      [][]player.QueueTrack
	appendToQueueCalls []player.QueueTrack
	calls              []string
}

func (s *stubQueueLifecyclePlayer) Stop() error {
	s.calls = append(s.calls, "Stop")
	return s.stubCommandPlayer.Stop()
}

func (s *stubQueueLifecyclePlayer) PlayFromIndex(idx int) error {
	s.calls = append(s.calls, "PlayFromIndex")
	return s.stubCommandPlayer.PlayFromIndex(idx)
}

func (s *stubQueueLifecyclePlayer) SetQueue(tracks []player.QueueTrack) {
	cp := make([]player.QueueTrack, len(tracks))
	copy(cp, tracks)
	s.setQueueCalls = append(s.setQueueCalls, cp)
	s.calls = append(s.calls, "SetQueue")
}

func (s *stubQueueLifecyclePlayer) AppendToQueue(track player.QueueTrack) {
	s.appendToQueueCalls = append(s.appendToQueueCalls, track)
	s.calls = append(s.calls, "AppendToQueue")
}

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
// It models tail growth only — appends and pastes at the end of the queue —
// which is what every natural queue extension does.
type fakeTraversalPlayer struct {
	order    []int        // queue indices in traversal order
	pos      int          // cursor into order; -1 when nothing is current
	missing  map[int]bool // queue indices whose load fails
	shuffled bool

	loaded               []int // queue indices actually loaded, in order
	playFromIndexCalls   []int
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
// unseen tail exactly as reconciliation puts it there.
func (f *fakeTraversalPlayer) AppendToQueue(player.QueueTrack) {
	f.order = append(f.order, len(f.order))
}

// SetQueue absorbs a rebuilt queue by extending the traversal to cover any rows
// added at the tail; the existing cycle is left intact, as identity-based
// reconciliation leaves it.
func (f *fakeTraversalPlayer) SetQueue(tracks []player.QueueTrack) {
	for i := len(f.order); i < len(tracks); i++ {
		f.order = append(f.order, i)
	}
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

// stubFailingVolumePlayer extends stubCommandPlayer to return errors from
// SetVolumeRelative, used to test volume-drift prevention.
type stubFailingVolumePlayer struct {
	stubCommandPlayer
	volumeErr error
}

func (s *stubFailingVolumePlayer) SetVolumeRelative(float64) error {
	return s.volumeErr
}

// stubFailingPlayer returns errors from Play() to test error paths.
type stubFailingPlayer struct {
	stubPlayer
	playErr error
}

func (s *stubFailingPlayer) Play() error {
	s.playCalls++
	return s.playErr
}
