package proxy

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/rules"
)

// meta is what the connection knows about a request before it is handled.
type meta struct {
	proto      string
	decrypted  bool
	tlsVersion string
	headers    []tool.Header
	replayOf   uint64
	program    tool.Program
	// tunnelHost is the host a decrypted tunnel was opened for; a request
	// for another host inside it is refused.
	tunnelHost string
}

// reply writes one response to the client and can drop its connection.
type reply struct {
	write func(*http.Response) error
	abort func()
	conn  net.Conn
	br    *bufio.Reader
	// upgraded is set when the connection became a tunnel, so the caller
	// stops reading requests from it.
	upgraded bool
	// closeAfter is set when the client connection must not carry another
	// request: the request body was not read, or the response ended with
	// the connection.
	closeAfter bool
}

// newFlow is the flow of a request that has just been read.
func newFlow(req *http.Request, m meta, started time.Time) tool.Flow {
	flow := tool.Flow{
		FlowRow: tool.FlowRow{
			Source:  tool.SourceProxy,
			Started: started,
			Method:  req.Method,
			Host:    requestHost(req),
			Path:    requestPath(req),
			Proto:   m.proto,
			Process: m.program,
			State:   tool.FlowOpen,
		},
		Request:    tool.HTTPMessage{Headers: m.headers},
		Decrypted:  m.decrypted,
		TLSVersion: m.tlsVersion,
		ReplayOf:   m.replayOf,
	}
	if req.URL != nil {
		flow.URL = req.URL.String()
	}
	return flow
}

// requestHost is the host the request is for.
func requestHost(req *http.Request) string {
	if req.Host != "" {
		return req.Host
	}
	if req.URL != nil {
		return req.URL.Host
	}
	return ""
}

// requestPath is the path and query of the request.
func requestPath(req *http.Request) string {
	if req.URL == nil {
		return ""
	}
	return req.URL.RequestURI()
}

// put records a flow, or returns its ID when there is no recorder.
func (s *Server) put(flow tool.Flow) uint64 {
	if s.opts.Log == nil {
		return flow.ID
	}
	return s.opts.Log.Put(flow)
}

// finishDone records a flow whose response has been sent.
func (s *Server) finishDone(flow tool.Flow, req *http.Request, resp *http.Response, reqCap, respCap *bodyCapture, headers []tool.Header, started time.Time) {
	flow.State = tool.FlowDone
	flow.Duration = time.Since(started)
	if resp != nil {
		flow.Status = resp.StatusCode
		flow.Type = mediaType(resp.Header.Get("Content-Type"))
		flow.Response = tool.HTTPMessage{Headers: headers, Body: responseBody(resp, respCap)}
	}
	if respCap != nil {
		_, flow.Size, _ = respCap.snapshot()
	}
	if reqCap != nil {
		flow.Request.Body = reqCap.body(requestHeader(req, "Content-Type"), requestHeader(req, "Content-Encoding"))
	}
	s.put(flow)
}

// finishFailed records a flow that ended with err.
func (s *Server) finishFailed(flow tool.Flow, started time.Time, err error) {
	flow.State = tool.FlowFailed
	flow.Duration = time.Since(started)
	flow.Info = err.Error()
	s.put(flow)
}

// finishBlocked records a flow a rule refused.
func (s *Server) finishBlocked(flow tool.Flow, verdict rules.Verdict, started time.Time) {
	flow.State = tool.FlowBlocked
	flow.Duration = time.Since(started)
	flow.RuleHits = verdict.Hits
	flow.Ruled = len(verdict.Hits) > 0
	s.put(flow)
}

// responseBody is the body a flow keeps of a response.
func responseBody(resp *http.Response, capture *bodyCapture) tool.Body {
	if capture == nil {
		return tool.Body{Type: mediaType(resp.Header.Get("Content-Type"))}
	}
	return capture.body(resp.Header.Get("Content-Type"), resp.Header.Get("Content-Encoding"))
}

// requestHeader reads a header of the request as it is being sent.
func requestHeader(req *http.Request, name string) string {
	if req == nil {
		return ""
	}
	return req.Header.Get(name)
}

// errorResponse is a short answer the proxy writes itself.
func errorResponse(status int, err error) *http.Response {
	text := fmt.Sprintf("%d %s\n", status, err.Error())
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Proto:         protoHTTP11,
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        http.Header{"Content-Type": []string{"text/plain; charset=utf-8"}},
		Body:          io.NopCloser(strings.NewReader(text)),
		ContentLength: int64(len(text)),
	}
}

// normalize fills in what a hand-built response may leave empty, so it can
// be written to either protocol.
func normalize(resp *http.Response) {
	if resp.ProtoMajor == 0 {
		resp.Proto, resp.ProtoMajor, resp.ProtoMinor = protoHTTP11, 1, 1
	}
	if resp.Status == "" {
		resp.Status = fmt.Sprintf("%d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	}
	if resp.Body == nil {
		resp.Body = http.NoBody
	}
}

// drain reads a request body the proxy did not send on, so the connection
// can carry the next request.
func drain(req *http.Request) {
	if req.Body != nil && req.Body != http.NoBody {
		_, _ = io.Copy(io.Discard, req.Body)
	}
}
