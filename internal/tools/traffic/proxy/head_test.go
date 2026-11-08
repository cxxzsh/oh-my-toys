package proxy

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// TestUpstreamHeadIsBounded checks that an upstream which never ends its
// header block fails the exchange instead of spending the app's memory.
func TestUpstreamHeadIsBounded(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go endlessHead(conn)
		}
	}()

	recorded := newTestRecorder()
	srv := startProxy(t, Options{Log: recorded})
	start := time.Now()
	status, _, body, err := get(t, clientThrough(t, srv, nil, false), "http://"+listener.Addr().String()+"/")
	if err == nil && status != http.StatusBadGateway {
		t.Fatalf("client saw %d %q, want the 502 of a failed exchange", status, body)
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Errorf("the exchange took %v to fail", elapsed)
	}
	flow := recorded.wait(t, func(flow tool.Flow) bool { return flow.State == tool.FlowFailed })
	if !strings.Contains(flow.Info, "head") {
		t.Errorf("flow info = %q, want a head limit error", flow.Info)
	}
}

// endlessHead writes 8 MiB of header lines and then holds the connection
// open, so a reader without a budget waits forever.
func endlessHead(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	if _, err := io.WriteString(conn, "HTTP/1.1 200 OK\r\n"); err != nil {
		return
	}
	line := bytes.Repeat([]byte("X-Pad: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\n"), 64)
	for range 8 << 10 {
		if _, err := conn.Write(line); err != nil {
			return
		}
	}
	_, _ = io.Copy(io.Discard, conn)
}

// TestInterimResponsesAreSkipped checks that a 100 Continue from the
// upstream is not mistaken for the answer.
func TestInterimResponsesAreSkipped(t *testing.T) {
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go continueAndAnswer(conn)
		}
	}()

	srv := startProxy(t, Options{})
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL(t, srv)),
		DisableKeepAlives:     true,
		ExpectContinueTimeout: time.Second,
	}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}

	request, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "http://"+listener.Addr().String()+"/", strings.NewReader("payload"))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Expect", "100-continue")
	resp, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "ok:payload" {
		t.Fatalf("client saw %d %q", resp.StatusCode, body)
	}
}

// continueAndAnswer sends a 100 Continue, reads the body and answers.
func continueAndAnswer(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil {
		return
	}
	if _, err := io.WriteString(conn, "HTTP/1.1 100 Continue\r\n\r\n"); err != nil {
		return
	}
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return
	}
	answer := "ok:" + string(body)
	_, _ = fmt.Fprintf(conn, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nContent-Type: text/plain\r\n\r\n%s", len(answer), answer)
}

// TestCloseDelimitedResponseClosesTheClient checks that a response whose end
// is the end of the connection also ends the client connection, so the
// client is not left waiting.
func TestCloseDelimitedResponseClosesTheClient(t *testing.T) {
	addr := startCloseDelimited(t)
	srv := startProxy(t, Options{})
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	reader := requestCloseDelimited(t, conn, addr.String())

	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("the connection still carries data after a close delimited response")
	} else {
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() {
			t.Fatal("the proxy did not close the client connection after a close delimited response")
		}
	}
}

// startCloseDelimited runs an upstream that ends its answer with the
// connection.
func startCloseDelimited(t *testing.T) net.Addr {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go answerCloseDelimited(conn)
		}
	}()
	return listener.Addr()
}

// answerCloseDelimited reads one request, answers without a length and
// closes.
func answerCloseDelimited(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	if _, err := http.ReadRequest(bufio.NewReader(conn)); err != nil {
		return
	}
	_, _ = io.WriteString(conn, "HTTP/1.1 200 OK\r\nContent-Type: text/plain\r\n\r\nclose delimited")
}

// requestCloseDelimited sends one request through the proxy and returns the
// reader left after the body.
func requestCloseDelimited(t *testing.T, conn net.Conn, authority string) *bufio.Reader {
	t.Helper()
	if _, err := fmt.Fprintf(conn, "GET http://%s/ HTTP/1.1\r\nHost: %s\r\n\r\n", authority, authority); err != nil {
		t.Fatal(err)
	}
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read the close delimited body: %v", err)
	}
	if string(body) != "close delimited" {
		t.Fatalf("body = %q", body)
	}
	return reader
}
