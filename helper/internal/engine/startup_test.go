package engine

import (
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// leftover puts a still running core of an earlier helper into the fakes and state.json.
func leftover(h *harness) *fakeProc {
	p := &fakeProc{rec: h.rec, id: store.CoreIdentity{PID: 777, Created: 99, Path: `C:\core\sing-box.exe`}, done: make(chan struct{})}
	h.core.leftover = p
	id := p.id
	h.store.state.Core = &id
	return p
}

func TestCleanStartIsOff(t *testing.T) {
	h := newHarness(t)
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if len(h.rec.list()) != 0 {
		t.Fatalf("nothing to do at a clean start: %v", h.rec.list())
	}
}

// Section 12: the helper was ended between steps 6 and 9 of 5.3: the new one gives the exit node
// back by the journal first, then stops the core left behind; "was on" was not set yet.
func TestStartAfterHelperDiedBetweenSteps6And9(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	set, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt 777")
	if set < 0 || stop < set || old.alive() || h.store.hasJournal() {
		t.Fatalf("restore first, then the old core: %v", h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "stopping,off" {
		t.Fatalf("phases %s", got)
	}
	if h.store.saved().Core != nil {
		t.Fatal("the old core identity must be cleared")
	}
}

// The previous helper failed while on ("was on" set): restore, stop the old core, enable again.
func TestStartAfterCrashWhileOn(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.Start()
	h.waitPhase(protocol.PhaseOn)
	if got := strings.Join(h.phases(), ","); got != "stopping,starting,on" {
		t.Fatalf("phases %s", got)
	}
	// All three steps must have happened, in this order: the exit node given back (the first
	// sys:set, before any new core exists), the old core stopped, the new core started.
	set := h.rec.index("sys:set 100.64.0.1 true")
	stop := h.rec.index("core:interrupt 777")
	start := h.rec.index("core:start")
	if set < 0 || stop <= set || start <= stop || old.alive() {
		t.Fatalf("restore, then stop the old core, then start the new one (old alive %v): %v", old.alive(), h.rec.list())
	}
}

// Final review of plan 1A: the start-up of 5.7 with "was on" set gives the exit node back and stops
// the old core in phase stopping, then enables again. A routing.disable in that stopping half did
// nothing, and the enable after it switched routing on against the user's disable. Now it clears
// "was on": after the restore and the stop of the old core the start-up ends in off.
func TestDisableDuringStartupCancelsTheReenable(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on = false })
	release := holdStatus(t, h)
	h.e.Start()
	h.waitFor("phase stopping", func() bool { return h.e.State().Phase == protocol.PhaseStopping })
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	release()
	h.e.Wait()
	if got := strings.Join(h.phases(), ","); got != "stopping,off" || h.rec.count("core:start") != 0 {
		t.Fatalf("the disable must cancel the enable of the start-up: phases %s, events %v", got, h.rec.list())
	}
	set, stop := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt 777")
	if set < 0 || stop < set || old.alive() || h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatalf("restore, then the old core stopped; journal gone, was-on cleared: %v", h.rec.list())
	}
}

// In stopping with restore_pending a plain routing.disable is «Повторить» (5.1, 5.4): one more
// restore attempt now and nothing else. The start-up with "was on" set keeps its enable: once the
// restore confirms, the old core is stopped and routing comes back on.
func TestRetryInRestorePendingKeepsTheReenable(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on, s.ignoreSet = false, true })
	h.e.Start()
	h.waitFor("restore_pending", func() bool { return h.sawReason(protocol.ReasonRestorePending) })
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	if !h.store.hasJournal() || !h.store.saved().WasOn || !old.alive() {
		t.Fatalf("a retry keeps the journal, \"was on\" and the old core: was-on %v, events %v", h.store.saved().WasOn, h.rec.list())
	}
	h.sys.set(func(s *fakeSys) { s.ignoreSet = false })
	h.e.Wait()
	if got := strings.Join(h.phases(), ","); got != "stopping,starting,on" || h.rec.count("core:start") != 1 {
		t.Fatalf("the retry must keep the enable of the start-up: phases %s, events %v", got, h.rec.list())
	}
	if old.alive() || !h.store.saved().WasOn {
		t.Fatalf("the old core stopped, \"was on\" set again: %v", h.rec.list())
	}
}

// A core left without a journal is stopped in phase stopping; Tailscale is not called.
func TestLeftoverCoreWithoutJournal(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if old.alive() || h.rec.index("sys:") >= 0 {
		t.Fatalf("events %v", h.rec.list())
	}
}

// A sing-box.exe that is not ours (other pid or creation time) is never touched.
func TestForeignCoreIsNotTouched(t *testing.T) {
	h := newHarness(t)
	foreign := leftover(h)
	h.store.state.Core = &store.CoreIdentity{PID: 777, Created: 1, Path: `C:\core\sing-box.exe`}
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if !foreign.alive() || h.rec.index("core:") >= 0 {
		t.Fatalf("a foreign core was touched: %v", h.rec.list())
	}
}

// At start, too, the old core keeps running until the restore is confirmed.
func TestStartWithUnconfirmedRestore(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on, s.ignoreSet = false, true })
	h.e.Start()
	h.waitFor("restore_pending", func() bool { return h.sawReason(protocol.ReasonRestorePending) })
	if !old.alive() {
		t.Fatal("the old core must keep running")
	}
	h.e.retryRestore(true)
	h.waitPhase(protocol.PhaseOff)
	if old.alive() || h.store.hasJournal() {
		t.Fatal("after skip: journal deleted, core stopped")
	}
}

// 5.4, end of the session: was-on cleared, one restore command, the core ended without a wait,
// the journal kept for the next start, no restart after the core's exit.
func TestSessionEndWhileOn(t *testing.T) {
	h := enabled(t)
	h.e.SessionEnding()
	if h.store.saved().WasOn {
		t.Fatal("was-on must be cleared")
	}
	if h.rec.count("sys:set") != 1 || h.rec.index("core:kill") < h.rec.index("sys:set") || h.rec.index("core:interrupt") >= 0 {
		t.Fatalf("one restore command, then the forced end: %v", h.rec.list())
	}
	if !h.store.hasJournal() {
		t.Fatal("the journal stays until a confirmed restore")
	}
	h.e.Wait()
	if n := h.rec.count("core:start"); n != 1 {
		t.Fatalf("no restart at the end of the session, starts %d", n)
	}
}

// 5.7 at the end of the session in blocked: the exit node the user selected again is not
// overwritten by the journal; the journal stays and the next start deletes it (exit node on).
func TestSessionEndInBlockedKeepsTheUsersExitNode(t *testing.T) {
	h := enabled(t)
	h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
	h.e.mu.Lock()
	h.e.sysExitOn = true
	h.e.setPhase(protocol.PhaseBlocked, nil)
	h.e.mu.Unlock()
	h.e.SessionEnding()
	if h.rec.count("sys:set") != 0 || h.rec.index("core:kill") < 0 {
		t.Fatalf("no restore command over the user's choice, the core ended: %v", h.rec.list())
	}
	if !h.store.hasJournal() || h.store.saved().WasOn {
		t.Fatal("the journal stays for the next start, was-on is cleared")
	}
	h.e.Wait()
}

// idleState tells whether ExitIfIdle has returned and whether helper.shutdown's path was taken.
func idleState(h *harness, decided chan struct{}) (returned, exiting bool) {
	select {
	case <-decided:
		returned = true
	default:
	}
	h.e.mu.Lock()
	defer h.e.mu.Unlock()
	return returned, h.e.exiting
}

// Spec 4.3, section 10: started by the task at logon after a restart with the journal left, the helper
// gives the exit node back (5.7); routing stays off and no window comes, so 60 s of the engine's clock
// after the start-up it ends by the path of helper.shutdown. The 60 s begin only when the start-up is
// over: a restore that waits for tailscale.exe holds them.
func TestIdleExitWithoutWindow(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on = false })
	release := holdStatus(t, h)
	h.e.Start()
	decided := make(chan struct{})
	go func() {
		h.e.ExitIfIdle(time.Minute)
		close(decided)
	}()
	h.waitFor("phase stopping", func() bool { return h.e.State().Phase == protocol.PhaseStopping })
	time.Sleep(20 * time.Millisecond)
	if returned, exiting := idleState(h, decided); returned || exiting {
		t.Fatalf("the wait begins after the start-up: returned %v, exiting %v", returned, exiting)
	}
	began := h.clock.Now()
	release()
	waitClosed(t, decided, "the idle decision")
	h.waitFor("exit", func() bool { return exitedNow(h) })
	if waited := h.clock.Now().Sub(began); waited < time.Minute {
		t.Fatalf("the helper ended %s of the engine's clock after the start-up, want 1m0s", waited)
	}
	if got := strings.Join(h.phases(), ","); got != "stopping,off" || h.rec.index("sys:set 100.64.0.1 true") < 0 || h.store.hasJournal() {
		t.Fatalf("the restore by the journal comes first: phases %s, events %v", got, h.rec.list())
	}
}

// "Was on" made the start-up enable again (5.7): routing is on without a window, and the helper stays.
func TestNoIdleExitWhenWasOn(t *testing.T) {
	h := newHarness(t)
	leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.Start()
	h.e.ExitIfIdle(time.Minute)
	time.Sleep(20 * time.Millisecond)
	if _, exiting := idleState(h, nil); exiting || exitedNow(h) || h.e.State().Phase != protocol.PhaseOn || !h.store.saved().WasOn {
		t.Fatalf("routing stays on: phase %s, exiting %v, was-on %v", h.e.State().Phase, exiting, h.store.saved().WasOn)
	}
}

// A window that connected once keeps the helper, even when it went again before the 60 s were over
// (spec 4.3: only a helper that no window reached ends); here it came during the start-up.
func TestNoIdleExitWhenAWindowCame(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on = false })
	release := holdStatus(t, h)
	h.e.Start()
	decided := make(chan struct{})
	go func() {
		h.e.ExitIfIdle(time.Minute)
		close(decided)
	}()
	h.waitFor("phase stopping", func() bool { return h.e.State().Phase == protocol.PhaseStopping })
	h.e.SetClients(1)
	h.e.SetClients(0)
	release()
	waitClosed(t, decided, "the idle decision")
	time.Sleep(20 * time.Millisecond)
	if _, exiting := idleState(h, decided); exiting || exitedNow(h) || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("the helper stays after a window: phase %s, exiting %v", h.e.State().Phase, exiting)
	}
}

func TestSessionEndWhileOff(t *testing.T) {
	h := newHarness(t)
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	h.e.SessionEnding()
	if len(h.rec.list()) != 0 {
		t.Fatalf("nothing to do: %v", h.rec.list())
	}
}
