package tui

import (
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
)

// genreCoverageShelves measures unheard coverage per genre. insights.CoverageRow
// carries no genre, so this is derived from the in-memory library, which is
// filtered to non-missing tracks exactly like the coverage query.
func genreCoverageShelves(tracks []library.Track) []insights.ShelfStat {
	byGenre := make(map[string]*insights.ShelfStat)
	for _, t := range tracks {
		for _, g := range library.SplitGenres(t.Genre) {
			s := byGenre[g]
			if s == nil {
				s = &insights.ShelfStat{Name: g}
				byGenre[g] = s
			}
			s.Tracks++
			if t.PlayCount == 0 {
				s.Unheard++
				s.UnheardSecs += t.Duration
			}
		}
	}

	shelves := make([]insights.ShelfStat, 0, len(byGenre))
	for _, s := range byGenre {
		if s.Unheard > 0 {
			shelves = append(shelves, *s)
		}
	}
	sort.Slice(shelves, func(i, j int) bool {
		if shelves[i].Unheard != shelves[j].Unheard {
			return shelves[i].Unheard > shelves[j].Unheard
		}
		return shelves[i].Name < shelves[j].Name
	})
	return shelves[:min(len(shelves), insightsDormantTopN)]
}

// dormantTracks lists tracks that were played at least once and then left
// untouched the longest. ref is the most recent play in the whole history, so
// the cutoff does not drift between renders the way a wall-clock one would.
func dormantTracks(tracks []library.Track, ref int64) []library.Track {
	if ref <= 0 {
		return nil
	}
	cutoff := ref - insightsDormantDays*86400
	var out []library.Track
	for _, t := range tracks {
		if t.PlayCount > 0 && t.LastPlayedAt > 0 && t.LastPlayedAt < cutoff {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].LastPlayedAt != out[j].LastPlayedAt {
			return out[i].LastPlayedAt < out[j].LastPlayedAt
		}
		return out[i].Hash < out[j].Hash
	})
	return out[:min(len(out), insightsDormantTopN)]
}

// backlogMonths groups the never-played tracks by the local month they entered
// the library, most recent first. Tracks with no recorded add date are left out
// rather than piled into the epoch; the Discovery body carries a total row so
// they stay reachable.
func backlogMonths(tracks []library.Track, loc *time.Location) []insightsBacklogMonth {
	byKey := map[string]*insightsBacklogMonth{}
	for _, t := range tracks {
		if t.PlayCount > 0 || t.CreatedAt <= 0 {
			continue
		}
		added := time.Unix(t.CreatedAt, 0).In(loc)
		key := insightsBucketKey(insights.ResolutionMonthly, added)
		b := byKey[key]
		if b == nil {
			b = &insightsBacklogMonth{Key: key, Label: added.Format("January 2006")}
			byKey[key] = b
		}
		b.Tracks++
		b.Secs += t.Duration
	}

	out := make([]insightsBacklogMonth, 0, len(byKey))
	for _, b := range byKey {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key > out[j].Key })
	return out[:min(len(out), insightsMonthRows)]
}

// insightsDrillTracks resolves a briefing row to the concrete tracks behind it,
// so every bar is a query with a shape rather than a picture.
func (m Model) insightsDrillTracks(row insightsRow) []library.Track {
	// Every dormant row opens the whole dormant set: a one-track drill would
	// make E and S meaningless, and the point of the section is the batch.
	switch row.drill {
	case drillDormant:
		return m.insights.dormant
	case drillHour, drillWeekday, drillRhythmGenre:
		return m.insightsPlayedTracks(row)
	case drillTrendBucket, drillFirstPlaysBucket:
		return m.insightsBucketTracks(row.key, row.drill == drillFirstPlaysBucket)
	case drillArtistPlays:
		return m.insightsPlayTracks(func(p insights.PlayRow) bool { return p.Artist == row.key })
	case drillTopArtists:
		return m.insightsTopArtistTracks(row.key)
	case drillHealth:
		return m.insightsHealthTracks(row.key)
	}

	// A Sessions row opens sessions, not tracks, but the batch actions still
	// need a track set behind it: E on the briefing row replays everything the
	// row stands for, in the order it was heard.
	if row.drill.opensSessions() {
		return m.insightsSessionSetTracks(insightsSessionsMatching(m.insights.sessions, row.drill, row.key))
	}

	match := func(t library.Track) bool {
		if t.PlayCount != 0 {
			return false
		}
		switch row.drill {
		case drillUnheardAll:
			return true
		case drillUnheardArtist:
			return t.Artist == row.key
		case drillUnheardAlbum:
			return t.Album == row.key
		case drillUnheardGenre:
			for _, g := range library.SplitGenres(t.Genre) {
				if strings.EqualFold(g, row.key) {
					return true
				}
			}
		case drillAddedNeverPlayed:
			// Always monthly: the backlog is grouped by the month a track was
			// added, which has nothing to do with how the play history is bucketed.
			return t.CreatedAt > 0 && insightsBucketKey(insights.ResolutionMonthly,
				time.Unix(t.CreatedAt, 0).In(m.insights.location())) == row.key
		}
		return false
	}

	var out []library.Track
	for _, t := range m.library.index.Tracks() {
		if match(t) {
			out = append(out, t)
		}
	}
	return out
}

// insightsHealthTracks resolves a Data Health row against the hashes the
// finding was built from, so the row's count and the list it opens are the same
// set rather than two queries that can drift apart.
func (m Model) insightsHealthTracks(key string) []library.Track {
	i, err := strconv.Atoi(key)
	if err != nil || i < 0 || i >= len(m.insights.health) {
		return nil
	}
	var out []library.Track
	for _, hash := range m.insights.health[i].hashes {
		if t, ok := m.library.index.FindByHash(hash); ok {
			out = append(out, t)
		}
	}
	return out
}

// insightsPlayTracks walks the window's plays in order and returns the distinct
// tracks the predicate keeps. Deduping on first appearance rather than last is
// what lets an enqueue replay a bucket in the order it was heard.
func (m Model) insightsPlayTracks(keep func(insights.PlayRow) bool) []library.Track {
	var out []library.Track
	seen := make(map[string]struct{})
	for _, p := range m.insights.plays {
		if !keep(p) {
			continue
		}
		if _, dup := seen[p.Hash]; dup {
			continue
		}
		seen[p.Hash] = struct{}{}
		if t, ok := m.library.index.FindByHash(p.Hash); ok {
			out = append(out, t)
		}
	}
	return out
}

// insightsPlayedTracks resolves a Rhythm row. Rhythm is a windowed panel, so it
// reads the windowed plays rather than the all-time timeline: an hour column
// drawn from the last 7 days must not open a track heard a year ago.
func (m Model) insightsPlayedTracks(row insightsRow) []library.Track {
	if row.drill == drillRhythmGenre {
		want := insightsGenreKey(row.key)
		return m.insightsPlayTracks(func(p insights.PlayRow) bool {
			t, ok := m.library.index.FindByHash(p.Hash)
			if !ok {
				return false
			}
			for _, g := range library.SplitGenres(t.Genre) {
				if insightsGenreKey(g) == want {
					return true
				}
			}
			return false
		})
	}

	want, err := strconv.Atoi(row.key)
	if err != nil {
		return nil
	}
	loc := m.insights.location()
	return m.insightsPlayTracks(func(p insights.PlayRow) bool {
		start := time.Unix(p.StartAt, 0).In(loc)
		if row.drill == drillWeekday {
			return int(start.Weekday()) == want
		}
		return start.Hour() == want
	})
}

// insightsTopArtistTracks resolves a Concentration ladder row: the tracks
// played by the n artists that hold the most listening time, which is the group
// the rung stands for.
func (m Model) insightsTopArtistTracks(key string) []library.Track {
	n, err := strconv.Atoi(key)
	if err != nil || n <= 0 {
		return nil
	}
	top := make(map[string]struct{}, n)
	for i, e := range m.insights.snap.Rankings.ArtistsByHours {
		if i >= n {
			break
		}
		top[e.Name] = struct{}{}
	}
	return m.insightsPlayTracks(func(p insights.PlayRow) bool {
		_, ok := top[p.Artist]
		return ok
	})
}

// insightsSessionSetTracks resolves a set of sessions to the distinct tracks
// heard across them, in play order.
func (m Model) insightsSessionSetTracks(sessions []insights.SessionRow) []library.Track {
	ids := make(map[string]struct{}, len(sessions))
	for _, s := range sessions {
		ids[s.ID] = struct{}{}
	}
	return m.insightsPlayTracks(func(p insights.PlayRow) bool {
		_, ok := ids[p.SessionID]
		return ok
	})
}

// insightsSessionTracks returns one session's tracks in the exact order they
// were played, repeats included. This is the one drill that must not dedupe: a
// replay that drops the second listen is not the session that happened.
func (m Model) insightsSessionTracks(id string) []library.Track {
	if id == "" {
		return nil
	}
	var out []library.Track
	for _, p := range m.insights.plays {
		if p.SessionID != id {
			continue
		}
		if t, ok := m.library.index.FindByHash(p.Hash); ok {
			out = append(out, t)
		}
	}
	return out
}

// insightsRutBreakerTracks builds a set weighted away from the artists that
// already hold the top of the time ranking. Artists are visited round-robin,
// least-heard first, so one prolific outsider cannot simply become the new rut.
func (m Model) insightsRutBreakerTracks(limit int) []library.Track {
	top := make(map[string]struct{}, insightsRutTopArtists)
	for i, e := range m.insights.snap.Rankings.ArtistsByHours {
		if i >= insightsRutTopArtists {
			break
		}
		top[e.Name] = struct{}{}
	}

	byArtist := map[string][]library.Track{}
	plays := map[string]int{}
	for _, t := range m.library.index.Tracks() {
		if _, dominant := top[t.Artist]; dominant {
			continue
		}
		byArtist[t.Artist] = append(byArtist[t.Artist], t)
		plays[t.Artist] += t.PlayCount
	}

	artists := make([]string, 0, len(byArtist))
	for a := range byArtist {
		artists = append(artists, a)
	}
	sort.Slice(artists, func(i, j int) bool {
		if plays[artists[i]] != plays[artists[j]] {
			return plays[artists[i]] < plays[artists[j]]
		}
		return artists[i] < artists[j]
	})
	for _, a := range artists {
		ts := byArtist[a]
		sort.Slice(ts, func(i, j int) bool {
			if ts[i].PlayCount != ts[j].PlayCount {
				return ts[i].PlayCount < ts[j].PlayCount
			}
			return ts[i].Hash < ts[j].Hash
		})
	}

	out := make([]library.Track, 0, min(limit, len(m.library.index.Tracks())))
	for round := 0; len(out) < limit; round++ {
		took := false
		for _, a := range artists {
			if round >= len(byArtist[a]) {
				continue
			}
			out = append(out, byArtist[a][round])
			took = true
			if len(out) == limit {
				return out
			}
		}
		if !took {
			break
		}
	}
	return out
}

// insightsCoverageShelf is one artist's never-played tracks: the unit the
// Coverage anti-rut draw weights over.
type insightsCoverageShelf struct {
	artist string
	tracks []library.Track
}

// insightsCoverageAntiRutTracks draws up to limit never-played tracks weighted
// toward the dustiest shelves. A shelf is an artist, and its weight is the
// square of the unheard tracks it still holds: squaring is what makes this a
// weighting rather than a uniform sample, because it leaves a *track* on a
// 40-track shelf ten times likelier than one on a 4-track shelf. The weight
// falls as a shelf is drawn from, so the biggest pile leads without swallowing
// the batch, and the draw is random rather than ordered so pressing ! twice
// does not hand back the same 25 tracks.
func (m Model) insightsCoverageAntiRutTracks(limit int) []library.Track {
	if limit <= 0 {
		return nil
	}

	byArtist := map[string][]library.Track{}
	// The index holds only non-missing tracks (db.ReadTracks filters them out),
	// so never-played is the only condition left to apply here.
	for _, t := range m.library.index.Tracks() {
		if t.PlayCount == 0 {
			byArtist[t.Artist] = append(byArtist[t.Artist], t)
		}
	}

	// Map order is randomized per run, so the shelves and the tracks on them are
	// sorted before any draw: without that a seeded source would still produce a
	// different set each time.
	shelves := make([]insightsCoverageShelf, 0, len(byArtist))
	for artist, ts := range byArtist {
		sort.Slice(ts, func(i, j int) bool { return ts[i].Hash < ts[j].Hash })
		shelves = append(shelves, insightsCoverageShelf{artist: artist, tracks: ts})
	}
	sort.Slice(shelves, func(i, j int) bool {
		if len(shelves[i].tracks) != len(shelves[j].tracks) {
			return len(shelves[i].tracks) > len(shelves[j].tracks)
		}
		return shelves[i].artist < shelves[j].artist
	})

	next := m.insightsRand
	if next == nil {
		next = rand.IntN
	}

	total := 0
	for _, s := range shelves {
		total += len(s.tracks) * len(s.tracks)
	}

	out := make([]library.Track, 0, limit)
	for len(out) < limit && total > 0 {
		r, cum, pick := next(total), 0, len(shelves)-1
		for i, s := range shelves {
			cum += len(s.tracks) * len(s.tracks)
			if r < cum {
				pick = i
				break
			}
		}

		ts := shelves[pick].tracks
		total -= len(ts) * len(ts)
		j := next(len(ts))
		out = append(out, ts[j])
		ts[j] = ts[len(ts)-1]
		ts = ts[:len(ts)-1]
		total += len(ts) * len(ts)

		// An emptied shelf is dropped rather than left at weight zero, so the
		// fallback index above can never land on a shelf with nothing on it.
		if len(ts) == 0 {
			shelves = append(shelves[:pick], shelves[pick+1:]...)
			continue
		}
		shelves[pick].tracks = ts
	}
	return out
}

// insightsBucketTracks resolves a Momentum or Discovery row against the
// all-time timeline. firstEverOnly narrows it to the plays that were a track's
// first, which is the set Discovery's "first heard" rows stand for. The bucket
// size comes from the snapshot the row was drawn from rather than from the live
// control, so a drill can never be keyed at one resolution and resolved at
// another.
func (m Model) insightsBucketTracks(key string, firstEverOnly bool) []library.Track {
	loc := m.insights.location()
	res := m.insights.snap.Trends.Resolution
	var out []library.Track
	seen := make(map[string]struct{})
	for _, r := range m.insights.timeline {
		if firstEverOnly && !r.FirstEver {
			continue
		}
		if insightsBucketKey(res, time.Unix(r.StartAt, 0).In(loc)) != key {
			continue
		}
		if _, dup := seen[r.Hash]; dup {
			continue
		}
		seen[r.Hash] = struct{}{}
		if t, ok := m.library.index.FindByHash(r.Hash); ok {
			out = append(out, t)
		}
	}
	return out
}
