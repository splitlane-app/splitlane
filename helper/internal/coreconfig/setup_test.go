package coreconfig

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func testSetupOptions(dataDir string) SetupOptions {
	return SetupOptions{
		DataDir:   dataDir,
		Hostname:  "PC-splitlane",
		APIPort:   30004,
		APISecret: "fedcba9876543210fedcba9876543210",
	}
}

func TestSetupConfigMatchesGolden(t *testing.T) {
	got, err := BuildSetup(testSetupOptions(`C:\ProgramData\SplitLane`))
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "setup.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(got, want) {
		t.Fatalf("config differs from testdata/setup.golden.json:\n%s", got)
	}
}

// Spec 7: no adapter and no route.auto_detect_interface in the setup mode; the node has no exit
// node there, and only the API service listens (no probes, no Clash API).
func TestSetupConfigHasNoAdapter(t *testing.T) {
	b, err := BuildSetup(testSetupOptions(`C:\ProgramData\SplitLane`))
	if err != nil {
		t.Fatal(err)
	}
	var c map[string]any
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if _, ok := c["inbounds"]; ok {
		t.Fatal("the setup mode has no inbounds: no tun adapter, no probes")
	}
	if _, ok := c["experimental"]; ok {
		t.Fatal("the setup mode has no clash_api")
	}
	route := c["route"].(map[string]any)
	if _, ok := route["auto_detect_interface"]; ok {
		t.Fatal("route.auto_detect_interface must not be set in the setup mode")
	}
	if route["default_domain_resolver"] != "dns-local" {
		t.Fatalf("route %v", route)
	}
	// rule 0 of spec 6.2 is the only rule: without it the default outbound direct would carry a
	// tailnet peer's flow to 127.0.0.1 of the PC
	if rules, _ := route["rules"].([]any); len(rules) != 1 || firstRule(t, b) != wantNodeInboundRule {
		t.Fatalf("route.rules %v, want only %s", route["rules"], wantNodeInboundRule)
	}
	node := c["endpoints"].([]any)[0].(map[string]any)
	if _, ok := node["exit_node"]; ok || node["type"] != "tailscale" || node["tag"] != TagVPN {
		t.Fatalf("node %v", node)
	}
	api := c["services"].([]any)[0].(map[string]any)
	if api["type"] != "api" || api["listen"] != "127.0.0.1" || api["listen_port"] != float64(30004) {
		t.Fatalf("api service %v", api)
	}
}

// Spec 7: the node of the setup mode resolves names to IPv4 only, as the main config does. With
// A and AAAA asked together, a lost A answer left the node with the IPv6 addresses of the control
// server only, and IPv6 that leads nowhere (the system exit node without IPv6) held it for minutes
// with backend stopped; with A alone a lost answer is a failed lookup, retried in 10 s.
func TestSetupConfigResolvesIPv4Only(t *testing.T) {
	b, err := BuildSetup(testSetupOptions(`C:\ProgramData\SplitLane`))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		DNS map[string]any `json:"dns"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	if c.DNS["strategy"] != "ipv4_only" {
		t.Fatalf("dns %v: want strategy ipv4_only", c.DNS)
	}
}

// Spec 6.1: the setup core writes a log of its own, so that a setup run never sets aside the log of
// a main core that failed (review of plan 1B, final, minor 4).
func TestSetupConfigWritesItsOwnLog(t *testing.T) {
	b, err := BuildSetup(testSetupOptions(`C:\ProgramData\SplitLane`))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Log struct {
			Output string `json:"output"`
		} `json:"log"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	p := PathsFor(`C:\ProgramData\SplitLane`)
	if c.Log.Output != "C:/ProgramData/SplitLane/logs/core-setup.log" || p.SetupLog != `C:\ProgramData\SplitLane\logs\core-setup.log` {
		t.Fatalf("setup log %q, paths %q", c.Log.Output, p.SetupLog)
	}
}

// The setup mode follows --core-log-level as the main config does (spec 6.1).
func TestSetupLogLevel(t *testing.T) {
	for _, c := range []struct{ level, want string }{{"", `"level": "info"`}, {"info", `"level": "info"`}, {"debug", `"level": "debug"`}} {
		o := testSetupOptions(`C:\ProgramData\SplitLane`)
		o.LogLevel = c.level
		b, err := BuildSetup(o)
		if err != nil || !bytes.Contains(b, []byte(c.want)) {
			t.Errorf("level %q: %v, want %s", c.level, err, c.want)
		}
	}
	o := testSetupOptions(`C:\ProgramData\SplitLane`)
	o.LogLevel = "trace"
	if _, err := BuildSetup(o); err == nil || !strings.Contains(err.Error(), "log level must be info or debug") {
		t.Errorf("level trace: want an error, got %v", err)
	}
}

func TestSetupConfigRejectsBadOptions(t *testing.T) {
	bad := []func(*SetupOptions){
		func(o *SetupOptions) { o.DataDir = `SplitLane` },
		func(o *SetupOptions) { o.Hostname = "" },
		func(o *SetupOptions) { o.APIPort = 0 },
		func(o *SetupOptions) { o.APIPort = 65536 },
		func(o *SetupOptions) { o.APISecret = "short" },
	}
	for i, change := range bad {
		o := testSetupOptions(`C:\ProgramData\SplitLane`)
		change(&o)
		if _, err := BuildSetup(o); err == nil {
			t.Errorf("case %d: want an error", i)
		}
	}
}

// TestSetupConfigPassesCheck runs `sing-box check` when SPLITLANE_CORE names sing-box.exe.
// The check reads the config only; it starts nothing.
func TestSetupConfigPassesCheck(t *testing.T) {
	core := os.Getenv("SPLITLANE_CORE")
	if core == "" {
		t.Skip("SPLITLANE_CORE is not set")
	}
	dir := t.TempDir()
	paths := PathsFor(dir)
	for _, d := range []string{paths.LogsDir, paths.StateDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	b, err := BuildSetup(testSetupOptions(dir))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Config, b, 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(core, "check", "-c", paths.Config, "-D", dir).CombinedOutput()
	if err != nil {
		t.Fatalf("sing-box check: %v\n%s", err, out)
	}
}
