//go:build !darwin

package capture

import (
	"fmt"
	"image"
)

func preflight() bool { return false }

func request() bool { return false }

func displays() ([]Display, error) {
	return nil, ErrUnsupported
}

func captureDisplay(Display) (*image.RGBA, error) {
	return nil, fmt.Errorf("capture: %w", ErrUnsupported)
}
