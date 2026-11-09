package traffic

import (
	"net/netip"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// programHook names the program that owns the client end of a connection to
// the proxy listening on addr. It returns nil when addr is not an address, or
// when there is no way to find owners, so the proxy then names no program.
func (t *Tool) programHook(addr string) func(netip.AddrPort) tool.Program {
	proxyEnd, err := netip.ParseAddrPort(addr)
	owner := t.sys.owner
	if err != nil || owner == nil {
		return nil
	}
	return func(client netip.AddrPort) tool.Program {
		// The client's own address is the local end, as Owner expects; a
		// connection nobody can name is simply not attributed.
		p, err := owner(client, proxyEnd)
		if err != nil {
			return tool.Program{}
		}
		return tool.Program{PID: p.PID, Name: p.Name, Path: p.Path}
	}
}
