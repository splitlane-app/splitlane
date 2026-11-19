package engine

import (
	"context"
	"time"

	"splitlane/helper/internal/connector"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// beginEnableLocked starts 5.3 in the background; e.mu must be held.
// unattended: the enable after a failure or at the start of the helper (no route yet is not a refusal).
// While another operation runs (also in the short window after a disable or rollback published its
// final phase and before its startOp cleanup clears e.op) it returns false and changes nothing:
// a phase "starting" without an operation behind it would leave the helper wedged.
func (e *Engine) beginEnableLocked(unattended bool) bool {
	if e.op != nil {
		return false
	}
	e.setPhase(protocol.PhaseStarting, nil)
	return e.startOp("enable", func(ctx context.Context) { e.runEnable(ctx, unattended) })
}

// runEnable is 5.3 with its ending: phase on, or the rollback of 5.4.
func (e *Engine) runEnable(ctx context.Context, unattended bool) {
	why := e.enable(ctx, unattended)
	if why == nil && ctx.Err() == nil {
		if why = e.finishOnChecked(); why == nil {
			return
		}
	}
	e.rollback(why, ctx.Err() != nil)
}

// finishOnChecked is step 9 of 5.3 with the last look at the core: the look and finishOn are one
// hold of e.mu. A core that exited after the last check of the steps before (for instance during
// the very probe that completed step 8) is a crash, not an "on": the reason core_crashed is
// returned and the phase stays as it is. Right after phase on the path to the exit node is read
// (spec 7, tailscale.path), in the background: the operation does not wait for the ping.
func (e *Engine) finishOnChecked() *protocol.Reason {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.run; r != nil && !r.stopping && exited(r.proc) {
		return reason(protocol.ReasonCoreCrashed)
	}
	e.finishOn()
	go e.readPath(context.Background())
	return nil
}

// enable is steps 1–8 of 5.3; the order was proven on the PC (appendix A) and does not change.
// Its first action ends the setup mode (spec 7): the main core shares the node's state folder with
// it, and step 1 must not meet it as another core.
func (e *Engine) enable(ctx context.Context, unattended bool) *protocol.Reason {
	e.stopSetup()
	if why := e.preflight(ctx, unattended); why != nil {
		return why
	}
	// Step 2: is an exit node selected in the system client, and with LAN access?
	sctx, cancel := context.WithTimeout(ctx, callTimeout)
	sys, err := e.d.Sys.Status(sctx)
	if err != nil {
		e.d.Log("system Tailscale status: %v; counted as no exit node (5.6)", err)
		sys = SysStatus{Installed: sys.Installed}
	}
	allowLAN := false
	if sys.ExitNodeOn {
		if sys.ExitNodeIP == "" {
			cancel()
			return reason(protocol.ReasonSwitchCheckFailed, protocol.ParamError, "the system exit node has no IPv4 address")
		}
		if allowLAN, err = e.d.Sys.AllowLAN(sctx); err != nil {
			cancel()
			return reason(protocol.ReasonSwitchCheckFailed, protocol.ParamError, err.Error())
		}
	}
	cancel()
	// The last check of step 1 needs step 2. The system client keeps a DNS rule for all names when
	// the tailnet overrides DNS, and also while it uses an exit node with its DNS management on
	// (appendix A, 2026-11-19). The second kind goes with the exit node at step 6, so with an exit
	// node on the answer waits until then.
	dnsRule := e.d.Host.DNSOverride()
	if dnsRule && !sys.ExitNodeOn {
		return reason(protocol.ReasonSystemDNSOverride)
	}
	r, directIP, why := e.startAndProve(ctx)
	if why != nil {
		return why
	}
	e.mu.Lock()
	stop := e.ending || e.exiting
	dead := exited(r.proc) && !r.stopping
	e.mu.Unlock()
	if stop || ctx.Err() != nil {
		return errCancelled
	}
	if dead {
		// The core failed after the proof: nothing was switched yet, nothing is left to give back.
		return reason(protocol.ReasonCoreCrashed)
	}
	// Step 6: the journal first, flushed to disk, then the system exit node off (5.7).
	if sys.ExitNodeOn {
		e.mu.Lock()
		have := e.journal != nil
		e.mu.Unlock()
		if !have {
			j := store.Journal{ExitNode: sys.ExitNodeIP, AllowLAN: allowLAN}
			if err := e.d.Store.SaveJournal(j); err != nil {
				return reason(protocol.ReasonSwitchCheckFailed, protocol.ParamError, "restore journal: "+err.Error())
			}
			e.mu.Lock()
			e.journal = &j
			e.publish()
			e.mu.Unlock()
		}
		cctx, cancel := context.WithTimeout(ctx, callTimeout)
		err := e.d.Sys.ClearExitNode(cctx)
		cancel()
		if err != nil {
			return reason(protocol.ReasonSwitchCheckFailed, protocol.ParamError, err.Error())
		}
		e.mu.Lock()
		e.st.SystemTailscale.Installed, e.st.SystemTailscale.ExitNodeOn = true, false
		r.cleared = true // from now on the watcher of this core gives the exit node back at once (5.5)
		e.mu.Unlock()
		if dnsRule {
			if why := e.waitDNSRule(ctx, r); why != nil {
				return why
			}
		}
	}
	// Step 7.
	e.flushDNS()
	// Step 8.
	return e.checkSwitch(ctx, r, directIP)
}

// waitDNSRule ends step 6 for a system client that had a DNS rule for all names at step 1: the rule
// of its exit node goes soon after the exit node is off. A rule that is still there after dnsRuleWait
// is the tailnet's override, under which names would go past the core's adapter and be blocked.
func (e *Engine) waitDNSRule(ctx context.Context, r *coreRun) *protocol.Reason {
	deadline := e.d.Clock.Now().Add(dnsRuleWait)
	for e.d.Host.DNSOverride() {
		if exited(r.proc) {
			return reason(protocol.ReasonCoreCrashed)
		}
		if !e.d.Clock.Now().Before(deadline) {
			return reason(protocol.ReasonSystemDNSOverride)
		}
		if !e.sleep(ctx, dnsRuleEvery) {
			return errCancelled
		}
	}
	return nil
}

// preflight is step 1 of 5.3. The connector decides the first checks (stage 3 part 1, section 4): the
// Tailscale node is logged in and has an exit node, or a server is selected and parses (no_server).
func (e *Engine) preflight(ctx context.Context, unattended bool) *protocol.Reason {
	e.mu.Lock()
	s := e.saved
	var why *protocol.Reason
	if connectorOf(s) == protocol.ConnectorProxy {
		_, why = e.laneLocked()
	}
	e.mu.Unlock()
	switch tailscale := connectorOf(s) == protocol.ConnectorTailscale; {
	case why != nil:
		return why
	case tailscale && !s.LoggedIn:
		return reason(protocol.ReasonNotLoggedIn)
	case tailscale && s.SelectedExitNode == "":
		return reason(protocol.ReasonNoExitNode)
	}
	if !e.d.Host.HasDefaultRoute() {
		if !unattended {
			return reason(protocol.ReasonNoDefaultRoute)
		}
		deadline := e.d.Clock.Now().Add(routeWait)
		for !e.d.Host.HasDefaultRoute() {
			if !e.d.Clock.Now().Before(deadline) {
				return reason(protocol.ReasonNoDefaultRoute)
			}
			if !e.sleep(ctx, 2*time.Second) {
				return errCancelled
			}
		}
	}
	switch {
	case e.d.Host.IPForwarding():
		return reason(protocol.ReasonIPForwarding)
	case e.d.Host.CoreRunning():
		return reason(protocol.ReasonOtherCore)
	}
	return nil
}

// startAndProve is steps 3–5 of 5.3 and returns the core that proved the tunnel with the address
// of the direct lane. A core that exits before its start line gets one more try with new ports
// (6.1). A tunnel that is not proven gets one more core with new ports too: the core's own DNS now
// and then gets no answers for the first tens of seconds (appendix A, checks of plan 1B), and the
// system is untouched before step 6. That core is stopped as in step 2 of 5.4 first. Each retry
// happens at most once per enable, so at most three cores start; same_address and login_expired
// are not retried. The restarts of 5.5 and the unattended enables run the same steps.
func (e *Engine) startAndProve(ctx context.Context) (*coreRun, string, *protocol.Reason) {
	exitRetried, proofRetried := false, false
	for {
		r, why, retry := e.startCore(ctx)
		if why != nil {
			if !retry || exitRetried || ctx.Err() != nil {
				return nil, "", why
			}
			exitRetried = true
			e.d.Log("core exited before its start line; trying again with new ports")
			continue
		}
		// Step 5: the tunnel works, without touching the system VPN.
		directIP, why := e.proveTunnel(ctx, r)
		if why == nil {
			return r, directIP, nil
		}
		if why.Code != protocol.ReasonTunnelNotProven || proofRetried || ctx.Err() != nil {
			return nil, "", why
		}
		proofRetried = true
		e.d.Log("tunnel not proven; trying the core once more with new ports")
		e.stopCore()
		if ctx.Err() != nil {
			return nil, "", errCancelled
		}
	}
}

// startCore is steps 3 and 4 of 5.3. retry is true when the core exited before its start line. The lane
// is built from the saved connector (laneLocked); a core of the proxy connector has no API service of a
// node and no login.
func (e *Engine) startCore(ctx context.Context) (*coreRun, *protocol.Reason, bool) {
	e.mu.Lock()
	s := e.saved
	e.relayout = false // the core is built from the layout in memory
	lane, why := e.laneLocked()
	e.mu.Unlock()
	if why != nil {
		return nil, why, false
	}
	invalid := func(err error) (*coreRun, *protocol.Reason, bool) {
		// The output of sing-box check may quote a server's secrets (stage 3 part 1, section 3).
		return nil, reason(protocol.ReasonConfigInvalid, protocol.ParamError, connector.Mask(err.Error())), false
	}
	direct, vpn, err := e.d.Core.LaneFiles(s.Apps, e.d.Rules)
	if err != nil {
		return invalid(err)
	}
	if err := e.d.Store.WriteRules(direct, vpn); err != nil {
		return invalid(err)
	}
	p, err := e.d.Host.FreePorts(4)
	if err != nil {
		return invalid(err)
	}
	r := &coreRun{
		ports:   Ports{ProbeVPN: p[0], ProbeDirect: p[1], Clash: p[2], API: p[3]},
		mode:    e.d.Core.Mode(s.DefaultLane, false, false),
		direct:  direct,
		vpn:     vpn,
		backend: protocol.BackendRunning,
		proxy:   lane.Proxy != nil,
	}
	clashSecret, apiSecret := e.d.Host.Secret(), e.d.Host.Secret()
	config, err := e.d.Core.BuildConfig(CoreOptions{
		Mode: r.mode, HelperPath: e.d.Settings.HelperPath, TailscaleDir: e.d.Sys.Dir(), Lane: lane,
		Ports: r.ports, ClashSecret: clashSecret, APISecret: apiSecret,
		DevPin: e.d.Settings.DevPin, LogLevel: e.d.Settings.CoreLogLevel,
	})
	if err != nil {
		return invalid(err)
	}
	path, err := e.d.Store.WriteConfig(config)
	if err != nil {
		return invalid(err)
	}
	if err := e.d.Core.Check(ctx, path); err != nil {
		return invalid(err)
	}
	// A forced end of an earlier core leaves its Wintun device behind, non-present, and the next core
	// may then fail to create its adapter (appendix A, checks of plan 1B). A failed removal does not
	// stop the start: the core may still take the device, and an early exit is tried again.
	if removed, err := e.d.Host.RemoveStaleAdapter(); err != nil {
		e.d.Log("stale Wintun device of the adapter not removed: %v", err)
	} else if removed {
		e.d.Log("removed the stale Wintun device of the adapter")
	}
	proc, err := e.d.Core.Start(path)
	if err != nil {
		return nil, reason(protocol.ReasonCoreStartTimeout, protocol.ParamError, err.Error()), false
	}
	r.proc, r.clash = proc, e.d.Core.Clash(r.ports.Clash, clashSecret)
	if !r.proxy {
		r.node = e.d.Core.Node(r.ports.API, apiSecret) // nil for a proxy: nodeCall makes no call
	}
	r.callsEnd, r.endCalls = context.WithCancel(context.Background())
	e.attach(r)
	deadline := e.d.Clock.Now().Add(startWait)
	for {
		ev := e.d.Core.Events(0)
		if ev.NeedLogin && !r.proxy {
			e.loginLost()
			return r, reason(protocol.ReasonLoginExpired), false
		}
		if ev.Started {
			return r, nil, false
		}
		if exited(proc) {
			e.mu.Lock()
			r.stopping = true // given up on purpose (a retry follows): its watcher has nothing to do
			e.mu.Unlock()
			return r, reason(protocol.ReasonCoreStartTimeout, protocol.ParamError, "the core exited before its start line"), true
		}
		if !e.d.Clock.Now().Before(deadline) {
			return r, reason(protocol.ReasonCoreStartTimeout), false
		}
		if !e.sleep(ctx, 250*time.Millisecond) {
			return r, errCancelled, false
		}
	}
}

// attach makes r the current core, records its identity in state.json (5.7) and starts its watcher (5.5).
func (e *Engine) attach(r *coreRun) {
	e.mu.Lock()
	if e.run != nil {
		e.run.stopping = true
	}
	e.run = r
	id := r.proc.Identity()
	e.saved.Core = &id
	e.save()
	e.st.AllDirect = false
	e.mu.Unlock()
	go e.watch(r)
}

// loginLost: the node needs a login; the "logged in" flag goes (5.3 step 4, 7).
func (e *Engine) loginLost() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.saved.LoggedIn = false
	e.save()
}

// proveTunnel is step 5 of 5.3: probe-vpn answers and its address differs from probe-direct's.
// It returns the address of the direct lane. A proof that gives up logs which probe never answered
// with an address and how long it waited (silentProbes): the core's DNS now and then gets no answers
// for the first tens of seconds (appendix A), on one lane or on both.
func (e *Engine) proveTunnel(ctx context.Context, r *coreRun) (string, *protocol.Reason) {
	e.mu.Lock()
	url := e.saved.ProbeURL
	e.mu.Unlock()
	began := e.d.Clock.Now()
	deadline := began.Add(proveWait)
	vpnSeen, directSeen := false, false
	for {
		if !r.proxy && e.d.Core.Events(0).NeedLogin {
			e.loginLost()
			return "", reason(protocol.ReasonLoginExpired)
		}
		if exited(r.proc) {
			return "", reason(protocol.ReasonCoreCrashed)
		}
		v := e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeVPN)
		d := e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeDirect)
		vpnSeen, directSeen = vpnSeen || v.IP != "", directSeen || d.IP != ""
		if v.IP != "" && d.IP != "" {
			if v.IP == d.IP {
				return "", reason(protocol.ReasonSameAddress)
			}
			e.mu.Lock()
			e.st.Addresses = protocol.Addresses{VPN: &v.IP, Direct: &d.IP}
			e.publish()
			e.mu.Unlock()
			return d.IP, nil
		}
		if now := e.d.Clock.Now(); !now.Before(deadline) {
			e.d.Log("the tunnel proof gave up after %s: %s", now.Sub(began), silentProbes(vpnSeen, directSeen))
			return "", reason(protocol.ReasonTunnelNotProven)
		}
		if !e.sleep(ctx, time.Second) {
			return "", errCancelled
		}
	}
}

// silentProbes names the probes of 5.3 step 5 that never answered with an address during a proof.
func silentProbes(vpn, direct bool) string {
	switch {
	case !vpn && !direct:
		return "probe-vpn and probe-direct never answered"
	case !vpn:
		return "probe-vpn never answered"
	case !direct:
		return "probe-direct never answered"
	}
	return "probe-vpn and probe-direct answered, never in the same round"
}

// checkSwitch is step 8 of 5.3: two good answers in a row through the adapter. Two answers in a
// row from the direct lane's address are a leak: the rollback starts at once. One such answer is
// not a leak: the core now and then misses the owner of a connection.
func (e *Engine) checkSwitch(ctx context.Context, r *coreRun, directIP string) *protocol.Reason {
	e.mu.Lock()
	url := e.saved.ProbeURL
	e.mu.Unlock()
	deadline := e.d.Clock.Now().Add(switchWait)
	good, leaks := 0, 0
	for {
		if exited(r.proc) {
			return reason(protocol.ReasonCoreCrashed)
		}
		res := e.d.Probe.Direct(ctx, url)
		switch {
		case res.IP != "" && res.IP == directIP:
			good, leaks = 0, leaks+1
			if leaks >= 2 {
				return reason(protocol.ReasonLeakDetected)
			}
		case res.IP != "":
			good, leaks = good+1, 0
			if good >= 2 {
				return nil
			}
		default:
			good = 0
		}
		if !e.d.Clock.Now().Before(deadline) {
			return reason(protocol.ReasonSwitchCheckFailed)
		}
		if !e.sleep(ctx, time.Second) {
			return errCancelled
		}
	}
}

// watch is the separate thread of 5.5: on an unexpected exit of the core it gives the system
// exit node back at once (journal present), without waiting for anything, then restarts the core.
// The restore is only for a run that switched the system exit node off (step 6 of 5.3, r.cleared):
// a core that exits before that, for instance before its start line, has nothing to give back and
// must not touch the system exit node (5.7: the proof of the tunnel leaves the system VPN alone).
// The check and the snapshot of the journal are one hold of e.mu, right before the call.
// In phase blocked the user has selected an exit node in the system client himself: it is neither
// overwritten from the journal nor switched off at step 6 of a restart (5.7 wins over 5.5 here).
// Then 5.4 runs without a restart: the journal goes, the user's choice stays, error core_crashed.
// While an operation runs it sees the exit itself at its next check; but the last check of an
// operation is followed by a short stretch (the last probe, finishOn, the cleanup of the
// operation) in which nobody looks, so the watcher waits for the operation and then acts on
// the core if it is still the current one in a live phase: a crash is never dropped.
// systemTailscale.clearExitNode is such an operation: when it ends, blocked may be over and the
// user's exit node off for this run, in the clear's journal. So when the watcher restored nothing
// at first, it reads blocked again after the wait and gives the exit node back now, before the
// restart. After its own restore it does not read blocked again: the exit node is on because of it.
// routing.allDirect and lanes.set while the core runs are such operations too: the restart that
// follows them resets "all direct" (5.2) and starts the core with the layout state.json holds then.
func (e *Engine) watch(r *coreRun) {
	<-r.proc.Done()
	e.mu.Lock()
	if r.stopping || e.run != r || e.ending || e.exiting {
		e.mu.Unlock()
		return
	}
	e.d.Log("core exited unexpectedly")
	blocked := e.sysExitOn // the user selected an exit node himself: it stays (5.7)
	var restore *store.Journal
	if r.cleared && e.journal != nil && !blocked {
		j := *e.journal
		restore = &j
	}
	e.mu.Unlock()
	if restore != nil {
		e.restoreAtOnce(*restore)
	}
	restored := restore != nil
	e.mu.Lock()
	defer e.mu.Unlock()
	waited := false
	for {
		for e.op != nil && e.run == r && !r.stopping && !e.exiting && !e.ending {
			o := e.op
			e.mu.Unlock()
			<-o.done
			e.mu.Lock()
			waited = true
		}
		if e.run != r || r.stopping || e.exiting || e.ending || (waited && !live(e.st.Phase)) {
			return
		}
		if restored {
			break
		}
		blocked = e.sysExitOn
		if blocked || !r.cleared || e.journal == nil {
			break
		}
		j := *e.journal
		e.mu.Unlock()
		e.restoreAtOnce(j)
		e.mu.Lock()
		restored, waited = true, true // the lock was free: look again before acting
	}
	if blocked {
		e.d.Log("core exited in phase blocked: no restart, the exit node the user selected stays")
		e.setPhase(protocol.PhaseStopping, nil)
		e.startOp("rollback", func(context.Context) { e.rollback(reason(protocol.ReasonCoreCrashed), false) })
		return
	}
	e.beginRestartLocked(protocol.ReasonCoreCrashed)
}

// beginRestartLocked starts the restart of 5.5 in phase starting; e.mu must be held. "All direct"
// ends here (5.2): the next core starts in the normal mode, and no state of the restart shows the
// flag, also not one that an all-direct switch which ended just before set for the core that is gone.
func (e *Engine) beginRestartLocked(why string) {
	e.st.AllDirect = false
	e.setPhase(protocol.PhaseStarting, nil)
	e.startOp("restart", func(ctx context.Context) { e.runRestart(ctx, why) })
}

// runRestart is the restart of 5.5: 5.4 (restore confirmed, core stopped), a pause, then 5.3;
// at most three times with a growing pause, in phase starting; "was on" stays set. A restart
// cancelled during its 5.4 (routing.disable, 5.1) goes from there to the rollback, not to starting.
func (e *Engine) runRestart(ctx context.Context, why string) {
	for _, pause := range restartPauses {
		if !e.teardown() {
			e.mu.Lock()
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
			e.mu.Unlock()
			return
		}
		if ctx.Err() != nil {
			e.rollback(errCancelled, true)
			return
		}
		e.mu.Lock()
		e.setPhase(protocol.PhaseStarting, nil)
		e.mu.Unlock()
		if !e.sleep(ctx, pause) {
			e.rollback(errCancelled, true)
			return
		}
		got := e.enable(ctx, true)
		if got == nil && ctx.Err() == nil {
			if got = e.finishOnChecked(); got == nil {
				return
			}
		}
		if ctx.Err() != nil {
			e.rollback(errCancelled, true)
			return
		}
		e.d.Log("restart attempt failed: %s", got.Code)
	}
	e.rollback(reason(why), false)
}
