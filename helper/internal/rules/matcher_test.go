package rules

import (
	"regexp"
	"strings"
	"testing"

	"splitlane/helper/internal/protocol"
)

var testEnv = Env{
	WindowsDir:      `C:\Windows`,
	ProgramFiles:    `C:\Program Files`,
	ProgramFilesX86: `C:\Program Files (x86)`,
	UserDirs: []string{`C:\Users\u\Desktop`, `C:\Users\u\Downloads`, `C:\Users\u\Documents`,
		`C:\Users\u\AppData\Local\Temp`},
	Protected: []string{`C:\Program Files\SplitLane\splitlane-helper.exe`, `C:\Program Files\SplitLane\sing-box.exe`,
		`C:\Program Files\Tailscale\tailscaled.exe`, `C:\Program Files\Tailscale\tailscale-ipn.exe`},
	Describe: func(exe string) string {
		if strings.EqualFold(exe, `C:\Games\Steam\steam.exe`) {
			return "Steam Client"
		}
		return ""
	},
}

func rule(id, lane, kind, exe string) protocol.AppRule {
	return protocol.AppRule{ID: id, Exe: exe, Matcher: protocol.Matcher{Kind: kind}, Lane: lane}
}

func normalized(t *testing.T, kind, exe string) protocol.AppRule {
	t.Helper()
	n, err := Normalize(rule("r", protocol.LaneVPN, kind, exe), testEnv)
	if err != nil {
		t.Fatalf("%s %s: %v", kind, exe, err)
	}
	return n
}

func matches(t *testing.T, a protocol.AppRule, path string) bool {
	t.Helper()
	for _, e := range Expressions(a, testEnv) {
		if regexp.MustCompile(e).MatchString(path) {
			return true
		}
	}
	return false
}

func TestFolderValue(t *testing.T) {
	cases := []struct{ exe, kind, value string }{
		{`C:\Games\Steam\steam.exe`, protocol.KindFolder, `C:\Games\Steam\`},
		{`C:\Users\u\AppData\Local\Discord\app-1.0.9\Discord.exe`, protocol.KindFolder, `C:\Users\u\AppData\Local\Discord\`},
		{`C:\Apps\Tool\124.0.6367.91\tool.exe`, protocol.KindFolder, `C:\Apps\Tool\`},
		{`C:\Apps\Tool\v2\tool.exe`, protocol.KindFolder, `C:\Apps\Tool\v2\`},
		{`C:\Apps\Tool\1.2.3.4.5\tool.exe`, protocol.KindFolder, `C:\Apps\Tool\1.2.3.4.5\`},
		// shared folders turn the rule into a file rule
		{`D:\tool.exe`, protocol.KindFile, `D:\tool.exe`},
		{`C:\Windows\System32\curl.exe`, protocol.KindFile, `C:\Windows\System32\curl.exe`},
		{`C:\Program Files\app.exe`, protocol.KindFile, `C:\Program Files\app.exe`},
		{`C:\Program Files (x86)\app.exe`, protocol.KindFile, `C:\Program Files (x86)\app.exe`},
		{`C:\Program Files\1.2\app.exe`, protocol.KindFile, `C:\Program Files\1.2\app.exe`},
		{`C:\Users\u\Downloads\setup.exe`, protocol.KindFile, `C:\Users\u\Downloads\setup.exe`},
		{`C:\Users\u\Desktop\tool.exe`, protocol.KindFile, `C:\Users\u\Desktop\tool.exe`},
		{`C:\Users\u\Documents\tool.exe`, protocol.KindFile, `C:\Users\u\Documents\tool.exe`},
		{`C:\Users\u\AppData\Local\Temp\x.exe`, protocol.KindFile, `C:\Users\u\AppData\Local\Temp\x.exe`},
		// a subfolder of a shared folder is a program folder of its own
		{`C:\Users\u\Downloads\tool\tool.exe`, protocol.KindFolder, `C:\Users\u\Downloads\tool\`},
	}
	for _, c := range cases {
		n := normalized(t, protocol.KindFolder, c.exe)
		if n.Matcher.Kind != c.kind || n.Matcher.Value != c.value {
			t.Errorf("%s: got %s %q, want %s %q", c.exe, n.Matcher.Kind, n.Matcher.Value, c.kind, c.value)
		}
	}
}

func TestFolderRegexMatchesWholeFolderOnly(t *testing.T) {
	n := normalized(t, protocol.KindFolder, `C:\Games\Steam\steam.exe`)
	if !matches(t, n, `c:\games\steam\bin\steamwebhelper.exe`) {
		t.Error("a file in a subfolder must match, in another case too")
	}
	if matches(t, n, `C:\Games\SteamTools\x.exe`) {
		t.Error("a sibling folder with the same prefix must not match")
	}
}

func TestFileRegexWithSpacesBracketsCyrillic(t *testing.T) {
	paths := []string{
		`C:\Tools\Dir With Spaces\curl-space.exe`,
		`C:\Tools\Brackets (x86) [v1]\curl-brackets.exe`,
		`C:\Tools\Кириллица Папка\curl-кириллица.exe`,
	}
	for _, p := range paths {
		n := normalized(t, protocol.KindFile, p)
		if !matches(t, n, p) || !matches(t, n, strings.ToUpper(p)) {
			t.Errorf("%s: the expression %v must match the path in any case", p, Expressions(n, testEnv))
		}
		if matches(t, n, p+".bak") || matches(t, n, `X`+p[1:]) {
			t.Errorf("%s: the expression must match the whole path only", p)
		}
	}
}

func TestPackageRule(t *testing.T) {
	exe := `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.21.3231.0_x64__8wekyb3d8bbwe\WindowsTerminal.exe`
	n := normalized(t, protocol.KindPackage, exe)
	if n.Matcher.Value != `Microsoft.WindowsTerminal_8wekyb3d8bbwe/WindowsTerminal.exe` {
		t.Fatalf("value %q", n.Matcher.Value)
	}
	if !matches(t, n, `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.22.1.0_x64__8wekyb3d8bbwe\WindowsTerminal.exe`) {
		t.Error("another version of the same package must match")
	}
	if !matches(t, n, `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.22.1.0_x64__8wekyb3d8bbwe\sub\WindowsTerminal.exe`) {
		t.Error("the same file in a subfolder of the package must match")
	}
	if matches(t, n, `C:\Program Files\WindowsApps\Microsoft.WindowsTerminal_1.22.1.0_x64__aaaaaaaaaaaaa\WindowsTerminal.exe`) {
		t.Error("another publisher must not match")
	}
	if _, err := Normalize(rule("p", protocol.LaneVPN, protocol.KindPackage, `C:\Tools\a.exe`), testEnv); err == nil {
		t.Error("a package rule outside WindowsApps must be an error")
	}
}

func TestSystemRule(t *testing.T) {
	n, err := Normalize(protocol.AppRule{ID: "s", Matcher: protocol.Matcher{Kind: protocol.KindSystem}, Lane: protocol.LaneDirect}, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	if n.Matcher.Value != "" || n.Name != "svchost" {
		t.Fatalf("system rule %+v", n)
	}
	if !matches(t, n, `C:\WINDOWS\system32\svchost.exe`) || !matches(t, n, `:System`) {
		t.Error("svchost.exe and the pseudo path :System must match")
	}
	if matches(t, n, `C:\Windows\explorer.exe`) || matches(t, n, `:System Idle Process`) {
		t.Error("nothing else may match")
	}
}

func TestNormalizeRejectsMalformedInput(t *testing.T) {
	bad := []protocol.AppRule{
		rule("a", protocol.LaneVPN, protocol.KindFile, `tools\curl.exe`),
		rule("b", protocol.LaneVPN, protocol.KindFile, `\\server\share\curl.exe`),
		rule("c", protocol.LaneVPN, protocol.KindFile, `C:\Tools\script.cmd`),
		rule("d", protocol.LaneVPN, "glob", `C:\Tools\curl.exe`),
		{ID: "e", Exe: `C:\Tools\curl.exe`, Matcher: protocol.Matcher{Kind: protocol.KindFile, Value: `C:\Other\curl.exe`}, Lane: protocol.LaneVPN},
		{ID: "f", Matcher: protocol.Matcher{Kind: protocol.KindSystem, Value: "x"}, Lane: protocol.LaneVPN},
	}
	for _, a := range bad {
		if _, err := Normalize(a, testEnv); err == nil {
			t.Errorf("rule %s must be rejected as malformed", a.ID)
		}
	}
}

func TestNormalizeKeepsAnUnchangedRule(t *testing.T) {
	first := normalized(t, protocol.KindFolder, `C:\Games\Steam\steam.exe`)
	again, err := Normalize(first, testEnv)
	if err != nil || again != first {
		t.Fatalf("an unchanged rule must normalize to itself: %v %+v", err, again)
	}
}

func TestNameComesFromDescriptionOrFileName(t *testing.T) {
	if n := normalized(t, protocol.KindFolder, `C:\Games\Steam\steam.exe`); n.Name != "Steam Client" {
		t.Errorf("name %q", n.Name)
	}
	if n := normalized(t, protocol.KindFile, `C:\Tools\curl-a.exe`); n.Name != "curl-a" {
		t.Errorf("name %q", n.Name)
	}
	r := rule("n", protocol.LaneVPN, protocol.KindFile, `C:\Tools\curl-a.exe`)
	r.Name = "Мой curl"
	if n, _ := Normalize(r, testEnv); n.Name != "Мой curl" {
		t.Errorf("a name from the window must stay, got %q", n.Name)
	}
}

func TestBuiltinExpression(t *testing.T) {
	re := regexp.MustCompile(BuiltinExpression(`C:\code\router\helper\bin\splitlane-helper.exe`))
	if !re.MatchString(`C:\CODE\ROUTER\HELPER\BIN\SPLITLANE-HELPER.EXE`) || re.MatchString(`C:\code\router\helper\bin\splitlane-helper.exe.old`) {
		t.Fatal("built-in expression must match the exact path in any case")
	}
}
