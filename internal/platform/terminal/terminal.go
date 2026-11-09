// Package terminal opens a terminal window with some environment variables
// set.
package terminal

import (
	"context"
	"errors"
)

// ErrUnsupported means this system has no terminal this package can open.
var ErrUnsupported = errors.New("opening a terminal is not supported on this system")

// Var is one environment variable.
type Var struct{ Name, Value string }

// Open opens a terminal window whose shell starts with vars set. The window
// belongs to the person from then on: the call returns once it is open, not
// when it closes.
func Open(ctx context.Context, vars []Var) error { return open(ctx, vars) }
