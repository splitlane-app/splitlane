package engine

import (
	"context"
	"time"

	"splitlane/helper/internal/protocol"
)

// Start loads state.json and the journal and begins the start-up of 5.7. It returns at once (after
// at most 600 ms when state.json cannot be read); the pipe already works while the start-up runs
// and shows its phase and reason.
func (e *Engine) Start() {
	e.load()
	e.mu.Lock()
	defer e.mu.Unlock()
	e.startOp("startup", e.startup)
}

// startup is 5.7: the journal first (phase stopping, step 1 of 5.4, whether a core is left or
// not; a journal that could not be read is read again until it reads), then the helper's own core
// left from an earlier run is stopped, the main core or the setup mode's (spec 7: both record their
// identity in state.json; a state.json that could not be read is read again first, readState); a
// foreign sing-box.exe is never touched. If "was on" is still set, the previous helper ended
// abnormally: enable again. A routing.disable in phase stopping without restore_pending clears it
// (cmdDisable, 5.1): the start-up then ends in off; one that comes while state.json is read again,
// as a helper.shutdown then, also clears the "was on" of the file that reads after it (offAsked).
func (e *Engine) startup(ctx context.Context) {
	e.readJournal()
	e.mu.Lock()
	if e.journal != nil {
		e.setPhase(protocol.PhaseStopping, nil)
	}
	e.mu.Unlock()
	e.restoreSystem() // nothing to do without a journal
	e.readState(ctx)
	e.mu.Lock()
	s := e.saved
	e.mu.Unlock()
	var left CoreProc
	if s.Core != nil {
		if p, ok := e.d.Core.Find(*s.Core); ok {
			left = p
		}
	}
	if left != nil {
		e.mu.Lock()
		e.setPhase(protocol.PhaseStopping, nil)
		e.mu.Unlock()
		e.d.Log("stopping the core left from the previous run")
		e.stopProc(left)
		if !e.waitAdapterGone() {
			e.mu.Lock()
			e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
			e.mu.Unlock()
			return
		}
	}
	e.mu.Lock()
	e.saved.Core = nil
	wasOn := e.saved.WasOn
	if !wasOn {
		e.finish(protocol.PhaseOff, nil)
		e.mu.Unlock()
		return
	}
	e.reenabled = true
	e.setPhase(protocol.PhaseStarting, nil)
	e.mu.Unlock()
	e.runEnable(ctx, true)
}

// ExitIfIdle ends a helper that nothing needs (spec 4.3): the scheduled task starts it at logon, and
// once the start-up of 5.7 is over (Start must have been called) it waits after on the engine's clock;
// if no window has connected since the start and the phase is off, it takes the path of
// helper.shutdown. It does nothing when "was on" made the start-up enable again, when a window has
// connected (even one that went again), or in another phase: an error stays for the window to show.
// It returns once it has decided.
func (e *Engine) ExitIfIdle(after time.Duration) {
	e.Wait()
	<-e.d.Clock.After(after)
	e.queue.Lock()
	defer e.queue.Unlock()
	e.mu.Lock()
	why := ""
	switch {
	case e.windowSeen.Load():
		why = "a window has connected"
	case e.reenabled:
		why = "the start-up enabled routing again"
	case e.st.Phase != protocol.PhaseOff:
		why = "phase " + e.st.Phase
	}
	e.mu.Unlock()
	if why != "" {
		e.d.Log("the helper stays: %s", why)
		return
	}
	e.d.Log("no window within %s after the start-up and routing is off: exiting (spec 4.3)", after)
	e.cmdShutdown(nil)
}

// sessionEndCall bounds the one restore command at the end of the session: Windows gives about 5 s.
const sessionEndCall = 3 * time.Second

// SessionEnding is the shortened order of 5.4 for the end of the Windows session (logoff,
// restart, shutdown): "was on" cleared, the restore command given once, the core ended without
// waiting for a confirmation. The journal stays and is handled at the next start (5.7).
// In blocked the user has selected an exit node in the system client again: that choice is not
// overwritten (5.7); the next start finds the exit node on and deletes the journal.
// The core of the setup mode (spec 7) is ended the same way, without waiting, and so are the calls of
// the main core's API service. It must finish within the few seconds Windows gives the process.
func (e *Engine) SessionEnding() {
	e.mu.Lock()
	e.ending = true
	e.saved.WasOn = false
	e.save()
	j, r, o, s := e.journal, e.run, e.op, e.setup
	blocked := e.sysExitOn
	if r != nil {
		r.stopping = true
		if r.endCalls != nil {
			r.endCalls()
		}
	}
	var setupProc CoreProc
	if s != nil {
		s.again, setupProc = false, s.proc
	}
	e.mu.Unlock()
	if o != nil {
		o.cancel()
	}
	if s != nil {
		s.cancel()
	}
	if setupProc != nil {
		_ = setupProc.Kill()
	}
	switch {
	case j != nil && blocked:
		e.d.Log("session end: the exit node the user selected stays; the journal waits for the next start")
	case j != nil:
		ctx, cancel := context.WithTimeout(context.Background(), sessionEndCall)
		if err := e.d.Sys.SetExitNode(ctx, j.ExitNode, j.AllowLAN); err != nil {
			e.d.Log("session end: restore command failed: %v", err)
		}
		cancel()
	}
	if r != nil {
		_ = r.proc.Kill()
	}
}
