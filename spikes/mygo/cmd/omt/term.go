package main

import "os"

// isTerminal reports whether f is a terminal. A character device such as
// /dev/null is not: a script that redirects standard input must not be asked
// to confirm.
func isTerminal(f *os.File) bool {
	return isatty(f.Fd())
}
