package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

const goodID = "0b5c2a6e-6a43-4c55-9d43-2f1f4a0c9e11"

func app(id, lane, kind string) AppRule {
	return AppRule{ID: id, Exe: `C:\Tools\curl-a.exe`, Matcher: Matcher{Kind: kind}, Lane: lane}
}

func TestEncodeResultAndError(t *testing.T) {
	got := string(EncodeResult(json.RawMessage(`7`), nil))
	if got != "{\"id\":7,\"result\":{}}\n" {
		t.Fatalf("result line %q", got)
	}
	e := &Error{Code: CodeProtocolMismatch, Message: "helper speaks 1", Protocol: 1}
	got = string(EncodeError(json.RawMessage(`"a"`), e))
	if got != "{\"id\":\"a\",\"error\":{\"code\":\"protocol_mismatch\",\"message\":\"helper speaks 1\",\"protocol\":1}}\n" {
		t.Fatalf("error line %q", got)
	}
	e = &Error{Code: CodeRuleRejected, Message: "1 rule", Rules: []RuleRejection{{ID: goodID, Reason: RejectProtected}}}
	if !strings.Contains(string(EncodeError(nil, e)), `"rules":[{"id":"`+goodID+`","reason":"protected"}]`) {
		t.Fatal("rules field missing")
	}
	if got := string(EncodeEvent("state", map[string]int{"x": 1})); got != "{\"event\":\"state\",\"data\":{\"x\":1}}\n" {
		t.Fatalf("event line %q", got)
	}
}

func TestDecodeParamsIsStrict(t *testing.T) {
	var p DisableParams
	if err := DecodeParams(nil, &p); err != nil || p.SkipRestore {
		t.Fatalf("missing params: %v %+v", err, p)
	}
	if err := DecodeParams(json.RawMessage(`{"skipRestore":true}`), &p); err != nil || !p.SkipRestore {
		t.Fatalf("skipRestore: %v %+v", err, p)
	}
	if err := DecodeParams(json.RawMessage(`{"skip":true}`), &p); err == nil || err.Code != CodeBadRequest {
		t.Fatalf("unknown field must be bad_request, got %v", err)
	}
	if err := DecodeParams(json.RawMessage(`{"skipRestore":"yes"}`), &p); err == nil || err.Code != CodeBadRequest {
		t.Fatalf("wrong type must be bad_request, got %v", err)
	}
	for _, raw := range []string{`{}}`, `{}]`, `{} {}`, `{} x`, `{"skipRestore":true} {"skipRestore":true}`} {
		var q DisableParams
		if err := DecodeParams(json.RawMessage(raw), &q); err == nil || err.Code != CodeBadRequest {
			t.Errorf("trailing data %q must be bad_request, got %v", raw, err)
		}
	}
}

func TestValidateLanes(t *testing.T) {
	ok := LanesSetParams{DefaultLane: LaneVPN, Apps: []AppRule{app(goodID, LaneDirect, KindFile)}}
	if err := ValidateLanes(ok); err != nil {
		t.Fatal(err)
	}
	long := app(goodID, LaneVPN, KindFile)
	long.Name = strings.Repeat("я", MaxNameLen)
	cases := map[string]LanesSetParams{
		"default lane": {DefaultLane: "both"},
		"not a uuid":   {DefaultLane: LaneVPN, Apps: []AppRule{app("1", LaneVPN, KindFile)}},
		"repeated id":  {DefaultLane: LaneVPN, Apps: []AppRule{app(goodID, LaneVPN, KindFile), app(goodID, LaneDirect, KindFile)}},
		"lane":         {DefaultLane: LaneVPN, Apps: []AppRule{app(goodID, "tor", KindFile)}},
		"kind":         {DefaultLane: LaneVPN, Apps: []AppRule{app(goodID, LaneVPN, "glob")}},
		"name length":  {DefaultLane: LaneVPN, Apps: []AppRule{long}},
	}
	for name, p := range cases {
		if err := ValidateLanes(p); err == nil || err.Code != CodeBadRequest {
			t.Errorf("%s: want bad_request, got %v", name, err)
		}
	}
}

func TestValidateExitNodeID(t *testing.T) {
	known := []ExitNode{{ID: "100.64.0.7", Name: "a", Online: true}}
	if err := ValidateExitNodeID("100.64.0.7", known); err != nil {
		t.Fatal(err)
	}
	if err := ValidateExitNodeID("100.64.0.8", known); err == nil || err.Code != CodeUnknownExitNode {
		t.Fatalf("not in list: %v", err)
	}
	if err := ValidateExitNodeID("100.100.1.1", nil); err != nil {
		t.Fatalf("in range without a list: %v", err)
	}
	if err := ValidateExitNodeID("192.0.2.1", nil); err == nil || err.Code != CodeUnknownExitNode {
		t.Fatalf("outside range: %v", err)
	}
	if err := ValidateExitNodeID("node-name", nil); err == nil || err.Code != CodeBadRequest {
		t.Fatalf("not an address: %v", err)
	}
}

func TestCloneIsDeep(t *testing.T) {
	ip := "192.0.2.1"
	s := State{Apps: []AppRule{app(goodID, LaneVPN, KindFile)}, Reason: &Reason{Code: ReasonRestorePending, Params: map[string]string{"command": "x"}}}
	s.Addresses.VPN = &ip
	c := s.Clone()
	c.Apps[0].Lane = LaneDirect
	c.Reason.Params["command"] = "y"
	*c.Addresses.VPN = "198.51.100.1"
	if s.Apps[0].Lane != LaneVPN || s.Reason.Params["command"] != "x" || *s.Addresses.VPN != ip {
		t.Fatal("clone shares memory with the original")
	}
}

func TestStateJSONHasEmptyListsAndNulls(t *testing.T) {
	s := State{Phase: PhaseOff, DefaultLane: LaneVPN, Apps: []AppRule{}, Tailscale: Tailscale{Backend: "stopped", ExitNodes: []ExitNode{}}}
	b, _ := json.Marshal(s)
	for _, want := range []string{`"reason":null`, `"apps":[]`, `"addresses":{"vpn":null,"direct":null}`, `"exitNodes":[]`, `"selectedExitNode":null`, `"connector":""`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("state JSON lacks %s: %s", want, b)
		}
	}
}

// Protocol version 2 (spec of stage 3 part 1, section 4): the proxy methods, the connector and the
// proxy block of the state.
func TestVersionTwoHasTheProxyMethods(t *testing.T) {
	if Version != 2 {
		t.Fatalf("version %d", Version)
	}
	for _, m := range []string{MethodProxyAdd, MethodProxyRefresh, MethodProxyRemove, MethodProxySelect} {
		found := false
		for _, k := range Methods {
			found = found || k == m
		}
		if !found {
			t.Errorf("%s is not in Methods", m)
		}
	}
	if MethodProxyAdd != "proxy.add" || MethodProxyRefresh != "proxy.refresh" || MethodProxyRemove != "proxy.remove" ||
		MethodProxySelect != "proxy.select" || ReasonNoServer != "no_server" || ConnectorTailscale != "tailscale" || ConnectorProxy != "proxy" {
		t.Fatal("names of version 2")
	}
}

func TestProxyStateJSON(t *testing.T) {
	s := State{Phase: PhaseOff, DefaultLane: LaneVPN, Apps: []AppRule{}, Connector: ConnectorTailscale,
		Tailscale: Tailscale{Backend: "stopped", ExitNodes: []ExitNode{}},
		Proxy:     Proxy{Servers: []ProxyServer{}, Subscriptions: []Subscription{}}}
	b, _ := json.Marshal(s)
	for _, want := range []string{`"connector":"tailscale"`, `"proxy":{"servers":[],"selected":null,"subscriptions":[]}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("state JSON lacks %s: %s", want, b)
		}
	}
	sel, errText := "0123456789abcdef", "fetch_timeout"
	s.Proxy = Proxy{
		Servers:       []ProxyServer{{ID: sel, Name: "a", Protocol: "vless", Source: "sub-0123456789abcdef"}},
		Selected:      &sel,
		Subscriptions: []Subscription{{ID: "sub-0123456789abcdef", Name: "example.com", UpdatedAt: "2026-11-16T12:00:00Z", Count: 1, Error: &errText}},
	}
	b, _ = json.Marshal(s.Proxy)
	want := `{"servers":[{"id":"0123456789abcdef","name":"a","protocol":"vless","source":"sub-0123456789abcdef"}],"selected":"0123456789abcdef",` +
		`"subscriptions":[{"id":"sub-0123456789abcdef","name":"example.com","updatedAt":"2026-11-16T12:00:00Z","count":1,"error":"fetch_timeout"}]}`
	if string(b) != want {
		t.Fatalf("proxy JSON\n%s\nwant\n%s", b, want)
	}
	c := s.Clone()
	c.Proxy.Servers[0].Name = "b"
	*c.Proxy.Selected = "x"
	c.Proxy.Subscriptions[0].Count = 7
	*c.Proxy.Subscriptions[0].Error = "y"
	if s.Proxy.Servers[0].Name != "a" || *s.Proxy.Selected != sel || s.Proxy.Subscriptions[0].Count != 1 || *s.Proxy.Subscriptions[0].Error != errText {
		t.Fatal("clone shares the proxy block with the original")
	}
	r, _ := json.Marshal(ProxyAddResult{Added: []string{}, Existing: 1, Rejected: []ProxyRejection{{Line: 2, Code: "bad_link"}}, RejectedMore: 3})
	if string(r) != `{"added":[],"existing":1,"rejected":[{"line":2,"code":"bad_link"}],"rejectedMore":3}` {
		t.Fatalf("result %s", r)
	}
}

// The params of the proxy methods are strict (spec 11): unknown fields are bad_request, the name of a
// .conf is a base name (never a path), the id a server's or a subscription's.
func TestProxyParams(t *testing.T) {
	var p ProxyAddParams
	if err := DecodeParams(json.RawMessage(`{"text":"vless://x","name":"home.conf"}`), &p); err != nil || p.Text != "vless://x" || p.Name != "home.conf" {
		t.Fatalf("%v %+v", err, p)
	}
	for _, raw := range []string{`{"text":"x","path":"C:\\a.conf"}`, `{"text":1}`, `{"id":"x"}`} {
		var q ProxyAddParams
		if err := DecodeParams(json.RawMessage(raw), &q); err == nil || err.Code != CodeBadRequest {
			t.Errorf("%s: want bad_request, got %v", raw, err)
		}
	}
	var id ProxyIDParams
	if err := DecodeParams(json.RawMessage(`{"id":"0123456789abcdef","x":1}`), &id); err == nil || err.Code != CodeBadRequest {
		t.Fatalf("unknown field: %v", err)
	}
	for _, name := range []string{"", "home", "home.conf", "дом 2.conf", strings.Repeat("a", MaxNameLen)} {
		if err := ValidateProxyAdd(ProxyAddParams{Text: "x", Name: name}); err != nil {
			t.Errorf("name %q: %v", name, err)
		}
	}
	for _, name := range []string{`C:\wg\home.conf`, "wg/home.conf", "..", "C:home.conf", "a\x00b", "a\nb", strings.Repeat("a", MaxNameLen+1)} {
		if err := ValidateProxyAdd(ProxyAddParams{Text: "x", Name: name}); err == nil || err.Code != CodeBadRequest {
			t.Errorf("name %q: want bad_request, got %v", name, err)
		}
	}
	if err := ValidateProxyAdd(ProxyAddParams{}); err == nil || err.Code != CodeBadRequest {
		t.Errorf("empty text: want bad_request, got %v", err)
	}
	for _, good := range []string{"0123456789abcdef", "sub-0123456789abcdef"} {
		if err := ValidateProxyID(good); err != nil {
			t.Errorf("%s: %v", good, err)
		}
	}
	for _, bad := range []string{"", "0123456789ABCDEF", "0123456789abcde", "sub-", "sub-0123456789abcdef0", "../servers.json", "100.64.0.1"} {
		if err := ValidateProxyID(bad); err == nil || err.Code != CodeBadRequest {
			t.Errorf("%q: want bad_request, got %v", bad, err)
		}
	}
}

func TestMaskAddresses(t *testing.T) {
	if got := MaskAddresses(`{"vpn":"203.0.113.9","core":"1.14.4"}`); got != `{"vpn":"<ip>","core":"1.14.4"}` {
		t.Fatalf("masked %q", got)
	}
	// The errors of the core's CLI name the peers next to their addresses (task 16 of plan 1B: a ping
	// to the exit node without a reply), and tailnet names and IPv6 addresses: none reaches a log.
	for in, want := range map[string]string{
		"sing-box.exe api tailscale ping 100.64.0.7: exit status 1: FATAL[0005] no reply from my-server (100.64.0.7) after 5s": "sing-box.exe api tailscale ping <ip>: exit status 1: FATAL[0005] no reply from <peer> (<ip>) after 5s",
		"pong from box-2 (100.64.0.9) via DERP(fra) in 31ms":                                                                   "pong from <peer> (<ip>) via DERP(fra) in 31ms",
		"lookup my-server.tail1234.ts.net: no such host":                                                                       "lookup <name>.ts.net: no such host",
		"dial fd7a:115c:a1e0::1:7 failed; next fd7a:115c:a1e0:ab12::42 is down":                                                "dial <ip6> failed; next <ip6> is down",
		"exit status 1: FATAL[0000] context deadline exceeded (5s)":                                                            "exit status 1: FATAL[0000] context deadline exceeded (5s)",
	} {
		if got := MaskAddresses(in); got != want {
			t.Errorf("masked %q\n  to %q\nwant %q", in, got, want)
		}
	}
}

func TestScannerTakesLongLines(t *testing.T) {
	long := `{"id":1,"method":"lanes.set","params":{"x":"` + strings.Repeat("a", 200000) + `"}}`
	s := NewScanner(strings.NewReader(long + "\n" + `{"id":2}` + "\n"))
	n := 0
	for s.Scan() {
		n++
	}
	if s.Err() != nil || n != 2 {
		t.Fatalf("lines %d err %v", n, s.Err())
	}
}
