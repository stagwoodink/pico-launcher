//go:build linux

// Package picker opens native, OS-styled file/folder pickers.
package picker

import (
	"strings"

	"github.com/rymdport/portal/filechooser"
)

// On Linux we talk to the xdg-desktop-portal FileChooser directly instead of
// shelling out to zenity/kdialog (often not installed) or a bundled GTK
// dialog (unstyled, breaks under Wayland compositors like Hyprland). The
// portal hands off to whatever the desktop actually provides, themed and
// scaled correctly.

// Directory opens a native folder picker.
func Directory(title string) (string, error) {
	return pick(title, true)
}

// File opens a native file picker.
func File(title string) (string, error) {
	return pick(title, false)
}

func pick(title string, dir bool) (string, error) {
	uris, err := filechooser.OpenFile("", title, &filechooser.OpenFileOptions{Directory: dir})
	if err != nil || len(uris) == 0 {
		return "", err
	}
	return strings.TrimPrefix(uris[0], "file://"), nil
}
