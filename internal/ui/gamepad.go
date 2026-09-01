package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Standard gamepad button mapping (Xbox/PlayStation/generic all agree on
// this via the browser Standard Gamepad layout ebiten follows):
//   - aButton: bottom face button (A / Cross) — primary "launch" button
//   - startButton: center-right (Start / Options)
//   - selectButton: center-left (Select / Back / Share) — "keep open" launch
const (
	aButton      = ebiten.StandardGamepadButtonRightBottom
	startButton  = ebiten.StandardGamepadButtonCenterRight
	selectButton = ebiten.StandardGamepadButtonCenterLeft
	dpadLeft     = ebiten.StandardGamepadButtonLeftLeft
	dpadRight    = ebiten.StandardGamepadButtonLeftRight
	dpadUp       = ebiten.StandardGamepadButtonLeftTop
	dpadDown     = ebiten.StandardGamepadButtonLeftBottom
)

// gamepadIDs returns the connected gamepads, or none while the window
// isn't focused. Ebiten's gamepad backend reads raw OS joystick state,
// which (unlike keyboard/mouse) isn't scoped to window focus on its own —
// without this, held controller input would keep driving the launcher
// even while some other window is focused.
func gamepadIDs() []ebiten.GamepadID {
	if !ebiten.IsFocused() {
		return nil
	}
	return ebiten.AppendGamepadIDs(nil)
}

func padJustPressed(b ebiten.StandardGamepadButton) bool {
	for _, id := range gamepadIDs() {
		if !ebiten.IsStandardGamepadLayoutAvailable(id) {
			continue
		}
		if inpututil.IsStandardGamepadButtonJustPressed(id, b) {
			return true
		}
	}
	return false
}

func padHeld(b ebiten.StandardGamepadButton) bool {
	for _, id := range gamepadIDs() {
		if !ebiten.IsStandardGamepadLayoutAvailable(id) {
			continue
		}
		if ebiten.IsStandardGamepadButtonPressed(id, b) {
			return true
		}
	}
	return false
}

// padDuration returns how many ticks b has been held across all connected
// gamepads (0 if it isn't currently held anywhere).
func padDuration(b ebiten.StandardGamepadButton) int {
	max := 0
	for _, id := range gamepadIDs() {
		if !ebiten.IsStandardGamepadLayoutAvailable(id) {
			continue
		}
		if d := inpututil.StandardGamepadButtonPressDuration(id, b); d > max {
			max = d
		}
	}
	return max
}
