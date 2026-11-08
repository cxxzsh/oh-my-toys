package proxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// Do sends req through the same pipeline a proxied request takes (rules,
// upstream, recording) and returns the ID of the flow it recorded, with
// ReplayOf set when replayOf is not 0.
func (s *Server) Do(ctx context.Context, req *http.Request, replayOf uint64) (uint64, error) {
	m := meta{proto: protoHTTP11, replayOf: replayOf, headers: headerList(req.Header)}
	if req.URL != nil {
		m.decrypted = strings.EqualFold(req.URL.Scheme, schemeHTTPS)
	}
	rp := &reply{
		write: func(resp *http.Response) error {
			_, err := io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			return err
		},
		abort: func() {},
	}
	return s.serve(ctx, req, m, rp)
}

// Request builds the request a flow sent, so Do can send it again.
func Request(f tool.Flow) (*http.Request, error) {
	if f.Request.Body.Truncated {
		return nil, fmt.Errorf("the body of flow %d was cut at %d bytes and cannot be sent again", f.ID, tool.MaxBody)
	}
	parsed, err := url.Parse(f.URL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("flow %d has no absolute URL", f.ID)
	}
	method := f.Method
	if method == "" {
		method = http.MethodGet
	}
	req, err := http.NewRequestWithContext(context.Background(), method, f.URL, replayBody(f.Request.Body.Data))
	if err != nil {
		return nil, fmt.Errorf("flow %d: %w", f.ID, err)
	}
	req.Header = headerLines(f.Request.Headers)
	if decodedEncoding(f.Request.Headers) {
		req.Header.Del("Content-Encoding")
	}
	req.Host = f.Host
	if len(f.Request.Body.Data) > 0 {
		body := f.Request.Body.Data
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
	}
	return req, nil
}

// decodedEncoding reports whether the proxy removed a content encoding from
// the recorded body. The stored bytes are then the decoded ones, so sending
// the header again would describe them wrongly. The decision comes from the
// same decoder table decodeBody uses.
func decodedEncoding(headers []tool.Header) bool {
	decoded := false
	for _, line := range headers {
		if !strings.EqualFold(line.Name, "Content-Encoding") {
			continue
		}
		if !decodedByProxy(line.Value) {
			return false
		}
		decoded = true
	}
	return decoded
}

// replayBody is the replayable body of a flow.
func replayBody(data []byte) io.Reader {
	if len(data) == 0 {
		return nil
	}
	return bytes.NewReader(data)
}

// headerLines rebuilds a header map from a flow, keeping the names as they
// were and dropping what a proxy must not send.
func headerLines(headers []tool.Header) http.Header {
	built := make(http.Header, len(headers))
	for _, line := range headers {
		if hopByHop(line.Name) || strings.EqualFold(line.Name, "content-length") {
			continue
		}
		built[line.Name] = append(built[line.Name], line.Value)
	}
	return built
}
