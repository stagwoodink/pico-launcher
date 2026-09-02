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

	"github.com/stagwoodink/pico-launcher/internal/bbsindex"
	"github.com/stagwoodink/pico-launcher/internal/bbsmatch"
	"github.com/stagwoodink/pico-launcher/internal/bbsreplace"
	"github.com/stagwoodink/pico-launcher/internal/carts"
	"github.com/stagwoodink/pico-launcher/internal/config"
	"github.com/stagwoodink/pico-launcher/internal/launcher"
	"github.com/stagwoodink/pico-launcher/internal/picker"
	"github.com/stagwoodink/pico-launcher/internal/pico8"
)

// bbsCandidateCount is how many options the [Tab] resolution picker offers.
const bbsCandidateCount = 5

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
	stateResolvingBBS // "[Tab]" candidate picker for a badged cart
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

	// bbsIndex is the scraped BBS cart index, loaded once in the background
	// after carts load; bbsIndexSorted is the same data sorted by title, for
	// the "!" full-list browse. bbsBadge marks a cart with candidates worth
	// narrowing from ("?"); bbsUnfound marks one with no parseable local
	// title at all, so there's nothing to fuzzy-match against ("!") — both
	// are resolved via [Tab] on that cart, just from a different source list.
	bbsIndex       []bbsindex.BBSCart
	bbsIndexSorted []bbsindex.BBSCart
	bbsBadge       map[string]bool
	bbsUnfound     map[string]bool
	bbsResultCh    chan bbsEnrichResult

	// Set while stateResolvingBBS is active.
	resolveCart        string
	resolveOptions     []bbsindex.BBSCart
	resolveSel         int
	resolveTypeahead   string
	resolveTypeaheadAt time.Time
	resolveCh          chan bbsResolveResult
}

// bbsEnrichResult is what the background enrichment pass hands back to the
// main (Ebiten) goroutine to apply — the scan/replace work itself happens
// off that goroutine, but every Game field mutation happens on it.
type bbsEnrichResult struct {
	index    []bbsindex.BBSCart
	replaced map[string]carts.Cart
	badges   map[string]bool // has candidates ("?")
	unfound  map[string]bool // no parseable title ("!")
}

type bbsResolveResult struct {
	name string
	cart carts.Cart
	ok   bool
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
	g.startBBSEnrichment()
}

// startBBSEnrichment fetches the BBS index and, for every local .p8-only
// cart (no cover art of its own), matches it and either replaces it
// silently (confident match) or flags it with a "?" badge (no dialog —
// this app never surfaces error/prompt UI, see handoff). All of it runs off
// the Ebiten goroutine; only the finished result is applied to Game state,
// via bbsResultCh polled from updateBrowsing.
func (g *Game) startBBSEnrichment() {
	snapshot := append([]carts.Cart(nil), g.baseCarts...)
	ch := make(chan bbsEnrichResult, 1)
	g.bbsResultCh = ch
	go func() {
		index, err := bbsindex.Fetch()
		if err != nil || len(index) == 0 {
			ch <- bbsEnrichResult{}
			return
		}
		replaced := map[string]carts.Cart{}
		badges := map[string]bool{}
		unfound := map[string]bool{}
		for _, c := range snapshot {
			if c.Image != "" || !strings.HasSuffix(strings.ToLower(c.Path), ".p8") {
				continue
			}
			title, author := bbsmatch.ParseP8Meta(c.Path)
			best, _, ok := bbsmatch.Match(title, author, index)
			if !ok {
				if title == "" {
					unfound[c.Name] = true
				} else {
					badges[c.Name] = true
				}
				continue
			}
			newPath, err := bbsreplace.Replace(c.Path, best.PNGURL)
			if err != nil {
				badges[c.Name] = true // a candidate exists, only the download failed
				continue
			}
			replaced[c.Name] = carts.Cart{Name: c.Name, Path: newPath, Image: newPath}
		}
		ch <- bbsEnrichResult{index: index, replaced: replaced, badges: badges, unfound: unfound}
	}()
}

// pollBBSEnrichment applies a finished background enrichment pass, if one
// has landed: swaps in any auto-replaced carts and records "?"/"!" badges.
func (g *Game) pollBBSEnrichment() {
	if g.bbsResultCh == nil {
		return
	}
	select {
	case res := <-g.bbsResultCh:
		g.bbsResultCh = nil
		g.bbsIndex = res.index
		g.bbsBadge = res.badges
		g.bbsUnfound = res.unfound
		if len(res.index) > 0 {
			sorted := append([]bbsindex.BBSCart(nil), res.index...)
			sort.Slice(sorted, func(i, j int) bool {
				return strings.ToLower(sorted[i].Title) < strings.ToLower(sorted[j].Title)
			})
			g.bbsIndexSorted = sorted
		}
		if len(res.replaced) > 0 {
			for i, c := range g.baseCarts {
				if nc, ok := res.replaced[c.Name]; ok {
					g.baseCarts[i] = nc
				}
			}
			g.rebuildOrder()
		}
	default:
	}
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
	case stateResolvingBBS:
		g.updateResolvingBBS()
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
	g.pollBBSEnrichment()

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
		if !shift {
			if cart, ok := g.selectedCart(); ok && (g.bbsBadge[cart.Name] || g.bbsUnfound[cart.Name]) {
				g.openBBSResolve(cart)
				return
			}
		}
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

// openBBSResolve enters the on-demand picker for a badged cart: a narrowed
// candidate list for "?" (has plausible matches), or the entire sorted BBS
// index for "!" (no local title to narrow from at all) — type-ahead is how
// you jump through that full list to find the right one by hand.
func (g *Game) openBBSResolve(cart carts.Cart) {
	var opts []bbsindex.BBSCart
	if g.bbsBadge[cart.Name] {
		title, author := bbsmatch.ParseP8Meta(cart.Path)
		opts = bbsmatch.Candidates(title, author, g.bbsIndex, bbsCandidateCount)
	} else if g.bbsUnfound[cart.Name] {
		opts = g.bbsIndexSorted
	}
	if len(opts) == 0 {
		return
	}
	g.resolveCart = cart.Name
	g.resolveOptions = opts
	g.resolveSel = 0
	g.resolveTypeahead = ""
	g.returnState = stateBrowsing
	g.state = stateResolvingBBS
}

func (g *Game) updateResolvingBBS() {
	if g.resolveCh != nil {
		select {
		case res := <-g.resolveCh:
			g.resolveCh = nil
			if res.ok {
				delete(g.bbsBadge, res.name)
				delete(g.bbsUnfound, res.name)
				for i, c := range g.baseCarts {
					if c.Name == res.name {
						g.baseCarts[i] = res.cart
					}
				}
				g.rebuildOrder()
			}
			g.state = g.returnState
		default:
		}
		return
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyUp) {
		g.resolveSel = wrap(g.resolveSel-1, len(g.resolveOptions))
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyDown) {
		g.resolveSel = wrap(g.resolveSel+1, len(g.resolveOptions))
	}
	g.updateResolveTypeahead()
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) || inpututil.IsKeyJustPressed(ebiten.KeyBackspace) {
		// bail out: no selection was confirmed, so the badge stays exactly
		// as it was — [Tab] can reopen this picker again later.
		g.state = g.returnState
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.confirmBBSResolve()
	}
}

// updateResolveTypeahead jumps resolveSel to the first option whose title
// starts with what's been typed — same prefix-search shape as the main
// cart browser's updateTypeahead, just over resolveOptions instead of
// allCarts, since the full "!" list is otherwise too long to page through.
func (g *Game) updateResolveTypeahead() {
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
	if now.Sub(g.resolveTypeaheadAt) > typeaheadTimeout {
		g.resolveTypeahead = ""
	}
	g.resolveTypeahead += string(typed)
	g.resolveTypeaheadAt = now

	needle := strings.ToUpper(g.resolveTypeahead)
	for i, c := range g.resolveOptions {
		if strings.HasPrefix(strings.ToUpper(c.Title), needle) {
			g.resolveSel = i
			return
		}
	}
}

// confirmBBSResolve downloads and swaps in the picked candidate off the
// Ebiten goroutine, same pattern as the file pickers (pollPicker).
func (g *Game) confirmBBSResolve() {
	name := g.resolveCart
	picked := g.resolveOptions[g.resolveSel]

	var cartPath string
	for _, c := range g.baseCarts {
		if c.Name == name {
			cartPath = c.Path
		}
	}
	if cartPath == "" {
		g.state = g.returnState
		return
	}

	ch := make(chan bbsResolveResult, 1)
	g.resolveCh = ch
	go func() {
		newPath, err := bbsreplace.Replace(cartPath, picked.PNGURL)
		if err != nil {
			ch <- bbsResolveResult{name: name, ok: false}
			return
		}
		ch <- bbsResolveResult{name: name, ok: true, cart: carts.Cart{Name: name, Path: newPath, Image: newPath}}
	}()
}

func wrap(i, n int) int {
	if n == 0 {
		return 0
	}
	return ((i % n) + n) % n
}
