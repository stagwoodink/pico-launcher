package ui

import (
	"image"
	"image/color"
	_ "image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

const (
	messageScale = 4
	listScale    = 2
)

var (
	bg     = color.RGBA{16, 16, 20, 255}
	white  = color.RGBA{255, 255, 255, 255}
	black  = color.RGBA{0, 0, 0, 255}
	dimBar = color.RGBA{70, 70, 74, 255}
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
		g.drawBrowsing(screen)
	}
}

func (g *Game) drawMessage(screen *ebiten.Image, msg string) {
	if g.font == nil {
		return
	}
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	g.font.draw(screen, msg, float64(w)/2, float64(h)/2, messageScale, white, alignCenter)
}

func (g *Game) drawBrowsing(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	showCarasel := len(g.carasel) > 0
	showList := len(g.list) > 0

	switch {
	case showCarasel && showList:
		listW := g.listWidth(w)
		caraselX := listW + panelGutter
		g.drawList(screen, 0, listW, h)
		g.drawCarasel(screen, caraselX, w-caraselX, h)
	case showCarasel:
		g.drawCarasel(screen, 0, w, h)
	case showList:
		g.drawList(screen, 0, w, h)
	}
}

const (
	listPadding = 24
	panelGutter = 32 // hard whitespace between the list and carasel panels
)

// listWidth sizes the list panel to fit its longest title, capped so the
// carasel always keeps most of the screen.
func (g *Game) listWidth(screenW int) int {
	if g.font == nil || len(g.list) == 0 {
		return 0
	}
	maxW := 0.0
	for _, c := range g.list {
		w := g.font.width(c.Name, listScale)
		if w > maxW {
			maxW = w
		}
	}
	width := int(maxW) + listPadding*2
	if cap := screenW * 45 / 100; width > cap {
		width = cap
	}
	return width
}

// drawCarasel renders the cover-art carasel centered in [x, x+areaW),
// scaling the tile size to whichever dimension is tighter so the side
// covers never spill outside the carasel's own area.
func (g *Game) drawCarasel(screen *ebiten.Image, x, areaW, areaH int) {
	cx := float64(x) + float64(areaW)/2
	cy := float64(areaH) / 2
	centerH := float64(areaH) * 0.55
	if byWidth := float64(areaW) * 0.45; byWidth < centerH {
		centerH = byWidth
	}
	gap := centerH * 0.9

	for off := -1; off <= 1; off++ {
		idx := wrap(g.caraselIdx+off, len(g.carasel))
		cart := g.carasel[idx]
		img := g.thumbnail(cart.Image)
		if img == nil {
			continue
		}
		scale := 1.0
		alpha := float32(1.0)
		if off != 0 {
			scale = 0.7
			alpha = 0.35
		}
		iw, ih := img.Bounds().Dx(), img.Bounds().Dy()
		s := centerH * scale / float64(ih)
		opt := &ebiten.DrawImageOptions{}
		opt.GeoM.Scale(s, s)
		opt.GeoM.Translate(cx+float64(off)*gap-float64(iw)*s/2, cy-float64(ih)*s/2)
		opt.ColorScale.ScaleAlpha(alpha)
		screen.DrawImage(img, opt)
	}
}

// drawList renders the plain-cart title list centered in [x, x+areaW), with
// the current selection always in the middle row.
func (g *Game) drawList(screen *ebiten.Image, x, areaW, areaH int) {
	if g.font == nil {
		return
	}
	// Fixed row height so line spacing looks the same at any window size;
	// how many rows fit (above/below the selection) adapts to areaH instead.
	rowH := float64(glyphCellH*listScale) * 2
	radius := int(float64(areaH) / 2 / rowH)
	textX := float64(x) + listPadding

	barColor := dimBar
	textOnBar := white
	if g.lastPanel == panelList {
		barColor = white
		textOnBar = black
	}

	for off := -radius; off <= radius; off++ {
		idx := wrap(g.listIdx+off, len(g.list))
		cart := g.list[idx]
		rowY := float64(areaH)/2 + float64(off)*rowH

		col := white
		if off == 0 {
			fillRect(screen, float64(x), rowY-rowH/2, float64(areaW), rowH, barColor)
			col = textOnBar
		}

		g.font.draw(screen, cart.Name, textX, rowY, listScale, col, alignStart)
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
