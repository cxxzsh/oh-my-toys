// Package power keeps the computer awake. It wraps MyGo's Power API, which
// already speaks to each system (an NSProcessInfo activity on macOS,
// PowerCreateRequest on Windows, the inhibit portal on Linux).
package power

import (
	"sync"

	"github.com/egoist/mygo"
)

// system asks the system to stay awake; tests replace it. MyGo runs it on
// the main thread, and waits for it there.
var system = mygo.Power.KeepAwake

// KeepAwake stops the computer from sleeping while idle, and the display
// too when display is set, until the returned function is called.
//
// It returns at once, and neither it nor the function it returns waits for
// the system: MyGo's call needs the main thread, and the main thread may be
// waiting for a lock the caller holds, as it does while it draws the state
// of a tool. The function may be called before the system has answered; it
// then lets go as soon as it does.
func KeepAwake(reason string, display bool) (release func()) {
	askLoop()
	held := make(chan func(), 1)
	go func() { held <- system(reason, display) }()
	var once sync.Once
	return func() {
		once.Do(func() { go func() { (<-held)() }() })
	}
}
