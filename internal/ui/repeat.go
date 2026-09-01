package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Held-key repeat timing, in ticks (Ebiten runs Update at a fixed tick
// rate, default 60/s): fire immediately on press, wait initialDelay before
// repeating, repeat at a steady pace, then speed up the longer it's held.
const (
	repeatInitialDelay = 24 // ~0.4s before repeat kicks in
	repeatSlowInterval = 9  // ~0.15s between repeats at first
	repeatFastAfter    = 90 // ~1.5s held before speeding up
	repeatFastInterval = 3  // ~0.05s between repeats once fast
)

// repeatFire reports whether a direction held for d ticks should move the
// selection this tick. d is 0 when not held, 1 on the tick it was pressed.
func repeatFire(d int) bool {
	if d <= 0 {
		return false
	}
	if d == 1 {
		return true
	}
	if d < repeatInitialDelay {
		return false
	}
	held := d - repeatInitialDelay
	interval := repeatSlowInterval
	if d >= repeatFastAfter {
		interval = repeatFastInterval
	}
	return held%interval == 0
}

// keyDuration is the ticks-held count for a key, 0 if not currently held.
func keyDuration(k ebiten.Key) int {
	return inpututil.KeyPressDuration(k)
}
