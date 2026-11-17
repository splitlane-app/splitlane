package engine

import (
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
)

func disable(h *harness) {
	h.e.mu.Lock()
	h.e.beginDisableLocked()
	h.e.mu.Unlock()
}

// 5.4 step 1 before step 2: the exit node is back and confirmed before the core gets its signal.
func TestDisableRestoresBeforeStoppingTheCore(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	set, del, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("journal:delete"), h.rec.index("core:interrupt")
	if set < 0 || del < set || stop < del {
		t.Fatalf("order set=%d delete=%d interrupt=%d: %v", set, del, stop, h.rec.list())
	}
	if h.rec.index("core:kill") >= 0 || p.alive() {
		t.Fatal("a core that obeys the signal must not be killed")
	}
	if h.store.hasJournal() || h.store.saved().WasOn || h.e.State().SystemTailscale.RestoreOnDisable {
		t.Fatal("journal and the was-on flag must be gone")
	}
	if !h.e.State().SystemTailscale.ExitNodeOn {
		t.Fatal("the confirmed restore shows in systemTailscale.exitNodeOn")
	}
	if got := h.phases(); strings.Join(got, ",") != "on,stopping,off" {
		t.Fatalf("phases %v", got)
	}
}

// 5.4 step 1: without a confirmation the core keeps running; after 30 s the reason is restore_pending
// with the manual command; only "disable without restore" (skipRestore) lets the core stop.
func TestUnconfirmedRestoreKeepsTheCoreUntilSkip(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	h.sys.set(func(s *fakeSys) { s.ignoreSet = true })
	disable(h)
	h.waitFor("restore_pending", func() bool {
		s := h.e.State()
		return s.Phase == protocol.PhaseStopping && s.Reason != nil && s.Reason.Code == protocol.ReasonRestorePending
	})
	s := h.e.State()
	if !strings.Contains(s.Reason.Params[protocol.ParamCommand], "--exit-node=100.64.0.1 --exit-node-allow-lan-access=true") || s.Reason.Params[protocol.ParamError] == "" {
		t.Fatalf("params %v", s.Reason.Params)
	}
	if !p.alive() || h.rec.index("core:interrupt") >= 0 {
		t.Fatal("the core must not be stopped before the restore is confirmed")
	}
	if h.rec.count("sys:set") < 6 {
		t.Fatalf("attempts every 5 s for 30 s: %d", h.rec.count("sys:set"))
	}
	h.e.retryRestore(true)
	h.waitPhase(protocol.PhaseOff)
	if p.alive() || h.store.hasJournal() {
		t.Fatal("after skip the core stops and the journal is deleted")
	}
}

// A late confirmation needs no user decision.
func TestRestoreConfirmedOnALaterAttempt(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.sys.set(func(s *fakeSys) { s.setErr = errBoom })
	disable(h)
	h.waitFor("two failed attempts", func() bool { return h.rec.count("sys:set") >= 2 })
	h.sys.set(func(s *fakeSys) { s.setErr = nil })
	h.waitPhase(protocol.PhaseOff)
	if h.store.hasJournal() {
		t.Fatal("journal must be deleted after the confirmation")
	}
}

// 5.4 step 1: a system client the user disconnected never shows an exit node in its status. The exit
// node back in its settings is the confirmation: no restore_pending, the core stops, the journal goes.
func TestRestoreIntoADisconnectedClientIsConfirmedByItsSettings(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	h.sys.set(func(s *fakeSys) { s.stopped = true })
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.sawReason(protocol.ReasonRestorePending) || h.rec.count("sys:set") != 1 {
		t.Fatalf("one restore command and no restore_pending: %v", h.rec.list())
	}
	if p.alive() || h.store.hasJournal() {
		t.Fatal("the core stops and the journal is deleted")
	}
	if on, ip := systemExitNode(h); !on || ip != exitNode {
		t.Fatalf("the exit node must be back in the client's settings: %v %s", on, ip)
	}
	if h.e.State().SystemTailscale.ExitNodeOn {
		t.Fatal("a disconnected client has no exit node on")
	}
}

// The settings confirm only a disconnected client that holds the exit node: one where the command
// changed nothing, one whose settings cannot be read and one that is to connect but does not run yet
// (its status will confirm) keep the restore unconfirmed and the core running.
func TestSettingsDoNotConfirmWithoutADisconnectedClientHoldingTheExitNode(t *testing.T) {
	cases := map[string]func(s *fakeSys){
		"the command changed nothing": func(s *fakeSys) { s.stopped, s.ignoreSet = true, true },
		"settings not read":           func(s *fakeSys) { s.stopped, s.prefsErr = true, errBoom },
		"the client still starts":     func(s *fakeSys) { s.starting = true },
	}
	for name, set := range cases {
		h := newHarness(t)
		p := h.running()
		h.sys.set(set)
		disable(h)
		h.waitFor(name+": restore_pending", func() bool {
			s := h.e.State()
			return s.Phase == protocol.PhaseStopping && s.Reason != nil && s.Reason.Code == protocol.ReasonRestorePending
		})
		if !p.alive() || !h.store.hasJournal() {
			t.Fatalf("%s: the core must keep running and the journal must stay", name)
		}
		h.e.retryRestore(true)
		h.waitPhase(protocol.PhaseOff)
	}
}

// 5.7: an exit node the user selected himself (phase blocked) stays as it is; the journal goes.
func TestUsersOwnExitNodeIsKept(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.rec.index("sys:set") >= 0 {
		t.Fatalf("the user's exit node must not be changed: %v", h.rec.list())
	}
	if h.store.hasJournal() {
		t.Fatal("journal must be deleted")
	}
}

// 5.4 step 2: a core that ignores the signal is ended after 5 s; a failed signal ends it at once.
func TestStubbornCoreIsKilled(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	p.ignoreInterrupt = true
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.rec.index("core:interrupt") < 0 || h.rec.index("core:kill") < h.rec.index("core:interrupt") {
		t.Fatalf("signal first, then the forced end: %v", h.rec.list())
	}

	h = newHarness(t)
	p = h.running()
	p.interruptErr = errBoom
	before := h.clock.Now()
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.rec.index("core:kill") < 0 {
		t.Fatal("a failed signal must end the core at once")
	}
	if waited := h.clock.Now().Sub(before); waited >= stopWait {
		t.Fatalf("no 5 s wait after a failed signal, waited %s", waited)
	}
}

// 5.4 step 3: an adapter that stays for 10 s gives error with reason adapter_left.
func TestAdapterLeft(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.host.adapterStuck = true
	disable(h)
	h.waitPhase(protocol.PhaseError)
	if r := h.e.State().Reason; r == nil || r.Code != protocol.ReasonAdapterLeft {
		t.Fatalf("reason %+v", r)
	}
}

// Without a journal the system Tailscale is not touched at all.
func TestNoJournalNoTailscaleCalls(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.store.journal = nil
	h.e.mu.Lock()
	h.e.journal = nil
	h.e.mu.Unlock()
	disable(h)
	h.waitPhase(protocol.PhaseOff)
	if h.rec.index("sys:") >= 0 {
		t.Fatalf("no tailscale call expected: %v", h.rec.list())
	}
}

// rollback after a failure ends in error with the reason; after a cancel it ends in off.
func TestRollbackFinalPhase(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.e.rollback(reason(protocol.ReasonTunnelNotProven), false)
	if s := h.e.State(); s.Phase != protocol.PhaseError || s.Reason.Code != protocol.ReasonTunnelNotProven {
		t.Fatalf("state %+v", s)
	}
	h = newHarness(t)
	h.running()
	h.e.rollback(errCancelled, true)
	if s := h.e.State(); s.Phase != protocol.PhaseOff || s.Reason != nil {
		t.Fatalf("state %+v", s)
	}
	if h.sawReason("cancelled") {
		t.Fatal("the internal cancel reason must never be published")
	}
}

// publishedCount is the number of states published so far.
func publishedCount(h *harness) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.states)
}

// While another operation still holds e.op (the enable and restart ops publish "on" and unlock
// before startOp's deferred cleanup clears it), a refused beginDisableLocked must change nothing:
// not the phase, not "was on", not state.json, not the published states, not the core or the system client.
func TestBusyDisableChangesNothing(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	busy := &op{kind: "enable", cancel: func() {}, done: make(chan struct{})}
	h.e.mu.Lock()
	h.e.op = busy
	h.e.mu.Unlock()
	statesBefore := publishedCount(h)
	eventsBefore := len(h.rec.list())

	h.e.mu.Lock()
	started := h.e.beginDisableLocked()
	h.e.mu.Unlock()

	if started {
		t.Fatal("beginDisableLocked must refuse while another operation runs")
	}
	s := h.e.State()
	if s.Phase != protocol.PhaseOn || s.Reason != nil {
		t.Fatalf("phase must stay on without a reason: %+v", s)
	}
	h.e.mu.Lock()
	wasOn, same := h.e.saved.WasOn, h.e.op == busy
	h.e.mu.Unlock()
	if !wasOn || !h.store.saved().WasOn {
		t.Fatalf("was-on must stay set: engine %v, state.json %v", wasOn, h.store.saved().WasOn)
	}
	if !same {
		t.Fatal("the running operation must stay in place")
	}
	if got := publishedCount(h); got != statesBefore {
		t.Fatalf("no state may be published by a refused disable: %d new", got-statesBefore)
	}
	if got := h.rec.list(); len(got) != eventsBefore {
		t.Fatalf("a refused disable must not touch the core or the system client: %v", got[eventsBefore:])
	}
	if !p.alive() || !h.store.hasJournal() {
		t.Fatal("the core must keep running and the journal must stay")
	}
}
