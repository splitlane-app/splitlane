package engine

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
)

// The log lines of the restarts after a network change (section 10).
const (
	logChecksFailed = "network changed and the checks did not pass in 60 s; restarting the core"
	logStayedRelay  = "the path to the exit node stayed relay after the network change; restarting the core"
)

// netOn is a helper in phase on after 5.3 whose log lines are events "log:<line>", with the path to
// the exit node read after phase on: direct.
func netOn(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	logEvents(h)
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
	return h
}

// ticks runs n rounds of 5.5 ten seconds apart on the clock of the test, the first one 10 s from now.
func ticks(h *harness, n int) {
	for i := 0; i < n; i++ {
		h.clock.After(healthEvery)
		h.e.checkHealth(context.Background())
	}
}

// routeUp gives the computer its default route back, or takes it away until the next call.
func routeUp(h *harness, up bool) {
	h.host.mu.Lock()
	defer h.host.mu.Unlock()
	h.host.noRoute = 0
	if !up {
		h.host.noRoute = -1
	}
}

// following tells whether the helper follows a network change now.
func following(h *harness) bool {
	h.e.mu.Lock()
	defer h.e.mu.Unlock()
	return h.e.net != nil
}

// healthyCores makes every core that starts from now on pass all checks: probe-vpn, probe-direct and
// the path through the adapter.
func healthyCores(h *harness) {
	h.core.mu.Lock()
	defer h.core.mu.Unlock()
	h.core.onStart = func(*fakeProc) {
		h.probe.script([]ProbeResult{atVPN}, []ProbeResult{atDirect}, []ProbeResult{atVPN})
	}
}

// restartedAfter waits for the restart that a network change led to, logged as why: the rollback of
// 5.4 (the system exit node given back, the old core stopped), then the enable of 5.3 up to phase on.
// No other phase came between: neither degraded nor error.
func restartedAfter(t *testing.T, h *harness, old *fakeProc, mark int, why string) {
	t.Helper()
	h.waitFor("the restarted core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	order := []string{"log:" + why, "sys:set " + exitNode + " true", fmt.Sprintf("core:interrupt %d", old.id.PID),
		fmt.Sprintf("core:start %d", old.id.PID+1)}
	if !inOrder(h, mark, order...) || h.rec.count("log:"+why) != 1 || old.alive() {
		t.Fatalf("restore, stop, start after %q: %v", why, h.rec.list()[mark:])
	}
	if got := strings.Join(h.phases(), ","); got != "starting,on,starting,on" {
		t.Fatalf("phases %s", got)
	}
}

// Section 10, the row «Сон и пробуждение, смена сети»: after a network change (here the wake from
// sleep) the helper waits for the default route; while there is none it leaves the core alone, however
// long: the failures of the rounds of 5.5 lead neither to degraded with the provider's DNS (probe-vpn
// silent while probe-direct answers) nor to the restart of a core whose path through the adapter fails
// while probe-vpn works. The route back and a round that passes end the wait (the path to the exit
// node, direct before, is read once more: direct); then the rounds act again.
func TestNoRouteHoldsTheCore(t *testing.T) {
	h := netOn(t)
	core := h.core.last()
	mark := len(h.rec.list())
	routeUp(h, false)
	h.e.NetworkChanged("resume")
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{noAnswer})
	ticks(h, 9)
	h.probe.script([]ProbeResult{atVPN}, nil, nil)
	ticks(h, 9)
	if s := h.e.State(); s.Phase != protocol.PhaseOn || !core.alive() || countSince(h, mark, "core:") != 0 ||
		countSince(h, mark, "sys:") != 0 || countSince(h, mark, "clash:") != 0 || countSince(h, mark, "dns:") != 0 {
		t.Fatalf("no route: the core is left alone: phase %s, events %v", s.Phase, h.rec.list()[mark:])
	}
	routeUp(h, true)
	h.probe.script(nil, nil, []ProbeResult{atVPN})
	ticks(h, 2)
	if following(h) || h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("the route back and a round that passes end the wait: %v", h.rec.list()[mark:])
	}
	h.probe.script([]ProbeResult{noAnswer}, nil, []ProbeResult{noAnswer})
	ticks(h, 3)
	if s := h.e.State(); s.Phase != protocol.PhaseDegraded || s.Reason == nil || s.Reason.Code != protocol.ReasonVPNUnreachable {
		t.Fatalf("after the wait the rounds act again: %+v", s)
	}
}

// Section 10: once the network is back the helper waits up to 60 s for a round of 5.5 in which
// probe-vpn and the path through the adapter pass; meanwhile the failures lead to nothing (no degraded,
// no switch of the DNS). A round in which probe-direct does not answer either finds no network (5.5):
// the 60 s start again. At 60 s with the network there the core is restarted: the rollback of 5.4 (the
// system exit node given back and confirmed, the core stopped), then the enable of 5.3.
func TestChecksFailAfterNetworkChangeRestart(t *testing.T) {
	h := netOn(t)
	healthyCores(h)
	old := h.core.last()
	mark := len(h.rec.list())
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{noAnswer}, []ProbeResult{noAnswer})
	h.e.NetworkChanged("route")
	ticks(h, 9)
	if h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("a route but no answer through probe-direct either is no network: %v", h.rec.list()[mark:])
	}
	h.probe.script(nil, []ProbeResult{atDirect}, nil)
	ticks(h, 5)
	if s := h.e.State(); s.Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 || countSince(h, mark, "clash:") != 0 {
		t.Fatalf("within 60 s of the network nothing happens: phase %s, events %v", s.Phase, h.rec.list()[mark:])
	}
	ticks(h, 1)
	restartedAfter(t, h, old, mark, logChecksFailed)
}

// The network is there when probe-vpn answers, also while probe-direct does not: a core whose direct lane
// and path through the adapter fail after a network change, while probe-vpn works, is restarted at 60 s
// like any other core whose checks do not pass. Only a round in which neither probe answers finds no
// network and starts the 60 s again.
func TestDeadDirectLaneAfterChangeRestarts(t *testing.T) {
	h := netOn(t)
	healthyCores(h)
	old := h.core.last()
	mark := len(h.rec.list())
	h.probe.script([]ProbeResult{atVPN}, []ProbeResult{noAnswer}, []ProbeResult{noAnswer})
	h.e.NetworkChanged("route")
	ticks(h, 5)
	if h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("within 60 s of the network nothing happens: %v", h.rec.list()[mark:])
	}
	ticks(h, 1)
	restartedAfter(t, h, old, mark, logChecksFailed)
}

// Review Focus 2: the network goes again during the 60 s of the wait. While there is no default route
// nothing happens, however long (probe-direct answers here, so that only the missing route holds the
// core); the 60 s start again with the round that finds the route back.
func TestFlappingNetworkWaitsAgain(t *testing.T) {
	h := netOn(t)
	healthyCores(h)
	old := h.core.last()
	mark := len(h.rec.list())
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{noAnswer})
	h.e.NetworkChanged("route")
	ticks(h, 3)
	routeUp(h, false)
	ticks(h, 6)
	if h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("90 s after the route came back first, but no route now: %v", h.rec.list()[mark:])
	}
	routeUp(h, true)
	ticks(h, 6)
	if h.e.State().Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("50 s after the route came back again: %v", h.rec.list()[mark:])
	}
	ticks(h, 1)
	restartedAfter(t, h, old, mark, logChecksFailed)
}

// 5.5 "при degraded ядро не останавливается": a network change while the VPN server does not answer
// (sleep and wake, WSL, Docker or Hyper-V, route updates of tailscaled) restarts no core at 60 s: probe-vpn
// would stay silent through every attempt, each teardown would hand the system exit node back and the
// last one end in error. The sequence ends at 60 s and the rounds go on as without a change: the core
// stays in degraded, and probe-vpn back returns to on.
func TestDegradedBeforeTheChangeKeepsTheCore(t *testing.T) {
	h := netOn(t)
	core := h.core.last()
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, []ProbeResult{noAnswer})
	ticks(h, 3)
	if s := h.e.State(); s.Phase != protocol.PhaseDegraded {
		t.Fatalf("setup: degraded before the change: %+v", s)
	}
	mark := len(h.rec.list())
	h.e.NetworkChanged("route")
	ticks(h, 9)
	h.e.Wait()
	if s := h.e.State(); s.Phase != protocol.PhaseDegraded || !core.alive() || following(h) ||
		countSince(h, mark, "core:") != 0 || countSince(h, mark, "sys:") != 0 {
		t.Fatalf("degraded: the core stays: phase %s, events %v", s.Phase, h.rec.list()[mark:])
	}
	if countSince(h, mark, "log:network changed and the checks did not pass in 60 s; no restart in degraded") != 1 {
		t.Fatalf("the restart that did not come is logged once: %v", h.rec.list()[mark:])
	}
	h.probe.script([]ProbeResult{atVPN}, nil, []ProbeResult{atVPN})
	ticks(h, 1)
	if s := h.e.State(); s.Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 {
		t.Fatalf("probe-vpn back: on with the same core: %+v %v", s, h.rec.list()[mark:])
	}
}

// The decision of plan 1B (appendix A, checks of plan 1B, (и): after a cable change the embedded node
// stays on a relay, while a new core gets the direct path at once). Once the checks pass after a
// network change, a path to the exit node that was direct before it is read every 10 s: direct again
// within 60 s keeps the core, relay at 60 s restarts it (5.4, then 5.3). A second change during the
// wait starts it again and keeps the path from before the first one.
func TestRelayAfterNetworkChangeRestarts(t *testing.T) {
	h := netOn(t)
	old := h.core.last()
	mark := len(h.rec.list())
	path := func(p string) { h.core.node.set(func(n *fakeNode) { n.path = p }) }
	path(protocol.PathRelay)
	h.e.NetworkChanged("route")
	ticks(h, 4) // the checks pass, then relay for 30 s
	path(protocol.PathDirect)
	ticks(h, 1)
	if following(h) || h.rec.count("core:start") != 1 {
		t.Fatalf("direct again within 60 s: the core stays: %v", h.rec.list()[mark:])
	}

	h.e.NetworkChanged("route")
	path(protocol.PathRelay)
	h.e.refreshNode(context.Background()) // the look every 60 s reads relay meanwhile
	if p, _ := h.e.lastPath(); p != protocol.PathRelay {
		t.Fatalf("setup: lastPath %q", p)
	}
	h.e.NetworkChanged("route")
	ticks(h, 6) // the checks pass, then relay for 50 s
	if h.rec.count("core:start") != 1 {
		t.Fatalf("relay for 50 s: the core stays: %v", h.rec.list()[mark:])
	}
	ticks(h, 1)
	restartedAfter(t, h, old, mark, logStayedRelay)
}

// The restart for relay is only for a path that was direct before the network change: a path that was
// relay or unknown before it is not watched once the checks pass, and the core stays.
func TestRelayWithoutDirectBeforeKeepsTheCore(t *testing.T) {
	for _, before := range []string{protocol.PathRelay, ""} {
		t.Run("before="+before, func(t *testing.T) {
			h := netOn(t)
			h.core.node.set(func(n *fakeNode) { n.path = before })
			h.e.readPath(context.Background())
			h.core.node.set(func(n *fakeNode) { n.path = protocol.PathRelay })
			mark := len(h.rec.list())
			h.e.NetworkChanged("route")
			ticks(h, 1)
			if following(h) {
				t.Fatal("the checks passed and the path before was not direct: the network change is over")
			}
			ticks(h, 8)
			if s := h.e.State(); s.Phase != protocol.PhaseOn || h.rec.count("core:start") != 1 || countSince(h, mark, "sys:") != 0 {
				t.Fatalf("the core stays: phase %s, events %v", s.Phase, h.rec.list()[mark:])
			}
		})
	}
}

// While the helper waits for the default route, a leak (two answers in a row from the address of the
// direct lane through the adapter, 5.5) still rolls back at once: 5.4, error leak_detected.
func TestLeakStillRollsBackDuringTheWait(t *testing.T) {
	h := netOn(t)
	routeUp(h, false)
	h.e.NetworkChanged("route")
	h.probe.script(nil, nil, []ProbeResult{atDirect})
	ticks(h, 1)
	if h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("one answer from the direct address is no leak: %v", h.rec.list())
	}
	ticks(h, 1)
	wantError(t, h, protocol.ReasonLeakDetected)
	if on, ip := systemExitNode(h); !on || ip != exitNode || h.core.anyAlive() || following(h) {
		t.Fatalf("the system exit node back, no core: %v", h.rec.list())
	}
}

// A network change is followed only while the core runs in on, degraded or blocked and no operation
// starts or stops it; "suspend" is never followed (the wake after it is).
func TestNetworkChangeWhenOffDoesNothing(t *testing.T) {
	h := newHarness(t)
	h.load()
	states := publishedCount(h)
	for _, why := range []string{"route", "resume", "suspend"} {
		h.e.NetworkChanged(why)
	}
	if following(h) || publishedCount(h) != states || len(h.rec.list()) != 0 {
		t.Fatalf("off: nothing happens: %v", h.rec.list())
	}

	h, release := inPhase(t, protocol.PhaseStarting)
	h.e.NetworkChanged("route")
	if following(h) {
		t.Fatal("an enable under way is not followed")
	}
	release()

	h = netOn(t)
	h.e.NetworkChanged("suspend")
	done := holdOp(h) // the enable that published phase on is still in place
	h.e.NetworkChanged("route")
	done()
	if following(h) {
		t.Fatal("suspend, and a change during an operation that starts the core, are not followed")
	}
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, nil)
	rounds(h, 3)
	if h.e.State().Phase != protocol.PhaseDegraded {
		t.Fatalf("nothing holds the rounds: %+v", h.e.State())
	}
}
