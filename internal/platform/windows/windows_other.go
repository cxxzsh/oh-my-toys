//go:build !darwin

package windows

import "image"

func list() ([]image.Rectangle, error) {
	return nil, ErrUnsupported
}
