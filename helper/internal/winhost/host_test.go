package winhost

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

var (
	ethernet = Iface{LUID: 1, Alias: "Ethernet", Type: 6, Up: true, Hardware: true}
	ours     = Iface{LUID: 2, Alias: "SplitLane", Type: 53, Up: true}
	ts       = Iface{LUID: 3, Alias: "Tailscale", Type: 53, Up: true}
	wsl      = Iface{LUID: 4, Alias: "vEthernet (WSL)", Type: 6, Up: true, Forwarding: true}
)

func TestDefaultRouteOK(t *testing.T) {
	ifaces := []Iface{ethernet, ours, ts}
	if !DefaultRouteOK([]Route{{LUID: 1}}, ifaces) {
		t.Fatal("a default route through Ethernet counts")
	}
	if DefaultRouteOK([]Route{{LUID: 2}, {LUID: 3}}, ifaces) {
		t.Fatal("default routes of SplitLane and Tailscale do not count")
	}
	if DefaultRouteOK([]Route{{LUID: 1, PrefixLen: 1}}, ifaces) || DefaultRouteOK(nil, ifaces) {
		t.Fatal("no default route")
	}
}

func TestPhysicalForwarding(t *testing.T) {
	if PhysicalForwarding([]Iface{ethernet, wsl}) {
		t.Fatal("forwarding on a virtual adapter (WSL, Hyper-V) does not count")
	}
	shared := ethernet
	shared.Forwarding = true
	if !PhysicalForwarding([]Iface{shared}) {
		t.Fatal("forwarding on a physical adapter counts")
	}
	shared.Up = false
	if PhysicalForwarding([]Iface{shared}) {
		t.Fatal("an adapter that is down does not count")
	}
}

func TestTailscaleCatchAll(t *testing.T) {
	if !TailscaleCatchAll([]NRPTRule{{Names: []string{".ts.net"}, Servers: "100.100.100.100"}, {Names: []string{"."}, Servers: "100.100.100.100"}}) {
		t.Fatal("the rule for all names counts")
	}
	if TailscaleCatchAll([]NRPTRule{{Names: []string{".ts.net"}, Servers: "100.100.100.100"}, {Names: []string{"."}, Servers: "10.0.0.1"}}) {
		t.Fatal("only Tailscale's rule for all names counts")
	}
}

func TestFreePortsAndSecret(t *testing.T) {
	ports, err := Host{}.FreePorts(4)
	if err != nil || len(ports) != 4 {
		t.Fatal(ports, err)
	}
	seen := map[int]bool{}
	for _, p := range ports {
		if seen[p] || p <= 0 {
			t.Fatalf("ports %v", ports)
		}
		seen[p] = true
	}
	a, b := Host{}.Secret(), Host{}.Secret()
	if len(a) != 32 || a == b {
		t.Fatalf("secrets %q %q", a, b)
	}
}

func TestFileDescription(t *testing.T) {
	win, _ := windows.GetWindowsDirectory()
	if d := FileDescription(filepath.Join(win, "System32", "kernel32.dll")); d == "" {
		t.Fatal("kernel32.dll has a description")
	}
	exe, _ := os.Executable()
	if d := FileDescription(exe); d != "" {
		t.Fatalf("the test binary has no version resources, got %q", d)
	}
}

// TestLiveReadOnly reads the real system and changes nothing. It runs with SPLITLANE_LIVE_READONLY=1
// and expects a PC with a working network, no sing-box.exe and no SplitLane adapter.
func TestLiveReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	h := Host{}
	ifaces, err := Interfaces()
	if err != nil || len(ifaces) == 0 {
		t.Fatalf("interfaces: %v", err)
	}
	t.Logf("LIVE default route %v, physical forwarding %v, NRPT catch-all %v, sing-box running %v, adapter %v",
		h.HasDefaultRoute(), h.IPForwarding(), h.DNSOverride(), h.CoreRunning(), h.AdapterPresent())
	if !h.HasDefaultRoute() || h.CoreRunning() || h.AdapterPresent() {
		t.Fatal("expected: a default route, no sing-box.exe, no SplitLane adapter")
	}
	env := RulesEnv(nil)
	if env.WindowsDir == "" || env.ProgramFiles == "" || len(env.UserDirs) != 4 || strings.Contains(strings.Join(env.UserDirs, "|"), "||") {
		t.Fatalf("env %+v", env)
	}
}
