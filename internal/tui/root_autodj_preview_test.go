package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/playback"
	"github.com/sglavoie/muse/internal/tui/queue"
)

// previewModel is a running session with the Tune panel open on the Preview tab
// and a draft plan already delivered, which is the state every assertion below
// starts from.
func previewModel(t *testing.T, writer *stubDJQueueWriter) Model {
	t.Helper()
	m := djRefillModel(writer)
	m, _ = m.openAutoDJTunePanel()
	m, cmd := djPress(t, m, "tab")
	if !djPanel(t, m).PreviewActive() {
		t.Fatal("tab did not reach the Preview page")
	}
	if cmd == nil {
		t.Fatal("arriving on the Preview page asked for no draft plan")
	}
	msg, ok := cmd().(djPreviewPlannedMsg)
	if !ok {
		t.Fatalf("the preview command produced %T, want a draft plan", cmd())
	}
	m = djDeliver(m, msg)
	if !m.djPreviewResult().valid {
		t.Fatal("the draft plan was not filed")
	}
	return m
}

// TestAutoDJPreviewTabReadsWithoutConsequence is the tab's whole contract, stated
// against the root model rather than the component: opening it, scrolling it and
// replanning from it may not move playback, the queue, the history, the
// allocation, or the separate audio-preview player.
func TestAutoDJPreviewTabReadsWithoutConsequence(t *testing.T) {
	writer := &stubDJQueueWriter{}
	m := djRefillModel(writer)
	m = m.radioNoteRecent("already-heard")
	m.dj.allocation = radio.Allocation{CarryUnits: 3}
	before := m

	m, _ = m.openAutoDJTunePanel()
	m, cmd := djPress(t, m, "tab")
	if cmd != nil {
		m = djDeliver(m, cmd())
	}
	m, _ = djPress(t, m, "j", "j", "k", "r", "shift+tab", "tab")

	if len(writer.prepared) != 0 || len(writer.accepted) != 0 {
		t.Errorf("reading the Preview tab wrote queue rows: prepared=%v accepted=%v",
			writer.prepared, writer.accepted)
	}
	if len(m.queue.Entries) != len(before.queue.Entries) {
		t.Errorf("the queue holds %d entries, want the %d it started with",
			len(m.queue.Entries), len(before.queue.Entries))
	}
	if m.playback.currentHash != before.playback.currentHash ||
		m.playback.currentIndex != before.playback.currentIndex ||
		m.playback.state != before.playback.state {
		t.Errorf("playback moved: %+v", m.playback)
	}
	if m.dj.allocation != before.dj.allocation {
		t.Errorf("the discovery carry moved to %v, want %v", m.dj.allocation, before.dj.allocation)
	}
	if len(m.radioRecentSet()) != len(before.radioRecentSet()) {
		t.Errorf("the recent-selection ring moved: %v", m.radioRecentSet())
	}
	if m.radio.requestID != before.radio.requestID || m.radio.enqueuePending != before.radio.enqueuePending {
		t.Error("reading the Preview tab moved the live refill identity")
	}
	if m.overlay.preview != nil || m.overlay.previewPlayer != nil {
		t.Error("the DJ preview reached the audio-preview player, which is a separate thing")
	}
}

// A held draft is reused while its inputs hold. Re-planning per gesture would
// mean a full candidate scan for every keypress on a page that cannot change.
func TestAutoDJPreviewReusesAValidDraft(t *testing.T) {
	m := previewModel(t, &stubDJQueueWriter{})
	draft := m.overlay.autoDJ.Draft()

	if _, cmd := m.ensureAutoDJPreview(draft, time.Now()); cmd != nil {
		t.Fatal("a valid draft was replanned instead of reused")
	}
	if _, cmd := djPress(t, m, "r"); cmd != nil {
		t.Fatal("an explicit replan of unchanged inputs did work the held draft already covers")
	}
}

// Every input the plan depends on must retire the held draft. These are the same
// five the live refill's staleness gate reads, which is the point: a preview the
// user applies has to be the batch they were shown.
func TestAutoDJPreviewRegeneratesWhenInputsMove(t *testing.T) {
	base := previewModel(t, &stubDJQueueWriter{})
	draft := base.overlay.autoDJ.Draft()
	now := time.Now()

	tuned := draft
	tuned.DiscoveryPercent = 80

	cases := map[string]struct {
		settings radio.Settings
		mutate   func(Model) Model
		at       time.Time
	}{
		"settings edited": {settings: tuned, at: now},
		"library rescanned": {settings: draft, at: now, mutate: func(m Model) Model {
			m.djData.dataRev++
			return m
		}},
		"queue changed": {settings: draft, at: now, mutate: func(m Model) Model {
			m.queue.Entries = append(m.queue.Entries,
				queue.Entry{ID: 99, Hash: "new", Path: "/new.flac"})
			return m
		}},
		"seed moved": {settings: draft, at: now, mutate: func(m Model) Model {
			m.dj.seed = radio.SeedSnapshot{}
			m.radio.seedHash = "somewhere-else"
			return m
		}},
		"hour rolled over": {settings: draft, at: now.Add(time.Hour)},
	}

	for name, tc := range cases {
		m := base
		if tc.mutate != nil {
			m = tc.mutate(m)
		}
		if m.djPreviewFresh(tc.settings, tc.at) {
			t.Errorf("%s: the held draft still reports itself fresh", name)
			continue
		}
		if _, cmd := m.ensureAutoDJPreview(tc.settings, tc.at); cmd == nil {
			t.Errorf("%s: no replan was launched", name)
		}
	}
}

// A second gesture arriving while the same draft is already being planned must
// not start a second identical plan.
func TestAutoDJPreviewDoesNotDuplicateAnInFlightPlan(t *testing.T) {
	m := djRefillModel(&stubDJQueueWriter{})
	m, _ = m.openAutoDJTunePanel()
	draft := m.overlay.autoDJ.Draft()

	m, first := m.ensureAutoDJPreview(draft, time.Now())
	if first == nil {
		t.Fatal("the first request launched nothing")
	}
	if _, second := m.ensureAutoDJPreview(draft, time.Now()); second != nil {
		t.Fatal("a second request duplicated the plan already in flight")
	}
}

// Escape discards the draft, so it must discard the plan computed for it too —
// otherwise the next visit opens on a batch planned for settings the user
// already abandoned.
func TestClosingTheAutoDJPanelDropsTheHeldDraft(t *testing.T) {
	m := previewModel(t, &stubDJQueueWriter{})

	m, _ = djPress(t, m, "esc")
	if m.overlay.autoDJ != nil {
		t.Fatal("escape did not close the panel")
	}
	if m.djPreviewResult().valid || m.djPlan.previewPending {
		t.Fatalf("the draft plan outlived the panel: %+v", m.djPreviewResult())
	}
}

// A late answer to a superseded request must land nowhere.
func TestAutoDJPreviewIgnoresALateDraft(t *testing.T) {
	m := djRefillModel(&stubDJQueueWriter{})
	m, _ = m.openAutoDJTunePanel()

	m, cmd := m.ensureAutoDJPreview(m.overlay.autoDJ.Draft(), time.Now())
	late, ok := cmd().(djPreviewPlannedMsg)
	if !ok {
		t.Fatalf("preview command produced %T", cmd())
	}
	m = m.closeAutoDJPanel()
	m = m.handleDJPreviewPlannedMsg(late)

	if m.djPreviewResult().valid {
		t.Fatal("a draft answered after the panel closed was filed anyway")
	}
}

// The snapshot the tab renders has to describe the real listening order, and has
// to read a committed row's explanation back from what was stored with it.
func TestAutoDJPreviewSnapshotDescribesWhatPlaysFirst(t *testing.T) {
	m := previewModel(t, &stubDJQueueWriter{})
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "f1", Path: "/f1.flac", Artist: "Alice", Title: "F1"},
		{ID: 2, Hash: "m2", Path: "/m2.flac", Artist: "Bob", Title: "Manual"},
		{ID: 3, Hash: "d3", Path: "/d3.flac", Artist: "Cy", Title: "DJ Pick",
			DJOwned: true, DJReasons: radio.Reasons{{Code: radio.ReasonDiscoveryNever}}},
	}
	m.playback.currentIndex = 0
	m.playback.currentHash = "f1"
	m.playback.state = playback.StatePlaying

	v := m.autoDJPreviewFor(m.overlay.autoDJ.Draft(), time.Now())
	if len(v.Ahead) != 3 || v.AheadMore != 0 {
		t.Fatalf("ahead = %+v (+%d), want all three committed rows", v.Ahead, v.AheadMore)
	}
	if !v.Ahead[0].Playing || v.Ahead[0].Label == "" {
		t.Errorf("first row = %+v, want the track playing right now", v.Ahead[0])
	}
	if v.Ahead[1].DJ {
		t.Errorf("row %q is claimed for the DJ, but the user added it", v.Ahead[1].Label)
	}
	if !v.Ahead[2].DJ || len(v.Ahead[2].Reasons) != 1 ||
		v.Ahead[2].Reasons[0].Code != radio.ReasonDiscoveryNever {
		t.Errorf("DJ row = %+v, want the explanation persisted with it", v.Ahead[2])
	}
}

// A long queue is reported as long. Trimming it silently would understate how
// far away the batch is, which is the one thing this list exists to say.
func TestAutoDJPreviewSnapshotReportsWhatItLeftOut(t *testing.T) {
	m := previewModel(t, &stubDJQueueWriter{})
	entries := make([]queue.Entry, 0, djPreviewAheadCap+5)
	for i := range djPreviewAheadCap + 5 {
		entries = append(entries, queue.Entry{ID: int64(i + 1), Path: "/q.flac", Title: "Q"})
	}
	m.queue.Entries = entries
	m.playback.currentIndex = 0

	v := m.autoDJPreviewFor(m.overlay.autoDJ.Draft(), time.Now())
	if len(v.Ahead) != djPreviewAheadCap || v.AheadMore != 5 {
		t.Fatalf("ahead = %d rows (+%d), want %d and the remaining 5 reported",
			len(v.Ahead), v.AheadMore, djPreviewAheadCap)
	}
}

// Rows already played are not ahead of anything.
func TestAutoDJPreviewSnapshotStartsAtTheCurrentRow(t *testing.T) {
	m := previewModel(t, &stubDJQueueWriter{})
	m.queue.Entries = []queue.Entry{
		{ID: 1, Hash: "a", Path: "/a.flac", Title: "Played"},
		{ID: 2, Hash: "b", Path: "/b.flac", Title: "Playing"},
		{ID: 3, Hash: "c", Path: "/c.flac", Title: "Next"},
	}
	m.playback.currentIndex = 1
	m.playback.currentHash = "b"

	v := m.autoDJPreviewFor(m.overlay.autoDJ.Draft(), time.Now())
	if len(v.Ahead) != 2 || v.Ahead[0].Label == "" {
		t.Fatalf("ahead = %+v, want the current row and what follows it", v.Ahead)
	}
	for _, q := range v.Ahead {
		if q.Label == "Played" {
			t.Error("a row already behind the player is listed as upcoming")
		}
	}
}

// A degraded ranking must say so. The reader has to be able to tell a track that
// scored badly from one whose evidence could not be read at all.
func TestAutoDJPreviewSnapshotNamesUnreadableEvidence(t *testing.T) {
	reader := newDJRecReader()
	reader.completionErr = errDJSignalUnavailable
	m := newDJModel(WithRecommendationReader(reader), WithDJQueueWriter(&stubDJQueueWriter{}))
	m.dj.active = true
	m.dj.seed = djTestRequest().Seed
	m, _ = m.openAutoDJTunePanel()

	m, cmd := m.ensureAutoDJPreview(m.overlay.autoDJ.Draft(), time.Now())
	if cmd == nil {
		t.Fatal("no preview was launched")
	}
	m = m.handleDJPreviewPlannedMsg(cmd().(djPreviewPlannedMsg))

	v := m.autoDJPreviewFor(m.overlay.autoDJ.Draft(), time.Now())
	if len(v.Gaps) != 1 {
		t.Fatalf("gaps = %+v, want the one signal that could not be read", v.Gaps)
	}
	if v.Gaps[0].Signal != "completion rates" ||
		!strings.Contains(v.Gaps[0].Detail, errDJSignalUnavailable.Error()) {
		t.Fatalf("gap = %+v, want it to name the signal and what went wrong", v.Gaps[0])
	}
}

// A reader that was never injected is not a gap: there was no evidence to lose,
// and reporting one would tell the user something went missing that never
// existed.
func TestAutoDJPreviewSnapshotReportsNoGapWithoutAReader(t *testing.T) {
	m := newDJModel(WithDJQueueWriter(&stubDJQueueWriter{}))
	m.dj.active = true
	m.dj.seed = djTestRequest().Seed
	m, _ = m.openAutoDJTunePanel()

	m, cmd := m.ensureAutoDJPreview(m.overlay.autoDJ.Draft(), time.Now())
	if cmd == nil {
		t.Fatal("no preview was launched")
	}
	m = m.handleDJPreviewPlannedMsg(cmd().(djPreviewPlannedMsg))

	if gaps := m.autoDJPreviewFor(m.overlay.autoDJ.Draft(), time.Now()).Gaps; len(gaps) != 0 {
		t.Fatalf("gaps = %+v, want none when no reader was wired at all", gaps)
	}
}

var errDJSignalUnavailable = errors.New("play events unavailable")

func TestAutoDJPreviewTabIsItsOwnOverlaySnapshot(t *testing.T) {
	m := previewModel(t, &stubDJQueueWriter{})
	if got := m.Snapshot().Overlay; got != "autodj_preview" {
		t.Fatalf("overlay snapshot = %q, want autodj_preview", got)
	}

	m, _ = djPress(t, m, "tab")
	if got := m.Snapshot().Overlay; got != "autodj_tune" {
		t.Fatalf("overlay snapshot = %q, want autodj_tune after leaving the page", got)
	}
}

// Applying from the Preview page goes through the same commit the Tune page
// does — it is the only gesture on the page that is allowed to change anything.
func TestApplyFromThePreviewTabStillCommits(t *testing.T) {
	m := previewModel(t, &stubDJQueueWriter{})
	m, _ = djPress(t, m, "shift+tab")
	m, _ = djPress(t, m, "down", "right")
	m, _ = djPress(t, m, "tab")
	if cmd := m.overlay.autoDJ; cmd == nil || !cmd.PreviewActive() {
		t.Fatal("the panel is not on the Preview page")
	}

	before := m.dj.settings
	m, _ = djPress(t, m, "ctrl+s")
	if m.dj.settings.Equal(before) {
		t.Fatalf("ctrl+s from the Preview page applied nothing: %+v", m.dj.settings)
	}
	// The proposal was acted on, so it is no longer a proposal. Leaving it up
	// would keep offering tracks the refill may have just enqueued.
	if m.djPreviewResult().valid {
		t.Fatal("the committed draft is still displayed as something that would be added")
	}
}

// The rendered overlay is the only thing a user sees, so the wiring from the
// root's snapshot through to the page has to be exercised end to end.
func TestAutoDJPreviewRendersTheDraftBatch(t *testing.T) {
	m := previewModel(t, &stubDJQueueWriter{})
	m.termWidth, m.termHeight = 100, 40

	body, ok := m.renderOverlayBody()
	if !ok {
		t.Fatal("no overlay body was rendered")
	}
	page := ansi.Strip(body)
	for _, want := range []string{"Auto-DJ", "Preview", "Would add",
		"Play data counts finished listens only"} {
		if !strings.Contains(page, want) {
			t.Fatalf("the rendered Preview page is missing %q:\n%s", want, page)
		}
	}
}
