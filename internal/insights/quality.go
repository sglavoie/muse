package insights

import (
	"fmt"
	"sort"
	"strings"
)

// deepCutsFlatShare is the point at which the Deep Cuts completion filter has
// stopped discriminating: when nearly every candidate clears it, the mix is
// really just "tracks played a handful of times".
const deepCutsFlatShare = 0.95

// DeepCutsMinSkips is how many skipped listens must be on record before a flat
// share is evidence of anything. Below it the completion average has no
// negative examples, so it is near 1.0 by construction rather than because the
// filter is badly chosen — and the mix itself is gated on the same number, so
// warning about it would describe a mix the user cannot even see. internal/db
// aliases this constant for that gate.
const DeepCutsMinSkips = 20

// foldGenres collapses case and whitespace variants of the same genre into one
// group. The raw variants are kept verbatim so a Data Health merge can target
// the exact stored strings.
//
// Tracks sums the per-variant distinct-track counts, so a track tagged with
// two variants of one genre is counted once per variant. That only happens on
// exactly the dirty data this group is asking the user to clean up.
func foldGenres(rows []GenreRow) []GenreGroup {
	if len(rows) == 0 {
		return nil
	}

	type acc struct {
		group    GenreGroup
		best     GenreRow
		variants map[string]struct{}
	}
	byKey := map[string]*acc{}

	for _, row := range rows {
		key := strings.ToLower(strings.TrimSpace(row.Genre))
		if key == "" {
			continue
		}
		a, ok := byKey[key]
		if !ok {
			a = &acc{variants: map[string]struct{}{}}
			byKey[key] = a
		}
		a.group.Plays += row.Plays
		a.group.ListenedSecs += row.ListenedSecs
		a.group.Tracks += row.Tracks
		a.variants[row.Genre] = struct{}{}
		if betterVariant(row, a.best) {
			a.best = row
		}
	}
	if len(byKey) == 0 {
		return nil
	}

	groups := make([]GenreGroup, 0, len(byKey))
	for _, a := range byKey {
		g := a.group
		g.Canonical = strings.TrimSpace(a.best.Genre)
		g.Variants = make([]string, 0, len(a.variants))
		for v := range a.variants {
			g.Variants = append(g.Variants, v)
		}
		sort.Strings(g.Variants)
		groups = append(groups, g)
	}
	sort.Slice(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.Plays != b.Plays {
			return a.Plays > b.Plays
		}
		if a.ListenedSecs != b.ListenedSecs {
			return a.ListenedSecs > b.ListenedSecs
		}
		return a.Canonical < b.Canonical
	})

	return groups
}

// placeholderGenres are the tags that mean "no genre" without being empty. They
// survive every not-null check and then become their own bar on the genre
// chart, which reads worse than a gap: a gap is visibly missing, whereas
// "Unknown" looks like an answer.
var placeholderGenres = map[string]struct{}{
	"unknown": {}, "unknown genre": {}, "other": {}, "misc": {},
	"miscellaneous": {}, "none": {}, "no genre": {}, "n/a": {}, "na": {},
	"undefined": {}, "unclassified": {}, "untagged": {}, "genre": {},
	"various": {}, "default": {}, "-": {}, "?": {},
}

// IsPlaceholderGenre reports whether a genre name stands in for a real one.
func IsPlaceholderGenre(name string) bool {
	_, ok := placeholderGenres[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

// betterVariant picks the spelling to display: the most played one, then the
// most listened, then the lexicographically first so the choice is stable.
func betterVariant(candidate, current GenreRow) bool {
	if current.Genre == "" {
		return true
	}
	if candidate.Plays != current.Plays {
		return candidate.Plays > current.Plays
	}
	if candidate.ListenedSecs != current.ListenedSecs {
		return candidate.ListenedSecs > current.ListenedSecs
	}
	return candidate.Genre < current.Genre
}

// computeQuality reports what is distorting the other panels. Every issue is
// stated with the number of plays or tracks it affects so the panel can rank
// them by how much they matter.
func computeQuality(groups []GenreGroup, counts QualityCounts) Quality {
	if len(groups) == 0 && counts == (QualityCounts{}) {
		return Quality{}
	}

	q := Quality{Genres: groups, Counts: counts}
	for _, g := range groups {
		if len(g.Variants) > 1 {
			q.Collisions = append(q.Collisions, g)
			q.CollisionPlays += g.Plays
		}
		if IsPlaceholderGenre(g.Canonical) {
			q.Placeholders = append(q.Placeholders, g)
			q.PlaceholderPlays += g.Plays
		}
	}
	q.DeepCutsShare = safeDiv(float64(counts.DeepCutsQualifying), float64(counts.DeepCutsCandidates))

	if len(q.Collisions) > 0 {
		q.Issues = append(q.Issues, HealthIssue{
			Kind:  IssueGenreCollision,
			Title: "Genre spellings collide",
			Detail: fmt.Sprintf("%d %s split across case or spacing variants, e.g. %s",
				len(q.Collisions), plural(len(q.Collisions), "genre is", "genres are"),
				strings.Join(q.Collisions[0].Variants, " / ")),
			Affected: q.CollisionPlays,
			Fixable:  true,
		})
	}
	if len(q.Placeholders) > 0 {
		names := make([]string, len(q.Placeholders))
		for i, g := range q.Placeholders {
			names[i] = g.Canonical
		}
		q.Issues = append(q.Issues, HealthIssue{
			Kind:  IssuePlaceholderGenre,
			Title: "Placeholder genres",
			Detail: fmt.Sprintf("%s: %s on the genre chart", strings.Join(names, ", "),
				plural(len(names), "a placeholder tag that still takes a bar of its own",
					"placeholder tags that still take a bar of their own")),
			Affected: q.PlaceholderPlays,
			Fixable:  true,
		})
	}
	if counts.PlaysWithoutGenre > 0 {
		q.Issues = append(q.Issues, HealthIssue{
			Kind:  IssueGenreMissing,
			Title: "Plays with no genre",
			Detail: fmt.Sprintf("%d %s", counts.TracksWithoutGenre, plural(counts.TracksWithoutGenre,
				"track carries no genre tag, so it is absent from every genre figure",
				"tracks carry no genre tag, so they are absent from every genre figure")),
			Affected: counts.PlaysWithoutGenre,
		})
	}
	// Stays silent below the skip gate: nothing the user can do makes a share
	// measured without skips meaningful, and the mix's own empty state already
	// says it is waiting for them. A permanent un-actionable warning is worse
	// than none, which is also why the issue is not Fixable once it does fire.
	if counts.SkipsRecorded >= DeepCutsMinSkips && counts.DeepCutsCandidates > 0 && q.DeepCutsShare >= deepCutsFlatShare {
		q.Issues = append(q.Issues, HealthIssue{
			Kind:  IssueDeepCutsFlat,
			Title: "Deep Cuts is not discriminating",
			Detail: fmt.Sprintf("%.0f%% of candidates clear the completion filter, so the mix is really just low-play tracks",
				q.DeepCutsShare*100),
			Affected: counts.DeepCutsCandidates,
		})
	}
	if counts.TracksMissing > 0 {
		q.Issues = append(q.Issues, HealthIssue{
			Kind:  IssueMissingFiles,
			Title: "Tracks missing from disk",
			Detail: fmt.Sprintf("%d %s", counts.TracksMissing, plural(counts.TracksMissing,
				"track is indexed but not on disk and is excluded from coverage",
				"tracks are indexed but not on disk and are excluded from coverage")),
			Affected: counts.TracksMissing,
		})
	}

	return q
}

// plural returns one when n is exactly 1 and many otherwise. Issue details
// pass whole phrases so the verb agrees with the count ("1 track carries",
// "2 tracks carry").
func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
