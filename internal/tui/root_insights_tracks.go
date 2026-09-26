package tui

import (
	"math/rand/v2"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
	instui "github.com/sglavoie/muse/internal/tui/insights"
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

// insightsDrillTracks resolves a briefing row to the concrete tracks behind it,
// so every bar is a query with a shape rather than a picture.
func (m Model) insightsDrillTracks(row instui.Row) []library.Track {
	// Every dormant row opens the whole dormant set: a one-track drill would
	// make E and S meaningless, and the point of the section is the batch.
	switch row.Drill {
	case instui.DrillDormant:
		return m.insights.Dormant
	case instui.DrillHour, instui.DrillWeekday, instui.DrillRhythmGenre:
		return m.insightsPlayedTracks(row)
	case instui.DrillTrendBucket, instui.DrillFirstPlaysBucket:
		return m.insightsBucketTracks(row.Key, row.Drill == instui.DrillFirstPlaysBucket)
	case instui.DrillArtistPlays:
		return m.insightsPlayTracks(func(p insights.PlayRow) bool { return p.Artist == row.Key })
	case instui.DrillTopArtists:
		return m.insightsTopArtistTracks(row.Key)
	case instui.DrillHealth:
		return m.insightsHealthTracks(row.Key)
	}

	// A Sessions row opens sessions, not tracks, but the batch actions still
	// need a track set behind it: E on the briefing row replays everything the
	// row stands for, in the order it was heard.
	if row.Drill.OpensSessions() {
		return m.insightsSessionSetTracks(instui.SessionsMatching(m.insights.Sessions, row.Drill, row.Key))
	}

	match := func(t library.Track) bool {
		if t.PlayCount != 0 {
			return false
		}
		switch row.Drill {
		case instui.DrillUnheardAll:
			return true
		case instui.DrillUnheardArtist:
			return t.Artist == row.Key
		case instui.DrillUnheardAlbum:
			return t.Album == row.Key
		case instui.DrillUnheardGenre:
			for _, g := range library.SplitGenres(t.Genre) {
				if strings.EqualFold(g, row.Key) {
					return true
				}
			}
		case instui.DrillAddedNeverPlayed:
			// Always monthly: the backlog is grouped by the month a track was
			// added, which has nothing to do with how the play history is bucketed.
			return t.CreatedAt > 0 && instui.BucketKey(insights.ResolutionMonthly,
				time.Unix(t.CreatedAt, 0).In(m.insights.Location())) == row.Key
		}
		return false
	}

	var out []library.Track
	for _, t := range m.library.Index.Tracks() {
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
	if err != nil || i < 0 || i >= len(m.insights.Health) {
		return nil
	}
	var out []library.Track
	for _, hash := range m.insights.Health[i].Hashes {
		if t, ok := m.library.Index.FindByHash(hash); ok {
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
	for _, p := range m.insights.Plays {
		if !keep(p) {
			continue
		}
		if _, dup := seen[p.Hash]; dup {
			continue
		}
		seen[p.Hash] = struct{}{}
		if t, ok := m.library.Index.FindByHash(p.Hash); ok {
			out = append(out, t)
		}
	}
	return out
}

// insightsPlayedTracks resolves a Rhythm row. Rhythm is a windowed panel, so it
// reads the windowed plays rather than the all-time timeline: an hour column
// drawn from the last 7 days must not open a track heard a year ago.
func (m Model) insightsPlayedTracks(row instui.Row) []library.Track {
	if row.Drill == instui.DrillRhythmGenre {
		want := instui.GenreKey(row.Key)
		return m.insightsPlayTracks(func(p insights.PlayRow) bool {
			t, ok := m.library.Index.FindByHash(p.Hash)
			if !ok {
				return false
			}
			for _, g := range library.SplitGenres(t.Genre) {
				if instui.GenreKey(g) == want {
					return true
				}
			}
			return false
		})
	}

	want, err := strconv.Atoi(row.Key)
	if err != nil {
		return nil
	}
	loc := m.insights.Location()
	return m.insightsPlayTracks(func(p insights.PlayRow) bool {
		start := time.Unix(p.StartAt, 0).In(loc)
		if row.Drill == instui.DrillWeekday {
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
	for i, e := range m.insights.Snap.Rankings.ArtistsByHours {
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
	for _, p := range m.insights.Plays {
		if p.SessionID != id {
			continue
		}
		if t, ok := m.library.Index.FindByHash(p.Hash); ok {
			out = append(out, t)
		}
	}
	return out
}

// insightsRutBreakerTracks builds a set weighted away from the artists that
// already hold the top of the time ranking. Artists are visited round-robin,
// least-heard first, so one prolific outsider cannot simply become the new rut.
func (m Model) insightsRutBreakerTracks(limit int) []library.Track {
	top := make(map[string]struct{}, instui.RutTopArtists)
	for i, e := range m.insights.Snap.Rankings.ArtistsByHours {
		if i >= instui.RutTopArtists {
			break
		}
		top[e.Name] = struct{}{}
	}

	byArtist := map[string][]library.Track{}
	plays := map[string]int{}
	for _, t := range m.library.Index.Tracks() {
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

	out := make([]library.Track, 0, min(limit, m.library.Index.Len()))
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
	for _, t := range m.library.Index.Tracks() {
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
	loc := m.insights.Location()
	res := m.insights.Snap.Trends.Resolution
	var out []library.Track
	seen := make(map[string]struct{})
	for _, r := range m.insights.Timeline {
		if firstEverOnly && !r.FirstEver {
			continue
		}
		if instui.BucketKey(res, time.Unix(r.StartAt, 0).In(loc)) != key {
			continue
		}
		if _, dup := seen[r.Hash]; dup {
			continue
		}
		seen[r.Hash] = struct{}{}
		if t, ok := m.library.Index.FindByHash(r.Hash); ok {
			out = append(out, t)
		}
	}
	return out
}
