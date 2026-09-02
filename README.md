# pico-launcher

Minimal, title-free launcher for PICO-8 carts. No text, no menus — just cart
covers you browse and launch, on keyboard or controller.

## Why

PICO-8's own splash/load screen is fine, but jumping between many carts means
either the command line or the BBS. This is a tiny always-on-top-of-your-carts
picker: point it at your carts folder once, then browse art and press a
button.

## Interface

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
  launcher stays open.
- **`Space`**: favorite/unfavorite the current cart (marked `*`).

Up to 3 recently-launched carts are pinned at the very front (newest
first, marked `~`), followed by your favorites (alphabetical, marked
`*`) — a cart in both stays marked `*` but only gets pinned once, by
whichever section it's currently in. Every cart still also appears in
its normal alphabetical spot.

First run: it tries to find your PICO-8 install and carts folder on its own
(common install paths, then any folder with 5+ carts). If it can't, it'll
prompt `hit [tab]` and open a native folder picker — a one-time thing, saved
to config after.

### BBS cart art

A `.p8` with no cover art gets checked in the background against a daily
scraped index of the PICO-8 BBS (`cmd/bbs-scraper`, `carts.json`, served via
jsDelivr — see `internal/bbsindex`). A confident title/author match
downloads the official `.p8.png` and swaps it in silently; the original
`.p8` is kept in a `.pico-launcher-backups/` folder alongside your carts, not
deleted. A weak match gets a `?` badge instead of guessing — focus that cart
and press `[Tab]` to pick from the closest candidates or keep it as-is
(`[Esc]`/`[Backspace]`). `[Tab]` otherwise reopens the carts-folder picker,
same as before — it only does BBS resolution when the focused cart has a
pending `?`.

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
PICO-8 path, carts folder, recents, and favorites. Delete it to reset
everything, or hand-edit it if you just want to clear recents/favorites.

## License

MIT — see [LICENSE](LICENSE).

`internal/ui/assets/pico8_font.png` is Lexaloffle's official PICO-8 font
glyph sheet, used under its CC-0 license for visual consistency with PICO-8
itself.
