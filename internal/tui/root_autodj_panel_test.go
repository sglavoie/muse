package tui

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
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

// TestAutoDJTuneCancelDiscardsTheDraft is the cancellation contract: esc must
// leave neither the panel nor the session carrying what was typed.
func TestAutoDJTuneCancelDiscardsTheDraft(t *testing.T) {
	m, _ := djPress(t, newTunableModel(), "ctrl+a")
	applied := m.dj.settings

	m, _ = djPress(t, m, "down", "right", "right") // tune discovery twice
	if !djPanel(t, m).Dirty() {
		t.Fatal("the edit did not reach the draft")
	}

	m, _ = djPress(t, m, "esc")
	if m.overlay.autoDJ != nil {
		t.Fatal("esc did not close the panel")
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

// TestAutoDJTuneReportsARefusedRefresh: the settings are durable either way, so
// only a genuine failure is worth saying anything about.
func TestAutoDJTuneReportsARefusedRefresh(t *testing.T) {
	writer := newDJReplacementWriter()
	writer.replaceErr = errDJPanelWrite
	m, _ := djRefreshModel(djDefaultQueue(), writer)
	m, _ = djPress(t, m, "ctrl+a")

	m, cmd := djPress(t, m, "down", "right", "ctrl+s")
	if cmd == nil {
		t.Fatal("Apply launched no refresh")
	}
	m = djDeliver(m, cmd())

	panel := djPanel(t, m)
	if panel.Busy() {
		t.Fatal("a failed refresh never answered the commit")
	}
	if panel.Err() == nil {
		t.Fatal("a failed refresh was reported as a success")
	}
	if got := panel.Err().Error(); !strings.Contains(got, "queue refresh") {
		t.Fatalf("err = %q, want it to name the refresh", got)
	}
	// The settings were adopted before the refresh was launched, so they stand.
	if got := m.dj.settings.DiscoveryPercent; got != 50 {
		t.Fatalf("discovery = %d, want the applied 50 to have survived", got)
	}
}

// TestAutoDJTuneIgnoresAnAnswerToAClosedPanel: closing does not withdraw the
// commit, so its answer has to land nowhere rather than on the next panel.
func TestAutoDJTuneIgnoresAnAnswerToAClosedPanel(t *testing.T) {
	m, _ := djRefreshModel(djDefaultQueue(), newDJReplacementWriter())
	m, _ = djPress(t, m, "ctrl+a")
	m, cmd := djPress(t, m, "down", "right", "ctrl+s")
	if cmd == nil {
		t.Fatal("Apply launched no refresh")
	}
	msg := cmd()

	m, _ = djPress(t, m, "esc")    // close the panel the commit belonged to
	m, _ = djPress(t, m, "ctrl+a") // and open a fresh one
	reopened := djPanel(t, m).Draft()

	m = djDeliver(m, msg)

	panel := djPanel(t, m)
	if panel.Busy() {
		t.Fatal("the reopened panel adopted an answer it never asked for")
	}
	if panel.Err() != nil {
		t.Fatalf("the reopened panel reported %v", panel.Err())
	}
	if got := panel.Draft(); !got.Equal(reopened) {
		t.Fatalf("draft = %+v, want the reopened %+v", got, reopened)
	}
}

// TestAutoDJTuneAnswersAWithdrawnRefresh: a commit whose refresh is superseded
// gets no result of its own, and the panel refuses a second commit while one is
// in flight — so the withdrawal has to answer it or the button stays dead.
func TestAutoDJTuneAnswersAWithdrawnRefresh(t *testing.T) {
	m, _ := djRefreshModel(djDefaultQueue(), newDJReplacementWriter())
	m, _ = djPress(t, m, "ctrl+a")

	m, cmd := djPress(t, m, "down", "right", "ctrl+s")
	if cmd == nil {
		t.Fatal("Apply launched no refresh")
	}
	committed := djPanel(t, m).Draft()
	if !djPanel(t, m).Busy() {
		t.Fatal("Apply is not waiting for its refresh")
	}

	// supersedeRadioBatch is the choke point every gesture that invalidates DJ
	// work goes through: a queue clear, a reseed, a disable.
	m = m.supersedeRadioBatch()

	panel := djPanel(t, m)
	if panel.Busy() {
		t.Fatal("the withdrawn refresh left the commit unanswered")
	}
	if panel.Err() != nil {
		t.Fatalf("a withdrawal was reported as a failure: %v", panel.Err())
	}
	if got := panel.AppliedSettings(); !got.Equal(committed) {
		t.Fatalf("applied = %+v, want the committed %+v: the settings are durable", got, committed)
	}

	// The withdrawn refresh still reports eventually, and must land nowhere.
	m = djDeliver(m, cmd())
	panel = djPanel(t, m)
	if panel.Busy() || panel.Err() != nil {
		t.Fatalf("a late result reopened the commit: busy=%v err=%v", panel.Busy(), panel.Err())
	}
	if got := panel.Draft(); !got.Equal(committed) {
		t.Fatalf("draft = %+v, want it untouched by a late result", got)
	}
}

// TestAutoDJTuneIgnoresASupersededAnswer: the panel refuses to commit again
// while one is in flight, so a second Apply cannot outrun the first.
func TestAutoDJTuneIgnoresASupersededAnswer(t *testing.T) {
	m, _ := djRefreshModel(djDefaultQueue(), newDJReplacementWriter())
	m, _ = djPress(t, m, "ctrl+a")

	m, first := djPress(t, m, "down", "right", "ctrl+s")
	if first == nil {
		t.Fatal("the first Apply launched no refresh")
	}

	m, second := djPress(t, m, "right", "ctrl+s")
	if second != nil {
		t.Fatal("a second Apply was issued while the first was still running")
	}
	panel := djPanel(t, m)
	if !panel.Busy() {
		t.Fatal("the panel stopped waiting for the commit in flight")
	}
	if !panel.Dirty() {
		t.Fatal("the edit made while applying was swallowed")
	}
	edited := panel.Draft()

	m = djDeliver(m, first())
	panel = djPanel(t, m)
	if panel.Busy() {
		t.Fatal("the refresh did not answer its own commit")
	}
	if got := panel.Draft(); !got.Equal(edited) {
		t.Fatalf("draft = %+v, want the edit made while applying (%+v)", got, edited)
	}
	// The commit carried the draft as it stood when ctrl+s was pressed, not the
	// edit that followed it.
	if got := panel.AppliedSettings(); got.Equal(edited) {
		t.Fatalf("applied = %+v, want the snapshot the commit was issued with", got)
	}
}

// TestAutoDJTuneRefusesAnUnresolvableFilter: the panel names the stale tag and
// offers the repair rather than widening what the DJ may play behind the user.
func TestAutoDJTuneRefusesAnUnresolvableFilter(t *testing.T) {
	m := newTunableModel()
	settings := radio.DefaultSettings()
	settings.Exclude.TagIDs = []int64{404}
	m.dj.settings = settings

	m, _ = djPress(t, m, "ctrl+a")
	m, cmd := djPress(t, m, "ctrl+s")

	if cmd != nil {
		t.Fatal("a draft with an unresolvable tag was committed")
	}
	if m.dj.active {
		t.Fatal("a refused commit started a session")
	}
	panel := djPanel(t, m)
	if panel.Err() == nil {
		t.Fatal("the refusal was not shown")
	}
	if got := panel.Unresolved(); len(got) != 1 || got[0] != 404 {
		t.Fatalf("unresolved = %v, want the stale tag named", got)
	}

	m, _ = djPress(t, m, "r")
	if got := djPanel(t, m).Unresolved(); len(got) != 0 {
		t.Fatalf("unresolved = %v after the repair, want none", got)
	}

	m, _ = djPress(t, m, "ctrl+s")
	if !m.dj.active {
		t.Fatal("the repaired draft was still refused")
	}
	if got := m.dj.settings.Exclude.TagIDs; len(got) != 0 {
		t.Fatalf("exclude tags = %v, want the repair to have dropped them", got)
	}
}

// TestAutoDJPanelStatusExplainsTheSession: the panel reads facts it cannot edit
// from the root, so a refresh that replaced nothing is still accounted for.
func TestAutoDJPanelStatusExplainsTheSession(t *testing.T) {
	writer := newDJReplacementWriter()
	m, _ := djRefreshModel(djDefaultQueue(), writer)
	m, msg := runDJRefresh(t, m, exploreSettings())
	m, _ = m.handleDJRefreshDoneMsg(msg)

	lines := m.autoDJPanelStatus()
	if len(lines) == 0 {
		t.Fatal("a completed refresh explained nothing")
	}
	if !strings.Contains(lines[0].Text, "Last refresh") {
		t.Fatalf("status = %q, want the last refresh reported", lines[0].Text)
	}
}

// TestAutoDJTunePanelFitsTheMinimumTerminal walks every row at 60x15, which is
// the smallest size Muse renders at all.
func TestAutoDJTunePanelFitsTheMinimumTerminal(t *testing.T) {
	m := newTunableModel()
	m.termWidth, m.termHeight = MinTerminalWidth, MinTerminalHeight
	m, _ = djPress(t, m, "ctrl+a")

	// The row the cursor is on has to be drawn at every stop, and the box has to
	// stay inside the 60 columns, all the way down the list and back to the top.
	seen := map[string]bool{}
	for step := range len(tuneRowNames) * 2 {
		body, ok := m.renderOverlayBody()
		if !ok {
			t.Fatal("the Tune panel rendered no overlay body")
		}
		for i, line := range strings.Split(body, "\n") {
			if w := ansi.StringWidth(line); w > MinTerminalWidth {
				t.Fatalf("line %d is %d columns wide at the minimum size: %q", i, w, line)
			}
		}
		focused := tuneRowNames[djPanel(t, m).Cursor()]
		if !strings.Contains(ansi.Strip(body), focused) {
			t.Fatalf("step %d put the cursor on %q, which the panel did not draw:\n%s", step, focused, body)
		}
		seen[focused] = true
		m, _ = djPress(t, m, "down")
	}

	// Scrolling has to reach both ends of the list, not just its middle.
	for _, name := range []string{tuneRowNames[0], tuneRowNames[len(tuneRowNames)-1]} {
		if !seen[name] {
			t.Fatalf("the %q row was never reachable at the minimum size (saw %v)", name, seen)
		}
	}
}

// tuneRowNames is what each row of the panel draws, in the order the cursor
// walks them. The panel's own row enum is unexported, so the root model names
// them the way a user reading the screen would.
var tuneRowNames = []string{
	"Preset", "Discovery", "Anchor",
	"Include tags", "Include genres", "Include artists",
	"Exclude tags", "Exclude genres", "Exclude artists",
	"[ Start ]",
}

// TestAutoDJTuneIsOnTheCommandPalette keeps the gesture discoverable for a user
// who never reads the help.
func TestAutoDJTuneIsOnTheCommandPalette(t *testing.T) {
	m := newTunableModel()
	var found bool
	for _, cmd := range m.BuildPaletteCommands() {
		if cmd.ID == "autodj.tune" {
			found = true
			if cmd.Name != "Tune Auto-DJ…" {
				t.Fatalf("palette entry reads %q", cmd.Name)
			}
		}
	}
	if !found {
		t.Fatal("the palette offers no Tune Auto-DJ command")
	}
	if !IsSafeDirectCommand("autodj.tune") {
		t.Fatal("Tune Auto-DJ is not fixture-safe, so the harness cannot open it")
	}
}
