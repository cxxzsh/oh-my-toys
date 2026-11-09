//go:build !darwin && !windows && !linux

package terminal

import "context"

func open(context.Context, []Var) error { return ErrUnsupported }
