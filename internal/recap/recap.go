package recap

import (
	"sort"
	"time"

	"github.com/sglavoie/muse/internal/insights"
)

// Sizes of the ranked lists a recap carries.
const (
	TopN         = 5  // artists and tracks shown on their pages
	PlaylistSize = 50 // tracks the recap saves as a playlist
	GenreN       = 6  // genres shown in the genre mix
)

// Input is everything Compute needs. Plays is the whole counted history, not
// just the period: "first heard" and the comparison with the period before
// both need to see past the period's edges.
type Input struct {
	Plays  []insights.PlayRow // all-time, start-corrected
	Loc    *time.Location     // nil is UTC
	Period Period
	// GenresOf returns a track's genres, already split. Nil means no genre
	// data, which only leaves the genre mix empty.
	GenresOf func(hash string) []string
}

// Totals are the period's headline numbers.
type Totals struct {
	Plays   int
	Secs    int
	Tracks  int
	Artists int
	Albums  int
	Days    int // distinct local days with at least one play
}

// Entry is one ranked artist, track or album. Hash names a representative
// track: the track itself, or the most-played track of an album, which is
// what a cover is looked up from.
type Entry struct {
	Name   string // artist name, track title, or album title
	Artist string // the track's or album's artist; empty for an artist entry
	Album  string // the track's album, or an artist's most-played album
	Hash   string
	Plays  int
	Secs   int
}

// Clock answers "when do I listen": plays by the local hour and weekday each
// one started in.
type Clock struct {
	Hours       [24]int
	Weekdays    [7]int // indexed by time.Weekday
	PeakHour    int
	PeakWeekday time.Weekday
}

// Streak is the longest run of consecutive local days with a play.
type Streak struct {
	Days       int
	Start, End time.Time // first and last day of the run, local midnights
}

// Discovery is the most-played track whose first play ever fell inside the
// period. NewTracks counts every track first heard in it.
type Discovery struct {
	Entry
	FirstHeard time.Time
	NewTracks  int
}

// RepeatDay is the most plays one track got in a single local day. Entry's
// Plays and Secs are that day's.
type RepeatDay struct {
	Entry
	Date time.Time
}

// GenreShare is one genre's slice of the period's tagged plays.
type GenreShare struct {
	Name  string
	Plays int
	Share float64
}

// DayCount is one local day of the period and how many plays started in it.
type DayCount struct {
	Date  time.Time
	Plays int
}

// Recap is the computed story of one period. A zero Totals.Plays means
// nothing was played in it and every other field is zero too.
type Recap struct {
	Period   Period
	Totals   Totals
	Previous Totals // the period before; zero when it had no plays
	// FirstPeriod is true when nothing at all was played before the period,
	// so there is no earlier listening to compare it with.
	FirstPeriod bool
	TopArtists  []Entry
	TopTracks   []Entry
	TopAlbum    Entry
	Clock       Clock
	Streak      Streak
	Discovery   Discovery
	RepeatDay   RepeatDay
	Genres      []GenreShare
	Daily       []DayCount // one per day of the period, silent days included
	Playlist    []string   // up to PlaylistSize track hashes, most played first
}

// Empty reports whether nothing was played in the period.
func (r Recap) Empty() bool { return r.Totals.Plays == 0 }

// Compute builds the recap for in.Period. It is pure and total: Compute of a
// zero Input is a zero-play recap.
func Compute(in Input) Recap {
	loc := location(in.Loc)
	period := in.Period
	if period.Start.IsZero() {
		return Recap{Period: period}
	}
	plays := within(in.Plays, period)
	r := Recap{Period: period}
	if len(plays) == 0 {
		return r
	}
	r.Totals = totals(plays, loc)
	r.Previous = totals(within(in.Plays, period.Previous()), loc)
	r.FirstPeriod = !playedBefore(in.Plays, period.Start.Unix())

	tracks := rankTracks(plays)
	r.TopTracks = head(tracks, TopN)
	r.Playlist = hashes(head(tracks, PlaylistSize))
	albums := rankAlbums(plays, tracks)
	if len(albums) > 0 {
		r.TopAlbum = albums[0]
	}
	r.TopArtists = rankArtists(plays, tracks, albums)
	r.Clock = clock(plays, loc)
	r.Daily = daily(plays, period, loc)
	r.Streak = longestStreak(r.Daily)
	r.Discovery = discovery(in.Plays, period, tracks, loc)
	r.RepeatDay = repeatDay(plays, loc)
	r.Genres = genreMix(plays, in.GenresOf)
	return r
}

// within returns the plays that started inside p.
func within(plays []insights.PlayRow, p Period) []insights.PlayRow {
	var out []insights.PlayRow
	for _, row := range plays {
		if p.Contains(row.StartAt) {
			out = append(out, row)
		}
	}
	return out
}

func playedBefore(plays []insights.PlayRow, unix int64) bool {
	for _, p := range plays {
		if p.StartAt < unix {
			return true
		}
	}
	return false
}

func secsOf(p insights.PlayRow) int { return max(p.ListenedSecs, 0) }

func totals(plays []insights.PlayRow, loc *time.Location) Totals {
	t := Totals{}
	tracks, artists, albums, days := set{}, set{}, set{}, map[time.Time]struct{}{}
	for _, p := range plays {
		t.Plays++
		t.Secs += secsOf(p)
		tracks.add(p.Hash)
		artists.add(p.Artist)
		if p.Album != "" {
			albums.add(albumKey(p.Artist, p.Album))
		}
		days[localDay(p.StartAt, loc)] = struct{}{}
	}
	t.Tracks, t.Artists, t.Albums, t.Days = len(tracks), len(artists), len(albums), len(days)
	return t
}

type set map[string]struct{}

func (s set) add(k string) {
	if k != "" {
		s[k] = struct{}{}
	}
}

func albumKey(artist, album string) string { return artist + "\x00" + album }

// rankTracks totals plays per track, most played first.
func rankTracks(plays []insights.PlayRow) []Entry {
	byHash := map[string]*Entry{}
	var order []*Entry
	for _, p := range plays {
		if p.Hash == "" {
			continue
		}
		e := byHash[p.Hash]
		if e == nil {
			e = &Entry{Name: p.Title, Artist: p.Artist, Album: p.Album, Hash: p.Hash}
			byHash[p.Hash] = e
			order = append(order, e)
		}
		e.Plays++
		e.Secs += secsOf(p)
	}
	return sortEntries(order)
}

// rankArtists ranks artists and gives each its most-played album, with that
// album's most-played track as the cover source. tracks and albums must be
// ranked.
func rankArtists(plays []insights.PlayRow, tracks, albums []Entry) []Entry {
	byName := map[string]*Entry{}
	var order []*Entry
	for _, p := range plays {
		if p.Artist == "" {
			continue
		}
		e := byName[p.Artist]
		if e == nil {
			e = &Entry{Name: p.Artist}
			byName[p.Artist] = e
			order = append(order, e)
		}
		e.Plays++
		e.Secs += secsOf(p)
	}
	ranked := head(sortEntries(order), TopN)
	for i := range ranked {
		for _, a := range albums {
			if a.Artist == ranked[i].Name {
				ranked[i].Album, ranked[i].Hash = a.Name, a.Hash
				break
			}
		}
		if ranked[i].Hash == "" {
			ranked[i].Hash = firstTrackBy(tracks, ranked[i].Name)
		}
	}
	return ranked
}

func firstTrackBy(tracks []Entry, artist string) string {
	for _, t := range tracks {
		if t.Artist == artist {
			return t.Hash
		}
	}
	return ""
}

// rankAlbums ranks albums by artist and title, each represented by its
// most-played track. tracks must be ranked.
func rankAlbums(plays []insights.PlayRow, tracks []Entry) []Entry {
	byKey := map[string]*Entry{}
	var order []*Entry
	for _, p := range plays {
		if p.Album == "" {
			continue
		}
		k := albumKey(p.Artist, p.Album)
		e := byKey[k]
		if e == nil {
			e = &Entry{Name: p.Album, Artist: p.Artist}
			byKey[k] = e
			order = append(order, e)
		}
		e.Plays++
		e.Secs += secsOf(p)
	}
	for _, t := range tracks {
		if e := byKey[albumKey(t.Artist, t.Album)]; e != nil && e.Hash == "" {
			e.Hash = t.Hash
		}
	}
	return sortEntries(order)
}

// sortEntries orders by plays, then time, then name and artist, so ties
// always break the same way.
func sortEntries(es []*Entry) []Entry {
	out := make([]Entry, len(es))
	for i, e := range es {
		out[i] = *e
	}
	sort.SliceStable(out, func(i, j int) bool { return entryLess(out[i], out[j]) })
	return out
}

func entryLess(a, b Entry) bool {
	if a.Plays != b.Plays {
		return a.Plays > b.Plays
	}
	if a.Secs != b.Secs {
		return a.Secs > b.Secs
	}
	if a.Name != b.Name {
		return a.Name < b.Name
	}
	return a.Artist < b.Artist
}

func head(es []Entry, n int) []Entry {
	if len(es) <= n {
		return es
	}
	return es[:n]
}

func hashes(es []Entry) []string {
	out := make([]string, len(es))
	for i, e := range es {
		out[i] = e.Hash
	}
	return out
}
