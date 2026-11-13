package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	"splitlane/helper/internal/protocol"
)

const (
	idA = "11111111-1111-4111-8111-111111111111"
	idB = "22222222-2222-4222-8222-222222222222"
)

var (
	curlA = `C:\Tools\a\curl-a.exe`
	curlB = `C:\Tools\b\curl-b.exe`
)

func call(h *harness, method string, params any) *protocol.Error {
	var raw json.RawMessage
	if params != nil {
		raw, _ = json.Marshal(params)
	}
	_, err := h.e.Handle(context.Background(), method, raw)
	return err
}

func wantCode(t *testing.T, err *protocol.Error, code string) {
	t.Helper()
	if err == nil || err.Code != code {
		t.Fatalf("want %s, got %v", code, err)
	}
}

func fileRule(id, exe, lane string) protocol.AppRule {
	return protocol.AppRule{ID: id, Exe: exe, Matcher: protocol.Matcher{Kind: protocol.KindFile}, Lane: lane}
}

func setPhase(h *harness, phase string) {
	h.e.mu.Lock()
	h.e.setPhase(phase, nil)
	h.e.mu.Unlock()
}

func TestUnknownAndLaterMethods(t *testing.T) {
	h := newHarness(t)
	wantCode(t, call(h, "routing.toggle", nil), protocol.CodeUnknownMethod)
	for _, m := range []string{protocol.MethodLogin, protocol.MethodLogout, protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport} {
		wantCode(t, call(h, m, nil), protocol.CodeInternal)
	}
	res, err := h.e.Handle(context.Background(), protocol.MethodStateGet, nil)
	if err != nil || res.(protocol.State).Phase != protocol.PhaseOff {
		t.Fatalf("state.get: %v %v", res, err)
	}
	wantCode(t, call(h, protocol.MethodEnable, map[string]any{"now": true}), protocol.CodeBadRequest)
}

// routing.enable by phase (5.1).
func TestEnableByPhase(t *testing.T) {
	h := newHarness(t)
	h.load()
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
	if err := call(h, protocol.MethodEnable, nil); err != nil || h.rec.count("core:start") != 1 {
		t.Fatalf("in on: no action, got %v", err)
	}
	setPhase(h, protocol.PhaseDegraded)
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatalf("in degraded (retry): %v", err)
	}
	setPhase(h, protocol.PhaseStopping)
	wantCode(t, call(h, protocol.MethodEnable, nil), protocol.CodeBusy)

	h = newHarness(t)
	h.load()
	setPhase(h, protocol.PhaseError)
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
}

// routing.disable by phase (5.1).
func TestDisableByPhase(t *testing.T) {
	h := newHarness(t)
	h.load()
	if err := call(h, protocol.MethodDisable, nil); err != nil || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("off: %v", err)
	}
	h.e.mu.Lock()
	h.e.finish(protocol.PhaseError, reason(protocol.ReasonTunnelNotProven))
	h.e.mu.Unlock()
	if err := call(h, protocol.MethodDisable, nil); err != nil || h.e.State().Phase != protocol.PhaseOff || h.e.State().Reason != nil {
		t.Fatalf("error -> off: %v %+v", err, h.e.State())
	}

	h = enabled(t)
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)

	h = newHarness(t)
	h.probe.hang = true
	h.load()
	call(h, protocol.MethodEnable, nil)
	h.waitFor("core started", func() bool { return h.rec.index("core:start") >= 0 })
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
}

// In stopping with restore_pending: "Повторить" (plain disable) keeps trying, skipRestore gives up.
func TestDisableInRestorePending(t *testing.T) {
	h := enabled(t)
	h.sys.set(func(s *fakeSys) { s.ignoreSet = true })
	call(h, protocol.MethodDisable, nil)
	h.waitFor("restore_pending", func() bool { return h.sawReason(protocol.ReasonRestorePending) })
	if err := call(h, protocol.MethodDisable, nil); err != nil || !h.store.hasJournal() {
		t.Fatalf("retry must keep the journal: %v", err)
	}
	if err := call(h, protocol.MethodDisable, protocol.DisableParams{SkipRestore: true}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
	if h.store.hasJournal() || h.core.last().alive() {
		t.Fatal("skip: journal deleted, core stopped")
	}
}

// applied waits until a routing.allDirect apply that holds the queue is done.
func applied(h *harness) {
	h.e.queue.Lock()
	h.e.queue.Unlock()
}

// inOrder reports whether the events from mark on hold these prefixes in this order (other events
// may come between them).
func inOrder(h *harness, mark int, prefixes ...string) bool {
	for _, ev := range h.rec.list()[mark:] {
		if len(prefixes) > 0 && strings.HasPrefix(ev, prefixes[0]) {
			prefixes = prefixes[1:]
		}
	}
	return len(prefixes) == 0
}

// routing.allDirect by phase, its reconciliation (8.3) and the DNS cache (6.4).
func TestAllDirect(t *testing.T) {
	h := newHarness(t)
	h.load()
	wantCode(t, call(h, protocol.MethodAllDirect, map[string]bool{"on": true}), protocol.CodeWrongPhase)
	h = enabled(t)
	wantCode(t, call(h, protocol.MethodAllDirect, nil), protocol.CodeBadRequest)
	mark := len(h.rec.list())
	if err := call(h, protocol.MethodAllDirect, map[string]bool{"on": true}); err != nil {
		t.Fatal(err)
	}
	applied(h)
	if !h.e.State().AllDirect || h.clash.currentMode() != "alldirect" {
		t.Fatalf("flag %v mode %s", h.e.State().AllDirect, h.clash.currentMode())
	}
	if countSince(h, mark, "dns:flush") != 1 || !inOrder(h, mark, "clash:mode alldirect", "dns:flush") {
		t.Fatalf("6.4: the switch to the adapter's DNS flushes the Windows cache after it: %v", h.rec.list()[mark:])
	}
	// a connection opened in "all direct" by an unpinned program stays wrong after the return: closed
	h.clash.conns = []Conn{{ID: "c1", ProcessPath: curlA, Lane: "direct", ByLanes: true}}
	mark = len(h.rec.list())
	if err := call(h, protocol.MethodAllDirect, map[string]bool{"on": false}); err != nil {
		t.Fatal(err)
	}
	applied(h)
	if h.e.State().AllDirect || countSince(h, mark, "clash:close c1") != 1 || countSince(h, mark, "dns:flush") != 1 ||
		!inOrder(h, mark, "clash:mode vpn", "dns:flush") {
		t.Fatalf("off: closed c1 and flushed the cache again after the switch: %v", h.rec.list()[mark:])
	}
	setPhase(h, protocol.PhaseBlocked)
	wantCode(t, call(h, protocol.MethodAllDirect, map[string]bool{"on": true}), protocol.CodeWrongPhase)

	// The switch is taken, the reconciliation fails: the previous mode comes back and with it the
	// VPN's DNS, so the cache is flushed after the switch and again after the return.
	h = enabled(t)
	h.clash.conns = []Conn{{ID: "stuck", ProcessPath: curlA, Lane: "vpn", ByLanes: true}}
	h.clash.reconnect = func(c Conn) *Conn { return &c } // the core keeps it on the VPN lane
	mark = len(h.rec.list())
	call(h, protocol.MethodAllDirect, map[string]bool{"on": true})
	applied(h)
	if h.e.State().AllDirect || h.clash.currentMode() != "vpn" || countSince(h, mark, "dns:flush") != 2 ||
		!inOrder(h, mark, "clash:mode alldirect", "dns:flush", "clash:mode vpn", "dns:flush") {
		t.Fatalf("failed apply: the previous mode and two flushes: %v", h.rec.list()[mark:])
	}

	// In degraded the names go through the adapter's DNS already: "all direct" moves no DNS.
	h = enabled(t)
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, nil)
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseDegraded {
		t.Fatalf("setup: phase %s", h.e.State().Phase)
	}
	flushes := h.rec.count("dns:flush")
	call(h, protocol.MethodAllDirect, map[string]bool{"on": true})
	applied(h)
	call(h, protocol.MethodAllDirect, map[string]bool{"on": false})
	applied(h)
	if h.rec.count("clash:mode alldirect") != 1 || h.clash.currentMode() != "vpn-localdns" || h.rec.count("dns:flush") != flushes {
		t.Fatalf("degraded: no flush expected: %v", h.rec.list())
	}
}

func TestAllDirectNotTakenKeepsTheFlag(t *testing.T) {
	h := enabled(t)
	h.clash.refuse["alldirect"] = true
	call(h, protocol.MethodAllDirect, map[string]bool{"on": true})
	applied(h)
	if h.e.State().AllDirect || h.clash.currentMode() != "vpn" {
		t.Fatalf("flag %v mode %s", h.e.State().AllDirect, h.clash.currentMode())
	}
}

// lanes.set while off: checked, normalized and saved; nothing is applied.
func TestLanesSetWhileOff(t *testing.T) {
	h := newHarness(t)
	h.load()
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "direct",
		Apps: []protocol.AppRule{{ID: idA, Exe: curlA, Matcher: protocol.Matcher{Kind: protocol.KindFolder}, Lane: "vpn"}}})
	if err != nil {
		t.Fatal(err)
	}
	s := h.e.State()
	if s.DefaultLane != "direct" || len(s.Apps) != 1 || s.Apps[0].Matcher.Value != `C:\Tools\a\` || s.Apps[0].Name != "curl-a" {
		t.Fatalf("state %+v", s)
	}
	if h.store.saved().DefaultLane != "direct" || h.rec.index("rules:write") >= 0 {
		t.Fatalf("saved %+v, events %v", h.store.saved(), h.rec.list())
	}
}

// lanes.set is atomic: one rejected rule, nothing saved; malformed input is bad_request.
func TestLanesSetRejections(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.e.d.Rules.Protected = []string{`C:\Tools\b\curl-b.exe`}
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct"), fileRule(idB, curlB, "direct")}})
	wantCode(t, err, protocol.CodeRuleRejected)
	if len(err.Rules) != 1 || err.Rules[0] != (protocol.RuleRejection{ID: idB, Reason: protocol.RejectProtected}) {
		t.Fatalf("rules %+v", err.Rules)
	}
	if len(h.store.saved().Apps) != 0 {
		t.Fatal("nothing may be saved")
	}
	wantCode(t, call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule("1", curlA, "direct")}}), protocol.CodeBadRequest)
	wantCode(t, call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, "curl-a.exe", "direct")}}), protocol.CodeBadRequest)
	setPhase(h, protocol.PhaseStarting)
	wantCode(t, call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn"}), protocol.CodeBusy)
}

// lanes.set while on: files and mode, then 3 s of reconciliation: a connection on the wrong lane
// is closed (it comes back on the right one); connections of built-in rules are left alone.
func TestLanesSetWhileOn(t *testing.T) {
	h := enabled(t)
	h.clash.conns = []Conn{
		{ID: "moved", ProcessPath: curlA, Lane: "vpn", ByLanes: true},
		{ID: "probe", ProcessPath: "", Lane: "vpn", ByLanes: false}, // a built-in rule (probe-vpn) chose it
		{ID: "free", ProcessPath: `C:\Other\x.exe`, Lane: "vpn", ByLanes: true},
	}
	h.clash.reconnect = func(c Conn) *Conn {
		return &Conn{ID: c.ID + "-again", ProcessPath: c.ProcessPath, Lane: "direct", ByLanes: true}
	}
	before := h.clock.Now()
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
	if err != nil {
		t.Fatal(err)
	}
	if h.rec.index("clash:close moved") < 0 || h.rec.index("clash:close probe") >= 0 || h.rec.index("clash:close free") >= 0 {
		t.Fatalf("events %v", h.rec.list())
	}
	if !strings.Contains(string(h.store.direct), `curl-a\\.exe$`) {
		t.Fatalf("direct file:\n%s", h.store.direct)
	}
	if h.clock.Now().Sub(before) < applyWindow {
		t.Fatal("the reconciliation lasts 3 s")
	}
	if h.store.saved().Apps[0].ID != idA {
		t.Fatal("layout not saved")
	}
	// the main lane: direct makes the mode direct; an unpinned program moves
	h.clash.conns = []Conn{{ID: "free2", ProcessPath: `C:\Other\x.exe`, Lane: "vpn", ByLanes: true}}
	if err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "direct",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}}); err != nil {
		t.Fatal(err)
	}
	if h.clash.currentMode() != "direct" || h.rec.index("clash:close free2") < 0 {
		t.Fatalf("mode %s events %v", h.clash.currentMode(), h.rec.list())
	}
}

// Review Focus 2: the core reads the new file about a second later; until then a closed connection
// comes back on the old lane. The check closes it again and succeeds once the core has the file.
func TestLanesSetConvergesWhileTheCoreReloads(t *testing.T) {
	h := enabled(t)
	h.clash.conns = []Conn{{ID: "app", ProcessPath: curlA, Lane: "vpn", ByLanes: true}}
	closes := 0
	h.clash.reconnect = func(c Conn) *Conn {
		closes++
		n := c
		n.ID = fmt.Sprintf("app-%d", closes)
		if closes >= 3 {
			n.Lane = "direct"
		}
		return &n
	}
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
	if err != nil || closes != 3 {
		t.Fatalf("err %v, closes %d", err, closes)
	}
}

// Section 10 row "rule file not applied": the previous files and mode come back, apply_failed.
func TestLanesSetFailuresRestorePrevious(t *testing.T) {
	h := enabled(t)
	oldDirect := append([]byte{}, h.store.direct...)
	h.store.onRules = func() { h.core.appendLog("ERROR router: reload rule-set lane-direct: parse error") }
	err := call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "direct",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
	wantCode(t, err, protocol.CodeApplyFailed)
	if string(h.store.direct) != string(oldDirect) || h.clash.currentMode() != "vpn" || len(h.store.saved().Apps) != 0 {
		t.Fatalf("not restored: mode %s, saved %+v", h.clash.currentMode(), h.store.saved().Apps)
	}

	h = enabled(t)
	h.clash.conns = []Conn{{ID: "stuck", ProcessPath: curlA, Lane: "vpn", ByLanes: true}}
	h.clash.reconnect = func(c Conn) *Conn { return &c } // the core keeps the old rules
	err = call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn",
		Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
	wantCode(t, err, protocol.CodeApplyFailed)
	if len(h.store.saved().Apps) != 0 {
		t.Fatal("nothing saved after a failed apply")
	}
}

// Commands that change state run one at a time: a second one waits for lanes.set (5.1).
func TestCommandsRunOneAtATime(t *testing.T) {
	h := enabled(t)
	done := make(chan struct{})
	go func() {
		call(h, protocol.MethodLanesSet, protocol.LanesSetParams{DefaultLane: "vpn", Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}})
		close(done)
	}()
	h.waitFor("lanes.set running", func() bool { return h.rec.count("rules:write") == 2 })
	call(h, protocol.MethodAllDirect, map[string]bool{"on": true})
	select {
	case <-done:
	default:
		t.Fatal("routing.allDirect was answered while lanes.set still ran")
	}
}

// tailscale.setExitNode: saved while off; checked against the list; while on: off and on again.
func TestSetExitNode(t *testing.T) {
	h := newHarness(t)
	h.load()
	if err := call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.2"}); err != nil {
		t.Fatal(err)
	}
	if h.store.saved().SelectedExitNode != "100.64.0.2" || *h.e.State().Tailscale.SelectedExitNode != "100.64.0.2" {
		t.Fatal("not saved")
	}
	h.e.mu.Lock()
	h.e.saved.ExitNodes = []protocol.ExitNode{{ID: "100.64.0.2", Name: "a", Online: true}}
	h.e.mu.Unlock()
	wantCode(t, call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.3"}), protocol.CodeUnknownExitNode)

	h = enabled(t)
	if err := call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.5"}); err != nil {
		t.Fatal(err)
	}
	h.waitFor("second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if h.core.options().ExitNode != "100.64.0.5" {
		t.Fatal("the new core must use the new exit node")
	}
	if h.rec.index("sys:set 100.64.0.1 true") < 0 {
		t.Fatal("the system exit node is restored between the two runs")
	}
}

func TestClearSystemExitNodeByPhase(t *testing.T) {
	h := newHarness(t)
	h.load()
	wantCode(t, call(h, protocol.MethodClearSystemExitNode, nil), protocol.CodeWrongPhase)
	h = enabled(t)
	if err := call(h, protocol.MethodClearSystemExitNode, nil); err != nil || h.rec.count("sys:clear") != 1 {
		t.Fatalf("in on: no action, got %v", err)
	}
}

// Outcomes of a cell of the phase table: the action of the table happened, or nothing changed;
// any other value is the error code of the answer.
const (
	cellAct  = "act"
	cellNone = "none"
)

// tablePhases are the columns of the phase table of 5.1, one phase each.
var tablePhases = []string{protocol.PhaseOff, protocol.PhaseError, protocol.PhaseStarting, protocol.PhaseOn,
	protocol.PhaseDegraded, protocol.PhaseBlocked, protocol.PhaseStopping}

// slowSys is a tailscale.exe whose `status` answers only after hold is closed.
type slowSys struct {
	*fakeSys
	hold chan struct{}
}

func (s slowSys) Status(ctx context.Context) (SysStatus, error) {
	<-s.hold
	return s.fakeSys.Status(ctx)
}

// inPhase puts a new harness into the phase for real: starting waits in step 5 of 5.3 (a probe
// that hangs), stopping waits in step 1 of 5.4 (a tailscale.exe that does not answer yet; no
// restore_pending). release lets a waiting operation go on and waits until it is done.
func inPhase(t *testing.T, phase string) (*harness, func()) {
	t.Helper()
	var h *harness
	wait := func() {}
	switch phase {
	case protocol.PhaseOff:
		h = newHarness(t)
		h.load()
	case protocol.PhaseError:
		h = newHarness(t)
		h.load()
		h.e.mu.Lock()
		h.e.finish(protocol.PhaseError, reason(protocol.ReasonTunnelNotProven))
		h.e.mu.Unlock()
	case protocol.PhaseStarting:
		h = newHarness(t)
		h.probe.hang = true
		h.load()
		call(h, protocol.MethodEnable, nil)
		h.waitFor("core started", func() bool { return h.rec.index("core:start") >= 0 })
		wait = func() { call(h, protocol.MethodDisable, nil) }
	case protocol.PhaseOn:
		h = enabled(t)
	case protocol.PhaseDegraded:
		h = enabled(t)
		h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, nil)
		rounds(h, 3)
		h.probe.script([]ProbeResult{atVPN}, nil, nil)
	case protocol.PhaseBlocked:
		h = enabled(t)
		h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
		h.e.pollSystem(context.Background())
	case protocol.PhaseStopping:
		h = enabled(t)
		hold := make(chan struct{})
		h.e.d.Sys = slowSys{h.sys, hold}
		call(h, protocol.MethodDisable, nil)
		wait = func() { close(hold) }
	}
	if got := h.e.State().Phase; got != phase {
		t.Fatalf("setup: phase %s, want %s; events %v", got, phase, h.rec.list())
	}
	var once sync.Once
	return h, func() {
		once.Do(wait)
		h.e.Wait()
	}
}

func exitedNow(h *harness) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.exited
}

// disabledAct checks a routing.disable that acted: off, no core, no journal, and the system
// exit node as 5.4 and 5.7 want it in that phase; skip is skipRestore: true (no restore, 5.1).
func disabledAct(skip bool) func(t *testing.T, h *harness, phase string, mark int, release func()) {
	return func(t *testing.T, h *harness, phase string, mark int, release func()) {
		if phase == protocol.PhaseStopping { // only with skipRestore: no restore, the core is stopped
			h.sys.set(func(s *fakeSys) { s.ignoreSet = true })
			release()
		}
		h.waitPhase(protocol.PhaseOff)
		if h.e.State().Reason != nil || h.core.anyAlive() || h.store.hasJournal() || h.store.saved().WasOn {
			t.Fatalf("off, no core, no journal, was-on cleared: %+v %v", h.e.State(), h.rec.list())
		}
		on, ip := systemExitNode(h)
		switch phase {
		case protocol.PhaseStarting: // step 6 was not reached: the system exit node was never touched
			if countSince(h, mark, "sys:") != 0 || !on {
				t.Fatalf("events %v", h.rec.list())
			}
		case protocol.PhaseBlocked: // the exit node the user selected stays (5.7)
			if countSince(h, mark, "sys:set") != 0 || !on || ip != "100.64.0.99" {
				t.Fatalf("the user's exit node must stay: %v", h.rec.list())
			}
		case protocol.PhaseOn, protocol.PhaseDegraded:
			if skip { // the journal goes without a restore, the system exit node stays off
				if countSince(h, mark, "sys:set") != 0 || on {
					t.Fatalf("no restore with skipRestore: %v", h.rec.list())
				}
			} else if countSince(h, mark, "sys:set "+exitNode) == 0 || !on || ip != exitNode { // 5.4 restores
				t.Fatalf("restore expected: %v", h.rec.list())
			}
		case protocol.PhaseStopping: // skipRestore: an attempt already under way ends, no new one follows
			if on {
				t.Fatalf("no restore with skipRestore: %v", h.rec.list())
			}
		}
	}
}

// TestPhaseTable walks the phase table of 5.1 cell by cell for the methods of this build
// (tailscale.login and tailscale.logout answer internal in every phase: TestUnknownAndLaterMethods).
// routing.disable with skipRestore disables without a restore wherever it disables (5.1).
func TestPhaseTable(t *testing.T) {
	busy, wrong := protocol.CodeBusy, protocol.CodeWrongPhase
	lanes := protocol.LanesSetParams{DefaultLane: "vpn", Apps: []protocol.AppRule{fileRule(idA, curlA, "direct")}}
	rows := []struct {
		name   string
		method string
		params any
		want   []string // off, error, starting, on, degraded, blocked, stopping
		act    func(t *testing.T, h *harness, phase string, mark int, release func())
	}{
		{"enable", protocol.MethodEnable, nil,
			[]string{cellAct, cellAct, cellNone, cellNone, cellAct, cellNone, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if phase == protocol.PhaseDegraded { // "Повторить": an extra check; the VPN lane answers again
					h.waitFor("on after the extra check", func() bool {
						return h.e.State().Phase == protocol.PhaseOn && h.clash.currentMode() == "vpn" && countSince(h, mark, "dns:flush") == 1
					})
					return
				}
				h.waitPhase(protocol.PhaseOn)
				if countSince(h, mark, "core:start") != 1 {
					t.Fatalf("one enable: %v", h.rec.list())
				}
			}},
		{"disable", protocol.MethodDisable, nil,
			[]string{cellNone, cellAct, cellAct, cellAct, cellAct, cellAct, cellNone}, disabledAct(false)},
		{"disable skipRestore", protocol.MethodDisable, protocol.DisableParams{SkipRestore: true},
			[]string{cellNone, cellAct, cellAct, cellAct, cellAct, cellAct, cellAct}, disabledAct(true)},
		{"allDirect", protocol.MethodAllDirect, map[string]bool{"on": true},
			[]string{wrong, wrong, busy, cellAct, cellAct, wrong, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if !h.e.State().AllDirect || countSince(h, mark, "clash:mode alldirect") != 1 {
					t.Fatalf("all direct applied: %v", h.rec.list())
				}
			}},
		{"lanes.set", protocol.MethodLanesSet, lanes,
			[]string{cellAct, cellAct, busy, cellAct, cellAct, cellAct, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				applied := countSince(h, mark, "rules:write") > 0
				if applied != live(phase) || len(h.store.saved().Apps) != 1 {
					t.Fatalf("applied %v (only while the core runs), saved %+v", applied, h.store.saved().Apps)
				}
			}},
		{"setExitNode", protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.5"},
			[]string{cellAct, cellAct, busy, cellAct, cellAct, cellAct, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if h.store.saved().SelectedExitNode != "100.64.0.5" {
					t.Fatal("not saved")
				}
				if !live(phase) {
					if countSince(h, mark, "core:") != 0 {
						t.Fatalf("saved only: %v", h.rec.list())
					}
					return
				}
				h.waitFor("off and on again", func() bool {
					return countSince(h, mark, "core:start") == 1 && h.e.State().Phase == protocol.PhaseOn && h.idle()
				})
			}},
		{"clearExitNode", protocol.MethodClearSystemExitNode, nil,
			[]string{wrong, wrong, busy, cellNone, cellNone, cellAct, busy},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if countSince(h, mark, "sys:clear") != 1 || h.e.State().Phase != protocol.PhaseOn {
					t.Fatalf("cleared in blocked: %s %v", h.e.State().Phase, h.rec.list())
				}
			}},
		{"shutdown", protocol.MethodShutdown, nil,
			[]string{cellAct, cellAct, cellAct, cellAct, cellAct, cellAct, cellAct},
			func(t *testing.T, h *harness, phase string, mark int, release func()) {
				if phase == protocol.PhaseStopping {
					if exitedNow(h) {
						t.Fatal("in stopping the helper waits for the end of the disabling")
					}
					release()
				}
				h.waitFor("exit", func() bool { return exitedNow(h) })
				want := protocol.PhaseOff
				if phase == protocol.PhaseError {
					want = protocol.PhaseError
				}
				if h.e.State().Phase != want || h.core.anyAlive() || h.store.saved().WasOn {
					t.Fatalf("phase %s, events %v", h.e.State().Phase, h.rec.list())
				}
			}},
	}
	for _, row := range rows {
		for i, phase := range tablePhases {
			t.Run(row.name+"/"+phase, func(t *testing.T) {
				h, release := inPhase(t, phase)
				defer release()
				before, phaseBefore := h.rec.list(), h.e.State().Phase
				err := call(h, row.method, row.params)
				h.e.queue.Lock() // an apply that took over the queue is done
				h.e.queue.Unlock()
				switch want := row.want[i]; want {
				case cellAct:
					if err != nil {
						t.Fatalf("answer %v", err)
					}
					row.act(t, h, phase, len(before), release)
				case cellNone:
					if err != nil {
						t.Fatalf("answer %v", err)
					}
					if strings.Join(h.rec.list(), "|") != strings.Join(before, "|") || h.e.State().Phase != phaseBefore {
						t.Fatalf("no action expected: phase %s, events %v", h.e.State().Phase, h.rec.list())
					}
				default:
					wantCode(t, err, want)
				}
			})
		}
	}
}

// helper.shutdown: disable (or cancel the enable), then the process ends.
func TestShutdown(t *testing.T) {
	h := newHarness(t)
	h.load()
	call(h, protocol.MethodShutdown, nil)
	h.waitFor("exit", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.exited })

	h = enabled(t)
	call(h, protocol.MethodShutdown, nil)
	h.waitFor("exit", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.exited })
	if h.e.State().Phase != protocol.PhaseOff || h.core.last().alive() || h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("shutdown must disable first: %+v", h.e.State())
	}
	wantCode(t, call(h, protocol.MethodEnable, nil), protocol.CodeBusy)

	h = newHarness(t)
	h.probe.hang = true
	h.load()
	call(h, protocol.MethodEnable, nil)
	h.waitFor("core started", func() bool { return h.rec.index("core:start") >= 0 })
	call(h, protocol.MethodShutdown, nil)
	h.waitFor("exit", func() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.exited })
	if h.e.State().Phase != protocol.PhaseOff || h.core.last().alive() {
		t.Fatalf("shutdown in starting rolls back: %+v", h.e.State())
	}
}

// holdOp puts an operation in place that has ended in substance but whose cleanup has not run: the
// enable and restart operations publish "on" and release e.mu before startOp's deferred cleanup
// clears e.op. release is that cleanup.
func holdOp(h *harness) (release func()) {
	o := &op{kind: "enable", cancel: func() {}, done: make(chan struct{})}
	h.e.mu.Lock()
	h.e.op = o
	h.e.mu.Unlock()
	return func() {
		h.e.mu.Lock()
		h.e.op = nil
		h.e.mu.Unlock()
		close(o.done)
	}
}

// helper.shutdown in a live phase while the operation that published it still holds e.op: the
// helper must neither exit with the core running nor leave the system exit node off. The disabling
// of 5.4 runs as soon as the operation is gone, and only then the process ends.
func TestShutdownWhileAnOpFinishes(t *testing.T) {
	for _, phase := range []string{protocol.PhaseOn, protocol.PhaseDegraded, protocol.PhaseBlocked} {
		t.Run(phase, func(t *testing.T) {
			h, _ := inPhase(t, phase)
			release := holdOp(h)
			if err := call(h, protocol.MethodShutdown, nil); err != nil {
				t.Fatalf("shutdown must be accepted in every phase: %v", err)
			}
			if exitedNow(h) || h.e.State().Phase != phase || !h.core.anyAlive() {
				t.Fatalf("the helper must wait for the operation: phase %s, events %v", h.e.State().Phase, h.rec.list())
			}
			release()
			h.waitFor("exit", func() bool { return exitedNow(h) })
			if h.e.State().Phase != protocol.PhaseOff || h.core.anyAlive() || h.store.hasJournal() || h.store.saved().WasOn {
				t.Fatalf("the helper exited before 5.4 was done: phase %s, events %v", h.e.State().Phase, h.rec.list())
			}
			on, ip := systemExitNode(h)
			switch phase {
			case protocol.PhaseBlocked: // the exit node the user selected stays (5.7)
				if !on || ip != "100.64.0.99" {
					t.Fatalf("the user's exit node must stay: %v", h.rec.list())
				}
			default: // the system exit node is given back (5.4 step 1)
				if !on || ip != exitNode {
					t.Fatalf("the system exit node must be given back: %v", h.rec.list())
				}
			}
		})
	}
}

// A core that crashes after helper.shutdown was accepted in that window is still stopped by the
// disabling (the watcher ignores it once exiting is set); the process ends only afterwards.
func TestShutdownWhileAnOpFinishesAfterTheCoreDied(t *testing.T) {
	h := enabled(t)
	release := holdOp(h)
	call(h, protocol.MethodShutdown, nil)
	h.core.last().crash()
	release()
	h.waitFor("exit", func() bool { return exitedNow(h) })
	if h.e.State().Phase != protocol.PhaseOff || h.core.anyAlive() || h.store.hasJournal() {
		t.Fatalf("phase %s, events %v", h.e.State().Phase, h.rec.list())
	}
	if on, ip := systemExitNode(h); !on || ip != exitNode {
		t.Fatalf("the system exit node must be given back: %v", h.rec.list())
	}
}

// tailscale.setExitNode in a live phase while an operation still holds e.op: busy, and nothing
// changes (no phase "stopping" without an operation behind it, no new choice saved); once the
// operation is gone the same command works.
func TestSetExitNodeBusyWhileAnOpFinishes(t *testing.T) {
	h := enabled(t)
	release := holdOp(h)
	states, events := publishedCount(h), len(h.rec.list())
	wantCode(t, call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.5"}), protocol.CodeBusy)
	if s := h.e.State(); s.Phase != protocol.PhaseOn || s.Reason != nil {
		t.Fatalf("phase must stay on: %+v", s)
	}
	if got := h.store.saved().SelectedExitNode; got != exitNode {
		t.Fatalf("a refused command must not save the choice: %q", got)
	}
	if got := publishedCount(h); got != states {
		t.Fatalf("a refused command must not publish: %d new states", got-states)
	}
	if got := h.rec.list(); len(got) != events {
		t.Fatalf("a refused command must not touch the core or the system client: %v", got[events:])
	}
	release()
	if err := call(h, protocol.MethodSetExitNode, map[string]string{"id": "100.64.0.5"}); err != nil {
		t.Fatal(err)
	}
	h.waitFor("second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
}

// routing.enable in off or error while an operation still holds e.op (the cleanup of a disable or a
// rollback that has published its final phase): busy, and the phase does not move to "starting"
// without an operation behind it; once the operation is gone the enable works.
func TestEnableBusyWhileAnOpFinishes(t *testing.T) {
	for _, phase := range []string{protocol.PhaseOff, protocol.PhaseError} {
		t.Run(phase, func(t *testing.T) {
			h, _ := inPhase(t, phase)
			release := holdOp(h)
			states, events := publishedCount(h), len(h.rec.list())
			wantCode(t, call(h, protocol.MethodEnable, nil), protocol.CodeBusy)
			if got := h.e.State().Phase; got != phase {
				t.Fatalf("phase must stay %s, got %s", phase, got)
			}
			if got := publishedCount(h); got != states {
				t.Fatalf("a refused command must not publish: %d new states", got-states)
			}
			if got := h.rec.list(); len(got) != events {
				t.Fatalf("a refused command must not touch the core: %v", got[events:])
			}
			release()
			if err := call(h, protocol.MethodEnable, nil); err != nil {
				t.Fatal(err)
			}
			h.waitPhase(protocol.PhaseOn)
		})
	}
}
