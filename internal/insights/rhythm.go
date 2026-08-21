package insights

import "time"

// computeRhythm buckets plays by the local hour and weekday of their *start*.
// Using played_at directly would misfile every play that crosses an hour
// boundary, which on the real library is roughly one play in eleven.
func computeRhythm(plays []PlayRow, groups []GenreGroup, loc *time.Location) Rhythm {
	if len(plays) == 0 && len(groups) == 0 {
		return Rhythm{}
	}

	r := Rhythm{Genres: groups}
	for _, g := range groups {
		if len(g.Variants) > 1 {
			r.GenresUnclean = true
			break
		}
	}
	if len(plays) == 0 {
		return r
	}

	for i := range r.Hours {
		r.Hours[i].Hour = i
	}
	for i := range r.Weekdays {
		r.Weekdays[i].Weekday = time.Weekday(i)
	}

	tracks := make(map[string]struct{}, len(plays))
	for _, p := range plays {
		secs := nonNegative(p.ListenedSecs)
		start := time.Unix(p.StartAt, 0).In(loc)

		hour := &r.Hours[start.Hour()]
		hour.Plays++
		hour.ListenedSecs += secs

		day := &r.Weekdays[int(start.Weekday())]
		day.Plays++
		day.ListenedSecs += secs

		r.TotalPlays++
		r.TotalSecs += secs
		if p.Hash != "" {
			tracks[p.Hash] = struct{}{}
		}
	}
	r.DistinctTracks = len(tracks)

	for i, h := range r.Hours {
		if h.Plays > r.PeakHourPlays {
			r.PeakHourPlays = h.Plays
			r.PeakHour = i
		}
		if h.Plays == 0 {
			r.QuietHours = append(r.QuietHours, i)
		}
	}

	top, bottom := 0, 0
	for i, d := range r.Weekdays {
		r.Weekdays[i].Share = safeDiv(float64(d.Plays), float64(r.TotalPlays))
		if d.Plays > r.Weekdays[top].Plays {
			top = i
		}
		if d.Plays < r.Weekdays[bottom].Plays {
			bottom = i
		}
	}
	r.TopWeekday = r.Weekdays[top]
	r.BottomWeekday = r.Weekdays[bottom]

	return r
}
