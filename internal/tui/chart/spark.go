package chart

import "math"

// SparkPoint is one sample of a SparklineGaps strip. An absent point renders
// as a gap glyph rather than a zero-height bar, so a month with no data cannot
// be mistaken for a month with no listening.
type SparkPoint struct {
	Value   float64
	Present bool
}

// Sparkline renders values as a one-line strip of block glyphs, at most width
// cells wide. More values than cells are averaged into equal buckets.
func Sparkline(values []float64, width int, p Palette) string {
	return styleLine(sparkSegs(presentPoints(values), width), p)
}

func sparklinePlain(values []float64, width int) string {
	return plainLine(sparkSegs(presentPoints(values), width))
}

// SparklineGaps renders points the way Sparkline does, except that absent
// points render as a muted gap glyph.
func SparklineGaps(points []SparkPoint, width int, p Palette) string {
	return styleLine(sparkSegs(points, width), p)
}

func sparklineGapsPlain(points []SparkPoint, width int) string {
	return plainLine(sparkSegs(points, width))
}

func presentPoints(values []float64) []SparkPoint {
	points := make([]SparkPoint, len(values))
	for i, v := range values {
		points[i] = SparkPoint{Value: v, Present: true}
	}
	return points
}

func sparkSegs(points []SparkPoint, width int) []seg {
	width = SafeWidth(width)
	if width == 0 || len(points) == 0 {
		return nil
	}
	buckets := bucketPoints(points, width)
	maxVal := 0.0
	for _, b := range buckets {
		if b.Present {
			maxVal = math.Max(maxVal, b.Value)
		}
	}
	segs := make([]seg, 0, len(buckets))
	for _, b := range buckets {
		if !b.Present {
			segs = append(segs, seg{glyphGap, roleMuted})
			continue
		}
		level := 0
		if maxVal > 0 {
			level = int(math.Round(b.Value / maxVal * float64(len(sparkLevels)-1)))
			level = min(max(level, 0), len(sparkLevels)-1)
		}
		segs = append(segs, seg{sparkLevels[level], roleBar})
	}
	return segs
}

// bucketPoints sanitizes points and, when there are more of them than cells,
// folds them into width contiguous buckets. A bucket is absent only when every
// point in it is absent; otherwise it averages the present values so a partial
// bucket is not dragged down by its gaps.
func bucketPoints(points []SparkPoint, width int) []SparkPoint {
	if len(points) <= width {
		out := make([]SparkPoint, len(points))
		for i, pt := range points {
			out[i] = SparkPoint{Value: SafeMagnitude(pt.Value), Present: pt.Present}
		}
		return out
	}
	out := make([]SparkPoint, width)
	for i := range out {
		start := i * len(points) / width
		end := (i + 1) * len(points) / width
		sum, n := 0.0, 0
		for _, pt := range points[start:end] {
			if pt.Present {
				sum += SafeMagnitude(pt.Value)
				n++
			}
		}
		if n > 0 {
			out[i] = SparkPoint{Value: sum / float64(n), Present: true}
		}
	}
	return out
}
