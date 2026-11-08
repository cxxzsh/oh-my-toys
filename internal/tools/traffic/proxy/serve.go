package proxy

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/rules"
)

// serveConn reads requests from one client connection until it closes.
func (s *Server) serveConn(ctx context.Context, conn net.Conn) {
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
		if req.Method == http.MethodConnect {
			s.connect(ctx, conn, br, req)
			return
		}
		if !s.plain(ctx, conn, br, req, headers) {
			return
		}
	}
}

// armHead bounds the time and the size of the next message head a client
// sends, so a client that stalls or never ends its headers cannot hold the
// connection open. Bodies have no deadline.
func armHead(conn net.Conn, limiter *headLimiter) bool {
	if err := conn.SetReadDeadline(time.Now().Add(headReadTimeout)); err != nil {
		return false
	}
	limiter.begin()
	return true
}

// plain proxies one request that came to the proxy in absolute form.
func (s *Server) plain(ctx context.Context, conn net.Conn, br *bufio.Reader, req *http.Request, headers []tool.Header) bool {
	if req.URL == nil || req.URL.Host == "" || (req.URL.Scheme != schemeHTTP && req.URL.Scheme != schemeHTTPS) {
		drain(req)
		_ = writeSimple(conn, http.StatusBadRequest)
		return false
	}
	m := meta{proto: protoHTTP11, headers: headers}
	rp := &reply{
		write: writeHTTP1(conn), //nolint:bodyclose // writeHTTP1 closes the body it writes.
		abort: func() { _ = conn.Close() },
		conn:  conn,
		br:    br,
	}
	if _, err := s.serve(ctx, req, m, rp); err != nil {
		return false
	}
	return !rp.upgraded && !rp.closeAfter && !req.Close
}

// serve runs one request through the rules, the server and the recorder,
// and writes the answer with rp.
func (s *Server) serve(ctx context.Context, req *http.Request, m meta, rp *reply) (id uint64, err error) {
	started := time.Now()
	flow := newFlow(req, m, started)
	flow.ID = s.put(flow)
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("proxy error: %v", recovered)
			flow.State = tool.FlowFailed
			flow.Duration = time.Since(started)
			flow.Info = err.Error()
			s.put(flow)
			id = flow.ID
			rp.abort()
		}
	}()
	if !s.sameTunnelHost(req, m) {
		err := fmt.Errorf("request for %s inside a tunnel opened for %s", hostOfURL(req.URL), m.tunnelHost)
		return s.refuse(flow, started, rp, http.StatusMisdirectedRequest, err)
	}
	if s.isSelf(ctx, req.URL) {
		return s.refuse(flow, started, rp, http.StatusLoopDetected, errProxyLoop)
	}
	verdict, err := s.rulerRequest(req)
	if err != nil {
		_ = writeError(rp, http.StatusBadGateway, err)
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	if s.isSelf(ctx, req.URL) {
		return s.refuse(flow, started, rp, http.StatusLoopDetected, errProxyLoop)
	}
	if verdict.Block {
		s.finishBlocked(flow, verdict, started)
		rp.abort()
		return flow.ID, nil
	}
	if verdict.Mock != nil {
		return s.respond(ctx, req, flow, verdict, started, rp)
	}
	return s.forward(ctx, req, flow, verdict, started, rp)
}

// refuse answers a request the proxy must not send on and records it.
func (s *Server) refuse(flow tool.Flow, started time.Time, rp *reply, status int, err error) (uint64, error) {
	_ = writeError(rp, status, err)
	s.finishFailed(flow, started, err)
	return flow.ID, err
}

// sameTunnelHost reports whether a decrypted request stays on the host the
// tunnel was opened for. A client may otherwise open a tunnel for one name
// and make the proxy reach another.
func (s *Server) sameTunnelHost(req *http.Request, m meta) bool {
	if m.tunnelHost == "" {
		return true
	}
	host := hostOfURL(req.URL)
	return host == "" || sameHost(host, m.tunnelHost)
}

// respond answers with the response a mock rule gave, without contacting
// the server.
func (s *Server) respond(ctx context.Context, req *http.Request, flow tool.Flow, verdict rules.Verdict, started time.Time, rp *reply) (uint64, error) {
	flow.RuleHits = verdict.Hits
	flow.Ruled = len(verdict.Hits) > 0
	drain(req)
	if err := wait(ctx, verdict.Delay); err != nil {
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	resp := verdict.Mock
	normalize(resp)
	capture, body := captureStream(resp.Body)
	resp.Body = body
	headers := headerList(resp.Header)
	if err := rp.write(resp); err != nil {
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	s.finishDone(flow, req, resp, nil, capture, headers, started)
	return flow.ID, nil
}

// forward sends the request to its server, applies the response rules and
// records the exchange.
func (s *Server) forward(ctx context.Context, req *http.Request, flow tool.Flow, verdict rules.Verdict, started time.Time, rp *reply) (uint64, error) {
	request := captureRequest(req)
	up, err := s.send(ctx, req)
	if err != nil {
		drain(req)
		_ = writeError(rp, http.StatusBadGateway, err)
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	defer up.close()
	flow.ServerAddr = up.addr
	if flow.TLSVersion == "" {
		flow.TLSVersion = up.tlsVersion
	}
	upstreamStatus := up.resp.StatusCode
	hits, err := s.rulerResponse(req, up.resp)
	flow.RuleHits = append(append([]string(nil), verdict.Hits...), hits...)
	flow.Ruled = len(flow.RuleHits) > 0
	if err != nil {
		_ = writeError(rp, http.StatusBadGateway, err)
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	if upstreamStatus == http.StatusSwitchingProtocols && up.conn != nil && rp.conn != nil {
		return s.upgrade(flow, up, started, rp)
	}
	if err := wait(ctx, verdict.Delay); err != nil {
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	normalize(up.resp)
	capture, body := captureStream(up.resp.Body)
	up.resp.Body = body
	if err := rp.write(up.resp); err != nil {
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	s.finishForward(flow, req, up, request, capture, started, rp)
	return flow.ID, nil
}

// finishForward records the exchange and decides whether the client
// connection can carry another request.
func (s *Server) finishForward(flow tool.Flow, req *http.Request, up *sent, request *requestBody, capture *bodyCapture, started time.Time, rp *reply) {
	var requestCapture *bodyCapture
	if request != nil {
		requestCapture = request.capture
	}
	s.finishDone(flow, req, up.resp, requestCapture, capture, up.headers, started)
	if request != nil {
		request.release()
		if !request.capture.finished() {
			rp.closeAfter = true
		}
	}
	if up.resp.Close || (up.resp.ContentLength < 0 && len(up.resp.TransferEncoding) == 0) {
		rp.closeAfter = true
	}
}

// upgrade records a protocol switch and then pipes both directions until
// either side closes.
func (s *Server) upgrade(flow tool.Flow, up *sent, started time.Time, rp *reply) (uint64, error) {
	if rp.conn == nil || up.conn == nil {
		err := errors.New("no connection to upgrade")
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	if err := writeUpgrade(rp.conn, up.resp, up.headers); err != nil {
		s.finishFailed(flow, started, err)
		return flow.ID, err
	}
	rp.upgraded = true
	flow.Status = up.resp.StatusCode
	flow.Response = tool.HTTPMessage{Headers: up.headers}
	pipe(rp.conn, rp.br, up.conn, up.br)
	flow.State = tool.FlowDone
	flow.Duration = time.Since(started)
	s.put(flow)
	return flow.ID, nil
}

// writeUpgrade writes a 101 response and its headers, in the order they
// arrived, so the tunnel can start right after it.
func writeUpgrade(conn net.Conn, resp *http.Response, headers []tool.Header) error {
	if len(headers) == 0 {
		headers = headerList(resp.Header)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "HTTP/1.1 %s\r\n", resp.Status)
	for _, header := range headers {
		fmt.Fprintf(&b, "%s: %s\r\n", header.Name, header.Value)
	}
	b.WriteString("\r\n")
	_, err := io.WriteString(conn, b.String())
	return err
}

// writeError writes a short answer the proxy makes itself and closes it.
func writeError(rp *reply, status int, err error) error {
	resp := errorResponse(status, err)
	defer func() { _ = resp.Body.Close() }()
	return rp.write(resp)
}

// writeSimple writes a short answer the proxy makes itself.
func writeSimple(conn net.Conn, status int) error {
	resp := errorResponse(status, errors.New(http.StatusText(status)))
	defer func() { _ = resp.Body.Close() }()
	return resp.Write(conn)
}

// wait sleeps for d, and stops early when ctx ends.
func wait(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// pipe copies bytes both ways until either side closes, and returns how many
// bytes went each way.
func pipe(client net.Conn, clientBr *bufio.Reader, upstream net.Conn, upstreamBr *bufio.Reader) (fromClient, fromUpstream int64) {
	var wg sync.WaitGroup
	var once sync.Once
	stop := func() {
		once.Do(func() {
			_ = upstream.Close()
			_ = client.Close()
		})
	}
	wg.Add(2)
	go func() {
		defer wg.Done()
		fromClient, _ = io.Copy(upstream, readerFor(clientBr, client))
		stop()
	}()
	go func() {
		defer wg.Done()
		fromUpstream, _ = io.Copy(client, readerFor(upstreamBr, upstream))
		stop()
	}()
	wg.Wait()
	return fromClient, fromUpstream
}

// readerFor reads through the buffered reader when there is one.
func readerFor(br *bufio.Reader, conn net.Conn) io.Reader {
	if br != nil {
		return br
	}
	return conn
}

// rulerRequest asks the rules about an outgoing request.
func (s *Server) rulerRequest(req *http.Request) (rules.Verdict, error) {
	if s.opts.Rules == nil {
		return rules.Verdict{}, nil
	}
	return s.opts.Rules.Request(req)
}

// rulerResponse applies the rules to a response on its way back.
func (s *Server) rulerResponse(req *http.Request, resp *http.Response) ([]string, error) {
	if s.opts.Rules == nil {
		return nil, nil
	}
	return s.opts.Rules.Response(req, resp)
}

// firstNonEmpty returns the first of its arguments that is not empty.
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
