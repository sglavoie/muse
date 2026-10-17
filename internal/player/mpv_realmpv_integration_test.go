//go:build integration

package player

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

// Acceptance checks against a real mpv process.
//
// Everything else in this package tests the Manager against a fake mpv, which is
// what makes those tests fast and deterministic. What a fake cannot answer is
// whether mpv itself behaves the way the queue refresh and the gapless successor
// assume: that a file appended to its playlist actually starts when the current
// one ends, that replacing the upcoming rows mid-track does not interrupt the
// track playing, and that an advance carries on without the process going idle.
// Those are the two claims the Auto-DJ rests on — audible continuity across an
// Apply, and gapless advance into the batch it published — so they are checked
// here against the real binary.
//
// Excluded from `just test` by the integration build tag, and skipped when mpv is
// not on PATH. Audio output is routed to the null device: these tests assert that
// decoding advances in real time and that no restart or gap occurs, which is
// everything about "audible" that a test can observe without a microphone.
//
// Run with: go test -tags integration -run RealMPV ./internal/player/

// realMPVClipSecs is the length of each generated clip. Long enough that an
// advance is a real transition rather than a rounding artifact, short enough that
// a five-clip queue plays out inside a test timeout.
const realMPVClipSecs = 1.2

// requireMPV skips when the binary under test is not installed.
func requireMPV(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("mpv"); err != nil {
		if os.Getenv("MUSE_REQUIRE_MPV") == "1" {
			t.Fatal("mpv is required for this run: " + err.Error())
		}
		t.Skip("mpv not found on PATH: " + err.Error())
	}
}

// writeToneWAV writes a mono 16-bit PCM sine tone. Real audio rather than
// silence, so mpv has to decode something and the elapsed position it reports
// describes actual playback.
//
// The sample count is unsigned from the moment it is computed and stays that
// way into the header fields, which is both what a WAV stores and the only way
// to write this without an integer conversion that has to be justified.
func writeToneWAV(t *testing.T, path string, seconds, hz float64) {
	t.Helper()
	const rate = 22050
	samples := uint32(seconds * rate)
	if samples == 0 {
		t.Fatalf("a %.2fs clip rounds to no samples", seconds)
	}
	dataLen := samples * 2

	data := make([]byte, 0, dataLen)
	for i := range samples {
		v := math.Sin(2 * math.Pi * hz * float64(i) / rate)
		// binary.Append rather than AppendUint16 so the signed sample never
		// passes through an integer conversion: PCM16 is two's complement, and
		// writing it as an int16 says that directly.
		next, err := binary.Append(data, binary.LittleEndian, int16(v*8000))
		if err != nil {
			t.Fatalf("encode sample %d: %v", i, err)
		}
		data = next
	}

	buf := make([]byte, 44, 44+len(data))
	copy(buf[0:], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:], 36+dataLen)
	copy(buf[8:], "WAVE")
	copy(buf[12:], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:], 16)
	binary.LittleEndian.PutUint16(buf[20:], 1) // PCM
	binary.LittleEndian.PutUint16(buf[22:], 1) // mono
	binary.LittleEndian.PutUint32(buf[24:], rate)
	binary.LittleEndian.PutUint32(buf[28:], rate*2) // byte rate
	binary.LittleEndian.PutUint16(buf[32:], 2)      // block align
	binary.LittleEndian.PutUint16(buf[34:], 16)     // bits per sample
	copy(buf[36:], "data")
	binary.LittleEndian.PutUint32(buf[40:], dataLen)
	buf = append(buf, data...)

	if err := os.WriteFile(path, buf, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// shortSocketPath returns a unix socket path well inside the ~104 byte sun_path
// limit. t.TempDir() embeds the test name, which for these tests is long enough
// on macOS that mpv silently declines to create the socket and the manager waits
// out its connect timeout instead.
func shortSocketPath(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "muse")
	if err != nil {
		t.Fatalf("socket dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	path := filepath.Join(dir, "s")
	if len(path) > 100 {
		t.Skipf("temp dir yields a %d byte socket path, over the unix socket limit", len(path))
	}
	return path
}

// realMPVStart is one observed start-file, with the instant it was seen.
type realMPVStart struct {
	hash  string
	index int
	at    time.Time
}

// realMPVFixture is a live mpv process with a queue of generated clips and a
// record of everything its event reader reported.
type realMPVFixture struct {
	t      *testing.T
	mgr    *Manager
	tracks []QueueTrack
	dir    string

	mu     sync.Mutex
	starts []realMPVStart
	ended  int
	paused []bool
	missed []string
}

// newRealMPVFixture generates count clips, launches mpv against them, and waits
// for the IPC socket to answer.
func newRealMPVFixture(t *testing.T, count int, first float64) *realMPVFixture {
	t.Helper()
	requireMPV(t)

	dir := t.TempDir()
	f := &realMPVFixture{t: t, dir: dir}
	for i := range count {
		secs := realMPVClipSecs
		if i == 0 {
			secs = first
		}
		path := filepath.Join(dir, fmt.Sprintf("clip%02d.wav", i))
		writeToneWAV(t, path, secs, 220+110*float64(i))
		f.tracks = append(f.tracks, QueueTrack{
			Path:    path,
			Hash:    fmt.Sprintf("clip%02d", i),
			QueueID: int64(i + 1),
			Artist:  "Tone",
			Title:   fmt.Sprintf("Clip %02d", i),
		})
	}

	f.mgr = NewManager(shortSocketPath(t))
	f.mgr.SetExtraFlags([]string{"--ao=null", "--no-config"})
	f.mgr.SetEventCallbacks(EventCallbacks{
		OnTrackChanged: func(hash, _, _, _ string, index int, _ int64) {
			f.mu.Lock()
			f.starts = append(f.starts, realMPVStart{hash: hash, index: index, at: time.Now()})
			f.mu.Unlock()
		},
		OnPlaybackEnded: func() {
			f.mu.Lock()
			f.ended++
			f.mu.Unlock()
		},
		OnPauseChanged: func(paused bool) {
			f.mu.Lock()
			f.paused = append(f.paused, paused)
			f.mu.Unlock()
		},
		OnMissingFile: func(path string) {
			f.mu.Lock()
			f.missed = append(f.missed, path)
			f.mu.Unlock()
		},
	})
	if err := f.mgr.Start(); err != nil {
		t.Fatalf("launch mpv: %v", err)
	}
	t.Cleanup(func() { _ = f.mgr.Close() })

	f.waitForSocket()
	f.mgr.SetQueue(f.tracks)
	return f
}

// waitForSocket blocks until mpv answers a property read, which is the point
// from which commands are executed rather than queued.
func (f *realMPVFixture) waitForSocket() {
	f.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := f.mgr.DebugInfo(); err == nil {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	f.t.Fatal("mpv never answered on its IPC socket")
}

// startCount returns how many start-file events have been seen.
func (f *realMPVFixture) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.starts)
}

// snapshotStarts returns the starts seen so far, oldest first.
func (f *realMPVFixture) snapshotStarts() []realMPVStart {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]realMPVStart(nil), f.starts...)
}

// waitForStarts blocks until at least n start-file events have been seen.
func (f *realMPVFixture) waitForStarts(n int, within time.Duration) []realMPVStart {
	f.t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if f.startCount() >= n {
			return f.snapshotStarts()
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.t.Fatalf("only %d of %d tracks started within %s: %v",
		f.startCount(), n, within, f.playedHashes())
	return nil
}

// playedHashes is the start order, for failure messages.
func (f *realMPVFixture) playedHashes() []string {
	out := []string{}
	for _, s := range f.snapshotStarts() {
		out = append(out, s.hash)
	}
	return out
}

// waitForElapsed blocks until the current file has played at least secs.
func (f *realMPVFixture) waitForElapsed(secs float64, within time.Duration) float64 {
	f.t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		elapsed, _, err := f.mgr.ElapsedPosition()
		if err == nil && elapsed >= secs {
			return elapsed
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.t.Fatalf("playback never reached %.2fs within %s", secs, within)
	return 0
}

// TestRealMPVGaplessAdvanceThroughTheQueue is the continuity claim the Auto-DJ
// depends on: a published batch plays through, in order, with no user gesture
// between the tracks.
//
// The advance is also timed. A file that fails to decode reports its start-file
// and its end-file almost immediately, so a queue that "played" in a fraction of
// its own length would satisfy an order-only assertion while producing silence.
// The clock runs from the play command to the last start-file, measured against
// the total length of the clips before it: too short means the clips were skipped
// rather than played, too long means there was a silence between them. That is
// the strongest statement about audibility available without listening to it.
//
// Elapsed time is measured from the play command rather than between consecutive
// starts because a start-file is reported after the manager has resolved the new
// row, which lags the actual start by a fraction of a second — enough to deflate a
// single 1.2s interval, but not a sum anchored outside the event path.
//
// The bounds are wide on purpose. Measured on macOS, three 1.2s clips reach their
// last start about 0.2s per transition early: with gapless on, mpv opens the next
// file while the current one is still draining, which is the point of gapless and
// which makes start-file a slightly early signal rather than an exact one.
func TestRealMPVGaplessAdvanceThroughTheQueue(t *testing.T) {
	f := newRealMPVFixture(t, 3, realMPVClipSecs)
	if err := f.mgr.SetGapless(true); err != nil {
		t.Fatalf("enable gapless: %v", err)
	}
	playAt := time.Now()
	if err := f.mgr.PlayFromIndex(0); err != nil {
		t.Fatalf("start playback: %v", err)
	}

	starts := f.waitForStarts(len(f.tracks), 30*time.Second)

	for i, want := range f.tracks {
		if starts[i].hash != want.Hash {
			t.Fatalf("track %d was %q, want the queue's %q (order: %v)",
				i, starts[i].hash, want.Hash, f.playedHashes())
		}
		if starts[i].index != i {
			t.Errorf("track %q started as index %d, want %d", want.Hash, starts[i].index, i)
		}
	}
	played := float64(len(f.tracks)-1) * realMPVClipSecs
	elapsed := starts[len(starts)-1].at.Sub(playAt).Seconds()
	t.Logf("the last of %d tracks started %.2fs in, after %.2fs of audio",
		len(f.tracks), elapsed, played)
	if elapsed < 0.7*played {
		t.Errorf("the queue reached its last track in %.2fs but the clips before it run %.2fs: they were not played through",
			elapsed, played)
	}
	if elapsed > played+2.5 {
		t.Errorf("the queue reached its last track in %.2fs, %.2fs more than the audio before it: the advances were not gapless",
			elapsed, elapsed-played)
	}

	f.mu.Lock()
	missed, pauses := append([]string(nil), f.missed...), append([]bool(nil), f.paused...)
	f.mu.Unlock()
	if len(missed) != 0 {
		t.Errorf("mpv reported files it could not open: %v", missed)
	}
	for _, paused := range pauses {
		if paused {
			t.Error("playback paused itself somewhere in the advance")
		}
	}
}

// TestRealMPVRefreshDoesNotInterruptTheCurrentTrack is the Apply gesture against
// the real process: the upcoming rows are replaced while a track is playing, and
// the track playing must neither restart nor stop.
//
// A restart is the specific failure this guards. Replacing mpv's playlist is done
// with the same commands a fresh load uses, so an implementation that rebuilt the
// playlist wholesale would reload the current file — inaudible in a fake, and an
// unmistakable stutter through speakers. The elapsed position is therefore read
// across the refresh and required to have advanced monotonically, and the
// start-file count is required not to have moved.
func TestRealMPVRefreshDoesNotInterruptTheCurrentTrack(t *testing.T) {
	// A long first clip so the refresh happens comfortably mid-track.
	f := newRealMPVFixture(t, 5, 6)
	if err := f.mgr.SetGapless(true); err != nil {
		t.Fatalf("enable gapless: %v", err)
	}
	if err := f.mgr.PlayFromIndex(0); err != nil {
		t.Fatalf("start playback: %v", err)
	}
	f.waitForStarts(1, 15*time.Second)
	before := f.waitForElapsed(0.5, 15*time.Second)
	startsBefore := f.startCount()

	// The successors the DJ would have prepared: two fresh rows taking over the
	// upcoming slots.
	successors := []QueueTrack{}
	for i := range 2 {
		path := filepath.Join(f.dir, fmt.Sprintf("new%02d.wav", i))
		writeToneWAV(t, path, realMPVClipSecs, 660+110*float64(i))
		successors = append(successors, QueueTrack{
			Path:    path,
			Hash:    fmt.Sprintf("new%02d", i),
			QueueID: int64(100 + i),
			Artist:  "Tone",
			Title:   fmt.Sprintf("New %02d", i),
		})
	}

	byID := make(map[int64]QueueTrack, len(f.tracks))
	for _, tr := range f.tracks {
		byID[tr.QueueID] = tr
	}
	var committed QueueRefresh
	res, err := f.mgr.CommitQueueRefresh(context.Background(), RefreshCommit{
		Prepare: func(snap RefreshSnapshot) (QueueRefresh, error) {
			if !snap.HasCurrent() {
				return QueueRefresh{}, errors.New("nothing is playing")
			}
			if len(snap.Eligible) == 0 {
				return QueueRefresh{}, errors.New("no upcoming rows to replace")
			}
			// Keep everything that is not up for replacement, in queue order,
			// then the successors at the tail — the shape the accept transaction
			// produces.
			kept := []RefreshRow{snap.Current}
			kept = append(kept, snap.Played...)
			kept = append(kept, snap.Protected...)
			kept = append(kept, snap.Unscheduled...)
			sort.Slice(kept, func(i, j int) bool { return kept[i].Index < kept[j].Index })

			plan := QueueRefresh{Snapshot: snap}
			for _, row := range kept {
				tr, ok := byID[row.QueueID]
				if !ok {
					return QueueRefresh{}, fmt.Errorf("row %d is not one of ours", row.QueueID)
				}
				plan.Queue = append(plan.Queue, tr)
			}
			plan.Queue = append(plan.Queue, successors...)
			for i, row := range snap.Eligible {
				rep := RowReplacement{Replaced: row.Identity}
				if i < len(successors) {
					rep.Successor = QueueRowIdentity(successors[i].QueueID)
				}
				plan.Replacements = append(plan.Replacements, rep)
			}
			return plan, nil
		},
		Commit: func(plan QueueRefresh) error {
			committed = plan
			return nil
		},
	})
	if err != nil {
		t.Fatalf("the refresh was refused by a real player: %v", err)
	}
	if !res.Committed {
		t.Fatal("the refresh reported no commit")
	}
	if res.Result.CurrentIndex != 0 {
		t.Errorf("the playing row moved to index %d, want 0", res.Result.CurrentIndex)
	}
	if len(committed.Replacements) == 0 {
		t.Error("the committed plan replaced nothing")
	}

	after, _, err := f.mgr.ElapsedPosition()
	if err != nil {
		t.Fatalf("read the position after the refresh: %v", err)
	}
	if after < before {
		t.Errorf("the current track restarted: position went from %.2fs to %.2fs", before, after)
	}
	if got := f.startCount(); got != startsBefore {
		t.Errorf("%d start-file events fired during the refresh, want none: %v",
			got-startsBefore, f.playedHashes())
	}

	// The replacement has to be what actually plays next, or the refresh changed
	// the database and nothing else.
	starts := f.waitForStarts(startsBefore+1, 20*time.Second)
	next := starts[len(starts)-1]
	if next.hash != successors[0].Hash {
		t.Errorf("the track after the refresh was %q, want the successor %q (order: %v)",
			next.hash, successors[0].Hash, f.playedHashes())
	}
	if gap := next.at.Sub(starts[startsBefore-1].at).Seconds(); gap > 8 {
		t.Errorf("the advance into the refreshed queue took %.2fs, which is a stall", gap)
	}
}
