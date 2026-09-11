package tui

import (
	"errors"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
)

// Radio mode and the DJ row are restored from two different places, so a launch
// can come up with the refill armed and the settings behind it unreadable. These
// tests drive that launch through the events the player actually emits — a
// confirmed track change, an exhausted queue — because the interesting failure
// is not what the accessors report, it is what reaches the queue table while
// nobody is asking for anything.
//
// Both run over a real database file with the real writer: a Model-only version
// would pass with rows that were never written and settings that were never
// saved, which is precisely what is in dispute.

// djRestoreDrive runs a command the runtime would have run, delivers every
// message it produces back through Update, and keeps going with whatever those
// handlers schedule — which is how a refill gets followed from the playback
// event that triggers it, through planning, to the acceptance that publishes
// the batch.
//
// The depth bound stops it there rather than chasing the periodic timers the
// runtime reschedules forever; three hops is one more than the refill needs.
func djRestoreDrive(m Model, cmd tea.Cmd) Model {
	return djRestoreDriveDepth(m, cmd, 3)
}

func djRestoreDriveDepth(m Model, cmd tea.Cmd, depth int) Model {
	if depth <= 0 {
		return m
	}
	for _, msg := range djRestoreMessages(cmd) {
		next, out := m.Update(msg)
		m = next.(Model)
		m = djRestoreDriveDepth(m, out, depth-1)
	}
	return m
}

// djRestoreMessages runs cmd and flattens what it produces, descending into
// batches the way the runtime's scheduler does.
func djRestoreMessages(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, djRestoreMessages(c)...)
		}
		return out
	}
	if msg == nil {
		return nil
	}
	return []tea.Msg{msg}
}

// djRestoreTrackChange is the event mpv emits when the queue's current row is
// confirmed playing, which is the ordinary low-water-mark trigger.
func djRestoreTrackChange(m Model) playback.TrackChangedMsg {
	entry := m.queue.Entries[m.playback.currentIndex]
	return playback.TrackChangedMsg{
		Artist:  entry.Artist,
		Title:   entry.Title,
		Hash:    entry.Hash,
		Index:   m.playback.currentIndex,
		QueueID: entry.ID,
	}
}

// djRestoreQueueRows counts the queue rows still on disk, including the hidden
// ones a two-phase batch prepares but has not published. Rows a refused
// acceptance retires are gone from this count; rows it stranded are not.
func djRestoreQueueRows(j *djJourney) int {
	j.t.Helper()
	var n int
	if err := j.d.QueryRow(`SELECT COUNT(*) FROM queue WHERE deleted_at IS NULL`).Scan(&n); err != nil {
		j.t.Fatalf("count the queue rows: %v", err)
	}
	return n
}

// djSaveRefusedState saves a restrictive active session, then writes a discovery
// target the UI cannot produce straight into the row, which is what an older or
// hand-edited database looks like to this build. Radio mode is saved too, so the
// relaunch comes up with the refill armed.
func djSaveRefusedState(j *djJourney) radio.Settings {
	j.t.Helper()
	settings := radio.Settings{
		DiscoveryPercent: 20,
		Anchor:           radio.AnchorSeed,
		Exclude:          radio.FilterSet{Genres: []string{"Jazz", "Folk"}},
	}
	seedTrack := j.tracks[0]
	if err := j.w.WriteDJSession(db.DJSession{
		Active:   true,
		Settings: settings,
		Seed: radio.SeedSnapshot{
			TrackHash:  seedTrack.Hash,
			Title:      seedTrack.Title,
			Artist:     seedTrack.Artist,
			Album:      seedTrack.Album,
			Genres:     []string{seedTrack.Genre},
			Duration:   seedTrack.Duration,
			CapturedAt: time.Now(),
		},
	}); err != nil {
		j.t.Fatalf("save the DJ session: %v", err)
	}
	if err := j.w.WriteRadioMode(true); err != nil {
		j.t.Fatalf("save radio mode: %v", err)
	}
	if _, err := j.d.Exec(`UPDATE dj_session SET discovery_percent = 45 WHERE id = 1`); err != nil {
		j.t.Fatalf("corrupt the saved discovery target: %v", err)
	}
	return settings
}

// TestAutoDJInvalidRestoreBlocksRefill covers the launch that restores radio
// mode alongside DJ state this build refuses to read.
//
// The settings the Model shows in that state are stock defaults standing in for
// the user's — unfiltered, Balanced, anchored — so every automatic route into
// the queue has to stay shut until the user commits settings they chose.
// Otherwise the first track change quietly fills the queue with the genres the
// saved session excluded.
func TestAutoDJInvalidRestoreBlocksRefill(t *testing.T) {
	j := newDJJourney(t)
	saved := djSaveRefusedState(j)

	restoreErr := j.relaunchRestoring()
	if !errors.Is(restoreErr, db.ErrDJStateInvalid) {
		t.Fatalf("the relaunch restored %v, want a refusal wrapping ErrDJStateInvalid", restoreErr)
	}
	if !j.m.radio.enabled {
		t.Fatal("the relaunch did not restore radio mode, so there is no refill to block")
	}
	if j.m.DJRestoreErr() == nil {
		t.Fatal("the Model dropped the refusal, so the user is never told why the DJ is idle")
	}
	if len(j.m.queue.Entries) != 1 {
		t.Fatalf("restored %d queue rows, want the single manual row", len(j.m.queue.Entries))
	}

	t.Run("a confirmed track change adds nothing", func(t *testing.T) {
		j.m = djRestoreDrive(j.m, func() tea.Msg { return djRestoreTrackChange(j.m) })

		if got := len(j.visibleQueue()); got != 1 {
			t.Fatalf("the queue grew to %d rows, want the manual row alone: the refill ran against stand-in settings", got)
		}
		if got := djRestoreQueueRows(j); got != 1 {
			t.Fatalf("%d queue rows survive on disk, want 1: a hidden batch was prepared behind the refusal", got)
		}
		if len(j.player.appended) != 0 {
			t.Fatalf("the player was handed %d new tracks, want none", len(j.player.appended))
		}
		if j.m.DJActive() {
			t.Fatal("the refused state started a session")
		}
	})

	t.Run("an exhausted queue adds nothing", func(t *testing.T) {
		j.m = djRestoreDrive(j.m, func() tea.Msg { return playback.PlaybackEndedMsg{} })

		if got := len(j.visibleQueue()); got != 1 {
			t.Fatalf("the queue grew to %d rows at queue end, want the manual row alone", got)
		}
		if got := djRestoreQueueRows(j); got != 1 {
			t.Fatalf("%d queue rows survive on disk, want 1", got)
		}
	})

	t.Run("a batch that reaches the handler anyway is refused and retired", func(t *testing.T) {
		// The gate above is the first refusal, not the only one it needs: a
		// result already in flight when the refusal took hold must not be
		// published either. Planning is forced here to put one there.
		m := j.m
		m.radio.requestID++
		m.radio.enqueuePending = true
		cmd := m.radioEnqueueCmd()
		if cmd == nil {
			t.Fatal("the planner refused to build a batch, so acceptance is not under test")
		}
		msg, ok := cmd().(RadioBatchEnqueuedMsg)
		if !ok {
			t.Fatal("the refill command did not produce a batch message")
		}
		if msg.Err != nil || len(msg.Entries) == 0 {
			t.Fatalf("planning produced no batch (err=%v), so acceptance is not under test", msg.Err)
		}

		next, _ := m.handleRadioBatchEnqueuedMsg(msg)

		if got := len(next.queue.Entries); got != 1 {
			t.Fatalf("the handler published %d rows, want the manual row alone", got)
		}
		if got := len(j.visibleQueue()); got != 1 {
			t.Fatalf("the durable queue grew to %d rows, want the manual row alone", got)
		}
		if got := djRestoreQueueRows(j); got != 1 {
			t.Fatalf("%d queue rows survive on disk, want 1: the refused batch stranded its hidden rows", got)
		}
	})

	t.Run("the saved rows and the explanation survive", func(t *testing.T) {
		var pct int
		if err := j.d.QueryRow(`SELECT discovery_percent FROM dj_session WHERE id = 1`).Scan(&pct); err != nil {
			t.Fatalf("read the saved discovery target: %v", err)
		}
		if pct != 45 {
			t.Fatalf("the saved discovery target is now %d, want the original 45 left untouched", pct)
		}
		var filters int
		if err := j.d.QueryRow(`SELECT COUNT(*) FROM dj_filter_values`).Scan(&filters); err != nil {
			t.Fatalf("count the saved filter rows: %v", err)
		}
		if filters != len(saved.Exclude.Genres) {
			t.Fatalf("%d saved filter rows remain, want the %d the session excluded", filters, len(saved.Exclude.Genres))
		}
		if !errors.Is(j.m.DJRestoreErr(), db.ErrDJStateInvalid) {
			t.Fatalf("the Model reports %v, want the refusal still explained", j.m.DJRestoreErr())
		}
		if !j.m.radio.enabled {
			t.Fatal("radio mode was switched off; the user's queue and mode are theirs to keep")
		}
	})

	t.Run("applying settings lifts the refusal", func(t *testing.T) {
		// The refusal is a pause, not a dead end: the user establishing what
		// the DJ runs on is exactly what it was waiting for.
		m, err := j.m.djApplySettings(saved)
		if err != nil {
			t.Fatalf("apply the settings the user chose: %v", err)
		}
		j.m = m
		if j.m.DJRestoreErr() != nil {
			t.Fatal("the refusal outlived the settings the user applied")
		}

		// The queue ran out above, so the user is starting the manual row
		// again; the refill is judged from there.
		j.m.playback.currentIndex = 0
		j.m.playback.currentHash = j.m.queue.Entries[0].Hash
		j.m.playback.state = playback.StatePlaying
		j.m = djRestoreDrive(j.m, func() tea.Msg { return djRestoreTrackChange(j.m) })

		owned := j.djOwnedHashes()
		if len(owned) == 0 {
			t.Fatal("the refill stayed blocked after the user applied settings")
		}
		for _, hash := range owned {
			if genre := j.genreOf(hash); genre == "Jazz" || genre == "Folk" {
				t.Errorf("the refill added %q (%s), which the applied filters exclude", hash, genre)
			}
		}
	})
}

// TestAutoDJLegacyRestoreStillRefills covers the launch the guard must not
// touch: radio mode restored with nothing wrong on disk.
//
// A database that predates the DJ upgrades into a valid inactive session, which
// is indistinguishable in the Model from a launch that never started the DJ.
// Both must keep the pre-DJ behaviour they have always had — playback reaching
// the low-water mark refills the queue — and neither may start playing on its
// own.
func TestAutoDJLegacyRestoreStillRefills(t *testing.T) {
	j := newDJJourney(t)
	if err := j.w.WriteRadioMode(true); err != nil {
		t.Fatalf("save radio mode: %v", err)
	}

	if err := j.relaunchRestoring(); err != nil {
		t.Fatalf("the relaunch refused clean saved state: %v", err)
	}
	if !j.m.radio.enabled {
		t.Fatal("the relaunch did not restore radio mode")
	}
	if j.m.DJRestoreErr() != nil {
		t.Fatalf("a clean launch restored a refusal: %v", j.m.DJRestoreErr())
	}
	if got := j.m.DJSettings(); got.Validate() != nil {
		t.Fatalf("restored unusable settings %+v", got)
	}

	t.Run("a paused launch adds nothing before playback moves", func(t *testing.T) {
		// Restoring radio mode arms the refill; it does not run it. Until the
		// player reports movement the queue is the one the user left, and a
		// launch that came up paused stays paused.
		j.m.playback.state = playback.StatePaused
		next, _ := j.m.Update(tickMsg{})
		j.m = next.(Model)

		if j.m.playback.state != playback.StatePaused {
			t.Fatalf("the launch started playing on its own: state=%v", j.m.playback.state)
		}
		if got := len(j.visibleQueue()); got != 1 {
			t.Fatalf("the queue grew to %d rows while paused, want the manual row alone", got)
		}
		if len(j.player.appended) != 0 {
			t.Fatalf("the player was handed %d new tracks while paused, want none", len(j.player.appended))
		}
	})

	t.Run("playback at the low-water mark refills", func(t *testing.T) {
		j.m.playback.state = playback.StatePlaying
		j.m = djRestoreDrive(j.m, func() tea.Msg { return djRestoreTrackChange(j.m) })

		owned := j.djOwnedHashes()
		if len(owned) != radioBatchSize {
			t.Fatalf("durable DJ-owned rows = %v, want a batch of %d", owned, radioBatchSize)
		}
		if got := len(j.m.queue.Entries); got != 1+radioBatchSize {
			t.Fatalf("the Model shows %d rows, want the manual row plus a batch of %d", got, radioBatchSize)
		}
		if got := j.visibleQueue()[0].Hash; got != j.tracks[0].Hash {
			t.Fatalf("the first durable row is %q, want the manual row %q still in front", got, j.tracks[0].Hash)
		}
		if len(j.player.appended) != radioBatchSize {
			t.Fatalf("the player was handed %d tracks, want the batch of %d", len(j.player.appended), radioBatchSize)
		}
	})
}
