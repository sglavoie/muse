package tui

import (
	"slices"
	"testing"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
	tagtui "github.com/sglavoie/muse/internal/tui/tags"
)

// Merging a tag the running Auto-DJ actually references, over a real database.
//
// A merge is the one tag gesture that changes an identity rather than a label.
// Storage answers for it inside the merge transaction — dj_tag_refs is remapped
// alongside track_tags — but the running session is a second copy of the same
// references, and it is the copy every later whole-session write is built from.
// The two have to move together: a session left naming the merged-away tag plans
// against something the catalog no longer offers, and the next Apply, reseed, or
// late refresh writes that stale id straight back over the durable remap.
//
// These tests use the journey harness because that is the only place the whole
// chain is real: the writer's MergeTags, the tag reader the merge handler
// re-reads through, the dj_tag_refs rows, and a relaunch that restores from
// them.

// createTag stores a tag and returns its id.
func (j *djJourney) createTag(name string) int64 {
	j.t.Helper()
	id, err := j.w.CreateTag(name, "")
	if err != nil {
		j.t.Fatalf("create tag %q: %v", name, err)
	}
	return id
}

// loadTags refreshes every tag cache the Model keeps from the database, through
// the same handler a tag mutation refresh goes through.
//
// The per-track map matters as much as the tag list: a tag filter selects on it,
// and the caches do not survive a relaunch, so a restored session that is never
// told which tracks carry which tag plans against an empty library and would
// make the assertions after a restart vacuous.
func (j *djJourney) loadTags() {
	j.t.Helper()
	tags, err := j.d.ReadTags()
	if err != nil {
		j.t.Fatalf("read tags: %v", err)
	}
	trackTags := make(map[string]map[int64]struct{})
	for _, t := range tags {
		hashes, err := j.d.ReadTrackHashesByTag(t.ID)
		if err != nil {
			j.t.Fatalf("read tracks for tag %d: %v", t.ID, err)
		}
		for _, hash := range hashes {
			if trackTags[hash] == nil {
				trackTags[hash] = make(map[int64]struct{})
			}
			trackTags[hash][t.ID] = struct{}{}
		}
	}
	counts, err := j.d.ReadTagCounts()
	if err != nil {
		j.t.Fatalf("read tag counts: %v", err)
	}
	j.m = j.m.handleTagsRefreshed(tagsRefreshedMsg{tags: tags, trackTags: trackTags})
	j.m.tagsView.Tags = tags
	j.m.tagsView.TagCounts = counts
}

// tagTracks attaches a tag to tracks and re-reads the caches, the way the tag
// picker leaves the two.
//
// The counts come along because the Tune panel only offers tags something
// actually carries; a tag applied without them stays invisible to the picker.
func (j *djJourney) tagTracks(id int64, hashes ...string) {
	j.t.Helper()
	for _, hash := range hashes {
		if err := j.w.AddTrackTag(hash, id); err != nil {
			j.t.Fatalf("tag %s with %d: %v", hash, id, err)
		}
	}
	j.loadTags()
}

// tagRow returns the stored row for a tag id.
func (j *djJourney) tagRow(id int64) db.TagRow {
	j.t.Helper()
	tags, err := j.d.ReadTags()
	if err != nil {
		j.t.Fatalf("read tags: %v", err)
	}
	for _, t := range tags {
		if t.ID == id {
			return t
		}
	}
	j.t.Fatalf("no active tag with id %d", id)
	return db.TagRow{}
}

// startFiltered starts a session on the given settings, seeded from the playing
// track, exactly as the Tune panel's Start does.
func (j *djJourney) startFiltered(s radio.Settings) {
	j.t.Helper()
	m, err := j.m.djApplySettings(s)
	if err != nil {
		j.t.Fatalf("apply settings: %v", err)
	}
	j.m = m.radioSetEnabled(true)
	j.m, _ = j.m.djStartFor(djSeedCurrent, time.Now())
	if !j.m.DJActive() {
		j.t.Fatal("the session did not start")
	}
}

// merge drives the whole merge gesture: the picker's answer, the durable
// MergeTags the command performs, and the message the runtime delivers back.
func (j *djJourney) merge(src, dest int64) {
	j.t.Helper()
	m, cmd := j.m.handleTagMergePicked(tagtui.TagMergePickedMsg{
		Src: j.tagRow(src), Dest: j.tagRow(dest),
	})
	if cmd == nil {
		j.t.Fatal("the merge gesture produced no command")
	}
	msg, ok := cmd().(tagMergedMsg)
	if !ok {
		j.t.Fatal("the merge command did not produce a tagMergedMsg")
	}
	if msg.err != nil {
		j.t.Fatalf("MergeTags(%d, %d): %v", src, dest, msg.err)
	}
	next, _ := m.handleTagMerged(msg)
	j.m = next
}

// sessionTagRefs reads the durable tag references the active session holds in
// one role, ascending. This is the row the merge transaction remapped and that a
// later whole-session write rewrites from the Model.
func (j *djJourney) sessionTagRefs(role string) []int64 {
	j.t.Helper()
	return j.djTagRefs("session", 1, role)
}

// refill runs one live refill through the real planner and publishes its result.
func (j *djJourney) refill() {
	j.t.Helper()
	next, cmd := j.m.startRadioEnqueueCmd()
	j.m = next
	if cmd == nil {
		j.t.Fatalf("no refill was launched: remaining = %d, blocked = %v",
			next.radioRemainingCount(), next.DJPlanBlocked())
	}
	j.runBatch(cmd)
}

// TestAutoDJTagMergeUpdatesLiveReferences pins what a merge does to the session
// that is running while it happens.
//
// Every reference the session holds has to follow the merge: the inclusion, the
// exclusion, and the tag ids frozen into the captured seed. Filter intent is what
// must survive the move — an exclusion stays an exclusion — and a set that
// already named the destination collapses onto one entry rather than carrying it
// twice. Everything the merge is not about, from the discovery target to the
// unrelated tag on the other side of the filter, has to be left alone.
func TestAutoDJTagMergeUpdatesLiveReferences(t *testing.T) {
	t.Run("an inclusion, a seed, and a duplicate destination collapse onto the kept tag", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.createTag("Late Night"), j.createTag("Loud"), j.createTag("Mellow")
		j.loadTags()
		playing := j.m.playback.currentHash
		j.tagTracks(src, playing, "jt02")
		j.tagTracks(dest, "jt04")
		j.tagTracks(other, "jt06")

		s := radio.DefaultSettings()
		// The inclusion already names the destination, which is the case a naive
		// substitution turns into a duplicate the unique index refuses.
		s.Include = radio.FilterSet{TagIDs: []int64{src, dest}}
		s.Exclude = radio.FilterSet{TagIDs: []int64{other}}
		j.startFiltered(s)

		if got := j.m.DJSeed().TagIDs; !slices.Contains(got, src) {
			t.Fatalf("the captured seed holds %v, want the tag the merge is about (%d)", got, src)
		}
		before := j.m.DJSettings()

		j.merge(src, dest)

		applied := j.m.DJSettings()
		if got := applied.Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the inclusion is %v, want the single kept tag [%d]", got, dest)
		}
		if got := applied.Exclude.TagIDs; !slices.Equal(got, []int64{other}) {
			t.Errorf("the merge rewrote the unrelated exclusion to %v, want [%d]", got, other)
		}
		if got := j.m.DJSeed().TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the captured seed still references %v, want [%d]", got, dest)
		}

		// Nothing else about the session is a merge's business.
		if applied.DiscoveryPercent != before.DiscoveryPercent || applied.Anchor != before.Anchor {
			t.Errorf("the merge changed the discovery target or anchor: %+v, want %+v", applied, before)
		}
		if got := j.m.DJSeed().TrackHash; got != playing {
			t.Errorf("the merge moved the seed to %q, want the captured %q", got, playing)
		}
		if !j.m.DJActive() {
			t.Error("the merge stopped the session")
		}
	})

	t.Run("an exclusion follows the merge and stays an exclusion", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.createTag("Noisy"), j.createTag("Loud"), j.createTag("Mellow")
		j.loadTags()
		playing := j.m.playback.currentHash
		j.tagTracks(other, playing, "jt03")
		j.tagTracks(src, "jt05")
		j.tagTracks(dest, "jt07")

		s := radio.DefaultSettings()
		s.Include = radio.FilterSet{TagIDs: []int64{other}}
		s.Exclude = radio.FilterSet{TagIDs: []int64{src}}
		j.startFiltered(s)

		j.merge(src, dest)

		applied := j.m.DJSettings()
		if got := applied.Exclude.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the exclusion is %v, want the kept tag [%d]", got, dest)
		}
		if got := applied.Include.TagIDs; !slices.Equal(got, []int64{other}) {
			t.Errorf("the exclusion leaked into the inclusion: %v, want [%d]", got, other)
		}
	})

	t.Run("the next batch plans against the kept tag", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest := j.createTag("Late Night"), j.createTag("Loud")
		j.loadTags()
		playing := j.m.playback.currentHash
		j.tagTracks(src, playing)
		wanted := []string{"jt02", "jt04", "jt06", "jt08", "jt10", "jt12", "jt14"}
		j.tagTracks(dest, wanted...)

		s := radio.DefaultSettings()
		s.Include = radio.FilterSet{TagIDs: []int64{src}}
		j.startFiltered(s)

		j.merge(src, dest)

		// Without the live remap the session would be filtering on a soft-deleted
		// tag, and the refill would park with an unresolved-tag reason instead of
		// enqueueing anything.
		j.refill()

		if j.m.DJPlanBlocked() {
			t.Fatalf("the refill parked after the merge: %v", j.m.djPlanBlockReasons())
		}
		owned := j.djOwnedHashes()
		if len(owned) == 0 {
			t.Fatal("the batch after the merge published no DJ-owned rows")
		}
		eligible := append([]string{playing}, wanted...)
		for _, hash := range owned {
			if !slices.Contains(eligible, hash) {
				t.Errorf("the batch selected %q, which carries neither the merged tag nor the kept one", hash)
			}
		}
	})

	t.Run("an open Tune panel cannot commit the merged-away reference", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest := j.createTag("Late Night"), j.createTag("Loud")
		j.loadTags()
		playing := j.m.playback.currentHash
		j.tagTracks(src, playing, "jt02")
		j.tagTracks(dest, "jt04")

		s := radio.DefaultSettings()
		s.Include = radio.FilterSet{TagIDs: []int64{src}}
		j.startFiltered(s)

		// The confirmation is answered asynchronously, so the panel can be opened
		// between the gesture and the message that lands here.
		j.press("ctrl+a")
		j.merge(src, dest)

		panel := djPanel(t, j.m)
		if got := panel.Draft().Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the draft still references %v, so committing it would restore the merged-away id", got)
		}
		if got := panel.AppliedSettings().Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the panel's baseline is %v, want the settings now in force", got)
		}
		if panel.Dirty() {
			t.Error("the merge left the panel reporting an unapplied edit the user never made")
		}
		if got := panel.Unresolved(); len(got) != 0 {
			t.Errorf("the panel refuses its own draft over %v, so the Apply button is dead", got)
		}
	})
}

// TestAutoDJTagMergeSurvivesSubsequentPersist is the durability half.
//
// The merge transaction leaves dj_tag_refs correct. What can undo it is the
// Model: every whole-session write rebuilds those rows from the settings and the
// seed the session holds, so a stale reference in memory becomes a stale
// reference on disk the next time anything persists — and then survives a
// restart, which is when it starts blocking the refill.
func TestAutoDJTagMergeSurvivesSubsequentPersist(t *testing.T) {
	t.Run("a Follow reseed does not write the merged-away filter back", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest, other := j.createTag("Late Night"), j.createTag("Loud"), j.createTag("Mellow")
		j.loadTags()
		playing := j.m.playback.currentHash
		j.tagTracks(src, playing, "jt02")
		j.tagTracks(dest, "jt04")
		j.tagTracks(other, "jt06")

		s := radio.DefaultSettings()
		// Follow is what makes an ordinary track change a whole-session write.
		s.Anchor = radio.AnchorFollow
		s.Include = radio.FilterSet{TagIDs: []int64{src}}
		s.Exclude = radio.FilterSet{TagIDs: []int64{other}}
		j.startFiltered(s)

		j.merge(src, dest)

		if got := j.sessionTagRefs("include"); !slices.Equal(got, []int64{dest}) {
			t.Fatalf("the merge transaction left include refs %v, want [%d]", got, dest)
		}

		j.m = j.m.djNoteTrackChanged("jt04", time.Now())
		if got := j.m.DJSeed().TrackHash; got != "jt04" {
			t.Fatalf("the Follow session did not reseed: seed is %q", got)
		}
		if got := j.sessionTagRefs("include"); !slices.Equal(got, []int64{dest}) {
			t.Errorf("the reseed wrote %v back over the merge's remap, want [%d]", got, dest)
		}
		if got := j.sessionTagRefs("exclude"); !slices.Equal(got, []int64{other}) {
			t.Errorf("the reseed rewrote the unrelated exclusion to %v, want [%d]", got, other)
		}

		j.relaunch()
		j.loadTags()

		if err := j.m.DJRestoreErr(); err != nil {
			t.Fatalf("the restarted launch refused the saved session: %v", err)
		}
		restored := j.m.DJSettings()
		if got := restored.Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the restored inclusion is %v, want the kept tag [%d]", got, dest)
		}
		if got := restored.Exclude.TagIDs; !slices.Equal(got, []int64{other}) {
			t.Errorf("the restored exclusion is %v, want [%d]", got, other)
		}
		if err := restored.ResolveTags(j.m.djTagCatalog()); err != nil {
			t.Errorf("the restored session references a tag the catalog cannot resolve: %v", err)
		}
	})

	t.Run("a whole-session write keeps the remapped seed reference across a restart", func(t *testing.T) {
		j := newDJJourney(t)
		src, dest := j.createTag("Late Night"), j.createTag("Loud")
		j.loadTags()
		playing := j.m.playback.currentHash
		j.tagTracks(src, playing, "jt02")
		j.tagTracks(dest, "jt04", "jt06", "jt08", "jt10", "jt12", "jt14")

		s := radio.DefaultSettings()
		s.Include = radio.FilterSet{TagIDs: []int64{src}}
		j.startFiltered(s)
		if got := j.sessionTagRefs("seed"); !slices.Equal(got, []int64{src}) {
			t.Fatalf("the started session stored seed refs %v, want [%d]", got, src)
		}

		j.merge(src, dest)

		// Adopting a batch's rounding carry rewrites the whole session row, seed
		// references included, from whatever the Model is holding.
		j.m = j.m.djAdoptAllocation(radio.Allocation{CarryUnits: 20})
		if got := j.sessionTagRefs("seed"); !slices.Equal(got, []int64{dest}) {
			t.Errorf("the session write left seed refs %v, want the kept tag [%d]", got, dest)
		}
		if got := j.sessionTagRefs("include"); !slices.Equal(got, []int64{dest}) {
			t.Errorf("the session write left include refs %v, want [%d]", got, dest)
		}

		j.relaunch()
		j.loadTags()

		if err := j.m.DJRestoreErr(); err != nil {
			t.Fatalf("the restarted launch refused the saved session: %v", err)
		}
		if got := j.m.DJSeed().TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the restored seed references %v, want the kept tag [%d]", got, dest)
		}
		if got := j.m.DJSeed().TrackHash; got != playing {
			t.Errorf("the restored seed is %q, want the captured %q", got, playing)
		}
		if got := j.m.DJSettings().Include.TagIDs; !slices.Equal(got, []int64{dest}) {
			t.Errorf("the restored inclusion is %v, want [%d]", got, dest)
		}

		// The point of all of it: the restored session can still plan.
		j.refill()
		if j.m.DJPlanBlocked() {
			t.Errorf("the restored session parked the refill: %v", j.m.djPlanBlockReasons())
		}
	})
}
