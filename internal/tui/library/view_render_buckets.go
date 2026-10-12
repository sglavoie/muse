package library

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/sglavoie/muse/internal/tui/buckets"
	"github.com/sglavoie/muse/internal/tui/filter"
	"github.com/sglavoie/muse/internal/tui/text"
)

// chunkIntoBuckets interleaves bucket headers and dividers into the rendered
// rows, returning them unchanged when bucketing is off.
func (v Panel) chunkIntoBuckets(lines []string, display []displayTrack, start, maxRows int) []string {
	if v.BucketLabel == nil {
		return lines
	}
	labels := make([]string, len(lines))
	for i := range lines {
		labels[i] = v.BucketLabel(display[start+i].track)
	}
	sticky := ""
	if len(labels) > 0 {
		sticky = labels[0]
	}
	return buckets.Chunk(lines, labels, buckets.ChunkOptions{
		StickyBucket:     sticky,
		MaxRows:          maxRows,
		DividerEvery:     buckets.DividerEvery,
		MakeDivider:      v.renderDivider,
		MakeBucketHeader: v.renderBucketHeader,
	})
}

// bucketWindowBudget replays the row budget buckets.Chunk would consume for a
// window of labels, so a caller can ask whether a given data row still fits.
type bucketWindowBudget struct {
	remaining        int
	prevBucket       string
	dataSinceDivider int
}

// takeChrome charges one row to a bucket header or divider. Chunk never emits
// chrome as the last visible row, so nothing is charged when fewer than two
// rows remain; the result reports whether the row was charged.
func (b *bucketWindowBudget) takeChrome() bool {
	if b.remaining < 2 {
		return false
	}
	b.remaining--
	return true
}

// takeChromeBefore charges whichever of a bucket header or a periodic divider
// Chunk would emit above the next data row.
func (b *bucketWindowBudget) takeChromeBefore(label string) {
	if label != "" && label != b.prevBucket {
		b.prevBucket = label
		if b.takeChrome() {
			b.dataSinceDivider = 0
		}
		return
	}
	if buckets.DividerEvery > 0 && b.dataSinceDivider >= buckets.DividerEvery && b.takeChrome() {
		b.dataSinceDivider = 0
	}
}

func (b *bucketWindowBudget) takeDataRow() {
	b.remaining--
	b.dataSinceDivider++
}

// bucketedWindowIncludesRow reports whether the data row at rowIdx is still
// drawn once the bucket headers and dividers above it have consumed part of
// the maxRows budget.
func bucketedWindowIncludesRow(labels []string, rowIdx, maxRows int) bool {
	if rowIdx < 0 || rowIdx >= len(labels) || maxRows <= 0 {
		return false
	}
	b := bucketWindowBudget{remaining: maxRows}
	if labels[0] != "" {
		b.takeChrome() // sticky bucket header above the first row
		b.prevBucket = labels[0]
	}
	for i, label := range labels {
		if b.remaining <= 0 {
			return false
		}
		b.takeChromeBefore(label)
		if i == rowIdx {
			return true
		}
		b.takeDataRow()
	}
	return false
}

func (v Panel) renderNoMatches() string {
	lines := strings.Split(filter.NoMatchesMessage(v.Filter.Text != "", v.TagFilterActive), "\n")
	for i, line := range lines {
		line = text.Truncate(line, v.Width)
		lines[i] = strings.Repeat(" ", max(0, (v.Width-ansi.StringWidth(line))/2)) + line
	}
	return strings.Join(lines, "\n")
}
