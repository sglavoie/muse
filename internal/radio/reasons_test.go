package radio

import "testing"

func TestReason_String(t *testing.T) {
	cases := []struct {
		name   string
		reason Reason
		want   string
	}{
		{"code only", Reason{Code: ReasonNoCandidates}, "no_candidates"},
		{"code and value", Reason{Code: ReasonExcludeGenre, Value: "Rock"}, "exclude_genre: Rock"},
		{"code and detail", Reason{Code: ReasonMissingGenre, Detail: "no genre"}, "missing_genre (no genre)"},
		{
			"everything",
			Reason{Code: ReasonIncludeTag, Value: "7", Detail: "shared tag"},
			"include_tag: 7 (shared tag)",
		},
		{"zero", Reason{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.reason.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestReason_IsZero(t *testing.T) {
	if !(Reason{}).IsZero() {
		t.Errorf("the zero reason must report zero")
	}
	if (Reason{Detail: "something"}).IsZero() {
		t.Errorf("a reason carrying detail is not zero")
	}
}

func TestReasons_Helpers(t *testing.T) {
	reasons := Reasons{
		{Code: ReasonIncludeGenre, Value: "Rock"},
		{Code: ReasonDiscoveryStale},
	}
	if !reasons.Has(ReasonDiscoveryStale) {
		t.Errorf("Has must find a present code")
	}
	if reasons.Has(ReasonExcludeTag) {
		t.Errorf("Has must not find an absent code")
	}
	codes := reasons.Codes()
	if len(codes) != 2 || codes[0] != ReasonIncludeGenre || codes[1] != ReasonDiscoveryStale {
		t.Errorf("Codes() = %v, want the codes in order", codes)
	}
	if got := reasons.Summary(); got != "include_genre: Rock; discovery_stale" {
		t.Errorf("Summary() = %q", got)
	}
	if got := (Reasons{}).Summary(); got != "" {
		t.Errorf("empty Summary() = %q, want empty", got)
	}
	if (Reasons{}).Codes() != nil {
		t.Errorf("empty Codes() must be nil")
	}
}

func TestReasons_SummarySkipsZeroReasons(t *testing.T) {
	reasons := Reasons{{}, {Code: ReasonFamiliar}, {}}
	if got := reasons.Summary(); got != "familiar" {
		t.Errorf("Summary() = %q, want zero reasons skipped", got)
	}
}

func TestDiscoveryReason(t *testing.T) {
	cases := []struct {
		class DiscoveryClass
		want  ReasonCode
	}{
		{DiscoveryNever, ReasonDiscoveryNever},
		{DiscoveryStale, ReasonDiscoveryStale},
		{DiscoveryFamiliar, ReasonFamiliar},
	}
	for _, tc := range cases {
		got := DiscoveryReason(tc.class)
		if got.Code != tc.want {
			t.Errorf("DiscoveryReason(%q).Code = %q, want %q", tc.class, got.Code, tc.want)
		}
		if got.Detail == "" {
			t.Errorf("DiscoveryReason(%q) needs a human-readable detail", tc.class)
		}
	}
}
