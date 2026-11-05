// Package artwork finds, caches and draws album cover art.
//
// Art is looked up lazily, one album at a time, the first time muse needs it:
// a cover image next to the music (cover.jpg, folder.png, …) wins, then the
// picture embedded in the file itself. Every result, including "this album has
// no art", is cached on disk so the lookup happens once per album.
//
// Drawing is terminal-agnostic: [Blocks] paints an image with colored half
// blocks that any truecolor terminal shows, and [KittyTransmit] with
// [KittyPlaceholders] shows the real image in terminals that implement the
// Kitty graphics protocol's Unicode placeholders.
package artwork

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Size is the edge length, in pixels, of the square every cover is normalized
// to. It is large enough for a full-screen Kitty placement and small enough to
// keep the cache and the in-memory copies cheap.
const Size = 320

// Art is one album's cover, normalized to a Size×Size square.
type Art struct {
	Image *image.RGBA
	// Accent is the cover's most characteristic vivid color. HasAccent is
	// false for greyscale or very dark covers that have none worth using.
	Accent    color.RGBA
	HasAccent bool
}

// New normalizes img into an Art: it is center-cropped to a square, resized to
// Size×Size, and its accent color is measured.
func New(img image.Image) *Art {
	square := Resize(CropSquare(img), Size, Size)
	accent, ok := AccentColor(square)
	return &Art{Image: square, Accent: accent, HasAccent: ok}
}

// CropSquare returns the largest centered square of img.
func CropSquare(img image.Image) image.Image {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == h {
		return img
	}
	side := min(w, h)
	x0 := b.Min.X + (w-side)/2
	y0 := b.Min.Y + (h-side)/2
	rect := image.Rect(x0, y0, x0+side, y0+side)
	if sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	}); ok {
		return sub.SubImage(rect)
	}
	out := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(out, out.Bounds(), img, rect.Min, draw.Src)
	return out
}

// Resize scales img to w×h with an area-averaging (box) filter, which keeps
// downscaled covers smooth instead of aliased. Upscaling degrades gracefully
// to nearest-neighbour sampling.
func Resize(img image.Image, w, h int) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || w <= 0 || h <= 0 {
		return out
	}
	src := toRGBA(img)
	sx := float64(sw) / float64(w)
	sy := float64(sh) / float64(h)
	for y := range h {
		y0 := int(float64(y) * sy)
		y1 := max(y0+1, int(math.Ceil(float64(y+1)*sy)))
		y1 = min(y1, sh)
		for x := range w {
			x0 := int(float64(x) * sx)
			x1 := max(x0+1, int(math.Ceil(float64(x+1)*sx)))
			x1 = min(x1, sw)
			var r, g, bl, a, n uint32
			for yy := y0; yy < y1; yy++ {
				off := src.PixOffset(b.Min.X+x0, b.Min.Y+yy)
				for xx := x0; xx < x1; xx++ {
					r += uint32(src.Pix[off])
					g += uint32(src.Pix[off+1])
					bl += uint32(src.Pix[off+2])
					a += uint32(src.Pix[off+3])
					off += 4
					n++
				}
			}
			if n == 0 {
				continue
			}
			o := out.PixOffset(x, y)
			out.Pix[o] = channelMean(r, n)
			out.Pix[o+1] = channelMean(g, n)
			out.Pix[o+2] = channelMean(bl, n)
			out.Pix[o+3] = channelMean(a, n)
		}
	}
	return out
}

// channelMean averages n 8-bit samples summing to sum.
func channelMean(sum, n uint32) uint8 {
	return uint8(min(sum/n, 255))
}

// toRGBA returns img as an *image.RGBA that shares bounds with img, converting
// only when necessary.
func toRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	return out
}

// AccentColor picks the cover's most prominent vivid hue. Pixels are grouped
// into hue bins weighted by saturation and brightness, so a small bright
// sticker on a grey sleeve can still win while a large muddy area cannot. It
// reports false when too little of the image is colorful to choose from.
func AccentColor(img image.Image) (color.RGBA, bool) {
	small := Resize(img, 48, 48)
	const bins = 18
	var weight [bins]float64
	var sumR, sumG, sumB [bins]float64
	total := 0.0
	for i := 0; i+3 < len(small.Pix); i += 4 {
		r, g, b := float64(small.Pix[i])/255, float64(small.Pix[i+1])/255, float64(small.Pix[i+2])/255
		total++
		h, s, v := rgbToHSV(r, g, b)
		if s < 0.25 || v < 0.25 {
			continue
		}
		w := s * s * v
		bin := int(h/360*bins) % bins
		weight[bin] += w
		sumR[bin] += r * w
		sumG[bin] += g * w
		sumB[bin] += b * w
	}
	best := 0
	for i := 1; i < bins; i++ {
		// Neighbouring bins reinforce each other so a gradient is not split.
		if binScore(weight, i) > binScore(weight, best) {
			best = i
		}
	}
	w, red, green, blue := weight[best], sumR[best], sumG[best], sumB[best]
	if total == 0 || w/total < 0.015 {
		return color.RGBA{}, false
	}
	return vivid(color.RGBA{R: unitToByte(red / w), G: unitToByte(green / w), B: unitToByte(blue / w), A: 255}), true
}

// unitToByte maps a 0–1 channel value to 0–255, clamping out-of-range input.
func unitToByte(v float64) uint8 {
	return uint8(math.Round(math.Min(1, math.Max(0, v)) * 255))
}

func binScore(weight [18]float64, i int) float64 {
	n := len(weight)
	return weight[i] + 0.5*(weight[(i+1)%n]+weight[(i+n-1)%n])
}

// vivid lifts a muted pick toward a usable interface color: saturation and
// brightness are kept inside a band that reads as an accent on dark and light
// canvases alike, while the hue is preserved exactly.
func vivid(c color.RGBA) color.RGBA {
	h, s, v := rgbToHSV(float64(c.R)/255, float64(c.G)/255, float64(c.B)/255)
	s = math.Min(math.Max(s, 0.45), 0.85)
	v = math.Min(math.Max(v, 0.70), 0.95)
	r, g, b := hsvToRGB(h, s, v)
	return color.RGBA{R: unitToByte(r), G: unitToByte(g), B: unitToByte(b), A: 255}
}

func rgbToHSV(r, g, b float64) (h, s, v float64) {
	maxC := math.Max(r, math.Max(g, b))
	minC := math.Min(r, math.Min(g, b))
	v = maxC
	d := maxC - minC
	if maxC > 0 {
		s = d / maxC
	}
	if d == 0 {
		return 0, s, v
	}
	switch maxC {
	case r:
		h = math.Mod((g-b)/d, 6)
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	h *= 60
	if h < 0 {
		h += 360
	}
	return h, s, v
}

func hsvToRGB(h, s, v float64) (r, g, b float64) {
	c := v * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := v - c
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return r + m, g + m, b + m
}
