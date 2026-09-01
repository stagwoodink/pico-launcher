//go:build !linux

// Package picker opens native, OS-styled file/folder pickers.
package picker

import "github.com/sqweek/dialog"

// Windows and macOS backends of sqweek/dialog call the real Win32 common
// dialog and Cocoa NSOpenPanel respectively, so they're native there
// already — only Linux needed a different approach (see picker_linux.go).

func Directory(title string) (string, error) {
	return dialog.Directory().Title(title).Browse()
}

func File(title string) (string, error) {
	return dialog.File().Title(title).Load()
}
