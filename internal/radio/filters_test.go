package radio

import (
	"errors"
	"testing"

	"github.com/sglavoie/muse/internal/library"
)

func facts(artist, genre string, tagIDs ...int64) TrackFacts {
	var tags map[int64]struct{}
	if len(tagIDs) > 0 {
		tags = make(map[int64]struct{}, len(tagIDs))
		for _, id := range tagIDs {
			tags[id] = struct{}{}
		}
	}
	return FactsFromTrack(library.Track{Hash: "h1", Artist: artist, Genre: genre}, tags)
}

func TestFilterSet_Normalized_DedupesAndSorts(t *testing.T) {
	set := FilterSet{
		TagIDs:  []int64{5, 2, 5, 0, -7},
		Genres:  []string{" Rock ", "rock", "", "  ", "Ambient"},
		Artists: []string{"Boards of Canada", "boards of canada", "  Aphex Twin"},
	}
	got := set.Normalized()

	if len(got.TagIDs) != 2 || got.TagIDs[0] != 2 || got.TagIDs[1] != 5 {
		t.Errorf("tag ids = %v, want [2 5]", got.TagIDs)
	}
	if len(got.Genres) != 2 || got.Genres[0] != "Ambient" || got.Genres[1] != "Rock" {
		t.Errorf("genres = %v, want [Ambient Rock] (first spelling kept)", got.Genres)
	}
	if len(got.Artists) != 2 || got.Artists[0] != "Aphex Twin" || got.Artists[1] != "Boards of Canada" {
		t.Errorf("artists = %v, want trimmed, case-folded duplicates removed", got.Artists)
	}
}

func TestFilterSet_Normalized_EmptyStaysNil(t *testing.T) {
	got := FilterSet{Genres: []string{"  ", ""}, TagIDs: []int64{0}}.Normalized()
	if !got.IsEmpty() {
		t.Errorf("blank-only filters must normalize to empty: %+v", got)
	}
	if got.TagIDSet() != nil {
		t.Errorf("TagIDSet must be nil when no usable id remains")
	}
}

func TestFilterSet_Validate_ReportsNonPositiveTagID(t *testing.T) {
	if err := (FilterSet{TagIDs: []int64{1, 0}}).Validate(); !errors.Is(err, ErrInvalidTagID) {
		t.Errorf("Validate() = %v, want ErrInvalidTagID", err)
	}
	if err := (FilterSet{TagIDs: []int64{1, 2}}).Validate(); err != nil {
		t.Errorf("positive tag ids must validate: %v", err)
	}
}

func TestMatchFilters_EmptyFiltersAcceptEverything(t *testing.T) {
	verdict := MatchFilters(FilterSet{}, FilterSet{}, facts("Anyone", "Anything"))
	if !verdict.Eligible {
		t.Errorf("empty filters must accept every track: %+v", verdict.Blocked)
	}
	if len(verdict.Matched) != 0 {
		t.Errorf("no inclusion field is populated, so no match reasons expected: %v", verdict.Matched)
	}
}

func TestMatchFilters_OrWithinFieldAndAcrossFields(t *testing.T) {
	include := FilterSet{Genres: []string{"Rock", "Jazz"}, Artists: []string{"Miles Davis"}}

	eligible := MatchFilters(include, FilterSet{}, facts("Miles Davis", "Jazz"))
	if !eligible.Eligible {
		t.Fatalf("track matching both fields must be eligible: %v", eligible.Blocked)
	}
	if !eligible.Matched.Has(ReasonIncludeGenre) || !eligible.Matched.Has(ReasonIncludeArtist) {
		t.Errorf("expected genre and artist match reasons, got %v", eligible.Matched.Codes())
	}

	// OR within the genre field: the other listed genre also matches.
	if v := MatchFilters(include, FilterSet{}, facts("Miles Davis", "Rock")); !v.Eligible {
		t.Errorf("either listed genre must satisfy the field: %v", v.Blocked)
	}

	// AND across populated fields: matching the genre alone is not enough.
	blocked := MatchFilters(include, FilterSet{}, facts("Someone Else", "Rock"))
	if blocked.Eligible {
		t.Errorf("a populated artist inclusion must also match")
	}
	if blocked.Blocked.Code != ReasonMissingArtist {
		t.Errorf("blocked code = %q, want %q", blocked.Blocked.Code, ReasonMissingArtist)
	}
}

func TestMatchFilters_ExclusionsWin(t *testing.T) {
	include := FilterSet{Genres: []string{"Rock"}}
	exclude := FilterSet{Artists: []string{"Noisy Band"}}

	verdict := MatchFilters(include, exclude, facts("Noisy Band", "Rock"))
	if verdict.Eligible {
		t.Errorf("an exclusion must beat a satisfied inclusion")
	}
	if verdict.Blocked.Code != ReasonExcludeArtist {
		t.Errorf("blocked code = %q, want %q", verdict.Blocked.Code, ReasonExcludeArtist)
	}
	if verdict.Blocked.Value != "Noisy Band" {
		t.Errorf("blocked value = %q, want the matching filter value", verdict.Blocked.Value)
	}
}

func TestMatchFilters_ConflictingValueIsUnsatisfiable(t *testing.T) {
	include := FilterSet{Genres: []string{"Rock"}}
	exclude := FilterSet{Genres: []string{"rock"}}

	if v := MatchFilters(include, exclude, facts("Someone", "Rock")); v.Eligible {
		t.Errorf("a value in both lists must never be eligible")
	}
	conflicts := Conflicts(include, exclude)
	if len(conflicts.Genres) != 1 || conflicts.Genres[0] != "Rock" {
		t.Errorf("Conflicts().Genres = %v, want [Rock]", conflicts.Genres)
	}
	if !Conflicts(include, FilterSet{}).IsEmpty() {
		t.Errorf("no overlap means no conflicts")
	}
}

func TestConflicts_TagsAndArtists(t *testing.T) {
	include := FilterSet{TagIDs: []int64{1, 2}, Artists: []string{"Aphex Twin"}}
	exclude := FilterSet{TagIDs: []int64{2, 3}, Artists: []string{"aphex twin"}}
	conflicts := Conflicts(include, exclude)
	if len(conflicts.TagIDs) != 1 || conflicts.TagIDs[0] != 2 {
		t.Errorf("conflicting tag ids = %v, want [2]", conflicts.TagIDs)
	}
	if len(conflicts.Artists) != 1 || conflicts.Artists[0] != "Aphex Twin" {
		t.Errorf("conflicting artists = %v, want [Aphex Twin]", conflicts.Artists)
	}
}

func TestMatchFilters_MissingMetadataNeverSatisfiesInclusion(t *testing.T) {
	cases := []struct {
		name     string
		include  FilterSet
		facts    TrackFacts
		wantCode ReasonCode
	}{
		{"no genre", FilterSet{Genres: []string{"Rock"}}, facts("Someone", ""), ReasonMissingGenre},
		{"no tags", FilterSet{TagIDs: []int64{1}}, facts("Someone", "Rock"), ReasonMissingTag},
		{"no artist", FilterSet{Artists: []string{"Miles Davis"}}, facts("", "Rock"), ReasonMissingArtist},
		{"other tag", FilterSet{TagIDs: []int64{1}}, facts("Someone", "Rock", 9), ReasonMissingTag},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			verdict := MatchFilters(tc.include, FilterSet{}, tc.facts)
			if verdict.Eligible {
				t.Fatalf("missing metadata must not satisfy an inclusion")
			}
			if verdict.Blocked.Code != tc.wantCode {
				t.Errorf("blocked code = %q, want %q", verdict.Blocked.Code, tc.wantCode)
			}
		})
	}
}

func TestMatchFilters_UnknownArtistSentinel(t *testing.T) {
	blank := facts("", "Rock")
	sentinel := facts("Unknown Artist", "Rock")

	include := FilterSet{Artists: []string{"Unknown Artist"}}
	if v := MatchFilters(include, FilterSet{}, blank); !v.Eligible {
		t.Errorf("a blank artist must match an explicit Unknown Artist inclusion")
	}
	if v := MatchFilters(include, FilterSet{}, sentinel); !v.Eligible {
		t.Errorf("the sentinel artist must match an Unknown Artist inclusion")
	}

	exclude := FilterSet{Artists: []string{"unknown artist"}}
	if v := MatchFilters(FilterSet{}, exclude, blank); v.Eligible {
		t.Errorf("excluding Unknown Artist must also exclude blank artists")
	}
	if v := MatchFilters(FilterSet{}, exclude, sentinel); v.Eligible {
		t.Errorf("excluding Unknown Artist must exclude the sentinel")
	}
	if v := MatchFilters(FilterSet{}, exclude, facts("Named", "Rock")); !v.Eligible {
		t.Errorf("a named artist must survive an Unknown Artist exclusion")
	}
}

func TestMatchFilters_CaseAndWhitespaceInsensitive(t *testing.T) {
	include := FilterSet{Genres: []string{" ROCK "}, Artists: []string{"  aphex twin"}}
	if v := MatchFilters(include, FilterSet{}, facts("Aphex Twin", "rock")); !v.Eligible {
		t.Errorf("filters must match case- and whitespace-insensitively: %v", v.Blocked)
	}
}

func TestMatchFilters_MultiGenreTrack(t *testing.T) {
	include := FilterSet{Genres: []string{"Jazz"}}
	if v := MatchFilters(include, FilterSet{}, facts("Someone", "Rock;Jazz")); !v.Eligible {
		t.Errorf("any of a track's split genres may satisfy the inclusion: %v", v.Blocked)
	}
}

func TestMatchFilters_TagMatchReportsTagID(t *testing.T) {
	verdict := MatchFilters(FilterSet{TagIDs: []int64{4}}, FilterSet{}, facts("Someone", "Rock", 4, 9))
	if !verdict.Eligible {
		t.Fatalf("shared tag must satisfy the inclusion: %v", verdict.Blocked)
	}
	if verdict.Matched[0].Code != ReasonIncludeTag || verdict.Matched[0].Value != "4" {
		t.Errorf("match reason = %+v, want include_tag with the tag id", verdict.Matched[0])
	}
}

func TestFilterSet_ResolveTags(t *testing.T) {
	set := FilterSet{TagIDs: []int64{1, 4}}
	if err := set.ResolveTags(TagIDSet{1: {}, 4: {}}); err != nil {
		t.Errorf("known tags must resolve: %v", err)
	}
	err := set.ResolveTags(TagIDSet{1: {}})
	if !errors.Is(err, ErrUnresolvedTag) {
		t.Fatalf("ResolveTags() = %v, want ErrUnresolvedTag", err)
	}
	if err.Error() == "" {
		t.Errorf("unresolved tag error needs a message")
	}
}

func TestFilterSet_TagIDSet(t *testing.T) {
	set := FilterSet{TagIDs: []int64{3, 3, 1}}.TagIDSet()
	if len(set) != 2 || !set.HasTag(1) || !set.HasTag(3) {
		t.Errorf("TagIDSet = %v, want deduplicated {1,3}", set)
	}
	if set.HasTag(2) {
		t.Errorf("HasTag must be false for absent ids")
	}
}

func TestCompiledFilters_MatchesLikeMatchFilters(t *testing.T) {
	settings := DefaultSettings()
	settings.Include = FilterSet{Genres: []string{"Rock", "Jazz"}}
	settings.Exclude = FilterSet{Artists: []string{"Noisy Band"}}
	compiled := settings.Compile()

	candidates := []TrackFacts{
		facts("Someone", "Rock"),
		facts("Noisy Band", "Rock"),
		facts("Someone", "Pop"),
		facts("", ""),
	}
	for _, candidate := range candidates {
		want := MatchFilters(settings.Include, settings.Exclude, candidate)
		got := compiled.Match(candidate)
		if got.Eligible != want.Eligible || got.Blocked != want.Blocked {
			t.Errorf("compiled verdict for %+v = %+v, want %+v", candidate, got, want)
		}
	}
}

func TestCompiledFilters_ReusableAcrossCandidates(t *testing.T) {
	compiled := CompileFilters(FilterSet{Genres: []string{"Rock"}}, FilterSet{})
	first := compiled.Match(facts("A", "Rock"))
	second := compiled.Match(facts("B", "Pop"))
	third := compiled.Match(facts("C", "rock"))

	if !first.Eligible || second.Eligible || !third.Eligible {
		t.Errorf("compiled filters must stay usable across candidates: %v %v %v",
			first.Eligible, second.Eligible, third.Eligible)
	}
	if first.Matched[0].Value != "Rock" {
		t.Errorf("matched value = %q, want the filter's spelling", first.Matched[0].Value)
	}
}

func TestSettings_MatchAndConflicts(t *testing.T) {
	settings := DefaultSettings()
	settings.Include = FilterSet{Genres: []string{"Rock"}}
	settings.Exclude = FilterSet{Genres: []string{"Rock"}, Artists: []string{"Noisy Band"}}

	if v := settings.Match(facts("Someone", "Rock")); v.Eligible {
		t.Errorf("exclusions win over inclusions through Settings.Match")
	}
	conflicts := settings.Conflicts()
	if len(conflicts.Genres) != 1 || conflicts.Genres[0] != "Rock" {
		t.Errorf("Settings.Conflicts().Genres = %v, want [Rock]", conflicts.Genres)
	}
	if len(conflicts.Artists) != 0 {
		t.Errorf("an exclusion-only artist is not a conflict: %v", conflicts.Artists)
	}
}

func TestFactsFromTrack_SplitsGenres(t *testing.T) {
	got := FactsFromTrack(library.Track{Hash: "h", Artist: "A", Genre: "Rock/Jazz"}, nil)
	if len(got.Genres) != 2 {
		t.Errorf("genres = %v, want two split values", got.Genres)
	}
	if got.Hash != "h" || got.Artist != "A" {
		t.Errorf("facts = %+v, want hash and artist carried over", got)
	}
}
