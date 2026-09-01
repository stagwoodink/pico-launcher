// Command pico-launcher is a minimal, title-free PICO-8 cart launcher.
package main

import (
	"log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/stagwoodink/pico-launcher/internal/config"
	"github.com/stagwoodink/pico-launcher/internal/ui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Printf("config load failed, starting fresh: %v", err)
	}

	ebiten.SetWindowSize(ui.ScreenW, ui.ScreenH)
	ebiten.SetWindowTitle("PICO-8 Launcher")
	ebiten.SetWindowResizable(true)

	if err := ebiten.RunGame(ui.New(cfg)); err != nil {
		log.Fatal(err)
	}
}
