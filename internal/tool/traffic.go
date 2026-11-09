package tool

import (
	"context"
	"errors"
	"io"
	"time"
)

// The errors a traffic tool returns, for the UI to tell what to ask the
// person to do. Each is wrapped with the detail, so test with errors.Is.
var (
	// ErrPermission: the system refuses the capture or the change, and a
	// person has to allow it (root or BPF access, an administrator prompt).
	ErrPermission = errors.New("permission denied")
	// ErrDriverMissing: the packet capture driver (Npcap on Windows) is not
	// installed.
	ErrDriverMissing = errors.New("packet capture driver missing")
	// ErrPortInUse: the proxy's port is taken by another program.
	ErrPortInUse = errors.New("proxy port in use")
	// ErrCertNotTrusted: a flow could not be decrypted because the root
	// certificate is not trusted by the system.
	ErrCertNotTrusted = errors.New("root certificate not trusted")
	// ErrUnsupportedHere: the thing asked for does not exist on this OS.
	ErrUnsupportedHere = errors.New("not supported on this system")
	// ErrNotRunning: the thing asked for needs the capture to be running.
	ErrNotRunning = errors.New("capture not running")
)

// CaptureMode is how a traffic tool sees the network.
type CaptureMode int

// The capture modes.
const (
	// ModeProxy runs a local HTTP(S) proxy, and the system proxy points at
	// it. It sees whole requests and responses, decrypted when asked to.
	ModeProxy CaptureMode = iota
	// ModePackets reads the packets of a network interface.
	ModePackets
)

// CaptureConfig is what a person sets before starting a capture. A change
// to a running capture restarts it.
type CaptureConfig struct {
	Mode CaptureMode
	// ProxyPort is the port of the proxy on 127.0.0.1; it never listens on
	// another address.
	ProxyPort int
	// SystemProxy points the system proxy at the proxy while it runs.
	SystemProxy bool
	// Decrypt opens HTTPS with the app's root certificate, so the flows
	// show their content. Off, an HTTPS connection is one tunnel row.
	Decrypt bool
	// Bypass lists the hosts that are never decrypted (glob patterns).
	Bypass []string
	// Interface is the name of the interface ModePackets reads; empty
	// means the one the system routes through.
	Interface string
	// Upstream is a proxy the capture sends its traffic through, as
	// "http://host:port" or "socks5://host:port"; empty goes direct. A
	// person whose internet only works through a proxy needs it, because
	// pointing the system proxy at the capture would otherwise cut that
	// proxy out. When it is empty and SystemProxy is on, the capture uses
	// the proxy the system had before it changed the setting.
	Upstream string
	// Scope lists the names of the programs whose traffic is recorded; the
	// traffic of every other program passes through the proxy without
	// being opened or logged. Empty records every program. It applies to
	// ModeProxy only.
	Scope []string
}

// FlowSource is what recorded a flow.
type FlowSource int

// The sources.
const (
	SourceProxy FlowSource = iota
	SourcePacket
)

// FlowState is how far a flow has got.
type FlowState int

// The states of a flow.
const (
	// FlowOpen: the request is out, or the tunnel is up, and the response
	// has not finished.
	FlowOpen FlowState = iota
	FlowDone
	// FlowFailed: the connection broke or the server could not be reached.
	FlowFailed
	// FlowBlocked: a rule refused it.
	FlowBlocked
)

// Header is one header line, in the order it was sent and with its case.
type Header struct {
	Name, Value string
}

// MaxBody is the most bytes of one body a flow keeps; the rest is counted
// in Size but dropped.
const MaxBody = 4 << 20

// Body is what one side of a flow carried.
type Body struct {
	// Type is the media type without parameters, such as "application/json".
	Type string
	// Data is the body with its content encoding (gzip, br, deflate)
	// removed when that is known, cut at MaxBody.
	Data []byte
	// Size is the bytes the body had on the wire.
	Size      int64
	Truncated bool
}

// HTTPMessage is a request or a response.
type HTTPMessage struct {
	Headers []Header
	Body    Body
}

// FlowRow is what the list shows of a flow: small, with no bodies.
type FlowRow struct {
	ID      uint64
	Source  FlowSource
	Started time.Time
	// Method is the HTTP method; empty for a packet.
	Method string
	Host   string
	// Path is the path and query. For a packet it is empty and Info says
	// what the packet is.
	Path string
	// Proto is "HTTP/1.1", "h2", "tunnel", or for packets "TCP", "UDP",
	// "DNS", "TLS", "ICMP" and the like.
	Proto string
	// Status is the response status; 0 while there is none, and for packets.
	Status int
	// Type is the media type of the response.
	Type string
	// Size is the bytes of the response; for a packet, its length.
	Size     int64
	Duration time.Duration
	State    FlowState
	// Info is a one-line note: the error of a failed flow, or what a packet
	// is ("SYN 52114 → 443").
	Info string
	// Ruled is whether a rule changed or answered the flow.
	Ruled bool
	// Process is the program that opened the connection, when the system
	// could tell; the zero value when it could not.
	Process Program
}

// Program is the program that owns a connection.
type Program struct {
	// PID is the process id; 0 when there is no process.
	PID int
	// Name is what a person calls the program: "Safari", "curl".
	Name string
	// Path is the executable's path, when known.
	Path string
}

// Facet is a value of a field and how many flows have it, for the lists a
// person narrows the log by.
type Facet struct {
	Name  string
	Count int
}

// Environment is what a program needs to go through the running proxy.
type Environment struct {
	// Vars are the variables, in the order they are set.
	Vars []Header
	// Script sets them in a shell: export lines for a POSIX shell, $env
	// lines for PowerShell.
	Script string
	// Shell names the syntax of Script: "sh" or "powershell".
	Shell string
}

// Layer is one protocol layer of a packet, as a tree for the detail pane.
type Layer struct {
	Name   string
	Fields []Header
}

// PacketDetail is what the detail pane shows of a packet.
type PacketDetail struct {
	Layers []Layer
	// Raw is the packet's bytes, cut at MaxBody.
	Raw []byte
}

// Flow is a flow with everything it carried.
type Flow struct {
	FlowRow
	// URL is the whole URL of a proxied request; empty for a packet.
	URL      string
	Request  HTTPMessage
	Response HTTPMessage
	// Decrypted is whether the flow was HTTPS opened with the root
	// certificate.
	Decrypted bool
	// ServerAddr is the IP and port the proxy reached.
	ServerAddr string
	// TLSVersion is "TLS 1.3" and the like; empty for plain HTTP.
	TLSVersion string
	// RuleHits names the rules that changed or answered the flow.
	RuleHits []string
	// ReplayOf is the ID of the flow this one sent again; 0 if none.
	ReplayOf uint64
	// Packet is set for a packet, and nil for a proxied flow.
	Packet *PacketDetail
}

// Filter picks the flows the list shows. The zero value shows every flow
// of Source.
type Filter struct {
	Source FlowSource
	// Text matches, ignoring case, the URL, the headers, the text bodies
	// and Info.
	Text string
	// Hosts, Methods and Types match any of their entries; Types by
	// prefix of the response media type ("image/").
	Hosts   []string
	Methods []string
	Types   []string
	// StatusClasses are the hundreds of the status wanted: 2 for 2xx; 0
	// for no response yet.
	StatusClasses []int
	// Failed shows only flows that failed or were blocked.
	Failed bool
	// Processes match the Process.Name of a flow, ignoring case; any of
	// them. Empty matches every program.
	Processes []string
}

// FlowLog is what a traffic tool has recorded. It is safe for use from
// any goroutine, and reading it never waits on the network.
type FlowLog interface {
	// Version changes whenever the log does. The UI asks for the rows again
	// only when it has changed.
	Version() uint64
	// Rows returns the flows f picks, oldest first. The slice is a
	// snapshot the caller may keep; it does not change.
	Rows(f Filter) []FlowRow
	// Get returns a flow with everything it carried.
	Get(id uint64) (Flow, bool)
	// Clear forgets every flow.
	Clear()
	// OnChange calls fn after the log changes, at most every 100 ms, and
	// off the UI thread. stop ends it.
	OnChange(fn func()) (stop func())
	// Processes returns the programs that opened the flows of the log, and
	// how many flows each opened, most flows first, then by name. Flows
	// with no known program are left out.
	Processes() []Facet
	// Hosts returns the hosts of the flows of the log with their counts,
	// most flows first, then by name.
	Hosts() []Facet
}

// RuleKind is what a rule does to the flows it matches.
type RuleKind int

// The kinds of rules.
const (
	// RuleRewrite changes the request on its way out and the response on
	// its way back.
	RuleRewrite RuleKind = iota
	// RuleMock answers with a response of its own; the request is never
	// sent on.
	RuleMock
	// RuleBlock closes the connection without an answer.
	RuleBlock
	// RuleDelay holds the response back.
	RuleDelay
)

// RuleMatch picks the requests a rule applies to.
type RuleMatch struct {
	// Methods are the methods matched; empty matches any.
	Methods []string
	// Pattern is matched against scheme://host/path?query; "*" stands for
	// any run of characters. Regexp reads it as a regular expression.
	Pattern string
	Regexp  bool
}

// HeaderEdit sets a header, or removes it.
type HeaderEdit struct {
	Name   string
	Value  string
	Remove bool
}

// BodyEdit changes a text body: it replaces Find with Replace, or, when
// Find is empty, the whole body with Replace.
type BodyEdit struct {
	Find    string
	Replace string
	Regexp  bool
}

// RewriteEffect is what RuleRewrite does.
type RewriteEffect struct {
	// URL, when not empty, sends the request to this URL instead.
	URL             string
	RequestHeaders  []HeaderEdit
	ResponseHeaders []HeaderEdit
	RequestBody     *BodyEdit
	ResponseBody    *BodyEdit
	// Status, when not 0, replaces the response status.
	Status int
}

// MockEffect is the response RuleMock answers with.
type MockEffect struct {
	Status  int
	Headers []Header
	Body    string
	// File, when not empty, is a local file served as the body instead.
	File string
}

// Rule is one rule of the rule book. Rules apply in order; every rewrite
// that matches applies, and the first mock or block that matches ends it.
type Rule struct {
	ID      string
	Name    string
	Enabled bool
	Match   RuleMatch
	Kind    RuleKind
	Rewrite RewriteEffect
	Mock    MockEffect
	Delay   time.Duration
}

// CertState is the state of the app's root certificate.
type CertState struct {
	// Exists is whether the certificate has been made.
	Exists bool
	// Trusted is whether the system trusts it.
	Trusted bool
	// Fingerprint is its SHA-256, in hex.
	Fingerprint string
	// Path is the PEM file a person can import into another program.
	Path     string
	NotAfter time.Time
}

// NetInterface is a network interface a capture can read.
type NetInterface struct {
	Name string
	// Description is what the system calls it ("Wi-Fi").
	Description string
	Up          bool
}

// Traffic is a switch tool that records network traffic and lets a person
// look through it, rewrite it and send it again. Start and Stop begin and
// end the capture; what was recorded stays until it is cleared. The UI
// draws the inspector for any tool that is one. The command line and MCP do
// not offer it at all: a flow can hold secrets, and a capture changes the
// system proxy, which is for a person to decide.
type Traffic interface {
	Switch
	Config() CaptureConfig
	// SetConfig keeps the config, and restarts a capture in progress.
	SetConfig(CaptureConfig) error
	// Log returns the log of the mode in Config: packets and proxied flows
	// are kept apart, so a change of mode changes the log. Ask again after
	// SetConfig, and follow the log by its identity.
	Log() FlowLog
	Rules() []Rule
	// SetRules replaces the rule book, which takes effect at once, and
	// keeps it for the next launch.
	SetRules([]Rule) error
	Cert() (CertState, error)
	// InstallCert makes the root certificate, if there is none, and makes
	// the system trust it. It may ask the person for a password.
	InstallCert(ctx context.Context) error
	// RemoveCert makes the system stop trusting the certificate and
	// deletes it and its key.
	RemoveCert(ctx context.Context) error
	// Replay sends the request of a flow again, by the proxy's rules, and
	// returns the ID of the new flow.
	Replay(ctx context.Context, id uint64) (uint64, error)
	// ExportHAR writes the proxied flows with the given IDs, or all of
	// them when ids is empty, as HAR 1.2.
	ExportHAR(w io.Writer, ids []uint64) error
	Interfaces() ([]NetInterface, error)
	// Environment returns what a program needs to send its traffic through
	// the running proxy and to trust its certificate, as variables and as
	// the text to paste into a shell (POSIX or PowerShell, by system). It
	// fails with ErrNotRunning when the proxy is not running and with
	// ErrUnsupportedHere in packet mode.
	Environment() (Environment, error)
	// OpenTerminal opens a terminal window with that environment set, so a
	// program started in it is captured. It fails as Environment does.
	OpenTerminal(ctx context.Context) error
}
