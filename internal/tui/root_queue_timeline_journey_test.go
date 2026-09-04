package tui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
	"github.com/sglavoie/muse/internal/tui/styles"
)

// This file is the one test that walks a whole listening session across every
// piece the player-owned timeline touches, in the order a user meets them:
// playback and its gapless successor, restarting the current track, an
// end-of-file handover, an Auto-DJ refill triggered while the cycle drains,
// the resume that picks that refill up after the cycle ran dry, and the panel
// the user was looking at the whole time. Each of those has focused tests of
// its own; what only this one can catch is the seams between them — a stage
// that leaves the traversal, the queue, and the TUI's idea of the current row
// disagreeing works in isolation and breaks here.
//
// The stages below are methods rather than one long function so each stays
// readable and inside the complexity budget; they run in order against one
// carried Model, so they are not independent and must not be reordered.

// journeyOrder is the traversal the fake player walks. It starts in the middle
// of the queue, so a stage that silently falls back to persisted order is
// visible immediately.
var journeyOrder = []int{3, 0, 4, 1, 2}

// journeyQueueLen is how many library tracks the queue starts with. It leaves
// exactly one library track unqueued, which is what radio has to pick from.
const journeyQueueLen = 5

// journey carries the model and the player across the stages of one session.
type journey struct {
	t *testing.T
	m Model
	p *fakeTraversalPlayer
}

func newJourney(t *testing.T) *journey {
	t.Helper()
	stubFileExistsTrue(t)

	p := newFakeTraversalPlayer(journeyOrder, -1)
	m := newModelWithTracks()
	m.player = p
	m.batchEnqueuer = &stubBatchEnqueuer{}
	m.queue.Traversal = queueTraversalSource(p)
	m.queue.Entries = journeyEntries(sampleLibraryTracks()[:journeyQueueLen])
	m.queue.Width, m.queue.Height = 80, 14
	m.ActiveView = viewQueue
	m.followNowPlaying = true
	m.radio.enabled = true
	return &journey{t: t, m: m, p: p}
}

// journeyEntries builds queue rows over real library tracks, so radio scoring
// has something to match against and the rows the panel draws are identifiable.
func journeyEntries(tracks []library.Track) []queue.Entry {
	entries := make([]queue.Entry, len(tracks))
	for i, t := range tracks {
		entries[i] = queue.Entry{
			ID:      int64(i + 1),
			Hash:    t.Hash,
			Path:    t.Path,
			Artist:  t.Artist,
			Title:   t.Title,
			Album:   t.Album,
			Display: t.Artist + " - " + t.Title,
		}
	}
	return entries
}

// trackChanged feeds the player's own report of the row that started, which is
// what mpv's start-file event becomes, and returns the batched follow-up.
func (j *journey) trackChanged(idx int) tea.Cmd {
	e := j.m.queue.Entries[idx]
	m, cmd := j.m.handleTrackChangedMsg(playback.TrackChangedMsg{
		Hash: e.Hash, Index: idx, Artist: e.Artist, Title: e.Title,
	})
	j.m = m
	return cmd
}

// start plays the first row through Enter, the way the user does, and checks
// the player lined up the next row of the cycle as its gapless successor.
func (j *journey) start() {
	j.t.Helper()
	m, res := runSelect(j.t, j.m, 0) // display row 0 is queue row 3
	if res.Replay {
		j.t.Fatal("the first selection is not a replay: nothing was playing")
	}
	j.m = m.handlePlayFromIndexResultMsg(res)
	// mpv answers the load on the IPC socket and reports the file it started on
	// the event socket, in that order, so the selection is only fully started
	// once its own start-file lands. Every later report in this journey — the
	// EOF handovers below — is emitted by the same reader after this one, which
	// is what entitles them to move the current row off the selected entry.
	j.trackChanged(3)

	if !slices.Equal(j.p.playFromIndexCalls, []int{3}) {
		j.t.Fatalf("PlayFromIndex calls = %v, want [3]", j.p.playFromIndexCalls)
	}
	if j.m.playback.awaitingSelectionStart {
		j.t.Fatal("the player's start-file report must retire the selection barrier")
	}
	if j.m.playback.currentIndex != 3 || j.m.playback.state != playback.StatePlaying {
		j.t.Fatalf("after start: currentIndex = %d state = %v; want 3 playing",
			j.m.playback.currentIndex, j.m.playback.state)
	}
	if j.p.prefetched != 0 {
		j.t.Fatalf("prefetched = %d, want queue row 0 (the next row of the cycle)", j.p.prefetched)
	}
}

// replayCurrent presses Enter on the row already playing. The cycle must not
// move: same order, same cursor, and therefore the same successor still queued.
func (j *journey) replayCurrent() {
	j.t.Helper()
	upcomingBefore := upcoming(j.p)

	m, res := runSelect(j.t, j.m, 0)
	if !res.Replay {
		j.t.Fatal("selecting the playing row must be a replay")
	}
	j.m = m.handlePlayFromIndexResultMsg(res)

	if j.p.replayCalls != 1 {
		j.t.Fatalf("ReplayCurrent calls = %d, want 1", j.p.replayCalls)
	}
	if !slices.Equal(j.p.playFromIndexCalls, []int{3}) {
		j.t.Fatalf("PlayFromIndex calls = %v; a replay must not reanchor", j.p.playFromIndexCalls)
	}
	if !slices.Equal(upcoming(j.p), upcomingBefore) {
		j.t.Fatalf("upcoming after replay = %v, want %v unchanged", upcoming(j.p), upcomingBefore)
	}
	if j.p.prefetched != 0 {
		j.t.Fatalf("prefetched after replay = %d, want the same successor (0)", j.p.prefetched)
	}
}

// handoverAtEOF ends the track so mpv promotes the successor, and checks the
// TUI follows the player's report rather than guessing the next persisted row.
func (j *journey) handoverAtEOF() {
	j.t.Helper()
	started, ok := j.p.finishCurrent()
	if !ok || started != 0 {
		j.t.Fatalf("EOF started row %d (ok=%v), want queue row 0", started, ok)
	}
	j.trackChanged(started)

	if j.m.playback.currentIndex != 0 {
		j.t.Fatalf("after EOF: currentIndex = %d, want 0", j.m.playback.currentIndex)
	}
	if j.m.radio.enqueuePending {
		j.t.Fatalf("remaining = %d is above the low-water mark; no refill yet",
			j.m.radioRemainingCount())
	}
	// The panel drew it one row below its predecessor: playback walks the
	// timeline downwards even though the queue rows jumped 3 -> 0.
	if got := j.m.queue.FindEntryFilteredPosition(0); got != 1 {
		j.t.Errorf("now-playing display row = %d, want 1", got)
	}
}

// refillAtLowWater drains one more row so Auto-DJ scores a refill, and returns
// the batch without applying it: scoring is asynchronous, and holding the
// result is what lets the cycle run out underneath it.
func (j *journey) refillAtLowWater() RadioBatchEnqueuedMsg {
	j.t.Helper()
	started, _ := j.p.finishCurrent()
	cmd := j.trackChanged(started)

	if !j.m.radio.enqueuePending {
		j.t.Fatalf("remaining = %d at the low-water mark; expected a radio refill",
			j.m.radioRemainingCount())
	}
	msg, ok := findRadioBatch(cmd)
	if !ok {
		j.t.Fatal("the batched commands did not include a radio refill")
	}
	if len(msg.Entries) == 0 {
		j.t.Fatal("the radio batch is empty; nothing to resume into")
	}
	return msg
}

// findRadioBatch runs cmd and everything batched inside it, returning the radio
// refill message if one of them produced it. handleTrackChangedMsg batches the
// refill together with the session write, the pulse, and the desktop notify, so
// the refill can only be reached through the batch the real runtime would run.
func findRadioBatch(cmd tea.Cmd) (RadioBatchEnqueuedMsg, bool) {
	if cmd == nil {
		return RadioBatchEnqueuedMsg{}, false
	}
	switch msg := cmd().(type) {
	case RadioBatchEnqueuedMsg:
		return msg, true
	case tea.BatchMsg:
		for _, c := range msg {
			if found, ok := findRadioBatch(c); ok {
				return found, true
			}
		}
	}
	return RadioBatchEnqueuedMsg{}, false
}

// exhaust plays the cycle out and reports the exhaustion to the TUI.
func (j *journey) exhaust() {
	j.t.Helper()
	for {
		started, ok := j.p.finishCurrent()
		if !ok {
			break
		}
		j.trackChanged(started)
	}
	m, cmd := j.m.handlePlaybackEndedMsg()
	j.m = m
	if cmd != nil {
		j.t.Error("a refill is already pending; the exhausted queue must not schedule another")
	}
	if !j.m.playback.queueExhausted || j.m.playback.state != playback.StateIdle {
		j.t.Fatalf("after the cycle ended: exhausted = %v state = %v; want true/idle",
			j.m.playback.queueExhausted, j.m.playback.state)
	}
}

// resumeInto applies the held refill onto the exhausted queue, which must
// continue the cycle into the new rows rather than reanchor onto a played one.
func (j *journey) resumeInto(msg RadioBatchEnqueuedMsg) {
	j.t.Helper()
	m, cmd := j.m.handleRadioBatchEnqueuedMsg(msg)
	j.m = m
	if cmd != nil {
		j.t.Error("a successful resume must not schedule another refill")
	}
	if j.p.resumeCalls != 1 {
		j.t.Fatalf("ResumeAfterQueueExtension calls = %d, want 1", j.p.resumeCalls)
	}
	if !slices.Equal(j.p.playFromIndexCalls, []int{3}) {
		j.t.Fatalf("PlayFromIndex calls = %v; the resume must not reanchor", j.p.playFromIndexCalls)
	}
	if j.m.playback.state != playback.StatePlaying || j.m.playback.queueExhausted {
		j.t.Fatalf("after resume: state = %v exhausted = %v; want playing and not exhausted",
			j.m.playback.state, j.m.playback.queueExhausted)
	}
	j.assertResumedRow(journeyQueueLen)
}

// assertResumedRow checks the resumed row is the appended one and that the
// cycle it continued visited every original row exactly once along the way.
func (j *journey) assertResumedRow(wantIdx int) {
	j.t.Helper()
	if j.m.playback.currentIndex != wantIdx {
		j.t.Fatalf("after resume: currentIndex = %d, want the appended row %d",
			j.m.playback.currentIndex, wantIdx)
	}
	if j.m.playback.currentHash != j.m.queue.Entries[wantIdx].Hash {
		j.t.Errorf("currentHash = %q, want the resumed row's hash %q",
			j.m.playback.currentHash, j.m.queue.Entries[wantIdx].Hash)
	}
	want := append(slices.Sorted(slices.Values(journeyOrder)), wantIdx)
	loaded := slices.Compact(slices.Sorted(slices.Values(j.p.loaded)))
	if !slices.Equal(loaded, want) {
		j.t.Errorf("rows loaded = %v, want every original row once plus the addition %v", loaded, want)
	}
}

// assertPanel checks the panel the user was looking at agrees with the player:
// it draws the traversal, extension included, and says so in the strip.
func (j *journey) assertPanel() {
	j.t.Helper()
	if got := displayRows(j.m); !slices.Equal(got, j.p.QueueTraversal().Order) {
		j.t.Errorf("display order = %v, want the traversal %v", got, j.p.QueueTraversal().Order)
	}
	out := j.m.queue.Render(styles.NewStyles(true), j.m.playback.currentIndex, j.m.playback.state, nil, nil, nil)
	if !strings.Contains(out, "shuffle order") {
		j.t.Error("the Queue strip must announce that rows are in shuffle order")
	}
	for _, e := range j.m.queue.Entries {
		if !strings.Contains(out, e.Title) {
			j.t.Errorf("rendered panel is missing queue row %q", e.Title)
		}
	}
}

// displayRows lists the queue rows in the order the panel draws them.
func displayRows(m Model) []int {
	filtered := m.queue.FilteredEntries()
	rows := make([]int, len(filtered))
	for i, e := range filtered {
		rows[i] = e.OrigIdx
	}
	return rows
}

func TestQueueTimelineEndToEndJourney(t *testing.T) {
	j := newJourney(t)
	j.start()         // 1. playback, with the next row prefetched
	j.replayCurrent() // 2. Enter on the playing row restarts it in place
	j.handoverAtEOF() // 3. the successor takes over at end of file
	refill := j.refillAtLowWater()
	j.exhaust()          // 4/5. the cycle runs dry while the refill is scoring
	j.resumeInto(refill) // 5. the refill resumes the cycle rather than reanchoring
	j.assertPanel()      // 6. the Queue panel matched the player throughout
}
