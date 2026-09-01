// Package ui implements the launcher's visual, input-driven interface.
package ui

import (
	"os"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/stagwoodink/pico-launcher/internal/carts"
	"github.com/stagwoodink/pico-launcher/internal/config"
	"github.com/stagwoodink/pico-launcher/internal/launcher"
	"github.com/stagwoodink/pico-launcher/internal/picker"
	"github.com/stagwoodink/pico-launcher/internal/pico8"
)

const (
	ScreenW = 900
	ScreenH = 540
)

type state int

const (
	stateAwaitPico8Tab state = iota
	statePickingPico8
	stateAwaitCartsTab
	statePickingCarts
	stateNoCarts
	stateBrowsing
)

type panel int

const (
	panelCarasel panel = iota
	panelList
)

type Game struct {
	cfg config.Config

	state       state
	returnState state       // state to restore to if the user cancels a picker
	pickerErr   chan string // resolved path or "" on cancel/failure

	allCarts   []carts.Cart
	listOnly   bool // debug: force every cart into the list panel
	carasel    []carts.Cart
	list       []carts.Cart
	caraselIdx int
	listIdx    int
	lastPanel  panel

	images map[string]*ebiten.Image

	font *bitmapFont
}

func New(cfg config.Config) *Game {
	g := &Game{cfg: cfg, images: map[string]*ebiten.Image{}}
	if f, err := loadBitmapFont(); err == nil {
		g.font = f
	}
	g.bootstrap()
	return g
}

// bootstrap tries to get straight to the browsing state with zero prompts,
// only falling back to a picker when auto-detection genuinely can't find
// what it needs.
func (g *Game) bootstrap() {
	if pico8.Resolve(g.cfg.Pico8Path) == "" {
		if found := pico8.Find(); found != "" {
			g.cfg.Pico8Path = found
			g.cfg.Save()
		}
	}
	if g.cfg.Pico8Path == "" {
		g.state = stateAwaitPico8Tab
		return
	}
	g.afterPico8Ready()
}

func (g *Game) afterPico8Ready() {
	if g.cfg.CartsDir == "" || carts.Scan(g.cfg.CartsDir) == nil {
		if found := carts.FindDir(); found != "" {
			g.cfg.CartsDir = found
			g.cfg.Save()
		}
	}
	if g.cfg.CartsDir == "" {
		g.state = stateAwaitCartsTab
		return
	}
	g.loadCarts()
}

func (g *Game) loadCarts() {
	all := carts.Scan(g.cfg.CartsDir)
	if len(all) == 0 {
		g.state = stateNoCarts
		return
	}
	g.allCarts = all
	g.rebuildPanels()
	g.state = stateBrowsing
}

// rebuildPanels splits g.allCarts into the carasel/list panels (or, in
// debug list-only mode, puts everything in the list) and resets selection.
func (g *Game) rebuildPanels() {
	if g.listOnly {
		g.carasel = nil
		g.list = append([]carts.Cart(nil), g.allCarts...)
	} else {
		g.carasel, g.list = carts.Split(g.allCarts)
	}
	g.caraselIdx, g.listIdx = 0, 0
	g.lastPanel = panelCarasel
	if len(g.carasel) == 0 {
		g.lastPanel = panelList
	}
}

// --- ebiten.Game ---

// Layout returns the window size unchanged so the UI always fills whatever
// space the user resizes the window to, with no fixed width/height cap.
func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return outsideWidth, outsideHeight
}

func (g *Game) Update() error {
	switch g.state {
	case stateAwaitPico8Tab:
		if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
			g.pickPico8()
		}
	case statePickingPico8:
		g.pollPicker(func(p string) {
			if p == "" {
				// cancelled: leave the existing config untouched.
				g.state = g.returnState
				return
			}
			resolved := pico8.Resolve(p)
			if resolved == "" {
				// couldn't find pico8 in what they picked; ask for the
				// executable directly instead.
				g.pickPico8File()
				return
			}
			g.cfg.Pico8Path = resolved
			g.cfg.Save()
			g.afterPico8Ready()
		})
	case stateAwaitCartsTab:
		if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
			g.pickCarts()
		}
	case statePickingCarts:
		g.pollPicker(func(p string) {
			if p == "" || p == g.cfg.CartsDir {
				// cancelled, or re-confirmed the same folder: don't touch
				// the carasel/list or reset the current selection.
				g.state = g.returnState
				return
			}
			g.cfg.CartsDir = p
			g.cfg.Save()
			g.loadCarts()
		})
	case stateNoCarts:
		if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
			g.pickCarts()
		}
	case stateBrowsing:
		g.updateBrowsing()
	}
	return nil
}

func (g *Game) pickPico8() {
	g.returnState = g.state
	g.state = statePickingPico8
	g.pickerErr = make(chan string, 1)
	go func() {
		p, err := picker.Directory("Select your PICO-8 install folder")
		if err != nil {
			p = ""
		}
		g.pickerErr <- p
	}()
}

func (g *Game) pickPico8File() {
	g.state = statePickingPico8
	g.pickerErr = make(chan string, 1)
	go func() {
		p, err := picker.File("Select the PICO-8 executable")
		if err != nil {
			p = ""
		}
		g.pickerErr <- p
	}()
}

func (g *Game) pickCarts() {
	g.returnState = g.state
	g.state = statePickingCarts
	g.pickerErr = make(chan string, 1)
	go func() {
		p, err := picker.Directory("Select your carts folder")
		if err != nil {
			p = ""
		}
		g.pickerErr <- p
	}()
}

func (g *Game) pollPicker(onResult func(string)) {
	select {
	case p := <-g.pickerErr:
		onResult(p)
	default:
	}
}

func (g *Game) updateBrowsing() {
	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		alt := ebiten.IsKeyPressed(ebiten.KeyAltLeft) || ebiten.IsKeyPressed(ebiten.KeyAltRight)
		shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
		switch {
		case alt:
			g.listOnly = !g.listOnly
			g.rebuildPanels()
		case shift:
			g.pickPico8()
		default:
			g.pickCarts()
		}
		return
	}

	if len(g.carasel) > 0 {
		if inpututil.IsKeyJustPressed(ebiten.KeyLeft) || padJustPressed(dpadLeft) {
			g.caraselIdx = wrap(g.caraselIdx-1, len(g.carasel))
			g.lastPanel = panelCarasel
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyRight) || padJustPressed(dpadRight) {
			g.caraselIdx = wrap(g.caraselIdx+1, len(g.carasel))
			g.lastPanel = panelCarasel
		}
	}
	if len(g.list) > 0 {
		if inpututil.IsKeyJustPressed(ebiten.KeyUp) || padJustPressed(dpadUp) {
			g.listIdx = wrap(g.listIdx-1, len(g.list))
			g.lastPanel = panelList
		}
		if inpututil.IsKeyJustPressed(ebiten.KeyDown) || padJustPressed(dpadDown) {
			g.listIdx = wrap(g.listIdx+1, len(g.list))
			g.lastPanel = panelList
		}
	}

	keepOpen := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight) || padHeld(selectButton)
	trigger := inpututil.IsKeyJustPressed(ebiten.KeySpace) ||
		inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
		padJustPressed(aButton) || padJustPressed(startButton) || padJustPressed(selectButton)

	if trigger {
		g.launchSelected(keepOpen)
	}
}

func (g *Game) selectedCart() (carts.Cart, bool) {
	if g.lastPanel == panelCarasel && len(g.carasel) > 0 {
		return g.carasel[g.caraselIdx], true
	}
	if g.lastPanel == panelList && len(g.list) > 0 {
		return g.list[g.listIdx], true
	}
	return carts.Cart{}, false
}

// launchSelected launches with silent self-healing: if it fails, it
// re-detects the PICO-8 install and re-checks the cart path before trying
// once more, without ever surfacing an error to the user.
func (g *Game) launchSelected(keepOpen bool) {
	cart, ok := g.selectedCart()
	if !ok {
		return
	}

	ok, cmd := launcher.Launch(g.cfg.Pico8Path, cart.Path)
	if !ok {
		if healed := g.selfHeal(cart); !healed {
			return
		}
		ok, cmd = launcher.Launch(g.cfg.Pico8Path, cart.Path)
		if !ok {
			// still broken after healing: only now ask the user directly.
			g.state = stateAwaitPico8Tab
			g.cfg.Pico8Path = ""
			return
		}
	}
	_ = cmd

	if !keepOpen {
		os.Exit(0)
	}
}

// selfHeal re-runs auto-detection for both the PICO-8 install and the carts
// dir, refreshing config in place. Returns false only if detection itself
// found nothing to try.
func (g *Game) selfHeal(cart carts.Cart) bool {
	healed := false
	if pico8.Resolve(g.cfg.Pico8Path) == "" {
		if found := pico8.Find(); found != "" {
			g.cfg.Pico8Path = found
			healed = true
		}
	}
	if _, err := os.Stat(cart.Path); err != nil {
		if all := carts.Scan(g.cfg.CartsDir); all != nil {
			g.allCarts = all
			g.rebuildPanels()
			healed = true
		}
	}
	if healed {
		g.cfg.Save()
	}
	return healed
}

func wrap(i, n int) int {
	if n == 0 {
		return 0
	}
	return ((i % n) + n) % n
}
