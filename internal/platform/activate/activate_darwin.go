package activate

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// nsApplicationActivateIgnoringOtherApps activates an app even while
// another holds the focus.
const nsApplicationActivateIgnoringOtherApps = 2

type darwinLib struct {
	msgSend   uintptr
	workspace uintptr // NSWorkspace sharedWorkspace

	frontmostApp  uintptr // -[NSWorkspace frontmostApplication]
	processID     uintptr // -[NSRunningApplication processIdentifier]
	runningApp    uintptr // +[NSRunningApplication runningApplicationWithProcessIdentifier:]
	activateApp   uintptr // -[NSRunningApplication activateWithOptions:]
	runningAppCls uintptr // NSRunningApplication class
	activateSelf  uintptr // -[NSApplication activateIgnoringOtherApps:]
	nsApp         uintptr // NSApp global
}

var darwinFuncs = sync.OnceValue(func() *darwinLib {
	objc, err := purego.Dlopen("/usr/lib/libobjc.A.dylib", purego.RTLD_LAZY)
	if err != nil {
		return nil
	}
	getClass, err := purego.Dlsym(objc, "objc_getClass")
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
	class := func(name string) uintptr {
		b := append([]byte(name), 0)
		c, _, _ := purego.SyscallN(getClass, uintptr(unsafe.Pointer(&b[0]))) //nolint:gosec // G103: purego's ABI takes the name as uintptr; b outlives the call
		return c
	}
	reg := func(name string) uintptr {
		b := append([]byte(name), 0)
		sel, _, _ := purego.SyscallN(selReg, uintptr(unsafe.Pointer(&b[0]))) //nolint:gosec // G103: purego's ABI takes the name as uintptr; b outlives the call
		return sel
	}
	workspaceClass := class("NSWorkspace")
	if workspaceClass == 0 {
		return nil
	}
	workspace, _, _ := purego.SyscallN(msgSend, workspaceClass, reg("sharedWorkspace"))
	return &darwinLib{
		msgSend:       msgSend,
		workspace:     workspace,
		frontmostApp:  reg("frontmostApplication"),
		processID:     reg("processIdentifier"),
		runningApp:    reg("runningApplicationWithProcessIdentifier:"),
		activateApp:   reg("activateWithOptions:"),
		runningAppCls: class("NSRunningApplication"),
		activateSelf:  reg("activateIgnoringOtherApps:"),
		nsApp:         nsAppGlobal(objc),
	}
})

func frontmost() int {
	f := darwinFuncs()
	if f == nil || f.workspace == 0 {
		return 0
	}
	app, _, _ := purego.SyscallN(f.msgSend, f.workspace, f.frontmostApp)
	if app == 0 {
		return 0
	}
	pid, _, _ := purego.SyscallN(f.msgSend, app, f.processID)
	return int(pid)
}

func activate(pid int) {
	f := darwinFuncs()
	if f == nil {
		return
	}
	app, _, _ := purego.SyscallN(f.msgSend, f.runningAppCls, f.runningApp, uintptr(pid))
	if app == 0 {
		return
	}
	purego.SyscallN(f.msgSend, app, f.activateApp, nsApplicationActivateIgnoringOtherApps)
}

func nsAppGlobal(objc uintptr) uintptr {
	getClass, err := purego.Dlsym(objc, "objc_getClass")
	if err != nil {
		return 0
	}
	msgSend, err := purego.Dlsym(objc, "objc_msgSend")
	if err != nil {
		return 0
	}
	selReg, err := purego.Dlsym(objc, "sel_registerName")
	if err != nil {
		return 0
	}
	name := append([]byte("NSApplication"), 0)
	cls, _, _ := purego.SyscallN(getClass, uintptr(unsafe.Pointer(&name[0]))) //nolint:gosec // G103: purego's ABI takes the name as uintptr; name outlives the call
	selName := append([]byte("sharedApplication"), 0)
	sel, _, _ := purego.SyscallN(selReg, uintptr(unsafe.Pointer(&selName[0]))) //nolint:gosec // G103: purego's ABI takes the name as uintptr; selName outlives the call
	app, _, _ := purego.SyscallN(msgSend, cls, sel)
	return app
}

func activateSelf() {
	f := darwinFuncs()
	if f == nil || f.nsApp == 0 || f.activateSelf == 0 {
		return
	}
	purego.SyscallN(f.msgSend, f.nsApp, f.activateSelf, 1)
}
