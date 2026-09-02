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
- **Launch and close launcher**: `Space`/`Enter`, or `A`/`Start` on a
  controller.
- **Launch and keep launcher open**: hold `Shift` while launching, or use
  `Select`/`Back` on a controller.

First run: it tries to find your PICO-8 install and carts folder on its own
(common install paths, then any folder with 5+ carts). If it can't, it'll
prompt `hit [tab]` and open a native folder picker — a one-time thing, saved
to config after.

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
PICO-8 path and carts folder. Delete it to re-run detection from scratch.

## License

MIT — see [LICENSE](LICENSE).

`internal/ui/assets/pico8_font.png` is Lexaloffle's official PICO-8 font
glyph sheet, used under its CC-0 license for visual consistency with PICO-8
itself.
