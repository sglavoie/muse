package artwork

import (
	"context"
	"hash/fnv"
	"image"
	"image/color"
	"math"
)

// Synthetic draws a deterministic abstract cover for seed: a two-hue diagonal
// gradient with a soft disc. Fixtures and demos use it so they can show art
// without reading any file.
func Synthetic(seed string) *Art {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	sum := h.Sum32()
	hue1 := float64(sum % 360)
	hue2 := math.Mod(hue1+40+float64(sum>>9%80), 360)
	cx := 0.3 + float64(sum>>3%40)/100
	cy := 0.3 + float64(sum>>13%40)/100
	const n = 96
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	for y := range n {
		for x := range n {
			t := (float64(x) + float64(y)) / (2 * n)
			hue := hue1*(1-t) + hue2*t
			v := 0.35 + 0.35*t
			dx, dy := float64(x)/n-cx, float64(y)/n-cy
			if d := math.Hypot(dx, dy); d < 0.22 {
				v = math.Min(1, v+0.45*(1-d/0.22))
			}
			r, g, b := hsvToRGB(hue, 0.65, v)
			img.SetRGBA(x, y, color.RGBA{R: uint8(r * 255), G: uint8(g * 255), B: uint8(b * 255), A: 255})
		}
	}
	return New(img)
}

// SyntheticLoader serves a [Synthetic] cover for every album.
type SyntheticLoader struct{}

// Load returns the synthetic cover for the request's album.
func (SyntheticLoader) Load(_ context.Context, req Request) (*Art, error) {
	seed := req.Album
	if seed == "" {
		seed = req.Path
	}
	return Synthetic(seed), nil
}

// Instant reports that synthetic covers cost nothing to make, so callers need
// not wait for focus to settle before asking for one.
func (SyntheticLoader) Instant() bool { return true }
