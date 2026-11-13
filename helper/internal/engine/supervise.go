package engine

import (
	"context"
	"sync"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// Run drives 5.5 and 5.6 until ctx ends: the system client every 3 s, the health checks every 10 s.
func (e *Engine) Run(ctx context.Context) {
	go e.every(ctx, systemEvery, e.pollSystem)
	e.every(ctx, healthEvery, e.checkHealth)
}

func (e *Engine) every(ctx context.Context, d time.Duration, fn func(context.Context)) {
	t := time.NewTicker(d)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fn(ctx)
		}
	}
}

// checkHealth is one round of 5.5. First: the VPN lane through probe-vpn; three failures in a row
// while probe-direct works give degraded; both silent means no network, nothing changes. Second
// (not in blocked): the path through the adapter; two answers in a row from the direct address are
// a leak (rollback, error); three failures while probe-vpn works mean a broken core (restart).
// The requests of a round run side by side. probe-direct runs every round: the addresses follow
// every successful check (5.5), and the leak check compares with this round's direct address, so
// a new address of the provider (DHCP, CGNAT, another network) does not hide a leak.
func (e *Engine) checkHealth(ctx context.Context) {
	e.mu.Lock()
	r, phase, url := e.run, e.st.Phase, e.saved.ProbeURL
	idle := e.op == nil && !e.exiting && !e.ending
	e.mu.Unlock()
	if r == nil || !idle || !live(phase) {
		return
	}
	adapter := phase != protocol.PhaseBlocked
	var v, d, a ProbeResult
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); v = e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeVPN) }()
	go func() { defer wg.Done(); d = e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeDirect) }()
	if adapter {
		wg.Add(1)
		go func() { defer wg.Done(); a = e.d.Probe.Direct(ctx, url) }()
	}
	wg.Wait()

	// Probes ran without the queue; acting on them takes it, so that lanes.set is not delayed by them.
	e.queue.Lock()
	defer e.queue.Unlock()
	e.mu.Lock()
	if e.run != r || e.op != nil || !live(e.st.Phase) {
		e.mu.Unlock()
		return
	}
	dirty := false
	// An answer of probe-direct equal to probe-vpn's is not the direct lane (the system exit node
	// is on again, 5.6): the stored direct address stays.
	if d.IP != "" && d.IP != v.IP && (e.st.Addresses.Direct == nil || *e.st.Addresses.Direct != d.IP) {
		ip := d.IP
		e.st.Addresses.Direct, dirty = &ip, true
	}
	switch {
	case v.Reached:
		e.vpnFails = 0
		if e.degraded {
			e.degraded, dirty = false, true // one good check of probe-vpn is enough to return to on
		}
		if v.IP != "" && (e.st.Addresses.VPN == nil || *e.st.Addresses.VPN != v.IP) {
			ip := v.IP
			e.st.Addresses.VPN, dirty = &ip, true
		}
	case d.Reached:
		e.vpnFails++
		if e.vpnFails >= 3 && !e.degraded {
			e.degraded, dirty = true, true
		}
	}
	if adapter {
		direct := e.st.Addresses.Direct // this round's when probe-direct answered, else the last one
		switch {
		case a.IP != "" && direct != nil && a.IP == *direct:
			e.leakHits, e.adapterFails = e.leakHits+1, 0
		case a.Reached:
			e.adapterFails = 0
			if a.IP != "" {
				e.leakHits = 0
			}
		case v.Reached:
			e.adapterFails++
		}
		if e.leakHits >= 2 {
			e.d.Log("traffic leaves through the direct address: rollback")
			e.startOp("rollback", func(context.Context) { e.rollback(reason(protocol.ReasonLeakDetected), false) })
			e.mu.Unlock()
			return
		}
		if e.adapterFails >= 3 {
			e.d.Log("the path through the adapter fails while probe-vpn works: restarting the core")
			e.setPhase(protocol.PhaseStarting, nil)
			e.startOp("restart", func(ctx context.Context) { e.runRestart(ctx, protocol.ReasonCoreUnresponsive) })
			e.mu.Unlock()
			return
		}
	}
	now := e.degraded
	mode, current := e.d.Core.Mode(e.saved.DefaultLane, e.st.AllDirect, now), r.mode
	if dirty {
		e.refreshLive()
	}
	e.mu.Unlock()
	// The mode follows the state, not the transition: every round tries again until the core matches.
	// degraded: names through the provider's DNS (6.3, 6.4); the Windows DNS cache is flushed both ways,
	// once per mode actually set. A refused mode would otherwise leave the direct lane without names
	// (degraded) or the VPN lane on the provider's DNS behind a healthy phase (back to on).
	if mode != current {
		if err := r.clash.SetMode(ctx, mode); err != nil {
			e.d.Log("mode %s not set: %v", mode, err)
		} else {
			e.mu.Lock()
			r.mode = mode
			e.mu.Unlock()
			e.flushDNS()
		}
	}
}

// refreshLive sets on, degraded or blocked from the flags and publishes; e.mu must be held.
// With both conditions the phase is blocked (5.2).
func (e *Engine) refreshLive() {
	phase, why := protocol.PhaseOn, (*protocol.Reason)(nil)
	if e.degraded {
		phase, why = protocol.PhaseDegraded, reason(protocol.ReasonVPNUnreachable)
	}
	if e.sysExitOn {
		phase, why = protocol.PhaseBlocked, nil
	}
	e.setPhase(phase, why)
}

// pollSystem is 5.6: while routing is on, and while a window is connected, the system client is
// read every 3 s; an exit node selected there again gives phase blocked.
func (e *Engine) pollSystem(ctx context.Context) {
	e.mu.Lock()
	watch := live(e.st.Phase) || e.clients.Load() > 0
	e.mu.Unlock()
	if !watch {
		return
	}
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	st, err := e.d.Sys.Status(cctx)
	cancel()
	if err != nil {
		st.ExitNodeOn = false // a stopped service or no login counts as no exit node (5.6)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	sys := &e.st.SystemTailscale
	changed := sys.Installed != st.Installed || sys.ExitNodeOn != st.ExitNodeOn
	sys.Installed, sys.ExitNodeOn = st.Installed, st.ExitNodeOn
	if live(e.st.Phase) && e.op == nil && e.run != nil && e.sysExitOn != st.ExitNodeOn {
		e.sysExitOn = st.ExitNodeOn
		e.refreshLive()
		return
	}
	if changed {
		e.publish()
	}
}

// clearSystemExitNode is systemTailscale.clearExitNode in phase blocked: the exit node the user
// selected now goes into the journal (5.7), then it is switched off; the queue must be held.
func (e *Engine) clearSystemExitNode(ctx context.Context) {
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	st, err := e.d.Sys.Status(cctx)
	if err != nil || !st.ExitNodeOn || st.ExitNodeIP == "" {
		e.d.Log("clear system exit node: nothing selected (%v)", err)
		return
	}
	allowLAN, err := e.d.Sys.AllowLAN(cctx)
	if err != nil {
		e.d.Log("clear system exit node: LAN flag unknown: %v", err)
		return
	}
	j := store.Journal{ExitNode: st.ExitNodeIP, AllowLAN: allowLAN}
	if err := e.d.Store.SaveJournal(j); err != nil {
		e.d.Log("clear system exit node: journal not written: %v", err)
		return
	}
	e.mu.Lock()
	e.journal = &j
	e.mu.Unlock()
	if err := e.d.Sys.ClearExitNode(cctx); err != nil {
		e.d.Log("clear system exit node: %v", err)
		return
	}
	e.flushDNS()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.st.SystemTailscale.ExitNodeOn = false
	if live(e.st.Phase) && e.run != nil {
		e.sysExitOn = false
		e.refreshLive()
	}
}
