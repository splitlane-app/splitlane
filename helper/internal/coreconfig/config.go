// Package coreconfig builds the config of sing-box 1.14.4 (spec 6). The helper
// alone builds it, from checked data; nothing in it comes from the window as is.
package coreconfig

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"splitlane/helper/internal/rules"
)

// Tags, names and the core's log lines the helper relies on (Plan 0, "Факты о ядре 1.14.4").
const (
	TagProbeVPN    = "probe-vpn"
	TagProbeDirect = "probe-direct"
	TagTun         = "tun-in"
	TagVPN         = "vpn"
	TagDirect      = "direct"
	RuleSetDirect  = "lane-direct"
	RuleSetVPN     = "lane-vpn"
	AdapterName    = "SplitLane"

	ModeVPN            = "vpn"
	ModeVPNLocalDNS    = "vpn-localdns"
	ModeDirect         = "direct"
	ModeDirectLocalDNS = "direct-localdns"
	ModeAllDirect      = "alldirect"

	// StartLine is written by the core when it has started (spec 5.3 step 4).
	StartLine = "sing-box started"
	// LoginLine means the embedded node needs a login (spec 5.3 step 4, 7).
	LoginLine = "Waiting for authentication"
	// ReloadErrorLine is written when a rule file could not be read again (spec 6.5, 8.3).
	ReloadErrorLine = "reload rule-set lane-"
)

// Modes are the five clash modes of spec 6.3.
var Modes = []string{ModeVPN, ModeVPNLocalDNS, ModeDirect, ModeDirectLocalDNS, ModeAllDirect}

// ModeFor is the clash mode for the main lane, the "all direct" switch and the degraded phase (spec 6.3).
func ModeFor(defaultLane string, allDirect, degraded bool) string {
	switch {
	case allDirect:
		return ModeAllDirect
	case defaultLane == TagDirect && degraded:
		return ModeDirectLocalDNS
	case defaultLane == TagDirect:
		return ModeDirect
	case degraded:
		return ModeVPNLocalDNS
	default:
		return ModeVPN
	}
}

// MainLane is the outbound that rule 11 of spec 6.2 gives to a program without a rule in this mode.
func MainLane(mode string) string {
	if mode == ModeVPN || mode == ModeVPNLocalDNS {
		return TagVPN
	}
	return TagDirect
}

// Development-only pin (Plan 1, safety rule 4): while the developer's AI assistant reaches its
// service only through the VPN, its programs and the service addresses stay on the VPN lane.
// The product never sets Options.DevPin.
var (
	DevPinProcesses = []string{"claude.exe", "bun.exe", "node.exe", "python.exe", "pythonw.exe"}
	DevPinDomains   = []string{"anthropic.com", "claude.ai", "claude.com"}
	DevPinCIDR      = []string{"160.79.104.0/23"}
)

// Ports of one core run: two probe inbounds, the Clash API and the API service (spec 6.1).
type Ports struct {
	ProbeVPN    int
	ProbeDirect int
	Clash       int
	API         int
}

// Options are the inputs of one config.
type Options struct {
	Mode         string // clash_api.default_mode: the mode needed at start (spec 6.3)
	DataDir      string // %ProgramData%\SplitLane or the development data folder
	HelperPath   string // full path of splitlane-helper.exe (rule 6)
	TailscaleDir string // install folder of the system Tailscale client (rule 5)
	Lane         Lane   // the connector of the VPN lane: the embedded Tailscale node or one server
	Ports        Ports
	ClashSecret  string
	APISecret    string
	DevPin       bool
	LogLevel     string // "" or info (spec 6.1); debug only from the development flag --core-log-level
}

// Levels of the core log (spec 6.1): the product writes info; debug is for development only, to
// collect evidence of a failure (appendix A: the core's own DNS without answers at its start).
const (
	LogInfo  = "info"
	LogDebug = "debug"
)

// logLevel is the level a config writes: "" is info; any level but info and debug is an error.
func logLevel(level string) (string, error) {
	switch level {
	case "", LogInfo:
		return LogInfo, nil
	case LogDebug:
		return LogDebug, nil
	}
	return "", fmt.Errorf("coreconfig: log level must be info or debug, not %q", level)
}

// Paths are the files of the data folder that the config names.
type Paths struct {
	RulesDir    string
	RulesDirect string
	RulesVPN    string
	LogsDir     string
	CoreLog     string
	SetupLog    string // the setup mode's core (spec 7): its runs never set CoreLog aside
	CoreStdout  string
	StateDir    string
	Config      string
}

// PathsFor lists the files under a data folder (spec 4.2).
func PathsFor(dataDir string) Paths {
	rulesDir := filepath.Join(dataDir, "rules")
	logs := filepath.Join(dataDir, "logs")
	return Paths{
		RulesDir:    rulesDir,
		RulesDirect: filepath.Join(rulesDir, "lane-direct.json"),
		RulesVPN:    filepath.Join(rulesDir, "lane-vpn.json"),
		LogsDir:     logs,
		CoreLog:     filepath.Join(logs, "core.log"),
		SetupLog:    filepath.Join(logs, "core-setup.log"),
		CoreStdout:  filepath.Join(logs, "core-stdout.log"),
		StateDir:    filepath.Join(dataDir, "tailscale"),
		Config:      filepath.Join(dataDir, "core-config.json"),
	}
}

type obj = map[string]any

func route(outbound string, match obj) obj {
	match["action"] = "route"
	match["outbound"] = outbound
	return match
}

// rejectNodeInbound is rule 0 of spec 6.2, first in both configs of the Tailscale lane (for a
// WireGuard endpoint see wireGuardNodeRule; a proxy outbound has no such flows). The embedded node hands the
// router every flow that a tailnet peer opens to the node's own address, with the inbound TagVPN
// and the destination rewritten to 127.0.0.1 (sing-box 1.14.4, protocol/tailscale/endpoint.go,
// NewConnectionEx and NewPacketConnectionEx); routed, it would reach the PC's localhost-only
// services, the API service among them (spec 11). The node's own traffic (control server, DERP,
// disco) and the flows the helper uses (the probes, the adapter, the API service) never come from
// that inbound.
func rejectNodeInbound() obj {
	return obj{"inbound": []string{TagVPN}, "action": "reject"}
}

// Build returns the config as indented JSON (UTF-8 without BOM, keys sorted). The lane decides the
// outbound or endpoint tagged vpn, rule 0 and the API service (vpnLane); the rest is the same for
// every connector (spec of stage 3 part 1, section 5).
func Build(o Options) ([]byte, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	level, err := logLevel(o.LogLevel)
	if err != nil {
		return nil, err
	}
	p := PathsFor(o.DataDir)
	laneOutbounds, laneEndpoints, nodeRule, api, err := vpnLane(o.Lane, p)
	if err != nil {
		return nil, err
	}
	tsDir := o.TailscaleDir
	if tsDir == "" {
		tsDir = `C:\Program Files\Tailscale`
	}
	var routeRules []any
	if nodeRule {
		routeRules = append(routeRules, rejectNodeInbound())
	}
	routeRules = append(routeRules,
		obj{"ip_version": 6, "action": "reject"},
		route(TagVPN, obj{"inbound": []string{TagProbeVPN}}),
		route(TagDirect, obj{"inbound": []string{TagProbeDirect}}),
		obj{"port": 53, "action": "hijack-dns"},
		obj{"ip_cidr": []string{"172.19.0.0/30"}, "action": "reject"},
		route(TagDirect, obj{"process_path_regex": []string{
			rules.BuiltinExpression(filepath.Join(tsDir, "tailscaled.exe")),
			rules.BuiltinExpression(filepath.Join(tsDir, "tailscale-ipn.exe")),
		}}),
		route(TagVPN, obj{"process_path_regex": []string{rules.BuiltinExpression(o.HelperPath)}}),
		route(TagDirect, obj{"ip_is_private": true}),
	)
	if o.DevPin {
		routeRules = append(routeRules,
			route(TagVPN, obj{"process_name": DevPinProcesses}),
			route(TagVPN, obj{"domain_suffix": DevPinDomains, "ip_cidr": DevPinCIDR}))
	}
	routeRules = append(routeRules,
		route(TagDirect, obj{"clash_mode": ModeAllDirect}),
		route(TagDirect, obj{"rule_set": []string{RuleSetDirect}}),
		route(TagVPN, obj{"rule_set": []string{RuleSetVPN}}),
		route(TagDirect, obj{"clash_mode": ModeDirect}),
		route(TagDirect, obj{"clash_mode": ModeDirectLocalDNS}),
		route(TagVPN, obj{"clash_mode": ModeVPN}),
		route(TagVPN, obj{"clash_mode": ModeVPNLocalDNS}),
	)
	localDNS := func(mode string) obj {
		return obj{"clash_mode": mode, "action": "route", "server": "dns-local"}
	}
	outbounds := []any{obj{"type": "direct", "tag": TagDirect, "domain_resolver": "dns-local"}}
	for _, l := range laneOutbounds {
		outbounds = append(outbounds, l)
	}
	config := obj{
		// a log written to a file carries no color codes (spec 6.1)
		"log": obj{"level": level, "timestamp": true, "output": filepath.ToSlash(p.CoreLog)},
		"dns": obj{
			"servers": []any{
				obj{"type": "local", "tag": "dns-local"},
				obj{"type": "https", "tag": "dns-vpn", "server": "1.1.1.1", "detour": TagVPN},
			},
			"rules":           []any{localDNS(ModeVPNLocalDNS), localDNS(ModeDirectLocalDNS), localDNS(ModeAllDirect)},
			"final":           "dns-vpn",
			"strategy":        "ipv4_only",
			"reverse_mapping": true,
		},
		"inbounds": []any{
			obj{"type": "mixed", "tag": TagProbeVPN, "listen": "127.0.0.1", "listen_port": o.Ports.ProbeVPN},
			obj{"type": "mixed", "tag": TagProbeDirect, "listen": "127.0.0.1", "listen_port": o.Ports.ProbeDirect},
			obj{
				"type":                  "tun",
				"tag":                   TagTun,
				"interface_name":        AdapterName,
				"address":               []string{"172.19.0.1/30", "fdfe:dcba:9876::1/126"},
				"auto_route":            true,
				"strict_route":          true,
				"route_exclude_address": []string{"100.64.0.0/10", "fd7a:115c:a1e0::/48"},
				"stack":                 "gvisor",
			},
		},
		"outbounds": outbounds,
		"route": obj{
			"rules": routeRules,
			"rule_set": []any{
				obj{"type": "local", "tag": RuleSetDirect, "format": "source", "path": filepath.ToSlash(p.RulesDirect)},
				obj{"type": "local", "tag": RuleSetVPN, "format": "source", "path": filepath.ToSlash(p.RulesVPN)},
			},
			"final":                   TagVPN,
			"auto_detect_interface":   true,
			"find_process":            true,
			"default_domain_resolver": "dns-local",
		},
		"experimental": obj{"clash_api": obj{
			"external_controller":                  fmt.Sprintf("127.0.0.1:%d", o.Ports.Clash),
			"secret":                               o.ClashSecret,
			"default_mode":                         o.Mode,
			"access_control_allow_origin":          []string{"http://127.0.0.1"},
			"access_control_allow_private_network": false,
		}},
	}
	if len(laneEndpoints) > 0 {
		endpoints := []any{}
		for _, e := range laneEndpoints {
			endpoints = append(endpoints, e)
		}
		config["endpoints"] = endpoints
	}
	if api {
		config["services"] = []any{obj{
			"type": "api", "tag": "api", "listen": "127.0.0.1", "listen_port": o.Ports.API, "secret": o.APISecret,
		}}
	}
	b, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func (o Options) validate() error {
	known := false
	for _, m := range Modes {
		known = known || m == o.Mode
	}
	if !known {
		return fmt.Errorf("coreconfig: unknown mode %q", o.Mode)
	}
	if err := o.Lane.check(); err != nil {
		return err
	}
	seen := map[int]bool{}
	for _, port := range []int{o.Ports.ProbeVPN, o.Ports.ProbeDirect, o.Ports.Clash, o.Ports.API} {
		if port <= 0 || port > 65535 || seen[port] {
			return fmt.Errorf("coreconfig: ports must be four different numbers 1..65535")
		}
		seen[port] = true
	}
	switch {
	case o.DataDir == "" || !filepath.IsAbs(o.DataDir):
		return fmt.Errorf("coreconfig: data folder must be an absolute path")
	case o.HelperPath == "" || !filepath.IsAbs(o.HelperPath):
		return fmt.Errorf("coreconfig: helper path must be an absolute path")
	case len(o.ClashSecret) < 16 || len(o.APISecret) < 16:
		return fmt.Errorf("coreconfig: secrets must be at least 16 characters")
	}
	return nil
}
