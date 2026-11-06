//go:build darwin || linux

package main

import (
	"syscall"
	"unsafe"
)

func isatty(fd uintptr) bool {
	var termios syscall.Termios
	//nolint:gosec // ioctl needs the termios address; it only reads whether fd is a terminal
	_, _, err := syscall.Syscall6(syscall.SYS_IOCTL, fd, ioctlReadTermios, uintptr(unsafe.Pointer(&termios)), 0, 0, 0)
	return err == 0
}
