package engine

import (
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

func startEnable(h *harness) {
	h.e.mu.Lock()
	h.e.beginEnableLocked(false)
	h.e.mu.Unlock()
}

func enabled(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	return h
}

func wantError(t *testing.T, h *harness, code string) {
	t.Helper()
	h.waitPhase(protocol.PhaseError)
	if r := h.e.State().Reason; r == nil || r.Code != code {
		t.Fatalf("reason %+v, want %s; events %v", r, code, h.rec.list())
	}
}

// 5.3 in order: rules and config, check, start, tunnel proven, journal before the switch, DNS flush, path check.
func TestEnableInOrder(t *testing.T) {
	h := enabled(t)
	order := []string{"rules:write", "core:check", "core:start", "journal:save 100.64.0.1 true", "sys:clear", "dns:flush"}
	last := -1
	for _, ev := range order {
		i := h.rec.index(ev)
		if i <= last {
			t.Fatalf("%s out of order: %v", ev, h.rec.list())
		}
		last = i
	}
	if h.rec.index("sys:clear WITHOUT JOURNAL") >= 0 {
		t.Fatal("the switch ran before the journal was written")
	}
	s := h.e.State()
	if *s.Addresses.VPN != vpnAddr || *s.Addresses.Direct != directAddr || !s.SystemTailscale.RestoreOnDisable || s.SystemTailscale.ExitNodeOn {
		t.Fatalf("state %+v", s)
	}
	saved := h.store.saved()
	if !saved.WasOn || saved.Core == nil || saved.Core.PID != h.core.last().id.PID {
		t.Fatalf("state.json %+v", saved)
	}
	if o := h.core.options(); o.Mode != "vpn" || o.ExitNode != exitNode || o.Ports != portsForTest() || o.DevPin {
		t.Fatalf("config options %+v", o)
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on" {
		t.Fatalf("phases %s", got)
	}
}

// Without an exit node in the system client (or without the client) nothing is switched or journaled.
func TestEnableWithoutSystemExitNode(t *testing.T) {
	for name, change := range map[string]func(s *fakeSys){
		"exit node off":   func(s *fakeSys) { s.on = false },
		"not installed":   func(s *fakeSys) { s.installed, s.on = false, false },
		"service stopped": func(s *fakeSys) { s.statusErr = errBoom },
	} {
		h := newHarness(t)
		h.sys.set(change)
		h.load()
		startEnable(h)
		h.waitPhase(protocol.PhaseOn)
		if h.rec.index("sys:") >= 0 || h.rec.index("journal:") >= 0 {
			t.Errorf("%s: no journal and no switch expected: %v", name, h.rec.list())
		}
	}
}

// 5.3 step 1 and section 10 row 1: a refusal leaves the system untouched.
func TestPreflightRefusals(t *testing.T) {
	cases := map[string]func(h *harness){
		protocol.ReasonNotLoggedIn:       func(h *harness) { h.store.state.LoggedIn = false },
		protocol.ReasonNoExitNode:        func(h *harness) { h.store.state.SelectedExitNode = "" },
		protocol.ReasonNoDefaultRoute:    func(h *harness) { h.host.noRoute = 1 },
		protocol.ReasonIPForwarding:      func(h *harness) { h.host.forwarding = true },
		protocol.ReasonOtherCore:         func(h *harness) { h.host.otherCore = true },
		protocol.ReasonSystemDNSOverride: func(h *harness) { h.host.dnsOverride = true },
	}
	for code, change := range cases {
		h := newHarness(t)
		change(h)
		h.load()
		startEnable(h)
		wantError(t, h, code)
		if h.rec.index("core:start") >= 0 || h.rec.index("sys:") >= 0 {
			t.Errorf("%s: the system must stay untouched: %v", code, h.rec.list())
		}
	}
}

// Unattended enable waits up to 60 s for a default route.
func TestUnattendedEnableWaitsForRoute(t *testing.T) {
	h := newHarness(t)
	h.host.noRoute = 3
	h.load()
	h.e.mu.Lock()
	h.e.beginEnableLocked(true)
	h.e.mu.Unlock()
	h.waitPhase(protocol.PhaseOn)

	h = newHarness(t)
	h.host.noRoute = -1
	h.load()
	before := h.clock.Now()
	h.e.mu.Lock()
	h.e.beginEnableLocked(true)
	h.e.mu.Unlock()
	wantError(t, h, protocol.ReasonNoDefaultRoute)
	if waited := h.clock.Now().Sub(before); waited < routeWait {
		t.Fatalf("gave up after %s", waited)
	}
}

// Section 10 row 1: the core did not start or its config failed the check.
func TestCoreDoesNotStart(t *testing.T) {
	h := newHarness(t)
	h.core.checkErr = errBoom
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonConfigInvalid)
	if h.rec.index("core:start") >= 0 || h.rec.index("sys:") >= 0 {
		t.Fatalf("nothing may start: %v", h.rec.list())
	}

	h = newHarness(t)
	h.core.silent = 1
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonCoreStartTimeout)
	if h.core.last().alive() || h.rec.index("sys:") >= 0 {
		t.Fatalf("the silent core must be stopped and the system untouched: %v", h.rec.list())
	}
}

// A core that exits before its start line gets one more start; two such exits are a failure.
func TestEarlyExitIsRetriedOnce(t *testing.T) {
	h := newHarness(t)
	h.core.dieEarly = 1
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	if n := h.rec.count("core:start"); n != 2 {
		t.Fatalf("starts %d", n)
	}
	h = newHarness(t)
	h.core.dieEarly = 2
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonCoreStartTimeout)
}

// 5.3 step 4: the node asks for a login: rollback at once, the "logged in" flag goes.
func TestLoginNeededAfterStart(t *testing.T) {
	h := newHarness(t)
	h.core.needLogin = true
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonLoginExpired)
	if h.store.saved().LoggedIn || h.rec.index("sys:clear") >= 0 {
		t.Fatalf("logged-in flag must be cleared, the system untouched: %v", h.rec.list())
	}
}

// Section 10 row 2: the tunnel is not proven at step 5: core stopped, system exit node never touched.
func TestTunnelNotProven(t *testing.T) {
	for name, script := range map[string][2][]ProbeResult{
		"vpn silent":    {{noAnswer}, {atDirect}},
		"direct silent": {{atVPN}, {noAnswer}},
	} {
		h := newHarness(t)
		h.probe.script(script[0], script[1], nil)
		h.load()
		startEnable(h)
		wantError(t, h, protocol.ReasonTunnelNotProven)
		if h.rec.index("sys:") >= 0 || h.core.last().alive() {
			t.Errorf("%s: %v", name, h.rec.list())
		}
	}
	h := newHarness(t)
	h.probe.script([]ProbeResult{atDirect}, []ProbeResult{atDirect}, nil)
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSameAddress)
}

// Section 10 row 3: the check after the switch fails: rollback 5.4 restores the exit node first.
func TestSwitchCheckFails(t *testing.T) {
	h := newHarness(t)
	h.probe.script(nil, nil, []ProbeResult{noAnswer})
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSwitchCheckFailed)
	set, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt")
	if set < 0 || stop < set || h.store.hasJournal() {
		t.Fatalf("restore before stop, journal deleted: %v", h.rec.list())
	}
}

// 5.3: two answers in a row from the direct address are a leak, rolled back at once; one is not.
func TestLeakAtSwitch(t *testing.T) {
	h := newHarness(t)
	h.probe.script(nil, nil, []ProbeResult{atDirect, atDirect, atVPN})
	h.load()
	before := h.clock.Now()
	startEnable(h)
	wantError(t, h, protocol.ReasonLeakDetected)
	if h.clock.Now().Sub(before) >= switchWait {
		t.Fatal("a leak must not wait for the end of the 45 s")
	}
	h = newHarness(t)
	h.probe.script(nil, nil, []ProbeResult{atDirect, atVPN, atVPN})
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
}

// Failures around step 6 never leave the exit node off without a journal.
func TestStep6Failures(t *testing.T) {
	h := newHarness(t)
	h.store.journalErr = errBoom
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSwitchCheckFailed)
	if h.rec.index("sys:clear") >= 0 {
		t.Fatalf("no switch without a journal: %v", h.rec.list())
	}
	h = newHarness(t)
	h.sys.set(func(s *fakeSys) { s.clearErr = errBoom })
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSwitchCheckFailed)
	if h.store.hasJournal() {
		t.Fatal("the journal must be deleted: the exit node is still on")
	}
	h = newHarness(t)
	h.sys.set(func(s *fakeSys) { s.allowErr = errBoom })
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSwitchCheckFailed)
	if h.rec.index("core:start") >= 0 {
		t.Fatal("without the LAN flag nothing may start")
	}
}

// 5.7: while a journal exists, enabling does not overwrite it.
func TestExistingJournalIsNotOverwritten(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: "100.64.0.50", AllowLAN: false}
	h.load()
	h.e.mu.Lock()
	h.e.st.Phase = protocol.PhaseOff
	h.e.mu.Unlock()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	j, _ := h.store.LoadJournal()
	if j == nil || j.ExitNode != "100.64.0.50" || j.AllowLAN || h.rec.index("journal:save") >= 0 {
		t.Fatalf("journal %+v, events %v", j, h.rec.list())
	}
}

// routing.disable during starting cancels the enable: rollback 5.4, phase off, no error.
func TestCancelDuringStarting(t *testing.T) {
	h := newHarness(t)
	h.probe.hang = true
	h.load()
	startEnable(h)
	h.waitFor("core started", func() bool { return h.rec.index("core:start") >= 0 })
	h.e.mu.Lock()
	h.e.op.cancel()
	h.e.mu.Unlock()
	h.waitPhase(protocol.PhaseOff)
	if h.e.State().Reason != nil || h.core.last().alive() || h.rec.index("sys:") >= 0 {
		t.Fatalf("cancel: %+v %v", h.e.State().Reason, h.rec.list())
	}
}

// Section 10 row 4: the core fails while on: the exit node comes back at once, then a restart
// in phase starting; "was on" stays set; the journal keeps the value read before the first switch.
func TestCrashRestoresAtOnceAndRestarts(t *testing.T) {
	h := enabled(t)
	first := h.core.last()
	h.e.mu.Lock()
	r := h.e.run
	h.e.mu.Unlock()
	if r.proc != first {
		t.Fatal("run is not the started core")
	}
	first.crash()
	h.waitFor("second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	crash, set, start := h.rec.index("core:crash"), h.rec.index("sys:set 100.64.0.1 true"), len(h.rec.list())-1
	for i, ev := range h.rec.list() {
		if strings.HasPrefix(ev, "core:start") {
			start = i
		}
	}
	if !(crash < set && set < start) {
		t.Fatalf("restore must come right after the crash, before the new start: %v", h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on,starting,on" {
		t.Fatalf("phases %s", got)
	}
	if !h.store.saved().WasOn {
		t.Fatal("was-on must stay set through a restart")
	}
	j, _ := h.store.LoadJournal()
	if j == nil || j.ExitNode != exitNode || !j.AllowLAN {
		t.Fatalf("journal %+v", j)
	}
}

// Review Focus 1: a crash in phase blocked. The exit node the user selected himself is neither
// overwritten from the journal nor switched off at step 6 of a restart (5.7): no restart, 5.4
// without a restore (the journal goes), error core_crashed.
func TestCrashInBlockedKeepsTheUsersExitNode(t *testing.T) {
	h := enabled(t)
	h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
	h.e.mu.Lock()
	h.e.sysExitOn = true
	h.e.setPhase(protocol.PhaseBlocked, nil)
	h.e.mu.Unlock()
	mark := len(h.rec.list())
	h.core.last().crash()
	wantError(t, h, protocol.ReasonCoreCrashed)
	on, ip := systemExitNode(h)
	if countSince(h, mark, "sys:") != 0 || countSince(h, mark, "core:start") != 0 || !on || ip != "100.64.0.99" {
		t.Fatalf("the user's exit node must stay, no restart: %v", h.rec.list())
	}
	if h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("journal deleted, was-on cleared: %v", h.rec.list())
	}
}

// Three failed restarts end in error with reason core_crashed.
func TestThreeFailedRestarts(t *testing.T) {
	h := enabled(t)
	checks := h.rec.count("core:check")
	h.core.mu.Lock()
	h.core.startErr = errBoom
	h.core.mu.Unlock()
	h.core.last().crash()
	wantError(t, h, protocol.ReasonCoreCrashed)
	if n := h.rec.count("core:check") - checks; n != 3 {
		t.Fatalf("restart attempts %d", n)
	}
	if h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatal("after the last failure: exit node restored, was-on cleared")
	}
}
