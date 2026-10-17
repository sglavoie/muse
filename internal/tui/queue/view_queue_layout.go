package queue

import (
	"github.com/charmbracelet/x/ansi"
	"strings"
)

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
		posWidth    = 5
		durWidth    = 10
	)
	cols := queueColumns{Position: posWidth, Duration: durWidth}
	if totalWidth >= 40 && totalWidth < 80 {
		flex := totalWidth - 2 - 3 - 7 - 3
		artist := flex * 2 / 5
		return queueColumns{Position: 3, Duration: 7, Title: flex - artist, Artist: artist}
	}

	sepCount := 4
	weights := []int{40, 35, 25}
	if hasSource {
		sepCount = 5
		weights = []int{40, 35, 25, 15}
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
	if total < len(widths) {
		for i := 0; i < total; i++ {
			widths[i] = 1
		}
		return widths
	}

	weightTotal := 0
	for _, w := range weights {
		weightTotal += w
	}
	// Title takes the rounding remainder, matching the other track tables.
	assigned := 0
	for i := 1; i < len(weights); i++ {
		widths[i] = max(1, total*weights[i]/weightTotal)
		assigned += widths[i]
	}
	widths[0] = total - assigned
	return widths
}

// assembleQueueRow joins pre-styled queue row columns using sep as the column
// separator. When hasSource is false the source column is omitted entirely.
func assembleQueueRow(sep string, hasSource bool, marker, pos, title, artist, album, source, dur string) string {
	cells := []string{marker + pos, title, artist}
	if ansi.StringWidth(album) > 0 {
		cells = append(cells, album)
	}
	if hasSource && ansi.StringWidth(source) > 0 {
		cells = append(cells, source)
	}
	return strings.Join(append(cells, dur), sep)
}
