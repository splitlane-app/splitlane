package engine

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
)

// pendingState is the first published state with reason restore_pending, or nil.
func pendingState(h *harness) *protocol.State {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.states {
		if s.Reason != nil && s.Reason.Code == protocol.ReasonRestorePending {
			return &s
		}
	}
	return nil
}

// 5.7 and the row "the helper failed" of section 10: a restore.json that is there but cannot be read
// (a sharing violation, access denied) is not "no journal". The start-up stays in stopping with
// restore_pending (the read error, no command: the exit node is unknown until the journal is read)
// and reads it again every 5 s; once it reads, step 1 of 5.4 gives the exit node back. Unreadable
// content, which the store sets aside, is no journal, as before.
func TestJournalReadErrorIsNotNoJournal(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.journalReadErrs = []error{readErr("restore.json"), readErr("restore.json")}
	h.sys.set(func(s *fakeSys) { s.on = false })
	began := h.clock.Now()
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if on, ip := systemExitNode(h); !on || ip != exitNode || h.store.hasJournal() {
		t.Fatalf("the exit node must come back from the journal read at last (on %v %s): %v", on, ip, h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "stopping,off" {
		t.Fatalf("phases %s", got)
	}
	p := pendingState(h)
	if p == nil || p.Phase != protocol.PhaseStopping || p.Reason.Params[protocol.ParamCommand] != "" ||
		!strings.Contains(p.Reason.Params[protocol.ParamError], "being used by another process") ||
		!p.SystemTailscale.RestoreOnDisable {
		t.Fatalf("want stopping with restore_pending, the read error and no command, got %+v", p)
	}
	if _, _, reads := h.store.calls(); reads != 3 || h.clock.Now().Sub(began) < 2*restoreEvery {
		t.Fatalf("read again every %s: %d reads in %s", restoreEvery, reads, h.clock.Now().Sub(began))
	}

	h = newHarness(t)
	h.store.journalReadErrs = []error{errors.New("restore.json is unreadable (invalid character 'b') and was set aside as restore.json.bad")}
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if got := strings.Join(h.phases(), ","); got != "off" || len(h.rec.list()) != 0 {
		t.Fatalf("a journal set aside is no journal: phases %s, events %v", got, h.rec.list())
	}
}

// "Disable without restore" (5.4) ends the wait for a journal that cannot be read, too: the journal is
// deleted as far as it can be, the system client is not called, the start-up goes on without it.
func TestUnreadJournalCanBeSkipped(t *testing.T) {
	h := newHarness(t)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	for i := 0; i < 10000; i++ {
		h.store.journalReadErrs = append(h.store.journalReadErrs, readErr("restore.json"))
	}
	h.e.Start()
	h.waitFor("restore_pending", func() bool { return pendingState(h) != nil })
	if err := call(h, protocol.MethodDisable, protocol.DisableParams{SkipRestore: true}); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
	if h.rec.count("journal:delete") != 1 || h.rec.index("sys:") >= 0 || h.e.State().SystemTailscale.RestoreOnDisable {
		t.Fatalf("skip: journal deleted, the system client left alone: %v", h.rec.list())
	}
}

// A state.json that is there but cannot be read is never written over. The start reads it three more
// times 200 ms apart, then goes on with the defaults in memory; every save reads it again first and
// writes nothing while that fails (lanes.set: apply_failed). Once it reads, its settings come back
// ("was on" and the core identity stay the current run's), the change that met it gives way
// (apply_failed), and from then on saving works as before.
func TestStateReadErrorIsNeverOverwritten(t *testing.T) {
	h := newHarness(t)
	h.store.state.Apps = []protocol.AppRule{fileRule(idA, curlA, protocol.LaneDirect)}
	h.store.stateReadFails = 1000
	began := h.clock.Now()
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	reads, saves, _ := h.store.calls()
	if saves != 0 || reads < 5 || h.clock.Now().Sub(began) < 600*time.Millisecond {
		t.Fatalf("want 4 reads 200 ms apart at start and a read instead of a write: %d reads, %d saves in %s",
			reads, saves, h.clock.Now().Sub(began))
	}
	if s := h.e.State(); len(s.Apps) != 0 || s.Tailscale.SelectedExitNode != nil {
		t.Fatalf("the defaults must be in memory: %+v", s)
	}
	lanes := protocol.LanesSetParams{DefaultLane: protocol.LaneDirect, Apps: []protocol.AppRule{fileRule(idB, curlB, protocol.LaneDirect)}}
	wantCode(t, call(h, protocol.MethodLanesSet, lanes), protocol.CodeApplyFailed)
	if _, saves, _ := h.store.calls(); saves != 0 || len(h.e.State().Apps) != 0 {
		t.Fatalf("still unreadable: nothing written, nothing kept: %d saves, %+v", saves, h.e.State().Apps)
	}

	h.store.mu.Lock()
	h.store.stateReadFails = 0
	h.store.mu.Unlock()
	wantCode(t, call(h, protocol.MethodLanesSet, lanes), protocol.CodeApplyFailed)
	s := h.e.State()
	if len(s.Apps) != 1 || s.Apps[0].ID != idA || s.Tailscale.SelectedExitNode == nil || *s.Tailscale.SelectedExitNode != exitNode {
		t.Fatalf("the settings of state.json must come back: %+v", s)
	}
	if got := h.store.saved(); len(got.Apps) != 1 || got.Apps[0].ID != idA || !got.LoggedIn || got.WasOn {
		t.Fatalf("state.json must keep its settings: %+v", got)
	}
	if err := call(h, protocol.MethodLanesSet, lanes); err != nil {
		t.Fatal(err)
	}
	if got := h.store.saved().Apps; len(got) != 1 || got[0].ID != idB {
		t.Fatalf("saving works again: %+v", got)
	}
}

// Review of task 1: a state.json that the start cannot read names the core left from the previous run
// and holds "was on". The start-up reads it again before it looks for that core (5.7): once it reads,
// the old core is stopped after the restore and routing comes back on. Before, the defaults hid the
// core: it ran on without an owner, and the first save that could read the file wrote its identity
// away. While the file cannot be read at all, the save that reads it at last keeps the identity it
// holds as long as this run has started no core of its own: the next start still finds that core.
// A routing.disable or a helper.shutdown that comes while the start-up reads state.json again (phase
// stopping, "was on" not known yet) is kept: the file that then reads with "was on" set does not make the
// start-up enable again (review of plan 1B, final, minor 2, and the deferred minor of the routing.disable).
// The read is held while the command comes; for helper.shutdown its own save meets the file still
// unreadable.
func TestOffDuringStateReadIsKept(t *testing.T) {
	for _, method := range []string{protocol.MethodDisable, protocol.MethodShutdown} {
		t.Run(method, func(t *testing.T) {
			h := newHarness(t)
			h.store.state.WasOn = true
			h.store.stateReadFails = 2 + stateRetries // every read of load and the first one of the start-up fail
			entered, release := make(chan struct{}), make(chan struct{})
			h.store.onLoad = func(n int) {
				if n == 3+stateRetries { // the second read of the start-up; the command's own read goes on
					close(entered)
					<-release
				}
			}
			h.e.Start()
			waitClosed(t, entered, "the second read of the start-up")
			h.store.mu.Lock()
			h.store.stateReadFails = 1 // the save of the command still finds the file unreadable
			h.store.mu.Unlock()
			if err := call(h, method, nil); err != nil {
				t.Fatal(err)
			}
			close(release)
			h.e.Wait()
			if h.rec.index("core:start") >= 0 || strings.Contains(strings.Join(h.phases(), ","), "starting") {
				t.Fatalf("%s during the read: no enable again; phases %v, events %v", method, h.phases(), h.rec.list())
			}
			if h.store.saved().WasOn {
				t.Fatalf("%s during the read: \"was on\" must end cleared in state.json", method)
			}
		})
	}
}

func TestStartWithUnreadStateStopsTheLeftCore(t *testing.T) {
	h := newHarness(t)
	old := leftover(h)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.store.state.WasOn = true
	h.store.stateReadFails = 1 + stateRetries // every read of load fails, the next one reads
	h.sys.set(func(s *fakeSys) { s.on = false })
	erased := false
	h.store.onSave = func(s store.State) {
		if old.alive() && (s.Core == nil || *s.Core != old.id) {
			erased = true
		}
	}
	h.e.Start()
	h.e.Wait()
	if got := strings.Join(h.phases(), ","); got != "stopping,starting,on" || old.alive() {
		t.Fatalf("the old core must be stopped and routing come back on: phases %s, old alive %v, events %v", got, old.alive(), h.rec.list())
	}
	set, stop, start := h.rec.index("sys:set 100.64.0.1 true"), h.rec.index("core:interrupt 777"), h.rec.index("core:start")
	if set < 0 || stop < set || start < stop {
		t.Fatalf("restore, then the old core stopped, then the new one started: %v", h.rec.list())
	}
	h.store.mu.Lock()
	gone := erased
	h.store.mu.Unlock()
	if gone {
		t.Fatal("the identity of the old core was written away while it ran")
	}

	h = newHarness(t)
	old = leftover(h)
	h.store.stateReadFails = 1000
	h.e.Start()
	h.waitPhase(protocol.PhaseOff)
	if !old.alive() {
		t.Fatal("setup: while state.json cannot be read no core of the previous run is known")
	}
	h.store.mu.Lock()
	h.store.stateReadFails = 0
	h.store.mu.Unlock()
	lanes := protocol.LanesSetParams{DefaultLane: protocol.LaneDirect, Apps: []protocol.AppRule{fileRule(idB, curlB, protocol.LaneDirect)}}
	wantCode(t, call(h, protocol.MethodLanesSet, lanes), protocol.CodeApplyFailed)
	if c := h.store.saved().Core; c == nil || *c != old.id {
		t.Fatalf("the identity of the old core must stay in state.json while it runs: %+v", c)
	}
}

// 8.3 step 3: lanes.set answers after the layout is in state.json. A write that fails is apply_failed;
// as on the other apply_failed paths the previous rule files and mode come back, and the layout in
// memory stays the previous one. What a failed save did not write goes with the next save.
func TestLanesSetSaveFailureIsApplyFailed(t *testing.T) {
	h := enabled(t)
	oldDirect, oldVPN := string(h.store.direct), string(h.store.vpn)
	h.store.mu.Lock()
	h.store.saveErr = errors.New("There is not enough space on the disk.")
	h.store.mu.Unlock()
	mark := len(h.rec.list())
	lanes := protocol.LanesSetParams{DefaultLane: protocol.LaneDirect, Apps: []protocol.AppRule{fileRule(idA, curlA, protocol.LaneDirect)}}
	err := call(h, protocol.MethodLanesSet, lanes)
	wantCode(t, err, protocol.CodeApplyFailed)
	if !strings.Contains(err.Message, "not enough space") {
		t.Fatalf("the message must give the write error: %q", err.Message)
	}
	if string(h.store.direct) != oldDirect || string(h.store.vpn) != oldVPN || h.clash.currentMode() != "vpn" ||
		!inOrder(h, mark, "rules:write", "clash:mode direct", "rules:write", "clash:mode vpn") {
		t.Fatalf("the previous rule files and mode must come back: mode %s, events %v", h.clash.currentMode(), h.rec.list()[mark:])
	}
	h.e.mu.Lock()
	runDirect, runMode := string(h.e.run.direct), h.e.run.mode
	h.e.mu.Unlock()
	if runDirect != oldDirect || runMode != "vpn" {
		t.Fatalf("the running core must be known to read the previous files in mode vpn, got mode %s", runMode)
	}
	if s := h.e.State(); s.DefaultLane != protocol.LaneVPN || len(s.Apps) != 0 {
		t.Fatalf("the layout must stay the previous one: %+v", s)
	}

	call(h, protocol.MethodDisable, nil) // "was on" goes, but the disk is full
	h.waitPhase(protocol.PhaseOff)
	if !h.store.saved().WasOn {
		t.Fatal("setup: the failed saves must have left state.json as it was")
	}
	h.store.mu.Lock()
	h.store.saveErr = nil
	h.store.mu.Unlock()
	if err := call(h, protocol.MethodLanesSet, lanes); err != nil {
		t.Fatal(err)
	}
	if got := h.store.saved(); got.WasOn || got.DefaultLane != protocol.LaneDirect || len(got.Apps) != 1 {
		t.Fatalf("the next save writes the whole state: %+v", got)
	}
}

// unreadState is a helper whose start could not read state.json: the defaults are in memory, and the
// file holds curl-a on the direct lane with the main lane direct, the exit node exitNode, "logged in".
// This run has logged in through the setup mode and selected the exit node selected, and could save
// neither: the file still cannot be read.
func unreadState(t *testing.T, selected string) *harness {
	t.Helper()
	h := newHarness(t)
	h.store.state.Apps = checkedApps(t, fileRule(idA, curlA, protocol.LaneDirect))
	h.store.state.DefaultLane = protocol.LaneDirect
	h.store.stateReadFails = 1000
	h.load()
	h.e.mu.Lock()
	h.e.saved.LoggedIn, h.e.saved.SelectedExitNode = true, selected
	h.e.mu.Unlock()
	return h
}

// strOf is the text of a nullable string of the state, "" for null.
func strOf(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// readable: state.json can be read from now on.
func readable(h *harness) {
	h.store.mu.Lock()
	h.store.stateReadFails = 0
	h.store.mu.Unlock()
}

// enableNow enables (5.3) and waits for phase on and the look at the path that follows it.
func enableNow(h *harness) {
	h.t.Helper()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
}

// coreHasLayout: the rule files, the mode of the core and what the engine knows of its run are those
// of the layout that the state s shows.
func coreHasLayout(t *testing.T, h *harness, s protocol.State) {
	t.Helper()
	direct, vpn, err := rules.Files(s.Apps, testRules)
	if err != nil {
		t.Fatal(err)
	}
	mode := coreconfig.ModeFor(s.DefaultLane, s.AllDirect, false)
	h.store.mu.Lock()
	gotDirect, gotVPN := string(h.store.direct), string(h.store.vpn)
	h.store.mu.Unlock()
	h.e.mu.Lock()
	runDirect, runMode := string(h.e.run.direct), h.e.run.mode
	h.e.mu.Unlock()
	if gotDirect != string(direct) || gotVPN != string(vpn) || runDirect != string(direct) || h.clash.currentMode() != mode || runMode != mode {
		t.Fatalf("the core must run the layout the state shows (%s, %d rules): mode %s (run %s), want %s; direct file:\n%s",
			s.DefaultLane, len(s.Apps), h.clash.currentMode(), runMode, mode, gotDirect)
	}
}

// Review of task 1: lanes.set while the core runs meets state.json readable only now (errStateLate):
// the layout of the file comes back and is published, and the core is given that layout (8.3). Before,
// the core went back to the layout from before the call, which neither the state nor the file showed.
func TestLateStateReadDuringLanesSetAppliesTheFileLayout(t *testing.T) {
	h := unreadState(t, exitNode)
	enableNow(h)
	if s := h.e.State(); len(s.Apps) != 0 || s.DefaultLane != protocol.LaneVPN {
		t.Fatalf("setup: the defaults: %+v", s)
	}
	readable(h)
	lanes := protocol.LanesSetParams{DefaultLane: protocol.LaneVPN, Apps: []protocol.AppRule{fileRule(idB, curlB, protocol.LaneDirect)}}
	wantCode(t, call(h, protocol.MethodLanesSet, lanes), protocol.CodeApplyFailed)
	s := h.e.State()
	if len(s.Apps) != 1 || s.Apps[0].ID != idA || s.DefaultLane != protocol.LaneDirect {
		t.Fatalf("the layout of state.json is published: %+v", s)
	}
	if got := h.store.saved(); len(got.Apps) != 1 || got.Apps[0].ID != idA || !got.WasOn {
		t.Fatalf("state.json keeps its layout: %+v", got)
	}
	coreHasLayout(t, h, s)
}

// Review of task 1: the other saves that meet state.json readable only now while the core runs keep
// what the core was built from and what this run knows of its node: the exit node selected in this run
// stays (the core runs with it) and an expired login stays cleared. The layout of the file comes back,
// and the next round of 5.5 gives it to the core (8.3). Before, the file's exit node and "logged in"
// replaced them, and the core kept the layout of the defaults under the file's in the state.
func TestLateStateReadWhileOnKeepsTheRunsChoices(t *testing.T) {
	const selected = "100.64.0.2"
	h := unreadState(t, selected)
	readable(h) // the first save of the enable, the identity of its core, reads the file
	enableNow(h)
	s := h.e.State()
	if s.Tailscale.SelectedExitNode == nil || *s.Tailscale.SelectedExitNode != selected ||
		h.core.options().ExitNode != selected || h.store.saved().SelectedExitNode != selected {
		t.Fatalf("the exit node the core runs with stays, in the state and in state.json: %q, saved %q",
			strOf(s.Tailscale.SelectedExitNode), h.store.saved().SelectedExitNode)
	}
	if len(s.Apps) != 1 || s.Apps[0].ID != idA || s.DefaultLane != protocol.LaneDirect {
		t.Fatalf("the layout of state.json is published: %+v", s)
	}
	rounds(h, 1)
	coreHasLayout(t, h, h.e.State())

	h = unreadState(t, exitNode)
	enableNow(h)
	readable(h)
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.e.checkNode(context.Background())
	if s := h.e.State(); s.Phase != protocol.PhaseDegraded || h.store.saved().LoggedIn {
		t.Fatalf("the expired login stays cleared in state.json: phase %s, saved %+v", s.Phase, h.store.saved())
	}
}
