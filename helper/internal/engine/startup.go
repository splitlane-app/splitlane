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
// left from an earlier run is stopped; a foreign sing-box.exe is never touched. If "was on" is
// still set, the previous helper ended abnormally: enable again.
func (e *Engine) startup(ctx context.Context) {
	e.readJournal()
	e.mu.Lock()
	j, s := e.journal, e.saved
	e.mu.Unlock()
	var left CoreProc
	if s.Core != nil {
		if p, ok := e.d.Core.Find(*s.Core); ok {
			left = p
		}
	}
	if j != nil || left != nil {
		e.mu.Lock()
		e.setPhase(protocol.PhaseStopping, nil)
		e.mu.Unlock()
		e.restoreSystem()
		if left != nil {
			e.d.Log("stopping the core left from the previous run")
			e.stopProc(left)
			if !e.waitAdapterGone() {
				e.mu.Lock()
				e.finish(protocol.PhaseError, reason(protocol.ReasonAdapterLeft))
				e.mu.Unlock()
				return
			}
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
	e.setPhase(protocol.PhaseStarting, nil)
	e.mu.Unlock()
	e.runEnable(ctx, true)
}

// sessionEndCall bounds the one restore command at the end of the session: Windows gives about 5 s.
const sessionEndCall = 3 * time.Second

// SessionEnding is the shortened order of 5.4 for the end of the Windows session (logoff,
// restart, shutdown): "was on" cleared, the restore command given once, the core ended without
// waiting for a confirmation. The journal stays and is handled at the next start (5.7).
// In blocked the user has selected an exit node in the system client again: that choice is not
// overwritten (5.7); the next start finds the exit node on and deletes the journal.
// It must finish within the few seconds Windows gives the process.
func (e *Engine) SessionEnding() {
	e.mu.Lock()
	e.ending = true
	e.saved.WasOn = false
	e.save()
	j, r, o := e.journal, e.run, e.op
	blocked := e.sysExitOn
	if r != nil {
		r.stopping = true
	}
	e.mu.Unlock()
	if o != nil {
		o.cancel()
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
