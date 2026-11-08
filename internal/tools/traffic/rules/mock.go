package rules

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// defaultContentType is what a mock answers with when its rule names none.
const defaultContentType = "text/plain; charset=utf-8"

// mock builds the response the rule answers with. A rule's file is read
// here, not when the rule was set: the file may come and go, and a rule
// book must not become unusable because a file moved.
func (r rule) mock(req *http.Request) (*http.Response, error) {
	body := []byte(r.raw.Mock.Body)
	if file := r.raw.Mock.File; file != "" {
		data, err := readMockFile(file)
		if err != nil {
			return nil, r.notApplied(err)
		}
		body = data
	}
	status := r.raw.Mock.Status
	if status == 0 {
		status = http.StatusOK
	}
	header := make(http.Header, len(r.raw.Mock.Headers)+2)
	for _, h := range r.raw.Mock.Headers {
		header.Add(h.Name, h.Value)
	}
	if header.Get("Content-Type") == "" {
		header.Set("Content-Type", defaultContentType)
	}
	header.Set("Content-Length", strconv.Itoa(len(body)))
	return &http.Response{
		Status:        statusLine(status),
		StatusCode:    status,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
	}, nil
}

// readMockFile reads a mock body of at most tool.MaxBody bytes from a
// regular file. The stat comes first: a device or a pipe never ends, and a
// file over the cap is refused rather than served cut short.
func readMockFile(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat mock body: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("mock body %s is not a regular file", path)
	}
	if info.Size() > tool.MaxBody {
		return nil, fmt.Errorf("%w: %s holds %d bytes", ErrBodyTooLarge, path, info.Size())
	}
	file, err := os.Open(path) //nolint:gosec // the path is the person's own rule
	if err != nil {
		return nil, fmt.Errorf("open mock body: %w", err)
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, tool.MaxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read mock body: %w", err)
	}
	if len(data) > tool.MaxBody {
		return nil, fmt.Errorf("%w: %s grew past %d bytes", ErrBodyTooLarge, path, tool.MaxBody)
	}
	return data, nil
}
