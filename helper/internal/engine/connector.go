package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"splitlane/helper/internal/protocol"
)

// Times of the connector (spec 7).
const (
	nodeEvery = 2 * time.Second // the node of the setup core is read so often
	setupIdle = 5 * time.Minute // the setup core runs so long after the last window went
)

// setupRun is one run of the setup mode (spec 7): the core with the embedded node, its local DNS and
// its API service, without an adapter. Its goroutine (runSetup) owns the core from its start to its end.
type setupRun struct {
	cancel context.CancelFunc
	done   chan struct{} // closed when the run is over and its core has ended
	ready  chan struct{} // closed when the API service has answered once
	node   NodeAPI       // the API service of this run; set before ready is closed
	proc   CoreProc      // guarded by Engine.mu; nil until the core has started
	ending bool          // guarded by Engine.mu: the run is stopping; a login or a window now asks for the next one
	again  bool          // guarded by Engine.mu: the next run starts as soon as this one is over
	gen    int           // guarded by Engine.mu: a logout moves it on, so a look at the node from before it is dropped
}

// setupPhaseLocked is the phase check of tailscale.login and tailscale.logout (5.1): nil in off and
// error; busy in starting and stopping and after helper.shutdown; wrong_phase in on, degraded and
// blocked. e.mu must be held.
func (e *Engine) setupPhaseLocked() *protocol.Error {
	switch phase := e.st.Phase; {
	case e.exiting || phase == protocol.PhaseStarting || phase == protocol.PhaseStopping:
		return busy()
	case phase != protocol.PhaseOff && phase != protocol.PhaseError:
		return wrongPhase(phase)
	}
	return nil
}

// cmdLogin is tailscale.login in off and error: the setup mode starts, the login link comes in the
// state (spec 7). While it runs nothing new starts: its link stays (Review Focus 5). The answer comes
// at once, the core starts in the background.
func (e *Engine) cmdLogin(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.setupPhaseLocked(); err != nil {
		return err
	}
	e.startSetupLocked()
	return nil
}

// cmdLogout is tailscale.logout in off and error (spec 7): the answer comes at once, the logout runs in
// the background (logout).
func (e *Engine) cmdLogout(params json.RawMessage) *protocol.Error {
	if err := noParams(params); err != nil {
		return err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.setupPhaseLocked(); err != nil {
		return err
	}
	go e.logout()
	return nil
}

// windowCame: a window connected while none was (SetClients). With "logged in" saved the setup mode
// starts, to show the state of the node and its exit nodes (spec 7).
func (e *Engine) windowCame() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.clients.Load() > 0 && e.saved.LoggedIn {
		e.startSetupLocked()
	}
}

// startSetupLocked starts the setup mode unless it runs (spec 7): only in off and error, never after
// helper.shutdown or the end of the session; e.mu must be held. A run that is ending is followed by a
// new one once its core has ended.
func (e *Engine) startSetupLocked() {
	if e.exiting || e.ending || (e.st.Phase != protocol.PhaseOff && e.st.Phase != protocol.PhaseError) {
		return
	}
	if s := e.setup; s != nil {
		if s.ending {
			s.again = true
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &setupRun{cancel: cancel, done: make(chan struct{}), ready: make(chan struct{})}
	e.setup = s
	go e.runSetup(ctx, s)
}

// stopSetup ends the setup mode and returns once its core has ended (step 2 of 5.4): the first action
// of an enable (spec 7; 5.3 step 1 does not count the own setup core as another core, it is gone by
// then) and of helper.shutdown. A logout under way is finished first. A starting run is given up: its
// core, if it has started, is stopped like a running one.
func (e *Engine) stopSetup() {
	e.connector.Lock()
	defer e.connector.Unlock()
	for {
		e.mu.Lock()
		s := e.setup
		if s != nil {
			s.again = false
		}
		e.mu.Unlock()
		if s == nil {
			return
		}
		s.cancel()
		<-s.done
	}
}

// setupMayStart waits for an operation that is still finishing (the setup core never runs beside the
// main core: they share the node's state folder) and tells whether the setup core may start: phase off
// or error, no helper.shutdown, no end of the session, the run not cancelled. An operation that enables
// keeps the phase away from off and error, and its first action cancels this run (stopSetup).
func (e *Engine) setupMayStart(ctx context.Context) bool {
	for {
		e.mu.Lock()
		o := e.op
		ok := !e.exiting && !e.ending && (e.st.Phase == protocol.PhaseOff || e.st.Phase == protocol.PhaseError)
		e.mu.Unlock()
		if !ok || o == nil {
			return ok && ctx.Err() == nil
		}
		select {
		case <-o.done:
		case <-ctx.Done():
			return false
		}
	}
}

// runSetup is one run of the setup mode: the core as in steps 3 and 4 of 5.3 without the adapter (its
// own API port and secret, the config checked, the identity in state.json for 5.7, the start line
// within 30 s), then a look at the node every 2 s until the run is cancelled (stopSetup, the end of the
// session), its core exits, or no window has been connected for 5 minutes. The core is stopped as in
// step 2 of 5.4 before the run is over (killed at the end of the session). The phase never changes.
func (e *Engine) runSetup(ctx context.Context, s *setupRun) {
	var proc CoreProc
	defer func() {
		e.mu.Lock()
		s.ending = true
		kill := e.ending
		e.mu.Unlock()
		switch {
		case proc == nil:
		case kill:
			_ = proc.Kill() // the end of the session: no wait (5.4)
		default:
			e.stopProc(proc)
		}
		e.mu.Lock()
		if e.setup == s {
			e.setup = nil
		}
		if e.nodeBackend != protocol.BackendStopped || e.nodeAuthURL != "" {
			e.nodeBackend, e.nodeAuthURL = protocol.BackendStopped, ""
			e.publish()
		}
		if s.again {
			e.startSetupLocked()
		}
		e.mu.Unlock()
		s.cancel()
		close(s.done)
	}()
	if !e.setupMayStart(ctx) {
		return
	}
	p, err := e.d.Host.FreePorts(1)
	if err != nil {
		e.d.Log("setup mode: no free port: %v", err)
		return
	}
	secret := e.d.Host.Secret()
	e.mu.Lock()
	host := e.hostname()
	e.mu.Unlock()
	config, err := e.d.Core.BuildSetupConfig(SetupOptions{Hostname: host, APIPort: p[0], APISecret: secret, LogLevel: e.d.Settings.CoreLogLevel})
	if err != nil {
		e.d.Log("setup mode: config: %v", err)
		return
	}
	path, err := e.d.Store.WriteConfig(config)
	if err != nil {
		e.d.Log("setup mode: config not written: %v", err)
		return
	}
	if err := e.d.Core.Check(ctx, path); err != nil {
		if ctx.Err() == nil {
			e.d.Log("setup mode: config refused: %v", err)
		}
		return
	}
	if ctx.Err() != nil {
		return
	}
	proc, err = e.d.Core.Start(path)
	if err != nil {
		proc = nil
		e.d.Log("setup mode: the core did not start: %v", err)
		return
	}
	e.mu.Lock()
	s.proc = proc
	id := proc.Identity()
	e.saved.Core = &id
	e.save()
	e.mu.Unlock()
	deadline := e.d.Clock.Now().Add(startWait)
	for !e.d.Core.Events(0).Started {
		switch {
		case exited(proc):
			e.d.Log("setup mode: the core exited before its start line")
			return
		case !e.d.Clock.Now().Before(deadline):
			e.d.Log("setup mode: no start line within %s", startWait)
			return
		}
		if !e.sleep(ctx, 250*time.Millisecond) {
			return
		}
	}
	e.d.Log("setup mode: core started")
	s.node = e.d.Core.Node(p[0], secret)
	since, ready, lastErr := e.d.Clock.Now(), false, ""
	for {
		if err := e.readNode(ctx, s); err != nil {
			// The errors carry the core's command line and its stderr: peers and tailnet addresses.
			if text := protocol.MaskAddresses(err.Error()); text != lastErr && ctx.Err() == nil {
				lastErr = text
				e.d.Log("setup mode: node: %s", text)
			}
		} else {
			lastErr = ""
			if !ready {
				ready = true
				close(s.ready)
			}
		}
		switch {
		case exited(proc):
			e.d.Log("setup mode: the core exited")
			return
		case e.windowless(since):
			e.d.Log("setup mode: no window for %s; stopping it", setupIdle)
			return
		}
		if !e.sleep(ctx, nodeEvery) {
			return
		}
	}
}

// windowless: no window is connected, and the last one went (or, when none went during this run, the
// run started) 5 minutes ago or more (spec 7).
func (e *Engine) windowless(since time.Time) bool {
	if e.clients.Load() > 0 {
		return false
	}
	if gone := time.Unix(0, e.lastGone.Load()); gone.After(since) {
		since = gone
	}
	return !e.d.Clock.Now().Before(since.Add(setupIdle))
}

// readNode is one look at the node of the setup core (spec 7, 5.2): tailscale.backend and authUrl from
// its state. "Logged in" is no login link, an address in the tailnet and the backend starting or
// running (not running only: with the system exit node on, the node of the setup mode stays in
// starting); then the flag and the exit nodes go into state.json, and the state shows that list while
// no core runs. needs_login clears the flag; needs_approval leaves it as it is. A change is published,
// the backend and the list in one state. A look that a logout overtook is dropped.
func (e *Engine) readNode(ctx context.Context, s *setupRun) error {
	e.mu.Lock()
	gen := s.gen
	e.mu.Unlock()
	cctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	st, err := s.node.State(cctx)
	if err != nil {
		return err
	}
	loggedIn := st.AuthURL == "" && st.HasAddress && (st.Backend == protocol.BackendStarting || st.Backend == protocol.BackendRunning)
	var nodes []protocol.ExitNode
	var nodesErr error
	if loggedIn {
		nodes, nodesErr = s.node.ExitNodes(cctx)
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if s.gen != gen {
		return nil
	}
	dirty := e.nodeBackend != st.Backend || e.nodeAuthURL != st.AuthURL
	e.nodeBackend, e.nodeAuthURL = st.Backend, st.AuthURL
	save := false
	switch {
	case loggedIn && !e.saved.LoggedIn:
		e.saved.LoggedIn, save = true, true
	case st.Backend == protocol.BackendNeedsLogin && e.saved.LoggedIn:
		e.saved.LoggedIn, save = false, true
	}
	if loggedIn && nodesErr == nil && !slices.Equal(nodes, e.saved.ExitNodes) {
		e.saved.ExitNodes, save = nodes, true
	}
	if save {
		e.save()
		dirty = true
	}
	if dirty {
		e.publish()
	}
	return nodesErr
}

// logout is tailscale.logout after its answer (spec 7): the setup mode is started unless it runs, its
// API service is waited for (at most 30 s), the node logs out through the core and "logged in" is
// cleared; the list of exit nodes stays, and the setup core keeps running with the node's new login
// link. A failure is logged and the state published: the method has answered. Logouts run one at a
// time, and an enable or helper.shutdown that comes meanwhile waits for this one (stopSetup); an enable
// that began ends the wait at once.
func (e *Engine) logout() {
	e.connector.Lock()
	defer e.connector.Unlock()
	e.mu.Lock()
	e.startSetupLocked()
	e.mu.Unlock()
	deadline := e.d.Clock.Now().Add(startWait)
	var s *setupRun
	for s == nil {
		e.mu.Lock()
		cur, phaseErr := e.setup, e.setupPhaseLocked()
		ending := cur != nil && cur.ending
		if ending {
			e.startSetupLocked() // its core is being stopped: the next run follows it
		}
		e.mu.Unlock()
		if cur == nil || phaseErr != nil {
			e.logoutFailed("the setup mode does not run")
			return
		}
		select {
		case <-cur.ready:
			if !ending {
				s = cur
				continue
			}
		default:
		}
		if !e.d.Clock.Now().Before(deadline) {
			e.logoutFailed(fmt.Sprintf("the API service of the setup core did not answer within %s", startWait))
			return
		}
		e.sleep(context.Background(), 250*time.Millisecond)
	}
	e.mu.Lock()
	s.gen++
	e.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), callTimeout)
	err := s.node.Logout(ctx)
	cancel()
	e.mu.Lock()
	defer e.mu.Unlock()
	s.gen++
	if err != nil {
		e.d.Log("tailscale.logout failed: %s", protocol.MaskAddresses(err.Error()))
		e.publish()
		return
	}
	e.d.Log("tailscale.logout: the node logged out")
	e.saved.LoggedIn = false
	e.save()
	e.publish()
}

func (e *Engine) logoutFailed(why string) {
	e.d.Log("tailscale.logout failed: %s", why)
	e.mu.Lock()
	defer e.mu.Unlock()
	e.publish()
}
