package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/connector"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// proxyEnabled is a helper in phase on with the proxy connector after 5.3: the servers of links, the
// first selected; logged in to Tailscale with an exit node selected, so that nothing but the connector
// decides the lane.
func proxyEnabled(t *testing.T, links ...string) *harness {
	t.Helper()
	h := newHarness(t)
	h.load()
	mustAdd(h, strings.Join(links, "\n"), "")
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	return h
}

// nodeUntouched: neither the API service of a core nor a setup core was asked for anything.
func nodeUntouched(t *testing.T, h *harness) {
	t.Helper()
	looks, lists := h.core.node.calls()
	h.core.mu.Lock()
	port := h.core.nodePort
	h.core.mu.Unlock()
	if looks != 0 || len(lists) != 0 || port != 0 || h.rec.index("core:setupconfig") >= 0 || h.rec.count("core:start 1000 setup") != 0 ||
		h.core.setupAlive() {
		t.Fatalf("the node or the setup mode was touched: looks %d, lists %d, port %d, events %v", looks, len(lists), port, h.rec.list())
	}
}

// Stage 3 part 1, section 5: enabling with the proxy connector builds the lane from the selected server,
// parsed again from its text in servers.json; nothing Tailscale-specific runs: no setup mode (not even when
// a window comes with "logged in" saved), no call of the node, no login or path; tailscale.backend stays
// stopped and the path null. The system Tailscale client is handled as for every connector (5.6, 5.7).
func TestEnableWithProxy(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.load()
	t.Cleanup(func() { shutdown(t, h) })
	mustAdd(h, linkVLESS+"\n"+linkTrojan, "")
	h.e.SetClients(1)
	time.Sleep(30 * time.Millisecond)
	nodeUntouched(t, h)
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	o := h.core.options()
	if o.Lane.Tailscale != nil || o.Lane.Proxy == nil || o.Lane.Proxy.ID != serverID(linkVLESS) || o.Lane.Proxy.Text != linkVLESS ||
		o.Lane.Proxy.Protocol != "vless" || o.Lane.Proxy.VLESS == nil || o.Lane.Proxy.VLESS.UUID != uuidA {
		t.Fatalf("lane %+v", o.Lane)
	}
	if !inOrder(h, 0, "core:check", "core:start", "journal:save 100.64.0.1 true", "sys:clear", "dns:flush") {
		t.Fatalf("5.3 in order, the system exit node off after the proof: %v", h.rec.list())
	}
	h.e.checkNode(context.Background())
	h.e.refreshNode(context.Background())
	rounds(h, 2)
	if p := h.e.readPath(context.Background()); p != "" {
		t.Fatalf("path %q", p)
	}
	h.e.NetworkChanged("route")
	rounds(h, 2)
	nodeUntouched(t, h)
	s := h.e.State()
	if s.Phase != protocol.PhaseOn || s.Connector != protocol.ConnectorProxy || s.Tailscale.Backend != protocol.BackendStopped ||
		s.Tailscale.Path != nil || s.Tailscale.AuthURL != nil || h.rec.count("core:start") != 1 {
		t.Fatalf("state %+v %+v", s.Phase, s.Tailscale)
	}
	h.e.mu.Lock()
	followed := h.e.net
	h.e.mu.Unlock()
	if followed != nil {
		t.Fatal("the checks passed after the network change: the sequence is over without a path to wait for")
	}
	noSecrets(t, h)
}

// The login line of the core's log belongs to the embedded node: with the proxy connector it is never a
// login_expired, neither at the start of the core nor while it runs.
func TestProxyRunIgnoresTheLoginLine(t *testing.T) {
	h := newHarness(t)
	h.core.needLogin = true
	h.load()
	mustAdd(h, linkTrojan, "")
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.core.appendLog("INFO endpoint/tailscale[vpn]: Waiting for authentication: https://login.example/x")
	h.e.checkNode(context.Background())
	rounds(h, 1)
	if s := h.e.State(); s.Phase != protocol.PhaseOn || s.Reason != nil || !h.store.saved().LoggedIn {
		t.Fatalf("state %s %+v, logged in %v", s.Phase, s.Reason, h.store.saved().LoggedIn)
	}
}

// 5.3 step 1 for the proxy connector (stage 3 part 1, section 4): no_server when no server is selected,
// when the selected one is not in servers.json, when its text does not parse any more or when servers.json
// cannot be read; not_logged_in and no_exit_node are Tailscale's and do not count. The system stays untouched.
func TestPreflightNoServer(t *testing.T) {
	broken := store.StoredServer{ID: "0123456789abcdef", Name: "x", Protocol: "vless", Source: "manual", Text: "vless://not-a-uuid@example.com:443"}
	cases := map[string]func(h *harness){
		"none selected": func(h *harness) {},
		"not in servers.json": func(h *harness) {
			h.store.state.SelectedServer = serverID(linkTrojan)
		},
		"does not parse": func(h *harness) {
			h.store.servers.Servers = []store.StoredServer{broken}
			h.store.state.SelectedServer = broken.ID
		},
		"servers.json unreadable": func(h *harness) {
			h.store.servers.Servers = []store.StoredServer{storedServerOf(t, linkTrojan)}
			h.store.state.SelectedServer = serverID(linkTrojan)
			h.store.serversReadFails = 100
		},
	}
	for name, change := range cases {
		h := newHarness(t)
		logEvents(h)
		h.store.state.LoggedIn, h.store.state.SelectedExitNode = false, ""
		h.store.state.Connector = protocol.ConnectorProxy
		change(h)
		h.load()
		startEnable(h)
		wantError(t, h, protocol.ReasonNoServer)
		if h.rec.index("core:start") >= 0 || h.rec.index("sys:") >= 0 || h.rec.index("journal:") >= 0 {
			t.Errorf("%s: the system must stay untouched: %v", name, h.rec.list())
		}
		if strings.Contains(strings.Join(h.rec.list(), "\n"), "not-a-uuid") {
			t.Errorf("%s: the text of the server is in the log: %v", name, h.rec.list())
		}
	}
	h := proxyHarness(t, linkTrojan)
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	if o := h.core.options(); o.Lane.Proxy == nil || h.store.saved().LoggedIn {
		t.Fatalf("not logged in to Tailscale, without an exit node: the proxy connector enables: %+v", o.Lane)
	}
}

// The core is built from the text in servers.json parsed again: that gives the very server proxy.add parsed,
// for a link and for a .conf named after its file.
func TestStoredServerParsesAgain(t *testing.T) {
	for _, c := range []struct{ text, name string }{{linkVLESS, ""}, {linkTrojan, ""}, {wgConfText, "home.conf"}, {wgConfText, ""}} {
		parsed, rejected, _ := connector.ParseText(c.text, c.name)
		if len(parsed) != 1 || len(rejected) != 0 {
			t.Fatalf("%q: %v", c.name, rejected)
		}
		h := newHarness(t)
		h.load()
		mustAdd(h, c.text, c.name)
		again, err := parseStored(h.store.savedServers().Servers[0])
		if err != nil || !reflect.DeepEqual(again, parsed[0]) {
			t.Fatalf("%q: %v\n%+v\n%+v", c.name, err, again, parsed[0])
		}
	}
}

func storedServerOf(t *testing.T, link string) store.StoredServer {
	t.Helper()
	h := newHarness(t)
	h.load()
	mustAdd(h, link, "")
	return h.store.savedServers().Servers[0]
}

// A refresh that takes the selected server leaves the connector proxy without a server: the next enable is
// no_server.
func TestRefreshWithoutTheSelectedServerIsNoServer(t *testing.T) {
	h := proxyHarness(t)
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, subURL, "")
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL] = subscriptionBody(linkTrojan) })
	if _, err := refresh(h, subID(subURL)); err != nil {
		t.Fatal(err)
	}
	startEnable(h)
	wantError(t, h, protocol.ReasonNoServer)
}

// Review Focus 3 with a real enable: an enable that comes while the selected server's subscription
// downloads is not held up by it, and the refresh that finds routing live after the download is refused;
// the core keeps the server it was built with.
func TestEnableDuringTheRefreshOfTheSelectedSubscription(t *testing.T) {
	h := proxyHarness(t)
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, subURL, "")
	hold := make(chan struct{})
	h.fetch.set(func(f *fakeFetcher) { f.hold, f.bodies[subURL] = hold, subscriptionBody(linkTrojan) })
	done := make(chan *protocol.Error)
	go func() {
		_, err := refresh(h, subID(subURL))
		done <- err
	}()
	h.waitFor("the download", func() bool { return h.rec.count("fetch") == 2 })
	if err := call(h, protocol.MethodEnable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOn)
	close(hold)
	if err := <-done; err == nil || err.Code != protocol.CodeWrongPhase {
		t.Fatalf("the refresh after the download: %v", err)
	}
	if s := h.e.State(); selectedOf(s) != serverID(linkVLESS) || len(s.Proxy.Servers) != 1 || h.core.options().Lane.Proxy.ID != serverID(linkVLESS) {
		t.Fatalf("servers %+v", s.Proxy)
	}
}

// proxy.select while routing is live is the reconnect of tailscale.setExitNode (7): 5.4 with the system exit
// node given back, then 5.3 with the new server. The same server again changes nothing.
func TestProxySelectReconnects(t *testing.T) {
	h := proxyEnabled(t, linkVLESS, linkTrojan)
	mark := len(h.rec.list())
	if err := call(h, protocol.MethodProxySelect, protocol.ProxyIDParams{ID: serverID(linkTrojan)}); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if o := h.core.options(); o.Lane.Proxy == nil || o.Lane.Proxy.ID != serverID(linkTrojan) || o.Lane.Proxy.Trojan == nil {
		t.Fatalf("the new core must use the new server: %+v", o.Lane)
	}
	if !inOrder(h, mark, "sys:set 100.64.0.1 true", "core:interrupt 1000", "core:start 1001", "sys:clear") {
		t.Fatalf("the system exit node is given back between the two runs: %v", h.rec.list()[mark:])
	}
	if got := strings.Join(h.phases(), ","); got != "off,starting,on,stopping,starting,on" { // off: the state of proxy.add
		t.Fatalf("phases %s", got)
	}
	if err := call(h, protocol.MethodProxySelect, protocol.ProxyIDParams{ID: serverID(linkTrojan)}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	if h.rec.count("core:start") != 2 || h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("the same server again: %v", h.rec.list())
	}
	nodeUntouched(t, h)
}

// The connector changes both ways while routing is live, each time with a reconnect: proxy.select from
// Tailscale to a server, tailscale.setExitNode back to the exit node.
func TestConnectorSwitchWhileLive(t *testing.T) {
	h := enabled(t)
	mustAdd(h, linkTrojan, "")
	if s := h.e.State(); s.Connector != protocol.ConnectorTailscale || h.rec.count("core:start") != 1 {
		t.Fatal("setup")
	}
	if err := call(h, protocol.MethodProxySelect, protocol.ProxyIDParams{ID: serverID(linkTrojan)}); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the proxy core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if o := h.core.options(); o.Lane.Proxy == nil || o.Lane.Tailscale != nil || h.e.State().Connector != protocol.ConnectorProxy ||
		h.e.State().Tailscale.Backend != protocol.BackendStopped {
		t.Fatalf("to proxy: %+v %+v", o.Lane, h.e.State().Tailscale)
	}
	if err := call(h, protocol.MethodSetExitNode, protocol.SetExitNodeParams{ID: exitNode}); err != nil {
		t.Fatal(err)
	}
	h.waitFor("the Tailscale core on", func() bool {
		return h.rec.count("core:start") == 3 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if o := h.core.options(); o.Lane.Proxy != nil || o.Lane.Tailscale == nil || o.Lane.Tailscale.ExitNode != exitNode ||
		h.e.State().Connector != protocol.ConnectorTailscale || h.e.State().Tailscale.Backend != protocol.BackendRunning {
		t.Fatalf("back to tailscale: %+v %+v", o.Lane, h.e.State().Tailscale)
	}
}

// Review Focus 4 and stage 3 part 1, section 5: a wrong key or password, or a server that does not answer,
// leaves the VPN lane without a link: the enable ends with tunnel_not_proven (no params, the neutral text
// is the window's), with one retry; the system exit node is never switched off, or given back when a
// reconnect had it off, and the journal goes.
func TestWrongServerIsTunnelNotProven(t *testing.T) {
	h := newHarness(t)
	h.load()
	mustAdd(h, linkTrojan, "")
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, nil)
	startEnable(h)
	wantError(t, h, protocol.ReasonTunnelNotProven)
	if r := h.e.State().Reason; len(r.Params) != 0 || h.rec.count("core:start") != 2 || h.core.anyAlive() {
		t.Fatalf("reason %+v, starts %d", r, h.rec.count("core:start"))
	}
	if on, ip := systemExitNode(h); !on || ip != exitNode || h.rec.index("sys:clear") >= 0 || h.rec.index("journal:") >= 0 {
		t.Fatalf("the system exit node untouched: on %v %s, %v", on, ip, h.rec.list())
	}

	h = proxyEnabled(t, linkVLESS, linkTrojan)
	if on, _ := systemExitNode(h); on || !h.store.hasJournal() {
		t.Fatal("setup: the system exit node is off for the first run")
	}
	h.probe.script([]ProbeResult{noAnswer}, []ProbeResult{atDirect}, nil)
	if err := call(h, protocol.MethodProxySelect, protocol.ProxyIDParams{ID: serverID(linkTrojan)}); err != nil {
		t.Fatal(err)
	}
	wantError(t, h, protocol.ReasonTunnelNotProven)
	if on, ip := systemExitNode(h); !on || ip != exitNode || h.store.hasJournal() || h.core.anyAlive() || h.store.saved().WasOn {
		t.Fatalf("the system exit node given back, the journal gone: on %v %s, journal %v, events %v", on, ip, h.store.hasJournal(), h.rec.list())
	}
	if s := h.e.State(); !s.SystemTailscale.ExitNodeOn || s.SystemTailscale.RestoreOnDisable || selectedOf(s) != serverID(linkTrojan) {
		t.Fatalf("state %+v %+v", s.SystemTailscale, s.Proxy)
	}
}

// The error of a config that the core refuses goes into the reason masked: the output of sing-box check may
// quote a server's secrets (stage 3 part 1, section 3).
func TestConfigInvalidIsMasked(t *testing.T) {
	h := newHarness(t)
	h.load()
	mustAdd(h, linkTrojan, "")
	h.core.checkErr = fmt.Errorf("sing-box check: exit status 1: FATAL decode config: outbound vpn: %s", linkTrojan)
	startEnable(h)
	wantError(t, h, protocol.ReasonConfigInvalid)
	text := h.e.State().Reason.Params[protocol.ParamError]
	if strings.Contains(text, passTrojan) || !strings.Contains(text, "example.com") {
		t.Fatalf("params %q", text)
	}
}

// A state.json of stage 2 has no connector: it is Tailscale's, and enables with the embedded node.
func TestOldStateJSONIsTailscale(t *testing.T) {
	h := newHarness(t)
	old := `{"version":1,"apps":[],"defaultLane":"vpn","selectedExitNode":"100.64.0.1","loggedIn":true,"exitNodes":[],"wasOn":false,"probeUrl":"https://api.ipify.org"}`
	var s store.State
	if err := json.Unmarshal([]byte(old), &s); err != nil {
		t.Fatal(err)
	}
	h.store.state = s
	h.load()
	if c := h.e.State().Connector; c != protocol.ConnectorTailscale {
		t.Fatalf("connector %q", c)
	}
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	if o := h.core.options(); o.Lane.Tailscale == nil || o.Lane.Proxy != nil || o.Lane.Tailscale.ExitNode != exitNode {
		t.Fatalf("lane %+v", o.Lane)
	}
}

// Review of task 1 for the connector: a save that meets state.json readable only now while a core runs keeps
// the connector and the server the core was built with.
func TestLateStateReadKeepsTheRunsServer(t *testing.T) {
	h := proxyEnabled(t, linkVLESS)
	h.e.mu.Lock()
	h.e.stateErr = readErr("state.json")
	h.e.mu.Unlock()
	h.store.mu.Lock()
	h.store.state.Connector, h.store.state.SelectedServer = protocol.ConnectorTailscale, ""
	h.store.mu.Unlock()
	h.e.mu.Lock()
	h.e.save()
	h.e.publish()
	h.e.mu.Unlock()
	if s, saved := h.e.State(), h.store.saved(); s.Connector != protocol.ConnectorProxy || selectedOf(s) != serverID(linkVLESS) ||
		saved.Connector != protocol.ConnectorProxy || saved.SelectedServer != serverID(linkVLESS) {
		t.Fatalf("state %s %+v, state.json %+v", s.Connector, s.Proxy, saved)
	}
}
