package autodj

import (
	"errors"
	"strings"
	"testing"

	"github.com/sglavoie/muse/internal/radio"
)

// testTagNames is the catalog the reason phrasing resolves tag ids against. Tag
// 3 is deliberately absent from the settings fixture and present here; tag 9 is
// in neither, which is how a deleted tag reaches the renderer.
func testTagNames() map[int64]string {
	return map[int64]string{1: "chill", 2: "focus", 3: "loud"}
}

// previewText renders a snapshot's logical lines as one plain string.
func previewText(v Preview) string {
	var b strings.Builder
	for _, l := range v.previewLines(testTagNames()) {
		b.WriteString(strings.Repeat(" ", l.indent))
		b.WriteString(l.text)
		b.WriteString("\n")
	}
	return b.String()
}

// assertContains fails with the whole page, which is what a reader would have to
// look at to judge the wording anyway.
func assertContains(t *testing.T, page string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(page, w) {
			t.Fatalf("page is missing %q:\n%s", w, page)
		}
	}
}

func assertAbsent(t *testing.T, page string, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if strings.Contains(page, u) {
			t.Fatalf("page contains %q, which the evidence does not support:\n%s", u, page)
		}
	}
}

// readyPreview is the ordinary case: a full batch under the default split, with
// nothing queued ahead of it.
func readyPreview() Preview {
	return Preview{
		Status:          PreviewReady,
		Settings:        radio.Settings{DiscoveryPercent: 40, Anchor: radio.AnchorSeed},
		Outcome:         radio.PlanOutcomeOK,
		DiscoveryTarget: 2,
		Tracks: []PreviewTrack{
			{Label: "Amy – One", Discovery: radio.DiscoveryNever, Reasons: radio.Reasons{
				{Code: radio.ReasonDiscoveryNever},
				{Code: radio.ReasonIncludeGenre, Value: "jazz"},
			}},
			{Label: "Amy – Two", Discovery: radio.DiscoveryStale, Reasons: radio.Reasons{
				{Code: radio.ReasonDiscoveryStale},
			}},
			{Label: "Bo – Three", Discovery: radio.DiscoveryFamiliar, Reasons: radio.Reasons{
				{Code: radio.ReasonFamiliar},
				{Code: radio.ReasonIncludeTag, Value: "1"},
			}},
			{Label: "Bo – Four", Discovery: radio.DiscoveryFamiliar, Reasons: radio.Reasons{
				{Code: radio.ReasonFamiliar},
			}},
			{Label: "Cy – Five", Discovery: radio.DiscoveryFamiliar, Reasons: radio.Reasons{
				{Code: radio.ReasonFamiliar},
			}},
		},
	}
}

func TestPreviewLabelsEverySelectionAndItsEvidence(t *testing.T) {
	page := previewText(readyPreview())

	assertContains(t, page,
		"Draft batch · Balanced · 40% discovery",
		"Would add 5 tracks · 2 discovery, 3 familiar (target 2 discovery):",
		"1. Amy – One  [discovery]",
		"no counted plays recorded; matches included genre “jazz”",
		"2. Amy – Two  [discovery]",
		"last counted play at least 90 days ago",
		"3. Bo – Three  [familiar]",
		"counted play within the last 90 days; carries included tag “chill”",
	)
}

// The footnote is the one statement that must never be dropped: everything the
// page says about play data is only true because of it.
func TestPreviewAlwaysStatesWhatPlayDataMeans(t *testing.T) {
	for name, v := range map[string]Preview{
		"ready":  readyPreview(),
		"empty":  emptyPreview(),
		"narrow": narrowPreview(),
	} {
		if page := previewText(v); !strings.Contains(page,
			"Play data counts finished listens only; skipped plays are not recorded.") {
			t.Fatalf("%s: page does not say what a counted play is:\n%s", name, page)
		}
	}
}

// A track with no counted plays is a track the DJ has no play evidence for. The
// wording must not promote that absence into a claim about the listener, and it
// must not describe a skip as something the ranking saw — the only sentence on
// the page that mentions skips is the one saying they are not recorded.
func TestPreviewNeverTurnsMissingEvidenceIntoAJudgement(t *testing.T) {
	page := previewText(readyPreview())
	assertAbsent(t, page,
		"never listened", "never played", "you skipped", "often skipped",
		"skip rate", "disliked", "you don't like")

	if n := strings.Count(page, "skip"); n != 1 {
		t.Fatalf("page mentions skipping %d times, want only the footnote:\n%s", n, page)
	}
}

func TestPreviewExplainsWhatPlaysBeforeTheBatch(t *testing.T) {
	v := readyPreview()
	v.Ahead = []QueuedTrack{
		{Label: "Amy – Now", Playing: true},
		{Label: "Bo – Manual"},
		{Label: "Cy – DJ", DJ: true, Reasons: radio.Reasons{{Code: radio.ReasonDiscoveryNever}}},
	}
	v.AheadMore = 4

	page := previewText(v)
	assertContains(t, page,
		"7 queued tracks play before this batch:",
		"▶ Amy – Now · added by you",
		"· Bo – Manual · added by you",
		"· Cy – DJ · Auto-DJ",
		"chosen because: no counted plays recorded",
		"and 4 more queued tracks",
	)
}

func TestPreviewSaysWhenNothingPlaysFirst(t *testing.T) {
	assertContains(t, previewText(readyPreview()),
		"Nothing is queued ahead — this batch would play next.")
}

// A DJ row written before provenance existed has no stored explanation. Saying
// so is the honest answer; re-deriving one against today's library would report
// what the DJ thinks now, not what it acted on then.
func TestPreviewDistinguishesAnUnrecordedExplanationFromNone(t *testing.T) {
	v := readyPreview()
	v.Ahead = []QueuedTrack{{Label: "Old – Row", DJ: true}}

	assertContains(t, previewText(v),
		"chosen before explanations were recorded — no stored evidence")
}

func TestPreviewReportsUnavailableEvidenceAsUnavailable(t *testing.T) {
	v := readyPreview()
	v.Gaps = []EvidenceGap{{Signal: "completion rates", Detail: "play events unavailable"}}

	page := previewText(v)
	assertContains(t, page,
		"Ranked without completion rates — that evidence was unavailable: play events unavailable")
	assertAbsent(t, page, "low completion", "poor completion")
}

// emptyPreview is a plan that ran and selected nothing, which is a result and
// not a failure.
func emptyPreview() Preview {
	return Preview{
		Status:   PreviewReady,
		Settings: radio.Settings{DiscoveryPercent: 40},
		Outcome:  radio.PlanOutcomeEmpty,
		Reasons:  radio.Reasons{{Code: radio.ReasonNoCandidates}},
		Rejections: radio.Rejections{
			{Code: radio.ReasonExcludeArtist, Value: "Zoe", Count: 120},
			{Code: radio.ReasonMissingGenre, Count: 80},
			{Code: radio.ReasonAlreadyQueued, Count: 9},
			{Code: radio.ReasonMissingTag, Count: 4},
			{Code: radio.ReasonExcludeTag, Value: "9", Count: 1},
		},
	}
}

func TestPreviewExplainsAnEmptyResultWithoutCallingItAFailure(t *testing.T) {
	page := previewText(emptyPreview())

	assertContains(t, page,
		"No tracks selected.",
		"no candidate matched these settings",
		"214 candidates were ruled out:",
		"120 × matches excluded artist “Zoe”",
		"80 × no genre matching the inclusion list",
		"9 × already playing or still queued",
		"and 2 more reasons",
	)
	assertAbsent(t, page, "Could not plan")
}

// narrowPreview is a filter set so tight the batch came back short.
func narrowPreview() Preview {
	v := readyPreview()
	v.Tracks = v.Tracks[:2]
	v.Outcome = radio.PlanOutcomeRelaxed
	v.DiscoveryTarget = 2
	v.Shortages = radio.Shortages{{Kind: radio.ShortageBatch, Want: 5, Got: 2}}
	v.Rejections = radio.Rejections{{Code: radio.ReasonMissingTag, Count: 300}}
	return v
}

func TestPreviewReportsAShortBatchAsACount(t *testing.T) {
	page := previewText(narrowPreview())

	assertContains(t, page,
		"Would add 2 tracks · 2 discovery, 0 familiar (target 2 discovery):",
		"Only 2 of 5 tracks could be selected from the eligible candidates.",
		"300 candidates were ruled out:",
		"300 × carries none of the included tags",
	)
}

func TestPreviewReportsARelaxedDiscoveryQuota(t *testing.T) {
	v := readyPreview()
	v.DiscoveryTarget = 4
	v.Shortages = radio.Shortages{{Kind: radio.ShortageDiscovery, Want: 4, Got: 2}}

	assertContains(t, previewText(v),
		"Only 2 of 4 discovery tracks were available. Added 2 extra familiar tracks instead.")
}

func TestPreviewSeparatesAFailureFromAnEmptyResult(t *testing.T) {
	v := Preview{
		Status:   PreviewFailed,
		Settings: radio.Settings{DiscoveryPercent: 40},
		Err:      errors.New("library index unavailable"),
	}

	page := previewText(v)
	assertContains(t, page, "Could not plan a batch: library index unavailable")
	assertAbsent(t, page, "No tracks selected.", "were ruled out")
}

func TestPreviewSaysWhenItHasNoDraftYet(t *testing.T) {
	assertContains(t, previewText(Preview{Status: PreviewIdle}),
		"No draft batch yet — press r to plan one.")
	assertContains(t, previewText(Preview{Status: PreviewPlanning}),
		"Planning a draft batch…")
}

// A page that is out of date has to say which of the two it is. Claiming a
// replan is under way when nothing is running would leave the user waiting for
// an update that is only ever going to arrive if they ask for it.
func TestPreviewSeparatesBeingStaleFromBeingReplanned(t *testing.T) {
	stale := readyPreview()
	stale.Stale = true
	page := previewText(stale)
	assertContains(t, page, "inputs moved — press r to replan")
	assertAbsent(t, page, "replanning")

	running := stale
	running.Replanning = true
	assertContains(t, previewText(running), "inputs moved, replanning")
}

// A tag the catalog no longer holds is shown as the id it was chosen under. The
// alternative — hiding it, or printing a bare number as if it were a name —
// would misdescribe the evidence the row actually carries.
func TestPreviewNamesADeletedTagAsDeleted(t *testing.T) {
	v := readyPreview()
	v.Tracks = []PreviewTrack{{Label: "Amy – One", Reasons: radio.Reasons{
		{Code: radio.ReasonIncludeTag, Value: "9"},
	}}}

	assertContains(t, previewText(v), "carries included tag #9 (deleted)")
}

func TestPreviewRendersAnUnexplainedSelectionHonestly(t *testing.T) {
	v := readyPreview()
	v.Tracks = []PreviewTrack{{Label: "Amy – One", Discovery: radio.DiscoveryFamiliar}}

	assertContains(t, previewText(v), "no evidence recorded")
}

func TestPreviewShortageDoesNotInventSubstitutions(t *testing.T) {
	for _, tc := range []struct {
		name                                          string
		discovery, familiar, wantDiscovery, wantBatch int
		extra                                         string
	}{
		{"one requested", 0, 0, 1, 1, ""},
		{"only discovery", 1, 0, 2, 5, ""},
		{"both short", 1, 1, 2, 5, ""},
		{"partial substitution", 1, 4, 4, 6, "Added 2 extra familiar tracks instead."},
		{"familiar substitution", 4, 0, 2, 5, "Added 2 extra discovery tracks instead."},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := Preview{Status: PreviewReady, DiscoveryTarget: tc.wantDiscovery}
			for range tc.discovery {
				v.Tracks = append(v.Tracks, PreviewTrack{Discovery: radio.DiscoveryNever})
			}
			for range tc.familiar {
				v.Tracks = append(v.Tracks, PreviewTrack{Discovery: radio.DiscoveryFamiliar})
			}
			v.Shortages = radio.Shortages{{Kind: radio.ShortageBatch, Want: tc.wantBatch, Got: len(v.Tracks)}}
			if tc.discovery < tc.wantDiscovery {
				v.Shortages = append(v.Shortages, radio.Shortage{Kind: radio.ShortageDiscovery, Want: tc.wantDiscovery, Got: tc.discovery})
			}
			if tc.familiar < tc.wantBatch-tc.wantDiscovery {
				v.Shortages = append(v.Shortages, radio.Shortage{Kind: radio.ShortageFamiliar, Want: tc.wantBatch - tc.wantDiscovery, Got: tc.familiar})
			}
			page := previewText(v)
			assertAbsent(t, page, "filled the rest")
			if tc.wantDiscovery == 1 {
				assertContains(t, page, "Only 0 of 1 discovery track was available.")
			}
			if tc.extra == "" {
				assertAbsent(t, page, "instead")
			} else {
				assertContains(t, page, tc.extra)
			}
			pinned, _ := v.presentationLines(nil, 56)
			if len(v.Tracks) == 1 {
				if len(pinned) == 0 || !strings.Contains(pinned[0].Text, "Would add 1 track ·") {
					t.Fatalf("singular summary: %+v", pinned)
				}
			}
		})
	}
}
