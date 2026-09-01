package ui

import (
	"image"
	"image/color"
	_ "image/png"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	messageScale = 4
	listScale    = 2
)

var (
	bg    = color.RGBA{16, 16, 20, 255}
	white = color.RGBA{255, 255, 255, 255}
	black = color.RGBA{0, 0, 0, 255}
)

func (g *Game) Draw(screen *ebiten.Image) {
	screen.Fill(bg)

	switch g.state {
	case stateAwaitPico8Tab, statePickingPico8:
		g.drawMessage(screen, "hit [tab] to select your pico-8 app")
	case stateAwaitCartsTab, statePickingCarts:
		g.drawMessage(screen, "hit [tab] to select your carts folder")
	case stateNoCarts:
		g.drawMessage(screen, "no carts found")
	case stateBrowsing:
		switch g.mode {
		case modeCarasel:
			g.drawCarasel(screen)
		case modeList:
			g.drawList(screen)
		}
	}
}

func (g *Game) drawMessage(screen *ebiten.Image, msg string) {
	if g.font == nil {
		return
	}
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	g.font.draw(screen, strings.ToUpper(msg), float64(w)/2, float64(h)/2, messageScale, white, alignCenter)
}

// cartAspect matches PICO-8's own cart cover art (160x205), used to size
// the placeholder tile for carts with no cover image.
const cartAspect = 160.0 / 205.0

// drawCarasel renders the cover-art carasel across the full window, with
// the selection centered and neighbors dimmed to the sides. Carts with no
// cover image get a hairline placeholder tile with their title centered.
func (g *Game) drawCarasel(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	cx, cy := float64(w)/2, float64(h)/2
	centerH := float64(h) * 0.55
	if byWidth := float64(w) * 0.45; byWidth < centerH {
		centerH = byWidth
	}
	gap := centerH * 0.9

	for off := -1; off <= 1; off++ {
		idx := wrap(g.idx+off, len(g.allCarts))
		cart := g.allCarts[idx]

		scale := 1.0
		alpha := float32(1.0)
		if off != 0 {
			scale = 0.7
			alpha = 0.35
		}
		tileH := centerH * scale
		tileX := cx + float64(off)*gap

		if img := g.thumbnail(cart.Image); img != nil {
			iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
			s := tileH / float64(ih)
			opt := &ebiten.DrawImageOptions{}
			opt.GeoM.Scale(s, s)
			opt.GeoM.Translate(tileX-float64(iw)*s/2, cy-float64(ih)*s/2)
			opt.ColorScale.ScaleAlpha(alpha)
			screen.DrawImage(img, opt)
			continue
		}

		tileW := tileH * cartAspect
		g.drawPlaceholderCart(screen, tileX-tileW/2, cy-tileH/2, tileW, tileH, alpha, cart.Name)
	}
}

// drawPlaceholderCart draws a white hairline cart outline with the cart's
// title centered inside, for carts that have no cover image.
func (g *Game) drawPlaceholderCart(screen *ebiten.Image, x, y, w, h float64, alpha float32, name string) {
	const border = 2.0
	c := color.RGBA{255, 255, 255, uint8(255 * alpha)}
	fillRect(screen, x, y, w, border, c)
	fillRect(screen, x, y+h-border, w, border, c)
	fillRect(screen, x, y, border, h, c)
	fillRect(screen, x+w-border, y, border, h, c)

	if g.font == nil {
		return
	}
	label := g.font.truncate(strings.ToUpper(name), w-16, listScale)
	g.font.draw(screen, label, x+w/2, y+h/2, listScale, c, alignCenter)
}

const listPadding = 24

// drawList renders every cart's title as a left-aligned row spanning the
// full window, with the current selection always in the middle row.
func (g *Game) drawList(screen *ebiten.Image) {
	if g.font == nil {
		return
	}
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()

	// Fixed row height so line spacing looks the same at any window size;
	// how many rows fit (above/below the selection) adapts to h instead.
	rowH := float64(glyphCellH*listScale) * 3.5
	radius := int(float64(h) / 2 / rowH)
	textX := float64(listPadding)
	maxTextW := float64(w) - listPadding*2

	for off := -radius; off <= radius; off++ {
		idx := wrap(g.idx+off, len(g.allCarts))
		cart := g.allCarts[idx]
		rowY := float64(h)/2 + float64(off)*rowH

		col := white
		if off == 0 {
			fillRect(screen, 0, rowY-rowH/2, float64(w), rowH, white)
			col = black
		}

		name := g.font.truncate(strings.ToUpper(cart.Name), maxTextW, listScale)
		g.font.draw(screen, name, textX, rowY, listScale, col, alignStart)
	}
}

// thumbnail lazily loads and caches a cart's cover image.
func (g *Game) thumbnail(path string) *ebiten.Image {
	if path == "" {
		return nil
	}
	if img, ok := g.images[path]; ok {
		return img
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	src, _, err := image.Decode(f)
	if err != nil {
		return nil
	}
	img := ebiten.NewImageFromImage(src)
	g.images[path] = img
	return img
}

func fillRect(screen *ebiten.Image, x, y, w, h float64, c color.Color) {
	sub := ebiten.NewImage(int(w), int(h))
	sub.Fill(c)
	opt := &ebiten.DrawImageOptions{}
	opt.GeoM.Translate(x, y)
	screen.DrawImage(sub, opt)
}
