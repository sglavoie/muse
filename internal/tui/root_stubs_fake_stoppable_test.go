package tui

// fakeStoppablePlayer is fakeTraversalPlayer plus commandPlayer, which is what
// the teardown paths need: Stop and the queue clear only touch the player when
// it accepts commands, so a traversal-owning fake that refuses them cannot be
// used to prove a stop supersedes a selection. The base fake stays
// non-commandPlayer on purpose — the replay tests rely on resetPlaybackState
// finding no Stop to call — so this wraps it rather than widening it.
//
// stopped is the audible state: Stop sets it, and any load clears it again, so
// a test can assert that a superseded request did not restart playback without
// having to reason about call counts.
type fakeStoppablePlayer struct {
	*fakeTraversalPlayer
	stopped   bool
	paused    bool
	stopCalls int
	stopErr   error
	// calls interleaves the stops with the loads, so a test can assert which of
	// the two the player was told last.
	calls []string
}

func newFakeStoppablePlayer(order []int, pos int) *fakeStoppablePlayer {
	return &fakeStoppablePlayer{fakeTraversalPlayer: newFakeTraversalPlayer(order, pos)}
}

func (f *fakeStoppablePlayer) Stop() error {
	f.calls = append(f.calls, "Stop")
	f.stopCalls++
	if f.stopErr != nil {
		return f.stopErr
	}
	f.stopped = true
	f.prefetched = -1
	return nil
}

func (f *fakeStoppablePlayer) Play() error {
	f.calls = append(f.calls, "Play")
	f.paused = false
	return nil
}

func (f *fakeStoppablePlayer) Pause() error {
	f.calls = append(f.calls, "Pause")
	f.paused = true
	return nil
}

func (f *fakeStoppablePlayer) PlayFromIndex(idx int) error {
	f.calls = append(f.calls, "PlayFromIndex")
	err := f.fakeTraversalPlayer.PlayFromIndex(idx)
	if err == nil {
		f.stopped = false
	}
	return err
}

func (f *fakeStoppablePlayer) ReplayCurrent() error {
	f.calls = append(f.calls, "ReplayCurrent")
	err := f.fakeTraversalPlayer.ReplayCurrent()
	if err == nil {
		f.stopped = false
	}
	return err
}

func (f *fakeStoppablePlayer) ResumeAfterQueueExtension() (int, error) {
	f.calls = append(f.calls, "ResumeAfterQueueExtension")
	idx, err := f.fakeTraversalPlayer.ResumeAfterQueueExtension()
	if err == nil {
		f.stopped = false
	}
	return idx, err
}

// The seek and volume calls stay inert; transport movement advances the fake
// traversal so ordering tests can observe which physical row received it.
func (f *fakeStoppablePlayer) SeekAbsolute(float64) error      { return nil }
func (f *fakeStoppablePlayer) SeekRelative(float64) error      { return nil }
func (f *fakeStoppablePlayer) SetVolumeRelative(float64) error { return nil }
func (f *fakeStoppablePlayer) Next() error {
	f.calls = append(f.calls, "Next")
	if f.pos+1 < len(f.order) {
		f.pos++
		f.loaded = append(f.loaded, f.order[f.pos])
		f.resolvePrefetch()
	}
	return nil
}

func (f *fakeStoppablePlayer) Previous() error {
	f.calls = append(f.calls, "Previous")
	if f.pos > 0 {
		f.pos--
		f.loaded = append(f.loaded, f.order[f.pos])
		f.resolvePrefetch()
	}
	return nil
}
