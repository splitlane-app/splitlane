package connector

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"
)

func TestMask(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"vless link", "vless://" + uuid1 + "@example.com:443?security=reality&pbk=" + pbk + "&sid=6ba8&sni=www.example.com#n",
			"vless://•••@example.com:443?security=reality&pbk=•••&sid=•••&sni=www.example.com#n"},
		{"uuid in a message", "user " + strings.ToUpper(uuid1) + " not found", "user ••• not found"},
		{"trojan password", "dial trojan://Pa55-w0rd@example.com:443#T failed", "dial trojan://•••@example.com:443#T failed"},
		{"trojan password with a slash", "trojan://pa/ss@example.com:443", "trojan://•••@example.com:443"},
		{"ss base64 userinfo", "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpFeDRtcGxlLVBhc3M@192.0.2.5:8388/?outline=1",
			"ss://•••@192.0.2.5:8388/?outline=1"},
		{"ss 2022 percent-encoded", "ss://2022-blake3-aes-256-gcm:4PoB4vkFjXqVfXgjVPYtaooUvyhVVFM7hpMV7X2WogQ%3D@example.com:443",
			"ss://•••@example.com:443"},
		{"ss legacy", "ss://YWVzLTEyOC1nY206bGVnYWN5UGFzc0AxOTIuMC4yLjc6ODM4OA==#legacy", "ss://•••#legacy"},
		{"vmess base64", "bad " + vmessLink(`{"id":"x"}`) + " here", "bad vmess://••• here"},
		{"hysteria2 password and obfs", "hy2://letmein@example.com:443?obfs=salamander&obfs-password=gawr-gura&sni=example.com",
			"hy2://•••@example.com:443?obfs=salamander&obfs-password=•••&sni=example.com"},
		{"tuic", "tuic://" + uuid1 + ":pa55@example.com:443?alpn=h3", "tuic://•••@example.com:443?alpn=h3"},
		{"ws path", "vless://x@example.com:443?type=ws&path=%2Fsecret-path&host=example.com", "vless://•••@example.com:443?type=ws&path=•••&host=example.com"},
		{"wireguard conf lines", "[Interface]\nPrivateKey = " + wgPriv + "\n[Peer]\npublickey=" + wgPub + "\nPresharedKey = " + wgPSK + "\nEndpoint = vpn.example.com:51820",
			"[Interface]\nPrivateKey = •••\n[Peer]\npublickey=•••\nPresharedKey = •••\nEndpoint = vpn.example.com:51820"},
		{"bare wireguard key", "peer " + wgPub + " handshake", "peer ••• handshake"},
		{"core json", `{"private_key": "` + wgPriv + `", "password":"p@ss", "uuid" : "u", "public_key":"` + pbk + `", "short_id":"6ba8", "pre_shared_key":"x", "server":"example.com"}`,
			`{"private_key": "•••", "password":"•••", "uuid" : "•••", "public_key":"•••", "short_id":"•••", "pre_shared_key":"•••", "server":"example.com"}`},
		{"subscription token", "fetch https://sub.example.com/api/v1/client/subscribe?token=abc123&flag=v2ray: timeout",
			"fetch https://sub.example.com/api/v1/client/subscribe?token=•••&flag=v2ray: timeout"},
		{"subscription key", "https://sub.example.com/link?key=K3y&target=v2ray", "https://sub.example.com/link?key=•••&target=v2ray"},
		{"subscription userinfo", "https://user:pa55@sub.example.com/s", "https://•••@sub.example.com/s"},
		{"subscription token in the path", "GET https://sub.example.com/sub/aB3dE5fG7hJ9kL1mN0pQ?flag=1", "GET https://sub.example.com/sub/•••?flag=1"},
		{"subscription uuid in the path", "https://sub.example.com/" + uuid1, "https://sub.example.com/•••"},
		{"subscription query values", "https://sub.example.com/s?client=abc&format=json&flag=1&target=clash&ID=42&Tok3nTok3n&empty=",
			"https://sub.example.com/s?client=•••&format=json&flag=1&target=clash&ID=•••&•••&empty="},
		{"subscription query without a path", "get https://sub.example.com?sub=abc#frag done", "get https://sub.example.com?sub=•••#frag done"},
		{"shadowrocket vless base64", "vless://" + base64.StdEncoding.EncodeToString([]byte("auto:"+uuid1+"@example.com:443")) + "?remarks=x&tls=1",
			"vless://•••?remarks=x&tls=1"},
		{"ssr base64", "ssr://" + base64.RawURLEncoding.EncodeToString([]byte("192.0.2.1:443:origin:aes-256-cfb:plain:cGFzcw/?remarks=eA")), "ssr://•••"},
		{"vmess base64 in quotes", `"vmess://eyJpZCI6IngifQ=="`, `"vmess://•••"`},
		{"core json path and service name", `{"type":"ws","path":"/secret?ed=2048","service_name":"grpc-svc","host":"example.com"}`,
			`{"type":"ws","path":"•••","service_name":"•••","host":"example.com"}`},
		{"bare 16-byte key", "key " + ss128 + " used", "key ••• used"},
		{"bare 16-byte multi-user keys", "password " + ss128 + ":" + ss128, "password •••:•••"},
		{"long base64 run masked whole", "blob " + strings.Repeat("Ab3/", 22) + "==" + " end", "blob ••• end"},
		{"short padded value stays", "count=3 name=x1= ok", "count=3 name=x1= ok"},

		{"plain log line", "core started on 127.0.0.1:9090 in 12 ms", "core started on 127.0.0.1:9090 in 12 ms"},
		{"probe address", "probe https://api.ipify.org?format=json via vpn", "probe https://api.ipify.org?format=json via vpn"},
		{"release address", "https://github.com/example/splitlane/releases/download/v0.3.1/SplitLane-Setup-0.3.1.exe",
			"https://github.com/example/splitlane/releases/download/v0.3.1/SplitLane-Setup-0.3.1.exe"},
		{"empty", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Mask(c.in)
			if got != c.want {
				t.Errorf("Mask\n in  %q\n got %q\nwant %q", c.in, got, c.want)
			}
			if again := Mask(got); again != got {
				t.Errorf("Mask is not idempotent: %q -> %q", got, again)
			}
		})
	}
}

// Every secret of every sample link disappears from its masked form.
func TestMaskHidesSampleSecrets(t *testing.T) {
	for _, c := range linkCases() {
		s, err := ParseLink(c.link)
		if err != nil {
			t.Fatal(err)
		}
		m := Mask(c.link)
		var secrets []string
		switch {
		case s.VLESS != nil:
			secrets = []string{s.VLESS.UUID}
		case s.VMess != nil:
			secrets = []string{s.VMess.UUID}
		case s.Trojan != nil:
			secrets = []string{s.Trojan.Password}
		case s.Shadowsocks != nil:
			secrets = []string{s.Shadowsocks.Password}
		case s.Hysteria2 != nil:
			secrets = []string{s.Hysteria2.Password, s.Hysteria2.ObfsPassword}
		case s.TUIC != nil:
			secrets = []string{s.TUIC.UUID, s.TUIC.Password}
		}
		if s.TLS != nil && s.TLS.Reality != nil {
			k := s.TLS.Reality.PublicKey
			secrets = append(secrets, k, strings.NewReplacer("-", "+", "_", "/").Replace(k)+"=", s.TLS.Reality.ShortID)
		}
		for _, sec := range secrets {
			for _, form := range rawForms(sec) {
				if len(form) >= 4 && strings.Contains(m, form) {
					t.Errorf("%s: masked link %q still holds %q", c.name, m, form)
				}
			}
		}
	}
	for _, conf := range []string{wgConf, strings.ReplaceAll(wgConf, "\n", "\r\n"), strings.ReplaceAll(wgConf, " = ", "=")} {
		m := Mask(conf)
		for _, sec := range []string{wgPriv, wgPub, wgPSK} {
			if strings.Contains(m, sec) {
				t.Errorf("masked .conf still holds %q", sec)
			}
		}
	}
}

// rawForms: a secret as it may stand in a link, decoded or percent-encoded either way, and a UUID in
// upper case.
func rawForms(sec string) []string {
	return []string{sec, url.PathEscape(sec), url.QueryEscape(sec), strings.ToUpper(sec),
		strings.ReplaceAll(sec, "=", "%3D"), strings.ReplaceAll(sec, "@", "%40"), strings.ReplaceAll(sec, ":", "%3A")}
}

// Inputs where one rule's mask could open another rule's match: Mask still settles.
func TestMaskIsIdempotent(t *testing.T) {
	for _, s := range []string{
		`{"url": "https://h/?k=\"`,
		`{"text": "vless://a@b?path=\"x`,
		"a://AAAAAAAAAAAAAAAAAAAAAA==BBBBBBBBBBBBBBBBBBBBBB==",
		"PrivateKey =\nhttps://u@h/?x=" + wgPub,
		"https://h/" + uuid1 + "?token=" + ss128 + "&flag=" + wgPriv,
		"ss://" + strings.Repeat("A", 30) + "#" + uuid1 + "@x",
	} {
		m := Mask(s)
		if Mask(m) != m {
			t.Errorf("%q -> %q -> %q", s, m, Mask(m))
		}
	}
}
