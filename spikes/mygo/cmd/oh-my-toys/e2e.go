//go:build e2e

package main

// The e2e build links MyGo's automation plugin, which listens on the socket
// MYGO_AUTOMATION names, for cmd/e2e to drive the app as a screen reader
// sees it. Release builds leave it out.
import _ "github.com/egoist/mygo/plugins/automation"
