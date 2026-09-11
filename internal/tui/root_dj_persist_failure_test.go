package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/autodj"
)

// A DJ lifecycle write the database refuses is the one failure the Model cannot
// roll back: the session goes on playing the settings the user chose while the
// row on disk still describes the session before them. These tests hold that
// boundary over a real database file — the commit is reported rather than
// acknowledged as durable, the divergence is reconciled by the app's own tick
// rather than by whatever the user happens to do next, and an ordinary accepted
// batch in between is shown to be no help at all, because it writes the
// discovery carry and nothing else.

// errDJSessionRefused is the storage failure these tests inject.
var errDJSessionRefused = errors.New("database is locked")

// djRefusingWriter is the journey's session writer with a fail counter in front
// of it.
//
// It resolves the real writer at call time rather than holding it, because a
// relaunch closes the database and builds a new writer over the reopened file;
// a captured one would write into the previous launch.
type djRefusingWriter struct {
	j *djJourney

	// fails is how many further session writes the database refuses. Both
	// statements draw from it, so "fail once" means the next lifecycle write of
	// either kind is refused.
	fails int

	// refFails is how many further merged-tag reference repairs the database
	// refuses. It is counted apart from fails so a test can refuse the repair
	// while the ordinary session writes around it keep landing, which is the
	// shape the merge-ordering failure actually has.
	refFails int

	writes int
	stops  int
	remaps int
}

func (w *djRefusingWriter) WriteDJSession(session db.DJSession) error {
	w.writes++
	if w.fails > 0 {
		w.fails--
		return errDJSessionRefused
	}
	return w.j.w.WriteDJSession(session)
}

func (w *djRefusingWriter) StopDJSession() error {
	w.stops++
	if w.fails > 0 {
		w.fails--
		return errDJSessionRefused
	}
	return w.j.w.StopDJSession()
}

func (w *djRefusingWriter) RemapDJTagRefs(sourceID, destID int64) error {
	w.remaps++
	if w.refFails > 0 {
		w.refFails--
		return errDJSessionRefused
	}
	return w.j.w.RemapDJTagRefs(sourceID, destID)
}

// newRefusingJourney is a journey whose DJ session writes go through a writer
// the test can make the database refuse.
func newRefusingJourney(t *testing.T) (*djJourney, *djRefusingWriter) {
	t.Helper()
	j := newDJJourney(t)
	w := &djRefusingWriter{j: j}
	j.djWriter = w
	// Rebuilding from disk is how the wrapper gets in front of the writer the
	// Model holds; nothing has been written yet, so the launch is the same one
	// newDJJourney produced.
	j.relaunch()
	return j, w
}

// djStartSession turns the DJ on with the `A` key and returns the refill command
// the press produced.
func djStartSession(t *testing.T, j *djJourney) tea.Cmd {
	t.Helper()
	next, cmd, handled := j.m.radioToggle()
	if !handled {
		t.Fatal("A was not handled")
	}
	j.m = next.(Model)
	if !j.m.DJActive() {
		t.Fatal("A left no DJ session running")
	}
	return cmd
}

// djApplyExplore commits an Explore draft through the Tune panel and runs
// whatever the commit launched, so the panel has been answered on return.
func djApplyExplore(t *testing.T, j *djJourney) *autodj.TunePanel {
	t.Helper()
	j.press("ctrl+a")
	// The preset row is focused first, and one step right of Balanced is Explore.
	cmd := j.press("right", "ctrl+s")
	if got := j.m.DJSettings().Preset(); got != radio.PresetExplore {
		t.Fatalf("applied preset = %q, want the committed Explore draft", got)
	}
	j.deliver(cmd)
	panel := djPanel(t, j.m)
	if panel.Busy() {
		t.Fatal("the panel stayed busy after its commit was answered")
	}
	return panel
}

// assertUnsavedReported checks every surface that has to say the applied
// settings are not on disk.
func assertUnsavedReported(t *testing.T, j *djJourney, panel *autodj.TunePanel) {
	t.Helper()
	err := panel.Err()
	if err == nil {
		t.Fatal("the panel reported a successful Apply for a session the database refused")
	}
	if !errors.Is(err, errDJUnsaved) || !errors.Is(err, errDJSessionRefused) {
		t.Errorf("panel error = %v, want the refusal reported as unsaved", err)
	}
	if !panel.Dirty() {
		t.Error("the panel moved its applied baseline onto a draft that was never saved")
	}
	if got := j.m.statusBar.TopNotification().Msg; !strings.Contains(got, "not saved") {
		t.Errorf("status bar = %q, want a notice that the session was not saved", got)
	}
	if !djStatusMentions(j.m.autoDJPanelStatus(), "Not saved") {
		t.Errorf("panel status lines = %+v, want a standing unsaved line", j.m.autoDJPanelStatus())
	}
}

// djStatusMentions reports whether any panel status line contains want.
func djStatusMentions(lines []autodj.StatusLine, want string) bool {
	for _, line := range lines {
		if strings.Contains(line.Text, want) {
			return true
		}
	}
	return false
}

// TestAutoDJApplyReportsSessionWriteFailure: an Apply whose session write the
// database refused is in force for this run only. Acknowledging it as a
// successful durable Apply would clear the draft baseline and leave the user
// believing the filters on screen are the ones a restart will restore.
//
// Both routes to the acknowledgement are covered, because they answer from
// different places: a refresh that finds nothing to replace hands the outcome
// back through the refresh, and a refresh that replaces rows succeeds at
// precisely the thing that could make the failure look like success.
func TestAutoDJApplyReportsSessionWriteFailure(t *testing.T) {
	t.Run("when there are no rows to refresh", func(t *testing.T) {
		j, w := newRefusingJourney(t)
		// The refill the start returned is deliberately not run: the queue keeps
		// its one manual row, so the Apply's refresh finds no DJ-owned slot and
		// the commit is acknowledged without any durable queue work behind it.
		djStartSession(t, j)
		before := j.session()
		if before.Settings.Preset() != radio.PresetBalanced {
			t.Fatalf("the started session saved %q, want Balanced", before.Settings.Preset())
		}
		w.fails = 1

		panel := djApplyExplore(t, j)

		assertUnsavedReported(t, j, panel)
		if got := j.session().Settings; !got.Equal(before.Settings) {
			t.Errorf("saved settings = %+v, want the previous %+v after a refused write", got, before.Settings)
		}
		if got := j.m.DJSettings().Preset(); got != radio.PresetExplore {
			t.Errorf("running settings = %q, want the Explore the user applied", got)
		}
		if !j.m.DJActive() {
			t.Error("a refused write stopped the session")
		}
	})

	t.Run("when the refresh replaced the queue", func(t *testing.T) {
		j, w := newRefusingJourney(t)
		j.runBatch(djStartSession(t, j))
		before := j.session()
		w.fails = 1

		panel := djApplyExplore(t, j)

		if j.m.djRefresh.status.replaced == 0 {
			t.Fatal("the refresh replaced nothing, so this is the no-slots case again")
		}
		assertUnsavedReported(t, j, panel)
		if got := j.session().Settings; !got.Equal(before.Settings) {
			t.Errorf("saved settings = %+v, want the previous %+v: the refresh commits the carry, not the settings",
				got, before.Settings)
		}
	})
}

// TestAutoDJLifecyclePersistenceFailureRecovery walks the whole recovery: what a
// refused write leaves behind, what an ordinary accepted batch does and does not
// repair, that the app reconciles the row itself, that a restart then restores
// what the user applied, and that the reconciliation is bounded and says so when
// it gives up.
func TestAutoDJLifecyclePersistenceFailureRecovery(t *testing.T) {
	j, w := newRefusingJourney(t)
	djStartSession(t, j)
	started := j.session()

	t.Run("a refused Apply keeps playing and records itself unsaved", func(t *testing.T) {
		w.fails = 1
		panel := djApplyExplore(t, j)

		assertUnsavedReported(t, j, panel)
		if got := j.session().Settings; !got.Equal(started.Settings) {
			t.Fatalf("saved settings = %+v, want the previous %+v", got, started.Settings)
		}
		if !j.m.dj.unsaved.pending() {
			t.Fatal("the refused write left nothing to reconcile")
		}
	})

	t.Run("an accepted batch writes the carry and cannot save the settings", func(t *testing.T) {
		m, cmd := j.m.startRadioEnqueueCmd()
		j.m = m
		if cmd == nil {
			t.Fatal("the refill was not launched, so this proves nothing about accepts")
		}
		j.runBatch(cmd)

		if got := len(j.djOwnedHashes()); got != radioBatchSize {
			t.Fatalf("durable DJ-owned rows = %d, want the accepted batch of %d", got, radioBatchSize)
		}
		// The accept transaction landed, and it wrote Allocation only. Nothing in
		// the ordinary refill path rewrites the session row, so a successful batch
		// must not be mistaken for the settings having been saved.
		if got := j.session().Settings; !got.Equal(started.Settings) {
			t.Errorf("saved settings = %+v, want the still-stale %+v", got, started.Settings)
		}
		if !j.m.dj.unsaved.pending() {
			t.Error("an accepted batch cleared the unsaved marker it never reconciled")
		}
	})

	t.Run("the tick reconciles the row and a restart restores the applied settings", func(t *testing.T) {
		applied := j.m.DJSettings()
		seed := j.m.DJSeed()

		j.m = djDeliver(j.m, tickMsg{})

		if j.m.dj.unsaved.pending() {
			t.Fatalf("the reconciliation did not clear the refusal: %v", j.m.dj.unsaved.err)
		}
		if got := j.session().Settings; !got.Equal(applied) {
			t.Fatalf("saved settings = %+v, want the reconciled %+v", got, applied)
		}
		if djStatusMentions(j.m.autoDJPanelStatus(), "Not saved") {
			t.Error("the panel still reports an unsaved session after the row was written")
		}

		j.relaunch()

		if !j.m.DJActive() {
			t.Fatal("the restored launch has no session running")
		}
		if got := j.m.DJSettings(); !got.Equal(applied) {
			t.Fatalf("restored settings = %+v, want the applied %+v", got, applied)
		}
		if got := j.m.DJSeed().TrackHash; got != seed.TrackHash {
			t.Errorf("restored seed = %q, want the captured %q", got, seed.TrackHash)
		}
	})

	t.Run("a refused stop is reconciled the same way", func(t *testing.T) {
		w.fails = 1
		next, _, _ := j.m.radioToggle()
		j.m = next.(Model)

		if j.m.DJActive() {
			t.Fatal("the toggle left the session running")
		}
		if stored := j.session(); !stored.Active {
			t.Fatal("the stop reached the database, so the refusal was not injected")
		}
		if got := j.m.statusBar.TopNotification().Msg; !strings.Contains(got, "not saved") {
			t.Errorf("status bar = %q, want a notice that the stop was not saved", got)
		}

		j.m = djDeliver(j.m, tickMsg{})

		stored := j.session()
		if stored.Active || !stored.Seed.IsZero() {
			t.Fatalf("saved session = %+v, want a stopped session with no seed", stored)
		}
		if j.m.dj.unsaved.pending() {
			t.Fatalf("the stop was reconciled but still reads unsaved: %v", j.m.dj.unsaved.err)
		}

		j.relaunch()

		if j.m.DJActive() {
			t.Error("the restored launch resumed a session the user stopped")
		}
	})

	t.Run("the reconciliation is bounded and says so when it gives up", func(t *testing.T) {
		w.fails = 99
		before := w.writes

		next, _, _ := j.m.radioToggle()
		j.m = next.(Model)
		for range djPersistMaxRetries + 3 {
			j.m = djDeliver(j.m, tickMsg{})
		}

		if got := w.writes - before; got != 1+djPersistMaxRetries {
			t.Errorf("session writes = %d, want the gesture plus %d bounded retries", got, djPersistMaxRetries)
		}
		if !j.m.dj.unsaved.exhausted() {
			t.Error("the budget was not reported as spent")
		}
		if !djStatusMentions(j.m.autoDJPanelStatus(), "A restart would restore the previously saved settings") {
			t.Errorf("panel status = %+v, want the exhausted wording", j.m.autoDJPanelStatus())
		}
		if got := j.m.statusBar.TopNotification().Msg; !strings.Contains(got, "could not be saved") {
			t.Errorf("status bar = %q, want the give-up notice", got)
		}

		// A later deliberate gesture is owed a fresh budget: this is the escape
		// hatch the give-up notice leaves the user, and it must actually work.
		w.fails = 0
		applied, err := j.m.DJSettings().WithPreset(radio.PresetFamiliar)
		if err != nil {
			t.Fatalf("WithPreset: %v", err)
		}
		m, err := j.m.djApplySettings(applied)
		if err != nil {
			t.Fatalf("djApplySettings: %v", err)
		}
		j.m = m
		if j.m.dj.unsaved.pending() {
			t.Fatalf("the retried Apply still reads unsaved: %v", j.m.dj.unsaved.err)
		}
		if got := j.session().Settings; !got.Equal(applied) {
			t.Errorf("saved settings = %+v, want the reapplied %+v", got, applied)
		}
	})
}
