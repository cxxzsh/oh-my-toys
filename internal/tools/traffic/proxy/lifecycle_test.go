package proxy

import (
	"context"
	"errors"
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

func TestNewAcceptsOnlyLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:0", "localhost:0", "[::1]:0"} {
		if _, err := New(Options{Addr: addr}); err != nil {
			t.Errorf("New(%q) = %v, want a server", addr, err)
		}
	}
	for _, addr := range []string{"0.0.0.0:9099", ":9099", "192.0.2.1:9099", "example.com:9099", "not an address"} {
		if _, err := New(Options{Addr: addr}); err == nil {
			t.Errorf("New(%q) accepted a non-loopback address", addr)
		}
	}
}

func TestStartReportsAPortInUse(t *testing.T) {
	holder, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Close() })
	srv, err := New(Options{Addr: holder.Addr().String()})
	if err != nil {
		t.Fatal(err)
	}
	err = srv.Start(context.Background())
	if !errors.Is(err, tool.ErrPortInUse) {
		t.Fatalf("Start on a taken port = %v, want tool.ErrPortInUse", err)
	}
}

func TestStopClosesLiveConnections(t *testing.T) {
	srv := startProxy(t, Options{})
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(context.Background(), "tcp", srv.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	// A request that never ends leaves the handler waiting for headers.
	if _, err := conn.Write([]byte("GET http://example.test/ HTTP/1.1\r\n")); err != nil {
		t.Fatal(err)
	}
	stopped := make(chan error, 1)
	go func() { stopped <- srv.Stop() }()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("Stop = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Stop waited forever for a live connection")
	}
	if _, err := conn.Write([]byte("Host: example.test\r\n\r\n")); err == nil {
		if _, err := conn.Read(make([]byte, 1)); err == nil {
			t.Error("the connection still answers after Stop")
		}
	}
	if err := srv.Stop(); err != nil {
		t.Errorf("second Stop = %v", err)
	}
}

func TestStopLeavesNoGoroutines(t *testing.T) {
	baseline := runtime.NumGoroutine()
	srv := startProxy(t, Options{})
	if _, _, _, err := get(t, clientThrough(t, srv, nil, false), "http://"+srv.Addr()+"/?probe"); err != nil {
		t.Logf("probe request: %v", err)
	}
	if err := srv.Stop(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if runtime.NumGoroutine() <= baseline {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("goroutines = %d after Stop, want at most %d", runtime.NumGoroutine(), baseline)
}

func TestCancellingTheContextStopsTheServer(t *testing.T) {
	srv, err := New(Options{Addr: "127.0.0.1:0"})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := srv.Start(ctx); err != nil {
		t.Fatal(err)
	}
	cancel()
	dialer := &net.Dialer{Timeout: 200 * time.Millisecond}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := dialer.DialContext(context.Background(), "tcp", srv.Addr())
		if err != nil {
			if err := srv.Stop(); err != nil {
				t.Fatalf("Stop after cancel = %v", err)
			}
			return
		}
		_ = conn.Close()
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the server still accepts connections after its context ended")
}
