package tui

import (
	"errors"
	"reflect"
	"testing"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// djStubWriter records every lifecycle write, so a test can assert not only the
// Model's state but that the database was told the same thing.
type djStubWriter struct {
	writes []db.DJSession
	stops  int
	// remaps records the merged-tag repairs asked of storage, in order.
	remaps []djTagMerge

	writeErr error
	stopErr  error
	remapErr error
}

func (w *djStubWriter) WriteDJSession(session db.DJSession) error {
	w.writes = append(w.writes, session)
	return w.writeErr
}

func (w *djStubWriter) StopDJSession() error {
	w.stops++
	return w.stopErr
}

func (w *djStubWriter) RemapDJTagRefs(sourceID, destID int64) error {
	w.remaps = append(w.remaps, djTagMerge{src: sourceID, dest: destID})
	return w.remapErr
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
