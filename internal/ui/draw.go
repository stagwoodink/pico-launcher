package ui

import (
	"image"
	"image/color"
	_ "image/png"
	"math"
	"os"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
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

// caraselMetrics returns the focused tile height and the pixel spacing
// (gap) between adjacent tile centers, shared by drawing and drag input so
// a swipe moves exactly as far as it visually should.
func caraselMetrics(w, h int) (centerH, gap float64) {
	centerH = float64(h) * 0.55
	if byWidth := float64(w) * 0.45; byWidth < centerH {
		centerH = byWidth
	}
	return centerH, centerH * 0.9
}

// drawCarasel renders the cover-art carasel across the full window,
// coverflow-style: tiles slide continuously with g.pos (not just snapping
// between integer selections), shrinking and fading the further they are
// from center. Carts with no cover image get a hairline placeholder tile
// with their title centered.
func (g *Game) drawCarasel(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	cx, cy := float64(w)/2, float64(h)/2
	centerH, gap := caraselMetrics(w, h)
	n := len(g.allCarts)

	base := int(math.Round(g.pos))
	frac := g.pos - float64(base)

	const visibleRadius = 2.05
	for off := -2; off <= 2; off++ {
		dist := float64(off) - frac
		absDist := math.Abs(dist)
		if absDist > visibleRadius {
			continue
		}
		idx := wrap(base+off, n)
		cart := g.allCarts[idx]

		scale := 1 - absDist*0.28
		if scale < 0.1 {
			scale = 0.1
		}
		alpha := float32(1 - absDist*0.45)
		if alpha < 0 {
			alpha = 0
		}
		tileH := centerH * scale
		tileX := cx + dist*gap

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

// drawPlaceholderCart draws a hairline outline shaped like an actual PICO-8
// cartridge label — rounded top corners, a chamfered bottom-right corner —
// with an art box and a label box (holding the cart's title) inside, for
// carts that have no cover image of their own.
func (g *Game) drawPlaceholderCart(screen *ebiten.Image, x, y, w, h float64, alpha float32, name string) {
	c := color.RGBA{255, 255, 255, uint8(255 * alpha)}
	strokeWidth := float32(w) * 0.006
	if strokeWidth < 1 {
		strokeWidth = 1
	}
	strokeOpts := &vector.StrokeOptions{Width: strokeWidth}
	drawOpts := &vector.DrawPathOptions{AntiAlias: true}
	drawOpts.ColorScale.ScaleWithColor(c)

	fx, fy, fw, fh := float32(x), float32(y), float32(w), float32(h)
	radius := fw * 0.02
	chamfer := fw * 0.04

	var outline vector.Path
	outline.MoveTo(fx+radius, fy)
	outline.LineTo(fx+fw-radius, fy)
	outline.ArcTo(fx+fw, fy, fx+fw, fy+radius, radius)
	outline.LineTo(fx+fw, fy+fh-chamfer)
	outline.LineTo(fx+fw-chamfer, fy+fh)
	outline.LineTo(fx, fy+fh)
	outline.LineTo(fx, fy+radius)
	outline.ArcTo(fx, fy, fx+radius, fy, radius)
	outline.Close()
	vector.StrokePath(screen, &outline, strokeOpts, drawOpts)

	// Art box: where a cover screenshot would sit.
	sideMargin := w * 0.09
	artX, artY := x+sideMargin, y+h*0.06
	artSize := w - sideMargin*2
	var artBox vector.Path
	artBox.MoveTo(float32(artX), float32(artY))
	artBox.LineTo(float32(artX+artSize), float32(artY))
	artBox.LineTo(float32(artX+artSize), float32(artY+artSize))
	artBox.LineTo(float32(artX), float32(artY+artSize))
	artBox.Close()
	vector.StrokePath(screen, &artBox, strokeOpts, drawOpts)

	// Label box: title area below the art box.
	labelY := artY + artSize + h*0.04
	labelH := (y + h*0.92) - labelY
	var labelBox vector.Path
	labelBox.MoveTo(float32(artX), float32(labelY))
	labelBox.LineTo(float32(artX+artSize), float32(labelY))
	labelBox.LineTo(float32(artX+artSize), float32(labelY+labelH))
	labelBox.LineTo(float32(artX), float32(labelY+labelH))
	labelBox.Close()
	vector.StrokePath(screen, &labelBox, strokeOpts, drawOpts)

	if g.font == nil {
		return
	}
	label := g.font.truncate(strings.ToUpper(name), artSize-12, listScale)
	g.font.draw(screen, label, artX+artSize/2, labelY+labelH/2, listScale, c, alignCenter)
}

const listPadding = 24

// listRowHeight is fixed (not screen-relative) so line spacing looks the
// same at any window size; how many rows fit adapts to the window instead.
func listRowHeight() float64 {
	return float64(glyphCellH*listScale) * 3.5
}

// drawList renders every cart's title as a left-aligned row spanning the
// full window. The selection highlight bar stays fixed at center; rows
// slide continuously past it with g.pos, and whichever row is currently
// under the bar gets its text inverted.
func (g *Game) drawList(screen *ebiten.Image) {
	if g.font == nil {
		return
	}
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	n := len(g.allCarts)

	rowH := listRowHeight()
	barY := float64(h) / 2
	radius := int(float64(h)/2/rowH) + 1
	textX := float64(listPadding)
	maxTextW := float64(w) - listPadding*2

	base := int(math.Round(g.pos))
	frac := g.pos - float64(base)

	fillRect(screen, 0, barY-rowH/2, float64(w), rowH, white)

	for off := -radius; off <= radius; off++ {
		idx := wrap(base+off, n)
		cart := g.allCarts[idx]
		rowY := barY + (float64(off)-frac)*rowH

		col := white
		if math.Abs(rowY-barY) < rowH/2 {
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
