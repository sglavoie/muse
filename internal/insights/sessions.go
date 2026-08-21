package insights

import "sort"

// sessionTopN is how many opener/closer artists the Sessions panel shows.
const sessionTopN = 5

// computeSessions summarizes listening sessions. Durations are wall-clock
// (start of the first play to end of the last), which is longer than the sum
// of listened seconds whenever the user paused or walked away.
func computeSessions(sessions []SessionRow) SessionStats {
	if len(sessions) == 0 {
		return SessionStats{}
	}

	s := SessionStats{
		Count:     len(sessions),
		Durations: make([]int, 0, len(sessions)),
	}
	totalPlays := 0
	openers := map[string]EntityStat{}
	closers := map[string]EntityStat{}

	for _, row := range sessions {
		duration := nonNegative(int(row.EndedAt - row.StartedAt))
		s.Durations = append(s.Durations, duration)
		if duration > s.LongestSecs {
			s.LongestSecs = duration
		}
		s.TotalSecs += nonNegative(row.ListenedSecs)
		totalPlays += nonNegative(row.Plays)

		if row.Plays <= 1 {
			s.SinglePlay++
		}
		if row.DistinctArtists == 1 {
			s.SingleArtistCount++
		}

		tally(openers, row.OpenerArtist, row.ListenedSecs)
		tally(closers, row.CloserArtist, row.ListenedSecs)
	}

	sort.Ints(s.Durations)
	s.MedianSecs = median(s.Durations)
	s.SingleArtistShare = safeDiv(float64(s.SingleArtistCount), float64(s.Count))
	s.AvgPlays = safeDiv(float64(totalPlays), float64(s.Count))
	s.TopOpeners = topEntities(openers, sessionTopN)
	s.TopClosers = topEntities(closers, sessionTopN)

	return s
}

func tally(into map[string]EntityStat, name string, secs int) {
	if name == "" {
		return
	}
	e := into[name]
	e.Name = name
	e.Plays++
	e.ListenedSecs += nonNegative(secs)
	into[name] = e
}

// topEntities returns the n highest-count entries, most frequent first.
func topEntities(counts map[string]EntityStat, n int) []EntityStat {
	if len(counts) == 0 {
		return nil
	}
	all := make([]EntityStat, 0, len(counts))
	for _, e := range counts {
		all = append(all, e)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Plays != all[j].Plays {
			return all[i].Plays > all[j].Plays
		}
		return all[i].Name < all[j].Name
	})
	all = all[:min(n, len(all))]
	for i := range all {
		all[i].Rank = i + 1
	}
	return all
}

// median expects an ascending slice and averages the two middle values on an
// even count.
func median(sorted []int) int {
	n := len(sorted)
	if n == 0 {
		return 0
	}
	if n%2 == 1 {
		return sorted[n/2]
	}
	return (sorted[n/2-1] + sorted[n/2]) / 2
}
