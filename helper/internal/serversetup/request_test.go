package serversetup

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// focusPassword is the password of the plan's Review Focus 1: quotes, a dollar, a backslash, a space,
// an exclamation mark and Cyrillic letters, none of which may go through a shell.
const focusPassword = "p'a\"s$ s\\w!рд"

// requestLine is a request as the window sends it, with the password JSON-encoded.
func requestLine(t *testing.T, fields map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestParseRequestHosts(t *testing.T) {
	// Review Focus 3 and Ruling 1: the host is bare; a port comes only from "port".
	cases := []struct {
		host string
		want string // "" means bad_input
	}{
		{"198.51.100.7", "198.51.100.7"},
		{"  198.51.100.7\t", "198.51.100.7"},
		{"2001:db8::7", "2001:db8::7"},
		{"[2001:db8::7]", "2001:db8::7"},
		{" [2001:db8::7] ", "2001:db8::7"},
		{"::ffff:198.51.100.7", "::ffff:198.51.100.7"},
		{"vps.example.com", "vps.example.com"},
		{"  vps.example.com  ", "vps.example.com"},
		{"VPS-1.Example.com", "VPS-1.Example.com"},
		{"myvps", "myvps"},
		{"127.0.0.1", "127.0.0.1"},
		{strings.Repeat("a", 63) + ".example.com", strings.Repeat("a", 63) + ".example.com"},

		{"198.51.100.7:2222", ""},
		{"[2001:db8::7]:22", ""},
		{"vps.example.com:22", ""},
		{"http://vps.example.com", ""},
		{"https://198.51.100.7/", ""},
		{"a b", ""},
		{"", ""},
		{"   ", ""},
		{"[198.51.100.7]", ""},
		{"[vps.example.com]", ""},
		{"[2001:db8::7", ""},
		{"fe80::1%eth0", ""},
		{"[fe80::1%25eth0]", ""},
		{"198.51.100.300", ""},
		{"198.51.100", ""},
		{"010.1.1.1", ""},
		{"12345", ""},
		{"-vps.example.com", ""},
		{"vps-.example.com", ""},
		{"vps..example.com", ""},
		{".example.com", ""},
		{"vps.example.com.", ""},
		{strings.Repeat("a", 64) + ".example.com", ""},
		{strings.Repeat("abcdefghi.", 26) + "com", ""}, // 263 characters
		{"vps_1.example.com", ""},
		{"пример.рф", ""},
		{"root@vps.example.com", ""},
		{"vps.example.com/x", ""},
	}
	for _, c := range cases {
		r, f := ParseRequest(requestLine(t, map[string]any{"host": c.host, "user": "root", "password": "x"}))
		if c.want == "" {
			if f == nil || f.Code != CodeBadInput || f.Detail != "host" {
				t.Errorf("host %q: %+v, want bad_input host", c.host, f)
			}
			continue
		}
		if f != nil {
			t.Errorf("host %q: %+v", c.host, f)
			continue
		}
		if r.Host != c.want || r.Port != 22 {
			t.Errorf("host %q: %q port %d, want %q port 22", c.host, r.Host, r.Port, c.want)
		}
	}
}

func TestParseRequestPorts(t *testing.T) {
	cases := []struct {
		port string // the raw JSON value; "" leaves the field out
		want int    // 0 means bad_input
	}{
		{"", 22},
		{"null", 22},
		{"22", 22},
		{"2222", 2222},
		{"1", 1},
		{"65535", 65535},
		{"0", 0},
		{"70000", 0},
		{"65536", 0},
		{"-1", 0},
		{"22.5", 0},
		{"2.2e1", 0},
		{`"22"`, 0},
		{"true", 0},
		{"99999999999999999999", 0},
	}
	for _, c := range cases {
		line := `{"host":"198.51.100.7","user":"root","password":"x"`
		if c.port != "" {
			line += `,"port":` + c.port
		}
		line += "}"
		r, f := ParseRequest([]byte(line))
		if c.want == 0 {
			if f == nil || f.Code != CodeBadInput || f.Detail != "port" {
				t.Errorf("port %s: %+v, want bad_input port", c.port, f)
			}
			continue
		}
		if f != nil || r.Port != c.want {
			t.Errorf("port %s: %d %+v, want %d", c.port, r.Port, f, c.want)
		}
	}
}

func TestParseRequestUsers(t *testing.T) {
	cases := []struct {
		user string
		ok   bool
	}{
		{"root", true},
		{" root ", true},
		{"deploy_user", true},
		{"_svc", true},
		{"a-b", true},
		{"u1", true},
		{strings.Repeat("a", 32), true},
		{strings.Repeat("a", 33), false},
		{"", false},
		{"Root", false},
		{"1user", false},
		{"-x", false},
		{"us er", false},
		{"user$", false},
		{"root;id", false},
		{"юзер", false},
	}
	for _, c := range cases {
		r, f := ParseRequest(requestLine(t, map[string]any{"host": "198.51.100.7", "user": c.user, "password": "x"}))
		if !c.ok {
			if f == nil || f.Code != CodeBadInput || f.Detail != "user" {
				t.Errorf("user %q: %+v, want bad_input user", c.user, f)
			}
			continue
		}
		if f != nil || r.User != strings.TrimSpace(c.user) {
			t.Errorf("user %q: %q %+v", c.user, r.User, f)
		}
	}
	// A missing user is not root by default: the window always sends one.
	if _, f := ParseRequest([]byte(`{"host":"198.51.100.7","password":"x"}`)); f == nil || f.Detail != "user" {
		t.Errorf("no user: %+v", f)
	}
}

func TestParseRequestCredentials(t *testing.T) {
	const key = "-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjEAAAAA\n-----END OPENSSH PRIVATE KEY-----\n"
	t.Run("password", func(t *testing.T) {
		r, f := ParseRequest(requestLine(t, map[string]any{"host": "198.51.100.7", "port": 22, "user": "root", "password": focusPassword}))
		if f != nil {
			t.Fatal(f)
		}
		if !bytes.Equal(r.Password, []byte(focusPassword)) || r.Key != nil || r.Passphrase != nil {
			t.Fatalf("%q %q %q", r.Password, r.Key, r.Passphrase)
		}
	})
	t.Run("password escapes", func(t *testing.T) {
		// Every JSON escape, a surrogate pair and raw UTF-8 come out as the bytes they stand for.
		line := `{"host":"198.51.100.7","user":"root","password":"p'a\"s$ s\\w!` + jsonU(0x0440) + jsonU(0x0434) +
			`\/\t\b\f` + jsonU(0xd83d) + jsonU(0xde00) + `é"}`
		r, f := ParseRequest([]byte(line))
		if f != nil {
			t.Fatal(f)
		}
		if want := "p'a\"s$ s\\w!рд/\t\b\f😀é"; string(r.Password) != want {
			t.Fatalf("%q, want %q", r.Password, want)
		}
	})
	t.Run("key", func(t *testing.T) {
		r, f := ParseRequest(requestLine(t, map[string]any{"host": "198.51.100.7", "user": "root", "key": key}))
		if f != nil {
			t.Fatal(f)
		}
		if string(r.Key) != key || r.Password != nil || r.Passphrase != nil {
			t.Fatalf("%q %q %q", r.Key, r.Password, r.Passphrase)
		}
	})
	t.Run("key with passphrase", func(t *testing.T) {
		r, f := ParseRequest(requestLine(t, map[string]any{"host": "198.51.100.7", "user": "root", "key": key, "passphrase": focusPassword}))
		if f != nil {
			t.Fatal(f)
		}
		if string(r.Key) != key || string(r.Passphrase) != focusPassword {
			t.Fatalf("%q %q", r.Key, r.Passphrase)
		}
	})
	t.Run("empty fields are absent", func(t *testing.T) {
		r, f := ParseRequest([]byte(`{"host":"198.51.100.7","user":"root","password":"","key":` + jsonString(key) + `,"passphrase":""}`))
		if f != nil {
			t.Fatal(f)
		}
		if r.Password != nil || r.Passphrase != nil || string(r.Key) != key {
			t.Fatalf("%q %q %q", r.Password, r.Key, r.Passphrase)
		}
		r, f = ParseRequest([]byte(`{"host":"198.51.100.7","user":"root","password":"x","key":null}`))
		if f != nil || string(r.Password) != "x" {
			t.Fatalf("%q %+v", r.Password, f)
		}
	})

	bad := []struct {
		name, line, detail string
	}{
		{"neither", `{"host":"198.51.100.7","user":"root"}`, "credentials"},
		{"empty password", `{"host":"198.51.100.7","user":"root","password":""}`, "credentials"},
		{"both", `{"host":"198.51.100.7","user":"root","password":"x","key":` + jsonString(key) + `}`, "credentials"},
		{"passphrase with a password", `{"host":"198.51.100.7","user":"root","password":"x","passphrase":"y"}`, "credentials"},
		{"passphrase alone", `{"host":"198.51.100.7","user":"root","passphrase":"y"}`, "credentials"},
		{"newline in the password", `{"host":"198.51.100.7","user":"root","password":"a\nb"}`, "password"},
		{"carriage return in the password", `{"host":"198.51.100.7","user":"root","password":"a\rb"}`, "password"},
		{"NUL in the password", `{"host":"198.51.100.7","user":"root","password":"a\u0000b"}`, "password"},
		{"password not a string", `{"host":"198.51.100.7","user":"root","password":123}`, "password"},
		{"lone surrogate", `{"host":"198.51.100.7","user":"root","password":"\ud83d"}`, "password"},
		{"reversed surrogates", `{"host":"198.51.100.7","user":"root","password":"\ude00\ud83d"}`, "password"},
		{"invalid UTF-8", "{\"host\":\"198.51.100.7\",\"user\":\"root\",\"password\":\"a\xffb\"}", "password"},
		{"key too long", `{"host":"198.51.100.7","user":"root","key":"` + strings.Repeat("k", 16<<10+1) + `"}`, "key"},
		{"unknown field", `{"host":"198.51.100.7","user":"root","password":"x","pass":"y"}`, "request"},
		{"field of another case", `{"Host":"198.51.100.7","user":"root","password":"x"}`, "request"},
		{"not JSON", `host=198.51.100.7`, "request"},
		{"truncated", `{"host":"198.51.100.7","user":"root","password":"x"`, "request"},
		{"trailing data", `{"host":"198.51.100.7","user":"root","password":"x"} {}`, "request"},
		{"an array", `[]`, "request"},
		{"null", `null`, "request"},
		{"empty", ``, "request"},
		{"too long", `{"host":"198.51.100.7","user":"root","password":"` + strings.Repeat("x", 64<<10) + `"}`, "request"},
		// Ruling 14: a name given twice is refused (json.Unmarshal would keep the last value and drop
		// the copy of the first unwiped).
		{"the password twice", `{"host":"198.51.100.7","user":"root","password":"a","password":"b"}`, "request"},
		{"the key twice", `{"host":"198.51.100.7","user":"root","key":"a","key":"b"}`, "request"},
		{"the host twice", `{"host":"198.51.100.7","host":"192.0.2.1","user":"root","password":"a"}`, "request"},
		{"the password twice, once escaped", `{"host":"198.51.100.7","user":"root","password":"a","pass` + jsonU('w') + `ord":"b"}`, "request"},
		{"a nested value with a brace in a string", `{"port":{"x":[1,{"y":"}\"]"}],"z":{"port":2}},"host":"198.51.100.7","user":"root","password":"a"}`, "port"},
		{"host not a string", `{"host":7,"user":"root","password":"x"}`, "host"},
		{"no host", `{"user":"root","password":"x"}`, "host"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			r, f := ParseRequest([]byte(c.line))
			if f == nil || f.Code != CodeBadInput || f.Detail != c.detail {
				t.Fatalf("%+v, want bad_input %s", f, c.detail)
			}
			if r.Password != nil || r.Key != nil || r.Passphrase != nil {
				t.Fatalf("a refused request keeps secrets: %q %q %q", r.Password, r.Key, r.Passphrase)
			}
		})
	}
}

func TestObjectNames(t *testing.T) {
	cases := []struct {
		in   string
		want []string // nil with ok false: not an object
		ok   bool
	}{
		{`{}`, nil, true},
		{` { } `, nil, true},
		{`{"a":1}`, []string{"a"}, true},
		{" {\n\t\"a\" : 1 ,\r\n \"b\" :\"x\" } ", []string{"a", "b"}, true},
		{`{"a":{"a":1,"b":[{"a":2}]},"a":2}`, []string{"a", "a"}, true},
		{`{"a\"b":1,"c\\":2}`, []string{`a"b`, `c\`}, true},
		{`{"a":"x\\","b":"}","c":"\"{"}`, []string{"a", "b", "c"}, true},
		{`{"a":[1,[2,[3]],{"b":{}}],"c":null,"d":true,"e":-1.5e3}`, []string{"a", "c", "d", "e"}, true},
		{`{"p` + jsonU('w') + `":1}`, []string{"pw"}, true},
		{`[]`, nil, false},
		{`null`, nil, false},
		{`"{}"`, nil, false},
	}
	for _, c := range cases {
		if !json.Valid([]byte(c.in)) {
			t.Fatalf("%s: not valid JSON", c.in)
		}
		got, ok := objectNames([]byte(c.in))
		if ok != c.ok || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %q %v, want %q %v", c.in, got, ok, c.want, c.ok)
		}
	}
}

// TestParseRequestFailureCarriesNoSecret: a refusal names the field, never its value.
func TestParseRequestFailureCarriesNoSecret(t *testing.T) {
	lines := []string{
		`{"host":"198.51.100.7","user":"root","password":"` + "Secr3t\\n" + `"}`,
		`{"host":"198.51.100.7","user":"root","password":"Secr3t","key":"Secr3t"}`,
		`{"host":"Secr3t host","user":"root","password":"x"}`,
		`{"host":"198.51.100.7","user":"Secr3t user","password":"x"}`,
		`{"host":"198.51.100.7","user":"root","password":"Secr3t","port":0}`,
	}
	for _, l := range lines {
		_, f := ParseRequest([]byte(l))
		if f == nil {
			t.Fatalf("%s: accepted", l)
		}
		if b, _ := json.Marshal(f); bytes.Contains(b, []byte("Secr3t")) {
			t.Fatalf("%s: %s", l, b)
		}
	}
}

func TestRequestWipe(t *testing.T) {
	line := requestLine(t, map[string]any{"host": "198.51.100.7", "user": "root", "key": "KEYTEXT", "passphrase": "PASSPHRASE"})
	r, f := ParseRequest(line)
	if f != nil {
		t.Fatal(f)
	}
	key, pass := r.Key, r.Passphrase
	r.Wipe()
	if r.Key != nil || r.Passphrase != nil || r.Password != nil {
		t.Fatal("Wipe left the fields")
	}
	if !allZero(key) || !allZero(pass) {
		t.Fatalf("Wipe left the bytes: %q %q", key, pass)
	}

	r, f = ParseRequest(requestLine(t, map[string]any{"host": "198.51.100.7", "user": "root", "password": focusPassword}))
	if f != nil {
		t.Fatal(f)
	}
	pw := r.Password
	r.Wipe()
	if r.Password != nil || !allZero(pw) {
		t.Fatalf("Wipe left the password: %q", pw)
	}
	r.Wipe() // twice is harmless
}

// TestParseRequestOwnsItsBytes: the secrets are copies, so the caller can wipe the line and the
// request separately.
func TestParseRequestOwnsItsBytes(t *testing.T) {
	line := requestLine(t, map[string]any{"host": "198.51.100.7", "user": "root", "password": "Pa55w0rd"})
	r, f := ParseRequest(line)
	if f != nil {
		t.Fatal(f)
	}
	clear(line)
	if string(r.Password) != "Pa55w0rd" {
		t.Fatalf("%q", r.Password)
	}
}

func allZero(b []byte) bool {
	for _, c := range b {
		if c != 0 {
			return false
		}
	}
	return len(b) > 0
}

// jsonU is the JSON escape of the UTF-16 unit r: a backslash, "u" and four hex digits.
func jsonU(r rune) string { return `\` + fmt.Sprintf("u%04x", r) }

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
