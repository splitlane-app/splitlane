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
	node := c["endpoints"].([]any)[0].(map[string]any)
	if _, ok := node["exit_node"]; ok || node["type"] != "tailscale" || node["tag"] != TagVPN {
		t.Fatalf("node %v", node)
	}
	api := c["services"].([]any)[0].(map[string]any)
	if api["type"] != "api" || api["listen"] != "127.0.0.1" || api["listen_port"] != float64(30004) {
		t.Fatalf("api service %v", api)
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
