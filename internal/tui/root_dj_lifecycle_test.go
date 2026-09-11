package tui

import (
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// Library and tag lifecycle events while the Auto-DJ is running.
//
// TestDJInvalidationWalk already proves each of these events moves the data
// revision. What it cannot show is the consequence: a session is running, a batch
// is being planned from the library as it was, and then the user renames a tag,
// imports metadata, deletes files, or plays something. Each of those has to leave
// the session itself untouched — the settings, the filters, the seed, and the
// queue rows the user is listening to — while the batch in flight is either
// admitted or retired on its own merits.
//
// The distinction being pinned throughout is between the two, because they fail
// in opposite directions. A retirement that also disturbs the session loses the
// user's filters to an unrelated tag edit; a session that survives while a stale
// batch publishes hands the user tracks chosen against a library that no longer
// exists.

// djLifecycleTags is the catalog a filtered session references.
func djLifecycleTags() []db.TagRow {
	return []db.TagRow{{ID: 7, Name: "Late Night"}, {ID: 8, Name: "Loud"}}
}

// djLifecycleModel is an active session over the planning fixture with the tag
// catalog loaded and the Tags view populated, so the real tag gestures can be
// driven against it.
func djLifecycleModel(writer *stubDJQueueWriter) Model {
	m := djRefillModel(writer)
	m.ActiveView = viewTags
	m.tagsView.Level = tagtui.TagLevel
	m.tagsView.Tags = djLifecycleTags()
	m.tagsView.TagCounts = map[int64]int{7: 2, 8: 1}
	m.tagsView.Height = 40
	m = m.rebuildTagCache(m.tagsView.Tags)
	m.trackTags = map[string]map[int64]struct{}{
		"f1": {7: {}}, "f2": {7: {}}, "f3": {7: {}},
		"d1": {7: {}}, "d2": {7: {}}, "d3": {8: {}},
	}
	return m
}

// djTaggedSettings is a session filtered to one tag, which is what makes a tag
// mutation something the planner has to answer for rather than ignore.
func djTaggedSettings(id int64) radio.Settings {
	s := radio.DefaultSettings()
	s.Include.TagIDs = []int64{id}
	return s
}

// djNewTrack is a file the scan or the watcher has just found.
func djNewTrack() library.Track {
	return library.Track{
		Artist: "Gus", Album: "Eta", Title: "N1", Hash: "n1", Path: "/n1.flac",
		Genre: "Drone", Duration: 300,
	}
}

// TestDJLifecycleEventsRetireInFlightBatchesWithoutLosingTheSession walks the
// library and tag events a user can cause while a batch is being planned.
//
// Each one must retire that batch — it was selected against inputs that have
// since moved — and each one must leave the session exactly as it was, because
// none of them is a decision about the DJ. The relaunch is asserted too: these are
// precisely the moments a DJ that gave up would go silent with a queue at the
// low-water mark.
func TestDJLifecycleEventsRetireInFlightBatchesWithoutLosingTheSession(t *testing.T) {
	cases := []struct {
		name  string
		apply func(t *testing.T, m Model) Model
	}{
		{"a counted play", func(_ *testing.T, m Model) Model {
			// The real order: the index is bumped on the update loop when the
			// listen ends, and the revision moves when the write comes back.
			m.library.Index.IncrementPlayCount("d1", djFixedNow.Unix())
			return m.handlePlayRecordedMsg(playback.PlayRecordedMsg{
				Hash: "d1", PlayedAt: djFixedNow.Unix(), Counted: true,
			})
		}},
		{"files arriving from a scan", func(_ *testing.T, m Model) Model {
			out, _ := m.handleScanBatchMsg(library.ScanBatchMsg{Tracks: []library.Track{djNewTrack()}})
			return out
		}},
		{"files disappearing from disk", func(_ *testing.T, m Model) Model {
			out, _ := m.handleWatchBatchMsg(watchBatchMsg{deleted: []string{"/d3.flac"}})
			return out
		}},
		{"an applied metadata import", func(_ *testing.T, m Model) Model {
			return m.handleMetadataImportAppliedMsg(metadataImportAppliedMsg{applied: 2})
		}},
		{"an undone metadata import", func(_ *testing.T, m Model) Model {
			m.setUndoSlot(undoImportMetadataAction{
				originals: map[string]importOriginal{
					"f1": {title: "F1", artist: "Alice", album: "Alpha", genre: "Punk"},
				},
			}, nil)
			next, _ := m.Update(UndoMsg{})
			return next.(Model)
		}},
		{"a tag merge", func(_ *testing.T, m Model) Model {
			out, _ := m.handleTagMerged(tagMergedMsg{
				src:  db.TagRow{ID: 7, Name: "Late Night"},
				dest: db.TagRow{ID: 8, Name: "Loud"},
			})
			return out
		}},
		{"a tag deletion", func(t *testing.T, m Model) Model {
			out := djDeleteFocusedTag(t, m)
			return out
		}},
		{"an undone tag deletion", func(t *testing.T, m Model) Model {
			out := djDeleteFocusedTag(t, m)
			next, _ := out.Update(UndoMsg{})
			return next.(Model)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			writer := &stubDJQueueWriter{}
			m := djLifecycleModel(writer)
			settings, seed := m.dj.settings, m.dj.seed
			queueBefore := queueIDs(m)

			m, msg := runRefill(t, m)
			m = tc.apply(t, m)
			m, cmd := m.handleRadioBatchEnqueuedMsg(msg)

			if len(writer.accepted) != 0 {
				t.Errorf("a batch planned before %s was published anyway: %v", tc.name, writer.accepted)
			}
			if len(writer.rejected) != 1 {
				t.Errorf("rejected %d batches, want the one %s invalidated", len(writer.rejected), tc.name)
			}
			if got := queueIDs(m); !sameIDs(got, queueBefore) {
				t.Errorf("%s changed the queue to %v, want %v", tc.name, got, queueBefore)
			}
			if !m.dj.active {
				t.Errorf("%s stopped the session", tc.name)
			}
			if !m.dj.settings.Equal(settings) {
				t.Errorf("%s changed the applied settings to %+v", tc.name, m.dj.settings)
			}
			if m.dj.seed.TrackHash != seed.TrackHash {
				t.Errorf("%s moved the seed to %q", tc.name, m.dj.seed.TrackHash)
			}
			if cmd == nil {
				t.Errorf("%s left nothing scheduled, so the DJ would go silent at the low-water mark", tc.name)
			}
		})
	}
}

// djDeleteFocusedTag drives the real delete gesture on the focused tag, which is
// what rebuilds the catalog the planner resolves filters against.
func djDeleteFocusedTag(t *testing.T, m Model) Model {
	t.Helper()
	before := len(m.tags)
	out, _ := m.handleTagDelete()
	if len(out.tags) != before-1 {
		t.Fatalf("the delete gesture left %d tags, want %d", len(out.tags), before-1)
	}
	return out
}

// TestDJTagRenameDoesNotDisturbAFilteredSession is the counter-case to the walk
// above. A rename changes no track's membership and no tag's identity, so a
// filtered session must keep running and the batch in flight must still publish;
// retiring it would mean a cosmetic edit costs the user a batch. What the rename
// does have to reach is the label, because a filter the panel shows under the old
// name reads as a filter on a tag that no longer exists.
func TestDJTagRenameDoesNotDisturbAFilteredSession(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djLifecycleModel(writer)
	m.dj.settings = djTaggedSettings(7)
	revBefore := m.djDataRevision()

	m, msg := runRefill(t, m)
	m = m.handleTagRenamed(tagRenamedMsg{id: 7, name: "After Hours"})

	if got := m.djDataRevision(); got != revBefore {
		t.Errorf("a rename advanced the data revision to %d, retiring work for a cosmetic edit", got)
	}

	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.accepted) != 1 {
		t.Fatalf("the batch was refused over a rename: accepted=%v rejected=%v",
			writer.accepted, writer.rejected)
	}
	if got := m.dj.settings.Include.TagIDs; len(got) != 1 || got[0] != 7 {
		t.Errorf("the filter now references %v, want the same tag id it always did", got)
	}
	if got := m.tagNames[7]; got != "After Hours" {
		t.Errorf("the tag cache still calls tag 7 %q, so the panel would show the old name", got)
	}
}

// TestDJDeletedFilterTagParksTheRefillUntilItIsRestored is the unresolved-tag
// contract from the user's side.
//
// A filter is stored by tag id, so deleting the tag it names leaves a session
// asking for something that no longer exists. Quietly dropping the filter would
// hand the user a batch from the whole library while the panel still shows the
// filter; so the refill parks with the reason instead, and undoing the deletion
// has to bring it back on its own — the user should not have to restart the DJ to
// recover from an edit they immediately took back.
func TestDJDeletedFilterTagParksTheRefillUntilItIsRestored(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djLifecycleModel(writer)
	m.dj.settings = djTaggedSettings(7)

	// It plans while the tag exists.
	m, msg := runRefill(t, m)
	m, _ = m.handleRadioBatchEnqueuedMsg(msg)
	if len(writer.accepted) != 1 {
		t.Fatalf("a session filtered to an existing tag could not publish: rejected=%v", writer.rejected)
	}

	deleted := djDeleteFocusedTag(t, m)
	deleted.queue.Entries = deleted.queue.Entries[:1] // back to the low-water mark
	deleted, cmd := deleted.startRadioEnqueueCmd()
	if cmd == nil {
		t.Fatal("no refill was launched after the tag was deleted")
	}
	blocked, _ := deleted.handleRadioBatchEnqueuedMsg(cmd().(RadioBatchEnqueuedMsg))

	if len(writer.accepted) != 1 {
		t.Error("a batch was published while the session's filter named a deleted tag")
	}
	if !blocked.DJPlanBlocked() {
		t.Fatal("an unresolved filter left the refill unparked, so it would re-ask every tick")
	}
	if !hasReasonCode(blocked.djPlanBlockReasons(), radio.ReasonUnresolvedTag) {
		t.Errorf("the refill parked for %v, want an unresolved-tag explanation",
			blocked.djPlanBlockReasons())
	}
	if got := blocked.dj.settings.Include.TagIDs; len(got) != 1 || got[0] != 7 {
		t.Errorf("the deletion rewrote the session's filter to %v", got)
	}

	restored, _ := blocked.Update(UndoMsg{})
	next := restored.(Model)
	if !next.radioCanEnqueue() {
		t.Fatal("restoring the tag left the refill parked, so the DJ stays silent")
	}
	next, recovered := next.startRadioEnqueueCmd()
	if recovered == nil {
		t.Fatal("no refill was launched after the tag came back")
	}
	next, _ = next.handleRadioBatchEnqueuedMsg(recovered().(RadioBatchEnqueuedMsg))
	if len(writer.accepted) != 2 {
		t.Errorf("published %d batches, want the one from before the deletion and one after the undo",
			len(writer.accepted))
	}
	if next.DJPlanBlocked() {
		t.Error("a session that recovered is still parked")
	}
}

// hasReasonCode reports whether reasons carry the given code.
func hasReasonCode(reasons radio.Reasons, code radio.ReasonCode) bool {
	for _, r := range reasons {
		if r.Code == code {
			return true
		}
	}
	return false
}

// TestDJMissingFileIsNotSelectedAgain covers the deletion case end to end rather
// than only as an invalidation. The watcher removing a file has to take it out of
// the candidate set: a batch that named a path no longer on disk would be
// published, handed to mpv, and fail at playback time.
func TestDJMissingFileIsNotSelectedAgain(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djLifecycleModel(writer)

	m, cmd := m.handleWatchBatchMsg(watchBatchMsg{deleted: []string{"/d1.flac", "/d2.flac"}})
	_ = cmd
	if _, ok := m.library.Index.FindByHash("d1"); ok {
		t.Fatal("a deleted file is still in the library index")
	}

	m, msg := runRefill(t, m)
	m, _ = m.handleRadioBatchEnqueuedMsg(msg)

	if len(writer.accepted) != 1 {
		t.Fatalf("the refill after a deletion was refused: rejected=%v", writer.rejected)
	}
	for _, e := range m.queue.Entries[1:] {
		if e.Hash == "d1" || e.Hash == "d2" {
			t.Errorf("the batch selected %q, which is no longer on disk", e.Hash)
		}
	}

	// A new file is the mirror image: it has to become selectable without
	// restarting the session.
	m.queue.Entries = m.queue.Entries[:1]
	m, _ = m.handleScanBatchMsg(library.ScanBatchMsg{Tracks: []library.Track{djNewTrack()}})
	if _, ok := m.library.Index.FindByHash("n1"); !ok {
		t.Fatal("a scanned file never reached the library index")
	}
	in := m.captureDJInputs(djFixedNow)
	found := false
	for _, tr := range in.tracks {
		if tr.Hash == "n1" {
			found = true
		}
	}
	if !found {
		t.Error("a file found mid-session is not among the candidates the next batch plans from")
	}
}
