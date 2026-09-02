// Package bbsreplace downloads a matched BBS cart image and swaps it in for
// a local .p8, backing up the original first. Side effects only — matching
// itself lives in internal/bbsmatch, which stays pure and testable.
package bbsreplace

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/stagwoodink/pico-launcher/internal/carts"
)

// Replace downloads pngURL and swaps it in for cartPath — a .p8 *or* a
// .p8.png (Shift+Tab can force-open the picker on a cart that already has
// real art, to manually pick a different one). The original is moved into
// carts.BackupDirName next to it, and the download is written as a .p8.png
// with the same base name — carts.Scan already prefers .p8.png over .p8
// for a shared base name, so this alone is what makes the app pick it up.
// Returns the new file's path.
func Replace(cartPath, pngURL string) (string, error) {
	body, err := download(pngURL)
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
		return "", err
	}
	return newPath, nil
}

// Undo reverses a Replace: removes the downloaded newPath and moves the
// backed-up original back to cartPath.
func Undo(cartPath, newPath string) error {
	if err := os.Remove(newPath); err != nil {
		return err
	}
	return os.Rename(BackupPath(cartPath), cartPath)
}

// Download saves pngURL directly to destPath — for adding a brand-new
// cart, where there's no existing file to back up or swap.
func Download(destPath, pngURL string) error {
	body, err := download(pngURL)
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

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{resp.StatusCode}
	}
	return io.ReadAll(resp.Body)
}

type statusError struct{ code int }

func (e *statusError) Error() string { return http.StatusText(e.code) }
