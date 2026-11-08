package traffic

import (
	"bufio"
	"context"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"net/url"
	"strconv"
	"strings"
	"time"

	xproxy "golang.org/x/net/proxy"

	"github.com/cxxzsh/oh-my-toys/internal/platform/sysproxy"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// upstreamTimeout is how long opening a connection through an upstream
// proxy may take.
const upstreamTimeout = 30 * time.Second

// The schemes an upstream proxy may have. socks5h resolves the name on the
// proxy side, which this capture does either way.
const (
	schemeHTTPProxy  = "http"
	schemeSOCKS5     = "socks5"
	schemeSOCKS5Host = "socks5h"
	// upstreamHTTP starts the address of the proxy the system had.
	upstreamHTTP = "http://"
)

// The names and addresses a proxy of this machine answers to, besides the
// loopback constant: a proxy there is this very capture.
const (
	hostLocalhost    = "localhost"
	hostIPv6Loopback = "::1"
	hostIPv4Any      = "0.0.0.0"
	hostIPv6Any      = "::"
)

// The modes a GNOME proxy reports.
const (
	gnomeManual = "manual"
	gnomeAuto   = "auto"
)

// defaultUpstreamPort is the port an upstream proxy without one uses.
const defaultUpstreamPort = "80"

// dialFunc opens a connection for the proxy, through an upstream proxy
// when the capture has one.
type dialFunc = func(ctx context.Context, network, addr string) (net.Conn, error)

// upstream is where a capture sends its traffic: nothing (directly), a
// SOCKS5 proxy, or an HTTP proxy that takes both plain requests and TLS
// tunnels.
type upstream struct {
	// proxy is the HTTP proxy plain requests go through in absolute form.
	proxy *url.URL
	dial  dialFunc
}

// upstreamFor builds the upstream of a config's address, which the config
// has already validated.
func upstreamFor(text string) (upstream, error) {
	if text == "" {
		return upstream{}, nil
	}
	u, err := parseUpstream(text)
	if err != nil {
		return upstream{}, err
	}
	switch strings.ToLower(u.Scheme) {
	case schemeSOCKS5, schemeSOCKS5Host:
		dial, err := socksDialer(u)
		return upstream{dial: dial}, err
	default:
		return upstream{proxy: u, dial: httpDialer(u)}, nil
	}
}

// parseUpstream reads "http://host:port", "socks5://host:port" or
// "socks5h://host:port", the only things Config.Upstream may hold.
// Credentials in the URL are the upstream proxy's, and are used when it
// asks for them; they are never quoted back in an error.
func parseUpstream(text string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(text))
	if err != nil {
		return nil, fmt.Errorf("%w: the upstream is no URL", ErrBadConfig)
	}
	switch strings.ToLower(u.Scheme) {
	case schemeHTTPProxy, schemeSOCKS5, schemeSOCKS5Host:
	default:
		return nil, fmt.Errorf("%w: upstream %q must start with http://, socks5:// or socks5h://", ErrBadConfig, u.Redacted())
	}
	host, port, err := net.SplitHostPort(u.Host)
	if err != nil || host == "" {
		return nil, fmt.Errorf("%w: upstream %q has no host:port", ErrBadConfig, u.Redacted())
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > portMax {
		return nil, fmt.Errorf("%w: upstream %q has no usable port", ErrBadConfig, u.Redacted())
	}
	return u, nil
}

// chooseUpstream picks where the capture sends its traffic. A person's
// choice wins; otherwise, when the system proxy is about to be pointed at
// the capture, the proxy the system had before it is used, because the
// internet of that person only works through it. The capture's own address
// is never an upstream, and a system proxy that only has a PAC file cannot
// be followed: the person has to name the proxy.
func chooseUpstream(cfg tool.CaptureConfig, before *sysproxy.Snapshot) (string, error) {
	if cfg.Upstream != "" {
		return cfg.Upstream, nil
	}
	if before == nil {
		return "", nil
	}
	if addr := snapshotProxy(*before); addr != "" {
		if isOurAddress(addr, cfg.ProxyPort) {
			return "", nil
		}
		return upstreamHTTP + addr, nil
	}
	if pacOnly(*before) {
		return "", fmt.Errorf("%w: the system proxy only has an automatic configuration file, which a capture cannot follow: set an upstream proxy", ErrBadConfig)
	}
	return "", nil
}

// pacOnly reports whether the snapshot's proxies are automatic
// configuration files, which a capture cannot follow. The caller has
// already found that it names no proxy that could be used.
func pacOnly(s sysproxy.Snapshot) bool {
	if s.Windows != nil && s.Windows.AutoConfigURL.Present && strings.TrimSpace(s.Windows.AutoConfigURL.Text) != "" {
		return true
	}
	if s.GNOME != nil && s.GNOME.Mode == gnomeAuto {
		return true
	}
	for _, service := range s.Services {
		if service.Auto.Enabled && service.Auto.URL != "" {
			return true
		}
	}
	return false
}

// isOurAddress reports whether addr is the proxy of this very capture.
func isOurAddress(addr string, port int) bool {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	n, err := strconv.Atoi(p)
	if err != nil || n != port {
		return false
	}
	switch strings.ToLower(host) {
	case hostLocalhost, loopback, hostIPv6Loopback, hostIPv4Any, hostIPv6Any:
		return true
	}
	return false
}

// snapshotProxy is the proxy a saved system snapshot points at, as
// "host:port", or "" when it had none. HTTPS is preferred: an HTTP proxy
// of the system serves both schemes.
func snapshotProxy(s sysproxy.Snapshot) string {
	if addr := windowsProxy(s.Windows); addr != "" {
		return addr
	}
	if addr := gnomeProxy(s.GNOME); addr != "" {
		return addr
	}
	var httpAddr string
	for _, service := range s.Services {
		if service.HTTPS.Enabled {
			if addr := endpoint(service.HTTPS.Host, service.HTTPS.Port); addr != "" {
				return addr
			}
		}
		if httpAddr == "" && service.HTTP.Enabled {
			httpAddr = endpoint(service.HTTP.Host, service.HTTP.Port)
		}
	}
	return httpAddr
}

// endpoint is a host and port as "host:port", or "" when the proxy was
// never configured.
func endpoint(host string, port int) string {
	if host == "" || port == 0 {
		return ""
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// windowsProxy reads the WinINET proxy of a Windows snapshot, which is
// either "host:port" for both schemes or "http=host:port;https=host:port".
func windowsProxy(w *sysproxy.WindowsSettings) string {
	if w == nil || !w.ProxyEnable.Present || w.ProxyEnable.Number != 1 || !w.ProxyServer.Present {
		return ""
	}
	var plain, httpAddr, httpsAddr string
	for _, part := range strings.Split(w.ProxyServer.Text, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		scheme, addr, named := strings.Cut(part, "=")
		if !named {
			if plain == "" {
				plain = part
			}
			continue
		}
		switch strings.ToLower(strings.TrimSpace(scheme)) {
		case "https":
			httpsAddr = strings.TrimSpace(addr)
		case schemeHTTPProxy:
			httpAddr = strings.TrimSpace(addr)
		}
	}
	for _, addr := range []string{httpsAddr, httpAddr, plain} {
		if addr != "" {
			return addr
		}
	}
	return ""
}

// gnomeProxy reads the manual proxy of a GNOME snapshot.
func gnomeProxy(g *sysproxy.GNOMESettings) string {
	if g == nil || g.Mode != gnomeManual {
		return ""
	}
	for _, p := range []sysproxy.GNOMEProxy{g.HTTPS, g.HTTP} {
		if addr := endpoint(p.Host, p.Port); addr != "" {
			return addr
		}
	}
	return ""
}

// httpDialer opens connections through an HTTP proxy. A connection to the
// proxy itself is raw, because that is how the proxy sends plain requests,
// in absolute form; a connection to anything else is a CONNECT tunnel, so
// that TLS keeps end to end through it.
func httpDialer(u *url.URL) dialFunc {
	endpoint := proxyEndpoint(u)
	auth := ""
	if u.User != nil {
		password, _ := u.User.Password()
		pair := u.User.Username() + ":" + password
		auth = "Basic " + base64.StdEncoding.EncodeToString([]byte(pair))
	}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{Timeout: upstreamTimeout}
		conn, err := dialer.DialContext(ctx, network, endpoint)
		if err != nil {
			return nil, fmt.Errorf("connect to the upstream proxy %s: %w", endpoint, err)
		}
		if addr == endpoint {
			return conn, nil
		}
		buffered, err := connectThrough(ctx, conn, addr, auth)
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return &bufferedConn{Conn: conn, r: buffered}, nil
	}
}

// proxyEndpoint is the address an HTTP proxy listens on, with the port it
// leaves out filled in.
func proxyEndpoint(u *url.URL) string {
	if u.Port() != "" {
		return u.Host
	}
	return net.JoinHostPort(u.Hostname(), defaultUpstreamPort)
}

// connectThrough asks an HTTP proxy for a tunnel to target and reads its
// answer, returning the reader that holds the start of the tunnel.
func connectThrough(ctx context.Context, conn net.Conn, target, auth string) (*bufio.Reader, error) {
	if err := conn.SetDeadline(deadlineOf(ctx)); err != nil {
		return nil, fmt.Errorf("set the deadline of the upstream proxy connection: %w", err)
	}
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: make(http.Header),
	}
	if auth != "" {
		req.Header.Set("Proxy-Authorization", auth)
	}
	if err := req.Write(conn); err != nil {
		return nil, fmt.Errorf("ask the upstream proxy for %s: %w", target, err)
	}
	br := bufio.NewReader(conn)
	tp := textproto.NewReader(br)
	line, err := tp.ReadLine()
	if err != nil {
		return nil, fmt.Errorf("read the answer of the upstream proxy for %s: %w", target, err)
	}
	code, err := statusCode(line)
	if err != nil {
		return nil, err
	}
	if _, err := tp.ReadMIMEHeader(); err != nil {
		return nil, fmt.Errorf("read the headers of the upstream proxy for %s: %w", target, err)
	}
	if code < 200 || code > 299 {
		return nil, fmt.Errorf("the upstream proxy refused %s: %s", target, line)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, fmt.Errorf("clear the deadline of the upstream proxy connection: %w", err)
	}
	return br, nil
}

// statusCode reads the code of an HTTP status line.
func statusCode(line string) (int, error) {
	fields := strings.SplitN(line, " ", 3)
	if len(fields) < 2 {
		return 0, fmt.Errorf("the upstream proxy answered %q, which is no HTTP status line", line)
	}
	code, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, fmt.Errorf("the upstream proxy answered %q, which is no HTTP status line", line)
	}
	return code, nil
}

// deadlineOf is when a step of the dialing has to be done.
func deadlineOf(ctx context.Context) time.Time {
	if d, ok := ctx.Deadline(); ok {
		return d
	}
	return time.Now().Add(upstreamTimeout)
}

// socksDialer opens connections through a SOCKS5 proxy.
func socksDialer(u *url.URL) (dialFunc, error) {
	var auth *xproxy.Auth
	if u.User != nil {
		password, _ := u.User.Password()
		auth = &xproxy.Auth{User: u.User.Username(), Password: password}
	}
	dialer, err := xproxy.SOCKS5("tcp", u.Host, auth, &net.Dialer{Timeout: upstreamTimeout})
	if err != nil {
		return nil, fmt.Errorf("upstream proxy %s: %w", u.Host, err)
	}
	contexted, ok := dialer.(xproxy.ContextDialer)
	if !ok {
		return nil, fmt.Errorf("upstream proxy %s cannot be dialled with a context", u.Host)
	}
	return contexted.DialContext, nil
}

// bufferedConn is a connection whose reader may already hold bytes the
// upstream proxy sent past its answer: the start of the tunnel.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) { return c.r.Read(p) }
