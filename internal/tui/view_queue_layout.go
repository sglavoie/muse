package tui

type queueColumns struct {
	Position int
	Title    int
	Artist   int
	Album    int
	Source   int
	Duration int
}

func computeQueueColumns(totalWidth int, hasSource bool) queueColumns {
	const (
		markerWidth = 2
		posWidth    = 4
		durWidth    = 10
	)
	cols := queueColumns{Position: posWidth, Duration: durWidth}

	sepCount := 4
	weights := []int{40, 20, 20}
	if hasSource {
		sepCount = 5
		weights = []int{30, 20, 20, 10}
	}

	available := totalWidth - markerWidth - sepCount
	if available <= 0 {
		cols.Position = 0
		cols.Duration = 0
		return cols
	}

	required := cols.Position + cols.Duration + len(weights)
	if available < required {
		deficit := required - available
		shrink := min(deficit, cols.Duration)
		cols.Duration -= shrink
		deficit -= shrink

		shrink = min(deficit, cols.Position)
		cols.Position -= shrink

	}

	flex := max(available-cols.Position-cols.Duration, 0)
	widths := distributeQueueFlex(flex, weights)
	cols.Title = widths[0]
	if len(widths) > 1 {
		cols.Artist = widths[1]
	}
	if len(widths) > 2 {
		cols.Album = widths[2]
	}
	if hasSource && len(widths) > 3 {
		cols.Source = widths[3]
	}
	return cols
}

func distributeQueueFlex(total int, weights []int) []int {
	widths := make([]int, len(weights))
	if total <= 0 || len(weights) == 0 {
		return widths
	}
	if total >= len(widths) {
		for i := range widths {
			widths[i] = 1
		}
		total -= len(widths)
	} else {
		for i := 0; i < total && i < len(widths); i++ {
			widths[i] = 1
		}
		return widths
	}

	weightTotal := 0
	for _, w := range weights {
		weightTotal += w
	}
	assigned := 0
	for i, w := range weights {
		add := total * w / weightTotal
		widths[i] += add
		assigned += add
	}
	for remaining := total - assigned; remaining > 0; remaining-- {
		widths[0]++
	}
	return widths
}

// assembleQueueRow joins pre-styled queue row columns using sep as the column
// separator. When hasSource is false the source column is omitted entirely.
func assembleQueueRow(sep string, hasSource bool, marker, pos, title, artist, album, source, dur string) string {
	if hasSource {
		return marker + pos + sep + title + sep + artist + sep + album + sep + source + sep + dur
	}
	return marker + pos + sep + title + sep + artist + sep + album + sep + dur
}
