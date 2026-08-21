package tui

import (
	"fmt"
	"sort"
	"time"

	"github.com/sglavoie/muse/internal/db"
	"github.com/sglavoie/muse/internal/insights"
	"github.com/sglavoie/muse/internal/library"
)

// uitestNow is the instant every fixture measures its history against. It is a
// single call site so the whole fixture history stays internally consistent
// even if a run straddles midnight.
func uitestNow() time.Time { return time.Now() }

// uitestInsights is an in-memory insightsReader that derives a whole listening
// history from the fixture's library tracks: a track with PlayCount n produces
// n plays walking backwards from its LastPlayedAt, gathered into sessions.
// Nothing here touches a database.
type uitestInsights struct{ tracks []library.Track }

// uitestPlayStep is how far back each successive session sits. The odd five
// hours on top of the three days are what give the fixture a rhythm at all: a
// whole number of days would land every play of a track in one hour-of-day and
// one weekday, leaving the punchcard a single column.
const uitestPlayStep = 3*86400 + 5*3600

// uitestSessionGap is the distance between two plays inside one session. It is
// small next to uitestPlayStep, so a session is a cluster rather than a smear,
// and it is what puts the fixture's sessions in more than one duration bucket.
const uitestSessionGap = 25 * 60

// uitestSessionSizes is the repeating shape of the fixture's sessions. The lone
// play is the important one: a one-play session is what the Sessions panel
// calls a false start, and a fixture without any cannot exercise that row.
var uitestSessionSizes = []int{1, 2, 3}

func (r *uitestInsights) allPlays() []insights.PlayRow {
	var out []insights.PlayRow
	for _, t := range r.tracks {
		session, slot := 0, 0
		for range t.PlayCount {
			if slot == uitestSessionSizes[session%len(uitestSessionSizes)] {
				session, slot = session+1, 0
			}
			out = append(out, insights.PlayRow{
				Hash:           t.Hash,
				StartAt:        t.LastPlayedAt - int64(session)*uitestPlayStep - int64(slot)*uitestSessionGap,
				ListenedSecs:   int(t.Duration),
				CompletionRate: 1,
				SessionID:      fmt.Sprintf("%s-%d", t.Hash, session),
				Artist:         t.Artist,
				Album:          t.Album,
				Title:          t.Title,
			})
			slot++
		}
	}
	// The real readers all order by the corrected start, and the Momentum
	// re-listen drill replays a month in the order it was heard, so the fixture
	// has to promise the same ordering the SQL does.
	sort.SliceStable(out, func(i, j int) bool { return out[i].StartAt < out[j].StartAt })
	return out
}

func (r *uitestInsights) ReadInsightPlays(sinceUnix int64) ([]insights.PlayRow, error) {
	all := r.allPlays()
	if sinceUnix <= 0 {
		return all, nil
	}
	var out []insights.PlayRow
	for _, p := range all {
		if p.StartAt >= sinceUnix {
			out = append(out, p)
		}
	}
	return out, nil
}

func (r *uitestInsights) ReadInsightGenres(sinceUnix int64) ([]insights.GenreRow, error) {
	plays, _ := r.ReadInsightPlays(sinceUnix)
	byHash := make(map[string]library.Track, len(r.tracks))
	for _, t := range r.tracks {
		byHash[t.Hash] = t
	}

	rows := make(map[string]*insights.GenreRow)
	get := func(genre string) *insights.GenreRow {
		row := rows[genre]
		if row == nil {
			row = &insights.GenreRow{Genre: genre}
			rows[genre] = row
		}
		return row
	}
	for _, t := range r.tracks {
		for _, g := range library.SplitGenres(t.Genre) {
			get(g).Tracks++
		}
	}
	for _, p := range plays {
		for _, g := range library.SplitGenres(byHash[p.Hash].Genre) {
			row := get(g)
			row.Plays++
			row.ListenedSecs += p.ListenedSecs
		}
	}

	out := make([]insights.GenreRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, *row)
	}
	return out, nil
}

func (r *uitestInsights) ReadInsightSessions(sinceUnix int64) ([]insights.SessionRow, error) {
	plays, _ := r.ReadInsightPlays(sinceUnix)
	order := make([]string, 0, len(plays))
	byID := make(map[string]*insights.SessionRow)
	artists := make(map[string]map[string]struct{})
	for _, p := range plays {
		s := byID[p.SessionID]
		if s == nil {
			s = &insights.SessionRow{
				ID:           p.SessionID,
				StartedAt:    p.StartAt,
				OpenerArtist: p.Artist,
				OpenerTitle:  p.Title,
			}
			byID[p.SessionID] = s
			artists[p.SessionID] = make(map[string]struct{})
			order = append(order, p.SessionID)
		}
		s.Plays++
		s.ListenedSecs += p.ListenedSecs
		s.StartedAt = min(s.StartedAt, p.StartAt)
		s.EndedAt = max(s.EndedAt, p.StartAt+int64(p.ListenedSecs))
		s.CloserArtist = p.Artist
		s.CloserTitle = p.Title
		artists[p.SessionID][p.Artist] = struct{}{}
	}

	out := make([]insights.SessionRow, 0, len(order))
	for _, id := range order {
		s := byID[id]
		s.DistinctArtists = len(artists[id])
		out = append(out, *s)
	}
	return out, nil
}

func (r *uitestInsights) ReadInsightCoverage() ([]insights.CoverageRow, error) {
	out := make([]insights.CoverageRow, 0, len(r.tracks))
	for _, t := range r.tracks {
		out = append(out, insights.CoverageRow{
			Hash:         t.Hash,
			Artist:       t.Artist,
			Album:        t.Album,
			DurationSecs: t.Duration,
			PlayCount:    t.PlayCount,
			AddedAt:      t.CreatedAt,
			LastPlayedAt: t.LastPlayedAt,
		})
	}
	return out, nil
}

func (r *uitestInsights) ReadInsightTimeline() ([]insights.TimelineRow, error) {
	firstSeen := make(map[string]int64)
	for _, p := range r.allPlays() {
		if at, ok := firstSeen[p.Hash]; !ok || p.StartAt < at {
			firstSeen[p.Hash] = p.StartAt
		}
	}
	var out []insights.TimelineRow
	for _, p := range r.allPlays() {
		out = append(out, insights.TimelineRow{
			Hash:         p.Hash,
			StartAt:      p.StartAt,
			ListenedSecs: p.ListenedSecs,
			FirstEver:    firstSeen[p.Hash] == p.StartAt,
		})
	}
	return out, nil
}

// ReadUnverifiedDurations treats a fixture track with no duration as one that
// was never checked against its file, which is exactly the state the real
// duration_verified flag records.
func (r *uitestInsights) ReadUnverifiedDurations() ([]db.UnverifiedTrack, error) {
	var out []db.UnverifiedTrack
	for _, t := range r.tracks {
		if t.Duration <= 0 {
			out = append(out, db.UnverifiedTrack{Hash: t.Hash, Path: t.Path})
		}
	}
	return out, nil
}

// uitestMetadata is the fixture's metadata writer. It mutates the very slice
// the insights reader derives from, so a genre merge is visible to the reload
// that follows it — which is the whole point of exercising the merge here.
type uitestMetadata struct{ ins *uitestInsights }

func (w *uitestMetadata) UpdateTrackMetadata(hash, title, artist, album, genre string) error {
	for i := range w.ins.tracks {
		if w.ins.tracks[i].Hash != hash {
			continue
		}
		w.ins.tracks[i].Title = title
		w.ins.tracks[i].Artist = artist
		w.ins.tracks[i].Album = album
		w.ins.tracks[i].Genre = genre
		break
	}
	return nil
}

func (r *uitestInsights) ReadInsightQuality(minPlays, maxPlays int, _ float64) (insights.QualityCounts, error) {
	q := insights.QualityCounts{TracksTotal: len(r.tracks)}
	for _, t := range r.tracks {
		if t.Genre == "" {
			q.TracksWithoutGenre++
		}
		q.PlaysTotal += t.PlayCount
		if t.Genre == "" {
			q.PlaysWithoutGenre += t.PlayCount
		}
		if t.PlayCount >= minPlays && t.PlayCount <= maxPlays {
			q.DeepCutsCandidates++
			q.DeepCutsQualifying++
		}
	}
	return q, nil
}
