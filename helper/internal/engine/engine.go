package engine

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"splitlane/helper/internal/activity"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// Times of spec 5.3–5.7 and 8.3.
const (
	startWait      = 30 * time.Second       // 5.3 step 4
	proveWait      = 45 * time.Second       // 5.3 step 5
	switchWait     = 45 * time.Second       // 5.3 step 8
	routeWait      = 60 * time.Second       // 5.3 step 1, unattended enable
	restoreEvery   = 5 * time.Second        // 5.4 step 1, 5.7
	restorePending = 30 * time.Second       // 5.4 step 1
	stopWait       = 5 * time.Second        // 5.4 step 2
	adapterWait    = 10 * time.Second       // 5.4 step 3
	applyWindow    = 3 * time.Second        // 8.3
	applyEvery     = 250 * time.Millisecond // 8.3
	healthEvery    = 10 * time.Second       // 5.5
	systemEvery    = 3 * time.Second        // 5.6
	callTimeout    = 20 * time.Second       // one call of tailscale.exe
)

// A state.json that cannot be read at start (an antivirus or a backup holds it for a moment) is read
// again so many times, this far apart, before the helper goes on with the defaults. The start-up of
// 5.7 reads it again for up to stateWait more (every restoreEvery) before it looks for the core left
// from the previous run: the file names that core.
const (
	stateRetries = 3
	stateEvery   = 200 * time.Millisecond
	stateWait    = 30 * time.Second
)

// restartPauses: at most three restarts with a growing pause (5.5).
var restartPauses = []time.Duration{2 * time.Second, 5 * time.Second, 10 * time.Second}

// errCancelled ends an operation that routing.disable or helper.shutdown cancelled; it is never published.
var errCancelled = &protocol.Reason{Code: "cancelled", Params: map[string]string{}}

// errStateLate: a save found state.json readable only now; its settings replaced the change being saved.
var errStateLate = errors.New("state.json could be read only now: the settings it holds are back and this change is not kept")

// Engine is the helper's phase machine.
type Engine struct {
	d Deps

	// queue keeps state-changing commands one at a time, in arrival order (5.1).
	queue sync.Mutex

	mu           sync.Mutex
	st           protocol.State
	saved        store.State
	journal      *store.Journal
	stateErr     error // state.json could not be read (store.ErrIO): nothing is written over it until it reads
	relayout     bool  // a save read state.json only now while a core ran: the core is to be given its layout (save)
	journalErr   error // restore.json could not be read (store.ErrIO): the start-up reads it again first
	run          *coreRun
	op           *op
	degraded     bool
	sysExitOn    bool
	vpnFails     int
	adapterFails int
	leakHits     int
	net          *netWatch    // the sequence after a network change (section 10, network.go); nil when none runs
	clients      atomic.Int32 // connected windows; atomic, so the pipe may set it under its own lock
	lastGone     atomic.Int64 // Clock time (UnixNano) at which the last window went (spec 7)
	windowSeen   atomic.Bool  // a window has connected since the start: no exit without a window (4.3)
	reenabled    bool         // the start-up of 5.7 enabled again because "was on" was set (4.3)
	offAsked     bool         // routing.disable came while the start-up read state.json again: its "was on" does not count
	ending       bool         // the Windows session ends (5.4)
	exiting      bool         // helper.shutdown was accepted
	skip         bool         // "disable without restore" (5.4)
	wake         chan struct{}

	// The proxy connector (stage 3 part 1, proxy.go): servers.json as it was read or last saved. While
	// it could not be read (store.ErrIO) the set is empty, nothing is written over the file, and each
	// proxy command reads it again first.
	servers     store.Servers
	serversErr  error
	downloading bool // a subscription is being downloaded: one at a time, a second one is busy

	// The connector (spec 7, connector.go).
	setup       *setupRun  // the setup mode now; nil when it does not run
	nodeBackend string     // tailscale.backend as the setup core's node says; stopped when it does not run
	nodeAuthURL string     // the login link of the setup core's node, "" when there is none
	connector   sync.Mutex // logouts one at a time; stopSetup waits for the one under way

	act liveActivity // the live activity (spec 8.4, activity.go)

	exporting sync.Mutex // held while diagnostics.export writes: a second one meanwhile is busy (cmdExport)
}

// coreRun is one running core with its ports, the rule files it reads now and its API service.
type coreRun struct {
	proc     CoreProc
	clash    ClashAPI
	node     NodeAPI // the API service of this run (spec 7): the port and the secret of its config
	ports    Ports
	mode     string // the driver's name of the core's mode now
	direct   []byte
	vpn      []byte
	stopping bool // guarded by Engine.mu: the helper stops this core on purpose
	cleared  bool // guarded by Engine.mu: this run has switched the system exit node off (step 6 of 5.3)
	proxy    bool // the lane is a server of the proxy connector: no node, no login, no path (stage 3 part 1, section 5)

	// The node of the core while it runs (spec 7, connector.go).
	calls    sync.WaitGroup     // the calls of node under way; none begins once stopping is set
	callsEnd context.Context    // ends with the run (stopCore, the end of the session), and the calls with it
	endCalls context.CancelFunc // ends callsEnd
	backend  string             // guarded by Engine.mu: tailscale.backend as the node said last (running: the tunnel was proven)
	expired  bool               // guarded by Engine.mu: the node asked for a login again while the core ran
	path     string             // guarded by Engine.mu: the path to the exit node read last, "" while unknown
	pathAt   time.Time          // guarded by Engine.mu: when the path was read
	logMark  int64              // guarded by Engine.mu: the core's log is read for a login request from here
	errs     map[string]string  // guarded by Engine.mu: the last error of each kind of call, masked
}

// op is the one background operation: enabling, disabling, a restart, the start-up,
// systemTailscale.clearExitNode, or routing.allDirect and lanes.set while the core runs.
type op struct {
	kind   string
	cancel context.CancelFunc
	done   chan struct{}
}

// New returns an engine in phase off; Start (start-up of 5.7) is separate.
func New(d Deps) *Engine {
	if d.Log == nil {
		d.Log = func(string, ...any) {}
	}
	if d.OnState == nil {
		d.OnState = func(protocol.State) {}
	}
	if d.OnExit == nil {
		d.OnExit = func() {}
	}
	if d.Fetch == nil {
		d.Fetch = noFetch{}
	}
	e := &Engine{d: d, wake: make(chan struct{}, 1), saved: store.Defaults(), servers: store.EmptyServers(),
		nodeBackend: protocol.BackendStopped, act: liveActivity{tracker: activity.New(d.Rules)}}
	e.st = protocol.State{
		Phase:       protocol.PhaseOff,
		DefaultLane: protocol.LaneVPN,
		Apps:        []protocol.AppRule{},
		Tailscale:   protocol.Tailscale{Backend: protocol.BackendStopped, ExitNodes: []protocol.ExitNode{}},
	}
	e.syncSaved()
	return e
}

// load reads state.json and restore.json into the engine. A state.json that cannot be read
// (store.ErrIO) is read again three times 200 ms apart; if it still fails, the defaults stay in
// memory and save writes nothing over the file until it reads. A journal that cannot be read is
// not "no journal": the start-up reads it again before anything else (readJournal). A journal with
// unreadable content has been set aside by the store: there is none.
func (e *Engine) load() {
	s, err := e.d.Store.LoadState()
	for n := 0; n < stateRetries && errors.Is(err, store.ErrIO); n++ {
		<-e.d.Clock.After(stateEvery)
		s, err = e.d.Store.LoadState()
	}
	var stateErr error
	switch {
	case errors.Is(err, store.ErrIO):
		stateErr = err
		e.d.Log("state.json: %v; starting from defaults, the file is not written over until it can be read", err)
	case err != nil:
		e.d.Log("state.json: %v; starting from defaults", err)
	}
	j, err := e.d.Store.LoadJournal()
	var journalErr error
	switch {
	case errors.Is(err, store.ErrIO):
		journalErr = err
		e.d.Log("restore journal: %v; it is read again before anything else", err)
	case err != nil:
		e.d.Log("restore journal: %v", err)
	}
	servers, serversErr := e.d.Store.LoadServers()
	switch {
	case errors.Is(serversErr, store.ErrIO):
		e.d.Log("servers.json: %v; it is read again by the next proxy command, and not written over until then", serversErr)
	case serversErr != nil:
		e.d.Log("servers.json: %v", serversErr)
		serversErr = nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.saved, e.journal = s, j
	e.stateErr, e.journalErr = stateErr, journalErr
	e.servers, e.serversErr = servers, serversErr
	e.syncSaved()
}

// readJournal begins the start-up (5.7) when restore.json could not be read: the journal may be
// there, so the helper does not go on as if there were none. The phase is stopping with reason
// restore_pending (the read error; no command: the exit node is unknown until the journal reads),
// and the journal is read again every 5 s, or at once on "Повторить", until it reads. "Disable
// without restore" (5.4) gives it up: the journal is deleted as far as it can be and the start-up
// goes on without it.
func (e *Engine) readJournal() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.journalErr == nil {
		return
	}
	for e.journalErr != nil {
		if e.skip {
			e.skip, e.journalErr = false, nil
			e.dropJournal()
			e.d.Log("restore skipped by the user: the unread journal deleted, system exit node left as is")
			return
		}
		e.setPhase(protocol.PhaseStopping, reason(protocol.ReasonRestorePending,
			protocol.ParamCommand, "", protocol.ParamError, e.journalErr.Error()))
		e.mu.Unlock()
		select {
		case <-e.wake:
		case <-e.d.Clock.After(restoreEvery):
		}
		j, err := e.d.Store.LoadJournal()
		e.mu.Lock()
		switch {
		case errors.Is(err, store.ErrIO):
			e.journalErr = err
		case err != nil:
			e.d.Log("restore journal: %v", err)
			e.journal, e.journalErr = nil, nil
		default:
			e.journal, e.journalErr = j, nil
		}
	}
	e.d.Log("restore journal read")
	e.setPhase(protocol.PhaseStopping, nil)
}

// readState is the step of the start-up (5.7) before the core left from the previous run is looked
// for, when state.json could not be read (load): the file names that core and holds "was on". The
// phase is stopping, and the file is read again now, then every 5 s for at most 30 s; once it reads,
// its settings replace the defaults. If it still cannot be read, or the start-up is cancelled, the
// start-up goes on with the defaults: no core of the previous run is known, none is stopped, and the
// first save that reads the file keeps the identity it holds (save).
func (e *Engine) readState(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stateErr == nil {
		return
	}
	e.setPhase(protocol.PhaseStopping, nil)
	deadline := e.d.Clock.Now().Add(stateWait)
	for {
		e.mu.Unlock()
		s, err := e.d.Store.LoadState()
		e.mu.Lock()
		switch {
		case e.stateErr == nil:
			return // a save has read it meanwhile and taken its settings over
		case !errors.Is(err, store.ErrIO):
			if err != nil {
				e.d.Log("state.json: %v", err)
			}
			e.d.Log("state.json can be read now: its settings are back")
			if e.offAsked || e.exiting || e.ending {
				s.WasOn = false // the off that came during the read is kept: no enable again
			}
			e.saved, e.stateErr = s, nil
			e.publish()
			return
		}
		e.stateErr = err
		if !e.d.Clock.Now().Before(deadline) {
			e.d.Log("state.json still cannot be read after %s: no core of the previous run is looked for", stateWait)
			return
		}
		e.mu.Unlock()
		ok := e.sleep(ctx, restoreEvery)
		e.mu.Lock()
		if !ok {
			return
		}
	}
}

// State is a copy of the current state (5.2).
func (e *Engine) State() protocol.State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.st.Clone()
}

// Hello is the answer to hello (5.1).
func (e *Engine) Hello() protocol.HelloResult {
	return protocol.HelloResult{Protocol: protocol.Version, Helper: e.d.Settings.HelperVersion, Core: e.d.Settings.CoreVersion}
}

// SetClients tells how many windows are connected (5.6: the system client is polled for them; spec 7:
// the first window that comes starts the setup mode when "logged in" is saved, and the setup mode
// stops 5 minutes after the last one went; spec 4.3: a helper that a window has reached does not end
// on its own). The pipe calls it under its own lock, which a published state takes after e.mu, so it
// takes no lock of the engine: the start runs on its own goroutine. The time the last window went is
// stored before the count shows no window, so windowless never pairs no window with the time an
// earlier one went; the pipe's lock keeps other calls out between the two.
func (e *Engine) SetClients(n int) {
	if n > 0 {
		e.windowSeen.Store(true)
	} else if e.clients.Load() > 0 {
		e.lastGone.Store(e.d.Clock.Now().UnixNano())
	}
	if old := e.clients.Swap(int32(n)); n > 0 && old == 0 {
		go e.windowCame()
	}
}

// Wait blocks until no background operation runs.
func (e *Engine) Wait() {
	for {
		e.mu.Lock()
		o := e.op
		e.mu.Unlock()
		if o == nil {
			return
		}
		<-o.done
	}
}

// startOp runs fn in the background; e.mu must be held. It refuses while another operation runs.
func (e *Engine) startOp(kind string, fn func(ctx context.Context)) bool {
	if e.op != nil {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	o := &op{kind: kind, cancel: cancel, done: make(chan struct{})}
	e.op = o
	e.skip = false
	go func() {
		defer func() {
			e.mu.Lock()
			if e.op == o {
				e.op = nil
			}
			e.mu.Unlock()
			cancel()
			close(o.done)
		}()
		fn(ctx)
	}()
	return true
}

// syncSaved copies the saved settings into the published state; e.mu must be held.
func (e *Engine) syncSaved() {
	e.st.DefaultLane = e.saved.DefaultLane
	e.st.Apps = append([]protocol.AppRule{}, e.saved.Apps...)
	e.st.Tailscale.ExitNodes = append([]protocol.ExitNode{}, e.saved.ExitNodes...)
	e.st.Tailscale.SelectedExitNode = nil
	if e.saved.SelectedExitNode != "" {
		v := e.saved.SelectedExitNode
		e.st.Tailscale.SelectedExitNode = &v
	}
	e.st.Tailscale.DeviceName = e.hostname()
	e.st.SystemTailscale.RestoreOnDisable = e.journal != nil || e.journalErr != nil
	e.syncProxy()
}

func (e *Engine) hostname() string {
	if e.saved.DeviceName != "" {
		return e.saved.DeviceName
	}
	return e.d.Settings.Hostname
}

// save writes state.json; e.mu must be held. A failed write leaves the engine's copy as it is, and
// the next save writes all of it again, "was on" and the core identity with it. While state.json
// could not be read (load), nothing is written over it: each save reads it again first. Once it
// reads, the settings it holds are taken over, "was on" stays that of this run, and so does the core
// identity once this run has started a core; until then the file's stays: the core it names may still
// run, and the next start must find it (5.7). While a core runs, the connector, the exit node and the
// server it was built with and this run's "logged in" stay too, and the core is to be given the layout of the file (relayout:
// lanes.set at once, otherwise the next round of 5.5). save then returns errStateLate: the change
// being saved has given way to the file.
func (e *Engine) save() error {
	late := false
	if e.stateErr != nil {
		s, err := e.d.Store.LoadState()
		if errors.Is(err, store.ErrIO) {
			e.stateErr = err
			e.d.Log("state.json not saved: it still cannot be read and is not written over: %v", err)
			return err
		}
		if err != nil {
			e.d.Log("state.json: %v", err)
		}
		e.d.Log("state.json can be read now: its settings are back")
		s.WasOn = e.saved.WasOn
		if e.saved.Core != nil {
			s.Core = e.saved.Core
		}
		if e.run != nil {
			s.SelectedExitNode, s.LoggedIn = e.saved.SelectedExitNode, e.saved.LoggedIn
			s.Connector, s.SelectedServer = e.saved.Connector, e.saved.SelectedServer
			e.relayout = true
		}
		e.saved, e.stateErr, late = s, nil, true
	}
	err := e.d.Store.SaveState(e.saved)
	if err != nil {
		e.d.Log("state.json not saved: %v", err)
	}
	if late {
		return errStateLate
	}
	return err
}

// publish sends the state to the clients; e.mu must be held. tailscale.backend, authUrl and path come
// from the connector (spec 7, 5.2). While the main core of the Tailscale connector runs in a live phase:
// its node (expired once it asked for a login again), no login link (the setup mode gives it), the path
// read last. Otherwise the node of the setup core with its link, stopped when no core runs, and path null;
// a core of the proxy connector has no node (stage 3 part 1, section 5): stopped, no link, path null.
func (e *Engine) publish() {
	e.syncSaved()
	ts := &e.st.Tailscale
	ts.Backend, ts.AuthURL, ts.Path = e.nodeBackend, nil, nil
	switch r := e.run; {
	case r != nil && live(e.st.Phase) && r.proxy:
	case r != nil && live(e.st.Phase):
		ts.Backend = protocol.BackendRunning
		switch {
		case r.expired:
			ts.Backend = protocol.BackendExpired
		case r.backend != "":
			ts.Backend = r.backend
		}
		if r.path != "" {
			p := r.path
			ts.Path = &p
		}
	case e.nodeAuthURL != "":
		u := e.nodeAuthURL
		ts.AuthURL = &u
	}
	e.d.OnState(e.st.Clone())
}

// setPhase changes phase and reason and publishes; e.mu must be held.
func (e *Engine) setPhase(phase string, why *protocol.Reason) {
	e.st.Phase, e.st.Reason = phase, why
	e.publish()
}

// finish ends in off or error: "was on" cleared, all direct reset (5.2); e.mu must be held.
// A network change that was followed is over with the core.
func (e *Engine) finish(phase string, why *protocol.Reason) {
	e.saved.WasOn = false
	e.save()
	e.st.AllDirect = false
	e.st.Addresses = protocol.Addresses{}
	e.degraded, e.sysExitOn = false, false
	e.vpnFails, e.adapterFails, e.leakHits = 0, 0, 0
	e.net = nil
	e.setPhase(phase, why)
}

// finishOn is step 9 of 5.3: phase on, "was on" set; e.mu must be held. A network change that was
// followed is over: the core is a new one.
func (e *Engine) finishOn() {
	e.saved.WasOn = true
	e.save()
	e.degraded, e.sysExitOn = false, false
	e.vpnFails, e.adapterFails, e.leakHits = 0, 0, 0
	e.net = nil
	e.setPhase(protocol.PhaseOn, nil)
}

// dropJournal removes restore.json; e.mu must be held.
func (e *Engine) dropJournal() {
	if err := e.d.Store.DeleteJournal(); err != nil {
		e.d.Log("restore journal not deleted: %v", err)
	}
	e.journal = nil
	if e.st.Reason != nil && e.st.Reason.Code == protocol.ReasonRestorePending {
		e.st.Reason = nil
	}
	e.publish()
}

func live(phase string) bool {
	return phase == protocol.PhaseOn || phase == protocol.PhaseDegraded || phase == protocol.PhaseBlocked
}

// localDNS: names resolve through the DNS of the physical adapter (spec 6.4), in degraded and in
// "all direct"; otherwise through the encrypted DNS of the VPN lane.
func localDNS(allDirect, degraded bool) bool { return allDirect || degraded }

// flushDNS empties the Windows DNS cache: at step 7 of 5.3 and at every switch of the DNS, both
// ways (6.4); the core clears only its own cache when the mode changes.
func (e *Engine) flushDNS() {
	if err := e.d.Host.FlushDNS(); err != nil {
		e.d.Log("flush DNS: %v", err)
	}
}

// reason builds a Reason from a code and key/value pairs.
func reason(code string, kv ...string) *protocol.Reason {
	r := &protocol.Reason{Code: code, Params: map[string]string{}}
	for i := 0; i+1 < len(kv); i += 2 {
		r.Params[kv[i]] = kv[i+1]
	}
	return r
}

// sleep waits d or until ctx ends; false when ctx ended.
func (e *Engine) sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-ctx.Done():
		return false
	case <-e.d.Clock.After(d):
		return ctx.Err() == nil
	}
}

func exited(p CoreProc) bool {
	select {
	case <-p.Done():
		return true
	default:
		return false
	}
}
