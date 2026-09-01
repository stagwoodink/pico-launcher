package ui

import (
	"image"
	"image/color"
	_ "image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
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
	if g.face == nil {
		return
	}
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	opt := &text.DrawOptions{}
	opt.GeoM.Translate(float64(w)/2, float64(h)/2)
	opt.PrimaryAlign = text.AlignCenter
	opt.SecondaryAlign = text.AlignCenter
	opt.ColorScale.ScaleWithColor(color.White)
	text.Draw(screen, msg, g.face, opt)
}

func (g *Game) drawBrowsing(screen *ebiten.Image) {
	w, h := screen.Bounds().Dx(), screen.Bounds().Dy()
	showCarasel := len(g.carasel) > 0
	showList := len(g.list) > 0

	switch {
	case showCarasel && showList:
		listW := g.listWidth(w)
		g.drawList(screen, 0, listW, h)
		g.drawCarasel(screen, listW, w-listW, h)
	case showCarasel:
		g.drawCarasel(screen, 0, w, h)
	case showList:
		g.drawList(screen, 0, w, h)
	}
}

const listPadding = 24

// listWidth sizes the list panel to fit its longest title, capped so the
// carasel always keeps most of the screen.
func (g *Game) listWidth(screenW int) int {
	if g.face == nil || len(g.list) == 0 {
		return 0
	}
	maxW := 0.0
	for _, c := range g.list {
		w, _ := text.Measure(c.Name, g.face, 0)
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

// drawCarasel renders the cover-art carasel centered in [x, x+areaW).
func (g *Game) drawCarasel(screen *ebiten.Image, x, areaW, areaH int) {
	cx := float64(x) + float64(areaW)/2
	cy := float64(areaH) / 2
	centerH := float64(areaH) * 0.55
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
	if g.face == nil {
		return
	}
	const radius = 4 // rows shown above/below the selection
	rows := radius*2 + 1
	rowH := float64(areaH) / float64(rows)
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

		opt := &text.DrawOptions{}
		opt.GeoM.Translate(textX, rowY)
		opt.PrimaryAlign = text.AlignStart
		opt.SecondaryAlign = text.AlignCenter
		opt.ColorScale.ScaleWithColor(col)
		text.Draw(screen, cart.Name, g.face, opt)
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
