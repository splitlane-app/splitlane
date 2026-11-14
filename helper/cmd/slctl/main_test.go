package main

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

func TestVerdict(t *testing.T) {
	cases := []struct {
		seen     []string
		ok, done bool
	}{
		{[]string{"vpn"}, true, true},
		{[]string{"none", "vpn"}, true, true},
		{[]string{"direct"}, false, false},
		{[]string{"direct", "vpn", "vpn"}, true, true},
		{[]string{"direct", "vpn", "direct"}, false, true},
		{[]string{"none", "none", "none", "none", "none", "none"}, false, true},
	}
	for _, c := range cases {
		if ok, done := verdict(c.seen, "vpn"); ok != c.ok || done != c.done {
			t.Errorf("%v: %v %v", c.seen, ok, done)
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
}

func (f *fakeHelper) set(phase string) {
	f.mu.Lock()
	f.st.Phase = phase
	s := f.st
	f.mu.Unlock()
	f.srv.Broadcast(s)
}

func (f *fakeHelper) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	switch method {
	case protocol.MethodStateGet:
		f.mu.Lock()
		defer f.mu.Unlock()
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
	}
	return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown")
}

func (f *fakeHelper) Hello() protocol.HelloResult { return protocol.HelloResult{Protocol: 1} }
func (f *fakeHelper) SetClients(int)              {}

func testPipeName() string { return fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63()) }

// serveFake opens the pipe name with the fake helper behind it.
func serveFake(name string) (*pipe.Server, error) {
	vpn, direct := "203.0.113.1", "198.51.100.1"
	return serveState(name, protocol.State{Phase: protocol.PhaseOff, Apps: []protocol.AppRule{},
		Addresses: protocol.Addresses{VPN: &vpn, Direct: &direct}})
}

// serveState opens the pipe name with a fake helper in state st.
func serveState(name string, st protocol.State) (*pipe.Server, error) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, err
	}
	f := &fakeHelper{st: st}
	srv, err := pipe.Listen(name, u.User.Sid.String(), f, nil)
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
		`C:\x\curl-vpn.exe`:    {"203.0.113.1"},                                // right at once
		`C:\x\curl-direct.exe`: {"203.0.113.1", "198.51.100.1", "203.0.113.1"}, // two of three wrong
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
	code := run([]string{"-pipe", name, "expect-lanes", `C:\x\curl-vpn.exe=vpn`, `C:\x\curl-direct.exe=direct`}, &out)
	text := out.String()
	if code != exitFailed || !strings.Contains(text, "LANE-OK curl-vpn.exe vpn") ||
		!strings.Contains(text, "LANE-WRONG curl-direct.exe want=direct seen=[vpn direct vpn]") {
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
