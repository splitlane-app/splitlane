package connector

import (
	"encoding/base64"
	"errors"
	"testing"
)

// `go test` runs only the seed corpus below; `go test -fuzz=FuzzParseLink` explores further.

var codes = map[string]bool{
	CodeUnsupportedProtocol: true, CodeUnsupportedTransport: true, CodeBadLink: true, CodeMissingField: true,
	CodeBadWireGuard: true, CodeAmneziaWG: true, CodePlugin: true, CodeTooLong: true,
}

func checkRejection(t *testing.T, err error) {
	t.Helper()
	var r *Rejection
	if !errors.As(err, &r) || !codes[r.Code] {
		t.Fatalf("error %v is not a known rejection", err)
	}
}

func FuzzParseLink(f *testing.F) {
	for _, c := range linkCases() {
		f.Add(c.link)
	}
	for _, s := range []string{"", "vless://", "ss://", "vmess://e30", "hy2://@:", "tuic://:@[::1]:1", "ss://YQ@x:1", "trojan://a@[::]:65535#%"} {
		f.Add(s)
	}
	f.Add(`{"url": "https://h/?k=\"`)
	f.Add("a://AAAAAAAAAAAAAAAAAAAAAA==BBBBBBBBBBBBBBBBBBBBBB==")
	f.Add("https://u:p@h/aB3dE5fG7hJ9kL1mN0pQ?token=x&format=json#f vmess://eyJpZCI6IngifQ== PrivateKey = k")
	f.Fuzz(func(t *testing.T, link string) {
		if m := Mask(link); Mask(m) != m {
			t.Fatalf("Mask is not idempotent: %q -> %q -> %q", link, m, Mask(m))
		}
		s, err := ParseLink(link)
		if err != nil {
			checkRejection(t, err)
			return
		}
		checkServer(t, s)
		again, err := ParseLink(s.Text)
		if err != nil || again.ID != s.ID {
			t.Fatalf("the text of a parsed link does not parse back: %v", err)
		}
	})
}

func FuzzParseWireGuardConf(f *testing.F) {
	f.Add([]byte(wgConf), "office")
	f.Add([]byte("[Interface]\nPrivateKey="+wgPriv+"\nAddress=10.0.0.2\n[Peer]\nPublicKey="+wgPub+"\nEndpoint=[::1]:1\nAllowedIPs=::/0\n"), "")
	f.Add([]byte("[Interface]\nJc=1\n"), "x.conf")
	f.Add([]byte("[Peer]\n[Peer]\n"), "")
	f.Add([]byte("\xff=\n["), "\x00")
	f.Fuzz(func(t *testing.T, conf []byte, name string) {
		_ = Mask(string(conf))
		s, err := ParseWireGuardConf(conf, name)
		if err != nil {
			checkRejection(t, err)
			return
		}
		checkServer(t, s)
	})
}

func FuzzParseSubscription(f *testing.F) {
	list := ""
	for _, c := range linkCases() {
		list += c.link + "\n"
	}
	f.Add([]byte(list))
	f.Add([]byte(base64.StdEncoding.EncodeToString([]byte(list))))
	f.Add([]byte(base64.RawURLEncoding.EncodeToString([]byte(list))))
	f.Add([]byte("#profile-title: x\r\n\r\n" + linkTrojan))
	f.Add([]byte(wgConf))
	f.Add([]byte("proxies:\n - {}"))
	f.Add([]byte("=\n"))
	f.Fuzz(func(t *testing.T, body []byte) {
		servers, rejected := ParseSubscription(body)
		seen := map[string]bool{}
		for _, s := range servers {
			checkServer(t, s)
			if seen[s.ID] {
				t.Fatalf("duplicate id %s", s.ID)
			}
			seen[s.ID] = true
		}
		for _, r := range rejected {
			if !codes[r.Code] || r.Line < 0 {
				t.Fatalf("bad rejection %+v", r)
			}
		}
		// ParseText is the same machinery with a .conf and a subscription address on top
		_, _, _ = ParseText(string(body), "x.conf")
	})
}
