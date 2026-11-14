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
	for _, want := range []string{`"reason":null`, `"apps":[]`, `"addresses":{"vpn":null,"direct":null}`, `"exitNodes":[]`, `"selectedExitNode":null`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("state JSON lacks %s: %s", want, b)
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
