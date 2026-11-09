package procnet

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// listenTCP opens a TCP listener with a background context, so the test
// does not depend on any caller's deadline.
func listenTCP(addr string) (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(context.Background(), "tcp", addr)
}

// dialTCP connects to addr with a background context.
func dialTCP(addr string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(context.Background(), "tcp", addr)
}

// acceptTimeout bounds how long the test waits for the listener to accept.
const acceptTimeout = 5 * time.Second

// ownerCase is one connection, as the proxy would name it, to ask Owner about.
type ownerCase struct {
	name          string
	local, remote netip.AddrPort
}

// testBinaryName returns this test binary's path and the program name Owner
// must report for it: the file name without ".exe".
func testBinaryName(t *testing.T) (string, string) {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe, strings.TrimSuffix(filepath.Base(exe), ".exe")
}

// isFamilyUnsupported reports whether a listen failed because the address
// family is not available on this machine.
func isFamilyUnsupported(err error) bool {
	return errors.Is(err, syscall.EAFNOSUPPORT) || strings.Contains(err.Error(), "address family not supported")
}

// dialPair connects a client to a fresh listener on addr and returns both
// ends. The test is skipped when the address family is not available.
func dialPair(t *testing.T, addr string) (net.Conn, net.Conn) {
	t.Helper()
	ln, err := listenTCP(addr)
	if err != nil {
		if isFamilyUnsupported(err) {
			t.Skipf("address family not supported: %v", err)
		}
		t.Fatalf("listen %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	accepted := make(chan net.Conn, 1)
	acceptErr := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		accepted <- c
	}()

	client, err := dialTCP(ln.Addr().String())
	if err != nil {
		t.Fatalf("dial %s: %v", ln.Addr(), err)
	}
	t.Cleanup(func() { _ = client.Close() })

	select {
	case server := <-accepted:
		t.Cleanup(func() { _ = server.Close() })
		return client, server
	case err := <-acceptErr:
		t.Fatalf("accept: %v", err)
	case <-time.After(acceptTimeout):
		t.Fatal("accept timed out")
	}
	return nil, nil
}

// addrPort returns the TCP address of a connection's end.
func addrPort(t *testing.T, a net.Addr) netip.AddrPort {
	t.Helper()
	tcp, ok := a.(*net.TCPAddr)
	if !ok {
		t.Fatalf("address %v is %T, not *net.TCPAddr", a, a)
	}
	return tcp.AddrPort()
}

// TestOwnerLoopback checks that a connection made by this process is owned
// by this process, from both ends, over IPv4 and IPv6.
func TestOwnerLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "[::1]:0"} {
		t.Run(addr, func(t *testing.T) {
			testOwnerLoopback(t, addr)
		})
	}
}

func testOwnerLoopback(t *testing.T, addr string) {
	client, server := dialPair(t, addr)
	exe, name := testBinaryName(t)

	clientLocal := addrPort(t, client.LocalAddr())
	clientRemote := addrPort(t, client.RemoteAddr())
	serverLocal := addrPort(t, server.LocalAddr())
	serverRemote := addrPort(t, server.RemoteAddr())

	// Owner takes a socket's own local end and its peer. Both ends of the
	// loopback connection belong to this process, so both sockets are checked.
	cases := []ownerCase{
		{"client socket", clientLocal, clientRemote},
		{"accepted socket", serverLocal, serverRemote},
	}
	if clientLocal.Addr().Is4() {
		mapped := netip.AddrPortFrom(netip.AddrFrom16(clientLocal.Addr().As16()), clientLocal.Port())
		cases = append(cases, ownerCase{"client socket, IPv4-mapped spelling", mapped, clientRemote})
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			checkOwner(t, c, exe, name)
		})
	}

	testUnownedConnections(t, addr, client, server, clientLocal, clientRemote)
}

// checkOwner asks Owner about one connection and checks it names this process.
func checkOwner(t *testing.T, c ownerCase, exe, name string) {
	t.Helper()
	p, err := Owner(c.local, c.remote)
	if err != nil {
		t.Fatalf("Owner(%v, %v): %v", c.local, c.remote, err)
	}
	t.Logf("Owner(%v, %v) = pid %d name %q path %q", c.local, c.remote, p.PID, p.Name, p.Path)
	if p.PID != os.Getpid() {
		t.Fatalf("pid = %d, want %d", p.PID, os.Getpid())
	}
	if p.Name != name {
		t.Fatalf("name = %q, want %q", p.Name, name)
	}
	if p.Path != "" && !sameFile(p.Path, exe) {
		t.Fatalf("path = %q, want %q", p.Path, exe)
	}
}

// testUnownedConnections checks that a closed connection and a port nobody
// listens on are reported as not found.
func testUnownedConnections(t *testing.T, addr string, client, server net.Conn, local, remote netip.AddrPort) {
	t.Helper()
	_ = client.Close()
	_ = server.Close()
	if _, err := Owner(local, remote); !errors.Is(err, ErrNotFound) {
		t.Fatalf("closed connection: err = %v, want ErrNotFound", err)
	}

	ln, err := listenTCP(addr)
	if err != nil {
		t.Fatalf("listen %s: %v", addr, err)
	}
	unused := addrPort(t, ln.Addr())
	_ = ln.Close()
	if _, err := Owner(unused, unused); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unused port: err = %v, want ErrNotFound", err)
	}
}

// sameFile reports whether two paths name the same file, following symlinks
// since a platform may report the resolved path.
func sameFile(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return a == b
	}
	return ra == rb
}
