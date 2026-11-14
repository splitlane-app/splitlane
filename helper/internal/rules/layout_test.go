package rules

import (
	"reflect"
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
)

func checked(t *testing.T, apps ...protocol.AppRule) []protocol.AppRule {
	t.Helper()
	out, rejected, err := Check(apps, testEnv)
	if err != nil || rejected != nil {
		t.Fatalf("check: %v %v", err, rejected)
	}
	return out
}

func TestCheckRejectsProtectedRules(t *testing.T) {
	_, rejected, err := Check([]protocol.AppRule{
		rule("helper", protocol.LaneDirect, protocol.KindFolder, `C:\Program Files\SplitLane\splitlane-helper.exe`),
		rule("ts", protocol.LaneVPN, protocol.KindFile, `C:\Program Files\Tailscale\tailscaled.exe`),
		rule("ok", protocol.LaneDirect, protocol.KindFile, `C:\Tools\curl-a.exe`),
	}, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := []protocol.RuleRejection{{ID: "helper", Reason: protocol.RejectProtected}, {ID: "ts", Reason: protocol.RejectProtected}}
	if !reflect.DeepEqual(rejected, want) {
		t.Fatalf("rejected %v", rejected)
	}
}

func TestCheckRejectsSameScopeInBothLanes(t *testing.T) {
	_, rejected, _ := Check([]protocol.AppRule{
		rule("a", protocol.LaneDirect, protocol.KindFile, `C:\Tools\curl-a.exe`),
		rule("b", protocol.LaneVPN, protocol.KindFile, `c:\tools\CURL-A.exe`),
	}, testEnv)
	want := []protocol.RuleRejection{{ID: "a", Reason: protocol.RejectDuplicate}, {ID: "b", Reason: protocol.RejectDuplicate}}
	if !reflect.DeepEqual(rejected, want) {
		t.Fatalf("rejected %v", rejected)
	}
	sys := protocol.AppRule{ID: "s1", Matcher: protocol.Matcher{Kind: protocol.KindSystem}, Lane: protocol.LaneDirect}
	svc := rule("s2", protocol.LaneVPN, protocol.KindFile, `C:\Windows\System32\svchost.exe`)
	if _, rejected, _ := Check([]protocol.AppRule{sys, svc}, testEnv); len(rejected) != 2 {
		t.Fatalf("system rule and a svchost file rule in other lanes: %v", rejected)
	}
	same := checked(t, rule("c", protocol.LaneVPN, protocol.KindFile, `C:\Tools\curl-a.exe`),
		rule("d", protocol.LaneVPN, protocol.KindFile, `C:\Tools\curl-a.exe`))
	if len(same) != 2 {
		t.Fatal("the same scope in one lane is allowed")
	}
}

func TestTooBroad(t *testing.T) {
	for _, v := range []string{`C:\`, `D:\`, `C:\Windows\`, `C:\Program Files\`, `C:\Program Files (x86)\`} {
		if !tooBroad(protocol.AppRule{Matcher: protocol.Matcher{Kind: protocol.KindFolder, Value: v}}, testEnv) {
			t.Errorf("%s must be too broad", v)
		}
	}
	for _, v := range []string{`C:\Games\`, `C:\Program Files\Vendor\`} {
		if tooBroad(protocol.AppRule{Matcher: protocol.Matcher{Kind: protocol.KindFolder, Value: v}}, testEnv) {
			t.Errorf("%s must be allowed", v)
		}
	}
}

func TestCheckReportsMalformedInputAsError(t *testing.T) {
	if _, _, err := Check([]protocol.AppRule{rule("a", protocol.LaneVPN, protocol.KindFile, "curl.exe")}, testEnv); err == nil {
		t.Fatal("want an error")
	}
}

func TestDocumentsAlwaysHaveTheStub(t *testing.T) {
	d, v, err := Files(nil, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"version\": 3,\n  \"rules\": [\n    {\n      \"process_path_regex\": [\n        \"^:splitlane-stub$\"\n      ]\n    }\n  ]\n}\n"
	if string(d) != want || string(v) != want {
		t.Fatalf("empty layout:\n%s\n%s", d, v)
	}
}

func TestNestedFileInsideDirectFolder(t *testing.T) {
	apps := checked(t,
		rule("steam", protocol.LaneDirect, protocol.KindFolder, `C:\Games\Steam\steam.exe`),
		rule("game", protocol.LaneVPN, protocol.KindFile, `C:\Games\Steam\steamapps\common\Game\game.exe`))
	d, v, err := Documents(apps, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	wantDirect := Headless{Type: "logical", Mode: "and", Rules: []Headless{
		{ProcessPathRegex: []string{`(?i)^C:\\Games\\Steam\\`}},
		{ProcessPathRegex: []string{`(?i)^C:\\Games\\Steam\\steamapps\\common\\Game\\game\.exe$`}, Invert: true},
	}}
	if !reflect.DeepEqual(d.Rules[1], wantDirect) {
		t.Fatalf("direct rule %+v", d.Rules[1])
	}
	if !reflect.DeepEqual(v.Rules[1], Headless{ProcessPathRegex: []string{`(?i)^C:\\Games\\Steam\\steamapps\\common\\Game\\game\.exe$`}}) {
		t.Fatalf("vpn rule %+v", v.Rules[1])
	}
	raw, _, _ := Files(apps, testEnv)
	if !strings.Contains(string(raw), `"invert": true`) || !strings.Contains(string(raw), `"type": "logical"`) {
		t.Fatalf("file text:\n%s", raw)
	}
}

func TestNestedDirectFileInsideVPNFolder(t *testing.T) {
	apps := checked(t,
		rule("tools", protocol.LaneVPN, protocol.KindFolder, `C:\Tools\bin\tool.exe`),
		rule("curl", protocol.LaneDirect, protocol.KindFile, `C:\Tools\bin\curl-d.exe`))
	d, v, _ := Documents(apps, testEnv)
	if v.Rules[1].Type != "logical" || len(v.Rules[1].Rules) != 2 || !v.Rules[1].Rules[1].Invert {
		t.Fatalf("vpn rule %+v", v.Rules[1])
	}
	if d.Rules[1].Type != "" {
		t.Fatalf("direct rule %+v", d.Rules[1])
	}
}

// unusableRule is a rule that skipped Normalize: Expressions returns nil for it.
func unusableRule(id, lane, kind, exe, value string) protocol.AppRule {
	return protocol.AppRule{ID: id, Exe: exe, Matcher: protocol.Matcher{Kind: kind, Value: value}, Lane: lane}
}

func TestDocumentsAndCompileRejectRulesWithoutExpression(t *testing.T) {
	cases := map[string]protocol.AppRule{
		"empty folder value":  unusableRule("bad", protocol.LaneDirect, protocol.KindFolder, `C:\Games\Steam\steam.exe`, ""),
		"empty file value":    unusableRule("bad", protocol.LaneDirect, protocol.KindFile, `C:\Games\Steam\steam.exe`, ""),
		"unparsable package":  unusableRule("bad", protocol.LaneVPN, protocol.KindPackage, `C:\Games\Steam\steam.exe`, "no-separators"),
		"unknown kind":        unusableRule("bad", protocol.LaneVPN, "bogus", `C:\Games\Steam\steam.exe`, `C:\Games\Steam\`),
		"empty folder in vpn": unusableRule("bad", protocol.LaneVPN, protocol.KindFolder, `C:\Games\Steam\steam.exe`, ""),
	}
	good := checked(t, rule("good", protocol.LaneVPN, protocol.KindFile, `C:\Tools\curl-a.exe`))
	for name, bad := range cases {
		t.Run(name, func(t *testing.T) {
			apps := append([]protocol.AppRule{bad}, good...)
			if _, _, err := Documents(apps, testEnv); err == nil || !strings.Contains(err.Error(), "rule bad") {
				t.Errorf("Documents: want an error naming the rule, got %v", err)
			}
			if _, _, err := Files(apps, testEnv); err == nil {
				t.Error("Files: want an error")
			}
			if _, err := Compile(apps, testEnv); err == nil || !strings.Contains(err.Error(), "rule bad") {
				t.Errorf("Compile: want an error naming the rule, got %v", err)
			}
		})
	}
}

func TestDocumentsRejectsNestedRuleWithoutExpression(t *testing.T) {
	outer := checked(t, rule("outer", protocol.LaneDirect, protocol.KindFolder, `C:\Games\Steam\steam.exe`))
	// inside outer by its exe, but its package value cannot be parsed
	inner := unusableRule("inner", protocol.LaneVPN, protocol.KindPackage, `C:\Games\Steam\apps\app.exe`, "no-separators")
	d, v, err := Documents(append(outer, inner), testEnv)
	if err == nil || !strings.Contains(err.Error(), "rule inner") {
		t.Fatalf("want an error naming the nested rule, got %v", err)
	}
	// an inverted part without expressions would exclude nothing and write {"invert": true}
	for _, doc := range []RuleSet{d, v} {
		for _, r := range doc.Rules {
			for _, part := range r.Rules {
				if part.Invert && len(part.ProcessPathRegex) == 0 {
					t.Fatalf("inverted part without expressions in %+v", doc)
				}
			}
		}
	}
}

func TestNestingChainAndLaneOf(t *testing.T) {
	apps := checked(t,
		rule("a", protocol.LaneDirect, protocol.KindFolder, `C:\Nest\outer\a.exe`),
		rule("b", protocol.LaneVPN, protocol.KindFolder, `C:\Nest\outer\inner\b.exe`),
		rule("c", protocol.LaneDirect, protocol.KindFile, `C:\Nest\outer\inner\deep\c.exe`))
	d, v, _ := Documents(apps, testEnv)
	// a excludes b only (c is in a's own lane), b excludes c
	if got := len(d.Rules[1].Rules); got != 2 || !strings.Contains(d.Rules[1].Rules[1].ProcessPathRegex[0], `inner\\`) {
		t.Fatalf("a: %+v", d.Rules[1])
	}
	if got := len(v.Rules[1].Rules); got != 2 || !strings.Contains(v.Rules[1].Rules[1].ProcessPathRegex[0], `c\.exe`) {
		t.Fatalf("b: %+v", v.Rules[1])
	}
	set, err := Compile(apps, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		`C:\Nest\outer\x.exe`:            protocol.LaneDirect,
		`C:\Nest\outer\inner\y.exe`:      protocol.LaneVPN,
		`C:\NEST\OUTER\INNER\DEEP\C.EXE`: protocol.LaneDirect,
		`C:\Nest\outer\inner\deep\z.exe`: protocol.LaneVPN,
	}
	for path, want := range cases {
		if got, ok := set.LaneOf(path); !ok || got != want {
			t.Errorf("%s: got %q %v, want %s", path, got, ok, want)
		}
	}
	if _, ok := set.LaneOf(`C:\Elsewhere\x.exe`); ok {
		t.Error("a path outside every rule has no lane")
	}
}

func TestRuleOfPrecedence(t *testing.T) {
	apps := checked(t,
		rule("steam", protocol.LaneDirect, protocol.KindFolder, `C:\Games\Steam\steam.exe`),
		rule("game", protocol.LaneVPN, protocol.KindFile, `C:\Games\Steam\steamapps\common\Game\game.exe`),
		rule("tools", protocol.LaneVPN, protocol.KindFolder, `C:\Games\Steam\tools\t.exe`))
	set, err := Compile(apps, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		`C:\Games\Steam\steam.exe`:                          "steam",
		`C:\Games\Steam\steamapps\common\Game\launcher.exe`: "steam",
		`C:\Games\Steam\steamapps\common\Game\game.exe`:     "game",
		`c:\games\steam\STEAMAPPS\common\game\GAME.EXE`:     "game",
		`C:\Games\Steam\tools\sub\helper.exe`:               "tools",
	}
	for path, want := range cases {
		r, ok := set.RuleOf(path)
		if !ok || r.ID != want {
			t.Errorf("%s: got %q %v, want %s", path, r.ID, ok, want)
			continue
		}
		if lane, _ := set.LaneOf(path); lane != r.Lane {
			t.Errorf("%s: RuleOf gives lane %s, LaneOf %s", path, r.Lane, lane)
		}
	}
	if r, _ := set.RuleOf(`C:\Games\Steam\steamapps\common\Game\game.exe`); r != apps[1] {
		t.Errorf("RuleOf must return the normalized rule, got %+v", r)
	}
	if r, ok := set.RuleOf(`C:\Elsewhere\x.exe`); ok || r.ID != "" {
		t.Errorf("a path outside every rule has no rule, got %+v", r)
	}
	var none *Set
	if _, ok := none.RuleOf(`C:\Games\Steam\steam.exe`); ok {
		t.Error("a nil set has no rules")
	}
}
