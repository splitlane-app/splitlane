package engine

import (
	"context"
	"testing"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

func rounds(h *harness, n int) {
	for i := 0; i < n; i++ {
		h.e.checkHealth(context.Background())
	}
}

// Section 10 row 5: the VPN server is unreachable: degraded after three failures while probe-direct
// works; DNS through the provider (mode vpn-localdns) and the Windows cache flushed; the core stays.
func TestDegradedAndBack(t *testing.T) {
	h := newHarness(t)
	p := h.running()
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{noAnswer})
	rounds(h, 2)
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatal("two failures are not enough")
	}
	rounds(h, 1)
	s := h.e.State()
	if s.Phase != protocol.PhaseDegraded || s.Reason.Code != protocol.ReasonVPNUnreachable {
		t.Fatalf("state %+v", s)
	}
	if h.clash.currentMode() != "vpn-localdns" || h.rec.index("dns:flush") < h.rec.index("clash:mode vpn-localdns") {
		t.Fatalf("mode %s, events %v", h.clash.currentMode(), h.rec.list())
	}
	if !p.alive() {
		t.Fatal("in degraded the core is not stopped")
	}
	h.probe.script([]ProbeResult{atVPN}, nil, []ProbeResult{atVPN})
	rounds(h, 1)
	if h.e.State().Phase != protocol.PhaseOn || h.clash.currentMode() != "vpn" || h.rec.count("dns:flush") != 2 {
		t.Fatalf("back to on: %+v %v", h.e.State(), h.rec.list())
	}
}

// The core mode follows the state, not the transition: a refused clash mode is tried again on every
// round until the core matches. Otherwise degraded would keep the VPN DNS (the direct lane loses
// names), and a refused way back would publish "on" while the core stays on the provider's DNS.
func TestModeFollowsStateAfterRefusedSwitch(t *testing.T) {
	h := newHarness(t)
	h.running()
	refuse := func(mode string, on bool) {
		h.clash.mu.Lock()
		defer h.clash.mu.Unlock()
		h.clash.refuse[mode] = on
	}

	// Into degraded while the core refuses vpn-localdns: the phase is published, the mode is not set.
	refuse("vpn-localdns", true)
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{noAnswer})
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseDegraded || h.clash.currentMode() != "vpn" {
		t.Fatalf("phase %s, mode %s, events %v", h.e.State().Phase, h.clash.currentMode(), h.rec.list())
	}
	if h.rec.count("dns:flush") != 0 {
		t.Fatalf("the cache is flushed only when the mode changed: %v", h.rec.list())
	}
	rounds(h, 2) // still refused: tried again, still not taken
	if h.clash.currentMode() != "vpn" || h.rec.count("clash:mode vpn-localdns") != 3 {
		t.Fatalf("a refused mode is tried on every round: mode %s, events %v", h.clash.currentMode(), h.rec.list())
	}

	// The refusal is lifted: the next round sets the mode and flushes the cache once.
	refuse("vpn-localdns", false)
	rounds(h, 1)
	if h.clash.currentMode() != "vpn-localdns" || h.rec.count("dns:flush") != 1 {
		t.Fatalf("mode %s, events %v", h.clash.currentMode(), h.rec.list())
	}
	rounds(h, 3) // the core matches: no more switching, no more flushing
	if h.rec.count("clash:mode") != 4 || h.rec.count("dns:flush") != 1 {
		t.Fatalf("a matching core is left alone: %v", h.rec.list())
	}

	// The way back: probe-vpn answers, the core refuses vpn. The phase is on, the mode is retried.
	refuse("vpn", true)
	h.probe.script([]ProbeResult{atVPN}, nil, []ProbeResult{atVPN})
	rounds(h, 2)
	if h.e.State().Phase != protocol.PhaseOn || h.clash.currentMode() != "vpn-localdns" || h.rec.count("clash:mode") != 6 {
		t.Fatalf("phase %s, mode %s, events %v", h.e.State().Phase, h.clash.currentMode(), h.rec.list())
	}
	refuse("vpn", false)
	rounds(h, 1)
	if h.clash.currentMode() != "vpn" || h.rec.count("dns:flush") != 2 {
		t.Fatalf("mode %s, events %v", h.clash.currentMode(), h.rec.list())
	}
	rounds(h, 3)
	if h.rec.count("clash:mode") != 7 || h.rec.count("dns:flush") != 2 {
		t.Fatalf("a matching core is left alone: %v", h.rec.list())
	}
}

// No network at all (both probes silent) is not a VPN failure: nothing changes.
func TestNoNetworkChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{noAnswer}, []ProbeResult{noAnswer})
	rounds(h, 5)
	h.e.Wait()
	if h.e.State().Phase != protocol.PhaseOn || h.rec.index("clash:") >= 0 || h.rec.count("core:start") != 1 {
		t.Fatalf("state %+v events %v", h.e.State(), h.rec.list())
	}
}

// An HTTP error from the address service is not a failure of the link (5.5).
func TestHTTPErrorIsNotAFailure(t *testing.T) {
	h := newHarness(t)
	h.running()
	served := ProbeResult{Reached: true}
	h.probe.script([]ProbeResult{served}, []ProbeResult{atDirect}, []ProbeResult{served})
	rounds(h, 4)
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("phase %s", h.e.State().Phase)
	}
}

// Section 10 row 6: two answers in a row from the direct address through the adapter: rollback, error.
func TestLeakWhileOn(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.probe.script(nil, nil, []ProbeResult{atDirect, atVPN, atDirect, atDirect})
	rounds(h, 3)
	h.e.Wait()
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatal("answers from the direct address that are not in a row are no leak")
	}
	rounds(h, 1)
	wantError(t, h, protocol.ReasonLeakDetected)
	if h.rec.index("sys:set") > h.rec.index("core:interrupt") {
		t.Fatalf("restore before stop: %v", h.rec.list())
	}
}

// Section 10 row 6 after the provider gave a new address (DHCP, CGNAT, another network): probe-direct
// runs every round, so a leak that answers from the new direct address is still caught.
func TestLeakAfterDirectAddressChange(t *testing.T) {
	h := newHarness(t)
	h.running()
	moved := ProbeResult{Reached: true, IP: "198.51.100.77"}
	h.probe.script(nil, []ProbeResult{moved}, []ProbeResult{moved, moved})
	rounds(h, 2)
	wantError(t, h, protocol.ReasonLeakDetected)
}

// 5.5: addresses follow every successful check, the direct one too while the VPN lane works.
// A probe-direct answer equal to probe-vpn's is not the direct lane: the stored one stays.
func TestAddressesFollowTheChecks(t *testing.T) {
	h := newHarness(t)
	h.running()
	vpn, direct := ProbeResult{Reached: true, IP: "203.0.113.77"}, ProbeResult{Reached: true, IP: "198.51.100.77"}
	h.probe.script([]ProbeResult{vpn}, []ProbeResult{direct}, []ProbeResult{vpn})
	rounds(h, 1)
	a := h.e.State().Addresses
	if a.VPN == nil || *a.VPN != vpn.IP || a.Direct == nil || *a.Direct != direct.IP {
		t.Fatalf("addresses %+v", a)
	}
	h.probe.script(nil, []ProbeResult{vpn}, nil)
	rounds(h, 1)
	if a := h.e.State().Addresses; a.Direct == nil || *a.Direct != direct.IP || h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("direct address taken from a VPN answer: %+v", a)
	}
}

// Section 10 row 7: the adapter path fails three times while probe-vpn works: 5.4 then 5.3.
func TestBrokenCoreIsRestarted(t *testing.T) {
	h := newHarness(t)
	old := h.running()
	h.probe.script(nil, nil, []ProbeResult{noAnswer, noAnswer, noAnswer, atVPN})
	rounds(h, 3)
	h.waitFor("restarted core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if old.alive() || h.rec.index("sys:set") > h.rec.index("core:interrupt") {
		t.Fatalf("old core stopped after the restore: %v", h.rec.list())
	}
}

// Failures of the adapter path while probe-vpn fails too are not counted.
func TestAdapterFailuresNeedAWorkingVPN(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{noAnswer}, []ProbeResult{noAnswer})
	rounds(h, 4)
	h.e.Wait()
	if h.rec.count("core:start") != 1 {
		t.Fatalf("no restart expected: %v", h.rec.list())
	}
}

// Section 10 row 8 and 5.6: the user selects an exit node again: blocked; cleared: back.
// With both blocked and degraded the phase is blocked. In blocked the adapter path is not checked.
func TestBlockedPhase(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.sys.set(func(s *fakeSys) { s.on, s.ip = true, "100.64.0.99" })
	h.e.pollSystem(context.Background())
	if h.e.State().Phase != protocol.PhaseBlocked {
		t.Fatalf("phase %s", h.e.State().Phase)
	}
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{atDirect})
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseBlocked {
		t.Fatalf("blocked wins over degraded, and no leak check in blocked: %s", h.e.State().Phase)
	}
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.e.pollSystem(context.Background())
	if h.e.State().Phase != protocol.PhaseDegraded {
		t.Fatalf("after the user cleared it: %s", h.e.State().Phase)
	}
}

// systemTailscale.clearExitNode: the exit node the user selected now goes into the journal.
func TestClearSystemExitNodeJournalsUsersChoice(t *testing.T) {
	h := newHarness(t)
	h.running()
	h.sys.set(func(s *fakeSys) { s.on, s.ip, s.allowLAN = true, "100.64.0.99", false })
	h.e.pollSystem(context.Background())
	h.e.queue.Lock()
	h.e.clearSystemExitNode(context.Background())
	h.e.queue.Unlock()
	j, _ := h.store.LoadJournal()
	if j == nil || *j != (store.Journal{ExitNode: "100.64.0.99", AllowLAN: false}) {
		t.Fatalf("journal %+v", j)
	}
	if h.rec.index("journal:save 100.64.0.99 false") > h.rec.index("sys:clear") || h.rec.index("dns:flush") < 0 {
		t.Fatalf("events %v", h.rec.list())
	}
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("phase %s", h.e.State().Phase)
	}
}

// Section 10, "the window crashed or was closed": the last window going away changes nothing in
// the routing; the core keeps running and the checks of 5.5 and 5.6 go on.
func TestLastWindowGoneKeepsRouting(t *testing.T) {
	h := enabled(t)
	core := h.core.last()
	mark := len(h.rec.list())
	h.e.SetClients(1)
	h.e.SetClients(0)
	h.e.pollSystem(context.Background())
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseOn || !core.alive() || countSince(h, mark, "core:") != 0 || countSince(h, mark, "sys:") != 0 {
		t.Fatalf("phase %s, events %v", h.e.State().Phase, h.rec.list())
	}
}

// While routing is off the system client is read only for a connected window (5.6).
func TestPollWhileOffNeedsAWindow(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.e.pollSystem(context.Background())
	if h.e.State().SystemTailscale.Installed {
		t.Fatal("no window: no poll")
	}
	h.e.SetClients(1)
	h.e.pollSystem(context.Background())
	s := h.e.State().SystemTailscale
	if !s.Installed || !s.ExitNodeOn || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("system %+v", s)
	}
}
