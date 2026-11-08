//go:build darwin

package sysproxy

import (
	"errors"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// fakeRunner answers with canned output and records every call, so a
// command sequence can be checked without changing the machine.
type fakeRunner struct {
	outputs  map[string]string
	failures map[string]error
	calls    []string
}

func newFake(outputs map[string]string) *fakeRunner {
	if outputs == nil {
		outputs = map[string]string{}
	}
	return &fakeRunner{outputs: outputs, failures: map[string]error{}}
}

func (f *fakeRunner) run(_ string, args ...string) (string, error) {
	key := strings.Join(args, " ")
	f.calls = append(f.calls, key)
	if err, ok := f.failures[key]; ok {
		return f.outputs[key], err
	}
	return f.outputs[key], nil
}

// servicesOutput has two live services and one disabled one.
const servicesOutput = `An asterisk (*) denotes that a network service is disabled.
Ethernet
*Thunderbolt Bridge
Wi-Fi
`

func TestParseServices(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		out  string
		want []string
	}{
		{name: "captured", out: servicesOutput, want: []string{"Ethernet", "Wi-Fi"}},
		{name: "names only", out: "Ethernet\nWi-Fi\n", want: []string{"Ethernet", "Wi-Fi"}},
		{name: "empty", out: "", want: nil},
		{name: "legend only", out: "An asterisk (*) denotes that a network service is disabled.\n", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := parseServices(tt.out); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parseServices(%q) = %v, want %v", tt.out, got, tt.want)
			}
		})
	}
}

func TestParseProxy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		out     string
		want    Proxy
		wantErr bool
	}{
		{
			name: "enabled",
			out:  "Enabled: Yes\nServer: 127.0.0.1\nPort: 8080\nAuthenticated Proxy Enabled: 0\n",
			want: Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080},
		},
		{
			name: "disabled without server",
			out:  "Enabled: No\nServer: \nPort: 0\nAuthenticated Proxy Enabled: 0\n",
			want: Proxy{},
		},
		{name: "bad port", out: "Enabled: Yes\nServer: 127.0.0.1\nPort: http\n", wantErr: true},
		{name: "no enabled", out: "Server: 127.0.0.1\nPort: 8080\n", wantErr: true},
		{name: "bad enabled", out: "Enabled: maybe\nPort: 0\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseProxy(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseProxy(%q) error = %v, want error %v", tt.out, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("parseProxy(%q) = %+v, want %+v", tt.out, got, tt.want)
			}
		})
	}
}

func TestParseAutoProxy(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		out     string
		want    AutoProxy
		wantErr bool
	}{
		{name: "captured null", out: "URL: (null)\nEnabled: No\n", want: AutoProxy{}},
		{
			name: "enabled",
			out:  "URL: http://pac.example/proxy.pac\nEnabled: Yes\n",
			want: AutoProxy{Enabled: true, URL: "http://pac.example/proxy.pac"},
		},
		{name: "bad enabled", out: "URL: (null)\nEnabled: 0\n", wantErr: true},
		{name: "no enabled", out: "URL: (null)\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseAutoProxy(tt.out)
			if (err != nil) != tt.wantErr {
				t.Fatalf("parseAutoProxy(%q) error = %v, want error %v", tt.out, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Fatalf("parseAutoProxy(%q) = %+v, want %+v", tt.out, got, tt.want)
			}
		})
	}
}

// serviceOutputs is the canned answer of one service's three queries.
func serviceOutputs(name string, p Proxy) map[string]string {
	return map[string]string{
		"-getwebproxy " + name:       proxyOutput(p),
		"-getsecurewebproxy " + name: proxyOutput(p),
		"-getautoproxyurl " + name:   autoProxyOutput(AutoProxy{}),
	}
}

func proxyOutput(p Proxy) string {
	enabled := "No"
	if p.Enabled {
		enabled = "Yes"
	}
	return "Enabled: " + enabled + "\nServer: " + p.Host + "\nPort: " + strconv.Itoa(p.Port) + "\n"
}

func autoProxyOutput(a AutoProxy) string {
	enabled := "No"
	if a.Enabled {
		enabled = "Yes"
	}
	url := a.URL
	if url == "" {
		url = nullURL
	}
	return "URL: " + url + "\nEnabled: " + enabled + "\n"
}

func TestReadServices(t *testing.T) {
	t.Parallel()
	fake := newFake(map[string]string{"-listallnetworkservices": servicesOutput})
	fake.outputs["-getwebproxy Ethernet"] = "Enabled: No\nServer: \nPort: 0\n"
	fake.outputs["-getsecurewebproxy Ethernet"] = "Enabled: No\nServer: \nPort: 0\n"
	fake.outputs["-getautoproxyurl Ethernet"] = autoProxyOutput(AutoProxy{})
	fake.outputs["-getwebproxy Wi-Fi"] = proxyOutput(Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080})
	fake.outputs["-getsecurewebproxy Wi-Fi"] = proxyOutput(Proxy{Enabled: false, Host: "127.0.0.1", Port: 8080})
	fake.outputs["-getautoproxyurl Wi-Fi"] = autoProxyOutput(AutoProxy{Enabled: true, URL: "http://pac.example/proxy.pac"})

	snap, err := readServices(fake.run)
	if err != nil {
		t.Fatalf("readServices: %v", err)
	}
	want := Snapshot{Services: []Service{
		{Name: "Ethernet"},
		{
			Name:  "Wi-Fi",
			HTTP:  Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080},
			HTTPS: Proxy{Host: "127.0.0.1", Port: 8080},
			Auto:  AutoProxy{Enabled: true, URL: "http://pac.example/proxy.pac"},
		},
	}}
	if !reflect.DeepEqual(snap, want) {
		t.Fatalf("readServices = %+v, want %+v", snap, want)
	}
	wantCalls := []string{
		"-listallnetworkservices",
		"-getwebproxy Ethernet", "-getsecurewebproxy Ethernet", "-getautoproxyurl Ethernet",
		"-getwebproxy Wi-Fi", "-getsecurewebproxy Wi-Fi", "-getautoproxyurl Wi-Fi",
	}
	if !reflect.DeepEqual(fake.calls, wantCalls) {
		t.Fatalf("readServices ran %v, want %v", fake.calls, wantCalls)
	}
}

func TestEnableServices(t *testing.T) {
	t.Parallel()
	fake := newFake(map[string]string{"-listallnetworkservices": servicesOutput})
	for key, out := range serviceOutputs("Ethernet", Proxy{}) {
		fake.outputs[key] = out
	}
	for key, out := range serviceOutputs("Wi-Fi", Proxy{Enabled: true, Host: "proxy.old", Port: 3128}) {
		fake.outputs[key] = out
	}

	snap, err := enableServices(fake.run, "127.0.0.1:8080")
	if err != nil {
		t.Fatalf("enableServices: %v", err)
	}
	want := Snapshot{Services: []Service{
		{Name: "Ethernet"},
		{Name: "Wi-Fi", HTTP: Proxy{Enabled: true, Host: "proxy.old", Port: 3128}, HTTPS: Proxy{Enabled: true, Host: "proxy.old", Port: 3128}},
	}}
	if !reflect.DeepEqual(snap, want) {
		t.Fatalf("enableServices returned %+v, want the settings it replaced %+v", snap, want)
	}
	wantCalls := []string{
		"-listallnetworkservices",
		"-getwebproxy Ethernet", "-getsecurewebproxy Ethernet", "-getautoproxyurl Ethernet",
		"-getwebproxy Wi-Fi", "-getsecurewebproxy Wi-Fi", "-getautoproxyurl Wi-Fi",
		"-setautoproxystate Ethernet off",
		"-setwebproxy Ethernet 127.0.0.1 8080",
		"-setsecurewebproxy Ethernet 127.0.0.1 8080",
		"-setwebproxystate Ethernet on",
		"-setsecurewebproxystate Ethernet on",
		"-setautoproxystate Wi-Fi off",
		"-setwebproxy Wi-Fi 127.0.0.1 8080",
		"-setsecurewebproxy Wi-Fi 127.0.0.1 8080",
		"-setwebproxystate Wi-Fi on",
		"-setsecurewebproxystate Wi-Fi on",
	}
	if !reflect.DeepEqual(fake.calls, wantCalls) {
		t.Fatalf("enableServices ran:\n%v\nwant:\n%v", fake.calls, wantCalls)
	}
}

func TestRestoreServices(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	snap := Snapshot{Services: []Service{{
		Name:  "Wi-Fi",
		HTTP:  Proxy{Enabled: true, Host: "proxy.example", Port: 3128},
		HTTPS: Proxy{Enabled: false, Host: "proxy.example", Port: 3128},
		Auto:  AutoProxy{Enabled: true, URL: "http://pac.example/proxy.pac"},
	}}}
	if err := restoreServices(fake.run, snap); err != nil {
		t.Fatalf("restoreServices: %v", err)
	}
	wantCalls := []string{
		"-setwebproxy Wi-Fi proxy.example 3128",
		"-setwebproxystate Wi-Fi on",
		"-setsecurewebproxy Wi-Fi proxy.example 3128",
		"-setsecurewebproxystate Wi-Fi off",
		"-setautoproxyurl Wi-Fi http://pac.example/proxy.pac",
		"-setautoproxystate Wi-Fi on",
	}
	if !reflect.DeepEqual(fake.calls, wantCalls) {
		t.Fatalf("restoreServices ran:\n%v\nwant:\n%v", fake.calls, wantCalls)
	}
}

func TestRestoreServicesWithoutConfiguredProxy(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	if err := restoreServices(fake.run, Snapshot{Services: []Service{{Name: "Wi-Fi"}}}); err != nil {
		t.Fatalf("restoreServices: %v", err)
	}
	if want := emptyRestoreCalls("Wi-Fi"); !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("restoreServices ran %v, want %v", fake.calls, want)
	}
}

func TestEnableServicesRollsBack(t *testing.T) {
	t.Parallel()
	fake := newFake(map[string]string{"-listallnetworkservices": servicesOutput})
	for key, out := range serviceOutputs("Ethernet", Proxy{}) {
		fake.outputs[key] = out
	}
	for key, out := range serviceOutputs("Wi-Fi", Proxy{Enabled: true, Host: "proxy.old", Port: 3128}) {
		fake.outputs[key] = out
	}
	failure := errors.New("exit status 1")
	fake.failures["-setsecurewebproxy Wi-Fi 127.0.0.1 8080"] = failure
	fake.outputs["-setsecurewebproxy Wi-Fi 127.0.0.1 8080"] = "** Error: not a valid proxy."

	snap, err := enableServices(fake.run, "127.0.0.1:8080")
	if !errors.Is(err, failure) {
		t.Fatalf("enableServices error = %v, want it to wrap %v", err, failure)
	}
	if errors.Is(err, ErrPermission) {
		t.Fatalf("enableServices reported a permission failure for %v", err)
	}
	if len(snap.Services) != 2 {
		t.Fatalf("enableServices returned %+v on failure, want the snapshot", snap)
	}
	wantTail := append(emptyRestoreCalls("Ethernet"), endpointRestoreCalls("Wi-Fi")...)
	if len(fake.calls) < len(wantTail) {
		t.Fatalf("enableServices ran %v, want a rollback after the failure", fake.calls)
	}
	if tail := fake.calls[len(fake.calls)-len(wantTail):]; !reflect.DeepEqual(tail, wantTail) {
		t.Fatalf("rollback ran %v, want %v", tail, wantTail)
	}
}

func TestEnableServicesPermission(t *testing.T) {
	t.Parallel()
	fake := newFake(map[string]string{"-listallnetworkservices": servicesOutput})
	for key, out := range serviceOutputs("Ethernet", Proxy{}) {
		fake.outputs[key] = out
	}
	for key, out := range serviceOutputs("Wi-Fi", Proxy{}) {
		fake.outputs[key] = out
	}
	fake.failures["-setwebproxystate Wi-Fi on"] = errors.New("exit status 1")
	fake.outputs["-setwebproxystate Wi-Fi on"] = "** Error: You must be root to run this command."

	if _, err := enableServices(fake.run, "127.0.0.1:8080"); !errors.Is(err, ErrPermission) {
		t.Fatalf("enableServices error = %v, want ErrPermission", err)
	}
}

func TestPointsAt(t *testing.T) {
	t.Parallel()
	full := Snapshot{Services: []Service{{
		Name:  "Wi-Fi",
		HTTP:  Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080},
		HTTPS: Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080},
	}}}
	tests := []struct {
		name     string
		snap     Snapshot
		hostport string
		want     bool
	}{
		{name: "points", snap: full, hostport: "127.0.0.1:8080", want: true},
		{name: "name is not resolved", snap: full, hostport: "localhost:8080", want: false},
		{name: "case is ignored", snap: Snapshot{Services: []Service{{
			Name:  "Wi-Fi",
			HTTP:  Proxy{Enabled: true, Host: "LocalHost", Port: 8080},
			HTTPS: Proxy{Enabled: true, Host: "localhost", Port: 8080},
		}}}, hostport: "localhost:8080", want: true},
		{name: "only https points", snap: Snapshot{Services: []Service{{
			Name:  "Wi-Fi",
			HTTP:  Proxy{Host: "127.0.0.1", Port: 8080},
			HTTPS: Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080},
		}}}, hostport: "127.0.0.1:8080", want: true},
		{name: "only http points", snap: Snapshot{Services: []Service{{
			Name:  "Wi-Fi",
			HTTP:  Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080},
			HTTPS: Proxy{Enabled: true, Host: "127.0.0.1", Port: 8443},
		}}}, hostport: "127.0.0.1:8080", want: true},
		{name: "one part in a later service", snap: Snapshot{Services: []Service{
			{HTTP: Proxy{Enabled: true, Host: "proxy.person", Port: 9090}},
			{HTTP: Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080}},
		}}, hostport: "127.0.0.1:8080", want: true},
		{name: "nothing points", snap: Snapshot{Services: []Service{
			{HTTP: Proxy{Enabled: true, Host: "proxy.person", Port: 9090}, HTTPS: Proxy{Enabled: true, Host: "127.0.0.1", Port: 8443}},
		}}, hostport: "127.0.0.1:8080", want: false},
		{name: "no services", snap: Snapshot{}, hostport: "127.0.0.1:8080", want: false},
		{name: "bad address", snap: full, hostport: "127.0.0.1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := pointsAt(tt.snap, tt.hostport); got != tt.want {
				t.Fatalf("pointsAt(%+v, %q) = %v, want %v", tt.snap, tt.hostport, got, tt.want)
			}
		})
	}
}

// TestCurrentReal reads the machine's settings. It only runs the read-only
// networksetup getters, and it prints what it saw because the output is
// what the tool shows before it changes anything.
func TestCurrentReal(t *testing.T) {
	if _, err := os.Stat(networksetupPath); err != nil {
		t.Skipf("networksetup is not installed: %v", err)
	}
	snap, err := Current()
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	t.Logf("snapshot: %+v", snap)
	for _, svc := range snap.Services {
		t.Logf("service %q: http=%+v https=%+v auto=%+v", svc.Name, svc.HTTP, svc.HTTPS, svc.Auto)
	}
}

// serviceNow makes the fake report p for both proxies of a service, with no
// PAC URL, which is what the package writes when it enables its own proxy.
func serviceNow(fake *fakeRunner, name string, p Proxy) {
	for key, out := range serviceOutputs(name, p) {
		fake.outputs[key] = out
	}
}

// restorable is a service with an old proxy that Restore would put back.
func restorable(name string) Service {
	return Service{
		Name:  name,
		HTTP:  Proxy{Enabled: true, Host: "proxy.old", Port: 3128},
		HTTPS: Proxy{Enabled: true, Host: "proxy.old", Port: 3128},
	}
}

func TestRestoreServicesSkipsGoneService(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	fake.failures["-setwebproxy USB LAN proxy.old 3128"] = errors.New("exit status 8")
	fake.outputs["-setwebproxy USB LAN proxy.old 3128"] = "** Error: Unable to find item in network database."
	snap := Snapshot{Services: []Service{restorable("Ethernet"), restorable("USB LAN"), restorable("Wi-Fi")}}

	if err := restoreServices(fake.run, snap); err != nil {
		t.Fatalf("restoreServices failed because one service is gone: %v", err)
	}
	wantCalls := append(endpointRestoreCalls("Ethernet"), "-setwebproxy USB LAN proxy.old 3128")
	wantCalls = append(wantCalls, endpointRestoreCalls("Wi-Fi")...)
	if !reflect.DeepEqual(fake.calls, wantCalls) {
		t.Fatalf("restoreServices ran:\n%v\nwant:\n%v", fake.calls, wantCalls)
	}
}

func TestRestorePointingOnlyRestoresWhatPointsHere(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	serviceNow(fake, "Ethernet", Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080})
	serviceNow(fake, "Wi-Fi", Proxy{Enabled: true, Host: "proxy.person", Port: 9090})
	snap := Snapshot{Services: []Service{restorable("Ethernet"), restorable("Wi-Fi")}}

	if err := restorePointingServices(fake.run, snap, "127.0.0.1:8080"); err != nil {
		t.Fatalf("restorePointingServices: %v", err)
	}
	wantCalls := []string{
		"-getwebproxy Ethernet", "-getsecurewebproxy Ethernet", "-getautoproxyurl Ethernet",
		"-setwebproxy Ethernet proxy.old 3128",
		"-setwebproxystate Ethernet on",
		"-setsecurewebproxy Ethernet proxy.old 3128",
		"-setsecurewebproxystate Ethernet on",
		"-setautoproxystate Ethernet off",
		"-getwebproxy Wi-Fi", "-getsecurewebproxy Wi-Fi", "-getautoproxyurl Wi-Fi",
	}
	if !reflect.DeepEqual(fake.calls, wantCalls) {
		t.Fatalf("restorePointingServices ran:\n%v\nwant only the pointing service restored:\n%v", fake.calls, wantCalls)
	}
}

func TestRestorePointingKeepsGoingAfterFailure(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	serviceNow(fake, "Ethernet", Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080})
	serviceNow(fake, "Wi-Fi", Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080})
	failure := errors.New("exit status 1")
	fake.failures["-setwebproxy Ethernet proxy.old 3128"] = failure
	fake.outputs["-setwebproxy Ethernet proxy.old 3128"] = "** Error: 3128 is not a valid port."
	snap := Snapshot{Services: []Service{restorable("Ethernet"), restorable("Wi-Fi")}}

	err := restorePointingServices(fake.run, snap, "127.0.0.1:8080")
	if !errors.Is(err, failure) {
		t.Fatalf("restorePointingServices error = %v, want it to wrap %v", err, failure)
	}
	wantTail := endpointRestoreCalls("Wi-Fi")
	if len(fake.calls) < len(wantTail) {
		t.Fatalf("restorePointingServices ran %v, want the second service restored", fake.calls)
	}
	if tail := fake.calls[len(fake.calls)-len(wantTail):]; !reflect.DeepEqual(tail, wantTail) {
		t.Fatalf("restorePointingServices repaired %v, want %v", tail, wantTail)
	}
}

func TestRestorePointingSkipsGoneService(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	serviceNow(fake, "Ethernet", Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080})
	fake.failures["-getwebproxy USB LAN"] = errors.New("exit status 8")
	fake.outputs["-getwebproxy USB LAN"] = "** Error: Unable to find item in network database."
	serviceNow(fake, "Wi-Fi", Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080})
	snap := Snapshot{Services: []Service{restorable("Ethernet"), restorable("USB LAN"), restorable("Wi-Fi")}}

	if err := restorePointingServices(fake.run, snap, "127.0.0.1:8080"); err != nil {
		t.Fatalf("restorePointingServices failed because one service is gone: %v", err)
	}
	tail := fake.calls[len(fake.calls)-len(endpointRestoreCalls("Wi-Fi")):]
	if !reflect.DeepEqual(tail, endpointRestoreCalls("Wi-Fi")) {
		t.Fatalf("restorePointingServices ran %v, want the last service restored", tail)
	}
}

// serviceOutputsSplit makes the fake answer a service's two proxies
// differently, which is what a person leaves behind by unticking one of the
// two proxy checkboxes.
func serviceOutputsSplit(fake *fakeRunner, name string, web, secure Proxy) {
	fake.outputs["-getwebproxy "+name] = proxyOutput(web)
	fake.outputs["-getsecurewebproxy "+name] = proxyOutput(secure)
	fake.outputs["-getautoproxyurl "+name] = autoProxyOutput(AutoProxy{})
}

// savedService is what Restore must put back: every field of the service.
func savedService(name string) Service {
	return Service{
		Name:  name,
		HTTP:  Proxy{Enabled: true, Host: "proxy.old", Port: 3128},
		HTTPS: Proxy{Enabled: true, Host: "proxy.old", Port: 3128},
		Auto:  AutoProxy{Enabled: true, URL: "http://pac.old/x.pac"},
	}
}

// restoreCalls is the command sequence that puts savedService back.
func restoreCalls(name string) []string {
	return []string{
		"-setwebproxy " + name + " proxy.old 3128",
		"-setwebproxystate " + name + " on",
		"-setsecurewebproxy " + name + " proxy.old 3128",
		"-setsecurewebproxystate " + name + " on",
		"-setautoproxyurl " + name + " http://pac.old/x.pac",
		"-setautoproxystate " + name + " on",
	}
}

func TestRestorePointingRestoresWhenOnlyWebPoints(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	serviceOutputsSplit(fake, "Ethernet", Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080}, Proxy{})
	if err := restorePointingServices(fake.run, Snapshot{Services: []Service{savedService("Ethernet")}}, "127.0.0.1:8080"); err != nil {
		t.Fatalf("restorePointingServices: %v", err)
	}
	want := append([]string{
		"-getwebproxy Ethernet", "-getsecurewebproxy Ethernet", "-getautoproxyurl Ethernet",
	}, restoreCalls("Ethernet")...)
	if !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("restorePointingServices ran:\n%v\nwant a service with only the web proxy at us restored:\n%v", fake.calls, want)
	}
}

func TestRestorePointingRestoresWhenOnlySecureWebPoints(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	serviceOutputsSplit(fake, "Wi-Fi", Proxy{Enabled: true, Host: "proxy.person", Port: 9090}, Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080})
	if err := restorePointingServices(fake.run, Snapshot{Services: []Service{savedService("Wi-Fi")}}, "127.0.0.1:8080"); err != nil {
		t.Fatalf("restorePointingServices: %v", err)
	}
	if len(fake.calls) < len(restoreCalls("Wi-Fi")) {
		t.Fatalf("restorePointingServices ran %v, want the whole service restored", fake.calls)
	}
	tail := fake.calls[len(fake.calls)-len(restoreCalls("Wi-Fi")):]
	if !reflect.DeepEqual(tail, restoreCalls("Wi-Fi")) {
		t.Fatalf("restorePointingServices ran %v, want the whole service restored", tail)
	}
}

func TestRestorePointingLeavesServiceWhenNoPartPoints(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	serviceOutputsSplit(fake, "Wi-Fi", Proxy{Enabled: true, Host: "proxy.person", Port: 9090}, Proxy{})
	if err := restorePointingServices(fake.run, Snapshot{Services: []Service{savedService("Wi-Fi")}}, "127.0.0.1:8080"); err != nil {
		t.Fatalf("restorePointingServices: %v", err)
	}
	for _, call := range fake.calls {
		if strings.HasPrefix(call, "-set") {
			t.Fatalf("restorePointingServices changed a service the person owns: %v", fake.calls)
		}
	}
}

// call is the key the fake records for one networksetup call, which keeps
// the expected sequences readable when an argument is empty.
func call(parts ...string) string { return strings.Join(parts, " ") }

// emptyRestoreCalls is what Restore must run for a service whose saved
// proxies and PAC URL were all empty, so no endpoint of ours is left in the
// disabled slot.
func emptyRestoreCalls(name string) []string {
	return []string{
		call("-setwebproxy", name, "", "0"),
		call("-setwebproxystate", name, "off"),
		call("-setsecurewebproxy", name, "", "0"),
		call("-setsecurewebproxystate", name, "off"),
		call("-setautoproxystate", name, "off"),
	}
}

// endpointRestoreCalls is what Restore runs for a service with saved
// proxies but no PAC URL.
func endpointRestoreCalls(name string) []string {
	return []string{
		call("-setwebproxy", name, "proxy.old", "3128"),
		call("-setwebproxystate", name, "on"),
		call("-setsecurewebproxy", name, "proxy.old", "3128"),
		call("-setsecurewebproxystate", name, "on"),
		call("-setautoproxystate", name, "off"),
	}
}

func TestRestoreServicesWritesEmptyEndpoint(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	if err := restoreServices(fake.run, Snapshot{Services: []Service{{Name: "Wi-Fi"}}}); err != nil {
		t.Fatalf("restoreServices: %v", err)
	}
	if want := emptyRestoreCalls("Wi-Fi"); !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("restoreServices ran:\n%v\nwant the empty endpoint written back:\n%v", fake.calls, want)
	}
}

func TestRestoreServicesMixedEndpoints(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	snap := Snapshot{Services: []Service{savedService("Ethernet"), {Name: "Wi-Fi"}}}
	if err := restoreServices(fake.run, snap); err != nil {
		t.Fatalf("restoreServices: %v", err)
	}
	want := append(restoreCalls("Ethernet"), emptyRestoreCalls("Wi-Fi")...)
	if !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("restoreServices ran:\n%v\nwant:\n%v", fake.calls, want)
	}
}

func TestRestorePointingWritesEmptyEndpoint(t *testing.T) {
	t.Parallel()
	fake := newFake(nil)
	serviceOutputsSplit(fake, "Wi-Fi", Proxy{Enabled: true, Host: "127.0.0.1", Port: 8080}, Proxy{})
	if err := restorePointingServices(fake.run, Snapshot{Services: []Service{{Name: "Wi-Fi"}}}, "127.0.0.1:8080"); err != nil {
		t.Fatalf("restorePointingServices: %v", err)
	}
	want := append([]string{
		"-getwebproxy Wi-Fi", "-getsecurewebproxy Wi-Fi", "-getautoproxyurl Wi-Fi",
	}, emptyRestoreCalls("Wi-Fi")...)
	if !reflect.DeepEqual(fake.calls, want) {
		t.Fatalf("restorePointingServices ran:\n%v\nwant:\n%v", fake.calls, want)
	}
}
