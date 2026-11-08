// Package windows lists the windows on screen, so the screenshot tool can
// snap a selection to a window's edges. On macOS it calls CoreGraphics
// through purego, so the build stays CGO_ENABLED=0; other systems are not
// supported yet.
package windows

import (
	"errors"
	"image"
)

// ErrUnsupported is what List returns on a system that cannot list
// windows yet.
var ErrUnsupported = errors.New("listing windows is not supported on this system")

// List returns the frames of the on-screen, normal-layer windows, in
// front-to-back order. A frame is in global screen points with the origin
// at the top left of the main display, the same space as the display
// bounds of the capture package. It returns an empty slice and a nil
// error when no window qualifies.
func List() ([]image.Rectangle, error) { return list() }
