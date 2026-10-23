package text

// TrackWidths shares song-identity priorities across track tables. Detail is
// an optional date, play-count, or queue-source column.
type TrackWidths struct {
	Position, Title, Artist, Album, Duration, Detail int
}

// TrackColumns allocates supported table widths (40+ columns). Fixed gutters
// use two marker cells, three to five position cells when requested, and seven clock
// cells. Song identity grows to 39 title / 25 artist cells before secondary
// columns appear. While there is too little room for a useful secondary column,
// its space remains padding beside the clock; widening never shortens identity.
// detailMin is always visible; detailFull appears once its complete width fits.
func TrackColumns(width int, position bool, detailMin, detailFull int) TrackWidths {
	fixed := 2 + 7 + 2 // marker, clock, title/artist/clock separators
	if position {
		fixed += 4
	}
	if detailMin > 0 {
		fixed += detailMin + 1
	}
	available := max(0, width-fixed)
	identity := min(available, 64)
	c := TrackWidths{Artist: identity * 2 / 5, Duration: 7, Detail: detailMin}
	c.Title = identity - c.Artist
	spare := available - identity
	if position {
		c.Position = 3
		if spare < 2 {
			c.Duration += spare
			return c
		}
		c.Position = 5
		spare -= 2
	}
	detailCost := detailFull - detailMin
	if detailMin == 0 && detailFull > 0 {
		detailCost++
	}
	if spare < detailCost {
		c.Duration += spare
		return c
	}
	spare -= detailCost
	c.Detail = detailFull
	// Eight album cells plus one separator is the smallest useful album column.
	if spare < 9 {
		c.Duration += spare
		return c
	}
	c.Album = min(spare-1, 20)
	spare -= c.Album + 1
	c.grow(spare)
	return c
}

func (c *TrackWidths) grow(spare int) {
	// A repeating allocation order keeps each column monotonic even at rounding
	// boundaries, while approaching the familiar 40/35/25 proportions.
	c.Title += spare / 20 * 8
	c.Artist += spare / 20 * 7
	c.Album += spare / 20 * 5
	for _, col := range []int{0, 1, 2, 0, 1, 0, 2, 1, 0, 1, 2, 0, 1, 0, 2, 1, 0, 1, 2, 0}[:spare%20] {
		switch col {
		case 0:
			c.Title++
		case 1:
			c.Artist++
		case 2:
			c.Album++
		}
	}
}
