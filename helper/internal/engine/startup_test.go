package engine

import (
	"strings"
	"testing"

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
	leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.Start()
	h.waitPhase(protocol.PhaseOn)
	if got := strings.Join(h.phases(), ","); got != "stopping,starting,on" {
		t.Fatalf("phases %s", got)
	}
	if h.rec.index("core:interrupt 777") > h.rec.index("core:start") {
		t.Fatal("the old core must be stopped before the new one starts")
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

func TestSessionEndWhileOff(t *testing.T) {
	h := newHarness(t)
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	h.e.SessionEnding()
	if len(h.rec.list()) != 0 {
		t.Fatalf("nothing to do: %v", h.rec.list())
	}
}
