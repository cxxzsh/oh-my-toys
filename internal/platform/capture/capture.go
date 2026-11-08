// Package capture takes pictures of the screens, one image per display,
// for the screenshot tool. On macOS it calls CoreGraphics through purego,
// so the build stays CGO_ENABLED=0; other systems are not supported yet.
package capture

import (
	"errors"
	"image"
)

// ErrUnsupported is what every function returns on a system that cannot
// capture yet.
var ErrUnsupported = errors.New("screen capture is not supported on this system")

// Display is a screen that can be captured. Bounds is its rectangle in
// global screen points, with the origin at the top left of the main
// display, the space windows are placed in.
type Display struct {
	ID     uint32
	Bounds image.Rectangle
}

// Shot is the picture of one display, in device pixels: its size is
// Bounds times the display's scale factor.
type Shot struct {
	Display Display
	Image   *image.RGBA
}

// Displays lists the screens that can be captured.
func Displays() ([]Display, error) { return displays() }

// Capture takes a picture of one display in device pixels.
func Capture(d Display) (*image.RGBA, error) { return captureDisplay(d) }

// Snapshot takes a picture of every display.
func Snapshot() ([]Shot, error) {
	displays, err := Displays()
	if err != nil {
		return nil, err
	}
	shots := make([]Shot, 0, len(displays))
	for _, d := range displays {
		img, err := Capture(d)
		if err != nil {
			return nil, err
		}
		shots = append(shots, Shot{Display: d, Image: img})
	}
	return shots, nil
}

// Preflight reports whether the app may capture the screen without
// asking.
func Preflight() bool { return preflight() }

// Request asks the system for screen capture permission, showing its
// prompt when needed, and reports whether it was granted.
func Request() bool { return request() }
