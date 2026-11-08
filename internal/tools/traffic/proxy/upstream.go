package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/http2"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// headerBuffer is how much net/http may read at once while it parses a
// message head.
const headerBuffer = 8 << 10

// sent is a response as it came off the wire.
type sent struct {
	resp       *http.Response
	body       io.ReadCloser
	addr       string
	tlsVersion string
	headers    []tool.Header
	conn       net.Conn
	br         *bufio.Reader
	cleanup    func()
	// stop releases the context hook that closes the connection when the
	// request is abandoned.
	stop func() bool
}

// close releases the response body and the connection it came from.
func (u *sent) close() {
	if u.stop != nil {
		u.stop()
	}
	if u.body != nil {
		_ = u.body.Close()
	}
	if u.cleanup != nil {
		u.cleanup()
	}
	if u.conn != nil {
		_ = u.conn.Close()
	}
}

// closingBody closes a response body and then the connection it came from.
type closingBody struct {
	io.ReadCloser
	closer func()
}

// Close closes the body and the connection.
func (b *closingBody) Close() error {
	err := b.ReadCloser.Close()
	b.closer()
	return err
}

// send forwards one request to its server over a fresh connection. A TLS
// server is verified against the configured roots, or the system roots: the
// proxy never weakens that check. Only the response is returned; the caller
// closes it.
func (s *Server) send(ctx context.Context, req *http.Request) (*sent, error) {
	addr, host, useTLS, err := target(req.URL)
	if err != nil {
		return nil, err
	}
	upgrade := isUpgrade(req)
	out := req.Clone(ctx)
	out.RequestURI = ""
	stripHopByHop(out.Header)
	if upgrade {
		// The switch is end-to-end even though its headers are hop-by-hop,
		// so they go back after the strip. Proxy credentials do not.
		out.Header.Set("Connection", "Upgrade")
		if value := req.Header.Get("Upgrade"); value != "" {
			out.Header.Set("Upgrade", value)
		}
	}
	if s.opts.UpstreamProxy != nil && !useTLS {
		return s.sendThroughProxy(ctx, out)
	}
	raw, err := s.dialUpstream(ctx, addr)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	stream, tlsConn, err := s.wrapTLS(ctx, raw, host, useTLS, upgrade)
	if err != nil {
		stop()
		_ = raw.Close()
		return nil, err
	}
	var res *sent
	if tlsConn != nil && tlsConn.ConnectionState().NegotiatedProtocol == http2.NextProtoTLS {
		res, err = sendH2(out, tlsConn, raw, addr, s.responseHeadTimeout)
	} else {
		res, err = sendH1(out, stream, raw, addr, tlsVersionName(tlsConn), out.Write, s.responseHeadTimeout)
	}
	if err != nil {
		stop()
		return nil, err
	}
	res.stop = stop
	return res, nil
}

// sendThroughProxy gives a plain HTTP request to the upstream proxy in
// absolute form, with the credentials of its URL. TLS tunnels are not its
// business: those keep the dialer the tool supplies.
func (s *Server) sendThroughProxy(ctx context.Context, out *http.Request) (*sent, error) {
	proxyURL := s.opts.UpstreamProxy
	addr := proxyURL.Host
	if proxyURL.Port() == "" {
		addr = net.JoinHostPort(proxyURL.Hostname(), "80")
	}
	raw, err := s.dialDirect(ctx, addr)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { _ = raw.Close() })
	if user := proxyURL.User; user != nil {
		password, _ := user.Password()
		token := base64.StdEncoding.EncodeToString([]byte(user.Username() + ":" + password))
		out.Header.Set("Proxy-Authorization", "Basic "+token)
	}
	res, err := sendH1(out, raw, raw, addr, "", out.WriteProxy, s.responseHeadTimeout)
	if err != nil {
		stop()
		return nil, err
	}
	res.stop = stop
	return res, nil
}

// wrapTLS puts a fresh TLS session on the connection. An upgrade offers only
// HTTP/1.1, because an HTTP/2 transport cannot carry a protocol switch.
func (s *Server) wrapTLS(ctx context.Context, conn net.Conn, host string, useTLS, upgrade bool) (net.Conn, *tls.Conn, error) {
	if !useTLS {
		return conn, nil, nil
	}
	protos := []string{http2.NextProtoTLS, alpnHTTP1}
	if upgrade {
		protos = []string{alpnHTTP1}
	}
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName: host,
		NextProtos: protos,
		RootCAs:    s.opts.RootCAs,
		MinVersion: tls.VersionTLS12,
	})
	hctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	if err := tlsConn.HandshakeContext(hctx); err != nil {
		return nil, nil, fmt.Errorf("tls handshake with %s: %w", conn.RemoteAddr(), err)
	}
	return tlsConn, tlsConn, nil
}

// sendH1 writes the request and reads the response over HTTP/1.1. The raw
// head is kept so the flow records the headers in the order they arrived,
// and interim 1xx answers are skipped so the real one is what the client
// gets. Everything after the head, including an upgraded stream, stays on
// stream: for a TLS server that is the TLS connection, never the raw socket.
func sendH1(out *http.Request, stream, raw net.Conn, addr, version string, write func(io.Writer) error, headTimeout time.Duration) (*sent, error) {
	tee := &teeConn{Conn: stream}
	limiter := &headLimiter{r: tee, tee: tee}
	br := bufio.NewReaderSize(limiter, headerBuffer)
	if err := write(tee); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("send request to %s: %w", addr, err)
	}
	limiter.begin()
	mark := tee.mark()
	if err := stream.SetReadDeadline(time.Now().Add(headTimeout)); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("watch the answer of %s: %w", addr, err)
	}
	resp, headers, err := readResponse(br, limiter, tee, &mark, out) //nolint:bodyclose // the body is returned in sent, and its caller closes it.
	if err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("read response from %s: %w", addr, err)
	}
	_ = stream.SetReadDeadline(time.Time{})
	body := &closingBody{ReadCloser: resp.Body, closer: func() { _ = raw.Close() }}
	resp.Body = body
	return &sent{
		resp:       resp,
		body:       body,
		addr:       raw.RemoteAddr().String(),
		tlsVersion: version,
		headers:    headers,
		conn:       stream,
		br:         br,
		cleanup:    func() { _ = raw.Close() },
	}, nil
}

// readResponse reads the final response, skipping the interim 1xx answers a
// server sends before it, and returns the head that belongs to it. An
// endless run of interim answers ends the exchange.
func readResponse(br *bufio.Reader, limiter *headLimiter, tee *teeConn, mark *int64, out *http.Request) (*http.Response, []tool.Header, error) {
	interim := 0
	for {
		limiter.next()
		resp, err := http.ReadResponse(br, out)
		if err != nil {
			limiter.lift()
			return nil, nil, err
		}
		head := tee.headFrom(*mark)
		if head != nil {
			*mark += int64(len(head))
		} else {
			*mark = tee.mark()
		}
		if !interimStatus(resp.StatusCode) {
			limiter.lift()
			return resp, parseHead(head), nil
		}
		_ = resp.Body.Close()
		interim++
		if interim > maxInterimResponses {
			limiter.lift()
			return nil, nil, errTooManyInterim
		}
	}
}

// interimStatus reports whether a status is a 1xx answer that comes before
// the real response.
func interimStatus(status int) bool {
	return status >= 100 && status < http.StatusOK && status != http.StatusSwitchingProtocols
}

// sendH2 sends the request over an HTTP/2 connection the TLS handshake
// negotiated.
func sendH2(out *http.Request, conn *tls.Conn, raw net.Conn, addr string, headTimeout time.Duration) (*sent, error) {
	if err := conn.SetReadDeadline(time.Now().Add(headTimeout)); err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("watch the answer of %s: %w", addr, err)
	}
	// The design fixes x/net/http2 for an upstream that negotiated h2 by ALPN.
	tr := &http2.Transport{ //nolint:staticcheck // x/net/http2 is what the design names.
		DialTLSContext: func(context.Context, string, string, *tls.Config) (net.Conn, error) { //nolint:staticcheck // x/net/http2 is what the design names.
			return conn, nil
		},
	}
	resp, err := tr.RoundTrip(out) //nolint:bodyclose,staticcheck // the caller closes the body; x/net/http2 is what the design names.
	if err != nil {
		_ = raw.Close()
		return nil, fmt.Errorf("send request to %s: %w", addr, err)
	}
	_ = conn.SetReadDeadline(time.Time{})
	closer := func() {
		tr.CloseIdleConnections() //nolint:staticcheck // x/net/http2 is what the design names.
	}
	body := &closingBody{ReadCloser: resp.Body, closer: closer}
	resp.Body = body
	return &sent{
		resp:       resp,
		body:       body,
		addr:       raw.RemoteAddr().String(),
		tlsVersion: tlsVersionName(conn),
		headers:    headerList(resp.Header),
		cleanup: func() {
			tr.CloseIdleConnections() //nolint:staticcheck // x/net/http2 is what the design names.
			_ = raw.Close()
		},
	}, nil
}

// dialDirect reaches the upstream proxy itself. The configured dialer is for
// tunnels to a target, so asking it to reach the proxy would loop.
func (s *Server) dialDirect(ctx context.Context, addr string) (net.Conn, error) {
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	var dialer net.Dialer
	conn, err := dialer.DialContext(dctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	tracked, ok := s.track(conn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("proxy is stopping")
	}
	return tracked, nil
}

// dialUpstream opens a connection the server owns, so Stop closes it.
func (s *Server) dialUpstream(ctx context.Context, addr string) (net.Conn, error) {
	dctx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	conn, err := s.dial(dctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", addr, err)
	}
	tracked, ok := s.track(conn)
	if !ok {
		_ = conn.Close()
		return nil, errors.New("proxy is stopping")
	}
	return tracked, nil
}

// target is where a URL points, and whether it is reached over TLS.
func target(u *url.URL) (addr, host string, secure bool, err error) {
	if u == nil {
		return "", "", false, errors.New("request has no URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != schemeHTTP && scheme != schemeHTTPS {
		return "", "", false, fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	host = u.Hostname()
	if host == "" {
		return "", "", false, errors.New("request URL has no host")
	}
	port := u.Port()
	if port == "" {
		port = defaultPort(scheme)
	}
	return net.JoinHostPort(host, port), host, scheme == schemeHTTPS, nil
}

// tlsVersionName is the name the inspector shows for a TLS version.
func tlsVersionName(conn *tls.Conn) string {
	if conn == nil {
		return ""
	}
	switch version := conn.ConnectionState().Version; version {
	case tls.VersionTLS13:
		return "TLS 1.3"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS10:
		return "TLS 1.0"
	default:
		return fmt.Sprintf("TLS %#x", version)
	}
}
