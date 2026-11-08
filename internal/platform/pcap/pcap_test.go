//go:build darwin || linux || windows

package pcap

import "testing"

func TestPickDefault(t *testing.T) {
	for _, c := range []struct {
		name   string
		ifaces []Interface
		want   string
		ok     bool
	}{
		{
			name:   "up and not loopback",
			ifaces: []Interface{{Name: "lo0", Up: true, Loopback: true}, {Name: "en0", Up: true}},
			want:   "en0",
			ok:     true,
		},
		{
			name:   "down interfaces only",
			ifaces: []Interface{{Name: "lo0", Loopback: true}, {Name: "en5"}},
			want:   "en5",
			ok:     true,
		},
		{
			name:   "nothing but loopback",
			ifaces: []Interface{{Name: "lo0", Up: true, Loopback: true}},
			want:   "lo0",
			ok:     true,
		},
		{"no interfaces", nil, "", false},
	} {
		got, ok := pickDefault(c.ifaces)
		if ok != c.ok || got != c.want {
			t.Errorf("%s: pickDefault = %q, %v, want %q, %v", c.name, got, ok, c.want, c.ok)
		}
	}
}
