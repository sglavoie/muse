package autodj

import (
	"strconv"

	"github.com/sglavoie/muse/internal/radio"
)

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
