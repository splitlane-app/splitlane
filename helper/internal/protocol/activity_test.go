package protocol

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestActivityJSONShape(t *testing.T) {
	id := goodID
	a := Activity{
		Lanes: ActivityLanes{VPN: Traffic{Down: 3, Up: 4}, Direct: Traffic{Down: 1, Up: 2}},
		Apps: []ActivityApp{
			{Key: id, Kind: KindApp, Name: "Steam", Exe: `C:\Games\Steam\steam.exe`, RuleID: &id, Lane: LaneDirect, Down: 1, Up: 2, Connections: 3},
			{Key: KeySystem, Kind: KindSystem, Lane: LaneVPN},
		},
	}
	b, err := json.Marshal(a)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"lanes":{"vpn":{"down":3,"up":4},"direct":{"down":1,"up":2}},"apps":[` +
		`{"key":"` + id + `","kind":"app","name":"Steam","exe":"C:\\Games\\Steam\\steam.exe","ruleId":"` + id + `","lane":"direct","down":1,"up":2,"connections":3},` +
		`{"key":"system","kind":"system","name":"","exe":"","ruleId":null,"lane":"vpn","down":0,"up":0,"connections":0}]}`
	if string(b) != want {
		t.Fatalf("without detailFor:\n got %s\nwant %s", b, want)
	}
	a.Detail = &ActivityDetail{Key: id, Exes: []string{`C:\Games\Steam\steam.exe`},
		Connections: []ActivityConn{{Host: "store.example", Lane: LaneDirect, Down: 5, Up: 6}}}
	b, _ = json.Marshal(a)
	if !strings.HasSuffix(string(b), `,"detail":{"key":"`+id+`","exes":["C:\\Games\\Steam\\steam.exe"],"connections":[{"host":"store.example","lane":"direct","down":5,"up":6}]}}`) {
		t.Fatalf("with detail: %s", b)
	}
	// nil lists are written as [], never null
	b, _ = json.Marshal(Activity{Detail: &ActivityDetail{Key: KeyUnattributed}})
	want = `{"lanes":{"vpn":{"down":0,"up":0},"direct":{"down":0,"up":0}},"apps":[],"detail":{"key":"unattributed","exes":[],"connections":[]}}`
	if string(b) != want {
		t.Fatalf("empty lists:\n got %s\nwant %s", b, want)
	}
	if got := string(EncodeEvent(EventActivity, Activity{})); !strings.HasPrefix(got, `{"event":"activity","data":{"lanes":`) || !strings.Contains(got, `"apps":[]`) {
		t.Fatalf("event line %q", got)
	}
}

func TestDecodeSubscribe(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", `{"detailFor":""}`} {
		if key, err := DecodeSubscribe(json.RawMessage(raw)); err != nil || key != "" {
			t.Errorf("%q: want no detail, got %q %v", raw, key, err)
		}
	}
	if key, err := DecodeSubscribe(json.RawMessage(`{"detailFor":"C:\\Apps\\Tool\\"}`)); err != nil || key != `C:\Apps\Tool\` {
		t.Fatalf("detailFor: %q %v", key, err)
	}
	longest := strings.Repeat("a", MaxPathLen)
	if key, err := DecodeSubscribe(json.RawMessage(`{"detailFor":"` + longest + `"}`)); err != nil || key != longest {
		t.Fatalf("a key of MaxPathLen bytes must pass: %v", err)
	}
	bad := map[string]string{
		"unknown field": `{"detailFor":"system","all":true}`,
		"too long key":  `{"detailFor":"` + longest + `a"}`,
		"wrong type":    `{"detailFor":5}`,
		"not an object": `["system"]`,
	}
	for name, raw := range bad {
		if key, err := DecodeSubscribe(json.RawMessage(raw)); err == nil || err.Code != CodeBadRequest || key != "" {
			t.Errorf("%s: want bad_request, got %q %v", name, key, err)
		}
	}
}
