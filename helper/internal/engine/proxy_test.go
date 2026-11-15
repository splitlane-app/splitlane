package engine

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"splitlane/helper/internal/connector"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

// Made-up servers (documentation addresses, invented keys). Each secret is a string the tests look for
// in everything the helper publishes, saves to state.json or logs.
const (
	uuidA      = "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	pbkA       = "ezvd-drM4QNV61tfmh5v-OP6yhTnAAQyxi5VK7zbfUo"
	passTrojan = "hunter2secretpass"
	passSecond = "other-secret-pw"
	linkVLESS  = "vless://" + uuidA + "@192.0.2.10:443?type=tcp&security=reality&pbk=" + pbkA + "&sni=www.example.com&fp=chrome#Reality"
	linkTrojan = "trojan://" + passTrojan + "@example.com:443?sni=example.com#Trojan"
	linkSecond = "trojan://" + passSecond + "@198.51.100.7:443?sni=example.com#Second"
	wgPrivate  = "lhw1Rezx8+44v1b7GrtW7HxgVikNwpLsRNzZk0ZbBXA="
	wgPublic   = "ZcQzkt7Yzwta15biT7QDOx+0JlXlF1K5Mnq0qUyDgLY="
	wgConfText = "[Interface]\nPrivateKey = " + wgPrivate + "\nAddress = 10.8.0.2/32\n\n[Peer]\nPublicKey = " + wgPublic +
		"\nEndpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0\n"
	subToken = "s3cr3tT0ken1234567890abcdef"
	subURL   = "https://sub.example.com/api/v1/client/subscribe?token=" + subToken
	subURL2  = "https://other.example.net/s/" + subToken + "2"
)

// secrets are the strings that must never leave servers.json.
var secrets = []string{uuidA, pbkA, passTrojan, passSecond, wgPrivate, wgPublic, subToken, "sub.example.com/api"}

func serverID(link string) string { return connector.ServerID(link) }

// add calls proxy.add as the pipe does and reads the answer back from its JSON.
func add(h *harness, text, name string) (protocol.ProxyAddResult, *protocol.Error) {
	h.t.Helper()
	return proxyCall(h, protocol.MethodProxyAdd, protocol.ProxyAddParams{Text: text, Name: name})
}

func proxyCall(h *harness, method string, params any) (protocol.ProxyAddResult, *protocol.Error) {
	h.t.Helper()
	raw, _ := json.Marshal(params)
	res, err := h.e.Handle(context.Background(), method, raw)
	var out protocol.ProxyAddResult
	if err == nil {
		b, _ := json.Marshal(res)
		if json.Unmarshal(b, &out) != nil {
			h.t.Fatalf("%s: answer %s", method, b)
		}
	}
	return out, err
}

func refresh(h *harness, id string) (protocol.ProxyAddResult, *protocol.Error) {
	h.t.Helper()
	return proxyCall(h, protocol.MethodProxyRefresh, protocol.ProxyIDParams{ID: id})
}

func mustAdd(h *harness, text, name string) protocol.ProxyAddResult {
	h.t.Helper()
	res, err := add(h, text, name)
	if err != nil {
		h.t.Fatalf("proxy.add: %v", err)
	}
	return res
}

func subID(url string) string { return subscriptionID(url) }

// subscriptionBody is a subscription answer: the links in base64, as most panels send them.
func subscriptionBody(links ...string) string {
	return base64.StdEncoding.EncodeToString([]byte(strings.Join(links, "\n") + "\n"))
}

func selectedOf(s protocol.State) string {
	if s.Proxy.Selected == nil {
		return ""
	}
	return *s.Proxy.Selected
}

// proxyHarness is a helper in phase off with the proxy connector: the servers of links added (the first
// selected), Tailscale not logged in.
func proxyHarness(t *testing.T, links ...string) *harness {
	t.Helper()
	h := newHarness(t)
	h.store.state.LoggedIn, h.store.state.SelectedExitNode = false, ""
	h.load()
	if len(links) > 0 {
		mustAdd(h, strings.Join(links, "\n"), "")
	}
	return h
}

// proxy.add with links (stage 3 part 1, section 4): every server goes into servers.json with its text, its
// name and protocol; the state shows id, name, protocol and source only. Each rejected line comes back with
// its number and code. The first server added becomes the selected one, and with Tailscale not ready in
// phase off the connector is proxy; a server that is there already is not added twice: it is counted as
// existing, and added lists only the new ones.
func TestProxyAddLinks(t *testing.T) {
	h := proxyHarness(t)
	res := mustAdd(h, linkVLESS+"\n\nvless://not-a-uuid@example.com:443\n"+linkTrojan+"\n", "")
	want := []string{serverID(linkVLESS), serverID(linkTrojan)}
	if fmt.Sprint(res.Added) != fmt.Sprint(want) || res.Existing != 0 || len(res.Rejected) != 1 || res.Rejected[0].Line != 3 ||
		res.Rejected[0].Code != connector.CodeBadLink || res.RejectedMore != 0 {
		t.Fatalf("answer %+v", res)
	}
	saved := h.store.savedServers().Servers
	if len(saved) != 2 || saved[0].ID != want[0] || saved[0].Text != linkVLESS || saved[0].Source != protocol.SourceManual ||
		saved[0].Protocol != connector.ProtocolVLESS || saved[1].Name != "Trojan" || saved[1].Protocol != connector.ProtocolTrojan {
		t.Fatalf("servers.json %+v", saved)
	}
	s := h.e.State()
	wantServers := []protocol.ProxyServer{
		{ID: want[0], Name: "Reality", Protocol: "vless", Source: "manual"},
		{ID: want[1], Name: "Trojan", Protocol: "trojan", Source: "manual"},
	}
	if fmt.Sprint(s.Proxy.Servers) != fmt.Sprint(wantServers) || selectedOf(s) != want[0] || s.Connector != protocol.ConnectorProxy {
		t.Fatalf("state %+v connector %s", s.Proxy, s.Connector)
	}
	if st := h.store.saved(); st.SelectedServer != want[0] || st.Connector != protocol.ConnectorProxy {
		t.Fatalf("state.json %+v", st)
	}
	res = mustAdd(h, linkTrojan+"\n"+linkSecond, "")
	if fmt.Sprint(res.Added) != fmt.Sprint([]string{serverID(linkSecond)}) || res.Existing != 1 || len(h.store.savedServers().Servers) != 3 {
		t.Fatalf("a server that is there is not added twice: %+v, %+v", res, h.store.savedServers().Servers)
	}
	if selectedOf(h.e.State()) != want[0] {
		t.Fatal("a later add keeps the selection")
	}
	if res, err := add(h, "ftp://example.com\n# a comment\nxhttp", ""); err != nil || len(res.Added) != 0 || len(res.Rejected) != 2 {
		t.Fatalf("nothing to add: %+v %v", res, err)
	}
}

// The ruling on the first server added (stage 3 part 1, section 4): it is always selected when none is; the
// connector becomes proxy with it only when Tailscale is not ready (not logged in, or no exit node selected)
// and routing is off or error. With Tailscale ready, or while routing is live, it is selected for later and
// the connector stays tailscale: the user switches by choosing it (proxy.select).
func TestFirstServerConnectorRule(t *testing.T) {
	for _, c := range []struct {
		name            string
		loggedIn        bool
		exitNode, phase string
		wantConnector   string
	}{
		{"ready, off", true, exitNode, protocol.PhaseOff, protocol.ConnectorTailscale},
		{"ready, error", true, exitNode, protocol.PhaseError, protocol.ConnectorTailscale},
		{"not logged in, off", false, exitNode, protocol.PhaseOff, protocol.ConnectorProxy},
		{"no exit node, error", true, "", protocol.PhaseError, protocol.ConnectorProxy},
		{"not logged in, stopping", false, "", protocol.PhaseStopping, protocol.ConnectorTailscale},
	} {
		h := newHarness(t)
		h.store.state.LoggedIn, h.store.state.SelectedExitNode = c.loggedIn, c.exitNode
		h.load()
		setPhase(h, c.phase)
		mustAdd(h, linkTrojan, "")
		if s := h.e.State(); s.Connector != c.wantConnector || selectedOf(s) != serverID(linkTrojan) || h.store.saved().Connector != c.wantConnector &&
			!(c.wantConnector == protocol.ConnectorTailscale && h.store.saved().Connector == "") {
			t.Errorf("%s: connector %s (state.json %q), selected %s", c.name, s.Connector, h.store.saved().Connector, selectedOf(s))
		}
	}
}

// The first server added while routing is live with Tailscale becomes the selected one, but the connector
// stays: a change of connector while live is a reconnect, and only proxy.select asks for one.
func TestProxyAddWhileLiveKeepsTheConnector(t *testing.T) {
	h := enabled(t)
	starts := h.rec.count("core:start")
	mustAdd(h, linkTrojan, "")
	s := h.e.State()
	if s.Connector != protocol.ConnectorTailscale || selectedOf(s) != serverID(linkTrojan) || s.Phase != protocol.PhaseOn ||
		h.rec.count("core:start") != starts {
		t.Fatalf("connector %s, selected %s, phase %s", s.Connector, selectedOf(s), s.Phase)
	}
}

// proxy.add with the text of a .conf and the base name of its file: the server is named after the file and
// its source is file; pasted, it is named host:port and its source is manual. A name that is a path is
// bad_request.
func TestProxyAddConf(t *testing.T) {
	h := newHarness(t)
	h.load()
	res := mustAdd(h, wgConfText, "home.conf")
	if len(res.Added) != 1 || len(res.Rejected) != 0 {
		t.Fatalf("answer %+v", res)
	}
	got := h.e.State().Proxy.Servers
	if len(got) != 1 || got[0].Name != "home" || got[0].Protocol != "wireguard" || got[0].Source != protocol.SourceFile {
		t.Fatalf("servers %+v", got)
	}
	h = newHarness(t)
	h.load()
	mustAdd(h, wgConfText, "")
	if got := h.e.State().Proxy.Servers; len(got) != 1 || got[0].Name != "vpn.example.com:51820" || got[0].Source != protocol.SourceManual {
		t.Fatalf("pasted: %+v", got)
	}
	for _, name := range []string{`C:\Users\x\home.conf`, "../home.conf"} {
		if _, err := add(h, wgConfText, name); err == nil || err.Code != protocol.CodeBadRequest {
			t.Fatalf("name %q: %v", name, err)
		}
	}
	if res, _ := add(h, strings.Replace(wgConfText, "[Interface]\n", "[Interface]\nJc = 4\n", 1), ""); len(res.Rejected) != 1 ||
		res.Rejected[0].Code != connector.CodeAmneziaWG || len(res.Added) != 0 {
		t.Fatalf("an AmneziaWG .conf: %+v", res)
	}
}

// The limits of proxy.add (stage 3 part 1, section 4): a text over 64 KB is the rejection too_long of the
// whole text; an add that would take servers.json over 500 servers adds nothing (too_many); the answer holds
// at most 50 rejected lines and counts the others.
func TestProxyAddLimits(t *testing.T) {
	h := newHarness(t)
	h.load()
	res, err := add(h, strings.Repeat("a", protocol.MaxProxyText+1), "")
	if err != nil || len(res.Rejected) != 1 || res.Rejected[0] != (protocol.ProxyRejection{Line: 0, Code: connector.CodeTooLong}) {
		t.Fatalf("too long: %+v %v", res, err)
	}
	var bad []string
	for i := range 60 {
		bad = append(bad, fmt.Sprintf("vless://bad-%d@example.com:443", i))
	}
	res, _ = add(h, strings.Join(bad, "\n"), "")
	if len(res.Rejected) != protocol.MaxRejected || res.RejectedMore != 10 || res.Rejected[49].Line != 50 {
		t.Fatalf("rejections: %d shown, %d more", len(res.Rejected), res.RejectedMore)
	}

	h = newHarness(t)
	for i := range store.MaxServers - 1 {
		h.store.servers.Servers = append(h.store.servers.Servers, store.StoredServer{ID: fmt.Sprintf("%016x", i), Name: "x",
			Protocol: "trojan", Source: protocol.SourceManual, Text: fmt.Sprintf("trojan://pw@192.0.2.1:%d", i+1)})
	}
	h.load()
	res, err = add(h, linkTrojan+"\n"+linkSecond+"\nvless://bad@example.com:1", "")
	if err != nil || len(res.Added) != 0 || len(res.Rejected) != 2 || res.Rejected[0] != (protocol.ProxyRejection{Line: 0, Code: protocol.CodeTooMany}) ||
		res.Rejected[1].Line != 3 {
		t.Fatalf("over 500: %+v %v", res, err)
	}
	if n := len(h.store.savedServers().Servers); n != store.MaxServers-1 || h.store.serversSaves != 0 {
		t.Fatalf("nothing is added: %d servers, %d saves", n, h.store.serversSaves)
	}
	if res := mustAdd(h, linkTrojan, ""); len(res.Added) != 1 || len(h.store.savedServers().Servers) != store.MaxServers {
		t.Fatalf("the 500th server: %+v", res)
	}
	if res := mustAdd(h, linkTrojan, ""); len(res.Added) != 0 || res.Existing != 1 || len(res.Rejected) != 0 {
		t.Fatalf("a server that is there takes no room: %+v", res)
	}
}

// The params of the proxy methods (spec 11): unknown fields, wrong types and malformed ids are bad_request;
// a well-formed id that no server or subscription has is unknown_server.
func TestProxyParamsAreChecked(t *testing.T) {
	h := proxyHarness(t, linkTrojan)
	for _, c := range []struct {
		method string
		params string
		code   string
	}{
		{protocol.MethodProxyAdd, `{"text":"x","path":"C:\\a.conf"}`, protocol.CodeBadRequest},
		{protocol.MethodProxyAdd, `{"text":""}`, protocol.CodeBadRequest},
		{protocol.MethodProxyAdd, `{}`, protocol.CodeBadRequest},
		{protocol.MethodProxyAdd, `{"text":["x"]}`, protocol.CodeBadRequest},
		{protocol.MethodProxyRefresh, `{"id":"sub-0123456789abcdef","url":"https://example.com"}`, protocol.CodeBadRequest},
		{protocol.MethodProxyRefresh, `{"id":"../servers.json"}`, protocol.CodeBadRequest},
		{protocol.MethodProxyRefresh, `{"id":"sub-0123456789abcdef"}`, protocol.CodeUnknownServer},
		{protocol.MethodProxyRefresh, `{"id":"` + serverID(linkTrojan) + `"}`, protocol.CodeUnknownServer},
		{protocol.MethodProxyRemove, `{}`, protocol.CodeBadRequest},
		{protocol.MethodProxyRemove, `{"id":"0123456789abcdef"}`, protocol.CodeUnknownServer},
		{protocol.MethodProxySelect, `{"id":"100.64.0.1"}`, protocol.CodeBadRequest},
		{protocol.MethodProxySelect, `{"id":"0123456789abcdef"}`, protocol.CodeUnknownServer},
		{protocol.MethodProxySelect, `{"id":"sub-0123456789abcdef"}`, protocol.CodeUnknownServer},
		{protocol.MethodProxySelect, `{"id":"` + serverID(linkTrojan) + `","x":1}`, protocol.CodeBadRequest},
	} {
		_, err := h.e.Handle(context.Background(), c.method, json.RawMessage(c.params))
		if err == nil || err.Code != c.code {
			t.Errorf("%s %s: want %s, got %v", c.method, c.params, c.code, err)
		}
	}
	if h.rec.count("fetch") != 0 || len(h.store.savedServers().Servers) != 1 {
		t.Fatalf("a refused call changes nothing: %v", h.rec.list())
	}
}

// proxy.add with a subscription address: the helper downloads it (the address is never in the state, in
// state.json or in a log), its servers come in with the subscription as their source, and the state shows
// the subscription by its host with the time of the update and the number of its servers. A download that
// fails adds nothing and comes back as the rejection of the whole text with the code of the failure.
func TestProxyAddSubscription(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS, linkTrojan, "ss://broken")
	res := mustAdd(h, "# my provider\n"+subURL+"\n", "")
	id := subID(subURL)
	if fmt.Sprint(res.Added) != fmt.Sprint([]string{serverID(linkVLESS), serverID(linkTrojan)}) || len(res.Rejected) != 1 || res.Rejected[0].Line != 3 {
		t.Fatalf("answer %+v", res)
	}
	s := h.e.State()
	if len(s.Proxy.Subscriptions) != 1 {
		t.Fatalf("subscriptions %+v", s.Proxy.Subscriptions)
	}
	sub := s.Proxy.Subscriptions[0]
	if sub.ID != id || sub.Name != "sub.example.com" || sub.Count != 2 || sub.Error != nil || sub.UpdatedAt != h.clock.Now().UTC().Format("2006-01-02T15:04:05Z07:00") {
		t.Fatalf("subscription %+v", sub)
	}
	if len(s.Proxy.Servers) != 2 || s.Proxy.Servers[0].Source != id || s.Proxy.Servers[1].Source != id || selectedOf(s) != serverID(linkVLESS) {
		t.Fatalf("servers %+v", s.Proxy)
	}
	saved := h.store.savedServers()
	if len(saved.Subscriptions) != 1 || saved.Subscriptions[0].URL != subURL || saved.Subscriptions[0].Count != 2 {
		t.Fatalf("servers.json keeps the address: %+v", saved.Subscriptions)
	}
	if h.rec.count("fetch") != 1 {
		t.Fatalf("downloads %d", h.rec.count("fetch"))
	}

	h.fetch.errs[subURL2] = &connector.FetchError{Code: connector.CodeFetchTimeout}
	res, err := add(h, subURL2, "")
	if err != nil || len(res.Added) != 0 || len(res.Rejected) != 1 || res.Rejected[0] != (protocol.ProxyRejection{Line: 0, Code: connector.CodeFetchTimeout}) {
		t.Fatalf("a failed download: %+v %v", res, err)
	}
	if len(h.e.State().Proxy.Subscriptions) != 1 || len(h.store.savedServers().Subscriptions) != 1 {
		t.Fatal("a subscription that never downloaded is not kept")
	}
	h.fetch.set(func(f *fakeFetcher) {
		delete(f.errs, subURL2)
		f.bodies[subURL2] = "proxies:\n  - name: a\n    type: ss\n"
	})
	if res, _ := add(h, subURL2, ""); len(res.Added) != 0 || len(res.Rejected) != 1 || res.Rejected[0].Code != connector.CodeUnsupportedProtocol {
		t.Fatalf("a subscription without links: %+v", res)
	}
	h.fetch.bodies[subURL2] = "\n"
	if res, _ := add(h, subURL2, ""); len(res.Added) != 0 || len(res.Rejected) != 1 || res.Rejected[0].Code != protocol.CodeEmptySubscription {
		t.Fatalf("an empty subscription: %+v", res)
	}
	noSecrets(t, h)
}

// The same subscription address added again is a refresh of that subscription, not a second one.
func TestProxyAddSubscriptionAgainRefreshesIt(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, subURL, "")
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL] = subscriptionBody(linkVLESS, linkSecond) })
	res := mustAdd(h, subURL, "")
	if s := h.e.State(); fmt.Sprint(res.Added) != fmt.Sprint([]string{serverID(linkSecond)}) || res.Existing != 1 || len(s.Proxy.Subscriptions) != 1 ||
		s.Proxy.Subscriptions[0].Count != 2 || len(s.Proxy.Servers) != 2 {
		t.Fatalf("%+v %+v", res, s.Proxy)
	}
}

// At most 20 subscriptions: the 21st is the rejection too_many, without a download.
func TestProxyAddSubscriptionLimit(t *testing.T) {
	h := newHarness(t)
	for i := range store.MaxSubscriptions {
		h.store.servers.Subscriptions = append(h.store.servers.Subscriptions, store.StoredSubscription{
			ID: fmt.Sprintf("sub-%016x", i), Name: "example.com", URL: fmt.Sprintf("https://example.com/%d", i), UpdatedAt: "2026-11-16T12:00:00Z"})
	}
	h.load()
	res, err := add(h, subURL, "")
	if err != nil || len(res.Rejected) != 1 || res.Rejected[0] != (protocol.ProxyRejection{Line: 0, Code: protocol.CodeTooMany}) || h.rec.count("fetch") != 0 {
		t.Fatalf("%+v %v, downloads %d", res, err, h.rec.count("fetch"))
	}
}

// proxy.refresh (stage 3 part 1, section 4): the servers of the subscription are replaced by those it gives
// now; the selected server stays selected when a server with its id is still there (a changed name does not
// change the id), else the selection goes and the connector stays proxy: an enable is then no_server.
func TestProxyRefresh(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false
	h.load()
	id := subID(subURL)
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS, linkTrojan)
	mustAdd(h, subURL, "")
	if selectedOf(h.e.State()) != serverID(linkVLESS) {
		t.Fatal("setup")
	}
	renamed := strings.Replace(linkVLESS, "#Reality", "#Reality%20(10%20GB%20left)", 1)
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL] = subscriptionBody(renamed, linkSecond) })
	<-h.clock.After(5 * time.Minute)
	res, err := refresh(h, id)
	if err != nil || fmt.Sprint(res.Added) != fmt.Sprint([]string{serverID(linkSecond)}) || res.Existing != 1 {
		t.Fatalf("%+v %v", res, err)
	}
	s := h.e.State()
	if len(s.Proxy.Servers) != 2 || s.Proxy.Servers[0].Name != "Reality (10 GB left)" || s.Proxy.Servers[1].ID != serverID(linkSecond) ||
		selectedOf(s) != serverID(linkVLESS) || s.Proxy.Subscriptions[0].UpdatedAt != h.clock.Now().UTC().Format("2006-01-02T15:04:05Z07:00") {
		t.Fatalf("after the refresh %+v", s.Proxy)
	}
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL] = subscriptionBody(linkTrojan) })
	if _, err := refresh(h, id); err != nil {
		t.Fatal(err)
	}
	if s := h.e.State(); selectedOf(s) != "" || s.Connector != protocol.ConnectorProxy || h.store.saved().SelectedServer != "" {
		t.Fatalf("the selected server is gone: %+v, connector %s", s.Proxy, s.Connector)
	}
}

// A refresh that fails keeps the servers of the subscription and keeps the code of the failure in it (the
// window shows it); the answer is that code. The next refresh that works clears it. A refresh that would take
// servers.json over 500 servers is too_many, and keeps the old servers as well.
func TestProxyRefreshFails(t *testing.T) {
	h := newHarness(t)
	h.load()
	id := subID(subURL)
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS, linkTrojan)
	mustAdd(h, subURL, "")
	h.fetch.set(func(f *fakeFetcher) { f.errs[subURL] = &connector.FetchError{Code: connector.CodeFetchBlockedAddress} })
	_, err := refresh(h, id)
	if err == nil || err.Code != connector.CodeFetchBlockedAddress || strings.Contains(err.Message, subToken) || strings.Contains(err.Message, "sub.example.com") {
		t.Fatalf("answer %v", err)
	}
	s := h.e.State()
	if sub := s.Proxy.Subscriptions[0]; sub.Error == nil || *sub.Error != connector.CodeFetchBlockedAddress || sub.Count != 2 || len(s.Proxy.Servers) != 2 {
		t.Fatalf("after the failure %+v", s.Proxy)
	}
	if got := h.store.savedServers().Subscriptions[0]; got.Error != connector.CodeFetchBlockedAddress {
		t.Fatalf("servers.json %+v", got)
	}
	h.fetch.set(func(f *fakeFetcher) { delete(f.errs, subURL); f.bodies[subURL] = "not a subscription at all" })
	if _, err := refresh(h, id); err == nil || err.Code != connector.CodeUnsupportedProtocol {
		t.Fatalf("a body without links: %v", err)
	}
	if len(h.e.State().Proxy.Servers) != 2 {
		t.Fatal("the servers stay")
	}
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL] = subscriptionBody(linkTrojan) })
	if _, err := refresh(h, id); err != nil {
		t.Fatal(err)
	}
	if sub := h.e.State().Proxy.Subscriptions[0]; sub.Error != nil || sub.Count != 1 {
		t.Fatalf("a refresh that works clears the error: %+v", sub)
	}

	h = newHarness(t)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, subURL, "")
	var many []string
	for i := range store.MaxServers {
		many = append(many, fmt.Sprintf("trojan://pw@192.0.2.1:%d", i+1))
	}
	mustAdd(h, strings.Join(many[:store.MaxServers-1], "\n"), "")
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL] = subscriptionBody(linkVLESS, linkTrojan) })
	if _, err := refresh(h, subID(subURL)); err == nil || err.Code != protocol.CodeTooMany {
		t.Fatalf("over 500: %v", err)
	}
	s = h.e.State()
	if len(s.Proxy.Servers) != store.MaxServers || s.Proxy.Subscriptions[0].Count != 1 || s.Proxy.Subscriptions[0].Error == nil ||
		*s.Proxy.Subscriptions[0].Error != protocol.CodeTooMany {
		t.Fatalf("over 500 keeps the old servers: %d servers, %+v", len(s.Proxy.Servers), s.Proxy.Subscriptions[0])
	}
}

// Review Focus 3: while routing is live with a server of the subscription, the subscription is not refreshed
// (wrong_phase, before any download): the core runs on its config until routing is switched off. Another
// subscription is refreshed as usual. In starting and stopping such a refresh is busy.
func TestProxyRefreshOfTheLiveServer(t *testing.T) {
	h := proxyOn(t)
	id := subID(subURL)
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL] = subscriptionBody(linkTrojan) })
	before := h.store.savedServers()
	if _, err := refresh(h, id); err == nil || err.Code != protocol.CodeWrongPhase {
		t.Fatalf("live: %v", err)
	}
	if h.rec.count("fetch") != 1 || fmt.Sprint(h.store.savedServers()) != fmt.Sprint(before) || selectedOf(h.e.State()) != serverID(linkVLESS) {
		t.Fatalf("nothing downloaded, nothing changed: %v", h.rec.list())
	}
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL2] = subscriptionBody(linkSecond) })
	mustAdd(h, subURL2, "")
	h.fetch.set(func(f *fakeFetcher) { f.bodies[subURL2] = subscriptionBody(linkSecond, linkTrojan) })
	if _, err := refresh(h, subID(subURL2)); err != nil || h.e.State().Phase != protocol.PhaseOn {
		t.Fatalf("another subscription: %v", err)
	}
	setPhase(h, protocol.PhaseStopping)
	if _, err := refresh(h, id); err == nil || err.Code != protocol.CodeBusy {
		t.Fatalf("stopping: %v", err)
	}
	setPhase(h, protocol.PhaseOff)
	if _, err := refresh(h, id); err != nil || selectedOf(h.e.State()) != "" {
		t.Fatalf("off: the refresh runs and the selection goes: %v %+v", err, h.e.State().Proxy)
	}
}

// A command that comes while a subscription downloads is not held up by it: the download runs outside the
// queue of the state-changing commands (5.1), which it takes only to apply what it got.
func TestProxyRefreshDownloadsOutsideTheQueue(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, subURL, "")
	hold := make(chan struct{})
	h.fetch.set(func(f *fakeFetcher) { f.hold, f.bodies[subURL] = hold, subscriptionBody(linkVLESS, linkTrojan) })
	done := make(chan *protocol.Error)
	go func() {
		_, err := refresh(h, subID(subURL))
		done <- err
	}()
	h.waitFor("the download", func() bool { return h.rec.count("fetch") == 2 })
	answered := make(chan struct{})
	go func() {
		defer close(answered)
		mustAdd(h, linkSecond, "")
	}()
	select {
	case <-answered:
	case <-time.After(5 * time.Second):
		t.Fatal("proxy.add waited for the download of another command")
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if s := h.e.State(); len(s.Proxy.Servers) != 3 {
		t.Fatalf("servers %+v", s.Proxy)
	}
}

// proxyOn is a helper in phase on with the proxy connector, as after step 9 of 5.3: the subscription subURL
// with linkVLESS (selected) and linkTrojan.
func proxyOn(t *testing.T) *harness {
	t.Helper()
	h := proxyHarness(t)
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS, linkTrojan)
	mustAdd(h, subURL, "")
	h.running()
	return h
}

// proxy.remove (stage 3 part 1, section 4): a server, or a subscription with all its servers. The selected
// server goes from the selection (an enable is then no_server); while routing is live with it, it is not
// removed (wrong_phase), nor is its subscription.
func TestProxyRemove(t *testing.T) {
	h := proxyHarness(t, linkVLESS, linkTrojan)
	if err := call(h, protocol.MethodProxyRemove, protocol.ProxyIDParams{ID: serverID(linkTrojan)}); err != nil {
		t.Fatal(err)
	}
	if s := h.e.State(); len(s.Proxy.Servers) != 1 || selectedOf(s) != serverID(linkVLESS) || len(h.store.savedServers().Servers) != 1 {
		t.Fatalf("%+v", s.Proxy)
	}
	if err := call(h, protocol.MethodProxyRemove, protocol.ProxyIDParams{ID: serverID(linkVLESS)}); err != nil {
		t.Fatal(err)
	}
	if s := h.e.State(); len(s.Proxy.Servers) != 0 || selectedOf(s) != "" || h.store.saved().SelectedServer != "" || s.Connector != protocol.ConnectorProxy {
		t.Fatalf("%+v", s.Proxy)
	}

	h = proxyOn(t)
	for _, id := range []string{serverID(linkVLESS), subID(subURL)} {
		if err := call(h, protocol.MethodProxyRemove, protocol.ProxyIDParams{ID: id}); err == nil || err.Code != protocol.CodeWrongPhase {
			t.Fatalf("live: %s: %v", id, err)
		}
	}
	mustAdd(h, linkSecond, "")
	if err := call(h, protocol.MethodProxyRemove, protocol.ProxyIDParams{ID: serverID(linkSecond)}); err != nil {
		t.Fatalf("another server while live: %v", err)
	}
	setPhase(h, protocol.PhaseStarting)
	if err := call(h, protocol.MethodProxyRemove, protocol.ProxyIDParams{ID: serverID(linkVLESS)}); err == nil || err.Code != protocol.CodeBusy {
		t.Fatalf("starting: %v", err)
	}
	setPhase(h, protocol.PhaseOff)
	if err := call(h, protocol.MethodProxyRemove, protocol.ProxyIDParams{ID: subID(subURL)}); err != nil {
		t.Fatal(err)
	}
	s := h.e.State()
	if len(s.Proxy.Servers) != 0 || len(s.Proxy.Subscriptions) != 0 || selectedOf(s) != "" || len(h.store.savedServers().Subscriptions) != 0 {
		t.Fatalf("the subscription with its servers: %+v", s.Proxy)
	}
}

// proxy.select (stage 3 part 1, section 4): the server becomes the selected one and the connector proxy;
// tailscale.setExitNode makes the connector tailscale again, and the proxy selection stays for later.
func TestProxySelectAndConnectorSwitch(t *testing.T) {
	h := newHarness(t)
	h.load()
	if s := h.e.State(); s.Connector != protocol.ConnectorTailscale {
		t.Fatalf("an old state.json is tailscale: %s", s.Connector)
	}
	mustAdd(h, linkVLESS+"\n"+linkTrojan, "")
	if err := call(h, protocol.MethodSetExitNode, protocol.SetExitNodeParams{ID: exitNode}); err != nil {
		t.Fatal(err)
	}
	if s := h.e.State(); s.Connector != protocol.ConnectorTailscale || h.store.saved().Connector != protocol.ConnectorTailscale {
		t.Fatalf("tailscale.setExitNode: %s", s.Connector)
	}
	if err := call(h, protocol.MethodProxySelect, protocol.ProxyIDParams{ID: serverID(linkTrojan)}); err != nil {
		t.Fatal(err)
	}
	s, saved := h.e.State(), h.store.saved()
	if s.Connector != protocol.ConnectorProxy || selectedOf(s) != serverID(linkTrojan) || saved.Connector != protocol.ConnectorProxy ||
		saved.SelectedServer != serverID(linkTrojan) || saved.SelectedExitNode != exitNode {
		t.Fatalf("proxy.select: %s %+v, state.json %+v", s.Connector, s.Proxy, saved)
	}
	if err := call(h, protocol.MethodSetExitNode, protocol.SetExitNodeParams{ID: exitNode}); err != nil {
		t.Fatal(err)
	}
	if s := h.e.State(); s.Connector != protocol.ConnectorTailscale || selectedOf(s) != serverID(linkTrojan) {
		t.Fatalf("back to tailscale, the selection stays: %s %+v", s.Connector, s.Proxy)
	}
	setPhase(h, protocol.PhaseStarting)
	if err := call(h, protocol.MethodProxySelect, protocol.ProxyIDParams{ID: serverID(linkVLESS)}); err == nil || err.Code != protocol.CodeBusy {
		t.Fatalf("starting: %v", err)
	}
}

// servers.json that cannot be read is not written over: the proxy methods are apply_failed until it reads,
// and then it is taken as it is.
func TestProxyServersUnreadable(t *testing.T) {
	h := newHarness(t)
	h.store.servers.Servers = []store.StoredServer{{ID: serverID(linkTrojan), Name: "Trojan", Protocol: "trojan", Source: "manual", Text: linkTrojan}}
	h.store.serversReadFails = 2
	h.load()
	if _, err := add(h, linkVLESS, ""); err == nil || err.Code != protocol.CodeApplyFailed || h.store.serversSaves != 0 {
		t.Fatalf("unreadable: %v, saves %d", err, h.store.serversSaves)
	}
	res := mustAdd(h, linkVLESS, "")
	if s := h.e.State(); len(res.Added) != 1 || len(s.Proxy.Servers) != 2 || s.Proxy.Servers[0].ID != serverID(linkTrojan) {
		t.Fatalf("readable again: %+v", s.Proxy)
	}
	h.store.mu.Lock()
	h.store.serversSaveErr = errBoom
	h.store.mu.Unlock()
	if _, err := add(h, linkSecond, ""); err == nil || err.Code != protocol.CodeApplyFailed || len(h.e.State().Proxy.Servers) != 2 {
		t.Fatalf("a save that fails changes nothing: %v", err)
	}
}

// Stage 3 part 1, section 3: the state, state.json and the log never carry a secret of a server or the address
// of a subscription: only ids, names, protocols, sources and hosts.
func TestProxyStateHasNoSecrets(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS, linkSecond)
	mustAdd(h, linkTrojan, "")
	mustAdd(h, wgConfText, "home.conf")
	mustAdd(h, subURL, "")
	h.fetch.set(func(f *fakeFetcher) {
		f.errs[subURL] = &connector.FetchError{Code: connector.CodeFetchFailed, Detail: "dns"}
	})
	refresh(h, subID(subURL))
	noSecrets(t, h)
	if n := len(h.e.State().Proxy.Servers); n != 4 {
		t.Fatalf("servers %d", n)
	}
}

// One download at a time: a second proxy.add of a subscription or proxy.refresh while one downloads is busy,
// without a download of its own.
func TestOneDownloadAtATime(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	h.fetch.bodies[subURL2] = subscriptionBody(linkSecond)
	mustAdd(h, subURL, "")
	hold := make(chan struct{})
	h.fetch.set(func(f *fakeFetcher) { f.hold = hold })
	done := make(chan *protocol.Error)
	go func() {
		_, err := refresh(h, subID(subURL))
		done <- err
	}()
	h.waitFor("the download", func() bool { return h.rec.count("fetch") == 2 })
	if _, err := add(h, subURL2, ""); err == nil || err.Code != protocol.CodeBusy {
		t.Fatalf("a second download: %v", err)
	}
	if _, err := refresh(h, subID(subURL)); err == nil || err.Code != protocol.CodeBusy || h.rec.count("fetch") != 2 {
		t.Fatalf("a second refresh: %v, downloads %d", err, h.rec.count("fetch"))
	}
	close(hold)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if res := mustAdd(h, subURL2, ""); len(res.Added) != 1 {
		t.Fatalf("after the download: %+v", res)
	}
}

// A download whose client went (the pipe ends the context) changes nothing: no server, no error in the
// subscription.
func TestCancelledDownloadChangesNothing(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, subURL, "")
	before := h.store.savedServers()
	hold := make(chan struct{})
	defer close(hold)
	h.fetch.set(func(f *fakeFetcher) { f.hold, f.bodies[subURL] = hold, subscriptionBody(linkTrojan) })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan *protocol.Error)
	go func() {
		raw, _ := json.Marshal(protocol.ProxyIDParams{ID: subID(subURL)})
		_, err := h.e.Handle(ctx, protocol.MethodProxyRefresh, raw)
		done <- err
	}()
	h.waitFor("the download", func() bool { return h.rec.count("fetch") == 2 })
	cancel()
	if err := <-done; err == nil {
		t.Fatal("a cancelled download is no success")
	}
	if fmt.Sprint(h.store.savedServers()) != fmt.Sprint(before) || h.e.State().Proxy.Subscriptions[0].Error != nil {
		t.Fatalf("nothing changes: %+v", h.store.savedServers())
	}
	if res := mustAdd(h, linkSecond, ""); len(res.Added) != 1 {
		t.Fatal("the next command runs")
	}
}

// proxy.add of a subscription that is there races its proxy.remove: the subscription removed during the
// download is not made again (unknown_server), and its servers stay gone.
func TestAddOfARemovedSubscriptionIsRefused(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, subURL, "")
	hold := make(chan struct{})
	h.fetch.set(func(f *fakeFetcher) { f.hold = hold })
	done := make(chan *protocol.Error)
	go func() {
		_, err := add(h, subURL, "")
		done <- err
	}()
	h.waitFor("the download", func() bool { return h.rec.count("fetch") == 2 })
	if err := call(h, protocol.MethodProxyRemove, protocol.ProxyIDParams{ID: subID(subURL)}); err != nil {
		t.Fatal(err)
	}
	close(hold)
	if err := <-done; err == nil || err.Code != protocol.CodeUnknownServer {
		t.Fatalf("the add after the removal: %v", err)
	}
	if s := h.e.State(); len(s.Proxy.Subscriptions) != 0 || len(s.Proxy.Servers) != 0 || len(h.store.savedServers().Subscriptions) != 0 {
		t.Fatalf("the subscription is not made again: %+v", s.Proxy)
	}
}

// noSecrets looks for every secret in every published state, in state.json and in the log.
func noSecrets(t *testing.T, h *harness) {
	t.Helper()
	h.mu.Lock()
	states, _ := json.Marshal(h.states)
	h.mu.Unlock()
	saved, _ := json.Marshal(h.store.saved())
	logs := strings.Join(h.rec.list(), "\n")
	for _, s := range secrets {
		for name, text := range map[string]string{"published state": string(states), "state.json": string(saved), "log": logs} {
			if strings.Contains(text, s) {
				t.Errorf("%s has the secret %q", name, s)
			}
		}
	}
}

// A published name fits the pipe's limit: the window refuses a state with a longer one. The cut is at a rune
// boundary and keeps the start of the name.
func TestPublishedNamesFitTheNameLimit(t *testing.T) {
	cjk := strings.Repeat("字", 100)   // 300 bytes
	emoji := strings.Repeat("😀", 100) // 400 bytes of 4-byte runes
	for _, name := range []string{cjk, emoji, strings.Repeat("a", 300), strings.Repeat("a", 256)} {
		got := fitName(name)
		if len(got) > protocol.MaxNameLen || !utf8.ValidString(got) || !strings.HasPrefix(name, got) {
			t.Errorf("fitName(%d bytes) = %d bytes, valid=%v", len(name), len(got), utf8.ValidString(got))
		}
	}
	if got := fitName(strings.Repeat("a", 256)); len(got) != 256 {
		t.Errorf("a name of exactly 256 bytes was cut to %d", len(got))
	}
	if got := fitName("short"); got != "short" {
		t.Errorf("short name changed: %q", got)
	}

	h := proxyHarness(t, "trojan://"+passTrojan+"@example.com:443?sni=example.com#"+emoji)
	servers := h.e.State().Proxy.Servers
	if len(servers) != 1 {
		t.Fatalf("servers = %d, want 1", len(servers))
	}
	if n := servers[0].Name; len(n) > protocol.MaxNameLen || !utf8.ValidString(n) || !strings.HasPrefix(emoji, n) || n == "" {
		t.Errorf("published server name: %d bytes, valid=%v", len(n), utf8.ValidString(n))
	}
}
