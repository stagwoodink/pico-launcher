// Package launcher runs PICO-8 with a cart and verifies it actually started.
package launcher

import (
	"os/exec"
	"time"
)

// graceWindow is how long we watch a freshly started PICO-8 process for an
// instant crash (bad cart path, bad executable, etc.) before trusting it.
const graceWindow = 750 * time.Millisecond

// Launch starts pico8Path with cartPath. It reports ok=true once the process
// has survived the grace window, so callers can tell a real launch from a
// silent instant failure without showing the user an error dialog directly.
func Launch(pico8Path, cartPath string) (ok bool, cmd *exec.Cmd) {
	c := exec.Command(pico8Path, "-run", cartPath)
	if err := c.Start(); err != nil {
		return false, nil
	}

	done := make(chan error, 1)
	go func() { done <- c.Wait() }()

	select {
	case err := <-done:
		return err == nil, c // exited within the window; ok only if clean exit
	case <-time.After(graceWindow):
		return true, c // still running past the grace window: treat as launched
	}
}
