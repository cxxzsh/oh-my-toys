package power

import (
	"log"
	"sync"

	"github.com/egoist/mygo"
)

// needLoop is closed by the first KeepAwake of a process run by Host. It is
// nil in the desktop app, which has its own event loop.
var (
	needLoop     chan struct{}
	needLoopOnce sync.Once
)

// Host runs work in a process that is not the desktop app and returns its
// exit code. The system calls of KeepAwake need MyGo's event loop on the main
// goroutine, so Host starts it, with no window and no Dock icon, when work
// first asks to keep the computer awake, and stops it when work returns.
// A command that never asks never loads the loop.
func Host(work func() int) int {
	needLoop = make(chan struct{})
	done := make(chan int, 1)
	go func() { done <- work() }()
	select {
	case code := <-done:
		return code
	case <-needLoop:
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyProhibited)
	go func() {
		code := <-done
		done <- code
		mygo.App.Quit()
	}()
	if err := mygo.App.Run(); err != nil {
		log.Print(err)
	}
	return <-done
}

// askLoop tells Host to start the event loop, when there is a Host.
func askLoop() {
	if needLoop != nil {
		needLoopOnce.Do(func() { close(needLoop) })
	}
}
