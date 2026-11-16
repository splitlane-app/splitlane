package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"splitlane/helper/internal/connector"
	"splitlane/helper/internal/protocol"
)

func ids(links ...string) []string {
	out := []string{}
	for _, l := range links {
		out = append(out, serverID(l))
	}
	return out
}

// Protocol 3: servers lists every server of the text, in order, new and there already; subscription is null
// for links and a .conf.
func TestAddResultServersOfLinks(t *testing.T) {
	h := proxyHarness(t)
	res := mustAdd(h, linkVLESS+"\nvless://not-a-uuid@example.com:443\n"+linkTrojan, "")
	if fmt.Sprint(res.Servers) != fmt.Sprint(ids(linkVLESS, linkTrojan)) || res.Subscription != nil {
		t.Fatalf("new: %+v", res)
	}
	res = mustAdd(h, linkTrojan+"\n"+linkSecond, "")
	if fmt.Sprint(res.Servers) != fmt.Sprint(ids(linkTrojan, linkSecond)) || len(res.Added) != 1 || res.Existing != 1 {
		t.Fatalf("partly there: %+v", res)
	}
	res = mustAdd(h, linkVLESS+"\n"+linkTrojan+"\n"+linkVLESS, "")
	if len(res.Added) != 0 || fmt.Sprint(res.Servers) != fmt.Sprint(ids(linkVLESS, linkTrojan)) || res.Subscription != nil {
		t.Fatalf("all there (and a repeated line): %+v", res)
	}
	res = mustAdd(h, wgConfText, "home.conf")
	if len(res.Servers) != 1 || res.Subscription != nil {
		t.Fatalf("conf: %+v", res)
	}
	// Nothing parsed: no servers, never null.
	res = mustAdd(h, "vless://bad", "")
	b, _ := json.Marshal(res)
	if !strings.Contains(string(b), `"servers":[]`) || !strings.Contains(string(b), `"subscription":null`) {
		t.Fatalf("json %s", b)
	}
}

func TestAddResultServersOfASubscription(t *testing.T) {
	h := newHarness(t)
	h.store.state.LoggedIn = false
	h.load()
	id := subID(subURL)
	mustAdd(h, linkTrojan, "")
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS, linkTrojan, "ss://broken")
	res := mustAdd(h, subURL, "")
	if res.Subscription == nil || *res.Subscription != id || fmt.Sprint(res.Servers) != fmt.Sprint(ids(linkVLESS, linkTrojan)) ||
		fmt.Sprint(res.Added) != fmt.Sprint(ids(linkVLESS)) || res.Existing != 1 {
		t.Fatalf("add: %+v", res)
	}
	// Every server of the subscription was there already.
	res, err := refresh(h, id)
	if err != nil || len(res.Added) != 0 || res.Subscription == nil || *res.Subscription != id ||
		fmt.Sprint(res.Servers) != fmt.Sprint(ids(linkVLESS, linkTrojan)) {
		t.Fatalf("refresh: %+v %v", res, err)
	}
	// The same address again.
	res = mustAdd(h, subURL, "")
	if res.Subscription == nil || *res.Subscription != id || len(res.Servers) != 2 {
		t.Fatalf("add again: %+v", res)
	}
	// A failed download: no servers, no subscription.
	h.fetch.errs[subURL2] = &connector.FetchError{Code: connector.CodeFetchTimeout}
	res = mustAdd(h, subURL2, "")
	if len(res.Servers) != 0 || res.Subscription != nil {
		t.Fatalf("failed: %+v", res)
	}
}

func TestAddResultServersAtMost500(t *testing.T) {
	h := newHarness(t)
	h.load()
	var links []string
	for i := 0; i < 500; i++ {
		links = append(links, fmt.Sprintf("trojan://pw%d@192.0.2.%d:%d#n%d", i, i%250+1, 1000+i, i))
	}
	res := mustAdd(h, strings.Join(links, "\n"), "")
	if len(res.Servers) != 500 {
		t.Fatalf("%d servers", len(res.Servers))
	}
	res = mustAdd(h, linkTrojan, "")
	if len(res.Servers) != 0 || len(res.Rejected) == 0 || res.Rejected[0].Code != protocol.CodeTooMany {
		t.Fatalf("over the limit: %+v", res)
	}
}

func reveal(h *harness, id string) (protocol.ProxyRevealResult, *protocol.Error) {
	h.t.Helper()
	raw, _ := json.Marshal(protocol.ProxyIDParams{ID: id})
	res, err := h.e.Handle(context.Background(), protocol.MethodProxyReveal, raw)
	var out protocol.ProxyRevealResult
	if err == nil {
		b, _ := json.Marshal(res)
		if json.Unmarshal(b, &out) != nil {
			h.t.Fatalf("answer %s", b)
		}
	}
	return out, err
}

func TestProxyReveal(t *testing.T) {
	h := newHarness(t)
	logEvents(h)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, linkTrojan, "")
	mustAdd(h, subURL, "")
	got, err := reveal(h, subID(subURL))
	if err != nil || got.Address != subURL {
		t.Fatalf("subscription: %+v %v", got, err)
	}
	if _, err := reveal(h, serverID(linkTrojan)); err == nil || err.Code != protocol.CodeBadRequest {
		t.Fatalf("server: %v", err)
	}
	if _, err := reveal(h, "sub-0123456789abcdef"); err == nil || err.Code != protocol.CodeUnknownServer {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := reveal(h, "../x"); err == nil || err.Code != protocol.CodeBadRequest {
		t.Fatalf("malformed: %v", err)
	}
	raw := json.RawMessage(`{"id":"` + subID(subURL) + `","x":1}`)
	if _, err := h.e.Handle(context.Background(), protocol.MethodProxyReveal, raw); err == nil || err.Code != protocol.CodeBadRequest {
		t.Fatalf("extra field: %v", err)
	}
	logs := strings.Join(h.rec.list(), "\n")
	if !strings.Contains(logs, "proxy.reveal") || !strings.Contains(logs, subID(subURL)) {
		t.Fatalf("the log names the method and id: %s", logs)
	}
	noSecrets(t, h)
}

// proxy.reveal answers in every phase.
func TestProxyRevealInAnyPhase(t *testing.T) {
	h := newHarness(t)
	h.load()
	h.fetch.bodies[subURL] = subscriptionBody(linkVLESS)
	mustAdd(h, subURL, "")
	for _, phase := range []string{protocol.PhaseOn, protocol.PhaseDegraded, protocol.PhaseBlocked, protocol.PhaseStarting, protocol.PhaseStopping, protocol.PhaseError} {
		h.e.mu.Lock()
		h.e.st.Phase = phase
		h.e.mu.Unlock()
		if got, err := reveal(h, subID(subURL)); err != nil || got.Address != subURL {
			t.Fatalf("%s: %+v %v", phase, got, err)
		}
	}
}
