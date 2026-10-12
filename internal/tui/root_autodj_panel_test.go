package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/tui/autodj"
	"github.com/sglavoie/muse/internal/tui/keys"
)

// djPanelKey builds the key press whose String() is key, so these tests name
// keys the way the panel and the registry do.
func djPanelKey(key string) tea.KeyPressMsg {
	named := map[string]rune{
		"esc": tea.KeyEscape, "enter": tea.KeyEnter, "tab": tea.KeyTab,
		"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	}
	if code, ok := named[key]; ok {
		return tea.KeyPressMsg{Code: code}
	}
	if rest, ok := strings.CutPrefix(key, "ctrl+"); ok {
		return tea.KeyPressMsg{Code: []rune(rest)[0], Mod: tea.ModCtrl}
	}
	return tea.KeyPressMsg{Code: []rune(key)[0], Text: key}
}

// djPress sends key presses through the whole update loop and keeps the last
// command, which is where a commit's refresh comes back from.
func djPress(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, key := range keys {
		msg := djPanelKey(key)
		if got := msg.String(); got != key {
			t.Fatalf("djPanelKey(%q).String() = %q", key, got)
		}
		next, out := m.Update(msg)
		m, cmd = next.(Model), out
	}
	return m, cmd
}

// djDeliver feeds an async result back into the model, which is where a commit
// acknowledgement lands.
func djDeliver(m Model, msg tea.Msg) Model {
	next, _ := m.Update(msg)
	return next.(Model)
}

// djPanel returns the open Tune panel, failing when there is none.
func djPanel(t *testing.T, m Model) *autodj.TunePanel {
	t.Helper()
	if m.overlay.autoDJ == nil {
		t.Fatal("no Tune panel is open")
	}
	return m.overlay.autoDJ
}

// errDJPanelWrite is the storage failure the refusal test injects.
var errDJPanelWrite = errors.New("prepare replacement: database is locked")

// newTunableModel returns a stopped session over the planning fixture with a
// tag catalog the pickers can offer.
func newTunableModel() Model {
	m := newDJModel()
	m.tags = []db.TagRow{{ID: 1, Name: "chill"}, {ID: 2, Name: "focus"}}
	m.tagsView.TagCounts = map[int64]int{1: 3, 2: 1}
	return m
}

func TestAutoDJTunePanelOpensOnTheAppliedSettings(t *testing.T) {
	m, err := newTunableModel().djApplySettings(exploreSettings())
	if err != nil {
		t.Fatalf("djApplySettings: %v", err)
	}

	m, _ = djPress(t, m, "ctrl+a")

	panel := djPanel(t, m)
	if got := panel.Draft(); !got.Equal(m.dj.settings) {
		t.Fatalf("draft = %+v, want the applied %+v", got, m.dj.settings)
	}
	if panel.Dirty() {
		t.Fatal("a freshly opened panel reports unapplied changes")
	}
	if got := panel.CommitLabel(); got != "Start" {
		t.Fatalf("commit label = %q, want Start with no session running", got)
	}
	if !m.hasActiveOverlay() {
		t.Fatal("the Tune panel is not counted as an active overlay")
	}
	if got := m.Snapshot().Overlay; got != "autodj_tune" {
		t.Fatalf("overlay snapshot = %q, want autodj_tune", got)
	}
}

func TestAutoDJTunePanelOffersTheLibraryCatalog(t *testing.T) {
	cat := newTunableModel().autoDJCatalog()

	if len(cat.Tags) != 2 {
		t.Fatalf("catalog tags = %+v, want the two tags that hold tracks", cat.Tags)
	}
	// Browse has never been visited, so both lists come from the library index.
	if len(cat.Genres) == 0 || len(cat.Artists) == 0 {
		t.Fatalf("catalog = %+v, want genres and artists derived from the library", cat)
	}
}

func TestAutoDJTunePanelExcludesEmptyTags(t *testing.T) {
	m := newTunableModel()
	m.tagsView.TagCounts = map[int64]int{1: 3, 2: 0}

	cat := m.autoDJCatalog()
	if len(cat.Tags) != 1 || cat.Tags[0].ID != 1 {
		t.Fatalf("catalog tags = %+v, want only the tag that holds tracks", cat.Tags)
	}
}

// TestAutoDJTuneKeepsTheRadioToggle: the panel is a new gesture, not a
// replacement for the one-key toggle the user already has.
func TestAutoDJTuneKeepsTheRadioToggle(t *testing.T) {
	m, _ := djPress(t, newTunableModel(), "A")

	if m.overlay.autoDJ != nil {
		t.Fatal("A opened the Tune panel instead of toggling radio mode")
	}
	if !m.radio.enabled {
		t.Fatal("A no longer toggles radio mode")
	}

	reg := keys.NewRegistry()
	if got := reg.Lookup(keys.ContextGlobal, "ctrl+a"); got != keys.ActionAutoDJTune {
		t.Fatalf("ctrl+a is bound to %v, want ActionAutoDJTune", got)
	}
	if got := reg.Lookup(keys.ContextGlobal, "A"); got != keys.ActionRadioToggle {
		t.Fatalf("A is bound to %v, want ActionRadioToggle", got)
	}
}

func TestAutoDJTunePanelOwnsTheKeyboardWhileOpen(t *testing.T) {
	m, _ := djPress(t, newTunableModel(), "ctrl+a")
	before := m.ActiveView

	m, _ = djPress(t, m, "down", "down")
	if got := djPanel(t, m).Cursor(); got != 2 {
		t.Fatalf("cursor = %d, want the panel to have taken the arrows", got)
	}

	// A key the library view would act on must not reach it.
	m, _ = djPress(t, m, "2")
	if m.ActiveView != before {
		t.Fatalf("view mode changed to %v while the panel was open", m.ActiveView)
	}
	if m.overlay.autoDJ == nil {
		t.Fatal("the panel closed on an unrelated key")
	}
}

// TestAutoDJTuneCancelDiscardsTheDraft verifies that only explicit discard
// closes an edited draft, without applying it to the running session.
func TestAutoDJTuneCancelDiscardsTheDraft(t *testing.T) {
	m, _ := djPress(t, newTunableModel(), "ctrl+a")
	applied := m.dj.settings

	m, _ = djPress(t, m, "down", "right", "right") // tune discovery twice
	if !djPanel(t, m).Dirty() {
		t.Fatal("the edit did not reach the draft")
	}

	m, _ = djPress(t, m, "esc")
	if m.overlay.autoDJ == nil {
		t.Fatal("edited draft closed before confirmation")
	}
	m, _ = djPress(t, m, "enter") // Keep editing is the default.
	if !djPanel(t, m).Dirty() {
		t.Fatal("keeping draft lost edits")
	}
	m, _ = djPress(t, m, "esc", "tab", "enter")
	if m.overlay.autoDJ != nil {
		t.Fatal("explicit discard did not close the panel")
	}
	if !m.dj.settings.Equal(applied) {
		t.Fatalf("settings = %+v after a cancel, want the applied %+v", m.dj.settings, applied)
	}

	m, _ = djPress(t, m, "ctrl+a")
	if got := djPanel(t, m).Draft(); !got.Equal(applied) {
		t.Fatalf("reopened draft = %+v, want the applied %+v", got, applied)
	}
}

// TestAutoDJTuneCancelDoesNotStopARunningSession: the panel edits settings, so
// leaving it must not be a way to stop the DJ.
func TestAutoDJTuneCancelDoesNotStopARunningSession(t *testing.T) {
	m, _ := djRefreshModel(djDefaultQueue(), newDJReplacementWriter())
	m, _ = djPress(t, m, "ctrl+a")

	m, _ = djPress(t, m, "esc")

	if !m.dj.active || !m.radio.enabled {
		t.Fatalf("cancel left the session at active=%v radio=%v", m.dj.active, m.radio.enabled)
	}
}

// TestAutoDJTuneStartsAStoppedSession covers the Start half of the primary
// action: no session is running, so the commit has to make one.
func TestAutoDJTuneStartsAStoppedSession(t *testing.T) {
	m := newTunableModel()
	m.playback.currentHash = "f1"
	m, _ = djPress(t, m, "ctrl+a")

	m, _ = djPress(t, m, "down", "right") // 40% -> 50%
	draft := djPanel(t, m).Draft()

	m, _ = djPress(t, m, "ctrl+s")

	if !m.dj.active {
		t.Fatal("Start did not start a session")
	}
	if !m.radio.enabled {
		t.Fatal("Start left radio mode off, so the refill would never run")
	}
	if got := m.dj.settings; !got.Equal(draft) {
		t.Fatalf("settings = %+v, want the committed draft %+v", got, draft)
	}
	if m.dj.seed.IsZero() {
		t.Fatal("Start captured no seed")
	}

	// Nothing is in flight, so the panel is answered on the spot.
	panel := djPanel(t, m)
	if panel.Busy() {
		t.Fatal("the panel is still waiting for an answer it already has")
	}
	if panel.Err() != nil {
		t.Fatalf("Start reported %v", panel.Err())
	}
	if panel.Dirty() {
		t.Fatal("the committed draft is still reported as unapplied")
	}
	if got := panel.CommitLabel(); got != "Apply" {
		t.Fatalf("commit label = %q, want Apply once the session runs", got)
	}
}

// TestAutoDJTuneAppliesToARunningSession covers the Apply half, including the
// wait: the answer only arrives when the queue refresh it launched comes back.
func TestAutoDJTuneAppliesToARunningSession(t *testing.T) {
	writer := newDJReplacementWriter()
	m, _ := djRefreshModel(djDefaultQueue(), writer)
	m, _ = djPress(t, m, "ctrl+a")

	m, _ = djPress(t, m, "down", "right", "right", "right", "right") // 40% -> 80%
	draft := djPanel(t, m).Draft()

	m, cmd := djPress(t, m, "ctrl+s")
	if cmd == nil {
		t.Fatal("Apply launched no refresh")
	}
	if got := m.dj.settings; !got.Equal(draft) {
		t.Fatalf("settings = %+v, want the applied draft %+v", got, draft)
	}
	panel := djPanel(t, m)
	if !panel.Busy() {
		t.Fatal("the panel is not waiting for the refresh it launched")
	}
	if got := panel.AppliedSettings(); got.Equal(draft) {
		t.Fatal("the panel moved its baseline before the commit was answered")
	}

	msg, ok := cmd().(djRefreshDoneMsg)
	if !ok {
		t.Fatalf("Apply produced %T, want a refresh result", cmd())
	}
	m = djDeliver(m, msg)

	panel = djPanel(t, m)
	if panel.Busy() {
		t.Fatal("the refresh result did not answer the commit")
	}
	if panel.Err() != nil {
		t.Fatalf("a successful refresh reported %v", panel.Err())
	}
	if got := panel.AppliedSettings(); !got.Equal(draft) {
		t.Fatalf("applied = %+v, want the committed %+v", got, draft)
	}
	if panel.Dirty() {
		t.Fatal("the panel still reports unapplied changes")
	}
}

// TestAutoDJTuneKeepsEditsMadeWhileApplying is the whole reason the
// acknowledgement never writes the draft.
func TestAutoDJTuneKeepsEditsMadeWhileApplying(t *testing.T) {
	m, _ := djRefreshModel(djDefaultQueue(), newDJReplacementWriter())
	m, _ = djPress(t, m, "ctrl+a")
	m, cmd := djPress(t, m, "down", "right", "ctrl+s")
	if cmd == nil {
		t.Fatal("Apply launched no refresh")
	}

	// The user keeps tuning while the refresh runs.
	m, _ = djPress(t, m, "right")
	edited := djPanel(t, m).Draft()

	m = djDeliver(m, cmd())

	panel := djPanel(t, m)
	if got := panel.Draft(); !got.Equal(edited) {
		t.Fatalf("draft = %+v, want the edit made while applying (%+v)", got, edited)
	}
	if !panel.Dirty() {
		t.Fatal("the pending edit was swallowed by the acknowledgement")
	}
}
