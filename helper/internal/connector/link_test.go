package connector

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// Made-up values: none of them belongs to a real server.
const (
	uuid1  = "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	uuid2  = "9a8b7c6d-5e4f-4321-8765-abcdef012345"
	pbk    = "ezvd-drM4QNV61tfmh5v-OP6yhTnAAQyxi5VK7zbfUo"  // 32 bytes, base64url without padding
	pbkStd = "ezvd+drM4QNV61tfmh5v+OP6yhTnAAQyxi5VK7zbfUo=" // the same key in the standard alphabet
	ss256  = "4PoB4vkFjXqVfXgjVPYtaooUvyhVVFM7hpMV7X2WogQ="
	ss128  = "QLsRSlv3rUWfM2zUNsBn5A=="
)

func vmessLink(js string) string {
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(js))
}

func vmessLinkURL(js string) string {
	return "vmess://" + base64.RawURLEncoding.EncodeToString([]byte(js))
}

func dump(v any) string {
	b, _ := json.MarshalIndent(v, "", "  ")
	return string(b)
}

type linkCase struct {
	name string
	link string
	want Server
}

func wsTLS(host, path string, ed int) *Transport {
	t := &Transport{Type: TransportWS, Path: path}
	if host != "" {
		t.Host = []string{host}
	}
	if ed > 0 {
		t.MaxEarlyData, t.EarlyDataHeaderName = ed, "Sec-WebSocket-Protocol"
	}
	return t
}

func linkCases() []linkCase {
	vmessWS := Server{Name: "Нидерланды", Protocol: ProtocolVMess, Host: "example.com", Port: 443,
		TLS:       &TLS{ServerName: "example.com", Fingerprint: "chrome"},
		Transport: wsTLS("example.com", "/vm", 2048),
		VMess:     &VMess{UUID: uuid1, Security: "auto"}}
	vmessURI := vmessWS
	vmessURI.Name = "URI"
	grpcVMess := Server{Name: "grpc", Protocol: ProtocolVMess, Host: "192.0.2.20", Port: 8443,
		TLS:       &TLS{ServerName: "example.com"},
		Transport: &Transport{Type: TransportGRPC, ServiceName: "svc"},
		VMess:     &VMess{UUID: uuid2, Security: "chacha20-poly1305"}}
	grpcJSON := `{"v":2,"ps":"grpc","add":"192.0.2.20","port":8443,"id":"` + uuid2 + `","aid":0,"scy":"chacha20-poly1305","net":"grpc","type":"gun","host":"","path":"svc","tls":"tls","sni":"example.com"}`

	return []linkCase{
		// VLESS
		{"vless reality vision (3x-ui)",
			"vless://" + uuid1 + "@192.0.2.10:443?type=tcp&security=reality&pbk=" + pbk +
				"&fp=chrome&sni=www.example.com&sid=6ba85179e30d4fc2&spx=%2F&flow=xtls-rprx-vision&encryption=none&headerType=none" +
				"#%D0%93%D0%B5%D1%80%D0%BC%D0%B0%D0%BD%D0%B8%D1%8F%20Reality",
			Server{Name: "Германия Reality", Protocol: ProtocolVLESS, Host: "192.0.2.10", Port: 443,
				TLS:   &TLS{ServerName: "www.example.com", Fingerprint: "chrome", Reality: &Reality{PublicKey: pbk, ShortID: "6ba85179e30d4fc2"}},
				VLESS: &VLESS{UUID: uuid1, Flow: "xtls-rprx-vision"}}},
		{"vless ws tls early data in path (v2rayN)",
			"vless://" + uuid1 + "@cdn.example.com:443?encryption=none&security=tls&sni=cdn.example.com&alpn=h2%2Chttp%2F1.1&fp=firefox&type=ws&host=cdn.example.com&path=%2Fws%3Fed%3D2048#ws-cdn",
			Server{Name: "ws-cdn", Protocol: ProtocolVLESS, Host: "cdn.example.com", Port: 443,
				TLS:       &TLS{ServerName: "cdn.example.com", ALPN: []string{"h2", "http/1.1"}, Fingerprint: "firefox"},
				Transport: wsTLS("cdn.example.com", "/ws", 2048),
				VLESS:     &VLESS{UUID: uuid1}}},
		{"vless ws early data as parameters (NekoBox)",
			"vless://" + uuid1 + "@cdn.example.com:443?security=tls&type=ws&host=cdn.example.com&path=%2Fws&ed=4096&eh=Sec-WebSocket-Protocol",
			Server{Name: "cdn.example.com:443", Protocol: ProtocolVLESS, Host: "cdn.example.com", Port: 443,
				TLS:       &TLS{ServerName: "cdn.example.com"},
				Transport: wsTLS("cdn.example.com", "/ws", 4096),
				VLESS:     &VLESS{UUID: uuid1}}},
		{"vless grpc reality multi mode, default fingerprint (Marzban)",
			"vless://" + uuid1 + "@example.com:8443?security=reality&type=grpc&serviceName=grpc-svc&mode=multi&pbk=" + pbkStd + "&sid=&fp=&sni=example.com#grpc",
			Server{Name: "grpc", Protocol: ProtocolVLESS, Host: "example.com", Port: 8443,
				TLS:       &TLS{ServerName: "example.com", Fingerprint: "chrome", Reality: &Reality{PublicKey: pbk}},
				Transport: &Transport{Type: TransportGRPC, ServiceName: "grpc-svc"},
				VLESS:     &VLESS{UUID: uuid1}}},
		{"vless httpupgrade ipv6 without tls",
			"vless://" + uuid1 + "@[2001:DB8::10]:8080?type=httpupgrade&host=example.com&path=%2Fup&security=none",
			Server{Name: "[2001:db8::10]:8080", Protocol: ProtocolVLESS, Host: "2001:db8::10", Port: 8080,
				Transport: &Transport{Type: TransportHTTPUpgrade, Host: []string{"example.com"}, Path: "/up"},
				VLESS:     &VLESS{UUID: uuid1}}},
		{"vless h2 host list insecure",
			"vless://" + uuid1 + "@192.0.2.11:443?type=http&host=a.example.com%2Cb.example.com&path=%2Fh2&security=tls&allowInsecure=1",
			Server{Name: "192.0.2.11:443", Protocol: ProtocolVLESS, Host: "192.0.2.11", Port: 443,
				TLS:       &TLS{Insecure: true},
				Transport: &Transport{Type: TransportHTTP, Host: []string{"a.example.com", "b.example.com"}, Path: "/h2"},
				VLESS:     &VLESS{UUID: uuid1}}},
		{"vless quic",
			"vless://" + uuid1 + "@example.com:443?type=quic&quicSecurity=none&key=&headerType=none&security=tls&sni=example.com#quic",
			Server{Name: "quic", Protocol: ProtocolVLESS, Host: "example.com", Port: 443,
				TLS:       &TLS{ServerName: "example.com"},
				Transport: &Transport{Type: TransportQUIC},
				VLESS:     &VLESS{UUID: uuid1}}},
		{"vless grpc authority as the server name of an address",
			"vless://" + uuid1 + "@192.0.2.12:443?type=grpc&serviceName=s&authority=Example.com&security=tls",
			Server{Name: "192.0.2.12:443", Protocol: ProtocolVLESS, Host: "192.0.2.12", Port: 443,
				TLS:       &TLS{ServerName: "example.com"},
				Transport: &Transport{Type: TransportGRPC, ServiceName: "s"},
				VLESS:     &VLESS{UUID: uuid1}}},
		{"vless loopback test server", "vless://" + uuid1 + "@127.0.0.1:10443?security=none#local test",
			Server{Name: "local test", Protocol: ProtocolVLESS, Host: "127.0.0.1", Port: 10443, VLESS: &VLESS{UUID: uuid1}}},
		{"vless upper-case uuid, xudp, trailing slash",
			"VLESS://" + strings.ToUpper(uuid1) + "@Example.COM:443/?security=tls&packetEncoding=xudp",
			Server{Name: "example.com:443", Protocol: ProtocolVLESS, Host: "example.com", Port: 443,
				TLS:   &TLS{},
				VLESS: &VLESS{UUID: uuid1, PacketEncoding: "xudp"}}},

		// VMess
		{"vmess v2rayN json ws tls",
			vmessLink(`{"v":"2","ps":"Нидерланды","add":"example.com","port":"443","id":"` + uuid1 + `","aid":"0","scy":"auto","net":"ws","type":"none","host":"example.com","path":"/vm?ed=2048","tls":"tls","sni":"","alpn":"","fp":"chrome"}`),
			vmessWS},
		{"vmess xray uri form", "vmess://" + uuid1 + "@example.com:443?encryption=auto&security=tls&sni=example.com&type=ws&host=example.com&path=%2Fvm%3Fed%3D2048&fp=chrome#URI",
			vmessURI},
		{"vmess json grpc numbers", vmessLink(grpcJSON), grpcVMess},
		{"vmess json base64url without padding", vmessLinkURL(grpcJSON), grpcVMess},
		{"vmess json tcp legacy alter id",
			vmessLink(`{"v":"2","ps":"","add":"198.51.100.5","port":"10086","id":"` + uuid2 + `","aid":"64","net":"tcp","type":"none","tls":""}`),
			Server{Name: "198.51.100.5:10086", Protocol: ProtocolVMess, Host: "198.51.100.5", Port: 10086,
				VMess: &VMess{UUID: uuid2, Security: "auto", AlterID: 64}}},
		{"vmess json h2",
			vmessLink(`{"v":"2","ps":"h2","add":"example.com","port":"443","id":"` + uuid2 + `","aid":"0","scy":"aes-128-gcm","net":"h2","type":"none","host":"example.com","path":"/h2","tls":"tls"}`),
			Server{Name: "h2", Protocol: ProtocolVMess, Host: "example.com", Port: 443,
				TLS:       &TLS{ServerName: "example.com"},
				Transport: &Transport{Type: TransportHTTP, Host: []string{"example.com"}, Path: "/h2"},
				VMess:     &VMess{UUID: uuid2, Security: "aes-128-gcm"}}},
		{"vmess json grpc mode none, authority equal to sni",
			vmessLink(`{"v":"2","ps":"g","add":"192.0.2.21","port":"443","id":"` + uuid2 + `","aid":"0","net":"grpc","type":"none","host":"example.com","path":"svc","tls":"tls","sni":"example.com"}`),
			Server{Name: "g", Protocol: ProtocolVMess, Host: "192.0.2.21", Port: 443,
				TLS:       &TLS{ServerName: "example.com"},
				Transport: &Transport{Type: TransportGRPC, ServiceName: "svc"},
				VMess:     &VMess{UUID: uuid2, Security: "auto"}}},

		{"vmess json grpc authority as the server name of an address (v2rayN)",
			vmessLink(`{"v":"2","ps":"g2","add":"192.0.2.22","port":"443","id":"` + uuid2 + `","aid":"0","net":"grpc","type":"gun","host":"example.com","path":"svc","tls":"tls","sni":""}`),
			Server{Name: "g2", Protocol: ProtocolVMess, Host: "192.0.2.22", Port: 443,
				TLS:       &TLS{ServerName: "example.com"},
				Transport: &Transport{Type: TransportGRPC, ServiceName: "svc"},
				VMess:     &VMess{UUID: uuid2, Security: "auto"}}},

		// Trojan
		{"trojan raw spaces in the name", "trojan://pw@example.com:443?sni=example.com#My  Server 1",
			Server{Name: "My  Server 1", Protocol: ProtocolTrojan, Host: "example.com", Port: 443,
				TLS: &TLS{ServerName: "example.com"}, Trojan: &Trojan{Password: "pw"}}},
		{"trojan default tls", "trojan://Pa55-w0rd@example.com:443#Trojan",
			Server{Name: "Trojan", Protocol: ProtocolTrojan, Host: "example.com", Port: 443,
				TLS: &TLS{}, Trojan: &Trojan{Password: "Pa55-w0rd"}}},
		{"trojan grpc", "trojan://secret%40pass@example.com:443?security=tls&sni=example.com&type=grpc&serviceName=trojan&mode=gun&alpn=h2#grpc",
			Server{Name: "grpc", Protocol: ProtocolTrojan, Host: "example.com", Port: 443,
				TLS:       &TLS{ServerName: "example.com", ALPN: []string{"h2"}},
				Transport: &Transport{Type: TransportGRPC, ServiceName: "trojan"},
				Trojan:    &Trojan{Password: "secret@pass"}}},
		{"trojan ws peer alias", "trojan://pw@192.0.2.30:443?type=ws&path=%2Ftr&host=cdn.example.com&peer=cdn.example.com&allowInsecure=0",
			Server{Name: "192.0.2.30:443", Protocol: ProtocolTrojan, Host: "192.0.2.30", Port: 443,
				TLS:       &TLS{ServerName: "cdn.example.com"},
				Transport: wsTLS("cdn.example.com", "/tr", 0),
				Trojan:    &Trojan{Password: "pw"}}},
		{"trojan reality", "trojan://pw@example.com:443?security=reality&pbk=" + pbk + "&sid=ab12&sni=www.example.com&fp=safari",
			Server{Name: "example.com:443", Protocol: ProtocolTrojan, Host: "example.com", Port: 443,
				TLS:    &TLS{ServerName: "www.example.com", Fingerprint: "safari", Reality: &Reality{PublicKey: pbk, ShortID: "ab12"}},
				Trojan: &Trojan{Password: "pw"}}},
		{"trojan ipv6 cyrillic name", "trojan://pw@[2001:db8::30]:443?sni=example.com#%D0%A2%D1%80%D0%BE%D1%8F%D0%BD",
			Server{Name: "Троян", Protocol: ProtocolTrojan, Host: "2001:db8::30", Port: 443,
				TLS: &TLS{ServerName: "example.com"}, Trojan: &Trojan{Password: "pw"}}},

		// Shadowsocks
		{"ss outline base64url userinfo", "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpFeDRtcGxlLVBhc3M@192.0.2.5:8388/?outline=1#Outline%20Server",
			Server{Name: "Outline Server", Protocol: ProtocolShadowsocks, Host: "192.0.2.5", Port: 8388,
				Shadowsocks: &Shadowsocks{Method: "chacha20-ietf-poly1305", Password: "Ex4mple-Pass"}}},
		{"ss sip002 base64 std with padding", "ss://YWVzLTI1Ni1nY206cGErc3Mvd28/cmQ=@example.com:8443#aes",
			Server{Name: "aes", Protocol: ProtocolShadowsocks, Host: "example.com", Port: 8443,
				Shadowsocks: &Shadowsocks{Method: "aes-256-gcm", Password: "pa+ss/wo?rd"}}},
		{"ss 2022 percent-encoded", "ss://2022-blake3-aes-256-gcm:" + strings.ReplaceAll(ss256, "=", "%3D") + "@example.com:443#ss2022",
			Server{Name: "ss2022", Protocol: ProtocolShadowsocks, Host: "example.com", Port: 443,
				Shadowsocks: &Shadowsocks{Method: "2022-blake3-aes-256-gcm", Password: ss256}}},
		{"ss 2022 multi-user ipv6", "ss://2022-blake3-aes-128-gcm:" + ss128 + "%3A" + ss128 + "@[2001:db8::5]:8443",
			Server{Name: "[2001:db8::5]:8443", Protocol: ProtocolShadowsocks, Host: "2001:db8::5", Port: 8443,
				Shadowsocks: &Shadowsocks{Method: "2022-blake3-aes-128-gcm", Password: ss128 + ":" + ss128}}},
		{"ss legacy fully base64", "ss://YWVzLTEyOC1nY206bGVnYWN5UGFzc0AxOTIuMC4yLjc6ODM4OA==#legacy",
			Server{Name: "legacy", Protocol: ProtocolShadowsocks, Host: "192.0.2.7", Port: 8388,
				Shadowsocks: &Shadowsocks{Method: "aes-128-gcm", Password: "legacyPass"}}},

		// Hysteria2
		{"hysteria2 salamander insecure", "hysteria2://letmein@example.com:443/?sni=example.com&obfs=salamander&obfs-password=gawr-gura&insecure=1#hy2",
			Server{Name: "hy2", Protocol: ProtocolHysteria2, Host: "example.com", Port: 443,
				TLS:       &TLS{ServerName: "example.com", Insecure: true},
				Hysteria2: &Hysteria2{Password: "letmein", Obfs: "salamander", ObfsPassword: "gawr-gura"}}},
		{"hy2 ipv6 mport", "hy2://p%40ss@[2001:db8::2]:8443?sni=example.com&mport=20000-30000,40000#hy2-v6",
			Server{Name: "hy2-v6", Protocol: ProtocolHysteria2, Host: "2001:db8::2", Port: 8443,
				TLS:       &TLS{ServerName: "example.com"},
				Hysteria2: &Hysteria2{Password: "p@ss", ServerPorts: []PortRange{{20000, 30000}, {40000, 40000}}}}},
		{"hysteria2 port list in the address", "hysteria2://pw@192.0.2.40:443,5000-6000/?sni=example.com&alpn=h3",
			Server{Name: "192.0.2.40:443", Protocol: ProtocolHysteria2, Host: "192.0.2.40", Port: 443,
				TLS:       &TLS{ServerName: "example.com", ALPN: []string{"h3"}},
				Hysteria2: &Hysteria2{Password: "pw", ServerPorts: []PortRange{{443, 443}, {5000, 6000}}}}},
		{"hy2 default port userpass", "hy2://user:pass@example.com",
			Server{Name: "example.com:443", Protocol: ProtocolHysteria2, Host: "example.com", Port: 443,
				TLS: &TLS{}, Hysteria2: &Hysteria2{Password: "user:pass"}}},
		{"hysteria2 bandwidth", "hysteria2://pw@example.com:443?upmbps=50&downmbps=200&fp=chrome",
			Server{Name: "example.com:443", Protocol: ProtocolHysteria2, Host: "example.com", Port: 443,
				TLS: &TLS{}, Hysteria2: &Hysteria2{Password: "pw", UpMbps: 50, DownMbps: 200}}},

		// TUIC
		{"tuic bbr native", "tuic://" + uuid1 + ":pa55@example.com:443?congestion_control=bbr&udp_relay_mode=native&alpn=h3&sni=example.com&allow_insecure=0#tuic",
			Server{Name: "tuic", Protocol: ProtocolTUIC, Host: "example.com", Port: 443,
				TLS:  &TLS{ServerName: "example.com", ALPN: []string{"h3"}},
				TUIC: &TUIC{UUID: uuid1, Password: "pa55", CongestionControl: "bbr", UDPRelayMode: "native"}}},
		{"tuic cubic quic zero rtt insecure", "tuic://" + uuid2 + ":p%3Ass@192.0.2.50:8443?congestion_control=cubic&udp_relay_mode=quic&allowInsecure=1&reduce_rtt=1&disable_sni=0",
			Server{Name: "192.0.2.50:8443", Protocol: ProtocolTUIC, Host: "192.0.2.50", Port: 8443,
				TLS:  &TLS{Insecure: true},
				TUIC: &TUIC{UUID: uuid2, Password: "p:ss", CongestionControl: "cubic", UDPRelayMode: "quic", ZeroRTT: true}}},
		{"tuic ipv6 cyrillic name", "tuic://" + uuid1 + ":pw@[2001:db8::50]:443?alpn=h3%2Ch3-29#%D0%A2%D0%A3%D0%98%D0%9A",
			Server{Name: "ТУИК", Protocol: ProtocolTUIC, Host: "2001:db8::50", Port: 443,
				TLS:  &TLS{ALPN: []string{"h3", "h3-29"}},
				TUIC: &TUIC{UUID: uuid1, Password: "pw"}}},
	}
}

func TestParseLink(t *testing.T) {
	for _, c := range linkCases() {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseLink("  " + c.link + "\r\n")
			if err != nil {
				t.Fatalf("ParseLink: %v", err)
			}
			if got.Text != c.link {
				t.Errorf("Text = %q, want the trimmed link", got.Text)
			}
			got.Text = ""
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %s\nwant %s", dump(got), dump(c.want))
			}
		})
	}
}

// The same server written by different clients is the same model (Review Focus 1).
func TestDialectsMeetInOneModel(t *testing.T) {
	groups := [][]string{
		{
			vmessLink(`{"v":"2","ps":"x","add":"example.com","port":"443","id":"` + uuid1 + `","aid":"0","scy":"auto","net":"ws","type":"none","host":"example.com","path":"/vm?ed=2048","tls":"tls","fp":"chrome"}`),
			"vmess://" + uuid1 + "@example.com:443?security=tls&sni=example.com&type=ws&host=example.com&path=%2Fvm%3Fed%3D2048&fp=chrome#x",
			"vmess://" + uuid1 + "@example.com:443?encryption=auto&security=tls&type=ws&host=example.com&path=%2Fvm&ed=2048&fp=chrome#x",
		},
		{
			"ss://YWVzLTI1Ni1nY206cGFzcw==@192.0.2.5:8388#x",
			"ss://YWVzLTI1Ni1nY206cGFzcw@192.0.2.5:8388/#x",
			"ss://aes-256-gcm:pass@192.0.2.5:8388#x",
			"ss://" + base64.StdEncoding.EncodeToString([]byte("aes-256-gcm:pass@192.0.2.5:8388")) + "#x",
		},
		{
			"vless://" + uuid1 + "@192.0.2.10:443?security=reality&pbk=" + pbk + "&sni=www.example.com&fp=chrome&type=tcp#x",
			"vless://" + uuid1 + "@192.0.2.10:443?security=reality&pbk=" + pbkStd + "&sni=www.example.com&spx=%2F#x",
			"vless://" + strings.ToUpper(uuid1) + "@192.0.2.10:443?Security=reality&PBK=" + pbk + "&SNI=www.example.com&encryption=none&headerType=none#x",
		},
		{
			vmessLink(`{"v":"2","ps":"x","add":"192.0.2.22","port":"443","id":"` + uuid2 + `","net":"grpc","type":"gun","host":"example.com","path":"svc","tls":"tls"}`),
			"vmess://" + uuid2 + "@192.0.2.22:443?type=grpc&serviceName=svc&mode=gun&security=tls&sni=example.com#x",
			"vmess://" + uuid2 + "@192.0.2.22:443?type=grpc&serviceName=svc&authority=example.com&security=tls#x",
		},
		{
			"hysteria2://pw@example.com:443?sni=example.com",
			"hy2://pw@example.com?sni=example.com&insecure=0",
			"hy2://pw@example.com:443/?peer=example.com",
		},
	}
	for _, g := range groups {
		first, err := ParseLink(g[0])
		if err != nil {
			t.Fatalf("%s: %v", g[0], err)
		}
		first.Text, first.Name = "", ""
		for _, l := range g[1:] {
			s, err := ParseLink(l)
			if err != nil {
				t.Fatalf("%s: %v", l, err)
			}
			s.Text, s.Name = "", ""
			if !reflect.DeepEqual(s, first) {
				t.Errorf("%s\ngot  %s\nwant %s", l, dump(s), dump(first))
			}
		}
	}
}

func TestParseLinkRejects(t *testing.T) {
	vl := "vless://" + uuid1 + "@example.com:443"
	cases := []struct{ name, link, code string }{
		{"ssr", "ssr://ZXhhbXBsZS5jb20", CodeUnsupportedProtocol},
		{"socks", "socks://dXNlcjpwYXNz@192.0.2.1:1080", CodeUnsupportedProtocol},
		{"http", "http://example.com/sub", CodeUnsupportedProtocol},
		{"https", "https://example.com/sub", CodeUnsupportedProtocol},
		{"hysteria v1", "hysteria://example.com:443?auth=x", CodeUnsupportedProtocol},
		{"wireguard link", "wireguard://key@example.com:51820", CodeUnsupportedProtocol},
		{"vless post-quantum encryption", vl + "?encryption=mlkem768x25519plus.native.0rtt.abc", CodeUnsupportedProtocol},
		{"vless flow direct", vl + "?security=tls&flow=xtls-rprx-direct", CodeUnsupportedProtocol},
		{"ss stream cipher", "ss://rc4-md5:pass@example.com:8388", CodeUnsupportedProtocol},
		{"ss none", "ss://none:pass@example.com:8388", CodeUnsupportedProtocol},
		{"vmess legacy cipher", vmessLink(`{"v":"2","add":"example.com","port":"443","id":"` + uuid1 + `","scy":"aes-128-cfb"}`), CodeUnsupportedProtocol},

		{"xhttp", vl + "?type=xhttp&mode=auto&path=%2Fx&security=tls", CodeUnsupportedTransport},
		{"splithttp", vl + "?type=splithttp", CodeUnsupportedTransport},
		{"kcp", vl + "?type=kcp&seed=abc", CodeUnsupportedTransport},
		{"tcp http header", vl + "?type=tcp&headerType=http&host=example.com", CodeUnsupportedTransport},
		{"vmess json kcp", vmessLink(`{"v":"2","add":"example.com","port":"443","id":"` + uuid1 + `","net":"kcp"}`), CodeUnsupportedTransport},
		{"vmess json tcp http header", vmessLink(`{"v":"2","add":"example.com","port":"443","id":"` + uuid1 + `","net":"tcp","type":"http"}`), CodeUnsupportedTransport},
		{"quic encryption", vl + "?type=quic&quicSecurity=aes-128-gcm&key=k&security=tls", CodeUnsupportedTransport},
		{"grpc custom path", vl + "?type=grpc&serviceName=%2Fmy%2Fpath&security=tls", CodeUnsupportedTransport},
		{"grpc other authority", vl + "?type=grpc&serviceName=s&authority=other.example.com&security=tls&sni=example.com", CodeUnsupportedTransport},
		{"xtls security", vl + "?security=xtls", CodeUnsupportedTransport},
		{"unknown transport", vl + "?type=meek", CodeUnsupportedTransport},

		{"no scheme", "example.com:443", CodeBadLink},
		{"empty", "   ", CodeBadLink},
		{"space inside", "vless://" + uuid1 + "@example.com:443?path=/a b", CodeBadLink},
		{"space before the name", "vless://" + uuid1 + "@example.com:443 #name", CodeBadLink},
		{"control character in the name", "vless://" + uuid1 + "@example.com:443#a\x01b", CodeBadLink},
		{"tab in the name", "vless://" + uuid1 + "@example.com:443#a\tb", CodeBadLink},
		{"raw bytes that are not utf-8", "vless://" + uuid1 + "@example.com:443#\xff\xfe", CodeBadLink},
		{"decoded value that is not utf-8", vl + "?type=ws&path=%2F%FF", CodeBadLink},
		{"decoded password that is not utf-8", "trojan://%C3%28@example.com:443", CodeBadLink},
		{"ws path with a line break", vl + "?type=ws&path=%2Fa%0Ab", CodeBadLink},
		{"httpupgrade path with a nul", vl + "?type=httpupgrade&path=%2Fa%00", CodeBadLink},
		{"h2 path with delete", vl + "?type=http&path=%2Fa%7F&security=tls", CodeBadLink},
		{"vmess json path with a control character", vmessLink(`{"v":"2","add":"example.com","port":"443","id":"` + uuid1 + `","net":"ws","path":"/a` + string(rune(92)) + `u0001b"}`), CodeBadLink},
		{"unspecified ipv4", "vless://" + uuid1 + "@0.0.0.0:443", CodeBadLink},
		{"unspecified ipv6", "vless://" + uuid1 + "@[::]:443", CodeBadLink},
		{"unspecified mapped", "trojan://pw@[::ffff:0.0.0.0]:443", CodeBadLink},
		{"multicast ipv4", "trojan://pw@224.0.0.1:443", CodeBadLink},
		{"multicast ipv6", "hy2://pw@[ff02::1]:443", CodeBadLink},
		{"vmess json unspecified", vmessLink(`{"v":"2","add":"0.0.0.0","port":"1","id":"` + uuid1 + `","ps":"Expires 2026-12-01"}`), CodeBadLink},
		{"grpc bad authority", vl + "?type=grpc&serviceName=s&authority=a%20b&security=tls", CodeBadLink},
		{"port zero", "vless://" + uuid1 + "@example.com:0", CodeBadLink},
		{"port too big", "vless://" + uuid1 + "@example.com:70000", CodeBadLink},
		{"port not a number", "trojan://pw@example.com:https", CodeBadLink},
		{"bad uuid", "vless://not-a-uuid@example.com:443", CodeBadLink},
		{"unknown parameter", vl + "?security=tls&fragment=1-3,5-10,tlshello", CodeBadLink},
		{"pqv unknown", vl + "?security=reality&pbk=" + pbk + "&sni=example.com&pqv=abc", CodeBadLink},
		{"duplicate parameter", vl + "?security=tls&sni=a.example.com&sni=b.example.com", CodeBadLink},
		{"bad fingerprint", vl + "?security=tls&fp=netscape", CodeBadLink},
		{"bad allowInsecure", vl + "?security=tls&allowInsecure=maybe", CodeBadLink},
		{"bad percent encoding", vl + "?path=%zz", CodeBadLink},
		{"vmess bad base64", "vmess://!!!notbase64", CodeBadLink},
		{"vmess not json", "vmess://" + base64.StdEncoding.EncodeToString([]byte("aes-128-gcm:"+uuid1+"@example.com:443")), CodeBadLink},
		{"vmess unknown field", vmessLink(`{"v":"2","add":"example.com","port":"443","id":"` + uuid1 + `","mux":"on"}`), CodeBadLink},
		{"vmess version 1", vmessLink(`{"v":"1","add":"example.com","port":"443","id":"` + uuid1 + `"}`), CodeBadLink},
		{"vmess bad aid", vmessLink(`{"v":"2","add":"example.com","port":"443","id":"` + uuid1 + `","aid":"-1"}`), CodeBadLink},
		{"vmess field of wrong type", vmessLink(`{"v":"2","add":["example.com"],"port":"443","id":"` + uuid1 + `"}`), CodeBadLink},
		{"ss 2022 short key", "ss://2022-blake3-aes-256-gcm:" + strings.ReplaceAll(ss128, "=", "%3D") + "@example.com:443", CodeBadLink},
		{"ss bad base64 userinfo", "ss://@@@@@example.com:8388", CodeBadLink},
		{"reality odd short id", vl + "?security=reality&pbk=" + pbk + "&sni=example.com&sid=abc", CodeBadLink},
		{"reality bad public key", vl + "?security=reality&pbk=abcd&sni=example.com", CodeBadLink},
		{"vision over ws", vl + "?security=tls&type=ws&flow=xtls-rprx-vision", CodeBadLink},
		{"vision without tls", vl + "?flow=xtls-rprx-vision", CodeBadLink},
		{"quic without tls", vl + "?type=quic", CodeBadLink},
		{"bad packet encoding", vl + "?packetEncoding=raw", CodeBadLink},
		{"grpc bad mode", vl + "?type=grpc&serviceName=s&mode=guna&security=tls", CodeBadLink},
		{"ws host list", vl + "?type=ws&host=a.example.com%2Cb.example.com", CodeBadLink},
		{"bad early data", vl + "?type=ws&path=%2Fws%3Fed%3Dlots", CodeBadLink},
		{"ipv6 without brackets", "vless://" + uuid1 + "@2001:db8::1:443", CodeBadLink},
		{"ipv6 zone", "vless://" + uuid1 + "@[fe80::1%25eth0]:443", CodeBadLink},
		{"non-ascii host", "vless://" + uuid1 + "@пример.рф:443", CodeBadLink},
		{"numeric host that is not an address", "vless://" + uuid1 + "@1.2.3:443", CodeBadLink},
		{"path in a vless address", vl + "/some/path?security=tls", CodeBadLink},
		{"hysteria2 certificate pin", "hysteria2://pw@example.com:443?pinSHA256=AA:BB", CodeBadLink},
		{"hysteria2 other obfs", "hysteria2://pw@example.com:443?obfs=xor&obfs-password=x", CodeBadLink},
		{"hysteria2 obfs password alone", "hysteria2://pw@example.com:443?obfs-password=x", CodeBadLink},
		{"hysteria2 bad port range", "hysteria2://pw@example.com:443?mport=30000-20000", CodeBadLink},
		{"hysteria2 two port lists", "hysteria2://pw@example.com:443,5000-6000?mport=7000-8000", CodeBadLink},
		{"hysteria2 bad bandwidth", "hysteria2://pw@example.com:443?upmbps=fast", CodeBadLink},
		{"tuic bad congestion control", "tuic://" + uuid1 + ":pw@example.com:443?congestion_control=reno", CodeBadLink},
		{"tuic bad relay mode", "tuic://" + uuid1 + ":pw@example.com:443?udp_relay_mode=tcp", CodeBadLink},
		{"tuic sni off", "tuic://" + uuid1 + ":pw@example.com:443?disable_sni=1", CodeBadLink},

		{"vless no uuid", "vless://@example.com:443", CodeMissingField},
		{"vless no user part", "vless://example.com:443", CodeMissingField},
		{"vless no port", "vless://" + uuid1 + "@example.com", CodeMissingField},
		{"vless no host", "vless://" + uuid1 + "@:443", CodeMissingField},
		{"trojan no password", "trojan://@example.com:443", CodeMissingField},
		{"reality without public key", vl + "?security=reality&sni=example.com", CodeMissingField},
		{"reality on an address without sni", "vless://" + uuid1 + "@192.0.2.10:443?security=reality&pbk=" + pbk, CodeMissingField},
		{"salamander without password", "hysteria2://pw@example.com:443?obfs=salamander", CodeMissingField},
		{"hysteria2 no password", "hysteria2://example.com:443", CodeMissingField},
		{"tuic no password", "tuic://" + uuid1 + "@example.com:443", CodeMissingField},
		{"ss no password", "ss://aes-256-gcm:@example.com:8388", CodeMissingField},
		{"vmess json no id", vmessLink(`{"v":"2","add":"example.com","port":"443"}`), CodeMissingField},
		{"vmess json no address", vmessLink(`{"v":"2","port":"443","id":"` + uuid1 + `"}`), CodeMissingField},

		{"ss plugin", "ss://YWVzLTI1Ni1nY206cGFzcw@example.com:8388/?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dexample.com#p", CodePlugin},
		{"ss plugin v2ray", "ss://aes-256-gcm:pass@example.com:443?plugin=v2ray-plugin", CodePlugin},

		{"too long", vl + "?path=%2F" + strings.Repeat("a", MaxLinkLen), CodeTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := ParseLink(c.link)
			if err == nil {
				t.Fatalf("accepted: %s", dump(s))
			}
			var r *Rejection
			if !errors.As(err, &r) {
				t.Fatalf("error %v is not a *Rejection", err)
			}
			if r.Code != c.code {
				t.Errorf("code = %s (%s), want %s", r.Code, r.Detail, c.code)
			}
			if s.Protocol != "" || s.Text != "" {
				t.Errorf("a rejected link returns a server: %s", dump(s))
			}
		})
	}
}

// A rejection never repeats a secret of the link.
func TestRejectionDetailCarriesNoSecret(t *testing.T) {
	links := []string{
		"vless://" + uuid1 + "@example.com:443?security=reality&pbk=" + pbk + "&sni=example.com&sid=abc",
		"trojan://TopSecret@example.com:0",
		"trojan://TopSecret@example.com:443?security=tls&fp=TopSecret",
		"ss://2022-blake3-aes-256-gcm:TopSecret@example.com:443",
		"hysteria2://TopSecret@example.com:443?obfs=xor&obfs-password=TopSecret",
		"tuic://" + uuid1 + ":TopSecret@example.com:443?congestion_control=TopSecret",
	}
	for _, l := range links {
		_, err := ParseLink(l)
		if err == nil {
			t.Fatalf("accepted %s", l)
		}
		msg := err.Error()
		for _, secret := range []string{uuid1, pbk, "TopSecret", "abc"} {
			if strings.Contains(msg, secret) {
				t.Errorf("%s: error %q repeats %q", l, msg, secret)
			}
		}
	}
}

func TestRejectionError(t *testing.T) {
	r := &Rejection{Line: 3, Code: CodeBadLink, Detail: "port"}
	if got := r.Error(); got != "bad_link: port" {
		t.Errorf("Error() = %q", got)
	}
	if got := (&Rejection{Code: CodePlugin}).Error(); got != "plugin" {
		t.Errorf("Error() = %q", got)
	}
}
