package tui

import (
	"errors"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/metadata"
	"github.com/sglavoie/muse/internal/tui/queue"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// These two tests cover the freshness half of the Apply gesture: a refresh
// judges its selections against tags and metadata copied when it started, so a
// change to either while it is in flight retires the answer it was about to
// commit.
//
// The barrier is what makes them a test of that and not of timing. Both drive a
// real update-loop handler at the one instant the finding describes — after the
// preparation has written its hidden rows, before the durable transaction — and
// then check three things: the obsolete pick never reached the database, the
// rows it had already written were retired, and the replan that followed
// produced a batch the change agrees with.

// djFreshnessToken stands in for the Tune-panel commit the Apply came from, so
// the tests can tell an answered wait from one still outstanding.
const djFreshnessToken uint64 = 1

// djFreshnessQueue is the fixture both tests plan against: a played row, a
// manual row playing, a forward-history row a Previous left unreplayed, and two
// DJ-owned slots the refresh may take over.
func djFreshnessQueue() []queue.Entry {
	return []queue.Entry{
		djRefreshRow(1, "d2", false),
		djRefreshRow(2, "f1", false),
		djRefreshRow(3, "d3", true),
		djRefreshRow(4, "f3", true),
		djRefreshRow(5, "d1", true),
	}
}

// djFreshnessModel puts the listener on the manual row, which leaves row 1
// played behind it and rows 3 to 5 ahead — of which only the last two are the
// cycle's to give up.
func djFreshnessModel(writer *stubDJReplacementWriter) (Model, *refreshPlayerStub) {
	m, p := djRefreshModel(djFreshnessQueue(), writer)
	p.current = 1
	p.forward[3] = true
	m.playback.currentIndex = 1
	m.playback.currentHash = "f1"
	return m, p
}

// djRefreshAcrossChange applies settings and lets the refresh run until it is
// holding a prepared plan, runs change as the update loop would, and only then
// lets the commit proceed.
//
// It returns the model the change left behind, the interrupted result, and
// whatever the completion handler asked to do about it.
func djRefreshAcrossChange(
	t *testing.T, m Model, p *refreshPlayerStub, settings radio.Settings, change func(Model) Model,
) (Model, djRefreshDoneMsg, tea.Cmd) {
	t.Helper()
	m, cmd, err := m.djApply(settings, djFixedNow)
	if err != nil {
		t.Fatalf("apply rejected valid settings: %v", err)
	}
	if cmd == nil {
		t.Fatal("apply launched no refresh")
	}
	// The Apply came from the Tune panel, which is waiting to hear how it went.
	// commitAutoDJDraft mints the token here, after the gesture, so that the
	// withdrawal the Apply itself performed cannot answer it.
	m.autoDJPanel = autoDJPanelState{token: djFreshnessToken, nextToken: djFreshnessToken}

	live, fired := m, false
	p.beforeCommit = func(int, *refreshPlayerStub) {
		// Only the first refresh is interrupted: the relaunch has to be able to
		// finish, or the test would prove nothing about replanning.
		if fired {
			return
		}
		fired = true
		live = change(live)
	}

	raw := cmd()
	msg, ok := raw.(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("apply produced %T, want a refresh result", raw)
	}
	if !fired {
		t.Fatal("the refresh never reached its commit, so nothing was interrupted")
	}

	live, next := live.handleDJRefreshDoneMsg(msg)
	return live, msg, next
}

// djPreparedHashes returns the hashes of one prepared batch, in the order the
// plan chose them.
func djPreparedHashes(entries []db.DJQueueEntry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Track.Hash
	}
	return out
}

// djEntryHashes returns the hashes of the successors an attempt committed.
func djEntryHashes(entries []queue.Entry) []string {
	out := make([]string, len(entries))
	for i, e := range entries {
		out[i] = e.Hash
	}
	return out
}

// containsHash reports whether a hash appears in a list.
func containsHash(hashes []string, want string) bool {
	for _, h := range hashes {
		if h == want {
			return true
		}
	}
	return false
}

// assertWithdrawnBeforeCommit checks the half of the contract both tests share:
// the interrupted attempt committed nothing, said why, retired the hidden rows
// it had written, and asked to be replanned.
func assertWithdrawnBeforeCommit(
	t *testing.T, writer *stubDJReplacementWriter, msg djRefreshDoneMsg, next tea.Cmd, obsolete string,
) {
	t.Helper()
	if msg.valid {
		t.Fatal("the refresh committed a batch chosen from data that had already moved")
	}
	if !errors.Is(msg.err, errDJRefreshInputsChanged) {
		t.Fatalf("refresh failed with %v, want it withdrawn as %v", msg.err, errDJRefreshInputsChanged)
	}
	if errors.Is(msg.err, errDJRefreshCancelled) {
		t.Fatal("a library change reported as a cancellation; nothing would replan")
	}
	if len(writer.prepared) == 0 {
		t.Fatal("the refresh prepared nothing, so it never reached the window under test")
	}
	if got := djPreparedHashes(writer.prepared[0]); !containsHash(got, obsolete) {
		t.Fatalf("the first plan picked %v, which does not include the track the change retires (%q)",
			got, obsolete)
	}
	if len(writer.accepted) != 0 {
		t.Fatalf("accepted %d batches before the change was accounted for, want none", len(writer.accepted))
	}
	if len(msg.discarded) != 1 {
		t.Fatalf("the result carries %d abandoned preparations, want the one it wrote", len(msg.discarded))
	}
	if len(writer.rejected) != 1 || !sameIDs(writer.rejected[0].IDs, msg.discarded[0].IDs) {
		t.Fatalf("hidden rows %v were left behind; rejected %v", msg.discarded[0].IDs, writer.rejected)
	}
	if next == nil {
		t.Fatal("a withdrawn refresh asked for no replan, so the applied settings never reach the queue")
	}
}

// assertPanelStillWaiting checks the Tune panel was not told the Apply was over
// while its replan is still in flight.
func assertPanelStillWaiting(t *testing.T, m Model) {
	t.Helper()
	if !m.djRefresh.pending {
		t.Fatal("the model does not consider a refresh in flight after asking for one")
	}
	if m.autoDJPanel.token != djFreshnessToken {
		t.Error("the panel's commit was answered by a refresh that withdrew without planning anything")
	}
}

// assertReplanSucceeded runs the relaunched refresh and checks it committed a
// batch the change agrees with, leaving the rows a refresh may not touch alone.
func assertReplanSucceeded(
	t *testing.T, m Model, writer *stubDJReplacementWriter, next tea.Cmd, obsolete string,
) Model {
	t.Helper()
	raw := next()
	msg, ok := raw.(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("the replan produced %T, want a refresh result", raw)
	}
	if !msg.valid {
		t.Fatalf("the replan committed nothing: err = %v", msg.err)
	}
	if len(writer.accepted) != 1 {
		t.Fatalf("accepted %d batches, want exactly the replanned one", len(writer.accepted))
	}
	hashes := djEntryHashes(msg.committed.entries)
	if len(hashes) == 0 {
		t.Fatal("the replan committed an empty batch, so the change left nothing playable")
	}
	if containsHash(hashes, obsolete) {
		t.Fatalf("the replan committed %v, which still includes the track the change ruled out (%q)",
			hashes, obsolete)
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)
	if m.djRefresh.pending {
		t.Error("a committed refresh is still reported as in flight")
	}
	// Row 1 is played, row 2 is the manual row playing, row 3 is forward
	// history: none of the three is the refresh's to move.
	ids := queueIDs(m)
	if len(ids) < 3 || ids[0] != 1 || ids[1] != 2 || ids[2] != 3 {
		t.Fatalf("visible queue is %v, want the played, playing, and forward-history rows first", ids)
	}
	if m.playback.currentIndex != 1 {
		t.Errorf("current index moved to %d; the playing row must keep its place", m.playback.currentIndex)
	}
	if m.autoDJPanel.token != 0 {
		t.Error("the panel is still waiting on an Apply whose replan has committed")
	}
	return m
}

// TestAutoDJRefreshRejectsChangedTagEligibility pins the tag half of the
// finding: an Include-by-tag refresh picks a track, the user unassigns that tag
// before the commit, and the pick must not survive it.
func TestAutoDJRefreshRejectsChangedTagEligibility(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djFreshnessModel(writer)
	m.tagWriter = &stubTagWriter{}
	m = m.rebuildTagCache([]db.TagRow{{ID: 7, Name: "chill"}})
	m = m.applyTagToggle(7, []string{"f2", "d1"}, true)

	settings := radio.DefaultSettings()
	settings.Include = radio.FilterSet{TagIDs: []int64{7}}

	m, msg, next := djRefreshAcrossChange(t, m, p, settings, func(live Model) Model {
		// The picker's own handler, so the toggle reaches the DJ exactly the way
		// a keypress would rather than through a shortcut this test invented.
		next, _ := live.handleTagPickerToggle(tagtui.TagPickerToggleMsg{
			TagID: 7, Hashes: []string{"d1"}, Add: false,
		})
		return next
	})

	assertWithdrawnBeforeCommit(t, writer, msg, next, "d1")
	assertPanelStillWaiting(t, m)
	m = assertReplanSucceeded(t, m, writer, next, "d1")

	if _, tagged := m.trackTags["d1"][7]; tagged {
		t.Error("the toggle the barrier ran was rolled back by the refresh")
	}
}

// TestAutoDJRefreshRejectsChangedGenreEligibility pins the metadata half: an
// Include-by-genre refresh picks a track whose genre is edited out of the filter
// before the commit lands.
func TestAutoDJRefreshRejectsChangedGenreEligibility(t *testing.T) {
	writer := newDJReplacementWriter()
	m, p := djFreshnessModel(writer)

	settings := radio.DefaultSettings()
	settings.Include = radio.FilterSet{Genres: []string{"Rock"}}

	m, msg, next := djRefreshAcrossChange(t, m, p, settings, func(live Model) Model {
		// The real save path, and then the message it produces: the index is
		// rewritten by the first and the DJ is told by the second, which is the
		// same two-step the runtime performs.
		ed := metadata.NewEditor("d1", "D1", "Dana", "Delta", "Jazz", 230)
		live.overlay.metadataEditor = &ed
		saved, _ := live.handleMetadataSave()
		edited, _ := saved.(Model).Update(metadataEditedMsg{})
		return edited.(Model)
	})

	assertWithdrawnBeforeCommit(t, writer, msg, next, "d1")
	assertPanelStillWaiting(t, m)
	m = assertReplanSucceeded(t, m, writer, next, "d1")

	if tr, ok := m.library.Index.FindByHash("d1"); !ok || tr.Genre != "Jazz" {
		t.Errorf("the genre edit the barrier ran did not survive the refresh: %+v", tr)
	}
}

// TestAutoDJRefreshKeepsACommitThatAlreadyLanded is the other side of the same
// boundary. A change arriving after the transaction cannot withdraw it: the rows
// are durable and visible, so discarding the result here would leave the
// database holding a batch the queue does not show. The change is answered by
// the refill's own freshness check instead, which is what the advanced data
// revision is for.
func TestAutoDJRefreshKeepsACommitThatAlreadyLanded(t *testing.T) {
	writer := newDJReplacementWriter()
	m, _ := djFreshnessModel(writer)

	settings := radio.DefaultSettings()
	settings.Include = radio.FilterSet{Genres: []string{"Rock"}}

	m, cmd, err := m.djApply(settings, djFixedNow)
	if err != nil {
		t.Fatalf("apply rejected valid settings: %v", err)
	}
	raw := cmd()
	msg, ok := raw.(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("apply produced %T, want a refresh result", raw)
	}
	if !msg.valid {
		t.Fatalf("the refresh committed nothing: err = %v", msg.err)
	}

	// The edit lands after the transaction and before the result is filed.
	before := m.djDataRevision()
	ed := metadata.NewEditor("d1", "D1", "Dana", "Delta", "Jazz", 230)
	m.overlay.metadataEditor = &ed
	saved, _ := m.handleMetadataSave()
	edited, _ := saved.(Model).Update(metadataEditedMsg{})
	m = edited.(Model)
	if m.djDataRevision() == before {
		t.Fatal("the edit did not advance the data revision, so nothing was invalidated")
	}

	m, _ = m.handleDJRefreshDoneMsg(msg)

	if len(writer.rejected) != 0 {
		t.Errorf("a committed batch was rejected after the fact: %v", writer.rejected)
	}
	for _, e := range msg.committed.entries {
		if !containsHash(djEntryHashes(m.queue.Entries), e.Hash) {
			t.Errorf("committed successor %q is durable but missing from the visible queue %v",
				e.Hash, djEntryHashes(m.queue.Entries))
		}
	}
	if m.djRefresh.pending {
		t.Error("a landed commit relaunched a refresh; the refill answers the change instead")
	}
}
