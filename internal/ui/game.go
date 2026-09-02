// Package ui implements the launcher's visual, input-driven interface.
package ui

import (
	"math"
	"os"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/stagwoodink/pico-launcher/internal/carts"
	"github.com/stagwoodink/pico-launcher/internal/config"
	"github.com/stagwoodink/pico-launcher/internal/launcher"
	"github.com/stagwoodink/pico-launcher/internal/picker"
	"github.com/stagwoodink/pico-launcher/internal/pico8"
)

// typeaheadTimeout is how long to wait after the last keystroke before a
// new letter starts a fresh search instead of extending the current one.
const typeaheadTimeout = 700 * time.Millisecond

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

// viewMode is the whole-window display: every cart is always in play, only
// how it's presented changes.
type viewMode int

const (
	modeCarasel viewMode = iota // default: cover art, .p8-only carts get a placeholder tile
	modeList
)

type Game struct {
	cfg config.Config

	state       state
	returnState state       // state to restore to if the user cancels a picker
	pickerErr   chan string // resolved path or "" on cancel/failure

	// baseCarts is the canonical, deduplicated, alphabetical scan result.
	// allCarts is what's actually shown: baseCarts with the recent and
	// favorite carts additionally duplicated up front — see rebuildOrder.
	baseCarts   []carts.Cart
	allCarts    []carts.Cart
	recentSet   map[string]bool // top-maxRecents cart names, for the ~ mark
	favoriteSet map[string]bool // all favorited cart names, for the * mark
	mode        viewMode

	// pos is the animated (unwrapped) scroll position rendering follows;
	// target is where it's easing/coasting toward. Both stay unwrapped so
	// wrap-around and shortest-path jumps are plain float math — see
	// scroll.go. The logical selection is always wrap(round(target), n).
	pos, target, vel float64
	dragging         bool
	dragIsTouch      bool
	dragTouchID      ebiten.TouchID
	dragAnchor       float64 // pointer coord (px) when the drag started
	dragAnchorPos    float64 // pos value when the drag started

	typeahead   string
	typeaheadAt time.Time

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
	g.baseCarts = all
	g.rebuildOrder()
	g.pos, g.target, g.vel = 0, 0, 0
	g.dragging = false
	g.state = stateBrowsing
}

// rebuildOrder regenerates allCarts from baseCarts plus the configured
// recents/favorites: up to maxRecents recently-launched carts duplicated
// at the very front (newest first), then favorited carts duplicated next
// (alphabetical) — except any favorite currently in that recents block,
// which is left for the recents duplicate to represent (it still gets the
// favorite mark, it just isn't duplicated a second time). Both sets are
// also recorded for the ~/* marks, which apply to every occurrence of a
// cart, duplicate or original. Selection follows the same cart through
// the reorder when possible.
func (g *Game) rebuildOrder() {
	selected, hadSelection := g.selectedCart()

	byName := make(map[string]carts.Cart, len(g.baseCarts))
	for _, c := range g.baseCarts {
		byName[c.Name] = c
	}

	recentSet := make(map[string]bool, len(g.cfg.RecentNames))
	var recentBlock []carts.Cart
	for _, name := range g.cfg.RecentNames {
		if c, ok := byName[name]; ok {
			recentBlock = append(recentBlock, c)
			recentSet[name] = true
		}
	}

	favoriteSet := make(map[string]bool, len(g.cfg.FavoriteNames))
	var favNames []string
	for _, name := range g.cfg.FavoriteNames {
		favoriteSet[name] = true
		if !recentSet[name] {
			if _, ok := byName[name]; ok {
				favNames = append(favNames, name)
			}
		}
	}
	sort.Strings(favNames)
	favBlock := make([]carts.Cart, len(favNames))
	for i, name := range favNames {
		favBlock[i] = byName[name]
	}

	all := make([]carts.Cart, 0, len(recentBlock)+len(favBlock)+len(g.baseCarts))
	all = append(all, recentBlock...)
	all = append(all, favBlock...)
	all = append(all, g.baseCarts...)

	g.allCarts = all
	g.recentSet = recentSet
	g.favoriteSet = favoriteSet

	if hadSelection {
		g.snapToCart(selected.Name)
	}
}

// snapToCart moves the selection to name's first occurrence in allCarts,
// with no animation — used after a reorder, where the list itself just
// changed shape rather than the user navigating through it.
func (g *Game) snapToCart(name string) {
	for i, c := range g.allCarts {
		if c.Name == name {
			g.pos, g.target, g.vel = float64(i), float64(i), 0
			return
		}
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
				// the current list or reset the current selection.
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
	if inpututil.IsKeyJustPressed(ebiten.KeyBackquote) {
		if g.mode == modeCarasel {
			g.mode = modeList
		} else {
			g.mode = modeCarasel
		}
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
		if shift {
			g.pickPico8()
		} else {
			g.pickCarts()
		}
		return
	}

	if len(g.allCarts) > 0 && !g.dragging {
		prevDuration := max(
			keyDuration(ebiten.KeyLeft), keyDuration(ebiten.KeyUp),
			padDuration(dpadLeft), padDuration(dpadUp),
		)
		nextDuration := max(
			keyDuration(ebiten.KeyRight), keyDuration(ebiten.KeyDown),
			padDuration(dpadRight), padDuration(dpadDown),
		)
		if repeatFire(prevDuration) {
			g.target--
		}
		if repeatFire(nextDuration) {
			g.target++
		}
	}

	g.updateTypeahead()
	g.updateScroll()

	if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		g.toggleFavorite()
	}

	trigger := inpututil.IsKeyJustPressed(ebiten.KeyEnter) ||
		padJustPressed(aButton) || padJustPressed(startButton) || padJustPressed(selectButton)

	if trigger {
		g.launchSelected()
	}
}

// toggleFavorite stars/unstars the current selection and re-derives the
// display order, since favorite status affects it.
func (g *Game) toggleFavorite() {
	cart, ok := g.selectedCart()
	if !ok {
		return
	}
	g.cfg.ToggleFavorite(cart.Name)
	g.cfg.Save()
	g.rebuildOrder()
}

// updateTypeahead composes typed letters/digits into a search string (a
// pause longer than typeaheadTimeout starts a fresh one) and jumps the
// selection to the first cart whose title starts with it.
func (g *Game) updateTypeahead() {
	var typed []rune
	for _, r := range ebiten.AppendInputChars(nil) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			typed = append(typed, r)
		}
	}
	if len(typed) == 0 {
		return
	}

	now := time.Now()
	if now.Sub(g.typeaheadAt) > typeaheadTimeout {
		g.typeahead = ""
	}
	g.typeahead += string(typed)
	g.typeaheadAt = now

	needle := strings.ToUpper(g.typeahead)
	for i, c := range g.allCarts {
		if strings.HasPrefix(strings.ToUpper(c.Name), needle) {
			g.target += shortestDelta(g.currentIndex(), i, len(g.allCarts))
			return
		}
	}
}

// currentIndex is the logical selection: wherever target is heading, not
// necessarily where the animation has visually settled yet.
func (g *Game) currentIndex() int {
	if len(g.allCarts) == 0 {
		return 0
	}
	return wrap(int(math.Round(g.target)), len(g.allCarts))
}

func (g *Game) selectedCart() (carts.Cart, bool) {
	if len(g.allCarts) == 0 {
		return carts.Cart{}, false
	}
	return g.allCarts[g.currentIndex()], true
}

// launchSelected launches with silent self-healing: if it fails, it
// re-detects the PICO-8 install and re-checks the cart path before trying
// once more, without ever surfacing an error to the user. The launcher
// itself always stays open.
func (g *Game) launchSelected() {
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

	g.cfg.TouchRecent(cart.Name)
	g.cfg.Save()
	g.rebuildOrder()
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
			g.baseCarts = all
			g.rebuildOrder()
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
