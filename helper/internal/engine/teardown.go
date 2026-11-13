package engine

import (
	"context"
	"time"

	"splitlane/helper/internal/protocol"
)

// restoreSystem is step 1 of 5.4: the system exit node back from the journal, confirmed by
// `tailscale status --json`, tried every 5 s. After 30 s without a confirmation the phase is
// stopping with reason restore_pending. It returns only after the confirmation or after the
// user's explicit "disable without restore"; until then the core is not stopped.
// An exit node the user selected himself in the meantime stays, and the journal goes (5.7).
func (e *Engine) restoreSystem() {
	e.mu.Lock()
	j := e.journal
	e.mu.Unlock()
	if j == nil {
		return
	}
	started := e.d.Clock.Now()
	for {
		e.mu.Lock()
		if e.skip {
			e.skip = false
			e.dropJournal()
			e.mu.Unlock()
			e.d.Log("restore skipped by the user: journal deleted, system exit node left as is")
			return
		}
		e.mu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
		st, statusErr := e.d.Sys.Status(ctx)
		var setErr error
		if statusErr != nil || !st.ExitNodeOn {
			setErr = e.d.Sys.SetExitNode(ctx, j.ExitNode, j.AllowLAN)
			st, statusErr = e.d.Sys.Status(ctx)
		}
		cancel()
		if statusErr == nil && st.ExitNodeOn {
			e.mu.Lock()
			e.st.SystemTailscale.Installed, e.st.SystemTailscale.ExitNodeOn = true, true
			e.dropJournal()
			e.mu.Unlock()
			if st.ExitNodeIP != j.ExitNode {
				e.d.Log("system exit node is on with another node than the journal's; the user's choice stays")
			}
			return
		}
		if e.d.Clock.Now().Sub(started) >= restorePending {
			text := "exit node not confirmed"
			if setErr != nil {
				text = setErr.Error()
			} else if statusErr != nil {
				text = statusErr.Error()
			}
			e.mu.Lock()
			e.setPhase(protocol.PhaseStopping, reason(protocol.ReasonRestorePending,
				protocol.ParamCommand, e.d.Sys.RestoreCommand(j.ExitNode, j.AllowLAN), protocol.ParamError, text))
			e.mu.Unlock()
		}
		select {
		case <-e.wake:
		case <-e.d.Clock.After(restoreEvery):
		}
	}
}

// retryRestore wakes the restore loop for an attempt now ("Повторить"); skip asks it to give up.
func (e *Engine) retryRestore(skip bool) {
	e.mu.Lock()
	if skip {
		e.skip = true
	}
	e.mu.Unlock()
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// stopCore is step 2 of 5.4 for the current core.
func (e *Engine) stopCore() {
	e.mu.Lock()
	r := e.run
	if r != nil {
		r.stopping = true
	}
	e.mu.Unlock()
	if r == nil {
		return
	}
	e.stopProc(r.proc)
	e.mu.Lock()
	if e.run == r {
		e.run = nil
	}
	e.mu.Unlock()
}

// stopProc: the graceful signal, at most 5 s, then the forced end; at once when the signal fails.
func (e *Engine) stopProc(p CoreProc) {
	if exited(p) {
		return
	}
	if err := p.Interrupt(); err != nil {
		e.d.Log("graceful stop signal failed: %v; ending the core", err)
		_ = p.Kill()
	} else if !exited(p) {
		select {
		case <-p.Done():
		case <-e.d.Clock.After(stopWait):
			e.d.Log("core did not stop within %s; ending it", stopWait)
			_ = p.Kill()
		}
	}
	if exited(p) {
		return
	}
	select {
	case <-p.Done():
	case <-e.d.Clock.After(stopWait):
		e.d.Log("core still runs after it was ended")
	}
}

// waitAdapterGone is step 3 of 5.4: false when the adapter is still there after 10 s.
func (e *Engine) waitAdapterGone() bool {
	deadline := e.d.Clock.Now().Add(adapterWait)
	for {
		if !e.d.Host.AdapterPresent() {
			return true
		}
		if !e.d.Clock.Now().Before(deadline) {
			return false
		}
		<-e.d.Clock.After(500 * time.Millisecond)
	}
}

// teardown is 5.4 in its order: restore and confirm, stop the core, the adapter gone.
func (e *Engine) teardown() bool {
	e.restoreSystem()
	e.stopCore()
	return e.waitAdapterGone()
}

// beginDisableLocked starts the disabling of 5.4 for routing.disable and helper.shutdown;
// e.mu must be held. "Was on" is cleared at its very beginning (5.2).
func (e *Engine) beginDisableLocked() bool {
	e.saved.WasOn = false
	e.save()
	e.setPhase(protocol.PhaseStopping, nil)
	return e.startOp("disable", func(context.Context) {
		ok := e.teardown()
		e.mu.Lock()
		defer e.mu.Unlock()
		if ok {
			e.finish(protocol.PhaseOff, nil)
		} else {
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
		}
	})
}

// rollback is 5.4 after a failure or a cancel: the final phase is error with the reason,
// off after a cancel, error with adapter_left when the adapter stayed.
func (e *Engine) rollback(why *protocol.Reason, cancelled bool) {
	e.mu.Lock()
	e.setPhase(protocol.PhaseStopping, nil)
	e.mu.Unlock()
	ok := e.teardown()
	e.mu.Lock()
	defer e.mu.Unlock()
	switch {
	case !ok:
		e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
	case cancelled || why == errCancelled:
		e.finish(protocol.PhaseOff, nil)
	default:
		e.finish(protocol.PhaseError, why)
	}
}
