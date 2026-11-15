package engine

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

func startEnable(h *harness) {
	h.e.mu.Lock()
	h.e.beginEnableLocked(false)
	h.e.mu.Unlock()
}

// enabled is a helper in phase on after 5.3, with the look at the path to the exit node that follows
// phase on done (spec 7): nothing of the enable runs on behind the test.
func enabled(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
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
	if o := h.core.options(); o.Mode != "vpn" || o.Ports != portsForTest() || o.DevPin || o.Lane.Proxy != nil ||
		o.Lane.Tailscale == nil || *o.Lane.Tailscale != (TailscaleLane{Hostname: "PC-splitlane", ExitNode: exitNode}) {
		t.Fatalf("config options %+v, lane %+v", o, o.Lane.Tailscale)
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on" {
		t.Fatalf("phases %s", got)
	}
}

// The development flag --core-log-level reaches the config of the core (spec 6.1); without a level
// in Settings the options carry none, and the driver writes info.
func TestCoreLogLevelGoesToTheConfig(t *testing.T) {
	h := enabled(t)
	if o := h.core.options(); o.LogLevel != "" {
		t.Fatalf("without a level in Settings: %q", o.LogLevel)
	}
	h = newHarness(t)
	h.e.d.Settings.CoreLogLevel = "debug"
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	if o := h.core.options(); o.LogLevel != "debug" {
		t.Fatalf("level %q", o.LogLevel)
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

// Review of task 7, for the investigation of the core's DNS in task 16: a proof of 5.3 step 5 that
// gives up logs which probe never answered with an address (probe-vpn, probe-direct or both; or that
// both did, never in the same round) and how long it waited; once per core, so the retry logs its own.
func TestTunnelNotProvenLogsTheSilentProbe(t *testing.T) {
	for _, c := range []struct {
		name        string
		vpn, direct []ProbeResult
		want        string
		cores       int // how many of the two cores log want
	}{
		{"vpn silent", []ProbeResult{noAnswer}, []ProbeResult{atDirect}, "probe-vpn never answered", 2},
		{"direct silent", []ProbeResult{atVPN}, []ProbeResult{noAnswer}, "probe-direct never answered", 2},
		{"both silent", []ProbeResult{noAnswer}, []ProbeResult{noAnswer}, "probe-vpn and probe-direct never answered", 2},
		{"never together", []ProbeResult{atVPN, noAnswer}, []ProbeResult{noAnswer, atDirect},
			"probe-vpn and probe-direct answered, never in the same round", 1},
	} {
		h := newHarness(t)
		logEvents(h)
		h.probe.script(c.vpn, c.direct, nil)
		h.load()
		startEnable(h)
		wantError(t, h, protocol.ReasonTunnelNotProven)
		line := fmt.Sprintf("log:the tunnel proof gave up after %s: %s", proveWait, c.want)
		if n := h.rec.count(line); n != c.cores {
			t.Errorf("%s: want %q from %d cores, got %d: %v", c.name, line, c.cores, n, h.rec.list())
		}
	}
}

// probesPerCore scripts both probe inbounds for each core that starts: the core whose PID is a key
// of answers gets that pair (probe-vpn, probe-direct), every other core proves the tunnel.
func probesPerCore(h *harness, answers map[int][2]ProbeResult) {
	h.core.mu.Lock()
	defer h.core.mu.Unlock()
	h.core.onStart = func(p *fakeProc) {
		a, ok := answers[p.id.PID]
		if !ok {
			a = [2]ProbeResult{atVPN, atDirect}
		}
		h.probe.script([]ProbeResult{a[0]}, []ProbeResult{a[1]}, nil)
	}
}

// Appendix A (checks of plan 1B): the core's own DNS now and then gets no answers for the first
// tens of seconds. A tunnel that is not proven within the 45 s of 5.3 step 5 gets one more core with
// new ports, in phase starting; the system is not touched before that core has proven the tunnel.
func TestTunnelNotProvenIsRetriedOnce(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	probesPerCore(h, map[int][2]ProbeResult{1000: {noAnswer, noAnswer}})
	onceOnState(h, func(s protocol.State) bool { return s.Addresses.VPN != nil }, func() { h.rec.add("test:proven") })
	h.load()
	before := h.clock.Now()
	startEnable(h)
	h.waitFor("the end of the enable", h.idle)
	if s := h.e.State(); s.Phase != protocol.PhaseOn {
		t.Fatalf("phase %s reason %+v, starts %d: %v", s.Phase, s.Reason, h.rec.count("core:start"), h.rec.list())
	}
	if n := h.rec.count("core:start"); n != 2 || h.rec.count("log:tunnel not proven; trying the core once more with new ports") != 1 {
		t.Fatalf("one retry expected, starts %d: %v", n, h.rec.list())
	}
	if !inOrder(h, 0, "core:start 1000", "log:tunnel not proven", "core:interrupt 1000", "core:start 1001", "test:proven", "journal:save", "sys:clear") {
		t.Fatalf("the first core stopped before the second, the system touched only after the proof: %v", h.rec.list())
	}
	if proven := h.rec.index("test:proven"); h.rec.index("sys:") < proven || h.rec.index("journal:") < proven || h.rec.index("sys:set") >= 0 {
		t.Fatalf("nothing switched or journaled before the second proof: %v", h.rec.list())
	}
	if waited := h.clock.Now().Sub(before); waited < proveWait {
		t.Fatalf("the first proof gave up after %s", waited)
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on" {
		t.Fatalf("phases %s", got)
	}
}

// Two cores that do not prove the tunnel: the rollback of 5.4, error tunnel_not_proven, the system
// untouched. With a core that exits before its start line first, the enable starts three cores, not more.
func TestTunnelNotProvenTwiceRollsBack(t *testing.T) {
	for _, c := range []struct {
		name   string
		early  int
		starts int
	}{{"two cores", 0, 2}, {"early exit first", 1, 3}} {
		h := newHarness(t)
		h.core.dieEarly = c.early
		h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{noAnswer}, nil)
		h.load()
		startEnable(h)
		wantError(t, h, protocol.ReasonTunnelNotProven)
		if n := h.rec.count("core:start"); n != c.starts {
			t.Errorf("%s: starts %d, want %d: %v", c.name, n, c.starts, h.rec.list())
		}
		if h.rec.index("sys:") >= 0 || h.rec.index("journal:") >= 0 || h.core.anyAlive() {
			t.Errorf("%s: system untouched, no core left: %v", c.name, h.rec.list())
		}
		if on, ip := systemExitNode(h); !on || ip != exitNode {
			t.Errorf("%s: exit node on=%v ip=%s", c.name, on, ip)
		}
	}
}

// The retry is for tunnel_not_proven alone: same_address rolls back at once, even when a second
// core would prove the tunnel, and also when the core of the retry gives it (no third core).
func TestSameAddressIsNotRetried(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	probesPerCore(h, map[int][2]ProbeResult{1000: {atDirect, atDirect}})
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSameAddress)
	if n := h.rec.count("core:start"); n != 1 || h.rec.count("log:tunnel not proven") != 0 || h.rec.index("sys:") >= 0 {
		t.Fatalf("same_address: one core, no retry, system untouched; starts %d: %v", n, h.rec.list())
	}

	h = newHarness(t)
	probesPerCore(h, map[int][2]ProbeResult{1000: {noAnswer, noAnswer}, 1001: {atDirect, atDirect}})
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonSameAddress)
	if n := h.rec.count("core:start"); n != 2 || h.rec.index("sys:") >= 0 {
		t.Fatalf("same_address of the retried core: two cores, system untouched; starts %d: %v", n, h.rec.list())
	}
}

// The restarts of 5.5 run the same 5.3: a core of a restart that does not prove the tunnel gets one
// more core within the same attempt, not a failed attempt.
func TestRestartRetriesTheTunnelProof(t *testing.T) {
	h := enabled(t)
	logEvents(h)
	probesPerCore(h, map[int][2]ProbeResult{1001: {noAnswer, noAnswer}})
	h.core.last().crash()
	h.waitFor("third core on", func() bool {
		return h.rec.count("core:start") == 3 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if h.rec.count("log:restart attempt failed") != 0 || h.rec.count("log:tunnel not proven") != 1 {
		t.Fatalf("one restart attempt with a retried proof: %v", h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on,starting,on" {
		t.Fatalf("phases %s", got)
	}
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

// hookProbe lets a test act inside the helper's own request of step 8 (the path through the
// adapter): call counts those requests from 1.
type hookProbe struct {
	*fakeProbe
	cmu      sync.Mutex
	calls    int
	onDirect func(call int)
}

func (p *hookProbe) Direct(ctx context.Context, url string) ProbeResult {
	p.cmu.Lock()
	p.calls++
	n := p.calls
	p.cmu.Unlock()
	if p.onDirect != nil {
		p.onDirect(n)
	}
	return p.fakeProbe.Direct(ctx, url)
}

// slowCheckCore makes the second `check` of the core wait a moment, so that everything that
// started at the first (failed) start has certainly run by the time the retry goes on.
type slowCheckCore struct {
	*fakeCore
	checks atomic.Int32
	delay  time.Duration
}

func (c *slowCheckCore) Check(ctx context.Context, path string) error {
	if c.checks.Add(1) == 2 {
		time.Sleep(c.delay)
	}
	return c.fakeCore.Check(ctx, path)
}

// onceOnState runs f the first time a published state satisfies when; it runs under the
// engine's lock, exactly where the operation publishes.
func onceOnState(h *harness, when func(protocol.State) bool, f func()) {
	var once sync.Once
	orig := h.e.d.OnState
	h.e.d.OnState = func(s protocol.State) {
		orig(s)
		if when(s) {
			once.Do(f)
		}
	}
}

// crashInProbe ends the current core from inside a probe of the operation and returns only once
// the watcher has acted (its restore is the sign): the watcher has then looked at the engine
// while the operation was still busy, which is the case these tests are about.
func crashInProbe(h *harness) {
	mark := len(h.rec.list())
	h.core.last().crash()
	deadline := time.Now().Add(2 * time.Second)
	for countSince(h, mark, "sys:set") == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
}

// Section 10 row 4 while an operation is busy: the core fails inside step 8 and the operation
// hangs in its probe, so it cannot restore anything. The exit node must come back from the
// watcher alone, at once, with the phase still starting.
func TestCrashWhileBusyRestoresAtOnce(t *testing.T) {
	h := newHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	free := func() { once.Do(func() { close(release) }) }
	t.Cleanup(free)
	h.e.d.Probe = &hookProbe{fakeProbe: h.probe, onDirect: func(call int) {
		if call == 1 {
			close(entered)
			<-release
		}
	}}
	h.load()
	startEnable(h)
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatalf("step 8 never started: %v", h.rec.list())
	}
	h.core.last().crash()
	h.waitFor("the immediate restore", func() bool { return h.rec.index("sys:set 100.64.0.1 true") >= 0 })
	if h.idle() || h.e.State().Phase != protocol.PhaseStarting {
		t.Fatalf("the operation must still hang in its probe, phase %s: %v", h.e.State().Phase, h.rec.list())
	}
	if on, ip := systemExitNode(h); !on || ip != exitNode {
		t.Fatalf("exit node on=%v ip=%s", on, ip)
	}
	free()
	wantError(t, h, protocol.ReasonCoreCrashed)
	if n := h.rec.count("sys:set"); n != 1 {
		t.Fatalf("one restore expected, got %d: %v", n, h.rec.list())
	}
}

// The core fails inside the very probe that completes step 8, and that probe still gets a good
// answer: the enable must not end in "on" with a dead core. Here it is a first enable: the rollback of
// 5.4 runs, error core_crashed, the exit node is back.
func TestCrashDuringTheLastProbeIsNotOn(t *testing.T) {
	h := newHarness(t)
	h.e.d.Probe = &hookProbe{fakeProbe: h.probe, onDirect: func(call int) {
		if call == 2 {
			crashInProbe(h)
		}
	}}
	h.load()
	startEnable(h)
	wantError(t, h, protocol.ReasonCoreCrashed)
	if got := strings.Join(h.phases(), ","); got != "starting,stopping,error" || h.rec.count("core:start") != 1 {
		t.Fatalf("phases %s, no restart expected: %v", got, h.rec.list())
	}
	if on, ip := systemExitNode(h); !on || ip != exitNode || h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("exit node restored, journal gone, was-on clear: on=%v ip=%s %v", on, ip, h.rec.list())
	}
}

// The same crash during a restart is one failed attempt: the loop of 5.5 goes on and the next
// core ends in "on"; nothing is published as an error and no attempt is lost.
func TestRestartCrashDuringTheLastProbeTriesAgain(t *testing.T) {
	h := newHarness(t)
	var armed atomic.Bool
	var seen atomic.Int32
	h.e.d.Probe = &hookProbe{fakeProbe: h.probe, onDirect: func(int) {
		if armed.Load() && seen.Add(1) == 2 {
			crashInProbe(h)
		}
	}}
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	armed.Store(true)
	h.core.last().crash()
	h.waitFor("third core on", func() bool {
		return h.rec.count("core:start") == 3 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if got := strings.Join(h.phases(), ","); got != "starting,on,starting,on" || h.sawReason(protocol.ReasonCoreCrashed) {
		t.Fatalf("phases %s: %v", got, h.rec.list())
	}
}

// The core fails while an operation is still registered (for instance in the short window after
// an enable published "on" and before its cleanup): the watcher restores at once, waits for that
// operation and then, the core being still the current one in a live phase, restarts it.
func TestWatcherActsAfterTheBusyOperationEnds(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	busy := &op{kind: "enable", cancel: func() {}, done: make(chan struct{})}
	h.e.mu.Lock()
	r := h.e.run
	r.cleared = true // as after step 6 of 5.3
	h.e.op = busy
	h.e.mu.Unlock()
	go h.e.watch(r)
	p.crash()
	h.waitFor("the immediate restore", func() bool { return h.rec.index("sys:set 100.64.0.1 true") >= 0 })
	time.Sleep(20 * time.Millisecond)
	if n := h.rec.count("core:start"); n != 1 || h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("no restart while the operation runs (starts %d, phase %s): %v", n, h.e.State().Phase, h.rec.list())
	}
	h.e.mu.Lock()
	h.e.op = nil
	h.e.mu.Unlock()
	close(busy.done)
	h.waitFor("second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if got := strings.Join(h.phases(), ","); got != "on,starting,on" || !h.core.last().alive() {
		t.Fatalf("phases %s: %v", got, h.rec.list())
	}
}

// A run that never switched the system exit node off has nothing to give back: while an
// operation is busy its watcher leaves the system alone, even with a journal at hand.
func TestWatcherDoesNotRestoreForARunThatSwitchedNothing(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	busy := &op{kind: "enable", cancel: func() {}, done: make(chan struct{})}
	h.e.mu.Lock()
	r := h.e.run // r.cleared is false: step 6 was not reached
	h.e.op = busy
	h.e.mu.Unlock()
	go h.e.watch(r)
	p.crash()
	time.Sleep(50 * time.Millisecond)
	if h.rec.index("sys:") >= 0 {
		t.Fatalf("nothing to restore for this run: %v", h.rec.list())
	}
	h.e.mu.Lock()
	r.stopping, h.e.op = true, nil // the operation ends by stopping this core
	h.e.mu.Unlock()
	close(busy.done)
}

// A core that exits before its start line is given up on purpose: it is marked stopping at once,
// so nothing acts for it, not even a watcher that is late.
func TestEarlyExitMarksTheCoreStopping(t *testing.T) {
	h := newHarness(t)
	h.core.dieEarly = 1
	h.load()
	busy := &op{kind: "enable", cancel: func() {}, done: make(chan struct{})}
	h.e.mu.Lock()
	h.e.op = busy
	h.e.mu.Unlock()
	t.Cleanup(func() {
		h.e.mu.Lock()
		h.e.op = nil
		h.e.mu.Unlock()
		close(busy.done)
	})
	r, why, retry := h.e.startCore(context.Background())
	h.e.mu.Lock()
	stopping := r.stopping
	h.e.mu.Unlock()
	if why == nil || !retry || !stopping {
		t.Fatalf("reason %+v retry %v stopping %v", why, retry, stopping)
	}
}

// The core fails between the proof of the tunnel and step 6: nothing may be switched, nothing
// may be restored (neither with nor without an older journal); error core_crashed.
func TestCrashBeforeStep6LeavesTheSystemAlone(t *testing.T) {
	for name, journal := range map[string]*store.Journal{
		"no journal":       nil,
		"existing journal": {ExitNode: "100.64.0.50", AllowLAN: false},
	} {
		h := newHarness(t)
		h.store.journal = journal
		onceOnState(h, func(s protocol.State) bool { return s.Addresses.VPN != nil }, func() { h.core.last().crash() })
		h.load()
		startEnable(h)
		wantError(t, h, protocol.ReasonCoreCrashed)
		if h.rec.index("sys:") >= 0 || h.rec.index("journal:save") >= 0 {
			t.Errorf("%s: the system exit node must stay as it was: %v", name, h.rec.list())
		}
		if on, ip := systemExitNode(h); !on || ip != exitNode {
			t.Errorf("%s: exit node on=%v ip=%s", name, on, ip)
		}
	}
}

// 5.7 and the proof without touching the system VPN: a core that exits before its start line,
// with an older journal at hand, never makes its watcher restore anything: that run switched nothing.
func TestEarlyExitWatcherLeavesTheSystemAlone(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: "100.64.0.50", AllowLAN: false}
	h.core.dieEarly = 1
	h.e.d.Core = &slowCheckCore{fakeCore: h.core, delay: 100 * time.Millisecond}
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	if n := h.rec.count("core:start"); n != 2 {
		t.Fatalf("starts %d", n)
	}
	if h.rec.index("sys:set") >= 0 {
		t.Fatalf("no restore for a run that never switched the exit node: %v", h.rec.list())
	}
	if on, _ := systemExitNode(h); on {
		t.Fatalf("the second run must have switched the system exit node off: %v", h.rec.list())
	}
}

// logEvents makes each log line of the engine an event "log:<line>" of the recorder.
func logEvents(h *harness) {
	h.e.d.Log = func(format string, args ...any) { h.rec.add("log:"+format, args...) }
}

// 5.3 step 4 and appendix A (checks of plan 1B): the Wintun device that a forced end of the core
// left behind goes right before every start of the core, after its check. A failed removal is
// logged and the start goes on: the core may still take the device, an early exit is tried again.
func TestStaleAdapterIsRemovedBeforeStart(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.host.staleAdapter = true
	h.core.dieEarly = 1
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	var order []string
	for _, ev := range h.rec.list() {
		if ev == "core:check" || ev == "host:removestale" || strings.HasPrefix(ev, "core:start") {
			order = append(order, strings.Fields(ev)[0])
		}
	}
	if got := strings.Join(order, ","); got != "core:check,host:removestale,core:start,core:check,host:removestale,core:start" {
		t.Fatalf("order %s: %v", got, h.rec.list())
	}
	if n := h.rec.count("log:removed the stale Wintun device of the adapter"); n != 1 {
		t.Fatalf("removal logged %d times: %v", n, h.rec.list())
	}

	h = newHarness(t)
	logEvents(h)
	h.host.removeErr = errBoom
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	remove, start := h.rec.index("host:removestale"), h.rec.index("core:start")
	if remove < 0 || start < remove || h.rec.count("log:stale Wintun device of the adapter not removed: boom") != 1 {
		t.Fatalf("a failed removal is logged and the start goes on: %v", h.rec.list())
	}
}
