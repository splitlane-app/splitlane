package coreconfig

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

func testOptions(dataDir string) Options {
	return Options{
		Mode:         ModeVPN,
		DataDir:      dataDir,
		HelperPath:   `C:\Program Files\SplitLane\helper\splitlane-helper.exe`,
		TailscaleDir: `C:\Program Files\Tailscale`,
		Hostname:     "PC-splitlane",
		ExitNode:     "100.64.0.1",
		Ports:        Ports{ProbeVPN: 30001, ProbeDirect: 30002, Clash: 30003, API: 30004},
		ClashSecret:  "0123456789abcdef0123456789abcdef",
		APISecret:    "fedcba9876543210fedcba9876543210",
	}
}

func TestModeFor(t *testing.T) {
	cases := []struct {
		lane              string
		allDirect, degrad bool
		want              string
	}{
		{"vpn", false, false, ModeVPN},
		{"vpn", false, true, ModeVPNLocalDNS},
		{"direct", false, false, ModeDirect},
		{"direct", false, true, ModeDirectLocalDNS},
		{"vpn", true, false, ModeAllDirect},
		{"direct", true, true, ModeAllDirect},
	}
	for _, c := range cases {
		if got := ModeFor(c.lane, c.allDirect, c.degrad); got != c.want {
			t.Errorf("ModeFor(%s,%v,%v)=%s want %s", c.lane, c.allDirect, c.degrad, got, c.want)
		}
	}
	for mode, lane := range map[string]string{ModeVPN: "vpn", ModeVPNLocalDNS: "vpn", ModeDirect: "direct", ModeDirectLocalDNS: "direct", ModeAllDirect: "direct"} {
		if MainLane(mode) != lane {
			t.Errorf("MainLane(%s)", mode)
		}
	}
}

func TestBuildMatchesGolden(t *testing.T) {
	got, err := Build(testOptions(`C:\ProgramData\SplitLane`))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "vpn.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("config differs from testdata/vpn.golden.json:\n%s", got)
	}
}

// summary turns route.rules into short words, to check the order of spec 6.2.
func summary(t *testing.T, config []byte) []string {
	t.Helper()
	var c struct {
		Route struct {
			Rules []map[string]any `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range c.Route.Rules {
		var key string
		for _, k := range []string{"ip_version", "inbound", "port", "domain_suffix", "ip_cidr", "process_path_regex", "ip_is_private", "process_name", "clash_mode", "rule_set"} {
			if v, ok := r[k]; ok {
				key = k
				if k == "clash_mode" {
					key += "=" + v.(string)
				}
				if k == "inbound" || k == "rule_set" {
					key += "=" + v.([]any)[0].(string)
				}
				break
			}
		}
		out = append(out, key+">"+anyString(r["outbound"])+anyString(r["action"]))
	}
	return out
}

func anyString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func TestRuleOrderOfSpec62(t *testing.T) {
	b, _ := Build(testOptions(`C:\ProgramData\SplitLane`))
	want := []string{
		"inbound=vpn>reject",
		"ip_version>reject",
		"inbound=probe-vpn>vpnroute", "inbound=probe-direct>directroute",
		"port>hijack-dns",
		"ip_cidr>reject",
		"process_path_regex>directroute", "process_path_regex>vpnroute",
		"ip_is_private>directroute",
		"clash_mode=alldirect>directroute",
		"rule_set=lane-direct>directroute", "rule_set=lane-vpn>vpnroute",
		"clash_mode=direct>directroute", "clash_mode=direct-localdns>directroute",
		"clash_mode=vpn>vpnroute", "clash_mode=vpn-localdns>vpnroute",
	}
	if got := summary(t, b); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("rule order:\n%v\nwant\n%v", got, want)
	}
}

// wantNodeInboundRule is rule 0 of spec 6.2 as compact JSON: the flows a tailnet peer opens to the
// node's own address come with the inbound "vpn" and the destination 127.0.0.1 (rejectNodeInbound).
const wantNodeInboundRule = `{"action":"reject","inbound":["vpn"]}`

// firstRule is route.rules[0] of a config as compact JSON, "" when there are no rules.
func firstRule(t *testing.T, config []byte) string {
	t.Helper()
	var c struct {
		Route struct {
			Rules []json.RawMessage `json:"rules"`
		} `json:"route"`
	}
	if err := json.Unmarshal(config, &c); err != nil {
		t.Fatal(err)
	}
	if len(c.Route.Rules) == 0 {
		return ""
	}
	var b bytes.Buffer
	if err := json.Compact(&b, c.Route.Rules[0]); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// Spec 6.2 rule 0: in every mode, with or without the development pin, the flows from tailnet
// peers are rejected before any rule could route them (rule 7 would send 127.0.0.1 direct).
func TestNodeInboundRejectedFirst(t *testing.T) {
	for _, mode := range Modes {
		for _, pin := range []bool{false, true} {
			o := testOptions(`C:\ProgramData\SplitLane`)
			o.Mode, o.DevPin = mode, pin
			b, err := Build(o)
			if err != nil {
				t.Fatal(err)
			}
			if got := firstRule(t, b); got != wantNodeInboundRule {
				t.Errorf("mode %s pin %v: the first rule is %s, want %s", mode, pin, got, wantNodeInboundRule)
			}
		}
	}
}

func TestDevPinOnlyWhenAsked(t *testing.T) {
	plain, _ := Build(testOptions(`C:\ProgramData\SplitLane`))
	if bytes.Contains(plain, []byte("claude.exe")) || bytes.Contains(plain, []byte("160.79.104.0/23")) {
		t.Fatal("the product config must not carry the development pin")
	}
	o := testOptions(`C:\ProgramData\SplitLane`)
	o.DevPin = true
	pinned, _ := Build(o)
	got := summary(t, pinned)
	pin, firstMode := -1, -1
	for i, s := range got {
		if strings.HasPrefix(s, "process_name>vpn") && pin < 0 {
			pin = i
		}
		if strings.HasPrefix(s, "clash_mode=") && firstMode < 0 {
			firstMode = i
		}
	}
	if pin < 0 || got[pin+1] != "domain_suffix>vpnroute" || pin > firstMode {
		t.Fatalf("the pin must come before every mode rule: %v", got)
	}
}

// Spec 6.1: the core log is written at level info; debug comes only from the development flag
// --core-log-level. Any other level is refused, also the names the core itself would take.
func TestLogLevel(t *testing.T) {
	for _, c := range []struct{ level, want string }{{"", `"level": "info"`}, {"info", `"level": "info"`}, {"debug", `"level": "debug"`}} {
		o := testOptions(`C:\ProgramData\SplitLane`)
		o.LogLevel = c.level
		b, err := Build(o)
		if err != nil || !bytes.Contains(b, []byte(c.want)) {
			t.Errorf("level %q: %v, want %s", c.level, err, c.want)
		}
	}
	for _, level := range []string{"trace", "warn", "INFO"} {
		o := testOptions(`C:\ProgramData\SplitLane`)
		o.LogLevel = level
		if _, err := Build(o); err == nil || !strings.Contains(err.Error(), "log level must be info or debug") {
			t.Errorf("level %q: want an error, got %v", level, err)
		}
	}
}

func TestBuildRejectsBadOptions(t *testing.T) {
	bad := []func(*Options){
		func(o *Options) { o.Mode = "global" },
		func(o *Options) { o.ExitNode = "exit-node-name" },
		func(o *Options) { o.Ports.API = o.Ports.Clash },
		func(o *Options) { o.Hostname = "" },
		func(o *Options) { o.ClashSecret = "short" },
		func(o *Options) { o.HelperPath = "splitlane-helper.exe" },
	}
	for i, change := range bad {
		o := testOptions(`C:\ProgramData\SplitLane`)
		change(&o)
		if _, err := Build(o); err == nil {
			t.Errorf("case %d: want an error", i)
		}
	}
}

// TestCoreAcceptsConfigs runs `sing-box check` when SPLITLANE_CORE names sing-box.exe.
// The check reads the config and the rule files only; it starts nothing.
func TestCoreAcceptsConfigs(t *testing.T) {
	core := os.Getenv("SPLITLANE_CORE")
	if core == "" {
		t.Skip("SPLITLANE_CORE is not set")
	}
	dir := t.TempDir()
	paths := PathsFor(dir)
	for _, d := range []string{paths.RulesDir, paths.LogsDir, paths.StateDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	env := rules.Env{WindowsDir: `C:\Windows`, ProgramFiles: `C:\Program Files`, ProgramFilesX86: `C:\Program Files (x86)`}
	apps, rejected, err := rules.Check([]protocol.AppRule{
		{ID: "a", Exe: `C:\Games\Steam\steam.exe`, Matcher: protocol.Matcher{Kind: protocol.KindFolder}, Lane: protocol.LaneDirect},
		{ID: "b", Exe: `C:\Games\Steam\Кириллица (x86) [1]\game.exe`, Matcher: protocol.Matcher{Kind: protocol.KindFile}, Lane: protocol.LaneVPN},
		{ID: "c", Matcher: protocol.Matcher{Kind: protocol.KindSystem}, Lane: protocol.LaneVPN},
	}, env)
	if err != nil || rejected != nil {
		t.Fatal(err, rejected)
	}
	direct, vpn, err := rules.Files(apps, env)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(paths.RulesDirect, direct, 0o644)
	os.WriteFile(paths.RulesVPN, vpn, 0o644)
	for _, mode := range Modes {
		for _, pin := range []bool{false, true} {
			o := testOptions(dir)
			o.Mode, o.DevPin = mode, pin
			b, err := Build(o)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(paths.Config, b, 0o644); err != nil {
				t.Fatal(err)
			}
			out, err := exec.Command(core, "check", "-c", paths.Config, "-D", dir).CombinedOutput()
			if err != nil {
				t.Errorf("mode %s pin %v: sing-box check: %v\n%s", mode, pin, err, out)
			}
		}
	}
}
