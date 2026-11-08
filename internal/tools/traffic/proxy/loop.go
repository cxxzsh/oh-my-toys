package proxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// checkUpstreamProxy validates the HTTP proxy that plain requests go
// through: it must be an http URL with a host, and must not be this proxy.
func checkUpstreamProxy(proxyURL *url.URL, host, port string) error {
	if !strings.EqualFold(proxyURL.Scheme, schemeHTTP) {
		return fmt.Errorf("upstream proxy scheme %q is not http", proxyURL.Scheme)
	}
	proxyHost := proxyURL.Hostname()
	if proxyHost == "" {
		return errors.New("upstream proxy has no host")
	}
	proxyPort := proxyURL.Port()
	if proxyPort == "" {
		proxyPort = "80"
	}
	if port != "0" && proxyPort == port && (sameHost(proxyHost, host) || reachesThisMachine(context.Background(), proxyHost)) {
		return fmt.Errorf("upstream proxy %s is the proxy itself", proxyURL.Host)
	}
	return nil
}

// checkLoopback refuses any address the proxy must not listen on.
func checkLoopback(host string) error {
	if host == "" {
		return fmt.Errorf("proxy address listens on every interface; only loopback is allowed")
	}
	if strings.EqualFold(host, "localhost") {
		return nil
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return fmt.Errorf("proxy address host %q is not an IP address", host)
	}
	if !addr.Unmap().IsLoopback() {
		return fmt.Errorf("proxy address %s is not loopback", host)
	}
	return nil
}

// decryptable reports whether a CONNECT to host is opened with the root
// certificate.
func (s *Server) decryptable(host string) bool {
	if !s.opts.Decrypt || s.opts.Authority == nil {
		return false
	}
	for _, pattern := range s.bypass {
		if matchHost(pattern, host) {
			return false
		}
	}
	return true
}

// matchHost matches a host against one glob pattern, ignoring case and a
// trailing dot. A pattern that is not a valid glob matches nothing.
func matchHost(pattern, host string) bool {
	ok, err := path.Match(normalizeHost(pattern), normalizeHost(host))
	return err == nil && ok
}

// normalizeHost is the form two host names are compared in.
func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

// sameHost compares two host names, ignoring case and a trailing dot.
func sameHost(a, b string) bool { return normalizeHost(a) == normalizeHost(b) }

// hostOfURL is the host of a URL without its port.
func hostOfURL(u *url.URL) string {
	if u == nil {
		return ""
	}
	return u.Hostname()
}

// isSelf reports whether the request would come back to this proxy, which
// would loop forever. The host is resolved, because a name that points at
// this machine loops just as surely as a loopback literal.
func (s *Server) isSelf(ctx context.Context, u *url.URL) bool {
	if u == nil || u.Host == "" {
		return false
	}
	port := u.Port()
	if port == "" {
		port = defaultPort(u.Scheme)
	}
	return s.isSelfAddr(ctx, u.Hostname(), port)
}

// isSelfAddr reports whether host:port is this proxy's listener.
func (s *Server) isSelfAddr(ctx context.Context, host, port string) bool {
	if host == "" || port == "" || port != s.port() {
		return false
	}
	return reachesThisMachine(ctx, host)
}

// reachesThisMachine reports whether host names this machine: localhost, a
// loopback or unspecified IP literal in any spelling, or a name that
// resolves to one of those.
func reachesThisMachine(ctx context.Context, host string) bool {
	host = normalizeHost(host)
	if host == "localhost" {
		return true
	}
	if addr, ok := parseHostAddr(host); ok {
		return localAddr(addr)
	}
	lctx, cancel := context.WithTimeout(ctx, lookupTimeout)
	defer cancel()
	addrs, err := net.DefaultResolver.LookupIPAddr(lctx, host)
	if err != nil {
		return false
	}
	for _, addr := range addrs {
		ip, ok := netip.AddrFromSlice(addr.IP)
		if ok && localAddr(ip.Unmap()) {
			return true
		}
	}
	return false
}

// localAddr reports whether an address reaches this machine from itself.
func localAddr(addr netip.Addr) bool { return addr.IsLoopback() || addr.IsUnspecified() }

// parseHostAddr reads an IP literal, including the legacy short forms such
// as 127.1 that name loopback without four parts.
func parseHostAddr(host string) (netip.Addr, bool) {
	if addr, err := netip.ParseAddr(host); err == nil {
		return addr.Unmap(), true
	}
	ip := parseLegacyIPv4(host)
	if ip == nil {
		return netip.Addr{}, false
	}
	addr, ok := netip.AddrFromSlice(ip)
	return addr.Unmap(), ok
}

// parseLegacyIPv4 reads the inet_aton forms: one to four numbers whose last
// part fills the remaining octets.
func parseLegacyIPv4(host string) net.IP {
	parts := strings.Split(host, ".")
	if len(parts) == 0 || len(parts) > 4 {
		return nil
	}
	values := make([]uint64, len(parts))
	for i, part := range parts {
		value, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil
		}
		values[i] = value
	}
	last := values[len(values)-1]
	limits := []uint64{0xffffffff, 0xffffff, 0xffff, 0xff}
	if last > limits[len(values)-1] {
		return nil
	}
	ip := last
	shift := uint(8 * (4 - (len(values) - 1)))
	for i := len(values) - 2; i >= 0; i-- {
		if values[i] > 0xff {
			return nil
		}
		ip |= values[i] << shift
		shift += 8
	}
	return net.IPv4(byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip)) //nolint:gosec // ip holds four octets.
}

// port is the port the server listens on.
func (s *Server) port() string {
	_, port, err := net.SplitHostPort(s.Addr())
	if err != nil {
		return ""
	}
	return port
}

// defaultPort is the port a scheme uses when the URL does not name one.
func defaultPort(scheme string) string {
	if strings.EqualFold(scheme, schemeHTTPS) {
		return "443"
	}
	return "80"
}
