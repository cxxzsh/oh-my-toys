# Traffic: capture, inspect, rewrite

The Traffic tool (`traffic`, category network) records network traffic and lets a person look through it, rewrite it and send it again. It is a switch tool: Start and Stop begin and end the capture. The contract the UI and the tool share is `internal/tool/traffic.go` (`tool.Traffic`); this file fixes everything below it.

## 1. What it does

| Part | Behaviour |
|---|---|
| Proxy mode | A local HTTP(S) proxy on `127.0.0.1:<port>` (never another address). The system proxy points at it while it runs. HTTPS is opened with the app's own root certificate when `Decrypt` is on, and is one tunnel row when it is off or the host is in `Bypass`. HTTP/1.1 and HTTP/2 are decoded; a WebSocket upgrade is recorded as a request and then tunnelled. |
| Packet mode | Reads the packets of one interface and shows one row per packet with a layer tree and a hex dump. Decodes Ethernet, loopback, IPv4, IPv6, ARP, TCP, UDP, ICMP, DNS, the SNI of a TLS hello and the first line of plain HTTP/1. There is no TCP stream reassembly. |
| Inspector | A virtualised list of flows, a detail pane (headers, body formatted as JSON or text, raw), filters and search. |
| Rules | Rewrite, mock, block and delay, matched by method and URL pattern, applied in order. There are no interactive breakpoints. |
| Replay | Sends the request of a flow again through the same proxy and rules; the new flow records `ReplayOf`. |
| Export | HAR 1.2 of proxied flows. |
| Programs | Each proxied flow records the program that opened the connection (`FlowRow.Process`), found from the client's local port in the system's socket table (`internal/platform/procnet`). The list narrows to programs (`Filter.Processes`), and a capture can be scoped to programs (`CaptureConfig.Scope`): the traffic of any other program, or of one the system cannot name, passes through unopened and unlogged, and no rule applies to it. |
| Capture a program | A program that ignores the system proxy (`curl` without `-x`, most Go and Rust programs) is started from a terminal the tool opens (`OpenTerminal`) or with the variables it copies (`Environment`): `HTTP(S)_PROXY`, `ALL_PROXY`, `NO_PROXY`, and the certificate variables. `NODE_EXTRA_CA_CERTS` adds our root; `SSL_CERT_FILE`, `REQUESTS_CA_BUNDLE`, `CURL_CA_BUNDLE` and `GIT_SSL_CAINFO` replace the trust list, so they point at a bundle of the system's roots plus ours (`traffic/ca/bundle.pem`) and are left out where the system has no bundle file (Windows). |

Decisions that bound the work: loopback only; bodies are cut at `tool.MaxBody` (4 MiB); the proxy log keeps at most 5000 flows and 256 MiB of bodies, the packet log 50000 packets, and the oldest go first; the tool exists in the desktop app only (like Screenshot, it is registered in `cmd/oh-my-toys` and not in `omt`), so neither the command line nor MCP can start a capture or read one: a flow can hold secrets and a capture changes the system proxy.

Known limit (macOS): the system proxy is set on every network service, but apps read the settings of the primary service. When a proxy app's virtual interface is primary (observed with Surge's `utun`: `scutil --proxy` showed no proxy while Traffic was capturing), system-proxy-aware apps do not reach the listener. The client has to be pointed at `127.0.0.1:<port>` directly, or the other proxy app's system proxy turned off. The tool asks `sysproxy.Effective` when it takes the system proxy and says so in its note while that run lasts.

Not done: capturing a program that neither reads the system proxy nor is started from our terminal. That needs a system-wide takeover (a macOS Network Extension, which needs an Apple-granted entitlement, or a signed driver on Windows). Packet mode sees every program but cannot name it yet and does not decrypt.

## 2. Safety rules (every slice follows these)

1. **The system proxy must come back.** Before the app changes it, the old settings are saved to a state file; Stop, quitting and a crash recovery at the next start restore them. A proxy left pointing at a dead port cuts the person off the network.
2. **The root certificate's key is a high-value secret.** It is made per installation (ECDSA P-256), kept only on this machine with mode 0600 in a 0700 folder, has `pathLen=0`, is never logged or exported, and the person can remove the trust and delete the key from the UI.
3. **Leaf certificates** are made per host, valid at most 397 days, carry the host as a SAN (DNS or IP), and are cached.
4. **Nothing is elevated silently.** Anything that needs a password goes through the system's own prompt, with a reason in plain words, and a refusal returns `ErrPermission`.
5. The proxy listens on loopback only, and refuses a request whose target is the proxy itself (no loops).
6. Errors are lowercase English sentinels wrapped with detail; the UI chooses the message (CONTRIBUTING.md section 7).

## 3. Layout

```
internal/tool/traffic.go              contract (done): tool.Traffic, Flow, Rule, ...
internal/platform/sysproxy/           system proxy: Current, Enable, Restore
internal/platform/trust/              trust store: Install, Remove, Trusted
internal/platform/pcap/               packet source: Interfaces, Open, Grant
internal/platform/procnet/            owner of a TCP connection: Owner
internal/platform/terminal/           a terminal window with variables set: Open
internal/tools/traffic/ca/            root and leaf certificates
internal/tools/traffic/flowlog/       the log (implements tool.FlowLog)
internal/tools/traffic/rules/         the rule engine
internal/tools/traffic/proxy/         the MITM proxy, plus replay
internal/tools/traffic/har/           HAR export
internal/tools/traffic/packets/       packet decoding into flows
internal/tools/traffic/               the tool: manifest, wiring, persistence
internal/ui/comp/, internal/ui/shell/ the inspector page
```

Sub-packages of one tool may import each other and `internal/tool`; they import no other tool, no core and no UI. Platform packages import nothing of the app and nothing of each other (depguard), so each does its own elevation.

Per-OS code follows `internal/platform/capture`: a wrapper file with the exported API, shared types and sentinel errors; `<name>_darwin.go`, `<name>_windows.go`, `<name>_linux.go` per OS; a `//go:build !darwin && !windows && !linux` stub returning `ErrUnsupported`. `CGO_ENABLED=0` everywhere: purego on macOS and Linux, `syscall` / `golang.org/x/sys` on Windows. Linux code must compile and be sensible, but is not run-tested yet; mark it so in its package comment.

## 4. Platform APIs (frozen)

### `internal/platform/sysproxy`

```go
// Snapshot is the system proxy settings before the app changed them. It
// is plain data that marshals to JSON, so it can be kept in a state file.
type Snapshot struct { /* per-OS fields, all exported */ }

func Current() (Snapshot, error)
// Enable points the HTTP and HTTPS proxy of the system (on macOS, of every
// network service that is up) at hostport and returns the settings it
// replaced.
func Enable(hostport string) (Snapshot, error)
func Restore(Snapshot) error
// Points reports whether the snapshot's proxy is hostport, so a leftover
// state can tell its own change from one the person made since.
func Points(s Snapshot, hostport string) bool
// RestorePointing restores, of the services (or settings) in the snapshot,
// only those that still point at hostport, and leaves the rest as the
// person has them now. It goes through every one even when one fails (a
// service that is gone is skipped) and returns the errors joined. It is what
// crash recovery uses.
func RestorePointing(s Snapshot, hostport string) error
// Effective reports whether the proxy apps read points at hostport: on
// macOS the setting of the primary network service, which is not always
// one Enable changed. Windows and Linux have one setting apps read, the
// one Enable writes, so they report true.
func Effective(hostport string) (bool, error)

var ErrPermission, ErrUnsupported error
```

macOS: `networksetup` per service. Windows: the Internet Settings registry key plus `InternetSetOption` (`INTERNET_OPTION_SETTINGS_CHANGED`, `INTERNET_OPTION_REFRESH`). Linux: GNOME `gsettings org.gnome.system.proxy`; other desktops return `ErrUnsupported`.

### `internal/platform/trust`

```go
// Install makes the system trust certPEM as a root for TLS servers.
func Install(certPEM []byte) error
func Remove(certPEM []byte) error
func Trusted(certPEM []byte) (bool, error)

var ErrPermission, ErrUnsupported error
```

macOS: `security add-trusted-cert` into the login keychain with user trust (the system asks for the person's approval), `security remove-trusted-cert` / `delete-certificate`, and `security verify-cert` or `find-certificate` for `Trusted`. Windows: the current user's Root store, through `certutil -user` or the CryptoAPI via `syscall`. Linux: the NSS database of Chrome and Firefox (`certutil -d sql:$HOME/.pki/nssdb`) when `certutil` exists, plus the system store through `pkexec` with `update-ca-certificates` or `update-ca-trust`; otherwise `ErrUnsupported` and the UI shows the PEM path.

### `internal/platform/pcap`

```go
type LinkType int // LinkEthernet, LinkNull (4-byte BSD family header, loopback), LinkRaw (starts at IP)

type Interface struct { Name, Description string; Up, Loopback bool }

type Packet struct {
	Time   time.Time
	Link   LinkType
	Data   []byte // the captured bytes, at most 65535
	Length int    // the packet's length on the wire
}

type Source interface {
	// Read blocks for the next packet; it returns io.EOF after Close.
	Read() (Packet, error)
	Close() error
}

func Interfaces() ([]Interface, error)
// DefaultInterface is the interface the system routes the internet through.
func DefaultInterface() (string, error)
func Open(name string) (Source, error)
// Grant asks the person, through the system's own prompt, to allow packet
// capture, where that is a thing (macOS: access to /dev/bpf*). reason is
// the sentence the prompt shows, already in the person's language, and it
// says exactly who gets the access and until when. The grant is as narrow
// as the system allows (macOS: the admin group, until the next restart).
func Grant(ctx context.Context, reason string) error

var ErrPermission, ErrDriverMissing, ErrUnsupported error
```

macOS: purego into the system `libpcap.dylib` (`pcap_open_live`, `pcap_next_ex`, `pcap_breakloop`, `pcap_findalldevs`), promiscuous off, a 100 ms read timeout so `Close` returns; `ErrPermission` when `/dev/bpf*` is not readable. Linux: `AF_PACKET` raw socket through `x/sys/unix`, `ErrPermission` without `CAP_NET_RAW`. Windows: `wpcap.dll` (Npcap) loaded with `syscall.LoadDLL`, `ErrDriverMissing` when it is absent.

### `internal/platform/procnet`

```go
type Process struct{ PID int; Name, Path string }
// Owner returns the process that owns the established TCP connection with
// these ends, both as that process sees them. For a client of the proxy,
// local is the client's address (the proxy's RemoteAddr), remote the proxy.
func Owner(local, remote netip.AddrPort) (Process, error)
var ErrNotFound, ErrUnsupported error
```

Name is the outermost `.app` bundle name without the suffix, or the executable's base name without `.exe`. macOS: purego into `libproc` (`proc_listallpids`, `proc_pidinfo(PROC_PIDLISTFDS)`, `proc_pidfdinfo(PROC_PIDFDSOCKETINFO)`, `proc_pidpath`). Linux: `/proc/net/tcp{,6}` to the socket inode, then `/proc/<pid>/fd`. Windows: `GetExtendedTcpTable` with `TCP_TABLE_OWNER_PID_ALL`. The proxy asks once per accepted connection and waits at most 200 ms.

### `internal/platform/terminal`

```go
type Var struct{ Name, Value string }
// Open opens a terminal window whose shell starts with vars set, and
// returns once it is open.
func Open(ctx context.Context, vars []Var) error
var ErrUnsupported error
```

macOS: a private `session.command` (mode 0700) that deletes itself, exports the variables and `exec`s the login shell, opened with `open -a Terminal`. Windows: a PowerShell window; values travel base64-encoded because `cmd.exe` would otherwise rewrite quotes and `%`. Linux: the first of a list of terminal emulators on `PATH`, when a display exists. Names are checked against `^[A-Za-z_][A-Za-z0-9_]*$`; values are quoted, never concatenated raw.

## 5. Tool-internal APIs (frozen)

### `ca`

```go
type Authority struct{ /* ... */ }
var ErrNotFound error
// Load reads the root from dir, or returns ErrNotFound.
func Load(dir string) (*Authority, error)
// Create makes a new root and writes it to dir (key 0600, dir 0700).
func Create(dir string) (*Authority, error)
func Remove(dir string) error
func (a *Authority) CertPEM() []byte
func (a *Authority) CertPath() string
func (a *Authority) Fingerprint() string // SHA-256, hex
func (a *Authority) NotAfter() time.Time
// Leaf returns a certificate for host (a name or an IP), signed by the root.
func (a *Authority) Leaf(host string) (*tls.Certificate, error)
```

### `flowlog`

```go
type Log struct{ /* ... */ }
func New(maxFlows int, maxBodyBytes int64) *Log
// Put records f. With f.ID == 0 it assigns the next ID and adds the flow;
// otherwise it replaces the flow with that ID, and does nothing when it has
// been evicted. It returns the ID. Safe for any goroutine.
func (l *Log) Put(f tool.Flow) uint64
// Log implements tool.FlowLog.
```

`Rows` is cached per (version, filter) so that asking again after no change costs nothing; matching on `Filter.Text` searches the URL, the header lines, text bodies and `Info`, case-insensitively.

### `rules`

```go
type Engine struct{ /* ... */ }
func New() *Engine
// Set validates the rules (patterns compile, status codes are sane) and
// swaps them in atomically.
func (e *Engine) Set(rules []tool.Rule) error
func (e *Engine) Get() []tool.Rule

type Verdict struct {
	Block bool          // close the connection, no answer
	Mock  *http.Response // answer with this instead of sending on; the body is fully buffered
	Delay time.Duration // hold the response back this long
	Hits  []string      // names of the rules that applied
}

// Request applies the matching rules to an outgoing request: rewrites
// change req in place (URL, headers, body replaced with the edited bytes).
func (e *Engine) Request(req *http.Request) (Verdict, error)
// Response applies the rewrites to a response, in place, and returns the
// names of the rules that applied.
func (e *Engine) Response(req *http.Request, resp *http.Response) ([]string, error)
```

### `proxy`

```go
// Recorder is where the proxy writes flows; flowlog.Log implements it.
type Recorder interface{ Put(tool.Flow) uint64 }

// Ruler is what the proxy asks about each exchange; rules.Engine implements it.
type Ruler interface {
	Request(*http.Request) (rules.Verdict, error)
	Response(*http.Request, *http.Response) ([]string, error)
}

type Options struct {
	Addr      string        // "127.0.0.1:9099"
	Authority *ca.Authority // nil: HTTPS is never decrypted
	Decrypt   bool
	Bypass    []string      // glob patterns of hosts never decrypted
	Log       Recorder
	Rules     Ruler
	Dial      func(ctx context.Context, network, addr string) (net.Conn, error) // nil: net.Dialer
	// RootCAs verifies upstream TLS servers; nil means the system roots.
	RootCAs *x509.CertPool
	// UpstreamProxy (scheme http, credentials in the userinfo) receives
	// plain-HTTP requests in absolute form; TLS tunnels still go through Dial.
	UpstreamProxy *url.URL
}

type Server struct{ /* ... */ }
func New(o Options) (*Server, error)
// Start listens and serves in the background; it returns an error wrapping
// tool.ErrPortInUse when the port is taken. Stop closes the listener and
// every connection and waits for the goroutines.
func (s *Server) Start(ctx context.Context) error
func (s *Server) Stop() error
func (s *Server) Addr() string
// Do sends req through the same pipeline a proxied request takes (rules,
// upstream, recording) and returns the ID of the flow it recorded, with
// ReplayOf set when replayOf is not 0.
func (s *Server) Do(ctx context.Context, req *http.Request, replayOf uint64) (uint64, error)
// Request builds the request a flow sent, for Do.
func Request(f tool.Flow) (*http.Request, error)
```

### `har`

```go
// Write writes the proxied flows of flows (packets are skipped) as HAR 1.2.
func Write(w io.Writer, flows []tool.Flow) error
```

### `packets`

```go
// Recorder is where decoded packets are written; flowlog.Log implements it.
type Recorder interface{ Put(tool.Flow) uint64 }

// Run reads src until it fails or ctx ends, and records one flow per
// packet (Source SourcePacket, Packet set).
func Run(ctx context.Context, src pcap.Source, log Recorder) error
// Decode turns one packet into a flow; exported for tests.
func Decode(p pcap.Packet) tool.Flow
```

### The tool (`internal/tools/traffic`)

`New(dir string) *Tool` takes the folder the app keeps its data in; the tool keeps its own state under `dir/traffic/`: `ca/` (root), `rules.json`, `config.json` and `sysproxy.json` (the saved system proxy, present only while the proxy is changed). It maps platform and proxy errors to the sentinels of `internal/tool/traffic.go`. It implements `tool.Traffic`, with the specs `start`, `stop`, `status` (risk `system`, as awake does), which only the desktop app uses.

## 6. The inspector page

`internal/ui/shell` draws it for any tool that is a `tool.Traffic` (type assertion in `toolBody`, as `Timed` and `Screened` are used); the UI never names the tool. The interaction is copied from Proxyman and HTTP Toolkit rather than invented; the spec each part follows is their documentation (Proxyman's Request / Response Previewer and Content Filter, HTTP Toolkit's View page). Nothing moves while a capture runs:

- **Toolbar**, one row: the sources toggle, record (start / stop, filled when stopped, a red dot while recording), the state text, the search field, Rules, Terminal (opens a terminal with the proxy) and the Settings gear. A warning dot keeps its space and carries the status note. The capture mode (proxy / packets) is in Settings, not on the toolbar.
- **Chip row**, one row that wraps instead of clipping: All, the type chips (JSON, Form, XML, JS, CSS, Document, Media, Other) and the status chips (2xx to 5xx, Failed). Several chips may be on; chips of one group OR together, the two groups AND. `All` clears the chips, not the search text. There are no HTTP / HTTPS chips because a flow row carries no scheme.
- **Search field**, one query line: plain words, `host:`, `method:`, `status:` (`4xx`, `404`, `none`), `program:`, `type:`, `failed:`, quoted values for spaces. It is parsed into `tool.Filter`; what it applies is shown as chips. A bad token is plain text, never an error. The sources' choices and the query intersect.
- **Sources**: the Programs and Hosts lists with counts, read from `FlowLog.Processes()` and `Hosts()` only when `Version()` changed, and the scope chip (`Capturing <programs>`, with a clear control). Clicking a row filters; a program's menu offers Capture only this program (`CaptureConfig.Scope`). The column opens by itself when the content is at least `ds.TrafficSourcesMin` wide and closes below, so a narrow window gives the list the room; the toggle overrides that for the session.
- **List**: `#`, method (coloured), status (a pill), a URL column (host and path) that takes the rest, program, started, duration and size. A narrow window drops size, started, duration and program, in that order; `#`, method, status and the URL are never dropped. A failed row has a red marker; every row reserves the marker's width so columns stay aligned. Keys: Up and Down (Page Up / Page Down, Home, End on the platforms where the framework handles them) move the selection, Esc clears it. Right click opens one menu: repeat, copy URL, copy as cURL, filter by host or program, mock / rewrite (the rule editor), block this host, do not decrypt this host, capture only this program. The text of a row is formatted once per flow and cached, not every frame.
- **Bottom bar**: Clear, Export HAR, the Auto select switch and the count (`M of N flows` when a filter narrows the list, `1 selected`). With Auto select on, the newest flow is selected as it arrives until the person picks a row by hand; Clear or Esc hands the choice back.
- **Detail** of a proxied flow: a head line (number, method, URL, status, duration, size, New Rule, Replay), then a Request pane (Headers, Query, Body, Raw, Summary) and a Response pane (Headers, Body, Raw), side by side from `ds.TrafficDetailWide` wide and stacked below. A pane remembers the tab the person chose. Pretty-printed bodies and raw text are built once per selected flow. A packet has its own Layers and Hex tabs. Nothing is drawn for the detail until the first flow exists.
- **Settings** is a dialog: capture mode, port, system proxy, decrypt, bypass, upstream, interface, the certificate, and Capture a program (copy the environment). Before the first flow the list area shows a three-step guide (certificate, system proxy, start) whose ticks follow the real state.
- The rules editor and the permission, certificate and driver guidance are as before.

### Cost of a frame

MyGo rebuilds the view on every frame, and a frame is drawn on every mouse move, so the page keeps its per-frame work small: no looping animation anywhere (a pulsing status dot used to redraw the window 60 times a second for as long as a tool ran; measured with `MYGO_FRAME_STATS` on the CI runners: 60 frames a second idle on Ubuntu before, 2 to 3 a second after, which is the app's one-second redraw while a tool runs), no formatting of rows or bodies per frame, and the log's lists read again only when its `Version()` changes.

## 7. What has run on a real system

The unit tests use fake command runners. These have also run against the real operating system:

| System | How | What |
|---|---|---|
| macOS | by hand | Root certificate install, trust and removal; the system proxy taken over, stopped, restored after a crash and after SIGTERM, each leaving the settings as found; HTTPS opened through the proxy; a real `curl` through the real proxy is named (`curl`, its pid) by the socket table, recorded with no scope and with `Scope=[curl]`, and passes unrecorded with `Scope=[Safari]`; the shell script of `Environment()` makes a real `curl https://example.com` verify through the proxy without touching the system trust store (it fails with exit 60 without the script) and the decrypted flow is recorded for `curl` over TLS 1.3; the Terminal script exports the variables intact (quotes, `$`, backticks) and deletes itself |
| Windows Server 2025 | CI job `Live`, `OMT_LIVE=1`; CI job `Test` | The Internet Settings registry key: `Enable`, `Restore` and `RestorePointing`; the current user's Root store: `Install`, `Trusted`, `Remove`, checked with `certutil` itself; `procnet.Owner` names the test process for a loopback connection (`GetExtendedTcpTable`) |
| Ubuntu | CI job `Live`, `OMT_LIVE=1`; CI job `Test` | The Debian anchor directory and the rebuilt system bundle, and Go's system pool trusting the root; `gsettings` under a session bus with dconf; an `AF_PACKET` capture of a TCP exchange on the loopback; `procnet.Owner` names the test process for a loopback connection (`/proc`) |

The live tests live next to the platform code (`*_live_*_test.go`), skip without `OMT_LIVE`, and fail rather than skip with it. On Windows, `certutil` asks the person to confirm adding and removing a root certificate; the runner has a desktop and nobody at it, so `.github/scripts/approve-root-prompt.ps1` answers Yes to those two dialogs (`Security Warning` and `Root Certificate Store`), as a person would.

Not run anywhere yet: the macOS `/dev/bpf*` grant; packet capture on Windows (it needs Npcap, which the runner does not have); the Linux trust store through `pkexec` (the runner is root) and the NSS databases; the terminal launcher on Windows and Linux (its builders are unit tested, no window was opened); the desktop app itself on Windows and Linux.
