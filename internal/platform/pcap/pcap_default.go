//go:build darwin || linux || windows

package pcap

import "slices"

// pickDefault names the interface the default route most likely goes
// through when the system will not say. The order is the first interface
// up and not loopback, then the first not loopback, then the first: the
// last two steps cover a libpcap too old to fill the flag bits, where
// every interface reports no flags at all.
func pickDefault(ifaces []Interface) (string, bool) {
	if i := slices.IndexFunc(ifaces, func(iface Interface) bool { return iface.Up && !iface.Loopback }); i >= 0 {
		return ifaces[i].Name, true
	}
	if i := slices.IndexFunc(ifaces, func(iface Interface) bool { return !iface.Loopback }); i >= 0 {
		return ifaces[i].Name, true
	}
	if len(ifaces) == 0 {
		return "", false
	}
	return ifaces[0].Name, true
}
