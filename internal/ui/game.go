// Package ui implements the launcher's visual, input-driven interface.
package ui

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
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

// bbsSuggestionCount is how many live-search suggestions the [Tab]
// resolution picker re-ranks and shows as the query is edited.
const bbsSuggestionCount = 30

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

	// lastCartsScanAt paces startCartsRescan — see cartsRescanInterval.
	// cartsScanCh is non-nil while a scan is in flight.
	lastCartsScanAt time.Time
	cartsScanCh     chan cartsScanResult

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

	// Set while stateResolvingBBS is active: a live search box (resolveQuery,
	// editable) with suggestions re-ranked on every edit against whichever
	// pool this picker is searching (resolveSearchIndex/-Sorted — the whole
	// BBS index when replacing a cart's art, or that minus already-owned
	// titles when adding a new one, see resolveAdding), and resolveSel
	// scrollable through them with the same held-key acceleration as the
	// main cart browser.
	resolveCart         string // "" when resolveAdding — there's no cart being replaced
	resolveAdding       bool
	resolveQuery        string
	resolveAuthorHint   string
	resolveSearchIndex  []bbsindex.BBSCart
	resolveSearchSorted []bbsindex.BBSCart
	resolveSuggestions  []bbsindex.BBSCart
	resolveSel          int
	resolveCh           chan bbsResolveResult

	// bbsLastUndo is the single most recent manual resolve action (replace
	// or add), reversible with Ctrl+Z. Only one level deep — good enough
	// for "I picked the wrong one," not a full history.
	bbsLastUndo *bbsUndo

	// Cart deletion: holding [Backspace]/[Delete]/[-] on the selected cart
	// fills it up (deleteProgress 0..1), arms once full, and a second press
	// deletes (moves to backup) — any other key cancels instead.
	deleteState    deleteState
	deleteCartName string
	deleteProgress float64
}

type deleteState int

const (
	deleteStateNone deleteState = iota
	deleteStateFilling
	deleteStateArmed
)

// bbsUndoKind distinguishes what Ctrl+Z needs to reverse.
type bbsUndoKind int

const (
	undoKindReplace bbsUndoKind = iota
	undoKindAdd
)

type bbsUndo struct {
	kind         bbsUndoKind
	cartName     string
	originalPath string // undoKindReplace only
	newPath      string
	wasBadge     bool // undoKindReplace only
	wasUnfound   bool // undoKindReplace only
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
	name         string
	cart         carts.Cart
	ok           bool
	adding       bool
	originalPath string // undoKindReplace only
	wasBadge     bool   // undoKindReplace only
	wasUnfound   bool   // undoKindReplace only
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
	g.lastCartsScanAt = time.Now()
	g.startBBSEnrichment()
}

// cartsRescanInterval is how often the carts dir gets re-scanned for
// changes made outside the app (added, deleted, renamed) while browsing.
const cartsRescanInterval = 2 * time.Second

// cartsScanResult carries a background ScanErr result back to the Ebiten
// goroutine, error included — a transient read failure (dir briefly
// unmounted, permission hiccup) needs to be told apart from the dir
// genuinely being empty now, so it can be ignored instead of wiping every
// cart off the screen until the next successful rescan.
type cartsScanResult struct {
	carts []carts.Cart
	err   error
}

// startCartsRescan runs carts.ScanErr off the Ebiten goroutine — a slow or
// networked carts dir shouldn't be able to stall a frame — and applies the
// result once it lands, polled from updateBrowsing like the other
// background work (BBS fetch/replace, file pickers).
func (g *Game) startCartsRescan() {
	if g.cartsScanCh != nil {
		return // one scan in flight at a time
	}
	dir := g.cfg.CartsDir
	ch := make(chan cartsScanResult, 1)
	g.cartsScanCh = ch
	go func() {
		found, err := carts.ScanErr(dir)
		ch <- cartsScanResult{carts: found, err: err}
	}()
}

func (g *Game) pollCartsRescan() {
	if g.cartsScanCh == nil {
		return
	}
	select {
	case res := <-g.cartsScanCh:
		g.cartsScanCh = nil
		if res.err != nil {
			return // transient — leave the current list alone, retry next interval
		}
		g.applyCartsRescan(res.carts)
	default:
	}
}

// applyCartsRescan picks up external changes to the carts dir: adds,
// deletes, and renames all show up as a different carts.Scan result, so
// this just diffs against baseCarts and applies whatever changed.
// Newly-appeared .p8-only carts get enriched against whatever BBS index
// is already loaded, without a fresh network fetch.
func (g *Game) applyCartsRescan(newBase []carts.Cart) {
	if slices.Equal(g.baseCarts, newBase) {
		return
	}

	oldNames := make(map[string]bool, len(g.baseCarts))
	for _, c := range g.baseCarts {
		oldNames[c.Name] = true
	}
	var added []carts.Cart
	for _, c := range newBase {
		if !oldNames[c.Name] {
			added = append(added, c)
		}
	}

	g.baseCarts = newBase
	g.rebuildOrder()

	newNames := make(map[string]bool, len(newBase))
	for _, c := range newBase {
		newNames[c.Name] = true
	}
	for name := range g.bbsBadge {
		if !newNames[name] {
			delete(g.bbsBadge, name)
		}
	}
	for name := range g.bbsUnfound {
		if !newNames[name] {
			delete(g.bbsUnfound, name)
		}
	}
	if g.bbsLastUndo != nil && !newNames[g.bbsLastUndo.cartName] {
		g.bbsLastUndo = nil
	}

	if len(g.bbsIndex) > 0 {
		g.startBBSEnrichmentFor(added)
	} else {
		g.startBBSEnrichment() // index never loaded (or failed) — try again now
	}
}

// startBBSEnrichment fetches the BBS index and enriches every local
// .p8-only cart. All of it runs off the Ebiten goroutine; only the
// finished result is applied to Game state, via bbsResultCh polled from
// updateBrowsing.
func (g *Game) startBBSEnrichment() {
	if g.bbsResultCh != nil {
		return // one pass at a time
	}
	snapshot := append([]carts.Cart(nil), g.baseCarts...)
	ch := make(chan bbsEnrichResult, 1)
	g.bbsResultCh = ch
	go func() {
		index, err := bbsindex.Fetch()
		if err != nil || len(index) == 0 {
			ch <- bbsEnrichResult{}
			return
		}
		ch <- enrichCarts(snapshot, index)
	}()
}

// startBBSEnrichmentFor is startBBSEnrichment scoped to just the carts in
// subset (e.g. ones that just appeared in the carts dir) — reuses the
// already-fetched index instead of hitting the network again.
func (g *Game) startBBSEnrichmentFor(subset []carts.Cart) {
	if g.bbsResultCh != nil || len(g.bbsIndex) == 0 || len(subset) == 0 {
		return
	}
	index := g.bbsIndex
	snapshot := append([]carts.Cart(nil), subset...)
	ch := make(chan bbsEnrichResult, 1)
	g.bbsResultCh = ch
	go func() { ch <- enrichCarts(snapshot, index) }()
}

// enrichCarts matches every local .p8-only cart in snapshot (no cover art
// of its own) against index and either replaces it silently (confident
// match) or flags it with a "?"/"!" badge (no dialog — this app never
// surfaces error/prompt UI, see handoff).
func enrichCarts(snapshot []carts.Cart, index []bbsindex.BBSCart) bbsEnrichResult {
	replaced := map[string]carts.Cart{}
	badges := map[string]bool{}
	unfound := map[string]bool{}
	for _, c := range snapshot {
		if c.Image != "" || !strings.HasSuffix(strings.ToLower(c.Path), ".p8") {
			continue
		}
		title, author := bbsmatch.ParseP8Meta(c.Path)
		searchTitle := title
		if searchTitle == "" {
			searchTitle = c.Name // cross-reference the filename when there's no title comment to go on
		}
		best, _, ok := bbsmatch.Match(searchTitle, author, index)
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
	return bbsEnrichResult{index: index, replaced: replaced, badges: badges, unfound: unfound}
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
		g.snapToCart(selected.Name, g.target)
	}
}

// snapToCart moves the selection to name's occurrence closest to
// preferNear (the pre-rebuild position), with no animation — used after a
// reorder, where the list itself just changed shape rather than the user
// navigating through it. "Closest" matters specifically for launching a
// cart into the recents block: without it, the newly-inserted recents
// duplicate up front would always win as "the first occurrence" and yank
// the view away from the copy the user was actually looking at.
func (g *Game) snapToCart(name string, preferNear float64) {
	n := len(g.allCarts)
	from := wrap(int(math.Round(preferNear)), n)
	bestIdx := -1
	bestDist := math.MaxFloat64
	for i, c := range g.allCarts {
		if c.Name != name {
			continue
		}
		if d := math.Abs(shortestDelta(from, i, n)); d < bestDist {
			bestDist = d
			bestIdx = i
		}
	}
	if bestIdx == -1 {
		return
	}
	g.pos, g.target, g.vel = float64(bestIdx), float64(bestIdx), 0
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
	g.pollCartsRescan()

	if time.Since(g.lastCartsScanAt) > cartsRescanInterval {
		g.lastCartsScanAt = time.Now()
		g.startCartsRescan()
	}

	ctrl := ebiten.IsKeyPressed(ebiten.KeyControlLeft) || ebiten.IsKeyPressed(ebiten.KeyControlRight)
	if ctrl && inpututil.IsKeyJustPressed(ebiten.KeyZ) && g.bbsLastUndo != nil {
		g.undoBBSResolve()
		return
	}

	g.updateCartDelete()

	if inpututil.IsKeyJustPressed(ebiten.KeyBackquote) {
		if g.mode == modeCarasel {
			g.mode = modeList
		} else {
			g.mode = modeCarasel
		}
		return
	}

	for _, r := range ebiten.AppendInputChars(nil) {
		if r == '+' {
			g.openAddCartPicker()
			return
		}
	}

	if inpututil.IsKeyJustPressed(ebiten.KeyTab) {
		shift := ebiten.IsKeyPressed(ebiten.KeyShiftLeft) || ebiten.IsKeyPressed(ebiten.KeyShiftRight)
		cart, ok := g.selectedCart()
		if ok && shift {
			// Shift+Tab: force the full search/replace picker on whatever
			// cart is selected, matched or not.
			g.openBBSResolveForce(cart)
			return
		}
		if ok && (g.bbsBadge[cart.Name] || g.bbsUnfound[cart.Name]) {
			g.openBBSResolve(cart)
			return
		}
		g.pickCarts()
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

// openBBSResolve enters the on-demand picker for a badged cart only.
func (g *Game) openBBSResolve(cart carts.Cart) {
	if !g.bbsBadge[cart.Name] && !g.bbsUnfound[cart.Name] {
		return
	}
	g.openBBSResolveAny(cart)
}

// openBBSResolveForce enters the picker on any cart, badged or not
// (Shift+Tab) — lets you manually override even a cart that already has
// real art.
func (g *Game) openBBSResolveForce(cart carts.Cart) {
	g.openBBSResolveAny(cart)
}

// openBBSResolveAny is the shared setup: a live, editable search box
// pre-filled with the best local guess (the parsed title, or the filename
// when there wasn't one to parse) and re-ranked suggestions below it,
// searching the whole BBS index.
func (g *Game) openBBSResolveAny(cart carts.Cart) {
	title, author := bbsmatch.ParseP8Meta(cart.Path)
	if title == "" {
		title = cart.Name
	}
	g.beginResolve(cart.Name, title, author, g.bbsIndex, g.bbsIndexSorted, false)
}

// openAddCartPicker enters the picker for adding a brand-new cart: the
// same live search, but over the BBS index minus anything whose title
// already matches a cart you have.
func (g *Game) openAddCartPicker() {
	if len(g.bbsIndex) == 0 {
		return
	}
	owned := make(map[string]bool, len(g.baseCarts))
	for _, c := range g.baseCarts {
		t, _ := bbsmatch.ParseP8Meta(c.Path)
		if t == "" {
			t = c.Name
		}
		owned[bbsmatch.Normalize(t)] = true
	}
	pool := make([]bbsindex.BBSCart, 0, len(g.bbsIndex))
	for _, e := range g.bbsIndex {
		if !owned[bbsmatch.Normalize(e.Title)] {
			pool = append(pool, e)
		}
	}
	if len(pool) == 0 {
		return
	}
	sorted := append([]bbsindex.BBSCart(nil), pool...)
	sort.Slice(sorted, func(i, j int) bool {
		return strings.ToLower(sorted[i].Title) < strings.ToLower(sorted[j].Title)
	})
	g.beginResolve("", "", "", pool, sorted, true)
}

func (g *Game) beginResolve(cartName, title, author string, pool, sorted []bbsindex.BBSCart, adding bool) {
	g.resolveCart = cartName
	g.resolveAdding = adding
	g.resolveQuery = title
	g.resolveAuthorHint = author
	g.resolveSearchIndex = pool
	g.resolveSearchSorted = sorted
	g.resolveSel = 0
	g.recomputeResolveSuggestions()
	g.returnState = stateBrowsing
	g.state = stateResolvingBBS
}

// recomputeResolveSuggestions re-ranks against the picker's search pool
// every time resolveQuery is edited. An empty query falls back to browsing
// the whole sorted pool rather than showing nothing.
func (g *Game) recomputeResolveSuggestions() {
	q := strings.TrimSpace(g.resolveQuery)
	if q == "" {
		g.resolveSuggestions = g.resolveSearchSorted
	} else {
		g.resolveSuggestions = bbsmatch.Candidates(q, g.resolveAuthorHint, g.resolveSearchIndex, bbsSuggestionCount)
	}
	if g.resolveSel >= len(g.resolveSuggestions) {
		g.resolveSel = 0
	}
}

func (g *Game) updateResolvingBBS() {
	if g.resolveCh != nil {
		select {
		case res := <-g.resolveCh:
			g.resolveCh = nil
			if res.ok {
				if res.adding {
					g.baseCarts = append(g.baseCarts, res.cart)
					sort.Slice(g.baseCarts, func(i, j int) bool { return g.baseCarts[i].Name < g.baseCarts[j].Name })
					g.bbsLastUndo = &bbsUndo{kind: undoKindAdd, cartName: res.cart.Name, newPath: res.cart.Path}
				} else {
					delete(g.bbsBadge, res.name)
					delete(g.bbsUnfound, res.name)
					for i, c := range g.baseCarts {
						if c.Name == res.name {
							g.baseCarts[i] = res.cart
						}
					}
					g.bbsLastUndo = &bbsUndo{
						kind: undoKindReplace, cartName: res.name,
						originalPath: res.originalPath, newPath: res.cart.Path,
						wasBadge: res.wasBadge, wasUnfound: res.wasUnfound,
					}
				}
				g.rebuildOrder()
			}
			g.state = g.returnState
		default:
		}
		return
	}

	// Scrollable, holdable — same accelerating key-repeat as the main cart
	// browser's left/right, just driving resolveSel instead of g.target.
	if n := len(g.resolveSuggestions); n > 0 {
		if repeatFire(keyDuration(ebiten.KeyUp)) {
			g.resolveSel = wrap(g.resolveSel-1, n)
		}
		if repeatFire(keyDuration(ebiten.KeyDown)) {
			g.resolveSel = wrap(g.resolveSel+1, n)
		}
	}

	g.updateResolveQueryInput()

	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		// bail out: no selection was confirmed, so the badge stays exactly
		// as it was — [Tab] can reopen this picker again later.
		g.state = g.returnState
		return
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEnter) {
		g.confirmBBSResolve()
	}
}

// updateResolveQueryInput is a plain editable text box: typed characters
// append, [Backspace] deletes the last rune, any edit re-ranks suggestions.
func (g *Game) updateResolveQueryInput() {
	edited := false
	for _, r := range ebiten.AppendInputChars(nil) {
		if unicode.IsPrint(r) {
			g.resolveQuery += string(r)
			edited = true
		}
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyBackspace) && len(g.resolveQuery) > 0 {
		runes := []rune(g.resolveQuery)
		g.resolveQuery = string(runes[:len(runes)-1])
		edited = true
	}
	if edited {
		g.recomputeResolveSuggestions()
	}
}

// confirmBBSResolve downloads and swaps in (or adds) the picked
// suggestion off the Ebiten goroutine, same pattern as the file pickers
// (pollPicker).
func (g *Game) confirmBBSResolve() {
	if g.resolveSel >= len(g.resolveSuggestions) {
		return
	}
	picked := g.resolveSuggestions[g.resolveSel]

	if g.resolveAdding {
		g.confirmAddCart(picked)
		return
	}

	name := g.resolveCart
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
	wasBadge := g.bbsBadge[name]
	wasUnfound := g.bbsUnfound[name]

	ch := make(chan bbsResolveResult, 1)
	g.resolveCh = ch
	go func() {
		newPath, err := bbsreplace.Replace(cartPath, picked.PNGURL)
		if err != nil {
			ch <- bbsResolveResult{name: name, ok: false}
			return
		}
		ch <- bbsResolveResult{
			name: name, ok: true, originalPath: cartPath, wasBadge: wasBadge, wasUnfound: wasUnfound,
			cart: carts.Cart{Name: name, Path: newPath, Image: newPath},
		}
	}()
}

// confirmAddCart downloads picked as a brand-new cart file, named after
// its title (sanitized, de-duplicated against what's already on disk).
func (g *Game) confirmAddCart(picked bbsindex.BBSCart) {
	dest := g.newCartPath(picked.Title)
	if dest == "" {
		g.state = g.returnState
		return
	}
	base := strings.TrimSuffix(filepath.Base(dest), ".p8.png")

	ch := make(chan bbsResolveResult, 1)
	g.resolveCh = ch
	go func() {
		if err := bbsreplace.Download(dest, picked.PNGURL); err != nil {
			ch <- bbsResolveResult{ok: false}
			return
		}
		ch <- bbsResolveResult{ok: true, adding: true, cart: carts.Cart{Name: base, Path: dest, Image: dest}}
	}()
}

// newCartPath turns a BBS title into a free .p8.png path in the carts
// dir, appending " (2)", " (3)", ... on a name collision.
func (g *Game) newCartPath(title string) string {
	base := sanitizeCartName(title)
	if base == "" {
		return ""
	}
	candidate := base
	for i := 2; ; i++ {
		p := filepath.Join(g.cfg.CartsDir, candidate+".p8.png")
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return p
		}
		candidate = base + " (" + strconv.Itoa(i) + ")"
	}
}

func sanitizeCartName(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == ' ':
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// undoBBSResolve reverses the single most recent manual resolve action.
func (g *Game) undoBBSResolve() {
	u := g.bbsLastUndo
	if u == nil {
		return
	}
	g.bbsLastUndo = nil
	switch u.kind {
	case undoKindReplace:
		if err := bbsreplace.Undo(u.originalPath, u.newPath); err != nil {
			return
		}
		for i, c := range g.baseCarts {
			if c.Name == u.cartName {
				g.baseCarts[i] = carts.Cart{Name: u.cartName, Path: u.originalPath}
			}
		}
		if u.wasBadge {
			g.bbsBadge[u.cartName] = true
		}
		if u.wasUnfound {
			g.bbsUnfound[u.cartName] = true
		}
	case undoKindAdd:
		if err := os.Remove(u.newPath); err != nil {
			return
		}
		kept := make([]carts.Cart, 0, len(g.baseCarts))
		for _, c := range g.baseCarts {
			if c.Name != u.cartName {
				kept = append(kept, c)
			}
		}
		g.baseCarts = kept
	}
	g.rebuildOrder()
}

// deleteFillTicks is how long [Backspace]/[Delete]/[-] must be held for
// the delete fill to complete and arm.
const deleteFillTicks = 45 // ~0.75s at 60 ticks/sec

var deleteKeys = [3]ebiten.Key{ebiten.KeyBackspace, ebiten.KeyDelete, ebiten.KeyMinus}

// updateCartDelete drives the hold-to-delete gesture on the selected cart:
// fill while held, arm once full, a second press (release + re-press, per
// Ebiten's just-pressed semantics) deletes; any other just-pressed key
// cancels without eating that key's own normal handling this tick.
func (g *Game) updateCartDelete() {
	holdDur := 0
	for _, k := range deleteKeys {
		if d := keyDuration(k); d > holdDur {
			holdDur = d
		}
	}
	held := holdDur > 0
	justPressed := false
	for _, k := range deleteKeys {
		if inpututil.IsKeyJustPressed(k) {
			justPressed = true
		}
	}
	selected, ok := g.selectedCart()

	switch g.deleteState {
	case deleteStateNone:
		if ok && held {
			g.deleteState = deleteStateFilling
			g.deleteCartName = selected.Name
			g.deleteProgress = float64(holdDur) / float64(deleteFillTicks)
			if g.deleteProgress >= 1 {
				g.deleteProgress = 1
				g.deleteState = deleteStateArmed
			}
		}
	case deleteStateFilling:
		if !ok || selected.Name != g.deleteCartName || !held || anyOtherKeyJustPressed() {
			g.resetDeleteState()
			return
		}
		g.deleteProgress = float64(holdDur) / float64(deleteFillTicks)
		if g.deleteProgress >= 1 {
			g.deleteProgress = 1
			g.deleteState = deleteStateArmed
		}
	case deleteStateArmed:
		if ok && selected.Name == g.deleteCartName && justPressed {
			g.confirmCartDelete(selected)
			return
		}
		if anyOtherKeyJustPressed() || !ok || selected.Name != g.deleteCartName {
			g.resetDeleteState()
		}
	}
}

func (g *Game) resetDeleteState() {
	g.deleteState = deleteStateNone
	g.deleteCartName = ""
	g.deleteProgress = 0
}

// confirmCartDelete moves the cart's file(s) to the backup folder and
// drops it from the collection.
func (g *Game) confirmCartDelete(cart carts.Cart) {
	g.resetDeleteState()
	if err := carts.Delete(g.cfg.CartsDir, cart.Name); err != nil {
		return
	}
	kept := make([]carts.Cart, 0, len(g.baseCarts))
	for _, c := range g.baseCarts {
		if c.Name != cart.Name {
			kept = append(kept, c)
		}
	}
	g.baseCarts = kept
	delete(g.bbsBadge, cart.Name)
	delete(g.bbsUnfound, cart.Name)
	g.rebuildOrder()
}

func anyOtherKeyJustPressed() bool {
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		excluded := false
		for _, e := range deleteKeys {
			if k == e {
				excluded = true
				break
			}
		}
		if !excluded {
			return true
		}
	}
	return false
}

func wrap(i, n int) int {
	if n == 0 {
		return 0
	}
	return ((i % n) + n) % n
}
