package engine

import (
	"context"
	"time"

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
// returned and the phase stays as it is.
func (e *Engine) finishOnChecked() *protocol.Reason {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r := e.run; r != nil && !r.stopping && exited(r.proc) {
		return reason(protocol.ReasonCoreCrashed)
	}
	e.finishOn()
	return nil
}

// enable is steps 1–8 of 5.3; the order was proven on the PC (appendix A) and does not change.
func (e *Engine) enable(ctx context.Context, unattended bool) *protocol.Reason {
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
	// Steps 3 and 4; a core that exits before its start line gets one more try with new ports (6.1).
	var r *coreRun
	for attempt := 1; ; attempt++ {
		var why *protocol.Reason
		var retry bool
		r, why, retry = e.startCore(ctx)
		if why == nil {
			break
		}
		if !retry || attempt == 2 || ctx.Err() != nil {
			return why
		}
		e.d.Log("core exited before its start line; trying again with new ports")
	}
	// Step 5: the tunnel works, without touching the system VPN.
	directIP, why := e.proveTunnel(ctx, r)
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
	}
	// Step 7.
	e.flushDNS()
	// Step 8.
	return e.checkSwitch(ctx, r, directIP)
}

// preflight is step 1 of 5.3.
func (e *Engine) preflight(ctx context.Context, unattended bool) *protocol.Reason {
	e.mu.Lock()
	s := e.saved
	e.mu.Unlock()
	switch {
	case !s.LoggedIn:
		return reason(protocol.ReasonNotLoggedIn)
	case s.SelectedExitNode == "":
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
	case e.d.Host.DNSOverride():
		return reason(protocol.ReasonSystemDNSOverride)
	}
	return nil
}

// startCore is steps 3 and 4 of 5.3. retry is true when the core exited before its start line.
func (e *Engine) startCore(ctx context.Context) (*coreRun, *protocol.Reason, bool) {
	e.mu.Lock()
	s := e.saved
	e.mu.Unlock()
	invalid := func(err error) (*coreRun, *protocol.Reason, bool) {
		return nil, reason(protocol.ReasonConfigInvalid, protocol.ParamError, err.Error()), false
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
		ports:  Ports{ProbeVPN: p[0], ProbeDirect: p[1], Clash: p[2], API: p[3]},
		mode:   e.d.Core.Mode(s.DefaultLane, false, false),
		direct: direct,
		vpn:    vpn,
	}
	clashSecret := e.d.Host.Secret()
	config, err := e.d.Core.BuildConfig(CoreOptions{
		Mode: r.mode, HelperPath: e.d.Settings.HelperPath, TailscaleDir: e.d.Sys.Dir(), Hostname: e.hostname(),
		ExitNode: s.SelectedExitNode, Ports: r.ports, ClashSecret: clashSecret, APISecret: e.d.Host.Secret(),
		DevPin: e.d.Settings.DevPin,
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
	proc, err := e.d.Core.Start(path)
	if err != nil {
		return nil, reason(protocol.ReasonCoreStartTimeout, protocol.ParamError, err.Error()), false
	}
	r.proc, r.clash = proc, e.d.Core.Clash(r.ports.Clash, clashSecret)
	e.attach(r)
	deadline := e.d.Clock.Now().Add(startWait)
	for {
		ev := e.d.Core.Events(0)
		if ev.NeedLogin {
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
// It returns the address of the direct lane.
func (e *Engine) proveTunnel(ctx context.Context, r *coreRun) (string, *protocol.Reason) {
	e.mu.Lock()
	url := e.saved.ProbeURL
	e.mu.Unlock()
	deadline := e.d.Clock.Now().Add(proveWait)
	for {
		if e.d.Core.Events(0).NeedLogin {
			e.loginLost()
			return "", reason(protocol.ReasonLoginExpired)
		}
		if exited(r.proc) {
			return "", reason(protocol.ReasonCoreCrashed)
		}
		v := e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeVPN)
		d := e.d.Probe.ViaProxy(ctx, url, r.ports.ProbeDirect)
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
		if !e.d.Clock.Now().Before(deadline) {
			return "", reason(protocol.ReasonTunnelNotProven)
		}
		if !e.sleep(ctx, time.Second) {
			return "", errCancelled
		}
	}
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
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		if err := e.d.Sys.SetExitNode(ctx, restore.ExitNode, restore.AllowLAN); err != nil {
			e.d.Log("immediate restore failed: %v", err)
		}
		cancel()
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	waited := false
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
	if blocked {
		e.d.Log("core exited in phase blocked: no restart, the exit node the user selected stays")
		e.setPhase(protocol.PhaseStopping, nil)
		e.startOp("rollback", func(context.Context) { e.rollback(reason(protocol.ReasonCoreCrashed), false) })
		return
	}
	e.setPhase(protocol.PhaseStarting, nil)
	e.startOp("restart", func(ctx context.Context) { e.runRestart(ctx, protocol.ReasonCoreCrashed) })
}

// runRestart is the restart of 5.5: 5.4 (restore confirmed, core stopped), a pause, then 5.3;
// at most three times with a growing pause, in phase starting; "was on" stays set.
func (e *Engine) runRestart(ctx context.Context, why string) {
	for _, pause := range restartPauses {
		if !e.teardown() {
			e.mu.Lock()
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
			e.mu.Unlock()
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
