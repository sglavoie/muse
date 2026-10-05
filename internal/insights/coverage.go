package insights

import (
	"sort"

	"github.com/sglavoie/muse/internal/library"
)

// dustyShelfTopN is how many unheard artist/album shelves Coverage surfaces.
const dustyShelfTopN = 10

// computeCoverage measures the library against the history: how much is owned
// but never heard, and how that compares in size to everything listened to so
// far. It is always all-time — "never played" is meaningless inside a window.
func computeCoverage(rows []CoverageRow, timeline []TimelineRow, trends Trends) Coverage {
	if len(rows) == 0 && len(timeline) == 0 {
		return Coverage{}
	}

	var c Coverage
	artists := map[string]ShelfStat{}
	albums := map[string]ShelfStat{}

	for _, row := range rows {
		c.TotalTracks++
		unheard := row.PlayCount <= 0
		secs := nonNegativeFloat(row.DurationSecs)
		if unheard {
			c.UnheardTracks++
			c.UnheardSecs += secs
		} else {
			c.HeardTracks++
		}
		shelve(artists, row.Artist, unheard, secs)
		if row.Album != "" {
			shelve(albums, library.AlbumKey(library.Track{Album: row.Album,
				AlbumArtist: row.AlbumArtist, Artist: row.Artist, Path: row.Path}), unheard, secs)
		}
	}

	for _, row := range timeline {
		c.ListenedSecs += float64(nonNegative(row.ListenedSecs))
	}

	c.HeardShare = safeDiv(float64(c.HeardTracks), float64(c.TotalTracks))
	c.UnheardVsHeard = safeDiv(c.UnheardSecs, c.ListenedSecs)
	c.MonthsToFinish = safeDiv(float64(c.UnheardTracks), trends.AvgMonthlyFirstPlays)
	c.DustyArtists = topShelves(artists, dustyShelfTopN)
	c.DustyAlbums = topShelves(albums, dustyShelfTopN)

	return c
}

func shelve(into map[string]ShelfStat, name string, unheard bool, secs float64) {
	if name == "" {
		return
	}
	s := into[name]
	s.Name = name
	s.Tracks++
	if unheard {
		s.Unheard++
		s.UnheardSecs += secs
	}
	into[name] = s
}

// topShelves returns the n dustiest shelves, most unheard tracks first.
// Shelves that are fully heard are dropped rather than ranked last.
func topShelves(shelves map[string]ShelfStat, n int) []ShelfStat {
	if len(shelves) == 0 {
		return nil
	}
	dusty := make([]ShelfStat, 0, len(shelves))
	for _, s := range shelves {
		if s.Unheard > 0 {
			dusty = append(dusty, s)
		}
	}
	if len(dusty) == 0 {
		return nil
	}
	sort.Slice(dusty, func(i, j int) bool {
		a, b := dusty[i], dusty[j]
		if a.Unheard != b.Unheard {
			return a.Unheard > b.Unheard
		}
		if a.UnheardSecs != b.UnheardSecs {
			return a.UnheardSecs > b.UnheardSecs
		}
		return a.Name < b.Name
	})
	return dusty[:min(n, len(dusty))]
}
