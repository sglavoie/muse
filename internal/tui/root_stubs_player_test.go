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
