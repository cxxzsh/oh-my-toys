// Package activate reports and restores the frontmost application, so
// the screenshot overlay can take the keyboard and give it back.
package activate

import "os"

// Frontmost returns the pid of the frontmost application, or 0 when
// there is none.
func Frontmost() int { return frontmost() }

// IsFrontmost reports whether this process is the frontmost application.
func IsFrontmost() bool { return frontmost() == os.Getpid() }

// Activate brings the application with the given pid to the front.
func Activate(pid int) { activate(pid) }

// Self brings this process to the front.
func Self() { activateSelf() }
