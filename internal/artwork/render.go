package artwork

import (
	"bytes"
	"fmt"
	"image"
	"strings"

	"github.com/charmbracelet/x/ansi/kitty"
)

// Blocks draws img into cols×rows terminal cells with upper half blocks: each
// cell shows two vertically stacked pixels, the top one as the foreground and
// the bottom one as the background. Terminal cells are about twice as tall as
// they are wide, so cols = 2×rows draws a square cover. Each line resets its
// colors at the end so the art never bleeds into neighbouring text.
func Blocks(img image.Image, cols, rows int) string {
	if img == nil || cols <= 0 || rows <= 0 {
		return ""
	}
	px := Resize(img, cols, rows*2)
	var b strings.Builder
	b.Grow(rows * cols * 40)
	for y := range rows {
		if y > 0 {
			b.WriteByte('\n')
		}
		for x := range cols {
			t := px.PixOffset(x, 2*y)
			u := px.PixOffset(x, 2*y+1)
			fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%d;48;2;%d;%d;%dm▀",
				px.Pix[t], px.Pix[t+1], px.Pix[t+2],
				px.Pix[u], px.Pix[u+1], px.Pix[u+2])
		}
		b.WriteString("\x1b[m")
	}
	return b.String()
}

// MaxKittyID is the largest image ID [KittyPlaceholders] can address. IDs are
// carried in a 256-color foreground index, which survives every color
// profile a terminal with Kitty graphics advertises.
const MaxKittyID = 255

// KittyTransmit returns the escape sequence that uploads img to the terminal
// as image id and creates a virtual placement of cols×rows cells for it. The
// image appears wherever [KittyPlaceholders] for the same id are drawn. Any
// image the terminal already holds under id is replaced.
func KittyTransmit(id int, img image.Image, cols, rows int) (string, error) {
	if id <= 0 || id > MaxKittyID || img == nil || cols <= 0 || rows <= 0 {
		return "", fmt.Errorf("artwork: invalid kitty placement %d (%dx%d)", id, cols, rows)
	}
	var out bytes.Buffer
	// Free the previous image under this id before replacing it.
	out.WriteString(kittyDelete(id))
	err := kitty.EncodeGraphics(&out, img, &kitty.Options{
		Action:           kitty.TransmitAndPut,
		ID:               id,
		Format:           kitty.PNG,
		Transmission:     kitty.Direct,
		Chunk:            true,
		Quite:            2,
		VirtualPlacement: true,
		Columns:          cols,
		Rows:             rows,
	})
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// KittyDeleteAll returns the sequence that frees every image muse uploaded.
func KittyDeleteAll(ids []int) string {
	var b strings.Builder
	for _, id := range ids {
		b.WriteString(kittyDelete(id))
	}
	return b.String()
}

func kittyDelete(id int) string {
	return fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id)
}

// KittyPlaceholders returns cols×rows Unicode placeholder cells for image id.
// A terminal with Kitty graphics draws the matching slice of the image in each
// cell; every other terminal shows nothing useful, so callers only emit these
// after deciding the terminal supports them. Each cell names its row and
// column explicitly, which keeps the art intact however the line is diffed.
func KittyPlaceholders(id, cols, rows int) string {
	if id <= 0 || id > MaxKittyID || cols <= 0 || rows <= 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(rows * (cols*7 + 16))
	for y := range rows {
		if y > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "\x1b[38;5;%dm", id)
		for x := range cols {
			b.WriteRune(kitty.Placeholder)
			b.WriteRune(kitty.Diacritic(y))
			b.WriteRune(kitty.Diacritic(x))
		}
		b.WriteString("\x1b[39m")
	}
	return b.String()
}
