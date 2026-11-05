//go:build integration

package player

import (
	"math"
	"testing"
	"time"
)

// TestRealMPVLevelMeterReadsLevelsAndKeepsGapless turns the level meter on the
// way the interface does, once a song is playing, and checks the two things a
// fake mpv cannot: that this mpv's FFmpeg builds the astats filter and reports
// its measurements through af-metadata, and that a chain carrying the meter
// (beside normalization, which the meter must not displace) still advances
// through the queue gaplessly.
//
// The clips are sine tones at 8000/32768 of full scale, a peak of about
// -12.2 dBFS and an RMS of about -15.3 dBFS; normalization changes the level,
// so the reading is only required to be a plausible tone, not that exact one.
// The first clip is long because loudnorm holds back its three-second
// look-ahead window before it releases audio: a shorter first clip never
// reports a position at all.
func TestRealMPVLevelMeterReadsLevelsAndKeepsGapless(t *testing.T) {
	f := newRealMPVFixture(t, 3, 6)
	if err := f.mgr.SetGapless(true); err != nil {
		t.Fatalf("enable gapless: %v", err)
	}
	if err := f.mgr.SetNormalize(true); err != nil {
		t.Fatalf("enable normalize: %v", err)
	}
	playAt := time.Now()
	if err := f.mgr.PlayFromIndex(0); err != nil {
		t.Fatalf("start playback: %v", err)
	}
	f.waitForStarts(1, 15*time.Second)
	f.waitForElapsed(0.2, 15*time.Second)
	if err := f.mgr.SetLevelMeter(true); err != nil {
		t.Fatalf("enable level meter: %v", err)
	}

	var reading Levels
	deadline := time.Now().Add(1500 * time.Millisecond)
	for {
		levels, ok, err := f.mgr.Levels()
		if err != nil {
			t.Fatalf("read levels: %v", err)
		}
		if ok {
			reading = levels
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no level reading arrived within 1.5s of turning the meter on")
		}
		time.Sleep(40 * time.Millisecond)
	}
	if reading.Channels() < 1 {
		t.Fatalf("reading has no channels: %+v", reading)
	}
	peak, rms := reading.Peak[0], reading.RMS[0]
	t.Logf("level reading: %d channel(s), peak %.1f dBFS, RMS %.1f dBFS", reading.Channels(), peak, rms)
	if math.IsInf(peak, 0) || peak > 0.5 || peak < -60 || rms > peak+0.01 {
		t.Fatalf("implausible reading for a tone: peak %.2f, RMS %.2f", peak, rms)
	}

	m := f.mgr
	m.mu.RLock()
	filters := len(m.audioFilters)
	m.mu.RUnlock()
	if filters != 1 {
		t.Fatalf("normalization was displaced by the meter: %d filters", filters)
	}

	starts := f.waitForStarts(len(f.tracks), 30*time.Second)
	for i, want := range f.tracks {
		if starts[i].hash != want.Hash {
			t.Fatalf("track %d was %q, want %q (order: %v)", i, starts[i].hash, want.Hash, f.playedHashes())
		}
	}
	played := 6 + realMPVClipSecs
	elapsed := starts[len(starts)-1].at.Sub(playAt).Seconds()
	t.Logf("the last of %d tracks started %.2fs in, after %.2fs of audio", len(f.tracks), elapsed, played)
	if elapsed < 0.7*played || elapsed > played+2.5 {
		t.Errorf("with the meter on, the last track started %.2fs in for %.2fs of audio: not a gapless advance", elapsed, played)
	}
	if _, ok, err := f.mgr.Levels(); err != nil || !ok {
		t.Errorf("levels stopped arriving after the gapless advances: ok=%v err=%v", ok, err)
	}
	f.mu.Lock()
	missed := append([]string(nil), f.missed...)
	f.mu.Unlock()
	if len(missed) != 0 {
		t.Errorf("mpv reported files it could not open: %v", missed)
	}
}
