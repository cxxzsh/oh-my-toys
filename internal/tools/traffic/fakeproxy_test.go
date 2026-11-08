package traffic

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
)

// The proxies a test dials through: both answer a CONNECT to any address by
// tunnelling to the one server the test names, so no test resolves a name
// or reaches the network.

// startCONNECTProxy runs a proxy that answers CONNECT and tunnels every
// connection to target, so that no test resolves a name, and that reports
// the target and credentials it was asked for.
func startCONNECTProxy(t *testing.T, target string) (addr string, seen chan string) {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	seen = make(chan string, 4)
	var wg sync.WaitGroup
	// Closing the listener is what ends the accept loop, so it comes
	// before the wait, and both happen once the test is done with them.
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				serveCONNECT(conn, target, seen)
			}()
		}
	}()
	return ln.Addr().String(), seen
}

// serveCONNECT answers one request: a CONNECT becomes a tunnel to target,
// and a plain request in absolute form is forwarded to it.
func serveCONNECT(conn net.Conn, target string, seen chan<- string) {
	defer func() { _ = conn.Close() }()
	br := bufio.NewReader(conn)
	req, err := http.ReadRequest(br)
	if err != nil {
		return
	}
	seen <- req.Method + " " + req.Host + " " + req.Header.Get("Proxy-Authorization")
	out, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", target)
	if err == nil {
		defer func() { _ = out.Close() }()
	}
	if req.Method == http.MethodConnect {
		_ = req.Body.Close()
		if err != nil {
			return
		}
		if _, err := io.WriteString(conn, "HTTP/1.1 200 Connection established\r\n\r\n"); err != nil {
			return
		}
		go func() { _, _ = io.Copy(out, br) }()
		_, _ = io.Copy(conn, out)
		return
	}
	// A plain request: forward it as it came and copy the answer back.
	if err != nil {
		_ = req.Body.Close()
		return
	}
	req.RequestURI = ""
	err = req.Write(out)
	_ = req.Body.Close()
	if err == nil {
		_, _ = io.Copy(conn, out)
	}
}

// startSOCKS5 runs a SOCKS5 proxy that requires user and password and
// tunnels every CONNECT to target.
func startSOCKS5(t *testing.T, target, user, password string) (addr string, seen chan string) {
	t.Helper()
	ln, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	seen = make(chan string, 4)
	var wg sync.WaitGroup
	// Closing the listener is what ends the accept loop, so it comes
	// before the wait, and both happen once the test is done with them.
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				serveSOCKS5(conn, target, user, password, seen)
			}()
		}
	}()
	return ln.Addr().String(), seen
}

// serveSOCKS5 answers the greeting, the password and one CONNECT of the
// SOCKS5 protocol (RFC 1928 and 1929), then copies bytes both ways.
func serveSOCKS5(conn net.Conn, target, user, password string, seen chan<- string) {
	defer func() { _ = conn.Close() }()
	br := bufio.NewReader(conn)
	if !socksGreeting(br, conn) || !socksAuth(br, conn, user, password) {
		return
	}
	host, _, ok := socksRequest(br)
	if !ok {
		return
	}
	seen <- host
	out, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", target)
	if err != nil {
		_, _ = conn.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer func() { _ = out.Close() }()
	if _, err := conn.Write([]byte{5, 0, 0, 1, 0, 0, 0, 0, 0, 0}); err != nil {
		return
	}
	go func() { _, _ = io.Copy(out, br) }()
	_, _ = io.Copy(conn, out)
}

// socksGreeting reads the methods a client offers and asks for a user and
// a password.
func socksGreeting(br *bufio.Reader, conn net.Conn) bool {
	head := make([]byte, 2)
	if _, err := io.ReadFull(br, head); err != nil || head[0] != 5 {
		return false
	}
	if _, err := io.ReadFull(br, make([]byte, int(head[1]))); err != nil {
		return false
	}
	_, err := conn.Write([]byte{5, 2})
	return err == nil
}

// socksAuth reads the user and password and accepts them when they are
// the ones the test set.
func socksAuth(br *bufio.Reader, conn net.Conn, user, password string) bool {
	version, err := br.ReadByte()
	if err != nil || version != 1 {
		return false
	}
	gotUser, ok := socksString(br)
	if !ok {
		return false
	}
	gotPassword, ok := socksString(br)
	if !ok {
		return false
	}
	status := byte(1)
	if gotUser == user && gotPassword == password {
		status = 0
	}
	if _, err := conn.Write([]byte{1, status}); err != nil {
		return false
	}
	return status == 0
}

// socksString reads one length-prefixed field of the authenticator.
func socksString(br *bufio.Reader) (string, bool) {
	length, err := br.ReadByte()
	if err != nil {
		return "", false
	}
	value := make([]byte, int(length))
	if _, err := io.ReadFull(br, value); err != nil {
		return "", false
	}
	return string(value), true
}

// socksRequest reads the CONNECT request and returns the target it names
// as "host:port".
func socksRequest(br *bufio.Reader) (target string, port int, ok bool) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(br, head); err != nil || head[0] != 5 || head[1] != 1 {
		return "", 0, false
	}
	host, ok := socksAddress(br, head[3])
	if !ok {
		return "", 0, false
	}
	var portBytes [2]byte
	if _, err := io.ReadFull(br, portBytes[:]); err != nil {
		return "", 0, false
	}
	port = int(portBytes[0])<<8 | int(portBytes[1])
	return net.JoinHostPort(host, strconv.Itoa(port)), port, true
}

// socksAddress reads the address of a request, which the tests always
// send as a name.
func socksAddress(br *bufio.Reader, atyp byte) (string, bool) {
	switch atyp {
	case 1, 4:
		size := 4
		if atyp == 4 {
			size = 16
		}
		ip := make([]byte, size)
		if _, err := io.ReadFull(br, ip); err != nil {
			return "", false
		}
		return net.IP(ip).String(), true
	case 3:
		return socksString(br)
	}
	return "", false
}
