package statusbar

import "math"

// Meter is one frame of the level meter shown in place of ▶ while playing:
// a spectrum, one bar per frequency band, lowest frequency on the left.
// Each value runs from 0 (at rest) to 1 (a full bar).
type Meter struct {
	Bands []float64
}

// meterGlyphs are the spectrum's bar heights; the lowest is rest, so the
// meter stays visible through silence.
var meterGlyphs = []rune("▁▂▃▄▅▆▇█")

// levelMeterNarrowBelow is the window width under which neighbouring bands
// share a bar, halving the meter so the song's title keeps its room.
const levelMeterNarrowBelow = 70

// renderLevelMeter draws the spectrum, one cell per band, or one per pair of
// bands (the louder of the two) in a narrow window.
func (s StatusBar) renderLevelMeter(ctx statusBarRenderContext, m Meter) (string, int) {
	bands := m.Bands
	if ctx.width > 0 && ctx.width < levelMeterNarrowBelow {
		bands = pairwiseMax(bands)
	}
	out := make([]rune, len(bands))
	for i, v := range bands {
		out[i] = meterGlyph(v)
	}
	return ctx.barSuccess.Render(string(out)), len(out)
}

// pairwiseMax halves bands, keeping the louder of each neighbouring pair.
func pairwiseMax(bands []float64) []float64 {
	out := make([]float64, 0, (len(bands)+1)/2)
	for i := 0; i < len(bands); i += 2 {
		v := bands[i]
		if i+1 < len(bands) {
			v = math.Max(v, bands[i+1])
		}
		out = append(out, v)
	}
	return out
}

// meterGlyph picks the bar for v, clamped to 0..1 with NaN at rest.
func meterGlyph(v float64) rune {
	if math.IsNaN(v) {
		v = 0
	}
	v = math.Min(1, math.Max(0, v))
	return meterGlyphs[int(math.Round(v*float64(len(meterGlyphs)-1)))]
}
