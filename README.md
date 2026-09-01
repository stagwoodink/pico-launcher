# pico-launcher

Minimal, title-free launcher for PICO-8 carts. No text, no menus — just cart
covers you browse and launch, on keyboard or controller.

## Why

PICO-8's own splash/load screen is fine, but jumping between many carts means
either the command line or the BBS. This is a tiny always-on-top-of-your-carts
picker: point it at your carts folder once, then browse art and press a
button.

## Interface

- **Cart covers** (`.p8.png`, left carasel): `←`/`→` or D-pad to browse.
- **Plain carts** (`.p8` with no matching cover, right list): `↑`/`↓` or
  D-pad to browse. Whichever panel is empty just doesn't show.
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
