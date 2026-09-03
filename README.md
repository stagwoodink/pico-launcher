# pico-launcher

Minimal, title-free launcher for PICO-8 carts. No text, no menus — just cart
covers you browse and launch, on keyboard or controller.

## Why

PICO-8's own splash/load screen is fine, but jumping between many carts means
either the command line or the BBS. This is a tiny always-on-top-of-your-carts
picker: point it at your carts folder once, then browse art and press a
button.

## First run

It tries to find your PICO-8 install and carts folder on its own (common
install paths, then any folder with 5+ carts). If it can't, the window sits
blank — press `Tab` and a native folder picker opens. That's the one and
only prompt this app will ever show you; do it once per missing piece
(PICO-8 install, then carts folder) and it's saved to config for good.

## Browsing

Every cart is always in play — how it's shown just depends on the view:

- **Carasel mode** (default): cover art, centered selection, `←`/`→` or
  D-pad (or `↑`/`↓`) to browse. A `.p8` with no matching `.p8.png` gets a
  hairline placeholder tile with its title centered instead of art.
- **List mode**: every title as a row, current selection in the middle.
- **`` ` `` (backtick)**: toggle between carasel and list mode.
- **Mouse wheel**, or **click-drag/touch-swipe**: scroll either mode.
  Flinging a swipe keeps coasting with a bit of momentum before it settles.
- **Type a letter**: jumps to the first cart whose title starts with it.
  Keep typing within ~0.7s to search a longer prefix; pause and the next
  letter starts a new search.
- **Launch**: `Enter`, or `A`/`Start`/`Select` on a controller. The
  launcher stays open. Launching doesn't yank your view over to the
  recents block, even if this launch is what just pinned the cart there —
  you stay put on whichever copy you were looking at.
- **`Space`**: favorite/unfavorite the current cart (marked `*`).
- **Shift + `←`/`→`/`↑`/`↓`**: reorder — swaps the selected cart with its
  neighbor instead of moving the selection, so the cart stays put while the
  list shifts around it. Saved immediately as your new permanent order.

Up to 3 recently-launched carts are pinned at the very front (newest
first, marked `~`), followed by your favorites (alphabetical, marked
`*`) — a cart in both stays marked `*` but only gets pinned once, by
whichever section it's currently in. Every cart still also appears in
its normal alphabetical spot.

## Deleting a cart

Hold `Backspace`, `Delete`, or `-` on the selected cart. It fills up (white
top-down in carasel; the selection bar fades to grey with the title pulled
left in list mode) over about three-quarters of a second. Once full it
settles into an armed state — let go, then press the same key again to
actually delete it. Pressing any *other* key at any point during the fill
or while armed cancels the delete and does whatever that key normally does
(keeps navigating, launches, whatever).

Deleting never actually destroys anything: the cart's file(s) move into a
`.pico-launcher-backups/` folder inside your carts dir.

## Undo

`Ctrl+Z` reverses the single most recent manual BBS pick — whether you
replaced a cart's art or added a new one from the `+` picker. One level
deep: it's for "wait, wrong one," not a full history.

## BBS cart art

A `.p8` with no cover art gets checked in the background against a daily
scraped index of the PICO-8 BBS (`cmd/bbs-scraper`, `carts.json`, served via
jsDelivr — see `internal/bbsindex`). The match runs against the cart's
`-- title` / `-- by author` Lua comments when present, falling back to the
filename otherwise.

- **Confident match**: downloads the official `.p8.png` and swaps it in
  silently. The original `.p8` moves to `.pico-launcher-backups/`, not
  deleted.
- **`?` badge**: a weaker match, but there are real candidates to pick
  from. Focus the cart and press `Tab`.
- **`!` badge**: nothing to go on at all (no title comment, and the
  filename didn't come close to anything). `Tab` still opens the picker,
  just starting from the entire BBS index instead of a narrowed list.

Either badge opens the same picker: a live, editable search box (start
typing straight away — no separate search key) with suggestions re-ranked
below it as you edit the query. `↑`/`↓` move the selection and repeat/
accelerate the longer you hold them, same as scrolling your own cart list.
`Enter` confirms; `Esc` backs out without touching anything — the badge
stays exactly as it was, so `Tab` reopens it later no worse off.

`Shift+Tab` opens that same picker on *any* selected cart, badged or not —
useful for manually overriding even a cart that already picked up real art,
if it picked up the wrong one.

`Tab` with no cart selected, or on an unbadged cart, reopens the
carts-folder picker instead.

## Adding a cart

Press `+` anywhere while browsing to open the same live search, but scoped
to BBS carts you don't already have (matched by title, so it won't offer
you a duplicate of something already in your collection). `Enter` downloads
the pick as a new cart file, named after its title.

## Build

Requires Go 1.22+ and the native build deps for
[Ebiten](https://ebitengine.org/en/documents/install.html) on your OS
(e.g. GTK dev headers on Linux for windowing + file dialogs).

```sh
go build -o bin/pico-launcher ./cmd/pico-launcher
```

Cross-compiled release binaries for Linux, Windows, and macOS are built by
CI (`.github/workflows/release.yml`) on every version tag and attached to
the GitHub Release.

## Config

A single JSON file at your OS's standard config dir
(`~/.config/pico-launcher/config.json` on Linux, etc.) stores the resolved
PICO-8 path, carts folder, recents, favorites, and your manual sort order.
Delete it to reset everything, or hand-edit it if you just want to clear
recents/favorites/order.

## License

MIT — see [LICENSE](LICENSE).

`internal/ui/assets/pico8_font.png` is Lexaloffle's official PICO-8 font
glyph sheet, used under its CC-0 license for visual consistency with PICO-8
itself.
