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
	bg           = color.RGBA{16, 16, 20, 255}
	placeholder  = color.RGBA{40, 40, 48, 255}
	highlight    = color.RGBA{255, 204, 0, 255}
	dim          = color.RGBA{90, 90, 100, 255}
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
	opt := &text.DrawOptions{}
	opt.GeoM.Translate(ScreenW/2, ScreenH/2)
	opt.PrimaryAlign = text.AlignCenter
	opt.SecondaryAlign = text.AlignCenter
	opt.ColorScale.ScaleWithColor(color.White)
	text.Draw(screen, msg, g.face, opt)
}

const (
	tileSize   = 200
	listTileSz = 64
	listGap    = 12
)

func (g *Game) drawBrowsing(screen *ebiten.Image) {
	if len(g.carasel) > 0 {
		g.drawCarasel(screen)
	}
	if len(g.list) > 0 {
		g.drawList(screen)
	}
}

func (g *Game) drawCarasel(screen *ebiten.Image) {
	cx, cy := ScreenW*0.38, ScreenH*0.5
	for off := -1; off <= 1; off++ {
		idx := wrap(g.caraselIdx+off, len(g.carasel))
		cart := g.carasel[idx]
		img := g.thumbnail(cart.Image)
		if img == nil {
			continue
		}
		scale := 1.0
		alpha := 1.0
		if off != 0 {
			scale = 0.7
			alpha = 0.35
		}
		w, h := img.Bounds().Dx(), img.Bounds().Dy()
		s := float64(tileSize) * scale / float64(h)
		opt := &ebiten.DrawImageOptions{}
		opt.GeoM.Scale(s, s)
		opt.GeoM.Translate(cx+float64(off)*float64(tileSize)*0.9-float64(w)*s/2, cy-float64(h)*s/2)
		opt.ColorScale.ScaleAlpha(float32(alpha))
		screen.DrawImage(img, opt)
	}
	if g.lastPanel == panelCarasel {
		drawBorder(screen, cx-tileSize/2-6, cy-tileSize/2-6, tileSize+12, tileSize+12, highlight)
	}
}

func (g *Game) drawList(screen *ebiten.Image) {
	x := ScreenW*0.8 - listTileSz/2
	total := len(g.list)
	top := ScreenH/2 - (total*(listTileSz+listGap))/2
	for i, _ := range g.list {
		y := top + i*(listTileSz+listGap)
		col := placeholder
		selected := i == g.listIdx
		if selected {
			col = dim
		}
		fillRect(screen, float64(x), float64(y), listTileSz, listTileSz, col)
		if selected && g.lastPanel == panelList {
			drawBorder(screen, float64(x)-4, float64(y)-4, listTileSz+8, listTileSz+8, highlight)
		}
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

func drawBorder(screen *ebiten.Image, x, y, w, h float64, c color.Color) {
	const t = 3.0
	fillRect(screen, x, y, w, t, c)
	fillRect(screen, x, y+h-t, w, t, c)
	fillRect(screen, x, y, t, h, c)
	fillRect(screen, x+w-t, y, t, h, c)
}
