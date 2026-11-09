// Package proxy is the local HTTP(S) proxy of the traffic tool. It listens
// on a loopback address, forwards plain HTTP, opens HTTPS with the app's
// root certificate when asked to and records one flow per exchange. The
// code is run-tested on macOS; the Windows and Linux builds compile, but
// they have not been run on those systems yet.
package proxy

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sync"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/ca"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/rules"
)

// errProxyLoop is what the proxy reports when a request would come back to
// its own listener.
var errProxyLoop = errors.New("request targets the proxy itself")

// errTooManyInterim is what an exchange reports when a server keeps sending
// 1xx answers instead of a real one.
var errTooManyInterim = errors.New("too many interim responses")

// maxInterimResponses bounds the 1xx answers the proxy waits through before
// it decides the server is not going to answer.
const maxInterimResponses = 8

// Recorder is where the proxy writes flows; flowlog.Log implements it.
type Recorder interface{ Put(tool.Flow) uint64 }

// Ruler is what the proxy asks about each exchange; rules.Engine implements
// it.
type Ruler interface {
	Request(*http.Request) (rules.Verdict, error)
	Response(*http.Request, *http.Response) ([]string, error)
}

// Options is what a server is made with.
type Options struct {
	Addr      string        // "127.0.0.1:9099"; only a loopback address is allowed
	Authority *ca.Authority // nil: HTTPS is never decrypted
	Decrypt   bool
	Bypass    []string                                                          // glob patterns of hosts never decrypted
	Log       Recorder                                                          // nil: nothing is recorded
	Rules     Ruler                                                             // nil: no rules apply
	Dial      func(ctx context.Context, network, addr string) (net.Conn, error) // nil: net.Dialer
	// RootCAs verifies the TLS servers the proxy talks to; nil means the
	// system roots. The proxy never weakens that check.
	RootCAs *x509.CertPool
	// UpstreamProxy, when set, is the HTTP proxy that plain HTTP requests
	// go through (scheme http; credentials in its userinfo). TLS tunnels
	// still dial the target through Dial.
	UpstreamProxy *url.URL
	// Program names the process that owns a client connection. It is asked
	// once per connection and may be slow; see programWait. Nil: programs
	// are unknown.
	Program func(client netip.AddrPort) tool.Program
	// Scope limits recording and decryption to the named programs
	// (case-insensitive). Empty: everything is recorded. When set, a
	// connection whose program is unknown is not recorded either.
	Scope []string
}

// Server is the proxy. It is safe to use from any goroutine.
type Server struct {
	opts   Options
	bypass []string
	dial   func(ctx context.Context, network, addr string) (net.Conn, error)
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// responseHeadTimeout and handshakeTimeout are set once, before Start,
	// and are fields so tests can shorten them.
	responseHeadTimeout time.Duration
	handshakeTimeout    time.Duration

	mu       sync.Mutex
	ln       net.Listener
	addr     string
	stopping bool
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup

	// scope is the normalized Options.Scope; self is the program this
	// process runs as, which owns the flows the proxy sends itself.
	scope []string
	self  tool.Program

	stopOnce sync.Once
}

// How long a connection to a server may take to come up.
const dialTimeout = 30 * time.Second

// headReadTimeout bounds how long a client may take to send a message head,
// including while an idle keep-alive connection waits for its next request.
// A body, once its head has arrived, gets no deadline.
const headReadTimeout = 30 * time.Second

// lookupTimeout bounds the name lookup the loop check does.
const lookupTimeout = 2 * time.Second

// responseHeadTimeoutDefault bounds how long a server may take to send the
// head of its answer, so a server that never answers cannot hold a request
// and two file descriptors forever. It is generous because a slow page may
// take a while to start.
const responseHeadTimeoutDefault = 120 * time.Second

// handshakeTimeoutDefault bounds the TLS handshake a decrypted client does
// after CONNECT, so a local client cannot stall the proxy for free.
const handshakeTimeoutDefault = 30 * time.Second

// The protocol names a flow shows, and the URL schemes the proxy serves.
const (
	protoHTTP11 = "HTTP/1.1"
	protoH2     = "h2"
	protoTunnel = "tunnel"
	schemeHTTP  = "http"
	schemeHTTPS = "https"
	// alpnHTTP1 is the ALPN name of HTTP/1.1.
	alpnHTTP1 = "http/1.1"
)

// New validates the options and makes a server that has not started yet.
func New(o Options) (*Server, error) {
	host, port, err := net.SplitHostPort(o.Addr)
	if err != nil {
		return nil, fmt.Errorf("proxy address %q: %w", o.Addr, err)
	}
	if err := checkLoopback(host); err != nil {
		return nil, err
	}
	if o.UpstreamProxy != nil {
		if err := checkUpstreamProxy(o.UpstreamProxy, host, port); err != nil {
			return nil, err
		}
	}
	if o.Dial == nil {
		dialer := &net.Dialer{Timeout: dialTimeout}
		o.Dial = dialer.DialContext
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		opts:                o,
		bypass:              append([]string(nil), o.Bypass...),
		dial:                o.Dial,
		ctx:                 ctx,
		cancel:              cancel,
		done:                make(chan struct{}),
		conns:               make(map[net.Conn]struct{}),
		responseHeadTimeout: responseHeadTimeoutDefault,
		handshakeTimeout:    handshakeTimeoutDefault,
		scope:               normalizeScope(o.Scope),
		self:                selfProgram(),
	}, nil
}

// Start listens on the configured address and serves in the background
// until ctx ends or Stop is called. A port that another program holds comes
// back as an error wrapping tool.ErrPortInUse.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.ln != nil {
		s.mu.Unlock()
		return errors.New("proxy is already started")
	}
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", s.opts.Addr)
	if err != nil {
		s.mu.Unlock()
		if isAddrInUse(err) {
			return fmt.Errorf("%w %s: %w", tool.ErrPortInUse, s.opts.Addr, err)
		}
		return fmt.Errorf("listen on %s: %w", s.opts.Addr, err)
	}
	s.ln = ln
	s.addr = ln.Addr().String()
	s.mu.Unlock()

	s.wg.Add(2)
	go s.accept()
	go s.watch(ctx)
	return nil
}

// Stop closes the listener and every live connection, then waits for the
// goroutines the server started.
func (s *Server) Stop() error {
	s.shutdown()
	s.wg.Wait()
	return nil
}

// Addr is the address the server listens on, once it has started.
func (s *Server) Addr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.addr
}

// accept takes connections until the listener is closed.
func (s *Server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		tracked, ok := s.track(conn)
		if !ok {
			_ = conn.Close()
			return
		}
		go s.handle(s.ctx, tracked)
	}
}

// watch ends the server when the context it was started with does.
func (s *Server) watch(ctx context.Context) {
	defer s.wg.Done()
	select {
	case <-ctx.Done():
		s.shutdown()
	case <-s.done:
	}
}

// shutdown closes the listener and every connection exactly once.
func (s *Server) shutdown() {
	s.stopOnce.Do(func() {
		close(s.done)
		s.cancel()
		s.mu.Lock()
		s.stopping = true
		ln := s.ln
		conns := make([]net.Conn, 0, len(s.conns))
		for conn := range s.conns {
			conns = append(conns, conn)
		}
		s.mu.Unlock()
		if ln != nil {
			_ = ln.Close()
		}
		for _, conn := range conns {
			_ = conn.Close()
		}
	})
}

// trackedConn is a connection the server owns: closing it unregisters it
// once, so Stop can wait for every handler to finish.
type trackedConn struct {
	net.Conn
	srv  *Server
	once sync.Once
}

// Close closes the connection and lets Stop stop waiting for it.
func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.srv.untrack(c) })
	return err
}

// track takes ownership of a connection, or reports that the server is
// already stopping.
func (s *Server) track(conn net.Conn) (*trackedConn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stopping {
		return nil, false
	}
	tracked := &trackedConn{Conn: conn, srv: s}
	s.conns[tracked] = struct{}{}
	s.wg.Add(1)
	return tracked, true
}

// untrack forgets a closed connection.
func (s *Server) untrack(conn net.Conn) {
	s.mu.Lock()
	delete(s.conns, conn)
	s.mu.Unlock()
	s.wg.Done()
}

// handle serves one client connection until it closes. A panic in this
// goroutine is recorded as a failed flow and closes only this connection,
// instead of taking the app down. Closing the connection unregisters it,
// which is also what Stop waits for.
func (s *Server) handle(ctx context.Context, conn net.Conn) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.put(tool.Flow{FlowRow: tool.FlowRow{
				Source:  tool.SourceProxy,
				Started: time.Now(),
				Proto:   protoHTTP11,
				Process: programOf(ctx),
				State:   tool.FlowFailed,
				Info:    fmt.Sprintf("proxy error: %v", recovered),
			}})
		}
		_ = conn.Close()
	}()
	ctx = withProgram(ctx, s.resolve(conn.RemoteAddr()))
	s.serveConn(ctx, conn)
}
