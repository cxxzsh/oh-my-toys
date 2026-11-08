package topmost

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// nsPopUpMenuWindowLevel is the AppKit level of pop-up menus and the
// tray flyouts: above the menu bar, below screen savers and the lock
// screen.
const nsPopUpMenuWindowLevel = 101

// nsWindowStyleMaskTitled is the style bit that keeps a window's title
// bar inside the visible screen area, which keeps an overlay from
// covering the menu bar.
const nsWindowStyleMaskTitled = 1

type darwinLib struct {
	msgSend                           uintptr
	setLevel, styleMask, setStyle     uintptr
	collectionBehavior, setCollection uintptr
}

var darwinFuncs = sync.OnceValue(func() *darwinLib {
	objc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY)
	if err != nil {
		return nil
	}
	msgSend, err := purego.Dlsym(objc, "objc_msgSend")
	if err != nil {
		return nil
	}
	selReg, err := purego.Dlsym(objc, "sel_registerName")
	if err != nil {
		return nil
	}
	reg := func(name string) uintptr {
		b := append([]byte(name), 0)
		sel, _, _ := purego.SyscallN(selReg, uintptr(unsafe.Pointer(&b[0]))) //nolint:gosec // G103: purego's ABI takes the name as uintptr; b outlives the call
		return sel
	}
	return &darwinLib{
		msgSend:            msgSend,
		setLevel:           reg("setLevel:"),
		styleMask:          reg("styleMask"),
		setStyle:           reg("setStyleMask:"),
		collectionBehavior: reg("collectionBehavior"),
		setCollection:      reg("setCollectionBehavior:"),
	}
})

func aboveAll(handle uintptr) {
	if handle == 0 {
		return
	}
	f := darwinFuncs()
	if f == nil || f.setLevel == 0 {
		return
	}
	purego.SyscallN(f.msgSend, handle, f.setLevel, nsPopUpMenuWindowLevel)
}

// borderless clears the titled style of the window: mygo windows always
// carry it, and AppKit then keeps the title bar inside the visible
// screen, so the window cannot cover the menu bar.
func borderless(handle uintptr) {
	if handle == 0 {
		return
	}
	f := darwinFuncs()
	if f == nil || f.styleMask == 0 || f.setStyle == 0 {
		return
	}
	mask, _, _ := purego.SyscallN(f.msgSend, handle, f.styleMask)
	purego.SyscallN(f.msgSend, handle, f.setStyle, mask&^nsWindowStyleMaskTitled)
}

// nsWindowCollectionBehaviorTransient hides a window when its app is
// deactivated; an overlay must stay while the user works in other apps.
const nsWindowCollectionBehaviorTransient = 1 << 3

// persistent clears the transient collection behavior of the window.
func persistent(handle uintptr) {
	if handle == 0 {
		return
	}
	f := darwinFuncs()
	if f == nil || f.collectionBehavior == 0 || f.setCollection == 0 {
		return
	}
	behavior, _, _ := purego.SyscallN(f.msgSend, handle, f.collectionBehavior)
	purego.SyscallN(f.msgSend, handle, f.setCollection, behavior&^nsWindowCollectionBehaviorTransient)
}

// overlay raises the window, makes it borderless and keeps it on screen
// while the app is inactive, in that order.
func overlay(handle uintptr) {
	borderless(handle)
	persistent(handle)
	aboveAll(handle)
}
