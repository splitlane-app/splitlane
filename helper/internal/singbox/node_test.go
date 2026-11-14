package singbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/protocol"
)

// Output of `sing-box api tailscale ...` of 1.14.4 when stdout is not a terminal, as
// cmd/sing-box/cmd_api_tailscale_*.go and cmd_api_output.go print it. `status` is the endpoint tag
// and a block of "  <label>:" lines, the values padded to the longest label plus three. `peer list`
// and `exit-node list` are rows of tab-separated cells, "-" for an empty one; `peer list` starts
// with the node itself, its IP cell is the first of its tailnet addresses. Values are invented.
const (
	testAPISecret = "fedcba9876543210fedcba9876543210"

	statusRunning    = "vpn\n  State:    Running\n  Network:  example.org\n"
	peersWithAddress = "pc-splitlane.example.ts.net\t100.101.102.103\tyes\nexit-a.example.ts.net\t100.101.102.104\tyes\n"
	peersNoAddress   = "pc\t-\tno\n"
)

type apiCall struct {
	env  []string
	exe  string
	args []string
}

// scripted is a Node whose runner answers from outputs (by the arguments after "api") and keeps
// every call.
func scripted(t *testing.T, outputs map[string]string) (*Node, *[]apiCall) {
	t.Helper()
	n := NewNode(`C:\core\sing-box.exe`, 40004, testAPISecret)
	var calls []apiCall
	n.Run = func(ctx context.Context, env []string, exe string, args ...string) ([]byte, error) {
		calls = append(calls, apiCall{env: env, exe: exe, args: args})
		out, ok := outputs[strings.Join(args[1:], " ")]
		if !ok {
			return nil, fmt.Errorf("unexpected command %v", args)
		}
		return []byte(out), nil
	}
	return n, &calls
}

// fakeAPICommand plays `sing-box api` when the test binary runs as the core (TestMain): the URL
// and the secret come from the environment, as for the real CLI without --url and --secret, and a
// failure is the core's FATAL line on stderr, colored as the core colors it.
func fakeAPICommand(args []string) {
	if os.Getenv("BOX_API_URL") != "127.0.0.1:40004" || os.Getenv("BOX_API_SECRET") != testAPISecret {
		os.Stderr.WriteString("\x1b[31mFATAL\x1b[0m[0000] Unauthenticated: invalid secret\n")
		os.Exit(1)
	}
	switch strings.Join(args, " ") {
	case "tailscale status":
		os.Stdout.WriteString(statusRunning)
	case "tailscale peer list":
		os.Stdout.WriteString(peersWithAddress)
	default:
		os.Stderr.WriteString("FATAL[0000] unknown command\n")
		os.Exit(1)
	}
}

func TestNodeStateParse(t *testing.T) {
	cases := []struct {
		name, status, peers string
		want                engine.NodeState
	}{
		{"no state", "vpn\n  State:  NoState\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStopped}},
		{"stopped", "vpn\n  State:  Stopped\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStopped}},
		{"empty state", "vpn\n  State:  -\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStopped}},
		{"another user", "vpn\n  State:  InUseOtherUser\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStopped}},
		{"login link", "vpn\n  State:   NeedsLogin\n  Log in:  https://login.example/a1b2\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: "https://login.example/a1b2"}},
		{"expired, no link yet", "vpn\n  State:    NeedsLogin\n  Network:  example.org\n", peersWithAddress,
			engine.NodeState{Backend: protocol.BackendNeedsLogin, HasAddress: true}},
		{"approval", "vpn\n  State:    NeedsMachineAuth\n  Network:  example.org\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendNeedsApproval}},
		{"starting without an address", "vpn\n  State:  Starting\n", peersNoAddress,
			engine.NodeState{Backend: protocol.BackendStarting}},
		{"starting", "vpn\n  State:    Starting\n  Network:  example.org\n", peersWithAddress,
			engine.NodeState{Backend: protocol.BackendStarting, HasAddress: true}},
		{"running, every label", "vpn\n  State:    Running\n  Network:  example.org\n  Auth:     auth key\n", peersWithAddress,
			engine.NodeState{Backend: protocol.BackendRunning, HasAddress: true}},
		{"no peers at all", statusRunning, "",
			engine.NodeState{Backend: protocol.BackendRunning}},
	}
	for _, c := range cases {
		n, _ := scripted(t, map[string]string{"tailscale status": c.status, "tailscale peer list": c.peers})
		got, err := n.State(context.Background())
		if err != nil || got != c.want {
			t.Errorf("%s: %+v %v, want %+v", c.name, got, err, c.want)
		}
	}
}

func TestExitNodesParse(t *testing.T) {
	n, _ := scripted(t, map[string]string{"tailscale exit-node list": "" +
		"exit-a.example.ts.net\t100.101.102.104\tyes\tselected\n" +
		"exit-b.example.ts.net\t100.101.102.105\tno\t-\n" +
		"exit-c.example.ts.net\t-\tyes\t-\n" +
		"exit-d.example.ts.net\tfd7a:115c:a1e0::5\tyes\t-\n"})
	got, err := n.ExitNodes(context.Background())
	want := []protocol.ExitNode{
		{ID: "100.101.102.104", Name: "exit-a", Online: true},
		{ID: "100.101.102.105", Name: "exit-b", Online: false},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v %v", got, err)
	}
	empty, _ := scripted(t, map[string]string{"tailscale exit-node list": ""})
	if got, err := empty.ExitNodes(context.Background()); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("no exit nodes is an empty list: %#v %v", got, err)
	}
}

func TestPathParse(t *testing.T) {
	const pong = "pong from exit-a (100.101.102.104) via "
	cases := map[string]struct {
		out  string
		want string
	}{
		"derp region code": {strings.Repeat(pong+"DERP(fra) in 23ms\n", 10) + "direct connection not established\n", protocol.PathRelay},
		"derp region id":   {strings.Repeat(pong+"DERP(4) in 23ms\n", 10) + "direct connection not established\n", protocol.PathRelay},
		"peer relay": {strings.Repeat(pong+"peer relay 198.51.100.7:7777:vni:3 in 12ms\n", 10) +
			"direct connection not established, relayed by peer relay 198.51.100.7:7777:vni:3\n", protocol.PathRelay},
		"direct after derp": {pong + "DERP(fra) in 31ms\n" + pong + "198.51.100.7:41641 in 4ms\n", protocol.PathDirect},
		"nothing":           {"", ""},
	}
	for name, c := range cases {
		n, calls := scripted(t, map[string]string{"tailscale ping 100.101.102.104": c.out})
		got, err := n.Path(context.Background(), "100.101.102.104")
		if err != nil || got != c.want {
			t.Errorf("%s: %q %v, want %q", name, got, err, c.want)
		}
		if len(*calls) != 1 {
			t.Errorf("%s: calls %v", name, *calls)
		}
	}
}

// Plan 0 (б): the URL and the secret go through the environment, so the secret is never on the
// command line of a process that every user can list.
func TestNodeUsesEnvSecret(t *testing.T) {
	n, calls := scripted(t, map[string]string{
		"tailscale status": statusRunning, "tailscale peer list": peersWithAddress,
		"tailscale exit-node list": "", "tailscale ping 100.101.102.104": "", "tailscale logout": "",
	})
	ctx := context.Background()
	n.State(ctx)
	n.ExitNodes(ctx)
	n.Path(ctx, "100.101.102.104")
	if err := n.Logout(ctx); err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, c := range *calls {
		commands = append(commands, strings.Join(c.args, " "))
		if c.exe != `C:\core\sing-box.exe` {
			t.Errorf("exe %s", c.exe)
		}
		if !reflect.DeepEqual(c.env, []string{"BOX_API_URL=127.0.0.1:40004", "BOX_API_SECRET=" + testAPISecret}) {
			t.Errorf("env %v", c.env)
		}
		for _, a := range c.args {
			if strings.Contains(a, testAPISecret) {
				t.Errorf("the secret is on the command line: %v", c.args)
			}
		}
	}
	want := "api tailscale status|api tailscale peer list|api tailscale exit-node list|" +
		"api tailscale ping 100.101.102.104|api tailscale logout"
	if strings.Join(commands, "|") != want {
		t.Fatalf("commands %v", commands)
	}
}

// The real runner, with the test binary as the core: the environment reaches the CLI, and a
// non-zero exit is an error with the core's message, without color codes and without the secret.
func TestNodeErrorIsReturned(t *testing.T) {
	d, _ := fakeDriver(t)
	st, err := d.Node(40004, testAPISecret).State(context.Background())
	if err != nil || st != (engine.NodeState{Backend: protocol.BackendRunning, HasAddress: true}) {
		t.Fatalf("with the run's secret: %+v %v", st, err)
	}
	const wrong = "0000000000000000wrong"
	_, err = d.Node(40004, wrong).State(context.Background())
	if err == nil || !strings.Contains(err.Error(), "FATAL[0000] Unauthenticated: invalid secret") {
		t.Fatalf("a non-zero exit must be an error with the core's message, got %v", err)
	}
	if strings.Contains(err.Error(), "\x1b[") || strings.Contains(err.Error(), wrong) {
		t.Fatalf("color codes or the secret in the error: %q", err)
	}
	failing, _ := scripted(t, nil)
	if nodes, err := failing.ExitNodes(context.Background()); err == nil || nodes != nil {
		t.Fatalf("a failed command gives no list: %v %v", nodes, err)
	}
	if path, err := failing.Path(context.Background(), "100.101.102.104"); err == nil || path != "" {
		t.Fatalf("a failed ping gives no path: %q %v", path, err)
	}
	if err := failing.Logout(context.Background()); err == nil {
		t.Fatal("a failed logout must be an error")
	}
}

// The setup mode in the driver's data folder (spec 7): the same log and node state as the main
// config, the API service of this run.
func TestDriverSetupConfig(t *testing.T) {
	d := &Driver{Exe: `C:\core\sing-box.exe`, DataDir: t.TempDir()}
	b, err := d.BuildSetupConfig(engine.SetupOptions{Hostname: "pc-splitlane", APIPort: 40004, APISecret: testAPISecret})
	if err != nil {
		t.Fatal(err)
	}
	p := coreconfig.PathsFor(d.DataDir)
	for _, want := range []string{`"output": "` + filepath.ToSlash(p.CoreLog) + `"`,
		`"state_directory": "` + filepath.ToSlash(p.StateDir) + `"`, `"hostname": "pc-splitlane"`, `"listen_port": 40004`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("setup config lacks %s", want)
		}
	}
	if strings.Contains(string(b), `"tun"`) {
		t.Fatal("the setup mode has no adapter")
	}
}
