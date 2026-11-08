package main

import (
	"syscall"
	"unsafe"
)

var getConsoleMode = syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleMode")

func isatty(fd uintptr) bool {
	var mode uint32
	r, _, _ := getConsoleMode.Call(fd, uintptr(unsafe.Pointer(&mode))) //nolint:gosec // G103: GetConsoleMode writes the mode through the pointer; &mode outlives the call
	return r != 0
}
