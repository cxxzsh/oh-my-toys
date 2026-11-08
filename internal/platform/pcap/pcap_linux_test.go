//go:build linux

package pcap

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestProcRouteInterface(t *testing.T) {
	const header = "Iface\tDestination\tGateway \tFlags\tRefCnt\tUse\tMetric\tMask\t\tMTU\tWindow\tIRTT\n"
	for _, c := range []struct {
		name  string
		table string
		want  string
	}{
		{
			name: "lowest metric wins",
			table: header +
				"enp0s3\t00000000\t0102A8C0\t0003\t0\t0\t100\t00000000\t0\t0\t0\n" +
				"docker0\t00000000\t00000000\t0003\t0\t0\t200\t00000000\t0\t0\t0\n",
			want: "enp0s3",
		},
		{
			name: "a down route is not the default",
			table: header +
				"wlan0\t00000000\t0102A8C0\t0002\t0\t0\t0\t00000000\t0\t0\t0\n" +
				"eth0\t00000000\t0102A8C0\t0003\t0\t0\t600\t00000000\t0\t0\t0\n",
			want: "eth0",
		},
		{
			name: "a subnet route is not the default",
			table: header +
				"enp0s3\t0002A8C0\t00000000\t0001\t0\t0\t100\t00FFFFFF\t0\t0\t0\n",
			want: "",
		},
		{"empty", "", ""},
	} {
		if got := procRouteInterface([]byte(c.table)); got != c.want {
			t.Errorf("%s: interface = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestHtons(t *testing.T) {
	for _, c := range []struct {
		in, want uint16
	}{
		{ethPAll, 0x0300},
		{0x1234, 0x3412},
		{0, 0},
	} {
		if got := htons(c.in); got != c.want {
			t.Errorf("htons(%#x) = %#x, want %#x", c.in, got, c.want)
		}
	}
}

func TestSocketErr(t *testing.T) {
	for _, c := range []struct {
		name      string
		err       error
		permitted bool
	}{
		{"without CAP_NET_RAW", unix.EPERM, true},
		{"access denied", unix.EACCES, true},
		{"no such device", unix.ENODEV, false},
	} {
		err := socketErr("enp0s3", c.err)
		if got := errors.Is(err, ErrPermission); got != c.permitted {
			t.Errorf("%s: errors.Is(err, ErrPermission) = %v, want %v (err: %v)", c.name, got, c.permitted, err)
		}
		if !strings.Contains(err.Error(), "enp0s3") {
			t.Errorf("%s: error %q does not name the interface", c.name, err)
		}
	}
}
