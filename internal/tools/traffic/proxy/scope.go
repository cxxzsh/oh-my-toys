package proxy

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// programWait bounds how long a connection waits for the answer about the
// program that owns it. Looking a connection up costs a system call; a slow
// answer must not hold the client, so an unanswered lookup counts as an
// unknown program.
const programWait = 200 * time.Millisecond

// programKey is the context key under which a connection carries its
// program.
type programKey struct{}

// withProgram returns ctx carrying the program of the client connection.
func withProgram(ctx context.Context, p tool.Program) context.Context {
	return context.WithValue(ctx, programKey{}, p)
}

// programOf is the program of the client connection ctx belongs to, or the
// zero Program when none is known.
func programOf(ctx context.Context) tool.Program {
	p, _ := ctx.Value(programKey{}).(tool.Program)
	return p
}

// resolve asks the hook which program owns the client end of a connection.
// The hook runs in its own goroutine so that a stuck lookup cannot hold the
// connection; the answer channel is buffered so that goroutine ends as soon
// as the hook returns, even after the wait has given up.
func (s *Server) resolve(remote net.Addr) tool.Program {
	if s.opts.Program == nil || remote == nil {
		return tool.Program{}
	}
	client, err := netip.ParseAddrPort(remote.String())
	if err != nil {
		return tool.Program{}
	}
	answer := make(chan tool.Program, 1)
	go func() { answer <- s.opts.Program(client) }()
	timer := time.NewTimer(programWait)
	defer timer.Stop()
	select {
	case p := <-answer:
		return p
	case <-timer.C:
		return tool.Program{}
	}
}

// selfProgram names this process. The flows the proxy sends itself, such as
// a replay, belong to it.
func selfProgram() tool.Program {
	pid := os.Getpid()
	path, err := os.Executable()
	if err != nil {
		return tool.Program{PID: pid}
	}
	return tool.Program{PID: pid, Name: filepath.Base(path), Path: path}
}

// normalizeScope lower-cases and trims the program names and drops the
// empty ones, so matching is case-insensitive.
func normalizeScope(names []string) []string {
	out := make([]string, 0, len(names))
	for _, name := range names {
		if name = strings.ToLower(strings.TrimSpace(name)); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// inScope reports whether the traffic of a program is recorded. With no
// scope everything is. With one, only the listed programs are, and a
// connection whose program is unknown is not, since nothing can match it.
func (s *Server) inScope(p tool.Program) bool {
	if len(s.scope) == 0 {
		return true
	}
	name := strings.ToLower(p.Name)
	if name == "" {
		return false
	}
	for _, want := range s.scope {
		if want == name {
			return true
		}
	}
	return false
}

// passThrough forwards one plain request of a program outside the scope. It
// is neither recorded nor decrypted, and the rules do not apply to it,
// because the user did not ask to see or change that traffic.
func (s *Server) passThrough(ctx context.Context, req *http.Request, rp *reply) error {
	if s.isSelf(ctx, req.URL) {
		_ = writeError(rp, http.StatusLoopDetected, errProxyLoop)
		return errProxyLoop
	}
	up, err := s.send(ctx, req)
	if err != nil {
		drain(req)
		_ = writeError(rp, http.StatusBadGateway, err)
		return err
	}
	defer up.close()
	if up.resp.StatusCode == http.StatusSwitchingProtocols && up.conn != nil && rp.conn != nil {
		if err := writeUpgrade(rp.conn, up.resp, up.headers); err != nil {
			return err
		}
		rp.upgraded = true
		pipe(rp.conn, rp.br, up.conn, up.br)
		return nil
	}
	normalize(up.resp)
	if err := rp.write(up.resp); err != nil {
		return err
	}
	// A request body that the server may not have read to the end leaves
	// the connection in an unknown state, so the client must reconnect.
	if up.resp.Close || (up.resp.ContentLength < 0 && len(up.resp.TransferEncoding) == 0) ||
		(req.Body != nil && req.Body != http.NoBody) {
		rp.closeAfter = true
	}
	return nil
}

// passTunnel carries the bytes of a CONNECT for a program outside the scope,
// without decrypting or recording them.
func (s *Server) passTunnel(ctx context.Context, conn net.Conn, br *bufio.Reader, authority string) {
	upstream, err := s.dialUpstream(ctx, authority)
	if err != nil {
		return
	}
	pipe(conn, br, upstream, nil)
}
