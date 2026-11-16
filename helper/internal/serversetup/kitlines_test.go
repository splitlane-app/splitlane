package serversetup

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseKitLineSteps(t *testing.T) {
	cases := []struct {
		line string
		want *Event // nil: the line is skipped
	}{
		{`{"step":"singbox","state":"start","detail":"downloading sing-box 1.14.4 (amd64)"}`, &Event{"install", "start", "downloading sing-box 1.14.4 (amd64)"}},
		{`{"step":"singbox","state":"skip","detail":"sing-box 1.14.4 is already installed"}`, &Event{"install", "skip", "sing-box 1.14.4 is already installed"}},
		{`{"step":"config","state":"done","detail":"ways in: vless, hy2"}`, &Event{"install", "done", "ways in: vless, hy2"}},
		{`{"step":"service","state":"done","detail":"the sing-box service is running"}`, &Event{"install", "done", "the sing-box service is running"}},
		{`{"step":"harden","state":"warn","detail":"hardening failed"}`, &Event{"harden", "warn", "hardening failed"}},
		{`{"step":"harden","state":"done","detail":""}`, &Event{"harden", "done", ""}},
		{`{"step":"subscription","state":"done","detail":"Caddy serves it on port 8443"}`, &Event{"subscription", "done", "Caddy serves it on port 8443"}},
		{`  {"step":"service","state":"start","detail":"x"}  ` + "\r", &Event{"install", "start", "x"}},
		// The detail goes through connector.Mask.
		{`{"step":"subscription","state":"warn","detail":"see https://192-0-2-10.sslip.io:8443/Abcdef0123456789Token/sub"}`,
			&Event{"subscription", "warn", "see https://192-0-2-10.sslip.io:8443/•••/sub"}},
		{`{"step":"config","state":"done","detail":"vless://11111111-2222-4333-8444-555555555555@192.0.2.10:443"}`,
			&Event{"install", "done", "vless://•••@192.0.2.10:443"}},
		// The kit's own system step: SplitLane checked the system before the kit.
		{`{"step":"system","state":"start","detail":"checking the system"}`, nil},
		{`{"step":"system","state":"done","detail":"debian 12, amd64; ports 443/tcp"}`, nil},
		// Steps and states it does not know.
		{`{"step":"reboot","state":"start","detail":""}`, nil},
		{`{"step":"singbox","state":"fail","detail":""}`, nil},
		{`{"step":"singbox","detail":""}`, nil},
		{`{"step":"","state":"start"}`, nil},
		{`{"step":7,"state":"start"}`, nil},
	}
	for _, c := range cases {
		got := parseKitLine([]byte(c.line))
		if got.result != nil || got.fail != nil {
			t.Errorf("%s: %+v", c.line, got)
			continue
		}
		if !reflect.DeepEqual(got.event, c.want) {
			t.Errorf("%s: %+v, want %+v", c.line, got.event, c.want)
		}
	}
}

// TestParseKitLineNoise (Review Focus 4): whatever is not a line of the kit's --json is skipped.
func TestParseKitLineNoise(t *testing.T) {
	lines := []string{
		"",
		"   ",
		"Reading package lists...",
		"debconf: unable to initialize frontend: Dialog",
		"dpkg: warning: files list file for package 'x' missing",
		"W: Some index files failed to download.",
		`{"step":"singbox","state":"start","detail":"downlo`,
		`{"result":{"links":["vless://x@192.0.2.10:443"],"subscription":null,"warnings":[]`,
		`{"error":{"code":"ports_busy","message":"taken"`,
		`{"hello":"world"}`,
		`{}`,
		`[]`,
		`"result"`,
		`null`,
		`42`,
		`{"result":null}`,
		`{"error":null}`,
		`{"result":{"links":"vless://x"}}`,
		`{"error":"ports_busy"}`,
		`{"result":{"links":["a"]},"error":{"code":"kit_failed","message":"both"}}`,
		`{"step":"config","state":"done","result":{"links":["a"]}}`,
	}
	for _, l := range lines {
		if got := parseKitLine([]byte(l)); got.event != nil || got.result != nil || got.fail != nil {
			t.Errorf("%q: %+v", l, got)
		}
	}
}

func TestParseKitLineResult(t *testing.T) {
	line := `{"result":{"links":["vless://a@192.0.2.10:443#A","hy2://b@192.0.2.10:443#B"],"wireguard":null,` +
		`"subscription":"https://192-0-2-10.sslip.io:8443/Abcdef0123456789Token/sub","ports":{"vless":443,"hy2":443,"sub":8443},` +
		`"warnings":["harden_failed","sub_port80_busy"]}}`
	got := parseKitLine([]byte(line))
	if got.result == nil || got.event != nil || got.fail != nil {
		t.Fatalf("%+v", got)
	}
	want := &kitResult{
		Links:        []string{"vless://a@192.0.2.10:443#A", "hy2://b@192.0.2.10:443#B"},
		Subscription: "https://192-0-2-10.sslip.io:8443/Abcdef0123456789Token/sub",
		Warnings:     []string{"harden_failed", "sub_port80_busy"},
	}
	if !reflect.DeepEqual(got.result, want) {
		t.Fatalf("%+v, want %+v", got.result, want)
	}

	cases := []struct {
		name, line string
		want       kitResult
	}{
		{"no subscription", `{"result":{"links":["vless://a@192.0.2.10:443"],"wireguard":null,"subscription":null,"ports":{},"warnings":[]}}`,
			kitResult{Links: []string{"vless://a@192.0.2.10:443"}}},
		{"an http subscription is not one", `{"result":{"links":["vless://a@h:1"],"subscription":"http://h/x","warnings":[]}}`,
			kitResult{Links: []string{"vless://a@h:1"}}},
		{"empty, multi-line and blank links are dropped", `{"result":{"links":["","  ","a\nb","vless://a@h:1"," hy2://b@h:1 "],"subscription":null}}`,
			kitResult{Links: []string{"vless://a@h:1", "hy2://b@h:1"}}},
		{"odd warnings are dropped, repeats count once", `{"result":{"links":["vless://a@h:1"],"warnings":["harden_failed","","Bad Code","harden_failed","sub_skipped"]}}`,
			kitResult{Links: []string{"vless://a@h:1"}, Warnings: []string{"harden_failed", "sub_skipped"}}},
		{"wireguard only", `{"result":{"links":[],"wireguard":"[Interface]\nPrivateKey = x","subscription":null}}`,
			kitResult{}},
	}
	for _, c := range cases {
		got := parseKitLine([]byte(c.line))
		if got.result == nil {
			t.Errorf("%s: %+v", c.name, got)
			continue
		}
		if !reflect.DeepEqual(*got.result, c.want) {
			t.Errorf("%s: %+v, want %+v", c.name, *got.result, c.want)
		}
	}
}

func TestParseKitLineError(t *testing.T) {
	cases := []struct {
		line string
		want Failure
	}{
		{`{"error":{"code":"os_unsupported","message":"centos 9 is not supported"}}`, Failure{CodeOSUnsupported, "centos 9 is not supported"}},
		{`{"error":{"code":"download_failed","message":"could not download sing-box; nothing was installed"}}`, Failure{CodeDownloadFailed, "could not download sing-box; nothing was installed"}},
		{`{"error":{"code":"ports_busy","message":"--auto-ports: every port it tries for --vless-port is taken"}}`, Failure{CodePortsBusy, "--auto-ports: every port it tries for --vless-port is taken"}},
		{`{"error":{"code":"kit_failed","message":"the kit stopped unexpectedly (exit code 2)"}}`, Failure{CodeKitFailed, "the kit stopped unexpectedly (exit code 2)"}},
		// Codes the window does not know become kit_failed.
		{`{"error":{"code":"no_space","message":"disk full"}}`, Failure{CodeKitFailed, "disk full"}},
		{`{"error":{"code":"timeout","message":"x"}}`, Failure{CodeKitFailed, "x"}},
		{`{"error":{"message":"no code"}}`, Failure{CodeKitFailed, "no code"}},
		{`{"error":{}}`, Failure{CodeKitFailed, ""}},
		// The message is masked: a link or a subscription address never reaches the window whole.
		{`{"error":{"code":"kit_failed","message":"bad link vless://11111111-2222-4333-8444-555555555555@192.0.2.10:443 and https://h.example.com:8443/Abcdef0123456789Token/sub"}}`,
			Failure{CodeKitFailed, "bad link vless://•••@192.0.2.10:443 and https://h.example.com:8443/•••/sub"}},
	}
	for _, c := range cases {
		got := parseKitLine([]byte(c.line))
		if got.fail == nil || got.event != nil || got.result != nil {
			t.Errorf("%s: %+v", c.line, got)
			continue
		}
		if *got.fail != c.want {
			t.Errorf("%s: %+v, want %+v", c.line, *got.fail, c.want)
		}
	}
}

func TestEachLine(t *testing.T) {
	long := strings.Repeat("x", 100)
	cases := []struct {
		name, in string
		want     []string
	}{
		{"lines", "a\nb\n", []string{"a\n", "b\n"}},
		{"last line without a newline", "a\nb", []string{"a\n", "b"}},
		{"empty lines", "\n\na\n", []string{"\n", "\n", "a\n"}},
		{"nothing", "", nil},
		{"a long line is skipped whole", "a\n" + long + "\nb\n", []string{"a\n", "b\n"}},
		{"a long last line", "a\n" + long, []string{"a\n"}},
		{"a line at the limit", strings.Repeat("y", 31) + "\n", []string{strings.Repeat("y", 31) + "\n"}},
	}
	for _, c := range cases {
		var got []string
		// A small reader buffer: lines longer than it come in pieces.
		err := eachLine(strings.NewReader(c.in), 32, func(l []byte) { got = append(got, string(l)) })
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestTail(t *testing.T) {
	tl := &tail{max: 16}
	tl.Write([]byte("0123456789"))
	tl.Write([]byte("abcdefghij"))
	if got := string(tl.bytes()); got != "456789abcdefghij" {
		t.Fatalf("%q", got)
	}
	tl.Write([]byte(strings.Repeat("z", 40)))
	if got := string(tl.bytes()); got != strings.Repeat("z", 16) {
		t.Fatalf("%q", got)
	}
}

func TestLastLines(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"one\n", "one"},
		{"one\ntwo\nthree\n", "two | three"},
		{"one\ntwo\n\n  \nthree\n\n", "two | three"},
		{"E: x\r\nE: y\r\n", "E: x | E: y"},
		{"\x1b[31mred\x1b[0m\n", "[31mred [0m"},
	}
	for _, c := range cases {
		if got := lastLines([]byte(c.in), 2); got != c.want {
			t.Errorf("%q: %q, want %q", c.in, got, c.want)
		}
	}
}
