package artwork

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/ansi/kitty"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = c.R, c.G, c.B, c.A
	}
	return img
}

func TestNewNormalizesToSquare(t *testing.T) {
	art := New(solid(300, 120, color.RGBA{R: 200, G: 40, B: 40, A: 255}))
	if b := art.Image.Bounds(); b.Dx() != Size || b.Dy() != Size {
		t.Fatalf("normalized size = %v, want %dx%d", b, Size, Size)
	}
	if got := art.Image.RGBAAt(Size/2, Size/2); got.R != 200 || got.G != 40 {
		t.Fatalf("resize changed a solid color: %v", got)
	}
}

func TestAccentColorPicksTheVividHue(t *testing.T) {
	// A grey sleeve with an orange band: the accent is orange, not grey.
	img := solid(100, 100, color.RGBA{R: 90, G: 90, B: 90, A: 255})
	for y := 40; y < 60; y++ {
		for x := range 100 {
			img.SetRGBA(x, y, color.RGBA{R: 240, G: 130, B: 20, A: 255})
		}
	}
	c, ok := AccentColor(img)
	if !ok {
		t.Fatal("no accent found on a cover with a vivid band")
	}
	if !(c.R > c.G && c.G > c.B) {
		t.Fatalf("accent %v is not orange", c)
	}
	if _, ok := AccentColor(solid(64, 64, color.RGBA{R: 128, G: 128, B: 128, A: 255})); ok {
		t.Fatal("a greyscale cover must report no accent")
	}
}

func TestBlocksDrawsTwoPixelsPerCell(t *testing.T) {
	img := solid(8, 8, color.RGBA{R: 10, G: 20, B: 30, A: 255})
	for x := range 8 {
		for y := 4; y < 8; y++ {
			img.SetRGBA(x, y, color.RGBA{R: 250, G: 0, B: 0, A: 255})
		}
	}
	out := Blocks(img, 4, 2)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	for _, line := range lines {
		if w := ansi.StringWidth(line); w != 4 {
			t.Fatalf("line width %d, want 4: %q", w, line)
		}
		if !strings.HasSuffix(line, "\x1b[m") {
			t.Fatalf("line does not reset its colors: %q", line)
		}
	}
	if !strings.Contains(lines[0], "38;2;10;20;30;48;2;10;20;30m▀") {
		t.Fatalf("top row should be the dark half: %q", lines[0])
	}
	if !strings.Contains(lines[1], "38;2;250;0;0;48;2;250;0;0m▀") {
		t.Fatalf("bottom row should be the red half: %q", lines[1])
	}
	if Blocks(nil, 4, 2) != "" || Blocks(img, 0, 2) != "" {
		t.Fatal("degenerate input must draw nothing")
	}
}

func TestKittyPlaceholdersAddressEveryCell(t *testing.T) {
	out := KittyPlaceholders(42, 3, 2)
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d rows, want 2", len(lines))
	}
	for row, line := range lines {
		if !strings.HasPrefix(line, "\x1b[38;5;42m") {
			t.Fatalf("row %d does not carry the image id: %q", row, line)
		}
		if w := ansi.StringWidth(line); w != 3 {
			t.Fatalf("row %d is %d cells wide, want 3", row, w)
		}
		for col := range 3 {
			cell := string([]rune{kitty.Placeholder, kitty.Diacritic(row), kitty.Diacritic(col)})
			if !strings.Contains(line, cell) {
				t.Fatalf("row %d is missing cell %d", row, col)
			}
		}
	}
	if KittyPlaceholders(0, 3, 2) != "" || KittyPlaceholders(MaxKittyID+1, 3, 2) != "" {
		t.Fatal("ids outside 1..MaxKittyID must draw nothing")
	}
}

func TestKittyTransmitCreatesVirtualPlacement(t *testing.T) {
	seq, err := KittyTransmit(17, solid(16, 16, color.RGBA{A: 255}), 8, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(seq, "\x1b_Ga=d,d=I,i=17") {
		t.Fatalf("transmit must first free the id: %q", seq[:40])
	}
	for _, want := range []string{"a=T", "i=17", "U=1", "c=8", "r=4", "f=100"} {
		if !strings.Contains(seq, want) {
			t.Fatalf("transmit sequence lacks %q", want)
		}
	}
	if _, err := KittyTransmit(0, solid(4, 4, color.RGBA{}), 8, 4); err == nil {
		t.Fatal("id 0 must be rejected")
	}
}

func writePNG(t *testing.T, path string, img image.Image) {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFileLoaderUsesSidecarAndCaches(t *testing.T) {
	music := t.TempDir()
	track := filepath.Join(music, "01 Song.flac")
	if err := os.WriteFile(track, []byte("not audio"), 0o644); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(music, "Cover.PNG"), solid(64, 64, color.RGBA{R: 20, G: 180, B: 90, A: 255}))

	loader := &FileLoader{CacheDir: t.TempDir()}
	req := Request{Path: track, Album: "Green", Artist: "Band"}
	art, err := loader.Load(context.Background(), req)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := art.Image.RGBAAt(5, 5); got.G != 180 {
		t.Fatalf("sidecar not used: %v", got)
	}
	if !art.HasAccent {
		t.Fatal("a vivid green cover should have an accent")
	}

	// The cache now answers even if the sidecar disappears.
	if err := os.Remove(filepath.Join(music, "Cover.PNG")); err != nil {
		t.Fatal(err)
	}
	if _, err := loader.Load(context.Background(), req); err != nil {
		t.Fatalf("cached Load: %v", err)
	}
}

func TestFileLoaderRemembersMissingArt(t *testing.T) {
	music := t.TempDir()
	track := filepath.Join(music, "song.mp3")
	if err := os.WriteFile(track, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	loader := &FileLoader{CacheDir: cache}
	req := Request{Path: track, Album: "Bare"}
	if _, err := loader.Load(context.Background(), req); !errors.Is(err, ErrNoArt) {
		t.Fatalf("want ErrNoArt, got %v", err)
	}
	_, none := loader.paths(req.Key())
	if _, err := os.Stat(none); err != nil {
		t.Fatalf("negative result not cached: %v", err)
	}
	// A cover added later is still found once the negative entry is gone.
	if err := os.Remove(none); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(music, "folder.png"), solid(32, 32, color.RGBA{R: 255, A: 255}))
	if _, err := loader.Load(context.Background(), req); err != nil {
		t.Fatalf("Load after adding art: %v", err)
	}
}

func TestRequestKeySharesAlbumsWithinAFolder(t *testing.T) {
	a := Request{Path: "/m/Album/01.flac", Album: "Record", Artist: "X"}
	b := Request{Path: "/m/Album/02.flac", Album: "record", Artist: "Y"}
	if a.Key() != b.Key() {
		t.Fatal("tracks of one album in one folder must share a cover")
	}
	c := Request{Path: "/m/Other/01.flac", Album: "Record"}
	if a.Key() == c.Key() {
		t.Fatal("same album name in another folder must not share a cover")
	}
	d := Request{Path: "/m/Album/03.flac"}
	e := Request{Path: "/m/Album/04.flac"}
	if d.Key() == e.Key() {
		t.Fatal("tracks without an album are their own key")
	}
}

func TestSyntheticIsDeterministic(t *testing.T) {
	a, b := Synthetic("Gamma"), Synthetic("Gamma")
	if !bytes.Equal(a.Image.Pix, b.Image.Pix) {
		t.Fatal("same seed must draw the same cover")
	}
	if bytes.Equal(a.Image.Pix, Synthetic("Delta").Image.Pix) {
		t.Fatal("different seeds should draw different covers")
	}
}

func TestBlendCrossFadesBetweenCovers(t *testing.T) {
	a := solid(40, 40, color.RGBA{R: 200, A: 255})
	b := solid(20, 20, color.RGBA{B: 100, A: 255})
	for _, tc := range []struct {
		t    float64
		want color.RGBA
	}{
		{0, color.RGBA{R: 200, A: 255}},
		{0.5, color.RGBA{R: 100, B: 50, A: 255}},
		{1, color.RGBA{B: 100, A: 255}},
		{2, color.RGBA{B: 100, A: 255}},
	} {
		out := Blend(a, b, 4, 6, tc.t)
		if out.Bounds().Dx() != 4 || out.Bounds().Dy() != 6 {
			t.Fatalf("t=%.1f: size %v", tc.t, out.Bounds())
		}
		if got := out.RGBAAt(2, 3); got != tc.want {
			t.Errorf("t=%.1f: got %v, want %v", tc.t, got, tc.want)
		}
	}
}
