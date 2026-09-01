package ui

import (
	"bytes"
	_ "embed"
	"image"
	"image/color"
	_ "image/png"

	"github.com/hajimehoshi/ebiten/v2"
)

// pico8_font.png is Lexaloffle's official PICO-8 font glyph sheet
// (lexaloffle.com/gfx/pico8_font.png), released under CC-0 — using it here
// keeps the launcher's text visually consistent with PICO-8 itself.
//
//go:embed assets/pico8_font.png
var fontSheetPNG []byte

const (
	glyphCellW  = 8 // column pitch in the source sheet
	glyphCellH  = 8
	glyphCols   = 16
	glyphStartY = 16 // ASCII space (32) starts here; rows above are PICO-8's non-ASCII icon glyphs
	firstRune   = ' '
	lastRune    = '~'

	// Every glyph's ink only fills the first few pixels of its 8px cell
	// (verified across the whole printable range); cropping to that and
	// advancing by a tighter step keeps text readable without the sheet's
	// baked-in padding ballooning letter spacing.
	glyphInkW    = 4
	glyphAdvance = 5
)

type align int

const (
	alignStart align = iota
	alignCenter
)

// bitmapFont draws text using PICO-8's own monospace glyph sheet, with
// black background pixels treated as transparent so glyphs can be tinted.
type bitmapFont struct {
	sheet *ebiten.Image
}

func loadBitmapFont() (*bitmapFont, error) {
	src, _, err := image.Decode(bytes.NewReader(fontSheetPNG))
	if err != nil {
		return nil, err
	}
	b := src.Bounds()
	keyed := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bch, _ := src.At(x, y).RGBA()
			if r == 0 && g == 0 && bch == 0 {
				continue // leave transparent
			}
			keyed.Set(x, y, color.White)
		}
	}
	return &bitmapFont{sheet: ebiten.NewImageFromImage(keyed)}, nil
}

func glyphRect(r rune) (image.Rectangle, bool) {
	if r < firstRune || r > lastRune {
		return image.Rectangle{}, false
	}
	idx := int(r - firstRune)
	col := idx % glyphCols
	row := idx / glyphCols
	x := col * glyphCellW
	y := glyphStartY + row*glyphCellH
	return image.Rect(x, y, x+glyphInkW, y+glyphCellH), true
}

// width returns the on-screen pixel width of s at the given integer scale
// (the font is monospace, so this is exact and cheap).
func (f *bitmapFont) width(s string, scale int) float64 {
	return float64(len([]rune(s)) * glyphAdvance * scale)
}

// truncate shortens s with a trailing "..." if it's wider than maxWidth at
// the given scale, so list rows never overflow their column.
func (f *bitmapFont) truncate(s string, maxWidth float64, scale int) string {
	if f.width(s, scale) <= maxWidth {
		return s
	}
	maxChars := int(maxWidth) / (glyphAdvance * scale)
	if maxChars <= 3 {
		return "..."[:max(0, maxChars)]
	}
	runes := []rune(s)
	if maxChars > len(runes) {
		maxChars = len(runes)
	}
	return string(runes[:maxChars-3]) + "..."
}

// draw renders s with its vertical center at y. x is the left edge for
// alignStart, or the horizontal center for alignCenter.
func (f *bitmapFont) draw(dst *ebiten.Image, s string, x, y float64, scale int, col color.Color, a align) {
	w := f.width(s, scale)
	left := x
	if a == alignCenter {
		left = x - w/2
	}
	top := y - float64(glyphCellH*scale)/2

	for i, r := range []rune(s) {
		rect, ok := glyphRect(r)
		if !ok {
			continue
		}
		sub := f.sheet.SubImage(rect).(*ebiten.Image)
		opt := &ebiten.DrawImageOptions{}
		opt.GeoM.Scale(float64(scale), float64(scale))
		opt.GeoM.Translate(left+float64(i*glyphAdvance*scale), top)
		opt.ColorScale.ScaleWithColor(col)
		dst.DrawImage(sub, opt)
	}
}
