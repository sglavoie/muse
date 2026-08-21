package insights

import "sort"

// rankDeltaTopN is how deep the Time vs Plays comparison goes. The panel is
// about the artists the user would name themselves, not the long tail.
const rankDeltaTopN = 15

// computeRankings ranks artists twice — by play count and by listening time —
// and measures how far each one moves between the two. The divergence is the
// whole point of the panel: pressing play often and spending time with
// somebody are different things.
func computeRankings(plays []PlayRow) Rankings {
	if len(plays) == 0 {
		return Rankings{}
	}

	totals := map[string]EntityStat{}
	totalSecs := 0
	for _, p := range plays {
		if p.Artist == "" {
			continue
		}
		secs := nonNegative(p.ListenedSecs)
		e := totals[p.Artist]
		e.Name = p.Artist
		e.Plays++
		e.ListenedSecs += secs
		totals[p.Artist] = e
		totalSecs += secs
	}
	if len(totals) == 0 {
		return Rankings{}
	}

	byPlays := make([]EntityStat, 0, len(totals))
	for _, e := range totals {
		byPlays = append(byPlays, e)
	}
	byHours := append([]EntityStat(nil), byPlays...)

	sort.Slice(byPlays, func(i, j int) bool {
		a, b := byPlays[i], byPlays[j]
		if a.Plays != b.Plays {
			return a.Plays > b.Plays
		}
		if a.ListenedSecs != b.ListenedSecs {
			return a.ListenedSecs > b.ListenedSecs
		}
		return a.Name < b.Name
	})
	sort.Slice(byHours, func(i, j int) bool {
		a, b := byHours[i], byHours[j]
		if a.ListenedSecs != b.ListenedSecs {
			return a.ListenedSecs > b.ListenedSecs
		}
		if a.Plays != b.Plays {
			return a.Plays > b.Plays
		}
		return a.Name < b.Name
	})

	hourRank := make(map[string]int, len(byHours))
	for i := range byHours {
		byHours[i].Rank = i + 1
		hourRank[byHours[i].Name] = i + 1
	}
	for i := range byPlays {
		byPlays[i].Rank = i + 1
	}

	r := Rankings{ArtistsByPlays: byPlays, ArtistsByHours: byHours}

	depth := min(rankDeltaTopN, len(byPlays))
	for _, e := range byPlays[:depth] {
		delta := RankDelta{
			Name:      e.Name,
			PlayRank:  e.Rank,
			HoursRank: hourRank[e.Name],
		}
		delta.Delta = delta.PlayRank - delta.HoursRank
		if delta.Delta != 0 {
			r.MovedCount++
		}
		r.Deltas = append(r.Deltas, delta)
	}

	r.Ladder = computeLadder(byHours, totalSecs)

	topFive := 0
	for _, e := range byHours[:min(5, len(byHours))] {
		topFive += e.ListenedSecs
	}
	r.TopFiveShare = safeDiv(float64(topFive), float64(totalSecs))

	return r
}

// computeLadder counts how few artists it takes to account for each share of
// total listening time. byHours must already be sorted descending by time.
func computeLadder(byHours []EntityStat, totalSecs int) Ladder {
	ladder := Ladder{TotalArtists: len(byHours)}
	if totalSecs <= 0 {
		return ladder
	}

	targets := []struct {
		share float64
		out   *int
	}{
		{0.25, &ladder.Artists25},
		{0.50, &ladder.Artists50},
		{0.75, &ladder.Artists75},
		{0.90, &ladder.Artists90},
	}

	running := 0
	next := 0
	for i, e := range byHours {
		running += e.ListenedSecs
		share := safeDiv(float64(running), float64(totalSecs))
		for next < len(targets) && share >= targets[next].share {
			*targets[next].out = i + 1
			next++
		}
		if next == len(targets) {
			break
		}
	}

	return ladder
}
