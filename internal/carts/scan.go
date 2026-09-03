// Package carts locates a carts directory and lists the PICO-8 carts in it.
package carts

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// minCartsForAutoDetect is how many carts a directory needs before we trust
// it as "the" carts folder without asking the user.
const minCartsForAutoDetect = 5

// BackupDirName is the sibling folder, inside the carts dir, that a
// deleted or BBS-replaced cart's original file gets moved into instead of
// being permanently removed.
const BackupDirName = ".pico-launcher-backups"

// Cart is a single playable cartridge. Image is set when a .p8.png with the
// same base name exists; a bare .p8 with no image sibling has Image == "".
type Cart struct {
	Name  string // base name, no extension
	Path  string // path to launch (prefers .p8.png)
	Image string // path to the .p8.png for thumbnailing, "" if none
}

// candidateDirs returns common carts locations, most likely first.
func candidateDirs() []string {
	home, _ := os.UserHomeDir()
	return []string{
		filepath.Join(home, "pico-8", "carts"),
		filepath.Join(home, "Documents", "PICO-8", "carts"),
		filepath.Join(home, "Desktop"),
		filepath.Join(home, "Downloads"),
	}
}

// FindDir searches known locations for a directory containing enough carts
// to auto-select with confidence. Returns "" if none qualifies.
func FindDir() string {
	for _, d := range candidateDirs() {
		if n := countCartFiles(d); n >= minCartsForAutoDetect {
			return d
		}
	}
	return ""
}

func countCartFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && isCartFile(e.Name()) {
			n++
		}
	}
	return n
}

func isCartFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".p8") || strings.HasSuffix(lower, ".p8.png")
}

func baseName(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".p8.png"):
		return name[:len(name)-len(".p8.png")]
	case strings.HasSuffix(lower, ".p8"):
		return name[:len(name)-len(".p8")]
	}
	return name
}

// Scan lists carts in dir, deduplicated by base name with the .p8.png
// version preferred when both exist for the same cart. Returns nil on a
// read error exactly like an empty dir would — callers that need to tell
// "genuinely empty" apart from "couldn't read it right now" (a rescan while
// already browsing, where the latter shouldn't wipe what's on screen) should
// use ScanErr instead.
func Scan(dir string) []Cart {
	carts, _ := ScanErr(dir)
	return carts
}

// ScanErr is Scan, but reports a read error instead of masking it as "no
// carts" — a transient failure (dir briefly unmounted, permission hiccup)
// shouldn't be indistinguishable from every cart having been deleted.
func ScanErr(dir string) ([]Cart, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	byName := map[string]*Cart{}
	for _, e := range entries {
		if e.IsDir() || !isCartFile(e.Name()) {
			continue
		}
		name := e.Name()
		base := baseName(name)
		full := filepath.Join(dir, name)
		c := byName[base]
		if c == nil {
			c = &Cart{Name: base}
			byName[base] = c
		}
		if strings.HasSuffix(strings.ToLower(name), ".p8.png") {
			c.Image = full
			c.Path = full
		} else if c.Path == "" {
			c.Path = full
		}
	}
	out := make([]Cart, 0, len(byName))
	for _, c := range byName {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Delete moves a cart's file(s) — its .p8 and/or .p8.png, whichever exist —
// into BackupDirName instead of removing them, so it's reversible. Returns
// os.ErrNotExist if neither file was found.
func Delete(dir, name string) error {
	backupDir := filepath.Join(dir, BackupDirName)
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	moved := false
	for _, ext := range []string{".p8", ".p8.png"} {
		src := filepath.Join(dir, name+ext)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		if err := os.Rename(src, filepath.Join(backupDir, name+ext)); err != nil {
			return err
		}
		moved = true
	}
	if !moved {
		return os.ErrNotExist
	}
	return nil
}
