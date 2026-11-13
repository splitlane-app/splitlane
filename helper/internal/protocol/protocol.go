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
)

// Version is the pipe protocol version of stage 1 (spec 5.1).
const Version = 1

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
)

// Methods lists every method of spec 5.1.
var Methods = []string{
	MethodHello, MethodStateGet, MethodEnable, MethodDisable, MethodAllDirect, MethodLanesSet,
	MethodLogin, MethodLogout, MethodSetExitNode, MethodClearSystemExitNode,
	MethodActivitySubscribe, MethodDiagnosticsExport, MethodShutdown,
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

// State is the full state of spec 5.2.
type State struct {
	Phase           string          `json:"phase"`
	Reason          *Reason         `json:"reason"`
	AllDirect       bool            `json:"allDirect"`
	DefaultLane     string          `json:"defaultLane"`
	Apps            []AppRule       `json:"apps"`
	Addresses       Addresses       `json:"addresses"`
	Tailscale       Tailscale       `json:"tailscale"`
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

var ipv4Re = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)

// MaskAddresses replaces IPv4 addresses with <ip>: logs and console output never carry them.
func MaskAddresses(s string) string { return ipv4Re.ReplaceAllString(s, "<ip>") }
