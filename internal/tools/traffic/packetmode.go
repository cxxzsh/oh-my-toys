package traffic

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
	"github.com/cxxzsh/oh-my-toys/internal/tools/traffic/packets"
)

// packetRun is a packet capture in progress.
type packetRun struct {
	iface  string
	cancel context.CancelFunc
	done   chan struct{}
}

// stop ends the reader and waits for the goroutine that runs it.
func (r *packetRun) stop() {
	r.cancel()
	<-r.done
}

// startPackets runs the packet capture of one interface. It changes no
// system setting: the packets of an interface are only read.
func (t *Tool) startPackets(ctx context.Context, cfg tool.CaptureConfig) error {
	name := cfg.Interface
	if name == "" {
		chosen, err := t.sys.pcap.defaultOne()
		if err != nil {
			return packetError("find the interface the system routes through", "", err)
		}
		name = chosen
	}
	if name == "" {
		return noInterfaceError()
	}
	src, err := t.openSource(ctx, name)
	if err != nil {
		return err
	}
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	run := &packetRun{iface: name, cancel: cancel, done: make(chan struct{})}
	t.mu.Lock()
	t.packet = run
	t.since = t.sys.now()
	t.note = ""
	t.mu.Unlock()
	go func() {
		defer close(run.done)
		t.packetsEnded(run, packets.Run(lifetime, src, t.packetlog))
	}()
	return nil
}

// openSource opens the interface, asking the person once for the access
// packet capture needs when the system refuses it.
func (t *Tool) openSource(ctx context.Context, name string) (pcap.Source, error) {
	src, err := t.sys.pcap.open(name)
	if !errors.Is(err, pcap.ErrPermission) {
		if err != nil {
			return nil, packetError("open the interface", name, err)
		}
		return src, nil
	}
	if err := t.sys.pcap.grant(ctx, i18n.TrafficGrantReason()); err != nil {
		return nil, packetError("ask for permission to read packets on", name, err)
	}
	src, err = t.sys.pcap.open(name)
	if err != nil {
		return nil, packetError("open the interface", name, err)
	}
	return src, nil
}

// packetsEnded takes the capture down when its reader stops. A failure is
// written to the log, and Status keeps a message of its own: the reason of
// a system call is no sentence for a person reading the tray.
func (t *Tool) packetsEnded(run *packetRun, err error) {
	failed := err != nil && !errors.Is(err, context.Canceled)
	if failed {
		log.Printf("traffic: the packet capture on %s stopped: %v", run.iface, err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.packet != run {
		return // a later capture has taken its place
	}
	t.packet, t.since = nil, time.Time{}
	if failed {
		t.note = i18n.TrafficCaptureStoppedNote()
	}
}

// Interfaces lists the interfaces a packet capture can read.
func (t *Tool) Interfaces() ([]tool.NetInterface, error) {
	list, err := t.sys.pcap.interfaces()
	if err != nil {
		return nil, packetError("list the interfaces", "", err)
	}
	out := make([]tool.NetInterface, len(list))
	for i, it := range list {
		out[i] = tool.NetInterface{Name: it.Name, Description: it.Description, Up: it.Up}
	}
	return out, nil
}

// noInterfaceError is what a capture without an interface and without a
// default on this system returns.
func noInterfaceError() error {
	return fmt.Errorf("no interface to read: %w", tool.ErrUnsupportedHere)
}
