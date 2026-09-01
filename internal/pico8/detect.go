// Package pico8 finds and validates a local PICO-8 install.
package pico8

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// candidatePaths returns common per-OS install locations, most likely first.
func candidatePaths() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		return []string{
			filepath.Join(home, "pico-8", "pico8.exe"),
			`C:\Program Files (x86)\PICO-8\pico8.exe`,
			`C:\Program Files\PICO-8\pico8.exe`,
			filepath.Join(home, "AppData", "Roaming", "itch", "apps", "pico-8", "pico8.exe"),
		}
	case "darwin":
		return []string{
			"/Applications/PICO-8.app/Contents/MacOS/pico8",
			filepath.Join(home, "Applications", "PICO-8.app", "Contents", "MacOS", "pico8"),
			filepath.Join(home, "Library", "Application Support", "itch", "apps", "pico-8", "PICO-8.app", "Contents", "MacOS", "pico8"),
		}
	default: // linux
		return []string{
			filepath.Join(home, "pico-8", "pico8"),
			filepath.Join(home, ".local", "share", "itch", "apps", "pico-8", "pico8"),
			"/usr/local/bin/pico8",
			"/opt/pico-8/pico8",
		}
	}
}

// Find searches known install locations and $PATH for a working PICO-8
// executable. Returns "" if none was found.
func Find() string {
	for _, p := range candidatePaths() {
		if IsValid(p) {
			return p
		}
	}
	if p, err := exec.LookPath("pico8"); err == nil {
		return p
	}
	return ""
}

// IsValid reports whether p is a usable PICO-8 executable, resolving a
// directory pick (e.g. an install folder or a .app bundle) to the binary
// inside it.
func IsValid(p string) bool {
	return Resolve(p) != ""
}

// Resolve turns a user-picked path (file, directory, or .app bundle) into
// the actual executable path, or "" if none can be found there.
func Resolve(p string) string {
	if p == "" {
		return ""
	}
	info, err := os.Stat(p)
	if err != nil {
		return ""
	}
	if !info.IsDir() {
		if isExecutable(info) {
			return p
		}
		return ""
	}
	names := []string{"pico8", "pico8.exe"}
	if runtime.GOOS == "darwin" {
		names = append(names, filepath.Join("Contents", "MacOS", "pico8"))
	}
	for _, n := range names {
		candidate := filepath.Join(p, n)
		if fi, err := os.Stat(candidate); err == nil && !fi.IsDir() && isExecutable(fi) {
			return candidate
		}
	}
	return ""
}

func isExecutable(info os.FileInfo) bool {
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0o111 != 0
}
