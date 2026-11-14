package engine

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
)

// loginURL is the login link the fake node offers while it needs a login.
const loginURL = "https://login.example/a/1f2e3d"

// shutdown is helper.shutdown and its end: the setup mode is stopped before the process ends (spec 7).
// Each test of the connector ends with it, so no setup core outlives its test.
func shutdown(t *testing.T, h *harness) {
	t.Helper()
	if err := call(h, protocol.MethodShutdown, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor("exit", func() bool { return exitedNow(h) })
}

// gateCore holds the check of the setup config until release is closed (or its context ends);
// entered is closed when the first such check came. The checks of the main core go through at once.
type gateCore struct {
	*fakeCore
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *gateCore) Check(ctx context.Context, path string) error {
	c.fakeCore.mu.Lock()
	setup := c.fakeCore.setupNext
	c.fakeCore.mu.Unlock()
	if setup {
		c.once.Do(func() { close(c.entered) })
		select {
		case <-c.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return c.fakeCore.Check(ctx, path)
}

// gateSetup puts a gateCore in front of the fake core: the setup core stays "still starting".
func gateSetup(h *harness) *gateCore {
	g := &gateCore{fakeCore: h.core, entered: make(chan struct{}), release: make(chan struct{})}
	h.e.d.Core = g
	return g
}

func waitClosed(t *testing.T, ch chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}

// backendIs: the state shows this tailscale.backend now.
func backendIs(h *harness, backend string) bool { return h.e.State().Tailscale.Backend == backend }

// setupRunning is a logged-in helper in phase off to which a window has connected: the setup core
// runs and its node has been read (backend running).
func setupRunning(t *testing.T) (*harness, *fakeProc) {
	t.Helper()
	h := newHarness(t)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	h.e.SetClients(1)
	h.waitFor("the setup core's node read", func() bool { return backendIs(h, protocol.BackendRunning) })
	return h, h.core.last()
}

// tailscale.login in off (spec 7, 5.1): the answer comes at once, before the core starts; the setup
// core starts without an adapter from its own config: the API port from FreePorts, the secret, the log
// level of Settings; the config is checked, the identity goes into state.json (5.7). The phase stays
// off; the node's state and its login link reach the state. A window alone starts nothing while the
// node is not logged in.
func TestLoginStartsSetupMode(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false
	h.e.d.Settings.CoreLogLevel = "debug"
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	h.e.SetClients(1)
	time.Sleep(20 * time.Millisecond)
	if len(h.rec.list()) != 0 {
		t.Fatalf("a window starts no setup mode before a login: %v", h.rec.list())
	}
	g := gateSetup(h)
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, g.entered, "the check of the setup config")
	if h.rec.count("core:start") != 0 {
		t.Fatalf("the answer comes before the core starts: %v", h.rec.list())
	}
	close(g.release)
	h.waitFor("the login link", func() bool {
		u := h.e.State().Tailscale.AuthURL
		return u != nil && *u == loginURL
	})
	if s := h.e.State(); s.Phase != protocol.PhaseOff || s.Tailscale.Backend != protocol.BackendNeedsLogin || strings.Join(h.phases(), ",") != "off" {
		t.Fatalf("phase off throughout, backend needs_login: %+v, phases %v", s, h.phases())
	}
	if !inOrder(h, 0, "core:setupconfig", "core:check", "core:start 1000 setup") || h.rec.index("host:removestale") >= 0 || h.rec.index("sys:") >= 0 {
		t.Fatalf("setup config, check, start; no adapter, the system untouched: %v", h.rec.list())
	}
	h.core.mu.Lock()
	o := h.core.setupOpts
	h.core.mu.Unlock()
	if o != (SetupOptions{Hostname: "PC-splitlane", APIPort: portVPN, APISecret: h.host.Secret(), LogLevel: "debug"}) {
		t.Fatalf("setup options %+v", o)
	}
	if c := h.store.saved().Core; c == nil || *c != h.core.last().id {
		t.Fatalf("the identity of the setup core in state.json: %+v", c)
	}
}

// Review Focus 5: tailscale.login again while the setup core runs and its link is there: no second
// core, the same link, no error.
func TestLoginTwiceKeepsOneSetupCore(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the login link", func() bool { return h.e.State().Tailscale.AuthURL != nil })
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatalf("a second login is no error: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	u := h.e.State().Tailscale.AuthURL
	if h.rec.count("core:start") != 1 || h.rec.count("core:setupconfig") != 1 || u == nil || *u != loginURL || !h.core.last().alive() {
		t.Fatalf("one setup core, the same link: %v %v", u, h.rec.list())
	}
}

// Review Focus 1: a window connects and disconnects several times while the setup core is still
// starting: one setup core runs. The 5 minutes count from the last disconnect: a new connection
// resets them, and the core stops 5 minutes after the window that went last. The core never
// outlives its run.
func TestSetupModeSurvivesReconnects(t *testing.T) {
	h := newHarness(t)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	g := gateSetup(h)
	h.e.SetClients(1)
	waitClosed(t, g.entered, "the check of the setup config")
	for i := 0; i < 3; i++ {
		h.e.SetClients(0)
		h.e.SetClients(1)
	}
	close(g.release)
	h.waitFor("the setup core's node read", func() bool { return backendIs(h, protocol.BackendRunning) })
	setup := h.core.last()
	h.e.SetClients(0)
	first := h.clock.Now() // read after: the disconnect is not later than this
	h.e.SetClients(1)
	h.waitFor("7 minutes with a window", func() bool { return h.clock.Now().Sub(first) > 7*time.Minute })
	if !setup.alive() || h.rec.count("core:start") != 1 {
		t.Fatalf("the reconnect resets the 5 minutes; one setup core: %v", h.rec.list())
	}
	last := h.clock.Now() // read first: the disconnect is not earlier than this
	h.e.SetClients(0)
	h.waitFor("the setup core stopped", func() bool { return !setup.alive() && backendIs(h, protocol.BackendStopped) })
	if waited := h.clock.Now().Sub(last); waited < 5*time.Minute || h.rec.count("core:start") != 1 || h.core.anyAlive() {
		t.Fatalf("stopped %s after the last window went; events %v", waited, h.rec.list())
	}
}

// Spec 7: the setup core stops 5 minutes after the last window disconnected, gracefully (step 2 of
// 5.4); the saved login and exit nodes stay, the state shows backend stopped.
func TestSetupStopsFiveMinutesAfterLastWindow(t *testing.T) {
	h, setup := setupRunning(t)
	gone := h.clock.Now() // read first: the disconnect is not earlier than this
	h.e.SetClients(0)
	h.waitFor("the setup core stopped", func() bool { return !setup.alive() && backendIs(h, protocol.BackendStopped) })
	if waited := h.clock.Now().Sub(gone); waited < 5*time.Minute || waited > 6*time.Minute {
		t.Fatalf("stopped %s after the last window went", waited)
	}
	if h.rec.index(fmt.Sprintf("core:interrupt %d", setup.id.PID)) < 0 || h.rec.index("core:kill") >= 0 {
		t.Fatalf("the graceful signal: %v", h.rec.list())
	}
	if s := h.store.saved(); !s.LoggedIn || len(s.ExitNodes) != 1 || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("state.json %+v, phase %s", s, h.e.State().Phase)
	}
}

// Spec 7 and 5.3 step 1: the first action of an enable stops the setup core and waits for its end;
// only then the preflight runs, so its check for another core does not see it (the fake host counts a
// setup core as another sing-box.exe), and the main core starts after it. A setup core that is still
// starting is stopped the same way and never runs beside the main core.
func TestEnableStopsSetupFirst(t *testing.T) {
	h, setup := setupRunning(t)
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
	stop, start := h.rec.index(fmt.Sprintf("core:interrupt %d", setup.id.PID)), h.rec.index("core:start 1001")
	if setup.alive() || stop < 0 || start < stop || h.sawReason(protocol.ReasonOtherCore) {
		t.Fatalf("the setup core ends before the main core starts: %v", h.rec.list())
	}
	if got := strings.Join(h.phases(), ","); got != "off,starting,on" {
		t.Fatalf("phases %s", got)
	}

	h = newHarness(t)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	g := gateSetup(h)
	h.e.SetClients(1)
	waitClosed(t, g.entered, "the check of the setup config")
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
	time.Sleep(20 * time.Millisecond)
	if h.rec.count("core:start") != 1 || h.rec.index("core:start 1000 setup") >= 0 || h.core.setupAlive() {
		t.Fatalf("the starting setup core is given up, only the main core runs: %v", h.rec.list())
	}
}

// Spec 7: "logged in" is no login link, an address in the tailnet and the backend starting or running
// (starting counts: with the system exit node on, the node of the setup mode stays there). Then the
// flag and the exit nodes go into state.json; the state shows them while no core runs, and they survive
// a restart of the helper.
func TestLoggedInIsSaved(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	if err := call(h, protocol.MethodLogin, nil); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the login link", func() bool { return h.e.State().Tailscale.AuthURL != nil })
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendStarting} })
	h.waitFor("starting without a link", func() bool {
		s := h.e.State().Tailscale
		return s.Backend == protocol.BackendStarting && s.AuthURL == nil
	})
	if h.store.saved().LoggedIn {
		t.Fatal("no address in the tailnet yet: not logged in")
	}
	nodes := []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: true}, {ID: "100.64.0.2", Name: "exit-b", Online: false}}
	h.core.node.set(func(n *fakeNode) {
		n.state = NodeState{Backend: protocol.BackendStarting, HasAddress: true}
		n.nodes = nodes
	})
	h.waitFor("logged in and the exit nodes saved", func() bool {
		s := h.store.saved()
		return s.LoggedIn && slices.Equal(s.ExitNodes, nodes) && slices.Equal(h.e.State().Tailscale.ExitNodes, nodes)
	})
	shutdown(t, h)
	next := newHarness(t)
	next.store.state = h.store.saved()
	next.e.Start()
	next.waitPhase(protocol.PhaseOff)
	if s := next.e.State().Tailscale; !slices.Equal(s.ExitNodes, nodes) || s.Backend != protocol.BackendStopped || !next.store.saved().LoggedIn {
		t.Fatalf("after a restart: %+v, logged in %v", s, next.store.saved().LoggedIn)
	}
}

// Spec 7: the node of the setup core needs a login (the key expired, the device was removed in the
// admin console): "logged in" goes from state.json, the last list of exit nodes stays, the state shows
// needs_login with the new link.
func TestNeedsLoginClearsLoggedIn(t *testing.T) {
	h, _ := setupRunning(t)
	list := h.store.saved().ExitNodes
	if len(list) != 1 {
		t.Fatalf("the exit nodes of the logged-in node are saved: %+v", list)
	}
	h.core.node.set(func(n *fakeNode) { n.state = NodeState{Backend: protocol.BackendNeedsLogin, AuthURL: loginURL} })
	h.waitFor("logged in cleared", func() bool { return !h.store.saved().LoggedIn })
	h.waitFor("needs_login with the link", func() bool {
		s := h.e.State().Tailscale
		return s.Backend == protocol.BackendNeedsLogin && s.AuthURL != nil && *s.AuthURL == loginURL
	})
	if got := h.store.saved().ExitNodes; !slices.Equal(got, list) || !slices.Equal(h.e.State().Tailscale.ExitNodes, list) {
		t.Fatalf("the list stays: %+v", got)
	}
}

// tailscale.logout in off (spec 7): the answer comes at once; the setup mode starts because it does not
// run, its API service is waited for, the node logs out through the core and "logged in" goes; the list
// of exit nodes stays and the setup core keeps running (the new login link comes from it).
func TestLogoutClearsLoggedIn(t *testing.T) {
	h := newHarness(t)
	list := []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: true}}
	h.store.state.ExitNodes = list
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	g := gateSetup(h)
	if err := call(h, protocol.MethodLogout, nil); err != nil {
		t.Fatal(err)
	}
	waitClosed(t, g.entered, "the check of the setup config")
	if h.rec.count("node:logout") != 0 || !h.store.saved().LoggedIn {
		t.Fatalf("the answer comes before the logout: %v", h.rec.list())
	}
	close(g.release)
	h.waitFor("logged out", func() bool { return h.rec.count("node:logout") == 1 && !h.store.saved().LoggedIn })
	h.waitFor("needs_login", func() bool { return backendIs(h, protocol.BackendNeedsLogin) })
	if !inOrder(h, 0, "core:start 1000 setup", "node:logout") || !h.core.last().alive() || h.e.State().Phase != protocol.PhaseOff {
		t.Fatalf("setup core first, it keeps running, phase off: %v", h.rec.list())
	}
	if got := h.store.saved().ExitNodes; !slices.Equal(got, list) {
		t.Fatalf("the list stays: %+v", got)
	}
	time.Sleep(20 * time.Millisecond)
	if h.store.saved().LoggedIn {
		t.Fatal("a look at the node from before the logout must not set it again")
	}
}

// The errors of the API service carry the core's command line and its standard error, which may name
// peers and addresses in the tailnet: the log gets them masked (<ip>), never as they are. A failed
// logout changes nothing.
func TestNodeErrorsAreMasked(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	h.e.SetClients(1)
	h.waitFor("the setup core's node read", func() bool { return backendIs(h, protocol.BackendRunning) })
	h.core.node.set(func(n *fakeNode) {
		n.err = errors.New("sing-box.exe api tailscale status: exit status 1: dial tcp 100.64.0.7:41641: connection refused")
	})
	if err := call(h, protocol.MethodLogout, nil); err != nil {
		t.Fatal(err)
	}
	logged := func(prefix string) string {
		for _, ev := range h.rec.list() {
			if strings.HasPrefix(ev, prefix) {
				return ev
			}
		}
		return ""
	}
	h.waitFor("both errors logged", func() bool {
		return logged("log:setup mode: node: ") != "" && logged("log:tailscale.logout failed: ") != ""
	})
	for _, prefix := range []string{"log:setup mode: node: ", "log:tailscale.logout failed: "} {
		if ev := logged(prefix); !strings.Contains(ev, "dial tcp <ip>:41641") {
			t.Errorf("masked: %q", ev)
		}
	}
	for _, ev := range h.rec.list() {
		if strings.Contains(ev, "100.64.0.7") {
			t.Fatalf("an address in the log: %q", ev)
		}
	}
	if !h.store.saved().LoggedIn {
		t.Fatal("a failed logout changes nothing")
	}
}

// helper.shutdown in off (spec 7): the setup core is stopped as in step 2 of 5.4 (the graceful
// signal, its end waited for) before the process ends.
func TestShutdownStopsSetup(t *testing.T) {
	h, setup := setupRunning(t)
	orig := h.e.d.OnExit
	h.e.d.OnExit = func() {
		h.rec.add("helper:exit")
		orig()
	}
	shutdown(t, h)
	stop, exit := h.rec.index(fmt.Sprintf("core:interrupt %d", setup.id.PID)), h.rec.index("helper:exit")
	if setup.alive() || stop < 0 || exit < stop {
		t.Fatalf("the setup core ends before the helper: %v", h.rec.list())
	}
}

// The end of the Windows session (5.4): the setup core is ended at once, without waiting.
func TestSessionEndKillsSetup(t *testing.T) {
	h, setup := setupRunning(t)
	h.e.SessionEnding()
	if setup.alive() || h.rec.index(fmt.Sprintf("core:kill %d", setup.id.PID)) < 0 || h.rec.index("core:interrupt") >= 0 {
		t.Fatalf("killed without the graceful signal: %v", h.rec.list())
	}
	h.waitFor("backend stopped", func() bool { return backendIs(h, protocol.BackendStopped) })
}

// 5.7: the identity of the setup core goes into state.json like the main core's. A helper that ended
// with the setup core running (a crash) leaves it behind; the next one stops it at its start-up, in
// phase stopping, and ends in off.
func TestLeftoverSetupCoreIsStopped(t *testing.T) {
	h, setup := setupRunning(t)
	saved := h.store.saved()
	if saved.Core == nil || *saved.Core != setup.id {
		t.Fatalf("the identity of the setup core in state.json: %+v", saved.Core)
	}
	next := newHarness(t)
	next.store.state = saved
	next.core.leftover = setup
	next.e.Start()
	next.waitPhase(protocol.PhaseOff)
	if setup.alive() || strings.Join(next.phases(), ",") != "stopping,off" || next.store.saved().Core != nil {
		t.Fatalf("the leftover setup core stopped in stopping: phases %v, events %v", next.phases(), h.rec.list())
	}
	if h.rec.index(fmt.Sprintf("core:interrupt %d", setup.id.PID)) < 0 {
		t.Fatalf("stopped with the graceful signal: %v", h.rec.list())
	}
	h.waitFor("the first helper sees its setup core gone", func() bool { return backendIs(h, protocol.BackendStopped) })
}

// TestLoginPhaseTable walks the cells of 5.1 for tailscale.login and tailscale.logout in off, error,
// starting, on, blocked and stopping: in off and error the setup mode starts (login) or the node logs
// out (logout) and the phase stays; starting and stopping are busy; on and blocked are wrong_phase.
func TestLoginPhaseTable(t *testing.T) {
	busy, wrong := protocol.CodeBusy, protocol.CodeWrongPhase
	phases := []string{protocol.PhaseOff, protocol.PhaseError, protocol.PhaseStarting, protocol.PhaseOn,
		protocol.PhaseBlocked, protocol.PhaseStopping}
	rows := []struct {
		name   string
		method string
		want   []string // off, error, starting, on, blocked, stopping
		act    func(t *testing.T, h *harness)
	}{
		{"login", protocol.MethodLogin, []string{cellAct, cellAct, busy, wrong, wrong, busy},
			func(t *testing.T, h *harness) {
				h.waitFor("the setup core's node read", func() bool {
					return h.rec.index("core:start 1000 setup") >= 0 && backendIs(h, protocol.BackendRunning)
				})
			}},
		{"logout", protocol.MethodLogout, []string{cellAct, cellAct, busy, wrong, wrong, busy},
			func(t *testing.T, h *harness) {
				h.waitFor("logged out", func() bool { return h.rec.count("node:logout") == 1 && !h.store.saved().LoggedIn })
			}},
	}
	for _, row := range rows {
		for i, phase := range phases {
			t.Run(row.name+"/"+phase, func(t *testing.T) {
				h, release := inPhase(t, phase)
				defer release()
				t.Cleanup(func() { shutdown(t, h) })
				before := h.rec.list()
				err := call(h, row.method, nil)
				switch want := row.want[i]; want {
				case cellAct:
					if err != nil {
						t.Fatalf("answer %v", err)
					}
					row.act(t, h)
					if got := h.e.State().Phase; got != phase {
						t.Fatalf("the phase stays %s, got %s", phase, got)
					}
				default:
					wantCode(t, err, want)
					time.Sleep(20 * time.Millisecond)
					if strings.Join(h.rec.list(), "|") != strings.Join(before, "|") || h.e.State().Phase != phase {
						t.Fatalf("nothing happens: phase %s, events %v", h.e.State().Phase, h.rec.list())
					}
				}
			})
		}
	}
}
