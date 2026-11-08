//go:build darwin || windows

package pcap

import (
	"errors"
	"strings"
	"testing"
)

func TestPcapLink(t *testing.T) {
	for _, c := range []struct {
		name string
		dlt  int32
		want LinkType
		ok   bool
	}{
		{"ethernet", dltEn10MB, LinkEthernet, true},
		{"loopback", dltNull, LinkNull, true},
		{"raw IP", dltRaw, LinkRaw, true},
		{"802.11", 105, 0, false},
	} {
		got, err := pcapLink(c.dlt)
		if !c.ok {
			if err == nil {
				t.Errorf("%s: link type %d was accepted as %v", c.name, c.dlt, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: link type %d: %v", c.name, c.dlt, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: link type %d is %v, want %v", c.name, c.dlt, got, c.want)
		}
	}
}

func TestErrText(t *testing.T) {
	buf := make([]byte, errBufLen)
	copy(buf, "en0: No such device exists")
	for _, c := range []struct {
		name string
		buf  []byte
		want string
	}{
		{"terminated", buf, "en0: No such device exists"},
		{"empty", make([]byte, errBufLen), ""},
		{"no terminator", []byte("short"), "short"},
	} {
		if got := errText(c.buf); got != c.want {
			t.Errorf("%s: errText = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestGoString(t *testing.T) {
	cstr := []byte("en0\x00rest of the buffer")
	if got := goString(&cstr[0]); got != "en0" {
		t.Errorf("goString = %q, want %q", got, "en0")
	}
	if got := goString(nil); got != "" {
		t.Errorf("goString(nil) = %q, want the empty string", got)
	}
}

func TestOpenErr(t *testing.T) {
	for _, c := range []struct {
		name      string
		text      string
		permitted bool
		detail    string
	}{
		{
			name:      "bpf not readable",
			text:      "en0: You don't have permission to perform this capture on that device",
			permitted: true,
			detail:    "permission",
		},
		{
			name:      "not permitted",
			text:      "en0: Operation not permitted",
			permitted: true,
			detail:    "not permitted",
		},
		{
			name:   "no such device",
			text:   "en0: No such device exists",
			detail: "No such device exists",
		},
		{
			name:   "empty",
			text:   "  ",
			detail: "no reason",
		},
	} {
		err := openErr("en0", c.text)
		if got := errors.Is(err, ErrPermission); got != c.permitted {
			t.Errorf("%s: errors.Is(err, ErrPermission) = %v, want %v (err: %v)", c.name, got, c.permitted, err)
		}
		if !strings.Contains(err.Error(), c.detail) {
			t.Errorf("%s: error %q does not mention %q", c.name, err, c.detail)
		}
	}
}

func TestReadErr(t *testing.T) {
	err := readErr("en0", -1)
	if !strings.Contains(err.Error(), "en0") || !strings.Contains(err.Error(), "-1") {
		t.Errorf("readErr = %q, want the device and the return value in it", err)
	}
}
