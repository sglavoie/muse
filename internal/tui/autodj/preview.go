package autodj

import (
	"fmt"
	"strconv"

	"github.com/sglavoie/muse/internal/radio"
	"github.com/sglavoie/muse/internal/tui/text"
)

// This file holds the read-only half of the Auto-DJ overlay: the Preview tab.
//
// Everything the tab shows is supplied by the root model at render time, in the
// same way the status block is. That is deliberate. A preview is a plan the
// caller threw the allocation away from, and the tab exists to *read* one — so
// it keeps no copy that could drift from the session, and it owns no state
// beyond how far the user has scrolled. Opening it, switching to it, and
// scrolling through it are all pure.
//
// The other rule this file follows is that an explanation may only state what
// the evidence supports. Muse's play history counts finished listens, so a
// track with no counted plays is a track the DJ has no play evidence for — not
// one the user disliked or skipped. The wording below keeps that distinction,
// and reports missing evidence as missing rather than as a negative signal.

// panelTab identifies one page of the Auto-DJ overlay.
type panelTab int

const (
	// tabTune is the settings editor.
	tabTune panelTab = iota
	// tabPreview is the read-only draft batch.
	tabPreview
	// tabRecipes is the stored settings snapshots.
	tabRecipes
	tabCount
)

// tabLabels is the tab bar, in order.
var tabLabels = [tabCount]string{
	tabTune:    "Tune",
	tabPreview: "Preview",
	tabRecipes: "Recipes",
}

// PreviewStatus reports what the Preview tab currently holds.
type PreviewStatus int

const (
	// PreviewIdle means no draft has been asked for yet.
	PreviewIdle PreviewStatus = iota
	// PreviewPlanning means a draft is being computed off the update loop.
	PreviewPlanning
	// PreviewReady means a draft plan is held.
	PreviewReady
	// PreviewFailed means the draft could not be planned at all, which is a
	// different thing from a plan that legitimately selected nothing.
	PreviewFailed
)

// PreviewTrack is one proposed selection and the evidence behind it.
type PreviewTrack struct {
	// Label is the display name of the track.
	Label string
	// Discovery is the classification the plan selected it under.
	Discovery radio.DiscoveryClass
	// Reasons is the planner's structured explanation for this selection.
	Reasons radio.Reasons
}

// QueuedTrack is one committed queue row that plays before the draft batch.
//
// Reasons here is the explanation persisted with the row when it was chosen,
// never a fresh classification: re-deriving it against today's library would
// report what the DJ would think now rather than what it acted on then.
type QueuedTrack struct {
	// Label is the display name of the track.
	Label string
	// Playing marks the row playing right now.
	Playing bool
	// DJ reports that the Auto-DJ chose this row rather than the user.
	DJ bool
	// Reasons is the explanation stored with the row at selection time. Empty
	// on a DJ row means the explanation is unavailable, not that there was
	// none.
	Reasons radio.Reasons
}

// EvidenceGap names one optional signal that could not be read, so the tab can
// say the ranking ran on less evidence than the database holds instead of
// letting the user read a degraded ordering as a considered one.
type EvidenceGap struct {
	// Signal is the human-facing name of the query that failed.
	Signal string
	// Detail is the failure, or empty when there is nothing more to say.
	Detail string
}

// Preview is the whole snapshot the root model supplies for the Preview tab.
type Preview struct {
	// Status is what the tab holds right now.
	Status PreviewStatus
	// Stale reports that the inputs moved after this draft was planned, so it
	// no longer describes what applying these settings would select.
	Stale bool
	// Replanning reports that a replacement is actually in flight. It is kept
	// apart from Stale because the page must not claim work is under way when
	// it is only waiting for the user to ask for it.
	Replanning bool
	// Settings is the draft the batch was planned for.
	Settings radio.Settings
	// Outcome, DiscoveryTarget, Tracks, Shortages, Reasons and Rejections are
	// the planner's result, unmodified.
	Outcome         radio.PlanOutcome
	DiscoveryTarget int
	Tracks          []PreviewTrack
	Shortages       radio.Shortages
	Reasons         radio.Reasons
	Rejections      radio.Rejections
	// Ahead is the committed queue, in the order the player will reach it.
	Ahead []QueuedTrack
	// AheadMore is how many further committed rows Ahead left out, so a long
	// queue is reported as long rather than silently trimmed.
	AheadMore int
	// Gaps lists the optional evidence that was unavailable when this batch
	// was ranked.
	Gaps []EvidenceGap
	// Err is why planning failed outright.
	Err error
}

// maxRejectionLines bounds the "what stood in the way" tally so one heavily
// filtered library cannot push the batch off the page.
const maxRejectionLines = 3

// previewLine is one logical line of the page, before wrapping.
type previewLine struct {
	text   string
	indent int
	tone   Tone
}

// previewLines assembles the page in reading order: what the batch is for, what
// the proposed songs, qualifying evidence, then the existing queue.
func (v Preview) previewLines(names map[int64]string) []previewLine {
	lines := []previewLine{{text: v.headline(), tone: ToneInfo}}
	switch v.Status {
	case PreviewIdle:
		return append(lines, previewLine{
			text: "No draft batch yet — press r to plan one.", tone: ToneInfo})
	case PreviewPlanning:
		return append(lines, previewLine{text: "Planning a draft batch…", tone: ToneInfo})
	case PreviewFailed:
		return append(lines, previewLine{
			text: "Could not plan a batch: " + errText(v.Err), tone: ToneError})
	}
	lines = append(lines, v.batchLines(names)...)
	lines = append(lines, v.outcomeLines(names)...)
	lines = append(lines, v.aheadLines(names)...)
	return append(lines, v.evidenceLines()...)
}

// headline names the settings the batch was planned for, and says plainly when
// what is on screen no longer describes them.
func (v Preview) headline() string {
	head := fmt.Sprintf("Draft batch · %s · %d%% discovery",
		v.Settings.Preset().Label(), v.Settings.DiscoveryPercent)
	switch {
	case v.Replanning:
		return head + " · inputs moved, replanning"
	case v.Stale:
		return head + " · inputs moved — press r to replan"
	}
	return head
}

// aheadLines explains what the user hears before any of this batch.
//
// It is not decoration. The player owns traversal, the queue may hold manual
// entries the DJ will never touch, and a batch is appended behind all of it —
// so a preview presented as "next" without this would be a preview of something
// the user may not hear for an hour.
func (v Preview) aheadLines(names map[int64]string) []previewLine {
	if len(v.Ahead) == 0 {
		return []previewLine{{
			text: "Nothing is queued ahead — this batch would play next.", tone: ToneInfo}}
	}
	total := len(v.Ahead) + v.AheadMore
	lines := []previewLine{{
		text: fmt.Sprintf("%d queued %s %s before this batch:", total,
			text.Pluralize(total, "track", "tracks"),
			text.Pluralize(total, "plays", "play")),
		tone: ToneWarn,
	}}
	for _, q := range v.Ahead {
		lines = append(lines, previewLine{
			text: queuedMarker(q) + q.Label + " · " + queuedOwner(q), indent: 2, tone: ToneInfo})
		if q.DJ {
			lines = append(lines, previewLine{
				text: queuedEvidence(q, names), indent: 5, tone: ToneInfo})
		}
	}
	if v.AheadMore > 0 {
		lines = append(lines, previewLine{
			text:   fmt.Sprintf("and %d more queued %s", v.AheadMore, text.Pluralize(v.AheadMore, "track", "tracks")),
			indent: 2,
			tone:   ToneInfo,
		})
	}
	return lines
}

// queuedMarker points at the row playing right now.
func queuedMarker(q QueuedTrack) string {
	if q.Playing {
		return "▶ "
	}
	return "· "
}

// queuedOwner says who put the row in the queue, which is what decides whether
// applying new settings can replace it.
func queuedOwner(q QueuedTrack) string {
	if q.DJ {
		return "Auto-DJ"
	}
	return "added by you"
}

// queuedEvidence renders a committed row's stored explanation.
//
// A DJ row with no stored reasons is reported as unavailable rather than as an
// empty explanation: rows written before provenance existed, and rows whose
// stored explanation could not be decoded, both land here, and neither means
// the DJ chose the track for no reason.
func queuedEvidence(q QueuedTrack, names map[int64]string) string {
	if len(q.Reasons) == 0 {
		return "chosen before explanations were recorded — no stored evidence"
	}
	return "chosen because: " + reasonPhrases(q.Reasons, names)
}

// batchLines renders the proposal itself.
func (v Preview) batchLines(names map[int64]string) []previewLine {
	if len(v.Tracks) == 0 {
		return []previewLine{{text: "No tracks selected.", tone: ToneWarn}}
	}
	lines := []previewLine{{text: v.batchSummary(), tone: ToneInfo}}
	for i, t := range v.Tracks {
		lines = append(lines, previewLine{
			text:   fmt.Sprintf("%d. %s  [%s]", i+1, t.Label, discoveryLabel(t.Discovery)),
			indent: 2,
			tone:   ToneInfo,
		})
		lines = append(lines, previewLine{
			text: reasonPhrases(t.Reasons, names), indent: 5, tone: ToneInfo})
	}
	return lines
}

// batchSummary states what the batch achieved against what was asked for.
func (v Preview) batchSummary() string {
	discovery := 0
	for _, t := range v.Tracks {
		if t.Discovery.IsDiscovery() {
			discovery++
		}
	}
	return fmt.Sprintf("Would add %d %s · %d discovery, %d familiar (target %d discovery):",
		len(v.Tracks), text.Pluralize(len(v.Tracks), "track", "tracks"),
		discovery, len(v.Tracks)-discovery, v.DiscoveryTarget)
}

// discoveryLabel is the one-word classification shown beside a selection.
func discoveryLabel(class radio.DiscoveryClass) string {
	if class.IsDiscovery() {
		return "discovery"
	}
	return "familiar"
}

// outcomeLines explains an outcome the batch alone cannot: a relaxed target, a
// filter set nothing matched, or settings that could not be evaluated.
func (v Preview) outcomeLines(names map[int64]string) []previewLine {
	var lines []previewLine
	for _, s := range v.Shortages {
		lines = append(lines, previewLine{text: shortageText(s), tone: ToneWarn})
	}
	if v.Outcome == radio.PlanOutcomeEmpty || v.Outcome == radio.PlanOutcomeBlocked {
		for _, r := range v.Reasons {
			lines = append(lines, previewLine{text: reasonPhrase(r, names), tone: ToneWarn})
		}
	}
	return append(lines, v.rejectionLines(names)...)
}

// rejectionLines tallies what stood in the way, so a short or empty batch says
// which filter did the work rather than only reporting the absence.
func (v Preview) rejectionLines(names map[int64]string) []previewLine {
	if v.Outcome == radio.PlanOutcomeOK || len(v.Rejections) == 0 {
		return nil
	}
	lines := []previewLine{{
		text: fmt.Sprintf("%d %s were ruled out:", v.Rejections.Total(),
			text.Pluralize(v.Rejections.Total(), "candidate", "candidates")),
		tone: ToneInfo,
	}}
	for i, r := range v.Rejections {
		if i == maxRejectionLines {
			lines = append(lines, previewLine{
				text:   fmt.Sprintf("and %d more %s", len(v.Rejections)-i, text.Pluralize(len(v.Rejections)-i, "reason", "reasons")),
				indent: 2,
				tone:   ToneInfo,
			})
			break
		}
		lines = append(lines, previewLine{
			text:   fmt.Sprintf("%d × %s", r.Count, reasonPhrase(r.Reason(), names)),
			indent: 2,
			tone:   ToneInfo,
		})
	}
	return lines
}

// shortageText states one unfilled quota as a count, not as a judgement.
func shortageText(s radio.Shortage) string {
	switch s.Kind {
	case radio.ShortageDiscovery:
		return fmt.Sprintf("Only %d of %d discovery %s were available; familiar tracks filled the rest.",
			s.Got, s.Want, text.Pluralize(s.Want, "track", "tracks"))
	case radio.ShortageFamiliar:
		return fmt.Sprintf("Only %d of %d familiar %s were available; discovery tracks filled the rest.",
			s.Got, s.Want, text.Pluralize(s.Want, "track", "tracks"))
	case radio.ShortageBatch:
		return fmt.Sprintf("Only %d of %d %s could be selected from the eligible candidates.",
			s.Got, s.Want, text.Pluralize(s.Want, "track", "tracks"))
	}
	return fmt.Sprintf("%s quota short: %d of %d.", s.Kind, s.Got, s.Want)
}

// evidenceLines reports what the ranking could not read, and states the one
// thing about Muse's play data a reader would otherwise have to guess at.
//
// The footnote is not a disclaimer. play_history counts finished listens only,
// so "no counted plays" is exactly that and nothing more — the DJ has no
// evidence a skipped track was ever skipped, and an explanation that let a
// reader infer otherwise would be describing a signal Muse does not record.
func (v Preview) evidenceLines() []previewLine {
	lines := make([]previewLine, 0, len(v.Gaps)+1)
	for _, g := range v.Gaps {
		lines = append(lines, previewLine{text: gapText(g), tone: ToneWarn})
	}
	return append(lines, previewLine{
		text: "Play data counts finished listens only; skipped plays are not recorded.",
		tone: ToneInfo,
	})
}

func gapText(g EvidenceGap) string {
	line := "Ranked without " + g.Signal + " — that evidence was unavailable"
	if g.Detail == "" {
		return line + "."
	}
	return line + ": " + g.Detail
}

// reasonPhrases renders a whole explanation as one sentence fragment.
func reasonPhrases(rs radio.Reasons, names map[int64]string) string {
	if len(rs) == 0 {
		return "no evidence recorded"
	}
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		if phrase := reasonPhrase(r, names); phrase != "" {
			parts = append(parts, phrase)
		}
	}
	if len(parts) == 0 {
		return "no evidence recorded"
	}
	return joinPhrases(parts)
}

func joinPhrases(parts []string) string {
	out := parts[0]
	for _, p := range parts[1:] {
		out += "; " + p
	}
	return out
}

// reasonPhrase turns one structured reason into plain English.
//
// Every phrase states an observation. "no counted plays recorded" is what the
// database can support; "never listened to" is not, because a skipped listen
// leaves no counted play behind. Likewise a missing inclusion match reports the
// absence of the metadata rather than implying the track failed a test.
func reasonPhrase(r radio.Reason, names map[int64]string) string {
	switch r.Code {
	case radio.ReasonDiscoveryNever:
		return "no counted plays recorded"
	case radio.ReasonDiscoveryStale:
		return "last counted play at least 90 days ago"
	case radio.ReasonFamiliar:
		return "counted play within the last 90 days"
	case radio.ReasonIncludeTag:
		return "carries included tag " + tagLabel(r.Value, names)
	case radio.ReasonIncludeGenre:
		return "matches included genre " + quoted(r.Value)
	case radio.ReasonIncludeArtist:
		return "matches included artist " + quoted(r.Value)
	case radio.ReasonExcludeTag:
		return "carries excluded tag " + tagLabel(r.Value, names)
	case radio.ReasonExcludeGenre:
		return "matches excluded genre " + quoted(r.Value)
	case radio.ReasonExcludeArtist:
		return "matches excluded artist " + quoted(r.Value)
	}
	return reasonPhraseRest(r)
}

// fixedReasonPhrases are the codes whose wording depends on nothing but the
// code itself.
//
// The unsatisfied inclusions are the ones worth reading twice. Each states the
// absence of a piece of metadata, because that is all the planner observed: a
// track with no matching genre has no matching genre, and saying it "failed" or
// "was rejected by" the filter would describe a judgement the data does not
// carry.
var fixedReasonPhrases = map[radio.ReasonCode]string{
	radio.ReasonMissingTag:       "carries none of the included tags",
	radio.ReasonMissingGenre:     "no genre matching the inclusion list",
	radio.ReasonMissingArtist:    "artist is not in the inclusion list",
	radio.ReasonAlreadyQueued:    "already playing or still queued",
	radio.ReasonDiscoveryRelaxed: "discovery target relaxed to fill the batch",
	radio.ReasonFamiliarRelaxed:  "familiar quota relaxed to fill the batch",
	radio.ReasonBatchShort:       "fewer eligible candidates than the batch size",
}

// reasonPhraseRest covers the plan-level codes and the unsatisfied inclusions.
// It is split from reasonPhrase to keep either half inside the complexity
// budget rather than because the two differ in kind.
func reasonPhraseRest(r radio.Reason) string {
	if phrase, ok := fixedReasonPhrases[r.Code]; ok {
		return phrase
	}
	switch r.Code {
	case radio.ReasonNoCandidates:
		return withDetail("no candidate matched these settings", r.Detail)
	case radio.ReasonUnresolvedTag:
		return withDetail("settings reference a tag that no longer exists", r.Detail)
	case radio.ReasonInvalidSettings:
		return withDetail("settings could not be evaluated", r.Detail)
	}
	// An unrecognised code still says something true — the structured reason's
	// own rendering — rather than silently vanishing from the explanation.
	return r.String()
}

func withDetail(phrase, detail string) string {
	if detail == "" {
		return phrase
	}
	return phrase + " (" + detail + ")"
}

// tagLabel resolves a reason's tag id against the catalog. An id the catalog no
// longer holds is shown as the id: the row was chosen under a tag that has since
// been deleted, and hiding that would misdescribe the evidence.
func tagLabel(value string, names map[int64]string) string {
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return quoted(value)
	}
	if name, ok := names[id]; ok {
		return quoted(name)
	}
	return "#" + value + " (deleted)"
}

func quoted(v string) string {
	if v == "" {
		return "(none)"
	}
	return "“" + v + "”"
}

func errText(err error) string {
	if err == nil {
		return "unknown error"
	}
	return err.Error()
}
