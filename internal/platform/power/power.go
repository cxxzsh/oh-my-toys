// Package power keeps the computer awake. It wraps MyGo's Power API, which
// already speaks to each system (an NSProcessInfo activity on macOS,
// PowerCreateRequest on Windows, the inhibit portal on Linux).
package power

import "github.com/egoist/mygo"

// KeepAwake stops the computer from sleeping while idle, and the display
// too when display is set, until the returned function is called.
func KeepAwake(reason string, display bool) (release func()) {
	askLoop()
	return mygo.Power.KeepAwake(reason, display)
}
