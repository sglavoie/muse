package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/radio"
	instui "github.com/sglavoie/muse/internal/tui/insights"
	"github.com/sglavoie/muse/internal/tui/statusbar"
)

// djStartSelectionID is the palette command that opens a contextual start.
const djStartSelectionID = "autodj.start.selection"

// djSelectionCommand returns the palette's contextual-start entry.
func djSelectionCommand(t *testing.T, m Model) (name string, enabled bool, exec func() tea.Cmd) {
	t.Helper()
	for _, cmd := range m.BuildPaletteCommands() {
		if cmd.ID == djStartSelectionID {
			on := cmd.Enabled == nil || cmd.Enabled()
			return cmd.Name, on, cmd.Exec
		}
	}
	t.Fatalf("the palette does not offer %q", djStartSelectionID)
	return "", false, nil
}

// The command exists, is named the way the plan names it, and is not a safe
// direct command: it starts real playback work, so it goes through the palette's
// ordinary confirmation path rather than firing from a typed prefix.
func TestStartAutoDJFromSelectionIsOnTheCommandPalette(t *testing.T) {
	m := djQueueModel("f1", "f2", "d1")

	name, enabled, exec := djSelectionCommand(t, m)

	if name != "Start Auto-DJ from selection…" {
		t.Fatalf("command name = %q", name)
	}
	if !enabled {
		t.Fatal("the command is dimmed with a focused row to seed from")
	}
	if exec == nil {
		t.Fatal("the command has no action")
	}
	if IsSafeDirectCommand(djStartSelectionID) {
		t.Fatal("a command that starts playback must not be a safe direct command")
	}
}

// With nothing selected and nothing focused the command is dimmed rather than
// quietly widening to the whole visible list, which is what the export commands
// do and what this one must not.
func TestStartAutoDJFromSelectionIsDimmedWithNothingToSeedFrom(t *testing.T) {
	// The queue is empty, so no row is focused — but the library behind it is
	// full, so a whole-visible-list fallback would have plenty to seize on.
	m := newDJModel(WithLibraryTracks(djTestTracks()))
	m.ActiveView = viewQueue

	if _, enabled, _ := djSelectionCommand(t, m); enabled {
		t.Fatal("the command is offered with nothing selected and nothing focused")
	}
	if m.djCanStartFromSelection() {
		t.Fatal("djCanStartFromSelection agreed to seed from nothing")
	}
}

// Running the command with nothing to seed from refuses in words, and leaves
// the session and radio mode exactly as they were.
func TestStartAutoDJFromSelectionRefusesRatherThanWidens(t *testing.T) {
	m := newDJModel(WithLibraryTracks(djTestTracks()))
	m.ActiveView = viewQueue

	next, cmd := m.djStartFromSelection(djFixedNow)

	if next.dj.active {
		t.Fatal("a refused start still started a session")
	}
	if next.radio.enabled {
		t.Fatal("a refused start still enabled radio mode")
	}
	if cmd == nil {
		t.Fatal("the refusal said nothing")
	}
	notify, ok := cmd().(statusbar.TransientNotifyMsg)
	if !ok {
		t.Fatalf("the refusal produced %T, want a transient notification", cmd())
	}
	if string(notify) != "Select or focus tracks to start Auto-DJ from" {
		t.Fatalf("refusal reads %q", string(notify))
	}
}

// The palette command reaches the same start the tests above exercise.
func TestTheSelectionCommandRoutesToTheContextualStart(t *testing.T) {
	m := djQueueModel("f1", "f2", "d1")
	m.queue.Cursor = 2
	_, _, exec := djSelectionCommand(t, m)

	msg := exec()()
	if _, ok := msg.(autoDJStartSelectionMsg); !ok {
		t.Fatalf("the command emitted %T", msg)
	}

	next, _ := m.Update(msg)
	if !next.(Model).dj.active {
		t.Fatal("the routed message started no session")
	}
}

// Precedence: a bulk selection is the most specific thing the user has said, so
// it beats the focused row and the current track, both of which point at the one
// Jazz track here.
func TestTheContextualStartPrefersTheSelectionOverTheFocusedRow(t *testing.T) {
	m := djQueueModel("f1", "f3", "d1")
	m.queue.Entries[0].Selected = true
	m.queue.Entries[2].Selected = true
	m.queue.Cursor = 1
	m.playback.currentHash = "f3"

	next, _ := m.djStartFromSelection(djFixedNow)

	if !next.dj.active {
		t.Fatal("no session started")
	}
	if next.dj.seed.Kind() != radio.SeedKindCollection {
		t.Fatalf("seed = %+v, want a collection seed over the selection", next.dj.seed)
	}
	if got := next.dj.seed.Genres; len(got) != 1 || got[0] != "Rock" {
		t.Fatalf("seed genres = %v, want only the selection's Rock", got)
	}
	// The pre-DJ scorer's own fields describe the same selection: a synthetic
	// aggregate, and no single-track hash left over from the current track.
	if next.radio.syntheticSeed == nil {
		t.Fatal("the legacy radio seed was not set from the selection")
	}
	if next.radio.seedHash != "" {
		t.Fatalf("seedHash = %q, want empty for an aggregate seed", next.radio.seedHash)
	}
	if !next.radio.enabled {
		t.Fatal("the start did not enable radio mode")
	}
}

// With no selection the focused row seeds the session — and it is the focused
// row, not the track that happens to be playing.
func TestTheContextualStartFallsBackToTheFocusedRow(t *testing.T) {
	m := djQueueModel("f1", "f2", "d1")
	m.queue.Cursor = 2
	m.playback.currentHash = "f1"

	next, _ := m.djStartFromSelection(djFixedNow)

	if want := djSeedFor(m, "d1"); next.dj.seed.TrackHash != want.TrackHash {
		t.Fatalf("seed = %+v, want the focused row %+v", next.dj.seed, want)
	}
	if next.radio.seedHash != "d1" {
		t.Fatalf("legacy seedHash = %q, want the focused row", next.radio.seedHash)
	}
	if next.radio.syntheticSeed != nil {
		t.Fatal("a single focused row produced an aggregate seed")
	}
}

// Mixes and Insights now start through the same lifecycle as everything else, so
// their seed key has to leave a real session behind rather than only flipping
// the legacy radio flag.
func TestTheMixSeedKeyStartsASession(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m = seedMixRegistry(m)
	m.mixes.CachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.SelectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	next, _ := m.handleMixesRadioSeed()

	if !next.dj.active {
		t.Fatal("seeding from a mix started no DJ session")
	}
	if next.dj.seed.Kind() != radio.SeedKindCollection {
		t.Fatalf("seed = %+v, want a collection seed over the mix", next.dj.seed)
	}
	if !next.radio.enabled || next.radio.syntheticSeed == nil {
		t.Fatal("the legacy radio fields were left behind by the shared start")
	}
}

// A bulk selection inside the open mix is still the more specific statement, and
// the shared start is what makes that true for Mixes without Mixes knowing it.
func TestASelectionInsideAMixOutranksTheMix(t *testing.T) {
	tracks := sampleMixTracks()
	m := newMixesModel(true, tracks)
	m = m.switchToView(viewMixes)
	m = seedMixRegistry(m)
	m.mixes.CachedMixes = map[db.MixID][]library.Track{db.MixRediscover: tracks}
	m.mixes.SelectedMix = 0
	m, _ = m.handleMixOpen()
	m = m.handleMixLoadedMsg(mixLoadedMsg{mixID: db.MixRediscover, tracks: tracks})

	plain, _ := m.handleMixesRadioSeed()

	m.mixes.TrackView.SelectedHashes = map[string]struct{}{tracks[0].Hash: {}}
	selected, _ := m.handleMixesRadioSeed()

	if !selected.dj.active {
		t.Fatal("no session started from the selection inside the mix")
	}
	if selected.dj.seed.Label() == plain.dj.seed.Label() {
		t.Fatalf("the selection seeded the same thing as the whole mix: %q",
			selected.dj.seed.Label())
	}
}

// Insights routes through the same shared start, so its seed key leaves a
// session behind too — and toggling it a second time still turns radio off
// rather than re-seeding, which is the behaviour the key had before.
func TestTheInsightsSeedKeyStartsASessionAndStillTogglesOff(t *testing.T) {
	m := newInsightsModel(t)
	m.insights.Level = instui.LevelBriefing

	started, _ := m.handleInsightsRadioSeed()
	if !started.dj.active {
		t.Fatal("seeding from an Insights row started no DJ session")
	}
	if !started.radio.enabled || started.radio.syntheticSeed == nil {
		t.Fatal("the legacy radio fields were left behind by the shared start")
	}

	off, _ := started.handleInsightsRadioSeed()
	if off.radio.enabled {
		t.Fatal("a second press did not turn radio off")
	}
	if off.radio.syntheticSeed != nil {
		t.Fatal("the synthetic seed survived the toggle off")
	}
}
