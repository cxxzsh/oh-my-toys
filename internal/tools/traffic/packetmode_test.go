package traffic

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cxxzsh/oh-my-toys/internal/i18n"
	"github.com/cxxzsh/oh-my-toys/internal/platform/pcap"
	"github.com/cxxzsh/oh-my-toys/internal/tool"
)

// packetConfig is the settings of a packet capture.
func packetConfig() tool.CaptureConfig {
	return tool.CaptureConfig{Mode: tool.ModePackets, ProxyPort: defaultProxyPort}
}

// udpPacket is one Ethernet frame holding an IPv4 UDP packet to a DNS
// server, which the decoder turns into a row without a network.
func udpPacket() pcap.Packet {
	ip := []byte{
		0x45, 0x00, 0x00, 0x1d, // version, header length, total length
		0x00, 0x01, 0x00, 0x00,
		0x40, 0x11, 0x00, 0x00, // TTL, UDP
		0xc0, 0xa8, 0x01, 0x01, // source
		0x08, 0x08, 0x08, 0x08, // destination
		0x30, 0x39, 0x00, 0x35, 0x00, 0x09, 0x00, 0x00, // ports, length, checksum
		0x00,
	}
	frame := append([]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 0x08, 0x00}, ip...)
	return pcap.Packet{Time: time.Unix(1, 0), Link: pcap.LinkEthernet, Data: frame, Length: len(frame)}
}

func TestPacketCaptureRecordsPackets(t *testing.T) {
	tl, fake := testTool(t)
	src := newFakeSource()
	fake.defName = "en0"
	fake.open = func(string) (pcap.Source, error) { return src, nil }
	if err := tl.SetConfig(packetConfig()); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	src.packets <- udpPacket()

	rows := waitRows(t, tl.Log(), tool.SourcePacket, 1)
	if rows[0].Source != tool.SourcePacket {
		t.Errorf("row source %v, want a packet", rows[0].Source)
	}
	if st := tl.Status(); !st.On {
		t.Errorf("status %+v, want on", st)
	}
	if err := tl.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if st := tl.Status(); st.On {
		t.Errorf("status %+v, want off", st)
	}
	select {
	case <-src.closed:
	default:
		t.Error("the packet source was not closed")
	}
}

func TestPacketCaptureMapsTheSystemRefusal(t *testing.T) {
	tests := []struct {
		name     string
		openErr  error
		grantErr error
		want     error
	}{
		{
			name: "permission, refused at the prompt", openErr: pcap.ErrPermission, grantErr: pcap.ErrPermission,
			want: tool.ErrPermission,
		},
		{name: "the driver is missing", openErr: pcap.ErrDriverMissing, want: tool.ErrDriverMissing},
		{name: "no packet capture on this system", openErr: pcap.ErrUnsupported, want: tool.ErrUnsupportedHere},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tl, fake := testTool(t)
			fake.defName = "en0"
			fake.open = func(string) (pcap.Source, error) { return nil, tt.openErr }
			fake.grant = func(context.Context, string) error { return tt.grantErr }
			if err := tl.SetConfig(packetConfig()); err != nil {
				t.Fatalf("SetConfig: %v", err)
			}
			err := tl.Start(context.Background())
			if !errors.Is(err, tt.want) {
				t.Errorf("Start: %v, want %v", err, tt.want)
			}
			if !errors.Is(err, tt.openErr) {
				t.Errorf("Start: %v, want the detail of %v kept", err, tt.openErr)
			}
			if st := tl.Status(); st.On {
				t.Errorf("status %+v, want off", st)
			}
		})
	}
}

func TestPacketCaptureAsksForPermissionOnceAndRetries(t *testing.T) {
	tl, fake := testTool(t)
	src := newFakeSource()
	fake.defName = "en0"
	opens := 0
	fake.open = func(string) (pcap.Source, error) {
		opens++
		if opens == 1 {
			return nil, pcap.ErrPermission
		}
		return src, nil
	}
	if err := tl.SetConfig(packetConfig()); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = tl.Stop() })
	if fake.granted != 1 {
		t.Errorf("the person was asked %d times, want once", fake.granted)
	}
	if want := i18n.TrafficGrantReason(); fake.grantReason != want {
		t.Errorf("the prompt said %q, want %q", fake.grantReason, want)
	}
	if opens != 2 {
		t.Errorf("opened %d times, want a retry after the grant", opens)
	}
	if st := tl.Status(); !st.On {
		t.Errorf("status %+v, want on", st)
	}
}

func TestPacketCaptureKeepsTheReasonItStopped(t *testing.T) {
	tl, fake := testTool(t)
	fake.defName = "en0"
	fake.open = func(string) (pcap.Source, error) {
		return brokenSource{err: errors.New("the device went away")}, nil
	}
	if err := tl.SetConfig(packetConfig()); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := tl.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	st := waitOff(t, tl)
	if want := i18n.TrafficCaptureStoppedNote(); st.Note != want {
		t.Errorf("note %q, want %q", st.Note, want)
	}
	if strings.Contains(st.Note, "the device went away") {
		t.Error("the note quotes the raw error; that belongs in the log")
	}
	if rows := tl.Log().Rows(tool.Filter{Source: tool.SourcePacket}); len(rows) != 0 {
		t.Errorf("%d rows, want none", len(rows))
	}
}

// brokenSource fails every read, as a device that goes away does.
type brokenSource struct{ err error }

func (s brokenSource) Read() (pcap.Packet, error) { return pcap.Packet{}, s.err }

func (s brokenSource) Close() error { return nil }

func TestInterfaces(t *testing.T) {
	tl, fake := testTool(t)
	fake.ifaces = []pcap.Interface{
		{Name: "en0", Description: "Wi-Fi", Up: true},
		{Name: "lo0", Description: "Loopback", Up: true, Loopback: true},
	}
	got, err := tl.Interfaces()
	if err != nil {
		t.Fatalf("Interfaces: %v", err)
	}
	want := []tool.NetInterface{
		{Name: "en0", Description: "Wi-Fi", Up: true},
		{Name: "lo0", Description: "Loopback", Up: true},
	}
	if len(got) != len(want) {
		t.Fatalf("Interfaces = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("interface %d = %+v, want %+v", i, got[i], want[i])
		}
	}

	fake.ifaceErr = pcap.ErrPermission
	if _, err := tl.Interfaces(); !errors.Is(err, tool.ErrPermission) {
		t.Errorf("Interfaces: %v, want tool.ErrPermission", err)
	}
}
