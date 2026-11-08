package sysproxy

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestParseHostPort(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		hostport string
		wantHost string
		wantPort int
		wantErr  bool
	}{
		{name: "loopback", hostport: "127.0.0.1:8080", wantHost: "127.0.0.1", wantPort: 8080},
		{name: "name", hostport: "localhost:1", wantHost: "localhost", wantPort: 1},
		{name: "highest port", hostport: "127.0.0.1:65535", wantHost: "127.0.0.1", wantPort: 65535},
		{name: "ipv6", hostport: "[::1]:8080", wantHost: "::1", wantPort: 8080},
		{name: "no port", hostport: "127.0.0.1", wantErr: true},
		{name: "no host", hostport: ":8080", wantErr: true},
		{name: "zero port", hostport: "127.0.0.1:0", wantErr: true},
		{name: "port too big", hostport: "127.0.0.1:65536", wantErr: true},
		{name: "not a port", hostport: "127.0.0.1:http", wantErr: true},
		{name: "empty", hostport: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			host, port, err := parseHostPort(tt.hostport)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseHostPort(%q) error = %v, want error %v", tt.hostport, err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if host != tt.wantHost || port != tt.wantPort {
				t.Fatalf("parseHostPort(%q) = %q, %d, want %q, %d", tt.hostport, host, port, tt.wantHost, tt.wantPort)
			}
		})
	}
}

func TestSameProxy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		host string
		port int
		want bool
	}{
		{name: "same", host: "localhost", port: 8080, want: true},
		{name: "case", host: "LocalHost", port: 8080, want: true},
		{name: "padded", host: " localhost ", port: 8080, want: true},
		{name: "other port", host: "localhost", port: 8081, want: false},
		{name: "other host", host: "example.test", port: 8080, want: false},
		{name: "empty", host: "", port: 0, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := sameProxy(tt.host, tt.port, "localhost", 8080); got != tt.want {
				t.Fatalf("sameProxy(%q, %d) = %v, want %v", tt.host, tt.port, got, tt.want)
			}
		})
	}
}

func TestProxyServerPoints(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		server string
		want   bool
	}{
		{name: "plain", server: "127.0.0.1:8080", want: true},
		{name: "padded", server: " 127.0.0.1:8080 ", want: true},
		{name: "other port", server: "127.0.0.1:8081", want: false},
		{name: "no port", server: "127.0.0.1", want: false},
		{name: "empty", server: "", want: false},
		{name: "per scheme same", server: "http=127.0.0.1:8080;https=127.0.0.1:8080", want: true},
		{name: "per scheme with socks", server: "http=127.0.0.1:8080;https=127.0.0.1:8080;socks=127.0.0.1:1080", want: true},
		{name: "per scheme split", server: "http=127.0.0.1:8080;https=127.0.0.1:8443", want: true},
		{name: "https only", server: "https=127.0.0.1:8080", want: true},
		{name: "socks only", server: "socks=127.0.0.1:8080", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := proxyServerPoints(tt.server, "127.0.0.1", 8080); got != tt.want {
				t.Fatalf("proxyServerPoints(%q) = %v, want %v", tt.server, got, tt.want)
			}
		})
	}
}

func TestParseGSettingsString(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		out  string
		want string
	}{
		{name: "mode", out: "'none'\n", want: "none"},
		{name: "empty", out: "''\n", want: ""},
		{name: "host", out: "'127.0.0.1'\n", want: "127.0.0.1"},
		{name: "unquoted", out: "manual\n", want: "manual"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseGSettingsString(tt.out); got != tt.want {
				t.Fatalf("parseGSettingsString(%q) = %q, want %q", tt.out, got, tt.want)
			}
		})
	}
}

func TestParseGSettingsInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		out     string
		want    int
		wantErr bool
	}{
		{name: "port", out: "8080\n", want: 8080},
		{name: "zero", out: "0\n", want: 0},
		{name: "typed", out: "uint32 8080\n", want: 8080},
		{name: "empty", out: "  \n", wantErr: true},
		{name: "text", out: "'8080'\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseGSettingsInt(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseGSettingsInt(%q) error = %v, want error %v", tt.out, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("parseGSettingsInt(%q) = %d, want %d", tt.out, got, tt.want)
			}
		})
	}
}

func TestGnomeEnableCommands(t *testing.T) {
	t.Parallel()
	want := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "none"},
		{"gsettings", "set", "org.gnome.system.proxy.http", "host", "127.0.0.1"},
		{"gsettings", "set", "org.gnome.system.proxy.http", "port", "8080"},
		{"gsettings", "set", "org.gnome.system.proxy.https", "host", "127.0.0.1"},
		{"gsettings", "set", "org.gnome.system.proxy.https", "port", "8080"},
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "manual"},
	}
	if got := gnomeEnableCommands("127.0.0.1", 8080); !reflect.DeepEqual(got, want) {
		t.Fatalf("gnomeEnableCommands = %v, want %v", got, want)
	}
}

func TestGnomeRestoreCommands(t *testing.T) {
	t.Parallel()
	s := GNOMESettings{
		Mode:  "auto",
		HTTP:  GNOMEProxy{Host: "proxy.example", Port: 3128},
		HTTPS: GNOMEProxy{},
	}
	want := [][]string{
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "none"},
		{"gsettings", "set", "org.gnome.system.proxy.http", "host", "proxy.example"},
		{"gsettings", "set", "org.gnome.system.proxy.http", "port", "3128"},
		{"gsettings", "set", "org.gnome.system.proxy.https", "host", ""},
		{"gsettings", "set", "org.gnome.system.proxy.https", "port", "0"},
		{"gsettings", "set", "org.gnome.system.proxy", "mode", "auto"},
	}
	if got := gnomeRestoreCommands(s); !reflect.DeepEqual(got, want) {
		t.Fatalf("gnomeRestoreCommands = %v, want %v", got, want)
	}
}

func TestGnomeRestoreCommandsWithoutMode(t *testing.T) {
	t.Parallel()
	commands := gnomeRestoreCommands(GNOMESettings{})
	for _, cmd := range commands {
		if cmd[3] == "mode" && cmd[4] != noneMode {
			t.Fatalf("gnomeRestoreCommands set mode %q without a recorded mode", cmd[4])
		}
	}
}

func TestWindowsSettingsPoint(t *testing.T) {
	t.Parallel()
	on := DWORD{Present: true, Number: 1}
	tests := []struct {
		name     string
		settings WindowsSettings
		want     bool
	}{
		{name: "plain", settings: WindowsSettings{ProxyEnable: on, ProxyServer: StringValue{Present: true, Text: "127.0.0.1:8080"}}, want: true},
		{name: "disabled", settings: WindowsSettings{ProxyEnable: DWORD{Present: true}, ProxyServer: StringValue{Present: true, Text: "127.0.0.1:8080"}}},
		{name: "absent", settings: WindowsSettings{ProxyServer: StringValue{Present: true, Text: "127.0.0.1:8080"}}},
		{name: "other server", settings: WindowsSettings{ProxyEnable: on, ProxyServer: StringValue{Present: true, Text: "127.0.0.1:9090"}}},
		{
			name: "per scheme same",
			settings: WindowsSettings{ProxyEnable: on, ProxyServer: StringValue{
				Present: true, Text: "http=127.0.0.1:8080;https=127.0.0.1:8080",
			}},
			want: true,
		},
		{
			name: "per scheme only http",
			settings: WindowsSettings{ProxyEnable: on, ProxyServer: StringValue{
				Present: true, Text: "http=127.0.0.1:8080;https=proxy.person:9090",
			}},
			want: true,
		},
		{
			name: "per scheme only https",
			settings: WindowsSettings{ProxyEnable: on, ProxyServer: StringValue{
				Present: true, Text: "http=127.0.0.1:9090;https=127.0.0.1:8080",
			}},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := windowsSettingsPoint(tt.settings, "127.0.0.1", 8080); got != tt.want {
				t.Fatalf("windowsSettingsPoint(%+v) = %v, want %v", tt.settings, got, tt.want)
			}
		})
	}
}

func TestGnomeSettingsPoint(t *testing.T) {
	t.Parallel()
	same := GNOMESettings{
		Mode:  manualMode,
		HTTP:  GNOMEProxy{Host: "127.0.0.1", Port: 8080},
		HTTPS: GNOMEProxy{Host: "127.0.0.1", Port: 8080},
	}
	tests := []struct {
		name     string
		settings GNOMESettings
		want     bool
	}{
		{name: "manual", settings: same, want: true},
		{name: "auto", settings: GNOMESettings{Mode: "auto", HTTP: same.HTTP, HTTPS: same.HTTPS}},
		{name: "only https", settings: GNOMESettings{Mode: manualMode, HTTP: GNOMEProxy{Host: "proxy.person", Port: 1}, HTTPS: same.HTTPS}, want: true},
		{name: "only http", settings: GNOMESettings{Mode: manualMode, HTTP: same.HTTP}, want: true},
		{name: "nothing points", settings: GNOMESettings{Mode: manualMode, HTTP: GNOMEProxy{Host: "proxy.person", Port: 1}, HTTPS: GNOMEProxy{Host: "proxy.person", Port: 2}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := gnomeSettingsPoint(tt.settings, "127.0.0.1", 8080); got != tt.want {
				t.Fatalf("gnomeSettingsPoint(%+v) = %v, want %v", tt.settings, got, tt.want)
			}
		})
	}
}

func TestCommandFailed(t *testing.T) {
	t.Parallel()
	base := errors.New("exit status 1")
	tests := []struct {
		name     string
		out      string
		wantPerm bool
	}{
		{name: "root", out: "** Error: You must be root to run this command.", wantPerm: true},
		{name: "permission", out: "permission denied", wantPerm: true},
		{name: "authorization", out: "** Error: Authorization required.", wantPerm: true},
		{name: "unauthorized", out: "error: unauthorized", wantPerm: true},
		{name: "not allowed", out: "not allowed to change this", wantPerm: true},
		{name: "ordinary", out: "** Error: 5 is not a valid port number."},
		{name: "silent", out: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := commandFailed("networksetup -setwebproxy", tt.out, base)
			if got := errors.Is(err, ErrPermission); got != tt.wantPerm {
				t.Fatalf("commandFailed(%q) permission = %v, want %v (err %v)", tt.out, got, tt.wantPerm, err)
			}
			if !tt.wantPerm && !errors.Is(err, base) {
				t.Fatalf("commandFailed(%q) = %v, want it to wrap the command error", tt.out, err)
			}
		})
	}
}

func TestSnapshotJSONRoundTrip(t *testing.T) {
	t.Parallel()
	original := Snapshot{
		Services: []Service{{
			Name:  "Wi-Fi",
			HTTP:  Proxy{Enabled: true, Host: "proxy.example", Port: 3128},
			HTTPS: Proxy{Enabled: false, Host: "proxy.example", Port: 3128},
			Auto:  AutoProxy{Enabled: true, URL: "http://pac.example/proxy.pac"},
		}},
		Windows: &WindowsSettings{
			ProxyEnable:   DWORD{Present: true, Number: 1},
			ProxyServer:   StringValue{Present: true, Text: "127.0.0.1:8080"},
			ProxyOverride: StringValue{Present: false},
			AutoConfigURL: StringValue{Present: true, Text: "http://pac.example/proxy.pac"},
		},
		GNOME: &GNOMESettings{
			Mode:  manualMode,
			HTTP:  GNOMEProxy{Host: "127.0.0.1", Port: 8080},
			HTTPS: GNOMEProxy{Host: "127.0.0.1", Port: 8080},
		},
	}
	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var restored Snapshot
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if !reflect.DeepEqual(restored, original) {
		t.Fatalf("round trip changed the snapshot:\n got %+v\nwant %+v\nJSON %s", restored, original, data)
	}
}
