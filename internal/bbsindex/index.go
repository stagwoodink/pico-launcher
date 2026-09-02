// Package bbsindex is the JSON schema for the scraped BBS cart index and the
// app-side fetch/cache of it. The scraper (cmd/bbs-scraper) writes this same
// schema independently — the two only agree on the JSON shape, never call
// into each other, which is the point (see handoff: scraper stays decoupled
// from the running app).
package bbsindex

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// BBSCart is one cart as published on the PICO-8 BBS.
type BBSCart struct {
	ID      string `json:"id"`    // BBS lid, e.g. "oswald_the_lucky_rabbit_000-1"
	Title   string `json:"title"`
	Author  string `json:"author"`
	TID     int    `json:"tid"`   // BBS forum thread id
	PNGURL  string `json:"png_url"` // absolute URL to the official .p8.png
}

// IndexURL is where the app fetches the scraper's published output from.
const IndexURL = "https://cdn.jsdelivr.net/gh/stagwoodink/pico-launcher@master/carts.json"

func cachePath() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "pico-launcher", "bbs-carts.json")
}

// Fetch returns the current BBS index, preferring a fresh download but
// falling back to the last cached copy if the network fails — this feature
// is best-effort background enrichment, never something worth blocking or
// erroring the launcher over.
func Fetch() ([]BBSCart, error) {
	cache := cachePath()
	if body, err := download(IndexURL); err == nil {
		if cache != "" {
			_ = os.MkdirAll(filepath.Dir(cache), 0o755)
			_ = os.WriteFile(cache, body, 0o644)
		}
		return decode(body)
	}
	if cache == "" {
		return nil, os.ErrNotExist
	}
	body, err := os.ReadFile(cache)
	if err != nil {
		return nil, err
	}
	return decode(body)
}

func download(url string) ([]byte, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &httpStatusError{resp.StatusCode}
	}
	return io.ReadAll(resp.Body)
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string { return http.StatusText(e.code) }

func decode(body []byte) ([]BBSCart, error) {
	var carts []BBSCart
	if err := json.Unmarshal(body, &carts); err != nil {
		return nil, err
	}
	return carts, nil
}
