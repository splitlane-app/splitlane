package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// verdict: a copy of the VPN lane fails at its first answer from the direct lane's address (that is
// traffic past the VPN); the Plan 0 rule (the first answer right, or two of three) stays for the
// direct lane and for answers from another address under the VPN lane (the exit node's external
// address may change).
func TestVerdict(t *testing.T) {
	cases := []struct {
		seen     []string
		want     string
		ok, done bool
	}{
		{[]string{"vpn"}, "vpn", true, true},
		{[]string{"none", "vpn"}, "vpn", true, true},
		{[]string{"direct"}, "vpn", false, true},
		{[]string{"none", "direct"}, "vpn", false, true},
		{[]string{"direct", "vpn", "vpn"}, "vpn", false, true},
		{[]string{"direct", "vpn", "direct"}, "vpn", false, true},
		{[]string{"other"}, "vpn", false, false},
		{[]string{"other", "vpn", "vpn"}, "vpn", true, true},
		{[]string{"other", "vpn", "other"}, "vpn", false, true},
		{[]string{"other", "vpn", "direct"}, "vpn", false, true},
		{[]string{"none", "none", "none", "none", "none", "none"}, "vpn", false, true},
		{[]string{"direct"}, "direct", true, true},
		{[]string{"none", "direct"}, "direct", true, true},
		{[]string{"vpn"}, "direct", false, false},
		{[]string{"vpn", "direct", "direct"}, "direct", true, true},
		{[]string{"vpn", "direct", "vpn"}, "direct", false, true},
		{[]string{"none", "none", "none", "none", "none", "none"}, "direct", false, true},
	}
	for _, c := range cases {
		if ok, done := verdict(c.seen, c.want); ok != c.ok || done != c.done {
			t.Errorf("%v want=%s: %v %v, expected %v %v", c.seen, c.want, ok, done, c.ok, c.done)
		}
	}
}

func TestLaneName(t *testing.T) {
	v, d := "203.0.113.1", "198.51.100.1"
	s := protocol.State{Addresses: protocol.Addresses{VPN: &v, Direct: &d}}
	for ip, want := range map[string]string{v: "vpn", d: "direct", "192.0.2.9": "other", "": "none"} {
		if got := laneName(ip, s); got != want {
			t.Errorf("%q: %s", ip, got)
		}
	}
}

// fakeProgramData points the product root at a temporary folder for the test and returns the
// product's data folder under it: a regression then writes only into a temporary folder.
func fakeProgramData(t *testing.T) string {
	t.Helper()
	if real := programData(); !filepath.IsAbs(real) {
		t.Fatalf("the known folder ProgramData: %q", real)
	}
	root := t.TempDir()
	old := programData
	programData = func() string { return root }
	t.Cleanup(func() { programData = old })
	return filepath.Join(root, "SplitLane")
}

func TestSeed(t *testing.T) {
	product := fakeProgramData(t)
	var out strings.Builder
	if code := run([]string{"seed", product, "100.64.0.1"}, &out); code != exitUsage {
		t.Fatalf("the product folder must be refused: %d %s", code, out.String())
	}
	if _, err := os.Stat(product); err == nil {
		t.Fatal("nothing may be written into the product folder")
	}
	dir := t.TempDir()
	if code := run([]string{"seed", dir, "192.0.2.1"}, &out); code != exitUsage {
		t.Fatal("an address outside the tailnet must be refused")
	}
	if code := run([]string{"seed", dir, "100.64.0.1", "pc-splitlane-dev"}, &out); code != exitOK {
		t.Fatalf("%d %s", code, out.String())
	}
	s, _ := store.Dir{Path: dir}.LoadState()
	if !s.LoggedIn || s.SelectedExitNode != "100.64.0.1" || s.DeviceName != "pc-splitlane-dev" {
		t.Fatalf("%+v", s)
	}
}

// fakeHelper answers like the engine: enable goes starting -> on through state events.
type fakeHelper struct {
	mu  sync.Mutex
	srv *pipe.Server
	st  protocol.State

	silent   map[string]bool      // methods that get no answer until release is closed
	release  chan struct{}        // lets the silent methods return
	stateErr *protocol.Error      // state.get is rejected with it
	login    []protocol.Tailscale // the node's states after tailscale.login, one state event each, 30 ms apart

	activity *protocol.Activity                 // the live activity (spec 8.4); nil: no core with the adapter runs
	details  map[string]protocol.ActivityDetail // the detail of each group key
	archive  string                             // the path diagnostics.export answers with; "": it fails
	params   map[string]string                  // the params of the last call of each method
}

func (f *fakeHelper) paramsOf(method string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.params[method]
}

// Activity is the engine's summary for the pipe's activity events: the detail of detailFor when it has one.
func (f *fakeHelper) Activity(detailFor string) (protocol.Activity, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.activity == nil {
		return protocol.Activity{}, false
	}
	a := *f.activity
	if d, ok := f.details[detailFor]; ok {
		a.Detail = &d
	}
	return a, true
}

func (f *fakeHelper) set(phase string) {
	f.mu.Lock()
	f.st.Phase = phase
	s := f.st
	f.mu.Unlock()
	f.srv.Broadcast(s)
}

func (f *fakeHelper) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	if f.silent[method] {
		<-f.release
		return nil, protocol.Errorf(protocol.CodeInternal, "too late")
	}
	f.mu.Lock()
	if f.params == nil {
		f.params = map[string]string{}
	}
	f.params[method] = string(params)
	f.mu.Unlock()
	switch method {
	case protocol.MethodProxyAdd:
		return protocol.ProxyAddResult{Added: []string{"00112233445566aa"}, Existing: 2, Rejected: []protocol.ProxyRejection{{Line: 2, Code: "bad_link"}}, RejectedMore: 3}, nil
	case protocol.MethodProxyRefresh:
		return nil, protocol.Errorf("fetch_timeout", "subscription not refreshed: fetch_timeout")
	case protocol.MethodProxyRemove, protocol.MethodProxySelect:
		return struct{}{}, nil
	case protocol.MethodStateGet:
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.stateErr != nil {
			return nil, f.stateErr
		}
		return f.st, nil
	case protocol.MethodEnable:
		f.set(protocol.PhaseStarting)
		go func() {
			time.Sleep(50 * time.Millisecond)
			f.set(protocol.PhaseOn)
		}()
		return struct{}{}, nil
	case protocol.MethodLanesSet:
		return nil, &protocol.Error{Code: protocol.CodeRuleRejected, Message: "rules rejected",
			Rules: []protocol.RuleRejection{{ID: "x", Reason: protocol.RejectProtected}}}
	case protocol.MethodLogin:
		go func() {
			for _, ts := range f.login {
				time.Sleep(30 * time.Millisecond)
				f.mu.Lock()
				f.st.Tailscale = ts
				s := f.st
				f.mu.Unlock()
				f.srv.Broadcast(s)
			}
		}()
		return struct{}{}, nil
	case protocol.MethodDiagnosticsExport:
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.archive == "" {
			return nil, protocol.Errorf(protocol.CodeInternal, "diagnostics.export: the disk is full")
		}
		return protocol.ExportResult{Path: f.archive}, nil
	}
	return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown")
}

func (f *fakeHelper) Hello() protocol.HelloResult {
	return protocol.HelloResult{Protocol: protocol.Version}
}
func (f *fakeHelper) SetClients(int) {}

func testPipeName() string { return fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63()) }

// serveFake opens the pipe name with the fake helper behind it.
func serveFake(name string) (*pipe.Server, error) {
	vpn, direct := "203.0.113.1", "198.51.100.1"
	return serveState(name, protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{},
		Addresses: protocol.Addresses{VPN: &vpn, Direct: &direct}})
}

// serveState opens the pipe name with a fake helper in state st.
func serveState(name string, st protocol.State) (*pipe.Server, error) {
	return serveHelper(name, &fakeHelper{st: st})
}

// serveHelper opens the pipe name with the fake helper f behind it.
func serveHelper(name string, f *fakeHelper) (*pipe.Server, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	srv, err := pipe.Listen(name, u.User.Sid.String(), f, nil, nil)
	if err != nil {
		return nil, err
	}
	f.srv = srv
	go srv.Serve()
	return srv, nil
}

func fakePipe(t *testing.T) string {
	t.Helper()
	name := testPipeName()
	srv, err := serveFake(name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	return name
}

// A helper that was just launched opens its pipe a moment later (Task 23, step 7): slctl waits.
func TestWaitsForThePipe(t *testing.T) {
	name := testPipeName()
	opened := make(chan *pipe.Server, 1)
	go func() {
		time.Sleep(500 * time.Millisecond)
		srv, _ := serveFake(name)
		opened <- srv
	}()
	var out strings.Builder
	code := run([]string{"-pipe", name, "-timeout", "10s", "state"}, &out)
	srv := <-opened
	if srv == nil {
		t.Fatal("the test pipe was not created")
	}
	defer srv.Close()
	if code != exitOK || !strings.Contains(out.String(), "SLCTL: phase=off") {
		t.Fatalf("%d\n%s", code, out.String())
	}
}

// reset-dev clears "was on", the core identity and the journal of a development folder only.
func TestResetDev(t *testing.T) {
	product := fakeProgramData(t)
	var out strings.Builder
	if code := run([]string{"reset-dev", product}, &out); code != exitUsage {
		t.Fatalf("the product folder must be refused: %d %s", code, out.String())
	}
	dir := t.TempDir()
	if code := run([]string{"reset-dev", dir}, &out); code != exitUsage {
		t.Fatal("a folder without state.json must be refused")
	}
	d := store.Dir{Path: dir}
	s := store.Defaults()
	s.LoggedIn, s.SelectedExitNode, s.WasOn = true, "100.64.0.1", true
	s.Core = &store.CoreIdentity{PID: 42, Created: 7, Path: `C:\core\sing-box.exe`}
	d.SaveState(s)
	d.SaveJournal(store.Journal{ExitNode: "100.64.0.1", AllowLAN: true})
	out.Reset()
	if code := run([]string{"reset-dev", dir}, &out); code != exitOK || !strings.Contains(out.String(), "RESET-OK") {
		t.Fatalf("%d %s", code, out.String())
	}
	got, _ := d.LoadState()
	j, _ := d.LoadJournal()
	if got.WasOn || got.Core != nil || j != nil || !got.LoggedIn || got.SelectedExitNode != "100.64.0.1" {
		t.Fatalf("state %+v journal %+v", got, j)
	}
}

// expect-lanes (Task 23, steps 5, 6, 8): the address of each copy against state.addresses by the
// Plan 0 rule; the copies of curl.exe are played by a scripted egress.
func TestExpectLanes(t *testing.T) {
	name := fakePipe(t)
	var mu sync.Mutex
	answers := map[string][]string{
		`C:\x\curl-vpn.exe`:    {"203.0.113.1"},                                 // right at once
		`C:\x\curl-direct.exe`: {"203.0.113.1", "198.51.100.1", "203.0.113.1"},  // two of three wrong
		`C:\x\curl-leak.exe`:   {"198.51.100.1", "203.0.113.1", "203.0.113.1"},  // VPN lane: one direct answer is a leak
		`C:\x\curl-late.exe`:   {"203.0.113.1", "198.51.100.1", "198.51.100.1"}, // direct lane: two of three right
	}
	old := egress
	egress = func(exe, url string) string {
		mu.Lock()
		defer mu.Unlock()
		next := answers[exe]
		if len(next) == 0 {
			return ""
		}
		answers[exe] = next[1:]
		return next[0]
	}
	t.Cleanup(func() { egress = old })
	var out strings.Builder
	code := run([]string{"-pipe", name, "expect-lanes", `C:\x\curl-vpn.exe=vpn`, `C:\x\curl-direct.exe=direct`,
		`C:\x\curl-leak.exe=vpn`, `C:\x\curl-late.exe=direct`}, &out)
	text := out.String()
	for _, line := range []string{
		"LANE-OK curl-vpn.exe vpn seen=[vpn]",
		"LANE-WRONG curl-direct.exe want=direct seen=[vpn direct vpn]",
		"LANE-WRONG curl-leak.exe want=vpn seen=[direct]",
		"LANE-OK curl-late.exe direct seen=[vpn direct direct]",
	} {
		if !strings.Contains(text, line) {
			t.Errorf("missing %q", line)
		}
	}
	if code != exitFailed || t.Failed() {
		t.Fatalf("%d\n%s", code, text)
	}
	for _, bad := range []string{`C:\x\curl-vpn.exe`, `C:\x\curl-vpn.exe=tunnel`} {
		if code := run([]string{"-pipe", name, "expect-lanes", bad}, &out); code != exitUsage {
			t.Errorf("%s: %d", bad, code)
		}
	}
	same := "203.0.113.1"
	equal := testPipeName()
	srv, err := serveState(equal, protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{},
		Addresses: protocol.Addresses{VPN: &same, Direct: &same}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	out.Reset()
	if code := run([]string{"-pipe", equal, "expect-lanes", `C:\x\curl-vpn.exe=vpn`}, &out); code != exitFailed ||
		!strings.Contains(out.String(), "lane addresses are unknown or equal") {
		t.Fatalf("equal addresses: %d\n%s", code, out.String())
	}
}

func TestStateIsMasked(t *testing.T) {
	name := fakePipe(t)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "state"}, &out); code != exitOK {
		t.Fatalf("%d\n%s", code, out.String())
	}
	if strings.Contains(out.String(), "203.0.113.1") || !strings.Contains(out.String(), "SLCTL: phase=off") || !strings.Contains(out.String(), "vpn=<ip>") {
		t.Fatalf("output:\n%s", out.String())
	}
	out.Reset()
	run([]string{"-pipe", name, "-show-addresses", "state"}, &out)
	if !strings.Contains(out.String(), "vpn=203.0.113.1") {
		t.Fatalf("output:\n%s", out.String())
	}
}

// The summary line of state, enable, wait and disable ends with the connector, which tasks 15-17 read:
// the backend, the path to the exit node or null, the number of exit nodes; then the connector of stage 3
// and the number of its servers. The login link never goes there: slctl login prints it once, to the
// user's console.
func TestSummaryShowsTheConnector(t *testing.T) {
	link, path := "https://login.example/a/1f2e3d", protocol.PathRelay
	name := testPipeName()
	srv, err := serveState(name, protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{}, Connector: protocol.ConnectorTailscale,
		Tailscale: protocol.Tailscale{Backend: protocol.BackendRunning, AuthURL: &link, Path: &path,
			ExitNodes: []protocol.ExitNode{{ID: "100.64.0.1", Name: "exit-a", Online: true}, {ID: "100.64.0.2", Name: "exit-b"}}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "state"}, &out); code != exitOK {
		t.Fatalf("%d\n%s", code, out.String())
	}
	_, line, _ := strings.Cut(out.String(), "SLCTL: ")
	if !strings.HasSuffix(line, " restoreOnDisable=false backend=running path=relay exitNodes=2 connector=tailscale servers=0\n") || strings.Contains(line, link) {
		t.Fatalf("output:\n%s", out.String())
	}
	if got := summary(protocol.State{Connector: protocol.ConnectorProxy, Tailscale: protocol.Tailscale{Backend: protocol.BackendExpired},
		Proxy: protocol.Proxy{Servers: []protocol.ProxyServer{{ID: "0123456789abcdef"}}}}); !strings.HasSuffix(got, " backend=expired path=null exitNodes=0 connector=proxy servers=1") {
		t.Fatalf("summary %q", got)
	}
}

// Addition of plan 1B: the RESULT lines carry the raw JSON of the answers, tailscale.authUrl of state.get
// among them. The login link is masked there as in every line slctl prints, with -show-addresses too;
// only slctl login prints it, once, in its own line (TestLoginPrintsTheLinkOnce).
func TestResultLinesHideTheLoginLink(t *testing.T) {
	link := "https://login.example/a/1f2e3d?next=%2F&x=1"
	name := testPipeName()
	srv, err := serveState(name, protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{},
		Tailscale: protocol.Tailscale{Backend: protocol.BackendNeedsLogin, AuthURL: &link}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	for _, args := range [][]string{{"state"}, {"-show-addresses", "state"}, {"call", "state.get"}} {
		var out strings.Builder
		if code := run(append([]string{"-pipe", name}, args...), &out); code != exitOK {
			t.Fatalf("%v: %d\n%s", args, code, out.String())
		}
		if text := out.String(); strings.Contains(text, "1f2e3d") || !strings.Contains(text, `"authUrl":"<login link>"`) {
			t.Errorf("%v:\n%s", args, text)
		}
	}
	for line, want := range map[string]string{
		`RESULT {"authUrl":null,"backend":"stopped"}`:  `RESULT {"authUrl":null,"backend":"stopped"}`,
		`{"authUrl":"https://x.example/a\"b\\","n":1}`: `{"authUrl":"<login link>","n":1}`,
		`{"authUrl" : "https://x.example/"}`:           `{"authUrl":"<login link>"}`,
	} {
		if got := hideLoginLink(line); got != want {
			t.Errorf("%s: %s", line, got)
		}
	}
}

// slctl activity (spec 8.4, task 16): activity.subscribe, then one ACTIVITY line per event for -seconds,
// the busiest group as top (its name, or its key when it has none); with -detail also the DETAIL line of
// that group. Exit code 0 when at least one event came, 3 when none did (no core with the adapter runs).
func TestActivityCommand(t *testing.T) {
	steam := "0b5c2a6e-6a43-4c55-9d43-2f1f4a0c9e11"
	f := &fakeHelper{st: protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{}},
		activity: &protocol.Activity{
			Lanes: protocol.ActivityLanes{VPN: protocol.Traffic{Down: 5000, Up: 300}, Direct: protocol.Traffic{Down: 400, Up: 20}},
			Apps: []protocol.ActivityApp{
				{Key: `C:\Games\Game\`, Kind: protocol.KindApp, Name: "Game", Lane: protocol.LaneVPN, Down: 5000, Up: 300, Connections: 3},
				{Key: steam, Kind: protocol.KindApp, Name: "Steam Client", RuleID: &steam, Lane: protocol.LaneDirect, Down: 400, Up: 20, Connections: 2},
				{Key: protocol.KeyUnattributed, Kind: protocol.KindUnattributed, Lane: protocol.LaneVPN},
			}},
		details: map[string]protocol.ActivityDetail{steam: {Key: steam, Exes: []string{`C:\Games\Steam\steam.exe`, `C:\Games\Steam\bin\helper.exe`},
			Connections: []protocol.ActivityConn{{Host: "203.0.113.7", Lane: protocol.LaneDirect, Down: 400, Up: 20}}}},
	}
	name := testPipeName()
	srv, err := serveHelper(name, f)
	if err != nil {
		t.Fatal(err)
	}
	go srv.RunActivity(f, 50*time.Millisecond)
	t.Cleanup(srv.Close)
	var out strings.Builder
	code := run([]string{"-pipe", name, "activity", "-seconds", "1"}, &out)
	text := out.String()
	if code != exitOK || !strings.Contains(text, "ACTIVITY vpn=5000/300 direct=400/20 apps=3 top=Game:vpn:5300\n") || strings.Contains(text, "DETAIL") {
		t.Fatalf("exit %d:\n%s", code, text)
	}
	out.Reset()
	code = run([]string{"-pipe", name, "activity", "-seconds", "1", "-detail", steam}, &out)
	text = out.String()
	if code != exitOK || !strings.Contains(text, "ACTIVITY vpn=5000/300") || !strings.Contains(text, "DETAIL key="+steam+" exes=2 connections=1\n") {
		t.Fatalf("with -detail, exit %d:\n%s", code, text)
	}
	if got := activityLine(protocol.Activity{Apps: []protocol.ActivityApp{{Key: protocol.KeyUnattributed, Lane: protocol.LaneVPN, Down: 1, Up: 2}}}); got != "ACTIVITY vpn=0/0 direct=0/0 apps=1 top=unattributed:vpn:3" {
		t.Fatalf("a group without a name: %q", got)
	}
	f.mu.Lock()
	f.activity = nil
	f.mu.Unlock()
	out.Reset()
	start := time.Now()
	code = run([]string{"-pipe", name, "activity", "-seconds", "1"}, &out)
	if text = out.String(); code != exitTimeout || strings.Contains(text, "ACTIVITY") || !strings.Contains(text, "SLCTL: no activity event") {
		t.Fatalf("no core: exit %d:\n%s", code, text)
	}
	if waited := time.Since(start); waited < 900*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("waited %s for -seconds 1", waited)
	}
	if code := run([]string{"-pipe", name, "activity", "-seconds", "0"}, &out); code != exitUsage {
		t.Fatalf("-seconds 0: %d", code)
	}
}

// slctl export (spec 5.1): diagnostics.export, then the path of the archive in one line, exit code 0. An
// error response is exit code 1 with its ERROR line; an argument is a usage error.
func TestExportCommand(t *testing.T) {
	path := `C:\ProgramData\SplitLane\export\splitlane-diagnostics-20261114-120000.zip`
	f := &fakeHelper{st: protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{}}, archive: path}
	name := testPipeName()
	srv, err := serveHelper(name, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "export"}, &out); code != exitOK || !strings.Contains(out.String(), "SLCTL: export written: "+path+"\n") {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	f.mu.Lock()
	f.archive = ""
	f.mu.Unlock()
	out.Reset()
	if code := run([]string{"-pipe", name, "export"}, &out); code != exitFailed ||
		!strings.Contains(out.String(), "ERROR internal diagnostics.export: the disk is full") || strings.Contains(out.String(), "export written") {
		t.Fatalf("a failed export: exit %d:\n%s", code, out.String())
	}
	if code := run([]string{"-pipe", name, "export", "now"}, &out); code != exitUsage {
		t.Fatalf("an argument: exit %d", code)
	}
}

func TestEnableWaitsForOn(t *testing.T) {
	name := fakePipe(t)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "-timeout", "10s", "enable"}, &out); code != exitOK {
		t.Fatalf("%d\n%s", code, out.String())
	}
	text := out.String()
	if !strings.Contains(text, "EVENT state phase=starting") || !strings.Contains(text, "SLCTL: phase=on") {
		t.Fatalf("output:\n%s", text)
	}
}

// slctl login (spec 7): tailscale.login, then the state events. Each change of the backend is printed;
// the login link once, though several states carry it; needs_approval is not the end. The command ends
// with exit code 0 when the node is logged in: backend starting or running without a link.
func TestLoginPrintsTheLinkOnce(t *testing.T) {
	link := "https://login.example/a/1f2e3d"
	name := testPipeName()
	srv, err := serveHelper(name, &fakeHelper{
		st: protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{}, Tailscale: protocol.Tailscale{Backend: protocol.BackendStopped}},
		login: []protocol.Tailscale{
			{Backend: protocol.BackendNeedsLogin, AuthURL: &link},
			{Backend: protocol.BackendNeedsLogin, AuthURL: &link, DeviceName: "pc-splitlane"},
			{Backend: protocol.BackendNeedsApproval},
			{Backend: protocol.BackendStarting},
		}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	var out strings.Builder
	code := run([]string{"-pipe", name, "login", "-timeout", "10s"}, &out)
	text := out.String()
	if code != exitOK || strings.Count(text, link) != 1 || !strings.Contains(text, "SLCTL: open this link to log in: "+link) {
		t.Fatalf("exit %d, the link %d times:\n%s", code, strings.Count(text, link), text)
	}
	for _, b := range []string{"needs_login", "needs_approval", "starting"} {
		if n := strings.Count(text, "SLCTL: backend="+b+"\n"); n != 1 {
			t.Errorf("backend=%s printed %d times", b, n)
		}
	}
	if t.Failed() {
		t.Fatalf("output:\n%s", text)
	}
}

func TestWaitForAPhase(t *testing.T) {
	name := fakePipe(t)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "-timeout", "10s", "wait", "off,error"}, &out); code != exitOK {
		t.Fatalf("%d\n%s", code, out.String())
	}
	if code := run([]string{"-pipe", name, "-timeout", "2s", "wait", "on"}, &out); code != exitTimeout {
		t.Fatalf("a phase that never comes is a timeout: %d\n%s", code, out.String())
	}
}

func TestErrorsAndRejections(t *testing.T) {
	name := fakePipe(t)
	file := filepath.Join(t.TempDir(), "lanes.json")
	os.WriteFile(file, []byte(`{"apps":[],"defaultLane":"vpn"}`), 0o644)
	var out strings.Builder
	if code := run([]string{"-pipe", name, "lanes", file}, &out); code != exitFailed || !strings.Contains(out.String(), "ERROR rule_rejected rules rejected rules: x=protected") {
		t.Fatalf("%d\n%s", code, out.String())
	}
	out.Reset()
	// PowerShell 5.1 loses the quotes: {on:true} is not JSON and must not hang the client
	if code := run([]string{"-pipe", name, "-timeout", "5s", "call", "routing.allDirect", "{on:true}"}, &out); code != exitUsage {
		t.Fatalf("invalid params: %d\n%s", code, out.String())
	}
	params := filepath.Join(t.TempDir(), "p.json")
	os.WriteFile(params, []byte(`{"on":true}`), 0o644)
	if code := run([]string{"-pipe", name, "call", "routing.allDirect", "@" + params}, &out); code != exitFailed || !strings.Contains(out.String(), "ERROR unknown_method") {
		t.Fatalf("params from a file: %d\n%s", code, out.String())
	}
	start := time.Now()
	if code := run([]string{"-pipe", `\\.\pipe\SplitLane.Missing.0`, "-timeout", "1s", "state"}, &out); code != exitUsage {
		t.Fatalf("no pipe: %d", code)
	}
	if waited := time.Since(start); waited < 500*time.Millisecond || waited > 5*time.Second {
		t.Fatalf("a missing pipe is waited for up to -timeout, waited %s", waited)
	}
	start = time.Now()
	if code := run([]string{"frobnicate"}, &out); code != exitUsage || time.Since(start) > time.Second {
		t.Fatalf("an unknown command is a usage error at once, without a pipe: %d", code)
	}
}

// slashes writes a path with forward slashes.
func slashes(p string) string { return strings.ReplaceAll(p, `\`, `/`) }

// shortPath is the 8.3 name of an existing path; the path itself when the volume has no short names.
func shortPath(p string) string {
	u, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return p
	}
	buf := make([]uint16, 1024)
	n, err := windows.GetShortPathName(u, &buf[0], uint32(len(buf)))
	if err != nil || int(n) > len(buf) {
		return p
	}
	return windows.UTF16ToString(buf[:n])
}

// The guard of seed and reset-dev (Task 20 review): every spelling of a path below the product root is
// refused, however it is written; a sibling folder that only shares a prefix is not the product root.
// devFolder only reads: nothing is created.
func TestDevFolder(t *testing.T) {
	product := fakeProgramData(t)
	root := programData()
	drive := filepath.VolumeName(root)
	refused := map[string]string{
		"the root":                   root,
		"the product folder":         product,
		"below the product folder":   filepath.Join(product, "sub", "dir"),
		"forward slashes":            slashes(product),
		"dot dot segments":           root + `\other\..\SplitLane`,
		"dot dot out and back in":    root + `\..\` + filepath.Base(root) + `\SplitLane`,
		"another case":               strings.ToUpper(product),
		"a name that starts with ..": root + `\..dev`,
		"extended-length path":       `\\?\` + product,
		"extended-length, slashes":   `//?/` + slashes(product),
		"device path":                `\\.\` + product,
		"administrative share":       `\\localhost\` + drive[:1] + `$` + product[2:],
		"relative":                   `dev`,
		"rooted, no drive":           `\dev`,
		"relative to the drive":      drive + `dev`,
		"empty":                      ``,
	}
	for what, dir := range refused {
		if devFolder(dir) {
			t.Errorf("%s: %q must be refused", what, dir)
		}
	}
	for l := 'Z'; l >= 'D'; l-- { // nothing can be shown about a folder on a drive that is not there
		d := string(l) + `:\`
		if _, err := os.Stat(d); err == nil {
			continue
		}
		if devFolder(d + `nothing\here`) {
			t.Errorf("a folder on the missing drive %s must be refused", d)
		}
		break
	}
	other := t.TempDir()
	allowed := map[string]string{
		"a sibling that shares the prefix": root + `2\dev`,
		"the sibling itself":               root + `2`,
		"another temporary folder":         other,
		"the same, forward slashes":        slashes(other),
		"the same, dot dot segments":       other + `\x\..`,
		"a folder that does not exist":     filepath.Join(other, "not", "yet"),
	}
	for what, dir := range allowed {
		if !devFolder(dir) {
			t.Errorf("%s: %q must be accepted", what, dir)
		}
	}
	if _, err := os.Stat(product); err == nil {
		t.Error("devFolder must not create the product folder")
	}
}

// An 8.3 short name reaches the same folder (C:\PROGRA~3 is C:\ProgramData).
func TestDevFolderShortNames(t *testing.T) {
	product := fakeProgramData(t)
	root := programData()
	short := shortPath(root)
	if strings.EqualFold(short, root) {
		t.Skip("this volume has no 8.3 names")
	}
	for _, dir := range []string{short, filepath.Join(short, "SplitLane"), filepath.Join(short, "SplitLane", "x")} {
		if devFolder(dir) {
			t.Errorf("%q is the product root and must be refused (%s)", dir, product)
		}
	}
}

// A directory junction (or a symbolic link) to the product root is the product root.
func TestDevFolderFollowsJunctions(t *testing.T) {
	product := fakeProgramData(t)
	if err := os.MkdirAll(product, 0o755); err != nil { // below the temporary root
		t.Fatal(err)
	}
	links := t.TempDir()
	junction := func(name, target string) string {
		link := filepath.Join(links, name)
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
			t.Skipf("no junction in this environment: %v %s", err, out)
		}
		t.Cleanup(func() { os.Remove(link) })
		return link
	}
	toProduct := junction("to-product", product)
	for _, dir := range []string{toProduct, filepath.Join(toProduct, "sub"), slashes(filepath.Join(toProduct, "sub"))} {
		if devFolder(dir) {
			t.Errorf("%q leads to the product folder and must be refused", dir)
		}
	}
	elsewhere := t.TempDir()
	if away := junction("to-elsewhere", elsewhere); !devFolder(away) || !devFolder(filepath.Join(away, "sub")) {
		t.Errorf("a junction to a folder outside the product root is fine: %q", away)
	}
	gone := filepath.Join(links, "gone")
	if err := os.Mkdir(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	dangling := junction("dangling", gone)
	if err := os.Remove(gone); err != nil {
		t.Fatal(err)
	}
	if devFolder(dangling) || devFolder(filepath.Join(dangling, "sub")) {
		t.Errorf("a junction that cannot be resolved cannot be shown to lead elsewhere: %q", dangling)
	}
}

// The same guard on the real known folder, as in production. Nothing is written: devFolder only reads.
func TestDevFolderRealProgramData(t *testing.T) {
	real := programData()
	if !filepath.IsAbs(real) {
		t.Fatalf("the known folder ProgramData: %q", real)
	}
	product := filepath.Join(real, "SplitLane")
	refused := map[string]string{
		"the root":             real,
		"the product folder":   product,
		"forward slashes":      slashes(product),
		"dot dot segments":     filepath.VolumeName(real) + `\Windows\..\` + filepath.Base(real) + `\SplitLane`,
		"extended-length path": `\\?\` + product,
		"another case":         strings.ToLower(product),
		"short name":           filepath.Join(shortPath(real), "SplitLane"),
	}
	for what, dir := range refused {
		if devFolder(dir) {
			t.Errorf("%s: %q must be refused", what, dir)
		}
	}
	if !devFolder(real + `2\dev`) {
		t.Errorf("%s2 is a sibling of ProgramData, not the product root", real)
	}
	if !devFolder(t.TempDir()) {
		t.Error("a temporary folder is a development folder")
	}
}

// The product folder is refused however it is spelled, by both commands that write there.
func TestSeedAndResetDevRefuseEverySpelling(t *testing.T) {
	product := fakeProgramData(t)
	for _, dir := range []string{slashes(product), filepath.Dir(product) + `\x\..\SplitLane`, `\\?\` + product} {
		var out strings.Builder
		if code := run([]string{"seed", dir, "100.64.0.1"}, &out); code != exitUsage {
			t.Errorf("seed %q: %d %s", dir, code, out.String())
		}
		if code := run([]string{"reset-dev", dir}, &out); code != exitUsage {
			t.Errorf("reset-dev %q: %d %s", dir, code, out.String())
		}
		if _, err := os.Stat(product); err == nil {
			t.Fatalf("%q: something was written into the product folder", dir)
		}
	}
}

// write puts text into path and returns path.
func write(t *testing.T, path, text string) string {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// slctl proxy (stage 3 part 1, for development): proxy.add with the text or @FILE (a .conf gives its base
// name as the name, -name another), refresh, remove, select and list. The text of a server never comes
// back in the output; the answer shows the ids and the rejected lines with their codes.
func TestProxyCommands(t *testing.T) {
	sel, failed := "0123456789abcdef", "fetch_timeout"
	f := &fakeHelper{st: protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{}, Connector: protocol.ConnectorProxy,
		Proxy: protocol.Proxy{
			Servers: []protocol.ProxyServer{{ID: sel, Name: "Reality", Protocol: "vless", Source: "sub-00112233445566ff"},
				{ID: "fedcba9876543210", Name: "home", Protocol: "wireguard", Source: "file"}},
			Selected: &sel,
			Subscriptions: []protocol.Subscription{{ID: "sub-00112233445566ff", Name: "sub.example.com", UpdatedAt: "2026-11-16T12:00:00Z",
				Count: 1, Error: &failed}}}}}
	name := testPipeName()
	srv, err := serveHelper(name, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	slctl := func(args ...string) (int, string) {
		var out strings.Builder
		code := run(append([]string{"-pipe", name}, args...), &out)
		return code, out.String()
	}
	const link = "trojan://hunter2secretpass@example.com:443#T"
	code, out := slctl("proxy", "add", link)
	if code != exitOK || !strings.Contains(out, "SLCTL: added=1 existing=2 rejected=1 more=3\n") || !strings.Contains(out, "REJECTED line=2 code=bad_link\n") ||
		strings.Contains(out, "hunter2secretpass") {
		t.Fatalf("add: %d\n%s", code, out)
	}
	if got := f.paramsOf(protocol.MethodProxyAdd); got != `{"text":"`+link+`"}` {
		t.Fatalf("add params %s", got)
	}
	dir := t.TempDir()
	conf := write(t, filepath.Join(dir, "home.conf"), "[Interface]\nPrivateKey = x\n")
	if code, out := slctl("proxy", "add", "@"+conf); code != exitOK {
		t.Fatalf("add @FILE: %d\n%s", code, out)
	}
	if got := f.paramsOf(protocol.MethodProxyAdd); got != `{"text":"[Interface]\nPrivateKey = x\n","name":"home.conf"}` {
		t.Fatalf("add @FILE params %s", got)
	}
	links := write(t, filepath.Join(dir, "links.txt"), link+"\n")
	if code, _ := slctl("proxy", "add", "-name", "work.conf", "@"+links); code != exitOK || f.paramsOf(protocol.MethodProxyAdd) != `{"text":"`+link+`\n","name":"work.conf"}` {
		t.Fatalf("add -name: %d %s", code, f.paramsOf(protocol.MethodProxyAdd))
	}
	if code, out := slctl("proxy", "refresh", "sub-00112233445566ff"); code != exitFailed || !strings.Contains(out, "ERROR fetch_timeout") {
		t.Fatalf("refresh: %d\n%s", code, out)
	}
	for _, cmd := range []string{"remove", "select"} {
		if code, out := slctl("proxy", cmd, sel); code != exitOK || f.paramsOf("proxy."+cmd) != `{"id":"`+sel+`"}` {
			t.Fatalf("%s: %d\n%s", cmd, code, out)
		}
	}
	code, out = slctl("proxy", "list")
	for _, want := range []string{
		"PROXY connector=proxy selected=" + sel + " servers=2 subscriptions=1\n",
		"SERVER * " + sel + " vless sub-00112233445566ff Reality\n",
		"SERVER   fedcba9876543210 wireguard file home\n",
		"SUBSCRIPTION sub-00112233445566ff sub.example.com count=1 updated=2026-11-16T12:00:00Z error=fetch_timeout\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("list lacks %q", want)
		}
	}
	if code != exitOK || t.Failed() {
		t.Fatalf("list: %d\n%s", code, out)
	}
	for _, args := range [][]string{{"proxy"}, {"proxy", "bogus"}, {"proxy", "select"}, {"proxy", "add"}, {"proxy", "list", "x"},
		{"proxy", "add", "@" + filepath.Join(dir, "missing.conf")}} {
		if code, out := slctl(args...); code != exitUsage {
			t.Errorf("%v: %d\n%s", args, code, out)
		}
	}
}

// A helper that does not answer a method within -timeout is exit code 3 for every command that
// sends one, and the output says so; it used to be a silent exit code 1.
func TestSilentHelperIsATimeout(t *testing.T) {
	name := testPipeName()
	f := &fakeHelper{st: protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{}}, release: make(chan struct{}),
		silent: map[string]bool{protocol.MethodSetExitNode: true, protocol.MethodClearSystemExitNode: true,
			protocol.MethodDisable: true, protocol.MethodAllDirect: true, protocol.MethodLanesSet: true, "test.silent": true}}
	srv, err := serveHelper(name, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	t.Cleanup(func() { close(f.release) }) // first: the answers in flight can finish, Close does not wait
	file := filepath.Join(t.TempDir(), "lanes.json")
	os.WriteFile(file, []byte(`{"apps":[],"defaultLane":"vpn"}`), 0o644)
	for _, args := range [][]string{{"exitnode", "100.64.0.1"}, {"clear-system-exit-node"}, {"disable"},
		{"alldirect", "on"}, {"lanes", file}, {"call", "test.silent"}} {
		var out strings.Builder
		start := time.Now()
		code := run(append([]string{"-pipe", name, "-timeout", "500ms"}, args...), &out)
		if code != exitTimeout || !strings.Contains(out.String(), "SLCTL: TIMEOUT") {
			t.Errorf("%v: no answer is a timeout, got %d\n%s", args, code, out.String())
		}
		if waited := time.Since(start); waited < 400*time.Millisecond || waited > 5*time.Second {
			t.Errorf("%v: waited %s", args, waited)
		}
	}
}

// An error response to state.get is a failed command with its ERROR line, not a timeout.
func TestRejectedStateIsAFailure(t *testing.T) {
	name := testPipeName()
	srv, err := serveHelper(name, &fakeHelper{st: protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{}},
		stateErr: protocol.Errorf(protocol.CodeBadRequest, "state refused")})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(srv.Close)
	for _, args := range [][]string{{"state"}, {"wait", "on"}, {"enable"}, {"alldirect", "on"}} {
		var out strings.Builder
		code := run(append([]string{"-pipe", name, "-timeout", "5s"}, args...), &out)
		if code != exitFailed || !strings.Contains(out.String(), "ERROR bad_request state refused") ||
			strings.Contains(out.String(), "TIMEOUT") {
			t.Errorf("%v: %d\n%s", args, code, out.String())
		}
	}
}

// rawFake listens on a new pipe name and hands every client to serve: the helper is played by hand.
func rawFake(t *testing.T, serve func(r *bufio.Reader, c net.Conn)) string {
	t.Helper()
	name := testPipeName()
	l, err := winio.ListenPipe(name, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				serve(bufio.NewReader(c), c)
			}()
		}
	}()
	return name
}

// A helper that is silent from the first message (hello) is a timeout; one that closes the pipe is a
// broken pipe with its own exit code, before or after hello and while a phase is awaited.
func TestSilentOrDyingHelper(t *testing.T) {
	hello := func(r *bufio.Reader, c net.Conn) {
		r.ReadString('\n')
		io.WriteString(c, `{"id":1,"result":{"protocol":2}}`+"\n")
	}
	cases := []struct {
		what    string
		serve   func(r *bufio.Reader, c net.Conn)
		args    []string
		timeout string
		want    int
		text    string
	}{
		{"silent at hello", func(r *bufio.Reader, c net.Conn) { io.Copy(io.Discard, c) }, []string{"state"}, "500ms", exitTimeout, "SLCTL: TIMEOUT"},
		{"closes at hello", func(r *bufio.Reader, c net.Conn) { r.ReadString('\n') }, []string{"state"}, "5s", exitBroken, "SLCTL: the pipe is broken or closed"},
		{"closes after hello", func(r *bufio.Reader, c net.Conn) { hello(r, c); r.ReadString('\n') }, []string{"state"}, "5s", exitBroken, "SLCTL: the pipe is broken or closed"},
		{"closes after hello, wait", func(r *bufio.Reader, c net.Conn) { hello(r, c); r.ReadString('\n') }, []string{"wait", "on"}, "5s", exitBroken, "SLCTL: the pipe is broken or closed"},
		{"closes before the answer", func(r *bufio.Reader, c net.Conn) { hello(r, c); r.ReadString('\n') }, []string{"exitnode", "100.64.0.1"}, "5s", exitBroken, "SLCTL: the pipe is broken or closed"},
		{"silent after hello", func(r *bufio.Reader, c net.Conn) { hello(r, c); io.Copy(io.Discard, c) }, []string{"clear-system-exit-node"}, "500ms", exitTimeout, "SLCTL: TIMEOUT"},
	}
	for _, c := range cases {
		name := rawFake(t, c.serve)
		var out strings.Builder
		code := run(append([]string{"-pipe", name, "-timeout", c.timeout}, c.args...), &out)
		if code != c.want || !strings.Contains(out.String(), c.text) {
			t.Errorf("%s: want %d with %q, got %d\n%s", c.what, c.want, c.text, code, out.String())
		}
	}
}
