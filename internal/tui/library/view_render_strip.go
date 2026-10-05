package library

import (
	"math"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/sglavoie/muse/internal/tui/strip"
	"github.com/sglavoie/muse/internal/tui/text"
)

// renderStateStrip builds the unified single-line state strip carrying
// the view label, sort, filter, and counts. total is the full track
// count before filtering; filtered is the count after filtering is
// applied (equal to total when no filter is active).
func (v Panel) renderStateStrip(total, filtered int) string {
	label := v.StripViewLabel
	if label == "" {
		label = "Library"
	}
	sort := strip.SortChunk{Label: v.StripSortLabel, Indicator: v.StripSortIndicator}
	f := strip.FilterChunk{Active: v.Filter.Active, Text: v.Filter.Text}
	if v.Filter.Active {
		if v.Filter.Text == "" {
			f.Hint = v.Styles.TextSecondary.Render("a: artist  t: title  l: album  g: genre")
		} else {
			f.InputView = v.Filter.View()
		}
	}
	counts := strip.CountsChunk{
		Filtered:  filtered,
		Total:     total,
		Filtering: v.Filter.Text != "",
	}
	var sel *strip.SelectionChunk
	if n := len(v.SelectedHashes); n > 0 {
		sel = &strip.SelectionChunk{Count: n, Duration: v.selectedDuration()}
	}
	return strip.Render(v.Width, []string{label}, sort, f, counts, sel, "", v.Styles)
}

// selectedDuration includes selections hidden by filters, just like bulk actions.
func (v Panel) selectedDuration() string {
	// Embedded track lists (for example Insights) may have no library index.
	var overrides map[string]float64
	if v.Index.Len() == 0 {
		overrides = make(map[string]float64, len(v.OverrideTracks))
		for _, track := range v.OverrideTracks {
			overrides[track.Hash] = track.Duration
		}
	}
	var seconds float64
	unknown := false
	for hash := range v.SelectedHashes {
		track, ok := v.Index.FindByHash(hash)
		duration := track.Duration
		if !ok {
			duration = overrides[hash]
		}
		if duration <= 0 || math.IsNaN(duration) || math.IsInf(duration, 0) {
			unknown = true
			continue
		}
		seconds += duration
	}
	duration := text.FormatPositionSecs(seconds)
	if unknown {
		duration += "+"
	}
	return duration
}

// renderDivider returns a single divider line spanning the visible content
// width. It is rendered with the dim Divider style and uses U+2500 box-drawing.
func (v Panel) renderDivider() string {
	w := v.Width
	if w <= 0 {
		w = 1
	}
	return v.Styles.Divider.Render(strings.Repeat("─", w))
}

// renderBucketHeader returns a styled bucket-header line padded to the
// content width so subsequent lines align cleanly.
func (v Panel) renderBucketHeader(label string) string {
	if label == "" {
		return ""
	}
	w := v.Width
	if w <= 0 {
		w = ansi.StringWidth(label)
	}
	padded := label
	if pw := ansi.StringWidth(label); pw < w {
		padded = label + strings.Repeat(" ", w-pw)
	}
	return v.Styles.BucketHeader.Render(padded)
}
