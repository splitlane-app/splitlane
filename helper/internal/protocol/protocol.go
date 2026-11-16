// Package protocol holds the pipe messages of spec 5.1, the state of 5.2 and
// the program rule of 8.1, with the input checks of spec 11.
package protocol

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Version is the pipe protocol version: 2 since stage 3 part 1 (its spec, section 4) added the proxy
// connector; 3 since the server setup (its spec, section 7) added the server ids to the answers of
// proxy.add and proxy.refresh and proxy.reveal. The window and the helper are installed together and
// speak the same one.
const Version = 3

// Methods (spec 5.1). There are no others.
const (
	MethodHello               = "hello"
	MethodStateGet            = "state.get"
	MethodEnable              = "routing.enable"
	MethodDisable             = "routing.disable"
	MethodAllDirect           = "routing.allDirect"
	MethodLanesSet            = "lanes.set"
	MethodLogin               = "tailscale.login"
	MethodLogout              = "tailscale.logout"
	MethodSetExitNode         = "tailscale.setExitNode"
	MethodClearSystemExitNode = "systemTailscale.clearExitNode"
	MethodActivitySubscribe   = "activity.subscribe"
	MethodDiagnosticsExport   = "diagnostics.export"
	MethodShutdown            = "helper.shutdown"
	MethodProxyAdd            = "proxy.add"
	MethodProxyRefresh        = "proxy.refresh"
	MethodProxyRemove         = "proxy.remove"
	MethodProxySelect         = "proxy.select"
	MethodProxyReveal         = "proxy.reveal"
)

// Methods lists every method of spec 5.1 and of stage 3 part 1 (section 4).
var Methods = []string{
	MethodHello, MethodStateGet, MethodEnable, MethodDisable, MethodAllDirect, MethodLanesSet,
	MethodLogin, MethodLogout, MethodSetExitNode, MethodClearSystemExitNode,
	MethodActivitySubscribe, MethodDiagnosticsExport, MethodShutdown,
	MethodProxyAdd, MethodProxyRefresh, MethodProxyRemove, MethodProxySelect,
	MethodProxyReveal,
}

// Error codes: the closed list of spec 5.1.
const (
	CodeBadRequest       = "bad_request"
	CodeUnknownMethod    = "unknown_method"
	CodeProtocolMismatch = "protocol_mismatch"
	CodeWrongPhase       = "wrong_phase"
	CodeBusy             = "busy"
	CodeRuleRejected     = "rule_rejected"
	CodeApplyFailed      = "apply_failed"
	CodeUnknownExitNode  = "unknown_exit_node"
	CodeInternal         = "internal"
	// The proxy methods (stage 3 part 1): no server or subscription has the id; servers.json would
	// hold more than its limits allow. proxy.refresh also answers with the code of a failed download
	// (connector.CodeFetch*: fetch_...) or of a subscription that gave no server (the code of its first
	// rejected line, else CodeEmptySubscription).
	CodeUnknownServer     = "unknown_server"
	CodeTooMany           = "too_many"
	CodeEmptySubscription = "empty_subscription"
)

// Phases (spec 5.2).
const (
	PhaseOff      = "off"
	PhaseStarting = "starting"
	PhaseOn       = "on"
	PhaseDegraded = "degraded"
	PhaseBlocked  = "blocked"
	PhaseStopping = "stopping"
	PhaseError    = "error"
)

// Reason codes (spec 5.2).
const (
	ReasonNoDefaultRoute    = "no_default_route"
	ReasonIPForwarding      = "ip_forwarding"
	ReasonOtherCore         = "other_core"
	ReasonNotLoggedIn       = "not_logged_in"
	ReasonNoExitNode        = "no_exit_node"
	ReasonSystemDNSOverride = "system_dns_override"
	ReasonConfigInvalid     = "config_invalid"
	ReasonCoreStartTimeout  = "core_start_timeout"
	ReasonTunnelNotProven   = "tunnel_not_proven"
	ReasonSameAddress       = "same_address"
	ReasonSwitchCheckFailed = "switch_check_failed"
	ReasonLeakDetected      = "leak_detected"
	ReasonCoreCrashed       = "core_crashed"
	ReasonCoreUnresponsive  = "core_unresponsive"
	ReasonVPNUnreachable    = "vpn_unreachable"
	ReasonLoginExpired      = "login_expired"
	ReasonRestorePending    = "restore_pending"
	ReasonAdapterLeft       = "adapter_left"
	// ReasonNoServer: the connector is proxy, but no server is selected or the selected one does not
	// parse (stage 3 part 1, section 4). not_logged_in, no_exit_node and login_expired are Tailscale's.
	ReasonNoServer = "no_server"
)

// Connectors: the way out of the VPN lane (stage 3 part 1, section 2).
const (
	ConnectorTailscale = "tailscale"
	ConnectorProxy     = "proxy"
)

// Sources of a proxy server (ProxyServer.Source) besides the id of a subscription.
const (
	SourceManual = "manual" // pasted
	SourceFile   = "file"   // a .conf the window read from a file
)

// Keys of Reason.Params (spec 5.2): restore_pending carries the manual restore command and the
// error text of tailscale.exe; switch_check_failed, config_invalid and core_start_timeout may
// carry the error text. Every other reason has empty params.
const (
	ParamCommand = "command"
	ParamError   = "error"
)

// Lanes and rule kinds (spec 3, 8.1).
const (
	LaneVPN    = "vpn"
	LaneDirect = "direct"

	KindFolder  = "folder"
	KindFile    = "file"
	KindPackage = "package"
	KindSystem  = "system"
)

// Rejection reasons of lanes.set (spec 8.3).
const (
	RejectTooBroad  = "too_broad"
	RejectProtected = "protected"
	RejectDuplicate = "duplicate"
)

// Limits of spec 11: the window's data is checked before it is used.
const (
	MaxLine    = 1 << 20
	MaxApps    = 500
	MaxNameLen = 256
	MaxPathLen = 1024

	// MaxProxyText is the limit of the text of proxy.add (connector.MaxTextLen): a longer text is the
	// rejection too_long of the whole text, which the window can show. MaxRejected bounds the rejected
	// lines of one answer; the others are counted in rejectedMore.
	MaxProxyText = 64 << 10
	MaxRejected  = 50
)

// Request is one line from a client.
type Request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Error is the error object of a response.
type Error struct {
	Code     string          `json:"code"`
	Message  string          `json:"message"`
	Protocol int             `json:"protocol,omitempty"`
	Rules    []RuleRejection `json:"rules,omitempty"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Errorf builds an *Error.
func Errorf(code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// RuleRejection names one rejected rule of lanes.set (spec 8.3).
type RuleRejection struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type okResponse struct {
	ID     json.RawMessage `json:"id"`
	Result any             `json:"result"`
}

type errResponse struct {
	ID    json.RawMessage `json:"id"`
	Error *Error          `json:"error"`
}

// Event is a message from the helper without an id (spec 5.1).
type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data"`
}

// Matcher says which executables belong to a program (spec 8.1).
type Matcher struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// AppRule is a pinned program (spec 8.1).
type AppRule struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Exe     string  `json:"exe"`
	Matcher Matcher `json:"matcher"`
	Lane    string  `json:"lane"`
}

// ExitNode is one entry of tailscale.exitNodes; ID is the node's IPv4 address in the tailnet (spec 7).
type ExitNode struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Online bool   `json:"online"`
}

// Reason explains the phase; Params carries extra text, for example the manual command of restore_pending.
type Reason struct {
	Code   string            `json:"code"`
	Params map[string]string `json:"params"`
}

// Addresses are the external addresses of the two lanes.
type Addresses struct {
	VPN    *string `json:"vpn"`
	Direct *string `json:"direct"`
}

// Tailscale describes the embedded node.
type Tailscale struct {
	Backend          string     `json:"backend"`
	AuthURL          *string    `json:"authUrl"`
	DeviceName       string     `json:"deviceName"`
	ExitNodes        []ExitNode `json:"exitNodes"`
	SelectedExitNode *string    `json:"selectedExitNode"`
	Path             *string    `json:"path"`
}

// SystemTailscale describes the official client (spec 5.6).
type SystemTailscale struct {
	Installed        bool `json:"installed"`
	ExitNodeOn       bool `json:"exitNodeOn"`
	RestoreOnDisable bool `json:"restoreOnDisable"`
}

// ProxyServer is one server of the proxy connector as the state shows it: nothing secret (stage 3
// part 1, section 4). Source is SourceManual, SourceFile or the id of its subscription.
type ProxyServer struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
}

// Subscription is one subscription as the state shows it, never with its address. UpdatedAt is the
// time of the last update that worked (RFC 3339); Error the code of the last one when it failed, null
// when it worked.
type Subscription struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	UpdatedAt string  `json:"updatedAt"`
	Count     int     `json:"count"`
	Error     *string `json:"error"`
}

// Proxy is the proxy connector: the servers, the selected one (null: none) and the subscriptions.
type Proxy struct {
	Servers       []ProxyServer  `json:"servers"`
	Selected      *string        `json:"selected"`
	Subscriptions []Subscription `json:"subscriptions"`
}

// State is the full state of spec 5.2 with the connector of stage 3 part 1 (section 4).
type State struct {
	Phase           string          `json:"phase"`
	Reason          *Reason         `json:"reason"`
	AllDirect       bool            `json:"allDirect"`
	DefaultLane     string          `json:"defaultLane"`
	Apps            []AppRule       `json:"apps"`
	Addresses       Addresses       `json:"addresses"`
	Connector       string          `json:"connector"`
	Tailscale       Tailscale       `json:"tailscale"`
	Proxy           Proxy           `json:"proxy"`
	SystemTailscale SystemTailscale `json:"systemTailscale"`
}

// Clone returns a deep copy, safe to hand to another goroutine.
func (s State) Clone() State {
	c := s
	c.Apps = append([]AppRule{}, s.Apps...)
	c.Tailscale.ExitNodes = append([]ExitNode{}, s.Tailscale.ExitNodes...)
	if s.Reason != nil {
		r := Reason{Code: s.Reason.Code, Params: map[string]string{}}
		for k, v := range s.Reason.Params {
			r.Params[k] = v
		}
		c.Reason = &r
	}
	c.Addresses.VPN = copyString(s.Addresses.VPN)
	c.Addresses.Direct = copyString(s.Addresses.Direct)
	c.Tailscale.AuthURL = copyString(s.Tailscale.AuthURL)
	c.Tailscale.SelectedExitNode = copyString(s.Tailscale.SelectedExitNode)
	c.Tailscale.Path = copyString(s.Tailscale.Path)
	c.Proxy.Servers = append([]ProxyServer{}, s.Proxy.Servers...)
	c.Proxy.Selected = copyString(s.Proxy.Selected)
	c.Proxy.Subscriptions = append([]Subscription{}, s.Proxy.Subscriptions...)
	for i := range c.Proxy.Subscriptions {
		c.Proxy.Subscriptions[i].Error = copyString(s.Proxy.Subscriptions[i].Error)
	}
	return c
}

func copyString(p *string) *string {
	if p == nil {
		return nil
	}
	v := *p
	return &v
}

// Parameters and results.
type HelloParams struct {
	Protocol int `json:"protocol"`
}

type HelloResult struct {
	Protocol int    `json:"protocol"`
	Helper   string `json:"helper"`
	Core     string `json:"core"`
}

// ExportResult is the answer of diagnostics.export: the full path of the archive.
type ExportResult struct {
	Path string `json:"path"`
}

type DisableParams struct {
	SkipRestore bool `json:"skipRestore"`
}

type AllDirectParams struct {
	On *bool `json:"on"`
}

type LanesSetParams struct {
	Apps        []AppRule `json:"apps"`
	DefaultLane string    `json:"defaultLane"`
}

type SetExitNodeParams struct {
	ID string `json:"id"`
}

// ProxyAddParams are the params of proxy.add: share links (one per line), the text of a WireGuard .conf
// or one subscription address; Name is the base name of the .conf the window read from a file (never a
// path), "" for pasted text.
type ProxyAddParams struct {
	Text string `json:"text"`
	Name string `json:"name,omitempty"`
}

// ProxyIDParams are the params of proxy.refresh (a subscription), proxy.remove (a server or a
// subscription) and proxy.select (a server).
type ProxyIDParams struct {
	ID string `json:"id"`
}

// ProxyRevealResult is the answer of proxy.reveal: the address of a subscription (a secret; only the
// window reads it, to show it as a QR code).
type ProxyRevealResult struct {
	Address string `json:"address"`
}

// ProxyRejection is one rejected line: its 1-based number (0: the whole text) and its code; never the line.
type ProxyRejection struct {
	Line int    `json:"line"`
	Code string `json:"code"`
}

// ProxyAddResult is the answer of proxy.add and proxy.refresh: the ids of the servers the text or the
// subscription added (new to the list), how many it gave that were there already (not added again), the
// first MaxRejected rejected lines and how many more there were.
//
// Servers are the ids of every server of the text or of the downloaded subscription, in their order, the
// new and the ones there already (at most store's MaxServers); Subscription is the id of the subscription
// this call added or refreshed, null for links and a .conf. The window selects a server from them.
type ProxyAddResult struct {
	Added        []string         `json:"added"`
	Existing     int              `json:"existing"`
	Servers      []string         `json:"servers"`
	Subscription *string          `json:"subscription"`
	Rejected     []ProxyRejection `json:"rejected"`
	RejectedMore int              `json:"rejectedMore"`
}

// DecodeParams decodes params strictly: unknown fields and trailing data are bad_request.
// Missing params decode as the zero value.
func DecodeParams(raw json.RawMessage, v any) *Error {
	if len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		raw = json.RawMessage("{}")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return Errorf(CodeBadRequest, "params: %v", err)
	}
	// dec.More() is false before a closing bracket, so "{}}" and "{}]" would pass;
	// requiring a clean EOF rejects every kind of trailing data.
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Errorf(CodeBadRequest, "params: trailing data")
	}
	return nil
}

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ValidateLanes checks the shape of lanes.set (spec 11): lanes, kinds, lengths, unique UUIDs.
// The meaning of each rule (8.2) is checked by package rules.
func ValidateLanes(p LanesSetParams) *Error {
	if p.DefaultLane != LaneVPN && p.DefaultLane != LaneDirect {
		return Errorf(CodeBadRequest, "defaultLane must be vpn or direct")
	}
	if len(p.Apps) > MaxApps {
		return Errorf(CodeBadRequest, "too many apps: %d > %d", len(p.Apps), MaxApps)
	}
	seen := map[string]bool{}
	for i, a := range p.Apps {
		switch {
		case !uuidRe.MatchString(a.ID):
			return Errorf(CodeBadRequest, "apps[%d].id is not a UUID", i)
		case seen[a.ID]:
			return Errorf(CodeBadRequest, "apps[%d].id repeats", i)
		case a.Lane != LaneVPN && a.Lane != LaneDirect:
			return Errorf(CodeBadRequest, "apps[%d].lane must be vpn or direct", i)
		case len(a.Name) > MaxNameLen:
			return Errorf(CodeBadRequest, "apps[%d].name is longer than %d bytes", i, MaxNameLen)
		case len(a.Exe) > MaxPathLen || len(a.Matcher.Value) > MaxPathLen:
			return Errorf(CodeBadRequest, "apps[%d] path is longer than %d bytes", i, MaxPathLen)
		}
		switch a.Matcher.Kind {
		case KindFolder, KindFile, KindPackage, KindSystem:
		default:
			return Errorf(CodeBadRequest, "apps[%d].matcher.kind is unknown", i)
		}
		seen[a.ID] = true
	}
	return nil
}

// ValidateProxyAdd checks proxy.add (spec 11): a text, and a name that is a plain base name of at most
// MaxNameLen bytes: no folder, no drive, no control character. The length of the text is the parser's
// (MaxProxyText: a rejection the window can show).
func ValidateProxyAdd(p ProxyAddParams) *Error {
	if p.Text == "" {
		return Errorf(CodeBadRequest, "text is empty")
	}
	n := p.Name
	switch {
	case len(n) > MaxNameLen:
		return Errorf(CodeBadRequest, "name is longer than %d bytes", MaxNameLen)
	case strings.ContainsAny(n, `/\:`) || n == "." || n == "..":
		return Errorf(CodeBadRequest, "name must be a base name, not a path")
	case strings.ContainsFunc(n, unicode.IsControl) || !utf8.ValidString(n):
		return Errorf(CodeBadRequest, "name has a control character")
	}
	return nil
}

var (
	serverIDRe       = regexp.MustCompile(`^[0-9a-f]{16}$`)
	subscriptionIDRe = regexp.MustCompile(`^sub-[0-9a-f]{16}$`)
)

// ValidateProxyID checks the id of proxy.refresh, proxy.remove and proxy.select (spec 11): the id of a
// server (16 hex digits) or of a subscription (sub- and 16 hex digits). Whether one has it is the
// engine's to say (unknown_server).
func ValidateProxyID(id string) *Error {
	if !serverIDRe.MatchString(id) && !subscriptionIDRe.MatchString(id) {
		return Errorf(CodeBadRequest, "id is not the id of a server or a subscription")
	}
	return nil
}

// IsSubscriptionID tells the id of a subscription from that of a server.
func IsSubscriptionID(id string) bool { return subscriptionIDRe.MatchString(id) }

var tailnetV4 = netip.MustParsePrefix("100.64.0.0/10")

// ValidateExitNodeID checks tailscale.setExitNode (spec 11): an IPv4 address from the known
// list, or, when there is no list, an address in 100.64.0.0/10.
func ValidateExitNodeID(id string, known []ExitNode) *Error {
	addr, err := netip.ParseAddr(id)
	if err != nil || !addr.Is4() {
		return Errorf(CodeBadRequest, "id must be an IPv4 address")
	}
	if len(known) == 0 {
		if !tailnetV4.Contains(addr) {
			return Errorf(CodeUnknownExitNode, "id is outside 100.64.0.0/10")
		}
		return nil
	}
	for _, n := range known {
		if n.ID == id {
			return nil
		}
	}
	return Errorf(CodeUnknownExitNode, "id is not in the list of exit nodes")
}

// NewScanner reads one message per line, a line at most MaxLine bytes long.
func NewScanner(r io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), MaxLine)
	return s
}

// EncodeResult is the line of a successful response.
func EncodeResult(id json.RawMessage, result any) []byte {
	if result == nil {
		result = struct{}{}
	}
	return line(okResponse{ID: rawID(id), Result: result})
}

// EncodeError is the line of a failed response.
func EncodeError(id json.RawMessage, e *Error) []byte {
	return line(errResponse{ID: rawID(id), Error: e})
}

// EncodeEvent is the line of an event.
func EncodeEvent(name string, data any) []byte {
	return line(Event{Event: name, Data: data})
}

func rawID(id json.RawMessage) json.RawMessage {
	if len(id) == 0 {
		return json.RawMessage("null")
	}
	return id
}

func line(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		b, _ = json.Marshal(errResponse{ID: json.RawMessage("null"), Error: Errorf(CodeInternal, "encode: %v", err)})
	}
	return append(b, '\n')
}

var (
	ipv4Re     = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)
	tailnet6Re = regexp.MustCompile(`(?i)\bfd7a:115c:a1e0:[0-9a-f:]*[0-9a-f]`)
	tsNameRe   = regexp.MustCompile(`(?i)\b[a-z0-9-]+(\.[a-z0-9-]+)*\.ts\.net\b`)
	peerRe     = regexp.MustCompile(`[^\s()]+ \(<ip>\)`) // the Tailscale CLI prints a peer as "name (address)"
)

// MaskAddresses replaces IPv4 addresses with <ip>, tailnet IPv6 addresses with <ip6>, tailnet names
// with <name>.ts.net and the name of a peer printed next to its address with <peer>: logs and console
// output never carry them.
func MaskAddresses(s string) string {
	s = ipv4Re.ReplaceAllString(s, "<ip>")
	s = tailnet6Re.ReplaceAllString(s, "<ip6>")
	s = tsNameRe.ReplaceAllString(s, "<name>.ts.net")
	return peerRe.ReplaceAllString(s, "<peer> (<ip>)")
}
