package engine

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

// Handle answers one request of 5.1. The pipe server answers hello itself and gates the
// protocol version. State-changing commands run one at a time in arrival order (the queue);
// the transitions starting and stopping do not hold the queue: the phase table answers there.
func (e *Engine) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	if !slices.Contains(protocol.Methods, method) {
		return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown method %q", method)
	}
	switch method {
	case protocol.MethodHello:
		return e.Hello(), nil
	case protocol.MethodStateGet:
		if err := noParams(params); err != nil {
			return nil, err
		}
		return e.State(), nil
	case protocol.MethodLogin, protocol.MethodLogout, protocol.MethodActivitySubscribe, protocol.MethodDiagnosticsExport:
		return nil, protocol.Errorf(protocol.CodeInternal, "%s is not available in this build (Plan 1B)", method)
	}
	e.queue.Lock()
	keep := false // true when a background job took over the queue and unlocks it when done
	defer func() {
		if !keep {
			e.queue.Unlock()
		}
	}()
	var err *protocol.Error
	switch method {
	case protocol.MethodEnable:
		err = e.cmdEnable(params)
	case protocol.MethodDisable:
		err = e.cmdDisable(params)
	case protocol.MethodAllDirect:
		keep, err = e.cmdAllDirect(params)
	case protocol.MethodLanesSet:
		err = e.cmdLanesSet(ctx, params)
	case protocol.MethodSetExitNode:
		err = e.cmdSetExitNode(params)
	case protocol.MethodClearSystemExitNode:
		keep, err = e.cmdClearSystemExitNode(params)
	case protocol.MethodShutdown:
		err = e.cmdShutdown(params)
	}
	if err != nil {
		return nil, err
	}
	return struct{}{}, nil
}

func noParams(params json.RawMessage) *protocol.Error {
	var none struct{}
	return protocol.DecodeParams(params, &none)
}

func busy() *protocol.Error { return protocol.Errorf(protocol.CodeBusy, "the helper is busy") }

func wrongPhase(phase string) *protocol.Error {
	return protocol.Errorf(protocol.CodeWrongPhase, "not possible in phase %s", phase)
}

// cmdEnable: off and error start 5.3; degraded runs an extra check ("Повторить"); stopping is busy.
// In off and error an operation that is still finishing (its cleanup has not cleared e.op) is busy too:
// beginEnableLocked refuses before it changes the phase.
func (e *Engine) cmdEnable(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.exiting {
		return busy()
	}
	switch e.st.Phase {
	case protocol.PhaseOff, protocol.PhaseError:
		if !e.beginEnableLocked(false) {
			return busy()
		}
	case protocol.PhaseDegraded:
		go e.checkHealth(context.Background())
	case protocol.PhaseStopping:
		return busy()
	}
	return nil
}

// cmdDisable: 5.4 with the phase table of 5.1; "was on" is cleared at the beginning. With
// skipRestore the journal goes without a restore in every phase that disables (5.1, 5.4).
func (e *Engine) cmdDisable(params json.RawMessage) *protocol.Error {
	var p protocol.DisableParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	switch e.st.Phase {
	case protocol.PhaseOff:
	case protocol.PhaseError:
		e.finish(protocol.PhaseOff, nil)
	case protocol.PhaseStarting:
		e.saved.WasOn = false
		e.save()
		e.skip = p.SkipRestore
		if e.op != nil {
			e.op.cancel()
		}
	case protocol.PhaseOn, protocol.PhaseDegraded, protocol.PhaseBlocked:
		if !e.beginDisableLocked() {
			return busy()
		}
		e.skip = p.SkipRestore // read by the operation under e.mu, so it is set before its first look
	case protocol.PhaseStopping:
		pending := e.st.Reason != nil && e.st.Reason.Code == protocol.ReasonRestorePending
		if p.SkipRestore {
			e.skip = true
			if e.op != nil {
				e.op.cancel()
			}
		}
		if p.SkipRestore || pending {
			select {
			case e.wake <- struct{}{}:
			default:
			}
		}
	}
	return nil
}

// cmdAllDirect: applied in on and degraded; the answer comes at once, the result in state events.
func (e *Engine) cmdAllDirect(params json.RawMessage) (bool, *protocol.Error) {
	var p protocol.AllDirectParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return false, err
	}
	if p.On == nil {
		return false, protocol.Errorf(protocol.CodeBadRequest, "params.on is required")
	}
	e.mu.Lock()
	phase, exiting := e.st.Phase, e.exiting
	e.mu.Unlock()
	switch {
	case exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping:
		return false, busy()
	case phase != protocol.PhaseOn && phase != protocol.PhaseDegraded:
		return false, wrongPhase(phase)
	}
	go func() {
		defer e.queue.Unlock()
		e.applyAllDirect(context.Background(), *p.On)
	}()
	return true, nil
}

// cmdLanesSet is 8.3: check the whole layout, apply it while the core runs, save it.
func (e *Engine) cmdLanesSet(ctx context.Context, params json.RawMessage) *protocol.Error {
	var p protocol.LanesSetParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	e.mu.Lock()
	phase, exiting := e.st.Phase, e.exiting
	e.mu.Unlock()
	if exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping {
		return busy()
	}
	if err := protocol.ValidateLanes(p); err != nil {
		return err
	}
	apps, rejected, err := rules.Check(p.Apps, e.d.Rules)
	if err != nil {
		return protocol.Errorf(protocol.CodeBadRequest, "%v", err)
	}
	if len(rejected) > 0 {
		return &protocol.Error{Code: protocol.CodeRuleRejected, Message: "rules rejected", Rules: rejected}
	}
	var undo func()
	if live(phase) {
		var aerr *protocol.Error
		if undo, aerr = e.applyLanes(ctx, apps, p.DefaultLane); aerr != nil {
			return aerr
		}
	}
	// Step 3: the answer comes only after the layout is in state.json. A write that fails is
	// apply_failed: the previous layout stays, and the core gets its previous files and mode back.
	e.mu.Lock()
	prevApps, prevLane := e.saved.Apps, e.saved.DefaultLane
	e.saved.Apps, e.saved.DefaultLane = apps, p.DefaultLane
	saveErr := e.save()
	switch {
	case saveErr == nil, errors.Is(saveErr, errStateLate):
		e.publish() // the new layout, or the one state.json holds, read only now
	default:
		e.saved.Apps, e.saved.DefaultLane = prevApps, prevLane
	}
	e.mu.Unlock()
	if saveErr != nil {
		if undo != nil {
			undo()
		}
		return protocol.Errorf(protocol.CodeApplyFailed, "layout not saved: %v", saveErr)
	}
	return nil
}

// applyLanes writes the rule files and switches the mode, then reconciles for 3 s (8.3).
// On a reload error or connections left on the wrong lane the previous files and mode come back.
// After a success it returns undo, which brings the previous files and mode back the same way.
func (e *Engine) applyLanes(ctx context.Context, apps []protocol.AppRule, defaultLane string) (func(), *protocol.Error) {
	e.mu.Lock()
	r, allDirect, degraded := e.run, e.st.AllDirect, e.degraded
	var oldDirect, oldVPN []byte
	var oldMode string
	if r != nil {
		oldDirect, oldVPN, oldMode = r.direct, r.vpn, r.mode
	}
	e.mu.Unlock()
	if r == nil {
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "no core is running")
	}
	set, err := rules.Compile(apps, e.d.Rules)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "%v", err)
	}
	direct, vpn, err := e.d.Core.LaneFiles(apps, e.d.Rules)
	if err != nil {
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "%v", err)
	}
	mode := e.d.Core.Mode(defaultLane, allDirect, degraded)
	mark := e.d.Core.LogSize()
	if err := e.d.Store.WriteRules(direct, vpn); err != nil {
		e.revert(ctx, r, oldDirect, oldVPN, oldMode, false)
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "rule files: %v", err)
	}
	if mode != oldMode {
		if err := r.clash.SetMode(ctx, mode); err != nil {
			e.revert(ctx, r, oldDirect, oldVPN, oldMode, false)
			return nil, protocol.Errorf(protocol.CodeApplyFailed, "mode: %v", err)
		}
	}
	if !e.reconcile(ctx, r, func(path string) string { return expectedLane(set, defaultLane, allDirect, path) }, mark) {
		e.revert(ctx, r, oldDirect, oldVPN, oldMode, mode != oldMode)
		return nil, protocol.Errorf(protocol.CodeApplyFailed, "the core did not apply the layout within %s", applyWindow)
	}
	e.mu.Lock()
	r.direct, r.vpn, r.mode = direct, vpn, mode
	e.mu.Unlock()
	undo := func() {
		e.revert(ctx, r, oldDirect, oldVPN, oldMode, mode != oldMode)
		e.mu.Lock()
		r.direct, r.vpn, r.mode = oldDirect, oldVPN, oldMode
		e.mu.Unlock()
	}
	return undo, nil
}

func (e *Engine) revert(ctx context.Context, r *coreRun, direct, vpn []byte, mode string, modeChanged bool) {
	if err := e.d.Store.WriteRules(direct, vpn); err != nil {
		e.d.Log("previous rule files not restored: %v", err)
	}
	if modeChanged {
		if err := r.clash.SetMode(ctx, mode); err != nil {
			e.d.Log("previous mode %s not restored: %v", mode, err)
		}
	}
}

// reconcile is the check of 8.3: for 3 s, every 250 ms, each connection whose lane rules 8–11 of
// 6.2 decide (Conn.ByLanes) and that runs on another lane than want names is closed through the
// API. False: the core could not read a lane file again, or connections on the wrong lane were
// still there at the end.
func (e *Engine) reconcile(ctx context.Context, r *coreRun, want func(path string) string, mark int64) bool {
	end := e.d.Clock.Now().Add(applyWindow)
	for {
		conns, err := r.clash.Connections(ctx)
		wrong := 0
		for _, c := range conns {
			if !c.ByLanes {
				continue
			}
			if c.Lane != want(c.ProcessPath) {
				wrong++
				if err := r.clash.CloseConn(ctx, c.ID); err != nil {
					e.d.Log("close connection: %v", err)
				}
			}
		}
		if e.d.Core.Events(mark).ReloadFailed {
			e.d.Log("the core could not read a rule file again")
			return false
		}
		if !e.d.Clock.Now().Before(end) {
			return err == nil && wrong == 0
		}
		if !e.sleep(ctx, applyEvery) {
			return false
		}
	}
}

// expectedLane is where rules 8–11 of 6.2 send a process: everything direct in "all direct",
// else the lane of its rule, else the main lane (in degraded too: only the DNS changes there).
func expectedLane(set *rules.Set, mainLane string, allDirect bool, path string) string {
	if allDirect {
		return protocol.LaneDirect
	}
	if path != "" {
		if lane, ok := set.LaneOf(path); ok {
			return lane
		}
	}
	return mainLane
}

// applyAllDirect switches "all direct" on or off and reconciles (8.3); on failure the previous
// mode comes back and allDirect stays as it was. The window learns the result from the state event.
// The DNS moves with the mode (6.4): to the adapter's DNS and back, unless degraded has it there
// already; each such switch, the one back after a failure too, flushes the Windows DNS cache.
func (e *Engine) applyAllDirect(ctx context.Context, on bool) {
	e.mu.Lock()
	r, lane, degraded, apps, was := e.run, e.saved.DefaultLane, e.degraded, e.saved.Apps, e.st.AllDirect
	old := ""
	if r != nil {
		old = r.mode
	}
	e.mu.Unlock()
	if r == nil {
		return
	}
	mode := e.d.Core.Mode(lane, on, degraded)
	set, err := rules.Compile(apps, e.d.Rules)
	if err != nil {
		e.d.Log("all direct: %v", err)
		return
	}
	dnsMoves := localDNS(was, degraded) != localDNS(on, degraded)
	mark := e.d.Core.LogSize()
	switched := r.clash.SetMode(ctx, mode) == nil
	if switched && dnsMoves {
		e.flushDNS()
	}
	ok := switched && e.reconcile(ctx, r, func(path string) string { return expectedLane(set, lane, on, path) }, mark)
	if !ok {
		if err := r.clash.SetMode(ctx, old); err != nil {
			e.d.Log("all direct: previous mode %s not restored: %v", old, err)
		} else if switched && dnsMoves {
			e.flushDNS()
		}
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if ok {
		r.mode = mode
		e.st.AllDirect = on
	}
	e.publish()
}

// cmdSetExitNode saves the choice; while routing is on it disables and enables again (7).
func (e *Engine) cmdSetExitNode(params json.RawMessage) *protocol.Error {
	var p protocol.SetExitNodeParams
	if err := protocol.DecodeParams(params, &p); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	phase := e.st.Phase
	// A live phase with an operation still in place is the short window after an enable or restart
	// published it and before its cleanup: the reconnect has no operation to run in, and a phase
	// "stopping" without one would wedge the helper. Nothing is saved or published then.
	if e.exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping || (live(phase) && e.op != nil) {
		return busy()
	}
	if err := protocol.ValidateExitNodeID(p.ID, e.saved.ExitNodes); err != nil {
		return err
	}
	e.saved.SelectedExitNode = p.ID
	e.save()
	if !live(phase) {
		e.publish()
		return nil
	}
	e.setPhase(protocol.PhaseStopping, nil)
	e.startOp("reconnect", func(ctx context.Context) {
		if !e.teardown() {
			e.mu.Lock()
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
			e.mu.Unlock()
			return
		}
		e.mu.Lock()
		e.setPhase(protocol.PhaseStarting, nil)
		e.mu.Unlock()
		e.runEnable(ctx, false)
	})
	return nil
}

// cmdClearSystemExitNode: in blocked the system exit node is switched off (5.6, 5.7). The clear is
// an operation: a crash of the core during it is handled by the watcher after it, never beside it
// (the rollback of blocked would otherwise delete the journal the clear has just written while the
// clear goes on to switch the exit node off). Another operation still in place is busy. The queue
// stays taken until the operation is gone, so the next command never meets it.
func (e *Engine) cmdClearSystemExitNode(params json.RawMessage) (bool, *protocol.Error) {
	if err := noParams(params); err != nil {
		return false, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	phase := e.st.Phase
	switch {
	case e.exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping:
		return false, busy()
	case phase == protocol.PhaseOff || phase == protocol.PhaseError:
		return false, wrongPhase(phase)
	case phase != protocol.PhaseBlocked:
		return false, nil
	}
	if !e.startOp("clear", e.clearSystemExitNode) {
		return false, busy()
	}
	o := e.op
	go func() {
		<-o.done
		e.queue.Unlock()
	}()
	return true, nil
}

// cmdShutdown: disable (or roll back) as the phase requires, then the process ends (OnExit).
func (e *Engine) cmdShutdown(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.exiting {
		return nil
	}
	e.exiting = true
	e.saved.WasOn = false
	e.save()
	switch e.st.Phase {
	case protocol.PhaseStarting:
		if e.op != nil {
			e.op.cancel()
		}
	case protocol.PhaseOn, protocol.PhaseDegraded, protocol.PhaseBlocked:
		e.beginDisableLocked() // refused in the window of an operation that is finishing: exitWhenDone retries
	}
	go e.exitWhenDone()
	return nil
}

// exitWhenDone waits for the operations to end, disables as long as a live phase is left, then calls
// OnExit. The loop closes the window in which an enable or restart has published a live phase but
// startOp's cleanup has not yet cleared e.op: beginDisableLocked refuses there, so shutdown would
// otherwise end the process with the core running and the system exit node off. exiting keeps every
// other command and the watcher out, so nothing but this loop starts an operation now.
func (e *Engine) exitWhenDone() {
	for {
		e.Wait()
		e.mu.Lock()
		if e.op != nil { // an operation began between Wait and the lock: wait for it too
			e.mu.Unlock()
			continue
		}
		again := live(e.st.Phase)
		if again {
			e.beginDisableLocked()
		}
		e.mu.Unlock()
		if !again {
			break
		}
	}
	e.d.OnExit()
}
