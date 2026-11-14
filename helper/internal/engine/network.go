package engine

import (
	"time"

	"splitlane/helper/internal/protocol"
)

// netWait: after a network change the checks of 5.5 have so long to pass once the network is back, and
// then a path to the exit node that was direct before the change so long to be direct again (section 10).
const netWait = 60 * time.Second

// netWatch is the sequence after a network change (section 10), driven by the rounds of 5.5 (netRound);
// guarded by Engine.mu. Until the checks pass, the failures of a round count for nothing: no degraded,
// no restart; a leak still rolls back at once.
type netWatch struct {
	before   string    // the path to the exit node before the first change of the sequence (lastPath)
	routeAt  time.Time // the 60 s of the checks count from here; zero while there is no default route
	pathFrom time.Time // the checks passed here, and the path is read from here on; zero before
}

// holds: the checks have not passed since the change; the failures of a round count for nothing.
func (w *netWatch) holds() bool { return w.pathFrom.IsZero() }

// NetworkChanged is a change of the network that Windows reports (section 10): why is "route" for a burst
// of route or interface changes (winhost.WatchRoutes) and "resume" for the wake from sleep (session); "suspend" is
// ignored, the wake after it counts. It is followed only while the main core runs in on, degraded or
// blocked and no operation starts or stops it: the core resets its connections itself. The path to the
// exit node read last is kept as the path before the change; a change while one is followed starts the
// wait again and keeps the path from before the first. It returns at once: the rounds of 5.5 follow the
// network from here (netRound).
func (e *Engine) NetworkChanged(why string) {
	e.mu.Lock()
	ok := e.followsNetworkLocked()
	e.mu.Unlock()
	if why == "suspend" || !ok {
		return
	}
	before, _ := e.lastPath()
	route := e.d.Host.HasDefaultRoute()
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.followsNetworkLocked() {
		return
	}
	w, again := &netWatch{before: before}, ""
	if e.net != nil {
		w.before, again = e.net.before, " again"
	}
	e.net = w
	if !route {
		e.d.Log("network changed%s (%s): no default route; the core is left alone until it is back", again, why)
		return
	}
	w.routeAt = e.d.Clock.Now()
	e.d.Log("network changed%s (%s): waiting up to %s for the checks of 5.5", again, why, netWait)
}

// followsNetworkLocked: a network change is followed now; e.mu must be held. Routing switches, lanes.set
// and systemTailscale.clearExitNode leave the core running; every other operation starts or stops it.
func (e *Engine) followsNetworkLocked() bool {
	r := e.run
	if r == nil || r.stopping || !live(e.st.Phase) || e.exiting || e.ending {
		return false
	}
	return e.op == nil || e.op.kind == "alldirect" || e.op.kind == "lanes" || e.op.kind == "clear"
}

// netRound is the part of a round of 5.5 that follows a network change (section 10); e.mu must be held.
// route: the default route was there when the round began; passed: probe-vpn and the path through the
// adapter passed (in blocked probe-vpn alone, 5.5); online: probe-vpn or probe-direct answered; path: the
// path to the exit node read in this round once the checks have passed. True when a restart began.
//  1. Without a default route nothing counts, however long; the 60 s start when it is back, again after
//     each time it went.
//  2. A round that passes within the 60 s ends the wait. A round in which neither probe answers finds no
//     network (5.5): the 60 s start again. At 60 s the core is restarted: 5.4, then 5.3.
//  3. After the wait a path that was direct before the change is read every round (10 s): direct ends
//     the sequence, relay at 60 s restarts the core: the embedded node does not get the direct path back
//     by itself, a new core does (appendix A, checks of plan 1B).
func (e *Engine) netRound(w *netWatch, route, passed, online bool, path string) bool {
	now := e.d.Clock.Now()
	if !w.holds() {
		switch {
		case path == protocol.PathDirect:
			e.d.Log("the path to the exit node is direct again after the network change")
			e.net = nil
		case !now.Before(w.pathFrom.Add(netWait)):
			e.net = nil
			if path == protocol.PathRelay {
				return e.netRestartLocked("the path to the exit node stayed relay after the network change")
			}
			e.d.Log("the path to the exit node is unknown %s after the network change; the core stays", netWait)
		}
		return false
	}
	switch {
	case !route:
		if !w.routeAt.IsZero() {
			e.d.Log("the default route is gone again; the core is left alone until it is back")
		}
		w.routeAt = time.Time{}
		return false
	case w.routeAt.IsZero():
		e.d.Log("the default route is back: waiting up to %s for the checks of 5.5", netWait)
		w.routeAt = now
	}
	switch {
	case passed && w.before == protocol.PathDirect:
		e.d.Log("the checks passed after the network change; the path to the exit node was direct before it and is read for %s", netWait)
		w.pathFrom = now
	case passed:
		e.d.Log("the checks passed after the network change")
		e.net = nil
	case !online:
		w.routeAt = now // both probes are silent: no network (5.5), the 60 s start again
	case !now.Before(w.routeAt.Add(netWait)):
		e.net = nil
		return e.netRestartLocked("network changed and the checks did not pass in 60 s")
	}
	return false
}

// netRestartLocked is the restart after a network change (5.5: 5.4, then 5.3), logged with its cause;
// e.mu must be held. There is none in blocked: step 6 of the enable would switch off the exit node the
// user selected (5.7); none with the login expired: no enable passes before the login (spec 7); and none
// in degraded: the core is not stopped while the VPN server does not answer (5.5), no new core would
// prove its tunnel, and each teardown would hand the system exit node back. The rounds of 5.5 then go on
// as they do without a network change.
func (e *Engine) netRestartLocked(cause string) bool {
	switch {
	case e.st.Phase == protocol.PhaseBlocked:
		e.d.Log("%s; no restart in phase blocked: the exit node the user selected stays", cause)
		return false
	case e.loginExpired():
		e.d.Log("%s; no restart while the login is expired", cause)
		return false
	case e.degraded:
		e.d.Log("%s; no restart in degraded: the core stays while the VPN server does not answer", cause)
		return false
	}
	e.d.Log("%s; restarting the core", cause)
	e.beginRestartLocked(protocol.ReasonCoreUnresponsive)
	return true
}
