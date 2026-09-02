// Package config persists the user's PICO-8 install path and carts directory
// so setup only ever runs once per machine.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// maxRecents caps how many recently-launched carts are remembered and
// pinned to the front of the list.
const maxRecents = 3

type Config struct {
	Pico8Path string `json:"pico8_path"`
	CartsDir  string `json:"carts_dir"`

	// RecentNames is cart names, most-recently-launched first, capped at
	// maxRecents. FavoriteNames is unordered; display order is decided by
	// the UI layer, not stored here.
	RecentNames   []string `json:"recent_names,omitempty"`
	FavoriteNames []string `json:"favorite_names,omitempty"`
}

// TouchRecent moves name to the front of RecentNames (adding it if new)
// and trims to maxRecents.
func (c *Config) TouchRecent(name string) {
	out := []string{name}
	for _, n := range c.RecentNames {
		if n != name {
			out = append(out, n)
		}
	}
	if len(out) > maxRecents {
		out = out[:maxRecents]
	}
	c.RecentNames = out
}

// ToggleFavorite adds name to FavoriteNames if absent, removes it if
// present.
func (c *Config) ToggleFavorite(name string) {
	for i, n := range c.FavoriteNames {
		if n == name {
			c.FavoriteNames = append(c.FavoriteNames[:i], c.FavoriteNames[i+1:]...)
			return
		}
	}
	c.FavoriteNames = append(c.FavoriteNames, name)
}

func path() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "pico-launcher", "config.json"), nil
}

// Load returns a zero-value Config (no error) if none was saved yet.
func Load() (Config, error) {
	p, err := path()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	return c, nil
}

func (c Config) Save() error {
	p, err := path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
