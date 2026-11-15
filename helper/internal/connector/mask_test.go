package connector

import (
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
			secrets = append(secrets, s.TLS.Reality.PublicKey, s.TLS.Reality.ShortID)
		}
		for _, sec := range secrets {
			if len(sec) >= 4 && strings.Contains(m, sec) {
				t.Errorf("%s: masked link %q still holds %q", c.name, m, sec)
			}
		}
	}
	m := Mask(wgConf)
	for _, sec := range []string{wgPriv, wgPub, wgPSK} {
		if strings.Contains(m, sec) {
			t.Errorf("masked .conf still holds %q", sec)
		}
	}
}
