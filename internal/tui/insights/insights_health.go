package insights

// Data Health: what is lying to the panels above, and what to press to fix it.
//
// Every other panel measures listening; this one measures the library the
// measurements were taken from. It is built from the in-memory library rather
// than from insights.Snapshot because a finding has to name the exact tracks it
// is about, and insights.CoverageRow carries neither title nor genre.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	insightsdata "github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	"github.com/sglavoie/muse/internal/tui/styles"
	"github.com/sglavoie/muse/internal/tui/text"
)

// The Data Health section names. They are the row headers on screen and the
// strings the layout scenarios assert on.
const (
	HealthSectionCollisions   = "Genre collisions"
	HealthSectionPlaceholders = "Placeholder genres"
	HealthSectionMissing      = "Missing metadata"
	HealthSectionDurations    = "Unverified durations"
	HealthSectionArtists      = "Artist near-duplicates"
	HealthSectionDeepCuts     = "Deep Cuts"
)

// HealthRowKind names what a finding is about. Only genre collisions have a
// one-key fix, so this is what m checks before offering to merge.
type HealthRowKind int

const (
	HealthGenreCollision HealthRowKind = iota
	HealthPlaceholderGenre
	HealthMissingMetadata
	HealthUnverifiedDuration
	HealthArtistDuplicate
	HealthDeepCutsFlat
)

// HealthRow is one finding together with the exact tracks behind it.
// The hashes are stored rather than re-derived when the row is opened, so the
// count a row reports and the list it opens cannot disagree.
type HealthRow struct {
	Kind    HealthRowKind
	Section string
	Label   string
	note    string
	Hashes  []string
	// Canonical and variants are set on collision rows only: the spelling to
	// keep, and every stored spelling to fold into it.
	Canonical string
	Variants  []string
}

// BuildHealth folds the library into the Data Health findings. It runs
// on the load goroutine beside the other library-derived Insights inputs.
// unverified is the set of tracks whose duration has never been checked against
// the file; deepCutsMin/Max mirror the Deep Cuts mix bounds.
func BuildHealth(q insightsdata.Quality, tracks []library.Track, unverified []string, deepCutsMin, deepCutsMax int) []HealthRow {
	indexed := make(map[string]struct{}, len(tracks))
	for _, t := range tracks {
		indexed[t.Hash] = struct{}{}
	}

	// Keyed on the folded genre so every spelling of a colliding genre lands in
	// the one bucket the merge will rewrite.
	byGenre := map[string][]string{}
	for _, t := range tracks {
		for _, g := range library.SplitGenres(t.Genre) {
			key := GenreKey(g)
			byGenre[key] = append(byGenre[key], t.Hash)
		}
	}

	var rows []HealthRow
	add := func(kind HealthRowKind, section, label, note string, hashes []string) {
		rows = append(rows, HealthRow{
			Kind: kind, Section: section, Label: label, note: note, Hashes: hashes,
		})
	}

	for _, g := range q.Collisions {
		hashes := byGenre[GenreKey(g.Canonical)]
		rows = append(rows, HealthRow{
			Kind:      HealthGenreCollision,
			Section:   HealthSectionCollisions,
			Label:     strings.Join(g.Variants, " / "),
			note:      healthTrackNote(len(hashes)),
			Hashes:    hashes,
			Canonical: g.Canonical,
			Variants:  g.Variants,
		})
	}

	for _, g := range q.Placeholders {
		hashes := byGenre[GenreKey(g.Canonical)]
		add(HealthPlaceholderGenre, HealthSectionPlaceholders, g.Canonical,
			healthTrackNote(len(hashes)), hashes)
	}

	for _, f := range healthMissingFields {
		var hashes []string
		for _, t := range tracks {
			if f.missing(t) {
				hashes = append(hashes, t.Hash)
			}
		}
		if len(hashes) > 0 {
			add(HealthMissingMetadata, HealthSectionMissing, f.label,
				healthTrackNote(len(hashes)), hashes)
		}
	}

	// Filtered to what the index actually holds: an unverified row that counts
	// tracks the drill cannot open would be the exact drift this panel exists
	// to report.
	var unverifiedHashes []string
	for _, h := range unverified {
		if _, ok := indexed[h]; ok {
			unverifiedHashes = append(unverifiedHashes, h)
		}
	}
	if len(unverifiedHashes) > 0 {
		add(HealthUnverifiedDuration, HealthSectionDurations, "Never checked against the file",
			healthTrackNote(len(unverifiedHashes)), unverifiedHashes)
	}

	rows = append(rows, healthArtistDuplicates(tracks)...)

	if qualityHasIssue(q, insightsdata.IssueDeepCutsFlat) {
		var hashes []string
		for _, t := range tracks {
			if t.PlayCount >= deepCutsMin && t.PlayCount <= deepCutsMax {
				hashes = append(hashes, t.Hash)
			}
		}
		add(HealthDeepCutsFlat, HealthSectionDeepCuts, "Completion filter clears almost everything",
			fmt.Sprintf("%.0f%% of candidates", q.DeepCutsShare*100), hashes)
	}

	return rows
}

// healthMissingFields is the metadata a track is expected to carry. Each entry
// becomes one row, so the bulk editor opens on exactly the field that is blank.
var healthMissingFields = []struct {
	label   string
	missing func(library.Track) bool
}{
	{"No title", func(t library.Track) bool { return strings.TrimSpace(t.Title) == "" }},
	{"No artist", func(t library.Track) bool { return strings.TrimSpace(t.Artist) == "" }},
	{"No album", func(t library.Track) bool { return strings.TrimSpace(t.Album) == "" }},
	{"No genre", func(t library.Track) bool { return len(library.SplitGenres(t.Genre)) == 0 }},
}

// healthArtistDuplicates groups artist names that differ only in case, spacing,
// punctuation, a leading article or "&" versus "and". Anything subtler — a
// dropped diacritic, a misspelling — is left alone: a false merge costs more
// than a missed one, because the merge rewrites tags on disk.
func healthArtistDuplicates(tracks []library.Track) []HealthRow {
	type cluster struct {
		names  map[string]struct{}
		hashes []string
	}
	byKey := map[string]*cluster{}
	var order []string

	for _, t := range tracks {
		name := strings.TrimSpace(t.Artist)
		if name == "" {
			continue
		}
		key := HealthArtistKey(name)
		if key == "" {
			continue
		}
		c := byKey[key]
		if c == nil {
			c = &cluster{names: map[string]struct{}{}}
			byKey[key] = c
			order = append(order, key)
		}
		c.names[name] = struct{}{}
		c.hashes = append(c.hashes, t.Hash)
	}

	var out []HealthRow
	for _, key := range order {
		c := byKey[key]
		if len(c.names) < 2 {
			continue
		}
		names := make([]string, 0, len(c.names))
		for n := range c.names {
			names = append(names, n)
		}
		sort.Strings(names)
		out = append(out, HealthRow{
			Kind:    HealthArtistDuplicate,
			Section: HealthSectionArtists,
			Label:   strings.Join(names, " / "),
			note:    healthTrackNote(len(c.hashes)),
			Hashes:  c.hashes,
		})
	}
	return out
}

// HealthArtistKey folds the spelling differences that produce two shelves for
// one artist.
func HealthArtistKey(name string) string {
	folded := strings.ToLower(strings.TrimSpace(name))
	folded = strings.ReplaceAll(folded, "&", " and ")
	folded = strings.TrimPrefix(folded, "the ")

	var b strings.Builder
	for _, r := range folded {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r > 127 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func healthTrackNote(n int) string {
	return fmt.Sprintf("%d %s", n, text.Pluralize(n, "track", "tracks"))
}

func qualityHasIssue(q insightsdata.Quality, kind insightsdata.IssueKind) bool {
	for _, issue := range q.Issues {
		if issue.Kind == kind {
			return true
		}
	}
	return false
}

// healthRows renders the precomputed findings as briefing rows. The bar is the
// share of the library a finding touches, so the panel ranks by blast radius
// rather than by section order.
func (v View) healthRows() []Row {
	total := max(v.Snap.Coverage.TotalTracks, 1)
	rows := make([]Row, 0, len(v.Health))
	for i, h := range v.Health {
		rows = append(rows, Row{
			Section:  h.Section,
			Label:    h.Label,
			note:     h.note,
			fraction: float64(len(h.Hashes)) / float64(total),
			Drill:    DrillHealth,
			Key:      strconv.Itoa(i),
		})
	}
	return rows
}

// healthAffectedTracks counts the distinct tracks named by at least one
// finding. Findings overlap — a track with no genre is often the same one with
// no album — so summing the rows would overstate the damage.
func (v View) healthAffectedTracks() int {
	seen := make(map[string]struct{})
	for _, h := range v.Health {
		for _, hash := range h.Hashes {
			seen[hash] = struct{}{}
		}
	}
	return len(seen)
}

// FocusedHealthRow returns the finding under the cursor, or nil when the Data
// Health panel is not the one being browsed.
func (v View) FocusedHealthRow() *HealthRow {
	if v.Panel != PanelDataHealth || v.Level != LevelBriefing {
		return nil
	}
	if v.Cursor < 0 || v.Cursor >= len(v.Health) {
		return nil
	}
	return &v.Health[v.Cursor]
}

func (v View) dataHealthHeaderLines(s styles.Styles) []string {
	head := []string{"", s.Dim.Render(v.fit(v.headline(PanelDataHealth))), ""}
	if len(v.Health) == 0 && len(v.Snap.Quality.Issues) == 0 {
		return append(head, s.TextSecondary.Render(v.fit(
			"Every genre folds cleanly and every track carries its metadata.")))
	}

	// The prose is where the findings with no track list behind them live —
	// missing files, an inert Deep Cuts filter — so dropping it would silently
	// lose them.
	blocks := make([][]string, 0, len(v.Snap.Quality.Issues))
	for _, issue := range v.Snap.Quality.Issues {
		blocks = append(blocks, []string{s.TextSecondary.Render(v.fit("· " + issue.Detail))})
	}
	return insightsFitHeader(v.headerBudget(), head, blocks, []string{""})
}
