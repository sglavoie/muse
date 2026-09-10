package tui

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
)

// djStubWriter records every lifecycle write, so a test can assert not only the
// Model's state but that the database was told the same thing.
type djStubWriter struct {
	writes []db.DJSession
	stops  int

	writeErr error
	stopErr  error
}

func (w *djStubWriter) WriteDJSession(session db.DJSession) error {
	w.writes = append(w.writes, session)
	return w.writeErr
}

func (w *djStubWriter) StopDJSession() error {
	w.stops++
	return w.stopErr
}

func (w *djStubWriter) last(t *testing.T) db.DJSession {
	t.Helper()
	if len(w.writes) == 0 {
		t.Fatal("no session write was issued")
	}
	return w.writes[len(w.writes)-1]
}

// djExploreFollow is a settings snapshot no default could be mistaken for.
func djExploreFollow() radio.Settings {
	s := radio.DefaultSettings()
	s.DiscoveryPercent = 80
	s.Anchor = radio.AnchorFollow
	return s
}

// djSeedFor snapshots one of the planning fixture's tracks.
func djSeedFor(m Model, hash string) radio.SeedSnapshot {
	track, _ := m.library.Index.FindByHash(hash)
	return radio.CaptureSeed(track, m.trackTags[hash], radio.FixedClock(djFixedNow))
}

// TestDJFreshModelStartsOnUsableSettings covers the first launch before
// anything is restored. The zero radio.Settings has no anchor mode and fails
// validation, so a Model that shipped it would refuse the user's first applied
// change and persist nothing.
func TestDJFreshModelStartsOnUsableSettings(t *testing.T) {
	m := newDJModel()

	if m.DJActive() {
		t.Error("a fresh Model reported a running DJ session")
	}
	if !m.DJSeed().IsZero() {
		t.Errorf("a fresh Model carried seed %+v", m.DJSeed())
	}
	if err := m.DJSettings().Validate(); err != nil {
		t.Errorf("a fresh Model's settings do not validate: %v", err)
	}
	if m.DJRestoreErr() != nil {
		t.Errorf("a fresh Model reported a restore error: %v", m.DJRestoreErr())
	}
}

// TestDJRestoreResumesTheRunningSession is the restart contract: the seed comes
// back rather than being re-captured, because re-capturing would silently
// re-point the session at whatever happened to be playing when Muse was closed.
func TestDJRestoreResumesTheRunningSession(t *testing.T) {
	w := &djStubWriter{}
	base := newDJModel(WithDJWriter(w))
	seed := djSeedFor(base, "f1")
	settings := djExploreFollow()

	m := base.djRestore(db.DJSession{
		Active:     true,
		Settings:   settings,
		Seed:       seed,
		Allocation: radio.Allocation{CarryUnits: 30},
	}, nil)

	if !m.DJActive() {
		t.Error("a restored active session came back stopped")
	}
	if !reflect.DeepEqual(m.DJSeed(), seed) {
		t.Errorf("Seed = %+v, want the persisted %+v", m.DJSeed(), seed)
	}
	if want := settings.Normalized(); !reflect.DeepEqual(m.DJSettings(), want) {
		t.Errorf("Settings = %+v, want %+v", m.DJSettings(), want)
	}
	if m.DJAllocation().CarryUnits != 30 {
		t.Errorf("CarryUnits = %d, want the carry the session earned (30)", m.DJAllocation().CarryUnits)
	}
	if len(w.writes) != 0 || w.stops != 0 {
		t.Errorf("restoring wrote back to storage: %d writes, %d stops", len(w.writes), w.stops)
	}
}

// TestDJRestoreKeepsSettingsOfAStoppedSession: a stopped session still owns the
// user's filters, so the Tune tab reopens where they left it rather than on
// stock defaults.
func TestDJRestoreKeepsSettingsOfAStoppedSession(t *testing.T) {
	settings := djExploreFollow()

	m := newDJModel().djRestore(db.DJSession{Settings: settings}, nil)

	if m.DJActive() {
		t.Error("a stopped session came back active")
	}
	if want := settings.Normalized(); !reflect.DeepEqual(m.DJSettings(), want) {
		t.Errorf("Settings = %+v, want the persisted %+v", m.DJSettings(), want)
	}
}

// TestDJRestoreRefusesUnusableState covers both ways persisted state can be
// unusable. Neither is repaired: stock defaults in place of unreadable
// restrictive filters would hand the user an unrestricted DJ they never asked
// for, which is the one outcome the contract forbids.
func TestDJRestoreRefusesUnusableState(t *testing.T) {
	boom := errors.New("read dj session: " + db.ErrDJStateInvalid.Error())

	t.Run("state the reader refused", func(t *testing.T) {
		m := newDJModel().djRestore(db.DJSession{}, boom)

		if !errors.Is(m.DJRestoreErr(), boom) {
			t.Errorf("DJRestoreErr = %v, want the read error", m.DJRestoreErr())
		}
		if m.DJActive() {
			t.Error("unreadable state started a session")
		}
		if want := defaultDJSettings(); !reflect.DeepEqual(m.DJSettings(), want) {
			t.Errorf("Settings = %+v, want stock defaults", m.DJSettings())
		}
	})

	t.Run("settings that do not validate", func(t *testing.T) {
		settings := radio.DefaultSettings()
		settings.DiscoveryPercent = 45 // not a ten-point increment

		m := newDJModel().djRestore(db.DJSession{Active: true, Settings: settings}, nil)

		if m.DJRestoreErr() == nil {
			t.Error("invalid settings were adopted without complaint")
		}
		if m.DJActive() {
			t.Error("invalid settings started a session")
		}
		if err := m.DJSettings().Validate(); err != nil {
			t.Errorf("the fallback settings do not validate: %v", err)
		}
	})

	t.Run("an active session with no seed", func(t *testing.T) {
		m := newDJModel().djRestore(db.DJSession{Active: true, Settings: radio.DefaultSettings()}, nil)

		// Storage refuses to write this, but a session that cannot say what it
		// plays from must not run either way: the planner would reject every
		// batch it produced.
		if m.DJActive() {
			t.Error("a seedless session came back active")
		}
	})
}

// TestDJStartCapturesAFreshSeedAfterARestart is the other half of the restart
// contract: a restored session keeps its seed, but a deliberate new start
// replaces it and drops the carry the previous session earned.
func TestDJStartCapturesAFreshSeedAfterARestart(t *testing.T) {
	w := &djStubWriter{}
	base := newDJModel(WithDJWriter(w))
	restored := base.djRestore(db.DJSession{
		Active:     true,
		Settings:   radio.DefaultSettings(),
		Seed:       djSeedFor(base, "f1"),
		Allocation: radio.Allocation{CarryUnits: 30},
	}, nil)

	fresh := djSeedFor(base, "d1")
	m := restored.djStart(fresh)

	if !reflect.DeepEqual(m.DJSeed(), fresh) {
		t.Errorf("Seed = %+v, want the freshly captured %+v", m.DJSeed(), fresh)
	}
	if !m.DJAllocation().IsZero() {
		t.Errorf("CarryUnits = %d after a new start: the previous session's rounding carry "+
			"biased the first batch", m.DJAllocation().CarryUnits)
	}
	if got := w.last(t); !got.Active || got.Seed.TrackHash != "d1" {
		t.Errorf("persisted session = %+v, want the new seed active", got)
	}
}

// TestDJStartRefusesAZeroSeed: the planner rejects an empty seed, so a session
// started from one would produce nothing and report itself as running.
func TestDJStartRefusesAZeroSeed(t *testing.T) {
	w := &djStubWriter{}

	m := newDJModel(WithDJWriter(w)).djStart(radio.SeedSnapshot{})

	if m.DJActive() {
		t.Error("a zero seed started a session")
	}
	if len(w.writes) != 0 {
		t.Errorf("a refused start still wrote %+v", w.writes)
	}
}

// TestDJStopClearsTheSeedAndKeepsSettings. Dropping the seed is what makes the
// next start a genuinely new one: a stopped session with a seed still on disk
// would be resumed by the next launch as if the user had never stopped it.
func TestDJStopClearsTheSeedAndKeepsSettings(t *testing.T) {
	w := &djStubWriter{}
	base := newDJModel(WithDJWriter(w))
	settings := djExploreFollow()
	m, err := base.djApplySettings(settings)
	if err != nil {
		t.Fatalf("djApplySettings: %v", err)
	}
	m = m.djStart(djSeedFor(base, "f1")).djAdoptAllocation(radio.Allocation{CarryUnits: 20})

	m = m.djStop()

	if m.DJActive() {
		t.Error("the session is still running after a stop")
	}
	if !m.DJSeed().IsZero() {
		t.Errorf("Seed = %+v after a stop, want none", m.DJSeed())
	}
	if !m.DJAllocation().IsZero() {
		t.Errorf("Allocation = %+v after a stop, want zero", m.DJAllocation())
	}
	if want := settings.Normalized(); !reflect.DeepEqual(m.DJSettings(), want) {
		t.Errorf("Settings = %+v after a stop, want them kept as %+v", m.DJSettings(), want)
	}
	if w.stops != 1 {
		t.Errorf("StopDJSession was called %d times, want 1", w.stops)
	}
}

// TestDJResumeNeedsASeed covers the transition a restored-but-stopped session
// goes through, and the case where there is nothing to resume — which is the
// caller's cue to capture a fresh seed instead.
func TestDJResumeNeedsASeed(t *testing.T) {
	w := &djStubWriter{}
	base := newDJModel(WithDJWriter(w))
	seed := djSeedFor(base, "f1")

	stopped := base.djRestore(db.DJSession{Settings: radio.DefaultSettings(), Seed: seed}, nil)
	resumed, ok := stopped.djResume()
	if !ok {
		t.Fatal("a session with a restored seed refused to resume")
	}
	if !resumed.DJActive() || !reflect.DeepEqual(resumed.DJSeed(), seed) {
		t.Errorf("resumed session = active:%v seed:%+v, want the restored seed running",
			resumed.DJActive(), resumed.DJSeed())
	}

	if _, ok := base.djResume(); ok {
		t.Error("a session with no seed reported itself resumed")
	}
}

// TestDJReseedKeepsTheAllocation: the session continues, so its rounding carry
// continues with it. Only a deliberate new start earns a clean slate.
func TestDJReseedKeepsTheAllocation(t *testing.T) {
	base := newDJModel(WithDJWriter(&djStubWriter{}))
	m := base.djStart(djSeedFor(base, "f1")).djAdoptAllocation(radio.Allocation{CarryUnits: 20})

	next := djSeedFor(base, "d1")
	m = m.djReseed(next)

	if !reflect.DeepEqual(m.DJSeed(), next) {
		t.Errorf("Seed = %+v, want %+v", m.DJSeed(), next)
	}
	if m.DJAllocation().CarryUnits != 20 {
		t.Errorf("CarryUnits = %d, want the running session's carry (20)", m.DJAllocation().CarryUnits)
	}
}

// TestDJApplySettingsResetsTheAllocation: a carry accumulated against a 40%
// discovery target would be spent on a batch planned at 80%, biasing it for
// reasons the user cannot see.
func TestDJApplySettingsResetsTheAllocation(t *testing.T) {
	base := newDJModel(WithDJWriter(&djStubWriter{}))
	m := base.djStart(djSeedFor(base, "f1")).djAdoptAllocation(radio.Allocation{CarryUnits: 20})

	m, err := m.djApplySettings(djExploreFollow())
	if err != nil {
		t.Fatalf("djApplySettings: %v", err)
	}
	if !m.DJAllocation().IsZero() {
		t.Errorf("CarryUnits = %d after a target change, want zero", m.DJAllocation().CarryUnits)
	}

	invalid := radio.DefaultSettings()
	invalid.DiscoveryPercent = 45
	after, err := m.djApplySettings(invalid)
	if err == nil {
		t.Fatal("djApplySettings accepted an off-step discovery target")
	}
	if !reflect.DeepEqual(after.DJSettings(), m.DJSettings()) {
		t.Errorf("a refused change still moved the settings to %+v", after.DJSettings())
	}
}

// TestDJAllocationOnlyMovesForARunningSession: allocation is consumed by
// accepted work, and a stopped session accepts none.
func TestDJAllocationOnlyMovesForARunningSession(t *testing.T) {
	m := newDJModel(WithDJWriter(&djStubWriter{})).djAdoptAllocation(radio.Allocation{CarryUnits: 20})

	if !m.DJAllocation().IsZero() {
		t.Errorf("a stopped session adopted allocation %+v", m.DJAllocation())
	}
}

// TestDJAnchorModeGovernsReseeding is the anchoring contract in both modes.
// Anchor retains the context the session was captured with; Follow moves, and
// only on a confirmed track change.
func TestDJAnchorModeGovernsReseeding(t *testing.T) {
	base := newDJModel(WithDJWriter(&djStubWriter{}))
	seed := djSeedFor(base, "f1")

	t.Run("anchor holds its captured context", func(t *testing.T) {
		m := base.djStart(seed)

		m = m.djNoteTrackChanged("d1", djFixedNow)

		if !reflect.DeepEqual(m.DJSeed(), seed) {
			t.Errorf("Seed = %+v after a track change under Anchor, want the captured %+v",
				m.DJSeed(), seed)
		}
	})

	t.Run("follow re-anchors on the new track", func(t *testing.T) {
		m, err := base.djApplySettings(djExploreFollow())
		if err != nil {
			t.Fatalf("djApplySettings: %v", err)
		}
		m = m.djStart(seed)

		m = m.djNoteTrackChanged("d1", djFixedNow)

		if want := djSeedFor(base, "d1"); !reflect.DeepEqual(m.DJSeed(), want) {
			t.Errorf("Seed = %+v after a Follow track change, want %+v", m.DJSeed(), want)
		}
	})

	t.Run("switching to anchor freezes the seed", func(t *testing.T) {
		m, err := base.djApplySettings(djExploreFollow())
		if err != nil {
			t.Fatalf("djApplySettings: %v", err)
		}
		m = m.djStart(seed).djNoteTrackChanged("d1", djFixedNow)
		followed := m.DJSeed()

		m, err = m.djApplySettings(radio.DefaultSettings())
		if err != nil {
			t.Fatalf("djApplySettings: %v", err)
		}
		m = m.djNoteTrackChanged("d2", djFixedNow)

		if !reflect.DeepEqual(m.DJSeed(), followed) {
			t.Errorf("Seed = %+v after switching to Anchor, want the seed it was on (%+v)",
				m.DJSeed(), followed)
		}
	})
}

// TestDJNoteTrackChangedIgnoresNonEvents. Each case here is a track change the
// seed must not move for: an unindexed row carries no metadata to score
// against, a repeat of the current seed is no change at all, and a stopped
// session has no seed to move.
func TestDJNoteTrackChangedIgnoresNonEvents(t *testing.T) {
	base := newDJModel(WithDJWriter(&djStubWriter{}))
	follow, err := base.djApplySettings(djExploreFollow())
	if err != nil {
		t.Fatalf("djApplySettings: %v", err)
	}
	seed := djSeedFor(base, "f1")
	running := follow.djStart(seed)

	cases := map[string]struct {
		model Model
		hash  string
	}{
		"an empty hash":                      {running, ""},
		"the seed the session is already on": {running, "f1"},
		"a row the library index has lost":   {running, "gone"},
		"a stopped session":                  {follow, "d1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			before := tc.model.DJSeed()

			got := tc.model.djNoteTrackChanged(tc.hash, djFixedNow)

			if !reflect.DeepEqual(got.DJSeed(), before) {
				t.Errorf("Seed moved to %+v, want it left at %+v", got.DJSeed(), before)
			}
		})
	}
}

// TestDJPersistFailureKeepsTheTransition: refusing the transition would leave
// the user looking at a DJ that ignored their keypress, and the next successful
// write reconciles the row anyway.
func TestDJPersistFailureKeepsTheTransition(t *testing.T) {
	boom := errors.New("database is locked")
	w := &djStubWriter{writeErr: boom, stopErr: boom}
	base := newDJModel(WithDJWriter(w))
	seed := djSeedFor(base, "f1")

	m := base.djStart(seed)
	if !m.DJActive() || !reflect.DeepEqual(m.DJSeed(), seed) {
		t.Errorf("a failed write rolled the start back: active=%v seed=%+v", m.DJActive(), m.DJSeed())
	}

	m = m.djStop()
	if m.DJActive() || !m.DJSeed().IsZero() {
		t.Errorf("a failed stop left the session running: active=%v seed=%+v", m.DJActive(), m.DJSeed())
	}
}

// TestDJLifecycleWithoutAWriter covers every transition in a build with no
// database — the uitest harness, and any Model constructed without options.
func TestDJLifecycleWithoutAWriter(t *testing.T) {
	m := newDJModel()
	seed := djSeedFor(m, "f1")

	m = m.djStart(seed)
	m = m.djAdoptAllocation(radio.Allocation{CarryUnits: 10})
	m = m.djReseed(djSeedFor(m, "d1"))
	if m, err := m.djApplySettings(djExploreFollow()); err != nil {
		t.Fatalf("djApplySettings: %v (state %+v)", err, m.dj)
	}
	m = m.djStop()

	if m.DJActive() {
		t.Error("the session survived a stop")
	}
}

// TestDJSnapshotMirrorsTheModel is the persistence boundary: whatever the Model
// believes has to be exactly what the row is told, or a restart would resume a
// session that is not the one that was running.
func TestDJSnapshotMirrorsTheModel(t *testing.T) {
	w := &djStubWriter{}
	base := newDJModel(WithDJWriter(w))
	settings := djExploreFollow()
	m, err := base.djApplySettings(settings)
	if err != nil {
		t.Fatalf("djApplySettings: %v", err)
	}
	seed := djSeedFor(base, "f1")
	m = m.djStart(seed).djAdoptAllocation(radio.Allocation{CarryUnits: 20})

	want := db.DJSession{
		Active:     true,
		Settings:   settings.Normalized(),
		Seed:       seed,
		Allocation: radio.Allocation{CarryUnits: 20},
	}
	if got := w.last(t); !reflect.DeepEqual(got, want) {
		t.Errorf("persisted session = %+v, want %+v", got, want)
	}
	if got := m.djSnapshot(); !reflect.DeepEqual(got, want) {
		t.Errorf("the Model's own snapshot = %+v, want %+v", got, want)
	}
}

// TestDJRestoreOptionAppliesAtStartup wires the option the startup path uses,
// so a change to djRestore's signature cannot quietly leave the model unsettled.
func TestDJRestoreOptionAppliesAtStartup(t *testing.T) {
	seed := radio.CaptureSeed(
		library.Track{Hash: "f1", Title: "F1", Artist: "Alice", Genre: "Rock", Duration: 200},
		nil,
		radio.FixedClock(djFixedNow),
	)

	m := newDJModel(WithDJSession(db.DJSession{
		Active:   true,
		Settings: djExploreFollow(),
		Seed:     seed,
	}, nil))

	if !m.DJActive() || !reflect.DeepEqual(m.DJSeed(), seed) {
		t.Errorf("WithDJSession left the model at active=%v seed=%+v", m.DJActive(), m.DJSeed())
	}
	if m.DJSettings().Anchor != radio.AnchorFollow {
		t.Errorf("Anchor = %q, want the restored %q", m.DJSettings().Anchor, radio.AnchorFollow)
	}
}
