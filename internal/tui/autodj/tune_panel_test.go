package autodj

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/radio"
)

// testCatalog is the fixture every picker test picks from.
func testCatalog() Catalog {
	return Catalog{
		Tags: []db.TagRow{
			{ID: 1, Name: "chill", Color: "blue"},
			{ID: 2, Name: "focus", Color: "green"},
			{ID: 3, Name: "loud"},
		},
		Genres:  []string{"Rock", "jazz", "Ambient"},
		Artists: []string{"Zoe", "amy"},
	}
}

func testSettings() radio.Settings {
	return radio.Settings{
		DiscoveryPercent: 40,
		Anchor:           radio.AnchorSeed,
		Include:          radio.FilterSet{TagIDs: []int64{1}, Genres: []string{"jazz"}},
		Exclude:          radio.FilterSet{Artists: []string{"Zoe"}},
	}
}

func newTestPanel(active bool) TunePanel {
	return New(Options{Applied: testSettings(), Catalog: testCatalog(), Active: active})
}

// keyMsg builds the key press whose String() is key, so the tests speak the
// same key names the panel switches on.
func keyMsg(key string) tea.KeyPressMsg {
	named := map[string]rune{
		"esc": tea.KeyEscape, "enter": tea.KeyEnter, "tab": tea.KeyTab,
		"backspace": tea.KeyBackspace, "space": tea.KeySpace,
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

// press feeds key names through HandleKey the way the root model does.
func press(t *testing.T, p TunePanel, keys ...string) (TunePanel, Action) {
	t.Helper()
	action := actionNone
	for _, key := range keys {
		msg := keyMsg(key)
		if got := msg.String(); got != key {
			t.Fatalf("keyMsg(%q).String() = %q: the fixture does not produce that key", key, got)
		}
		p, action = p.HandleKey(msg)
	}
	return p, action
}

func focus(t *testing.T, p TunePanel, r row) TunePanel {
	t.Helper()
	p.cursor = int(r)
	return p
}

func TestNewDraftIsIsolatedFromApplied(t *testing.T) {
	applied := testSettings()
	p := New(Options{Applied: applied, Catalog: testCatalog()})

	// Mutating the caller's slices must not reach the panel.
	applied.Include.Genres[0] = "polka"
	applied.Include.TagIDs[0] = 99

	if got := p.Draft().Include.Genres[0]; got != "jazz" {
		t.Fatalf("draft genre = %q, want the snapshot taken at open", got)
	}
	if got := p.AppliedSettings().Include.TagIDs[0]; got != 1 {
		t.Fatalf("applied tag id = %d, want 1", got)
	}
	if p.Dirty() {
		t.Fatal("a freshly opened panel is dirty")
	}
}

func TestDraftReturnsADeepCopy(t *testing.T) {
	p := newTestPanel(false)

	draft := p.Draft()
	draft.Include.Genres[0] = "polka"
	draft.Include.TagIDs = append(draft.Include.TagIDs, 7)

	if got := p.Draft().Include.Genres[0]; got != "jazz" {
		t.Fatalf("panel draft genre = %q, want jazz: Draft aliases panel state", got)
	}
	if got := len(p.Draft().Include.TagIDs); got != 1 {
		t.Fatalf("panel draft tag count = %d, want 1", got)
	}
}

func TestEditingTheDraftLeavesAppliedAlone(t *testing.T) {
	p := focus(t, newTestPanel(true), rowDiscovery)
	p, _ = press(t, p, "right")

	if got := p.Draft().DiscoveryPercent; got != 50 {
		t.Fatalf("draft discovery = %d, want 50", got)
	}
	if got := p.AppliedSettings().DiscoveryPercent; got != 40 {
		t.Fatalf("applied discovery = %d, want the untouched 40", got)
	}
	if !p.Dirty() {
		t.Fatal("an edited draft is not reported dirty")
	}
}

func TestPresetChangePreservesFiltersAndAnchor(t *testing.T) {
	p := newTestPanel(true)
	p.draft.Anchor = radio.AnchorFollow
	p = focus(t, p, rowPreset)

	p, _ = press(t, p, "right") // balanced -> explore

	draft := p.Draft()
	if got := draft.Preset(); got != radio.PresetExplore {
		t.Fatalf("preset = %q, want explore", got)
	}
	if got := draft.DiscoveryPercent; got != 80 {
		t.Fatalf("discovery = %d, want the explore target 80", got)
	}
	if got := draft.Anchor; got != radio.AnchorFollow {
		t.Fatalf("anchor = %q, want the preset change to leave it alone", got)
	}
	if len(draft.Include.TagIDs) != 1 || len(draft.Include.Genres) != 1 || len(draft.Exclude.Artists) != 1 {
		t.Fatalf("preset change lost filters: %+v", draft)
	}
}

func TestCustomTuningIsIdentifiedAsCustom(t *testing.T) {
	p := focus(t, newTestPanel(true), rowDiscovery)
	p, _ = press(t, p, "right")

	if got := p.Draft().Preset(); got != radio.PresetCustom {
		t.Fatalf("preset = %q, want custom after a hand-tuned target", got)
	}
	if got := p.rowValue(rowPreset); !strings.Contains(got, "Custom") {
		t.Fatalf("preset row = %q, want it to name Custom", got)
	}

	// Stepping off Custom re-enters the stock cycle from the end the direction
	// implies rather than staying stuck.
	p = focus(t, p, rowPreset)
	p, _ = press(t, p, "left")
	if got := p.Draft().Preset(); got == radio.PresetCustom {
		t.Fatal("preset stayed custom after stepping the preset row")
	}
}

func TestAnchorTogglesBetweenTheTwoModes(t *testing.T) {
	p := focus(t, newTestPanel(true), rowAnchor)

	p, _ = press(t, p, "right")
	if got := p.Draft().Anchor; got != radio.AnchorFollow {
		t.Fatalf("anchor = %q, want follow", got)
	}
	p, _ = press(t, p, "right")
	if got := p.Draft().Anchor; got != radio.AnchorSeed {
		t.Fatalf("anchor = %q, want seed", got)
	}
}

func TestCommitLabelFollowsSessionState(t *testing.T) {
	if got := newTestPanel(true).CommitLabel(); got != "Apply" {
		t.Fatalf("running session label = %q, want Apply", got)
	}
	if got := newTestPanel(false).CommitLabel(); got != "Start" {
		t.Fatalf("stopped session label = %q, want Start", got)
	}
	if got := newTestPanel(false).rowValue(rowCommit); got != "[ Start ]" {
		t.Fatalf("commit row = %q, want the Start button", got)
	}
}

func TestCommitAsksTheRootToApply(t *testing.T) {
	for _, key := range []string{"ctrl+s", "enter"} {
		p := focus(t, newTestPanel(true), rowCommit)
		p, action := press(t, p, key)
		if action != ActionCommit {
			t.Fatalf("%s on the commit row returned %v, want ActionCommit", key, action)
		}
		if p.Err() != nil {
			t.Fatalf("%s reported %v on a valid draft", key, p.Err())
		}
	}
}

func TestCommitIsRefusedWhileOneIsInFlight(t *testing.T) {
	p := newTestPanel(true).Submit(7)

	p, action := press(t, p, "ctrl+s")
	if action != actionNone {
		t.Fatalf("second commit returned %v, want it ignored", action)
	}
	if !p.Busy() {
		t.Fatal("panel stopped reporting the commit in flight")
	}
}

func TestCommitIsRefusedForAnInvalidDraft(t *testing.T) {
	p := newTestPanel(true)
	p.draft.Anchor = radio.AnchorMode("sideways")

	p, action := press(t, p, "ctrl+s")
	if action != actionNone {
		t.Fatalf("invalid draft returned %v, want no commit", action)
	}
	if !errors.Is(p.Err(), radio.ErrUnknownAnchorMode) {
		t.Fatalf("err = %v, want the anchor refusal", p.Err())
	}
}

func TestCancelAsksTheRootToClose(t *testing.T) {
	p := newTestPanel(true)
	p, action := press(t, p, "esc")
	if action != ActionCancel {
		t.Fatalf("esc returned %v, want ActionCancel", action)
	}
	if p.picker != nil {
		t.Fatal("esc opened a picker")
	}
}

func TestAckMovesAppliedWithoutTouchingTheDraft(t *testing.T) {
	p := focus(t, newTestPanel(true), rowDiscovery)
	committed := p.Draft()
	p = p.Submit(3)

	// The user keeps editing while the commit runs.
	p, _ = press(t, p, "right")
	edited := p.Draft()

	p = p.Ack(Applied{Token: 3, Settings: committed, Active: true})

	if p.Busy() {
		t.Fatal("panel is still busy after its acknowledgement")
	}
	if got := p.AppliedSettings().DiscoveryPercent; got != committed.DiscoveryPercent {
		t.Fatalf("applied discovery = %d, want the committed %d", got, committed.DiscoveryPercent)
	}
	if got := p.Draft().DiscoveryPercent; got != edited.DiscoveryPercent {
		t.Fatalf("draft discovery = %d, want the pending edit %d kept", got, edited.DiscoveryPercent)
	}
	if !p.Dirty() {
		t.Fatal("the edit made while the commit ran was swallowed by the acknowledgement")
	}
}

func TestAckIgnoresAStaleToken(t *testing.T) {
	p := focus(t, newTestPanel(true), rowDiscovery)
	p, _ = press(t, p, "right")
	p = p.Submit(9)
	before := p.AppliedSettings()

	stale := testSettings()
	stale.DiscoveryPercent = 0
	p = p.Ack(Applied{Token: 8, Settings: stale, Active: true})

	if !p.Busy() {
		t.Fatal("a stale acknowledgement cleared the commit in flight")
	}
	if got := p.AppliedSettings(); !got.Equal(before) {
		t.Fatalf("applied = %+v, want the untouched %+v", got, before)
	}
}

func TestAckOnAnIdlePanelIsIgnored(t *testing.T) {
	p := newTestPanel(true)
	other := testSettings()
	other.DiscoveryPercent = 0

	p = p.Ack(Applied{Token: 0, Settings: other, Active: false})

	if got := p.AppliedSettings().DiscoveryPercent; got != 40 {
		t.Fatalf("applied discovery = %d, want an idle panel to ignore the answer", got)
	}
	if !p.active {
		t.Fatal("an idle panel adopted a stale session state")
	}
}

func TestFailedAckKeepsTheAppliedBaseline(t *testing.T) {
	p := focus(t, newTestPanel(true), rowDiscovery)
	p, _ = press(t, p, "right")
	p = p.Submit(4)

	failed := errors.New("write settings: disk is full")
	p = p.Ack(Applied{Token: 4, Settings: testSettings(), Active: true, Err: failed})

	if p.Busy() {
		t.Fatal("a failed acknowledgement left the panel busy")
	}
	if !errors.Is(p.Err(), failed) {
		t.Fatalf("err = %v, want the failure surfaced", p.Err())
	}
	if got := p.AppliedSettings().DiscoveryPercent; got != 40 {
		t.Fatalf("applied discovery = %d, want the baseline that stayed in force", got)
	}
	if !p.Dirty() {
		t.Fatal("a refused commit stopped reporting unapplied changes")
	}
}

func TestAckAdoptsTheSessionState(t *testing.T) {
	p := newTestPanel(false)
	if got := p.CommitLabel(); got != "Start" {
		t.Fatalf("label = %q, want Start", got)
	}

	p = p.Submit(1).Ack(Applied{Token: 1, Settings: testSettings(), Active: true})

	if !p.active {
		t.Fatal("panel did not adopt the started session")
	}
	if got := p.CommitLabel(); got != "Apply" {
		t.Fatalf("label = %q, want Apply once a session runs", got)
	}
}

func TestUnresolvedTagsBlockTheCommitUntilRepaired(t *testing.T) {
	p := newTestPanel(true)
	p.draft.Include.TagIDs = []int64{1, 42}
	p.draft.Exclude.TagIDs = []int64{42, 7}

	if got := p.Unresolved(); len(got) != 2 || got[0] != 7 || got[1] != 42 {
		t.Fatalf("unresolved = %v, want [7 42] deduplicated and sorted", got)
	}

	p, action := press(t, p, "ctrl+s")
	if action != actionNone {
		t.Fatalf("commit returned %v, want it blocked by the unresolved tags", action)
	}
	var unresolved *radio.UnresolvedTagError
	if !errors.As(p.Err(), &unresolved) {
		t.Fatalf("err = %v, want an UnresolvedTagError", p.Err())
	}

	p, _ = press(t, p, "r")
	if got := p.Unresolved(); len(got) != 0 {
		t.Fatalf("unresolved = %v after the repair, want none", got)
	}
	if got := p.Draft().Include.TagIDs; len(got) != 1 || got[0] != 1 {
		t.Fatalf("include tags = %v, want the resolvable 1 kept", got)
	}
	if got := p.Draft().Exclude.TagIDs; got != nil {
		t.Fatalf("exclude tags = %v, want the unresolvable ones dropped", got)
	}
	if p.Err() != nil {
		t.Fatalf("err = %v, want the repair to clear the refusal", p.Err())
	}

	if _, action = press(t, p, "ctrl+s"); action != ActionCommit {
		t.Fatalf("commit after repair returned %v, want ActionCommit", action)
	}
}

func TestUnresolvedTagsAreNamedInTheRow(t *testing.T) {
	p := newTestPanel(true)
	p.draft.Include.TagIDs = []int64{1, 42}

	got := p.rowValue(rowIncludeTags)
	if !strings.Contains(got, "chill") || !strings.Contains(got, "#42 (missing)") {
		t.Fatalf("include tags row = %q, want the resolved name and the missing id", got)
	}
}

func TestClearEmptiesOnlyTheFocusedField(t *testing.T) {
	p := focus(t, newTestPanel(true), rowIncludeGenres)
	p, _ = press(t, p, "x")

	draft := p.Draft()
	if len(draft.Include.Genres) != 0 {
		t.Fatalf("include genres = %v, want cleared", draft.Include.Genres)
	}
	if len(draft.Include.TagIDs) != 1 {
		t.Fatalf("include tags = %v, want untouched", draft.Include.TagIDs)
	}
	if len(draft.Exclude.Artists) != 1 {
		t.Fatalf("exclude artists = %v, want untouched", draft.Exclude.Artists)
	}
	if got := p.rowValue(rowIncludeGenres); got != "any" {
		t.Fatalf("cleared row = %q, want any", got)
	}
}

func TestClearIgnoresNonFilterRows(t *testing.T) {
	p := focus(t, newTestPanel(true), rowDiscovery)
	p, _ = press(t, p, "x")
	if got := p.Draft().DiscoveryPercent; got != 40 {
		t.Fatalf("discovery = %d, want x to do nothing on a non-filter row", got)
	}
}

func TestCursorWrapsAroundTheRowList(t *testing.T) {
	p := newTestPanel(true)

	p, _ = press(t, p, "up")
	if got := p.Cursor(); got != int(rowCommit) {
		t.Fatalf("cursor = %d, want it wrapped to the commit row %d", got, rowCommit)
	}
	p, _ = press(t, p, "down")
	if got := p.Cursor(); got != 0 {
		t.Fatalf("cursor = %d, want it wrapped back to the first row", got)
	}
	p, _ = press(t, p, "j", "j")
	if got := p.Cursor(); got != int(rowAnchor) {
		t.Fatalf("cursor = %d, want j to move like down", got)
	}
	p, _ = press(t, p, "k")
	if got := p.Cursor(); got != int(rowDiscovery) {
		t.Fatalf("cursor = %d, want k to move like up", got)
	}
}
