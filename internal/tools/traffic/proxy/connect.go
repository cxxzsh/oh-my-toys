package proxy

import (
	"bufio"
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"time"

	"golang.org/x/net/http2"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// connect opens an HTTPS tunnel, decrypted when the options and the rules
// allow it, and blind otherwise.
func (s *Server) connect(ctx context.Context, conn net.Conn, br *bufio.Reader, req *http.Request) {
	authority := req.Host
	if authority == "" && req.URL != nil {
		authority = req.URL.Host
	}
	host, port, err := net.SplitHostPort(authority)
	if err != nil {
		host, port = authority, "443"
		authority = net.JoinHostPort(host, port)
	}
	if s.isSelfAddr(ctx, host, port) {
		_, _ = io.WriteString(conn, "HTTP/1.1 508 Loop Detected\r\nContent-Length: 0\r\n\r\n")
		return
	}
	if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	started := time.Now()
	if !s.inScope(programOf(ctx)) {
		s.passTunnel(ctx, conn, br, authority)
		return
	}
	if s.decryptable(host) {
		s.mitm(ctx, conn, br, authority, host, started)
		return
	}
	s.tunnel(ctx, conn, br, authority, started)
}

// tunnel copies bytes between the client and the server and records the
// connection as one flow.
func (s *Server) tunnel(ctx context.Context, conn net.Conn, br *bufio.Reader, authority string, started time.Time) {
	flow := tunnelFlow(authority, started, programOf(ctx))
	flow.ID = s.put(flow)
	upstream, err := s.dialUpstream(ctx, authority)
	if err != nil {
		s.finishFailed(flow, started, err)
		return
	}
	fromClient, fromUpstream := pipe(conn, br, upstream, nil)
	flow.Size = fromClient + fromUpstream
	flow.Duration = time.Since(started)
	flow.State = tool.FlowDone
	s.put(flow)
}

// tunnelFlow is the flow of a CONNECT that is not decrypted.
func tunnelFlow(authority string, started time.Time, program tool.Program) tool.Flow {
	return tool.Flow{FlowRow: tool.FlowRow{
		Source:  tool.SourceProxy,
		Started: started,
		Method:  http.MethodConnect,
		Host:    authority,
		Process: program,
		Proto:   protoTunnel,
		State:   tool.FlowOpen,
	}}
}

// mitm terminates TLS with a leaf certificate for host and serves the
// requests inside it.
func (s *Server) mitm(ctx context.Context, conn net.Conn, br *bufio.Reader, authority, host string, started time.Time) {
	cfg := &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			return s.opts.Authority.Leaf(host)
		},
		NextProtos: []string{http2.NextProtoTLS, alpnHTTP1},
		MinVersion: tls.VersionTLS12,
	}
	tlsConn := tls.Server(&bufferedConn{Conn: conn, r: br}, cfg)
	hctx, cancel := context.WithTimeout(ctx, s.handshakeTimeout)
	defer cancel()
	if err := tlsConn.SetReadDeadline(time.Now().Add(s.handshakeTimeout)); err != nil {
		return
	}
	err := tlsConn.HandshakeContext(hctx)
	_ = tlsConn.SetReadDeadline(time.Time{})
	if err != nil {
		s.connectFailed(authority, started, programOf(ctx), err)
		return
	}
	if tlsConn.ConnectionState().NegotiatedProtocol == http2.NextProtoTLS {
		s.serveH2(ctx, tlsConn, authority)
		return
	}
	s.serveTLS1(ctx, tlsConn, authority)
}

// connectFailed records a tunnel whose TLS handshake with the client broke,
// which in practice means the root certificate is not trusted.
func (s *Server) connectFailed(authority string, started time.Time, program tool.Program, err error) {
	flow := tunnelFlow(authority, started, program)
	flow.State = tool.FlowFailed
	flow.Duration = time.Since(started)
	flow.Info = "client did not trust the certificate: " + err.Error()
	s.put(flow)
}

// serveTLS1 reads the HTTP/1.1 requests a decrypted client sends.
func (s *Server) serveTLS1(ctx context.Context, conn *tls.Conn, authority string) {
	version := tlsVersionName(conn)
	tee := &teeConn{Conn: conn}
	limiter := &headLimiter{r: tee, tee: tee}
	br := bufio.NewReaderSize(limiter, headerBuffer)
	for {
		if !armHead(conn, limiter) {
			return
		}
		mark := tee.mark()
		req, err := http.ReadRequest(br)
		limiter.lift()
		_ = conn.SetReadDeadline(time.Time{})
		if err != nil {
			return
		}
		headers := parseHead(tee.headFrom(mark))
		if req.URL.Host == "" {
			req.URL.Scheme = schemeHTTPS
			req.URL.Host = firstNonEmpty(req.Host, authority)
		}
		m := meta{
			program:    programOf(ctx),
			proto:      protoHTTP11,
			decrypted:  true,
			tlsVersion: version,
			headers:    headers,
			tunnelHost: authorityHost(authority),
		}
		rp := &reply{
			write: writeHTTP1(conn), //nolint:bodyclose // writeHTTP1 closes the body it writes.
			abort: func() { _ = conn.Close() },
			conn:  conn,
			br:    br,
		}
		if _, err := s.serve(ctx, req, m, rp); err != nil || rp.upgraded || rp.closeAfter {
			return
		}
	}
}

// authorityHost is the host of a host:port authority.
func authorityHost(authority string) string {
	if host, _, err := net.SplitHostPort(authority); err == nil {
		return host
	}
	return authority
}

// serveH2 serves the requests a decrypted client sends over HTTP/2.
func (s *Server) serveH2(ctx context.Context, conn *tls.Conn, authority string) {
	version := tlsVersionName(conn)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Scheme == "" {
			req.URL.Scheme = schemeHTTPS
		}
		if req.URL.Host == "" {
			req.URL.Host = firstNonEmpty(req.Host, authority)
		}
		m := meta{
			program:    programOf(ctx),
			proto:      protoH2,
			decrypted:  true,
			tlsVersion: version,
			headers:    headerList(req.Header),
			tunnelHost: authorityHost(authority),
		}
		rp := &reply{write: h2Writer(w), abort: func() { _ = conn.Close() }} //nolint:bodyclose // h2Writer closes the body it copies.
		_, _ = s.serve(req.Context(), req, m, rp)                            //nolint:contextcheck // the stream's own context: http2 cancels it when the stream ends.
	})
	// The design fixes x/net/http2 for streams over the connection the proxy
	// terminated; the standard library has no other way to serve one conn.
	srv := &http2.Server{}                                                    //nolint:staticcheck // the design asks for x/net/http2 on this connection.
	srv.ServeConn(conn, &http2.ServeConnOpts{Handler: handler, Context: ctx}) //nolint:staticcheck // same reason as above.
}

// writeHTTP1 writes a response and closes its body. Hop-by-hop headers are
// dropped, as a proxy must: net/http writes its own connection headers from
// the response fields.
func writeHTTP1(conn net.Conn) func(*http.Response) error {
	return func(resp *http.Response) error {
		normalize(resp)
		// The client speaks HTTP/1.1, whatever the server used.
		resp.Proto, resp.ProtoMajor, resp.ProtoMinor = protoHTTP11, 1, 1
		stripHopByHop(resp.Header)
		err := resp.Write(conn)
		_ = resp.Body.Close()
		return err
	}
}

// h2Writer copies a response into an HTTP/2 response writer.
func h2Writer(w http.ResponseWriter) func(*http.Response) error {
	return func(resp *http.Response) error {
		stripHopByHop(resp.Header)
		headers := w.Header()
		for name, values := range resp.Header {
			for _, value := range values {
				headers.Add(name, value)
			}
		}
		w.WriteHeader(resp.StatusCode)
		_, err := io.Copy(w, resp.Body)
		_ = resp.Body.Close()
		return err
	}
}
