// Package topmost styles a window for the screenshot overlay on
// macOS: above everything, and without the title bar that would keep
// it from covering the menu bar.
package topmost

// Overlay raises the window with the given native handle (NSWindow* on
// macOS) to the level of a pop-up menu and clears its titled style, so
// it covers the menu bar and every ordinary window. It does nothing on
// systems where the overlay does not exist yet.
func Overlay(handle uintptr) { overlay(handle) }
