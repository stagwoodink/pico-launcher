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
)

// backupDirName is the sibling folder, alongside the carts dir, that
// replaced originals get moved into.
const backupDirName = ".pico-launcher-backups"

// Replace downloads pngURL and swaps it in for cartPath (a .p8 file): the
// original is moved into backupDirName next to it, and the download is
// written as a .p8.png with the same base name in the same directory —
// carts.Scan already prefers .p8.png over .p8 for a shared base name, so
// this alone is what makes the app pick up the official cart. Returns the
// new file's path.
func Replace(cartPath, pngURL string) (string, error) {
	body, err := download(pngURL)
	if err != nil {
		return "", err
	}

	dir := filepath.Dir(cartPath)
	backupDir := filepath.Join(dir, backupDirName)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(cartPath, filepath.Join(backupDir, filepath.Base(cartPath))); err != nil {
		return "", err
	}

	base := strings.TrimSuffix(filepath.Base(cartPath), filepath.Ext(cartPath))
	newPath := filepath.Join(dir, base+".p8.png")
	if err := os.WriteFile(newPath, body, 0o644); err != nil {
		return "", err
	}
	return newPath, nil
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
