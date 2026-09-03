// Package bbsreplace downloads a matched BBS cart image and swaps it in for
// a local .p8, backing up the original first. Side effects only — matching
// itself lives in internal/bbsmatch, which stays pure and testable.
package bbsreplace

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stagwoodink/pico-launcher/internal/carts"
	"github.com/stagwoodink/pico-launcher/internal/httpfetch"
)

// Replace downloads pngURL and swaps it in for cartPath — a .p8 *or* a
// .p8.png (Shift+Tab can force-open the picker on a cart that already has
// real art, to manually pick a different one). The original is moved into
// carts.BackupDirName next to it, and the download is written as a .p8.png
// with the same base name — carts.Scan already prefers .p8.png over .p8
// for a shared base name, so this alone is what makes the app pick it up.
// Returns the new file's path.
func Replace(cartPath, pngURL string) (string, error) {
	body, err := httpfetch.Get(pngURL, 20*time.Second)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(cartPath)
	backupDir := filepath.Join(dir, carts.BackupDirName)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(cartPath, BackupPath(cartPath)); err != nil {
		return "", err
	}

	newPath := newPathFor(cartPath)
	if err := os.WriteFile(newPath, body, 0o644); err != nil {
		// Put the original back so a write failure (disk full, permissions)
		// never leaves the cart stranded in the backup folder with nothing
		// in its place.
		_ = os.Rename(BackupPath(cartPath), cartPath)
		return "", err
	}
	return newPath, nil
}

// Undo reverses a Replace: removes the downloaded newPath and moves the
// backed-up original back to cartPath. cartPath and newPath can be the same
// file (Shift+Tab replacing a cart that was already a .p8.png) — in that
// case the remove must run first, or restoring the original would just get
// immediately deleted again. When they differ, restoring first is safer: a
// failure on the remove step then leaves a harmless leftover .p8.png next
// to the recovered original, rather than losing the cart entirely if the
// rename never runs.
func Undo(cartPath, newPath string) error {
	if cartPath == newPath {
		if err := os.Remove(newPath); err != nil {
			return err
		}
		return os.Rename(BackupPath(cartPath), cartPath)
	}
	if err := os.Rename(BackupPath(cartPath), cartPath); err != nil {
		return err
	}
	return os.Remove(newPath)
}

// Download saves pngURL directly to destPath — for adding a brand-new
// cart, where there's no existing file to back up or swap.
func Download(destPath, pngURL string) error {
	body, err := httpfetch.Get(pngURL, 20*time.Second)
	if err != nil {
		return err
	}
	return os.WriteFile(destPath, body, 0o644)
}

// BackupPath is where Replace/Delete moves cartPath's original file to.
func BackupPath(cartPath string) string {
	return filepath.Join(filepath.Dir(cartPath), carts.BackupDirName, filepath.Base(cartPath))
}

// newPathFor strips whichever cart extension cartPath has (.p8 or
// .p8.png) and rebuilds it as .p8.png — filepath.Ext alone would only
// strip ".png" off a ".p8.png" path, corrupting the result.
func newPathFor(cartPath string) string {
	base := strings.TrimSuffix(cartPath, ".p8.png")
	base = strings.TrimSuffix(base, ".p8")
	return base + ".p8.png"
}
