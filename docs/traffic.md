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

Decisions that bound the work: loopback only; bodies are cut at `tool.MaxBody` (4 MiB); the proxy log keeps at most 5000 flows and 256 MiB of bodies, the packet log 50000 packets, and the oldest go first; the tool exists in the desktop app only (like Screenshot, it is registered in `cmd/oh-my-toys` and not in `omt`), so neither the command line nor MCP can start a capture or read one: a flow can hold secrets and a capture changes the system proxy.

Known limit (macOS): the system proxy is set on every network service, but apps read the settings of the primary service. When a proxy app's virtual interface is primary (observed with Surge's `utun`: `scutil --proxy` showed no proxy while Traffic was capturing), system-proxy-aware apps do not reach the listener. The client has to be pointed at `127.0.0.1:<port>` directly, or the other proxy app's system proxy turned off. The tool asks `sysproxy.Effective` when it takes the system proxy and says so in its note while that run lasts.

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

`internal/ui/shell` draws it for any tool that is a `tool.Traffic` (type assertion in `toolBody`, as `Timed` and `Screened` are used); the UI never names the tool. It holds: a toolbar (start / stop, mode, clear, export, certificate status), the status note when the tool has one, a filter bar, a virtualised list, a detail pane, a rules editor and the permission, certificate and driver guidance.
