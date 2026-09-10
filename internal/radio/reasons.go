package radio

import "strings"

// ReasonCode identifies why a track was kept, rejected, or why a plan could
// not be fulfilled. Codes are stable strings so they can be persisted and
// rendered without a translation table.
type ReasonCode string

const (
	// Inclusion matches.
	ReasonIncludeTag    ReasonCode = "include_tag"
	ReasonIncludeGenre  ReasonCode = "include_genre"
	ReasonIncludeArtist ReasonCode = "include_artist"

	// Exclusion matches — these block a track outright.
	ReasonExcludeTag    ReasonCode = "exclude_tag"
	ReasonExcludeGenre  ReasonCode = "exclude_genre"
	ReasonExcludeArtist ReasonCode = "exclude_artist"

	// Unsatisfied inclusion fields, including tracks whose metadata is
	// missing entirely.
	ReasonMissingTag    ReasonCode = "missing_tag"
	ReasonMissingGenre  ReasonCode = "missing_genre"
	ReasonMissingArtist ReasonCode = "missing_artist"

	// Discovery classification.
	ReasonDiscoveryNever ReasonCode = "discovery_never"
	ReasonDiscoveryStale ReasonCode = "discovery_stale"
	ReasonFamiliar       ReasonCode = "familiar"

	// Plan-level outcomes.
	ReasonUnresolvedTag    ReasonCode = "unresolved_tag"
	ReasonInvalidSettings  ReasonCode = "invalid_settings"
	ReasonNoCandidates     ReasonCode = "no_candidates"
	ReasonDiscoveryRelaxed ReasonCode = "discovery_relaxed"
)

// Reason is a structured explanation. Value carries the filter value or tag id
// that triggered the reason; Detail carries free-form context.
type Reason struct {
	Code   ReasonCode
	Value  string
	Detail string
}

// IsZero reports whether the reason is unset.
func (r Reason) IsZero() bool {
	return r.Code == "" && r.Value == "" && r.Detail == ""
}

// String renders the reason as "code: value (detail)", omitting empty parts.
func (r Reason) String() string {
	if r.IsZero() {
		return ""
	}
	var b strings.Builder
	b.WriteString(string(r.Code))
	if r.Value != "" {
		b.WriteString(": ")
		b.WriteString(r.Value)
	}
	if r.Detail != "" {
		b.WriteString(" (")
		b.WriteString(r.Detail)
		b.WriteString(")")
	}
	return b.String()
}

// Reasons is an ordered list of explanations.
type Reasons []Reason

// Has reports whether any reason carries code.
func (rs Reasons) Has(code ReasonCode) bool {
	for _, r := range rs {
		if r.Code == code {
			return true
		}
	}
	return false
}

// Codes returns the codes in order, including duplicates.
func (rs Reasons) Codes() []ReasonCode {
	if len(rs) == 0 {
		return nil
	}
	codes := make([]ReasonCode, 0, len(rs))
	for _, r := range rs {
		codes = append(codes, r.Code)
	}
	return codes
}

// Summary joins the non-empty reasons with "; " for single-line display.
func (rs Reasons) Summary() string {
	parts := make([]string, 0, len(rs))
	for _, r := range rs {
		if s := r.String(); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, "; ")
}

// DiscoveryReason maps a classification onto its reason code.
func DiscoveryReason(class DiscoveryClass) Reason {
	switch class {
	case DiscoveryNever:
		return Reason{Code: ReasonDiscoveryNever, Detail: "no counted plays"}
	case DiscoveryStale:
		return Reason{Code: ReasonDiscoveryStale, Detail: "last counted play is at least 90 days old"}
	default:
		return Reason{Code: ReasonFamiliar, Detail: "played within the last 90 days"}
	}
}
