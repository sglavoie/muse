package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/radio"
)

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
