package coreconfig

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"flag"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"splitlane/helper/internal/connector"
)

// update rewrites the goldens of the proxy servers from Build:
//
//	go test ./internal/coreconfig -run TestBuildProxyMatchesGoldens -update
//
// It never touches vpn.golden.json and setup.golden.json: the Tailscale configs stay byte for byte.
var update = flag.Bool("update", false, "rewrite testdata/proxy-*.golden.json (never the Tailscale goldens)")

// minProxyGoldens: at least one golden per protocol and per transport and TLS variant.
const minProxyGoldens = 12

// Made-up values: none of them belongs to a real server.
const (
	uuidA  = "3f1b2c4d-5e6f-4a7b-8c9d-0e1f2a3b4c5d"
	uuidB  = "9a8b7c6d-5e4f-4321-8765-abcdef012345"
	pbk    = "ezvd-drM4QNV61tfmh5v-OP6yhTnAAQyxi5VK7zbfUo" // 32 bytes, base64url without padding
	ss256  = "4PoB4vkFjXqVfXgjVPYtaooUvyhVVFM7hpMV7X2WogQ="
	wgPriv = "lhw1Rezx8+44v1b7GrtW7HxgVikNwpLsRNzZk0ZbBXA="
	wgPub  = "ZcQzkt7Yzwta15biT7QDOx+0JlXlF1K5Mnq0qUyDgLY="
	wgPSK  = "G7sxQw6Upoi06QWKlQg49YeU7FjgUsa0xJx95nCpBDk="
)

const wgConf = `[Interface]
PrivateKey = ` + wgPriv + `
Address = 10.8.0.2/32, fd00:8::2/128
DNS = 1.1.1.1
MTU = 1420

[Peer]
PublicKey = ` + wgPub + `
PresharedKey = ` + wgPSK + `
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

func vmessLink(js string) string {
	return "vmess://" + base64.StdEncoding.EncodeToString([]byte(js))
}

// proxyCase is one server of the goldens, given as the user gives it: a share link or a .conf.
type proxyCase struct {
	name string // the golden is testdata/proxy-<name>.golden.json
	text string // a share link, or the text of a WireGuard .conf when conf is set
	conf bool
}

func proxyCases() []proxyCase {
	return []proxyCase{
		// VLESS: REALITY with Vision, and every transport.
		{name: "vless-reality-vision", text: "vless://" + uuidA + "@192.0.2.10:443?type=tcp&security=reality&pbk=" + pbk +
			"&fp=chrome&sni=www.example.com&sid=6ba85179e30d4fc2&spx=%2F&flow=xtls-rprx-vision&encryption=none#reality"},
		{name: "vless-ws-tls", text: "vless://" + uuidA + "@cdn.example.com:443?encryption=none&security=tls&sni=cdn.example.com" +
			"&alpn=h2%2Chttp%2F1.1&fp=firefox&type=ws&host=cdn.example.com&path=%2Fws%3Fed%3D2048#ws"},
		{name: "vless-grpc-reality", text: "vless://" + uuidA + "@example.com:8443?security=reality&type=grpc&serviceName=grpc-svc" +
			"&mode=multi&pbk=" + pbk + "&sni=example.com#grpc"},
		{name: "vless-httpupgrade-tls", text: "vless://" + uuidA + "@example.com:443?type=httpupgrade&host=example.com&path=%2Fup" +
			"&security=tls&alpn=h2%2Chttp%2F1.1#httpupgrade"},
		{name: "vless-http-tls", text: "vless://" + uuidA + "@192.0.2.11:443?type=http&host=a.example.com%2Cb.example.com&path=%2Fh2" +
			"&security=tls&allowInsecure=1"},
		{name: "vless-quic-tls", text: "vless://" + uuidA + "@example.com:443?type=quic&quicSecurity=none&security=tls&sni=example.com#quic"},
		{name: "vless-tcp-no-packet-encoding", text: "vless://" + uuidA + "@198.51.100.10:10443?security=none&packetEncoding=none#plain"},
		// VMess: the v2rayN JSON over ws and TLS, and the legacy protocol (alter id) over plain TCP.
		{name: "vmess-ws-tls", text: vmessLink(`{"v":"2","ps":"vmess","add":"example.com","port":"443","id":"` + uuidB +
			`","aid":"0","scy":"auto","net":"ws","type":"none","host":"example.com","path":"/vm?ed=2048","tls":"tls","sni":"","alpn":"","fp":"chrome"}`)},
		{name: "vmess-tcp-legacy", text: vmessLink(`{"v":"2","ps":"","add":"198.51.100.5","port":"10086","id":"` + uuidB +
			`","aid":"64","scy":"aes-128-gcm","net":"tcp","type":"none","tls":""}`)},
		// Trojan: TLS over TCP, gRPC, ws to an IPv6 address.
		{name: "trojan-tls", text: "trojan://Pa55-w0rd@example.com:443?sni=example.com&fp=chrome&alpn=h2%2Chttp%2F1.1#trojan"},
		{name: "trojan-grpc-tls", text: "trojan://secret%40pass@example.com:443?security=tls&sni=example.com&type=grpc&serviceName=trojan" +
			"&mode=gun&alpn=h2#grpc"},
		{name: "trojan-ws-ipv6", text: "trojan://pw@[2001:db8::30]:443?type=ws&path=%2Ftr&host=cdn.example.com&sni=cdn.example.com"},
		// Shadowsocks: AEAD (Outline) and 2022.
		{name: "shadowsocks-aead", text: "ss://Y2hhY2hhMjAtaWV0Zi1wb2x5MTMwNTpFeDRtcGxlLVBhc3M@192.0.2.5:8388/?outline=1#Outline"},
		{name: "shadowsocks-2022", text: "ss://2022-blake3-aes-256-gcm:" + strings.ReplaceAll(ss256, "=", "%3D") + "@example.com:443#ss2022"},
		// Hysteria2: one port; hopping ranges beside the port with salamander and Brutal; a port list.
		{name: "hysteria2-tls", text: "hysteria2://letmein@example.com:443/?sni=example.com#hy2"},
		{name: "hysteria2-hopping-obfs", text: "hy2://p%40ss@example.com:8443?sni=example.com&mport=20000-30000,40000&obfs=salamander" +
			"&obfs-password=gawr-gura&upmbps=50&downmbps=200#hy2-hop"},
		{name: "hysteria2-port-list", text: "hysteria2://pw@192.0.2.40:443,5000-6000/?sni=example.com&alpn=h3&insecure=1"},
		// TUIC v5.
		{name: "tuic", text: "tuic://" + uuidA + ":pa55@example.com:443?congestion_control=bbr&udp_relay_mode=native&alpn=h3" +
			"&sni=example.com&reduce_rtt=1#tuic"},
		// WireGuard: an endpoint, not an outbound.
		{name: "wireguard", text: wgConf, conf: true},
	}
}

// server parses the case as the engine will: from its text.
func (c proxyCase) server(t *testing.T) *connector.Server {
	t.Helper()
	var s connector.Server
	var err error
	if c.conf {
		s, err = connector.ParseWireGuardConf([]byte(c.text), "wg.conf")
	} else {
		s, err = connector.ParseLink(c.text)
	}
	if err != nil {
		t.Fatalf("%s: %v", c.name, err)
	}
	return &s
}

func mustParse(t *testing.T, link string) *connector.Server {
	t.Helper()
	return proxyCase{name: link, text: link}.server(t)
}

func proxyOptions(dataDir string, s *connector.Server) Options {
	o := testOptions(dataDir)
	o.Lane = Lane{Proxy: s}
	return o
}

func mustBuild(t *testing.T, o Options) []byte {
	t.Helper()
	b, err := Build(o)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func decode(t *testing.T, config []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(config, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "proxy-"+name+".golden.json")
}

func TestBuildProxyMatchesGoldens(t *testing.T) {
	cases := proxyCases()
	if len(cases) < minProxyGoldens {
		t.Fatalf("%d cases, want at least %d", len(cases), minProxyGoldens)
	}
	named := map[string]bool{}
	for _, c := range cases {
		path := goldenPath(c.name)
		if named[path] {
			t.Fatalf("two cases write %s", path)
		}
		named[path] = true
		t.Run(c.name, func(t *testing.T) {
			got := mustBuild(t, proxyOptions(`C:\ProgramData\SplitLane`, c.server(t)))
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (go test -run TestBuildProxyMatchesGoldens -update writes it)", err)
			}
			want = bytes.ReplaceAll(want, []byte("\r\n"), []byte("\n"))
			if !bytes.Equal(got, want) {
				t.Fatalf("config differs from %s:\n%s", path, got)
			}
		})
	}
	// A golden without its case would still be checked by the core and mislead.
	files, err := filepath.Glob(filepath.Join("testdata", "proxy-*.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if !named[f] {
			t.Errorf("%s has no case in proxyCases", f)
		}
	}
}

// The goldens hold every protocol, every transport and both kinds of TLS.
func TestProxyCasesCoverEveryKind(t *testing.T) {
	seen := map[string]bool{}
	for _, c := range proxyCases() {
		s := c.server(t)
		seen[s.Protocol] = true
		if s.Transport != nil {
			seen["transport "+s.Transport.Type] = true
		}
		switch {
		case s.TLS == nil:
			seen["no tls"] = true
		case s.TLS.Reality != nil:
			seen["reality"] = true
		default:
			seen["tls"] = true
		}
	}
	for _, want := range []string{
		connector.ProtocolVLESS, connector.ProtocolVMess, connector.ProtocolTrojan, connector.ProtocolShadowsocks,
		connector.ProtocolHysteria2, connector.ProtocolTUIC, connector.ProtocolWireGuard,
		"transport " + connector.TransportWS, "transport " + connector.TransportGRPC, "transport " + connector.TransportHTTPUpgrade,
		"transport " + connector.TransportHTTP, "transport " + connector.TransportQUIC,
		"no tls", "tls", "reality",
	} {
		if !seen[want] {
			t.Errorf("no golden for %s", want)
		}
	}
}

// laneOf returns the one outbound or endpoint tagged vpn of a config and tells where it is.
func laneOf(t *testing.T, config map[string]any) (lane map[string]any, endpoint bool) {
	t.Helper()
	found := 0
	for _, key := range []string{"outbounds", "endpoints"} {
		list, _ := config[key].([]any)
		for _, x := range list {
			if m := x.(map[string]any); m["tag"] == TagVPN {
				lane, endpoint = m, key == "endpoints"
				found++
			}
		}
	}
	if found != 1 {
		t.Fatalf("%d outbounds and endpoints are tagged %s, want 1", found, TagVPN)
	}
	return lane, endpoint
}

// Spec of stage 3 part 1, section 5: the lane of a server is its outbound (WireGuard: its endpoint)
// tagged vpn; there is no Tailscale endpoint and no API service; everything else (log, DNS,
// inbounds, route, Clash API) is the Tailscale config's, but for rule 0.
func TestProxyConfigsShareTheRest(t *testing.T) {
	base := decode(t, mustBuild(t, testOptions(`C:\ProgramData\SplitLane`)))
	baseRoute := base["route"].(map[string]any)
	baseRules := baseRoute["rules"].([]any)
	for _, c := range proxyCases() {
		s := c.server(t)
		cfg := decode(t, mustBuild(t, proxyOptions(`C:\ProgramData\SplitLane`, s)))
		for _, k := range []string{"log", "dns", "inbounds", "experimental"} {
			if !reflect.DeepEqual(cfg[k], base[k]) {
				t.Errorf("%s: %s differs from the Tailscale config", c.name, k)
			}
		}
		if _, ok := cfg["services"]; ok {
			t.Errorf("%s: the API service belongs to the Tailscale node only", c.name)
		}
		route := cfg["route"].(map[string]any)
		wantRules := baseRules[1:] // rule 0 is the node's
		if s.Protocol == connector.ProtocolWireGuard && wireGuardNodeRule {
			wantRules = baseRules
		}
		if !reflect.DeepEqual(route["rules"], wantRules) {
			t.Errorf("%s: route rules differ from the Tailscale config:\n%v", c.name, route["rules"])
		}
		for k, v := range baseRoute {
			if k != "rules" && !reflect.DeepEqual(route[k], v) {
				t.Errorf("%s: route.%s differs from the Tailscale config", c.name, k)
			}
		}
		lane, endpoint := laneOf(t, cfg)
		outbounds := cfg["outbounds"].([]any)
		if !reflect.DeepEqual(outbounds[0], base["outbounds"].([]any)[0]) {
			t.Errorf("%s: the first outbound must be the Tailscale config's direct", c.name)
		}
		if s.Protocol == connector.ProtocolWireGuard {
			if !endpoint || lane["type"] != "wireguard" || len(outbounds) != 1 {
				t.Errorf("%s: WireGuard must be the one endpoint: %v", c.name, cfg["endpoints"])
			}
		} else {
			if endpoint || lane["type"] != s.Protocol || len(outbounds) != 2 {
				t.Errorf("%s: the server must be the second outbound: %v", c.name, outbounds)
			}
			if _, ok := cfg["endpoints"]; ok {
				t.Errorf("%s: no endpoint without Tailscale or WireGuard: %v", c.name, cfg["endpoints"])
			}
		}
		for _, k := range []string{"detour", "domain_resolver", "bind_interface"} {
			if _, ok := lane[k]; ok {
				t.Errorf("%s: the lane sets %s: the server's address resolves through route.default_domain_resolver", c.name, k)
			}
		}
	}
}

// Rule 0 of spec 6.2 rejects the flows that the embedded Tailscale node hands the router from tailnet
// peers. A proxy outbound opens no flow towards the PC. Whether a WireGuard endpoint does is for the
// live check (task 9): until then wireGuardNodeRule is off and the rule is not written for it.
func TestNodeRuleOnlyForTheTailscaleNode(t *testing.T) {
	if firstRule(t, mustBuild(t, testOptions(`C:\ProgramData\SplitLane`))) != wantNodeInboundRule {
		t.Fatal("the Tailscale config starts with rule 0")
	}
	for _, c := range proxyCases() {
		s := c.server(t)
		for _, mode := range Modes {
			o := proxyOptions(`C:\ProgramData\SplitLane`, s)
			o.Mode = mode
			want := s.Protocol == connector.ProtocolWireGuard && wireGuardNodeRule
			if got := firstRule(t, mustBuild(t, o)) == wantNodeInboundRule; got != want {
				t.Errorf("%s mode %s: rule 0 written %v, want %v", c.name, mode, got, want)
			}
		}
	}
}

// lookup follows a dotted path through nested objects.
func lookup(m map[string]any, path string) (any, bool) {
	var v any = m
	for _, k := range strings.Split(path, ".") {
		o, ok := v.(map[string]any)
		if !ok {
			return nil, false
		}
		if v, ok = o[k]; !ok {
			return nil, false
		}
	}
	return v, true
}

// The rendering choices of the lane, checked field by field (the goldens hold them too).
func TestProxyLaneDecisions(t *testing.T) {
	cases := []struct {
		name string
		link string
		want map[string]string // dotted path in the lane → its compact JSON; "" = the field is absent
	}{
		// WebSocket and HTTPUpgrade are HTTP/1.1 upgrades: h2 in the ALPN breaks them on a server that
		// speaks HTTP/2, and the core would offer it as given.
		{"ws: the ALPN of the link becomes http/1.1",
			"vless://" + uuidA + "@example.com:443?security=tls&type=ws&alpn=h2%2Chttp%2F1.1",
			map[string]string{"tls.alpn": `["http/1.1"]`}},
		{"httpupgrade: the same",
			"vless://" + uuidA + "@example.com:443?security=tls&type=httpupgrade&alpn=h2",
			map[string]string{"tls.alpn": `["http/1.1"]`}},
		{"ws without ALPN: the core offers http/1.1 itself",
			"vless://" + uuidA + "@example.com:443?security=tls&type=ws",
			map[string]string{"tls.alpn": "", "tls.enabled": "true"}},
		{"plain TCP keeps the ALPN of the link",
			"trojan://pw@example.com:443?alpn=h2%2Chttp%2F1.1",
			map[string]string{"tls.alpn": `["h2","http/1.1"]`, "transport": ""}},
		{"grpc keeps the ALPN; mode multi is grpc too",
			"vless://" + uuidA + "@example.com:443?security=tls&type=grpc&serviceName=s&mode=multi&alpn=h2",
			map[string]string{"tls.alpn": `["h2"]`, "transport": `{"service_name":"s","type":"grpc"}`}},
		{"grpc without a service name: the core's default",
			"trojan://pw@example.com:443?type=grpc",
			map[string]string{"transport": `{"type":"grpc"}`}},
		{"ws: Host goes into the headers, early data into its header",
			"vless://" + uuidA + "@example.com:443?security=tls&type=ws&host=cdn.example.com&path=%2Fws%3Fed%3D2048",
			map[string]string{"transport": `{"early_data_header_name":"Sec-WebSocket-Protocol","headers":{"Host":"cdn.example.com"},"max_early_data":2048,"path":"/ws","type":"ws"}`}},
		{"httpupgrade: host is its own field",
			"vless://" + uuidA + "@example.com:443?type=httpupgrade&host=cdn.example.com&path=%2Fup",
			map[string]string{"transport": `{"host":"cdn.example.com","path":"/up","type":"httpupgrade"}`, "tls": ""}},
		{"VLESS packetEncoding=none: no encoding, written as an empty string",
			"vless://" + uuidA + "@example.com:443?packetEncoding=none",
			map[string]string{"packet_encoding": `""`}},
		{"VLESS without packetEncoding: left out, the core's default xudp",
			"vless://" + uuidA + "@example.com:443",
			map[string]string{"packet_encoding": "", "flow": ""}},
		{"VLESS packetaddr",
			"vless://" + uuidA + "@example.com:443?packetEncoding=packetaddr",
			map[string]string{"packet_encoding": `"packetaddr"`}},
		{"VMess: the AEAD protocol has no alter id",
			"vmess://" + uuidA + "@example.com:443?encryption=auto",
			map[string]string{"security": `"auto"`, "alter_id": ""}},
		{"REALITY: uTLS on, no short id when the link has none",
			"vless://" + uuidA + "@example.com:443?security=reality&pbk=" + pbk + "&sni=www.example.com",
			map[string]string{"tls.reality": `{"enabled":true,"public_key":"` + pbk + `"}`, "tls.utls": `{"enabled":true,"fingerprint":"chrome"}`,
				"tls.server_name": `"www.example.com"`, "tls.insecure": ""}},
		{"Hysteria2 without hopping: server_port only",
			"hysteria2://pw@example.com:8443",
			map[string]string{"server_port": "8443", "server_ports": "", "obfs": "", "up_mbps": "", "down_mbps": ""}},
		{"Hysteria2 mport: the port of the address joins the ranges, server_port is left out",
			"hy2://pw@example.com:8443?mport=20000-30000",
			map[string]string{"server_port": "", "server_ports": `["8443:8443","20000:30000"]`}},
		{"Hysteria2 mport that holds the port",
			"hy2://pw@example.com:25000?mport=20000-30000,40000",
			map[string]string{"server_port": "", "server_ports": `["20000:30000","40000:40000"]`}},
		{"Hysteria2 port list in the address",
			"hysteria2://pw@example.com:443,5000-6000",
			map[string]string{"server_port": "", "server_ports": `["443:443","5000:6000"]`}},
		{"TUIC: defaults left out",
			"tuic://" + uuidA + ":pw@example.com:443",
			map[string]string{"congestion_control": "", "udp_relay_mode": "", "zero_rtt_handshake": "", "tls.enabled": "true"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := decode(t, mustBuild(t, proxyOptions(`C:\ProgramData\SplitLane`, mustParse(t, c.link))))
			lane, _ := laneOf(t, cfg)
			for path, want := range c.want {
				v, ok := lookup(lane, path)
				got := ""
				if ok {
					b, _ := json.Marshal(v)
					got = string(b)
				}
				if got != want {
					t.Errorf("%s = %s, want %s", path, got, want)
				}
			}
		})
	}
}

func TestWireGuardEndpoint(t *testing.T) {
	s := proxyCase{name: "wg", text: wgConf, conf: true}.server(t)
	lane, endpoint := laneOf(t, decode(t, mustBuild(t, proxyOptions(`C:\ProgramData\SplitLane`, s))))
	got, _ := json.Marshal(lane)
	want := `{"address":["10.8.0.2/32","fd00:8::2/128"],"mtu":1420,"peers":[{"address":"vpn.example.com","allowed_ips":["0.0.0.0/0","::/0"],` +
		`"persistent_keepalive_interval":25,"port":51820,"pre_shared_key":"` + wgPSK + `","public_key":"` + wgPub + `"}],` +
		`"private_key":"` + wgPriv + `","tag":"vpn","type":"wireguard"}`
	if !endpoint || string(got) != want {
		t.Fatalf("endpoint %v:\n%s\nwant\n%s", endpoint, got, want)
	}
	// Without the optional keys: no MTU, no preshared key, no keepalive.
	bare := `[Interface]
PrivateKey = ` + wgPriv + `
Address = 10.8.0.2/32
[Peer]
PublicKey = ` + wgPub + `
Endpoint = 192.0.2.60:51820
AllowedIPs = 0.0.0.0/0
`
	s = proxyCase{name: "bare", text: bare, conf: true}.server(t)
	lane, _ = laneOf(t, decode(t, mustBuild(t, proxyOptions(`C:\ProgramData\SplitLane`, s))))
	got, _ = json.Marshal(lane)
	want = `{"address":["10.8.0.2/32"],"peers":[{"address":"192.0.2.60","allowed_ips":["0.0.0.0/0"],"port":51820,"public_key":"` + wgPub + `"}],` +
		`"private_key":"` + wgPriv + `","tag":"vpn","type":"wireguard"}`
	if string(got) != want {
		t.Fatalf("bare endpoint:\n%s\nwant\n%s", got, want)
	}
}

// A lane is either Tailscale or one server, never neither or both; a server the parser could not have
// made is refused rather than rendered half. The error names what is wrong, never a value of the
// server (it reaches config_invalid and the logs).
func TestBuildRejectsBadLanes(t *testing.T) {
	const pw = "S3cret-pw"
	trojan := func() *connector.Server { return mustParse(t, "trojan://"+pw+"@example.com:443?sni=example.com") }
	cases := map[string]func(o *Options){
		"no connector":    func(o *Options) { o.Lane = Lane{} },
		"both connectors": func(o *Options) { o.Lane.Proxy = trojan() },
		"unknown protocol": func(o *Options) {
			s := trojan()
			s.Protocol = "socks"
			o.Lane = Lane{Proxy: s}
		},
		"a protocol without its settings": func(o *Options) {
			s := trojan()
			s.Protocol = connector.ProtocolVLESS
			o.Lane = Lane{Proxy: s}
		},
		"the settings of two protocols": func(o *Options) {
			s := trojan()
			s.VLESS = &connector.VLESS{UUID: uuidA}
			o.Lane = Lane{Proxy: s}
		},
		"no address": func(o *Options) {
			s := trojan()
			s.Host = ""
			o.Lane = Lane{Proxy: s}
		},
		"port 0": func(o *Options) {
			s := trojan()
			s.Port = 0
			o.Lane = Lane{Proxy: s}
		},
		"port 65536": func(o *Options) {
			s := trojan()
			s.Port = 65536
			o.Lane = Lane{Proxy: s}
		},
		"unknown transport": func(o *Options) {
			s := trojan()
			s.Transport = &connector.Transport{Type: "xhttp"}
			o.Lane = Lane{Proxy: s}
		},
		"ws with two hosts": func(o *Options) {
			s := trojan()
			s.Transport = &connector.Transport{Type: connector.TransportWS, Host: []string{"a.example.com", "b.example.com"}}
			o.Lane = Lane{Proxy: s}
		},
		"QUIC transport without TLS": func(o *Options) {
			s := trojan()
			s.TLS, s.Transport = nil, &connector.Transport{Type: connector.TransportQUIC}
			o.Lane = Lane{Proxy: s}
		},
		"REALITY without a fingerprint": func(o *Options) {
			s := trojan()
			s.TLS.Reality = &connector.Reality{PublicKey: pbk}
			o.Lane = Lane{Proxy: s}
		},
		"REALITY without a key": func(o *Options) {
			s := trojan()
			s.TLS.Fingerprint, s.TLS.Reality = "chrome", &connector.Reality{}
			o.Lane = Lane{Proxy: s}
		},
		"unknown VLESS packet encoding": func(o *Options) {
			s := mustParse(t, "vless://"+uuidA+"@example.com:443")
			s.VLESS.PacketEncoding = "bogus"
			o.Lane = Lane{Proxy: s}
		},
		"a transport on Shadowsocks": func(o *Options) {
			s := mustParse(t, "ss://2022-blake3-aes-256-gcm:"+strings.ReplaceAll(ss256, "=", "%3D")+"@example.com:443")
			s.Transport = &connector.Transport{Type: connector.TransportWS}
			o.Lane = Lane{Proxy: s}
		},
		"TLS on Shadowsocks": func(o *Options) {
			s := mustParse(t, "ss://2022-blake3-aes-256-gcm:"+strings.ReplaceAll(ss256, "=", "%3D")+"@example.com:443")
			s.TLS = &connector.TLS{}
			o.Lane = Lane{Proxy: s}
		},
		"Hysteria2 without TLS": func(o *Options) {
			s := mustParse(t, "hysteria2://"+pw+"@example.com:443")
			s.TLS = nil
			o.Lane = Lane{Proxy: s}
		},
		"Hysteria2 with a reversed port range": func(o *Options) {
			s := mustParse(t, "hysteria2://"+pw+"@example.com:443")
			s.Hysteria2.ServerPorts = []connector.PortRange{{From: 6000, To: 5000}}
			o.Lane = Lane{Proxy: s}
		},
		"Hysteria2 with unknown obfuscation": func(o *Options) {
			s := mustParse(t, "hysteria2://"+pw+"@example.com:443")
			s.Hysteria2.Obfs = "other"
			o.Lane = Lane{Proxy: s}
		},
		"TUIC without TLS": func(o *Options) {
			s := mustParse(t, "tuic://"+uuidA+":"+pw+"@example.com:443")
			s.TLS = nil
			o.Lane = Lane{Proxy: s}
		},
		"TLS on WireGuard": func(o *Options) {
			s := proxyCase{name: "wg", text: wgConf, conf: true}.server(t)
			s.TLS = &connector.TLS{}
			o.Lane = Lane{Proxy: s}
		},
		"WireGuard without allowed IPs": func(o *Options) {
			s := proxyCase{name: "wg", text: wgConf, conf: true}.server(t)
			s.WireGuard.Peer.AllowedIPs = nil
			o.Lane = Lane{Proxy: s}
		},
		"WireGuard with an invalid address": func(o *Options) {
			s := proxyCase{name: "wg", text: wgConf, conf: true}.server(t)
			s.WireGuard.Address = []netip.Prefix{{}}
			o.Lane = Lane{Proxy: s}
		},
	}
	for name, change := range cases {
		o := testOptions(`C:\ProgramData\SplitLane`)
		change(&o)
		_, err := Build(o)
		if err == nil {
			t.Errorf("%s: want an error", name)
			continue
		}
		if !strings.HasPrefix(err.Error(), "coreconfig: ") {
			t.Errorf("%s: %v", name, err)
		}
		for _, secret := range []string{pw, uuidA, wgPriv, "example.com", "socks", "xhttp", "bogus", "other"} {
			if strings.Contains(err.Error(), secret) {
				t.Errorf("%s: the error %q repeats a value of the server", name, err)
			}
		}
	}
}
