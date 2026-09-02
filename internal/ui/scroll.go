package ui

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

const (
	scrollEase      = 0.22 // per-tick catch-up fraction while easing toward target
	scrollFriction  = 0.92 // per-tick velocity decay while coasting on momentum
	scrollVelEps    = 0.003
	scrollSettleEps = 0.001
)

// updateScroll drives the mouse-wheel/drag input and the pos/target/vel
// animation state that both drawCarasel and drawList render from. Discrete
// navigation (keys, pad, typeahead) only ever touches target; this is what
// turns those steps — and drag gestures — into motion.
func (g *Game) updateScroll() {
	n := len(g.allCarts)
	if n == 0 {
		return
	}

	w, h := ebiten.WindowSize()
	pitch := g.itemPitch(w, h)

	g.updatePointer(pitch)
	if !g.dragging {
		g.updateWheel()
	}

	if g.dragging {
		return
	}

	if math.Abs(g.vel) > scrollVelEps {
		g.pos += g.vel
		g.vel *= scrollFriction
		if math.Abs(g.vel) <= scrollVelEps {
			g.vel = 0
			g.target = math.Round(g.pos)
		}
		return
	}

	g.pos += (g.target - g.pos) * scrollEase
	if math.Abs(g.target-g.pos) < scrollSettleEps {
		g.pos = g.target
	}
}

// itemPitch is the on-screen pixel spacing between adjacent carts in the
// current mode, i.e. how far a drag has to travel to move one item.
func (g *Game) itemPitch(w, h int) float64 {
	if g.mode == modeList {
		return listRowHeight()
	}
	_, gap := caraselMetrics(w, h)
	return gap
}

func (g *Game) updateWheel() {
	_, yoff := ebiten.Wheel()
	if yoff == 0 {
		return
	}
	if yoff > 0 {
		g.target--
	} else {
		g.target++
	}
}

// updatePointer drags pos directly under the mouse or a single touch,
// tracking per-tick velocity so a fling keeps coasting after release.
func (g *Game) updatePointer(pitch float64) {
	if g.dragging {
		if g.dragIsTouch {
			if !touchStillActive(g.dragTouchID) {
				g.endDrag()
				return
			}
			x, y := ebiten.TouchPosition(g.dragTouchID)
			g.dragTo(pointerCoord(g.mode, x, y), pitch)
			return
		}
		if inpututil.IsMouseButtonJustReleased(ebiten.MouseButtonLeft) {
			g.endDrag()
			return
		}
		mx, my := ebiten.CursorPosition()
		g.dragTo(pointerCoord(g.mode, mx, my), pitch)
		return
	}

	if ids := ebiten.AppendTouchIDs(nil); len(ids) > 0 {
		x, y := ebiten.TouchPosition(ids[0])
		g.startDrag(pointerCoord(g.mode, x, y), true, ids[0])
		return
	}
	if inpututil.IsMouseButtonJustPressed(ebiten.MouseButtonLeft) {
		mx, my := ebiten.CursorPosition()
		g.startDrag(pointerCoord(g.mode, mx, my), false, 0)
	}
}

func pointerCoord(m viewMode, x, y int) float64 {
	if m == modeList {
		return float64(y)
	}
	return float64(x)
}

func touchStillActive(id ebiten.TouchID) bool {
	for _, active := range ebiten.AppendTouchIDs(nil) {
		if active == id {
			return true
		}
	}
	return false
}

func (g *Game) startDrag(coord float64, isTouch bool, id ebiten.TouchID) {
	g.dragging = true
	g.dragIsTouch = isTouch
	g.dragTouchID = id
	g.dragAnchor = coord
	g.dragAnchorPos = g.pos
	g.vel = 0
}

// dragTo moves pos to follow the pointer 1:1 (dragging the content, so
// moving the pointer forward reveals earlier items — the opposite sign of
// a forward navigation step) and keeps target glued to it so a launch
// mid-drag acts on whatever's nearest-center right now.
func (g *Game) dragTo(coord, pitch float64) {
	prev := g.pos
	g.pos = g.dragAnchorPos - (coord-g.dragAnchor)/pitch
	g.vel = g.pos - prev
	g.target = g.pos
}

func (g *Game) endDrag() {
	g.dragging = false
	if math.Abs(g.vel) < scrollVelEps {
		g.vel = 0
		g.target = math.Round(g.pos)
	}
}

// shortestDelta is the shortest signed step count from `from` to `to`,
// wrapping either direction around a cycle of n items — used so a
// type-ahead jump animates the short way around instead of spinning past
// everything in between.
func shortestDelta(from, to, n int) float64 {
	if n == 0 {
		return 0
	}
	d := wrap(to-from, n)
	if d > n/2 {
		d -= n
	}
	return float64(d)
}
