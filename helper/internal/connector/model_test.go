package connector

import (
	"regexp"
	"strings"
	"testing"
)

func TestServerID(t *testing.T) {
	hex16 := regexp.MustCompile(`^[0-9a-f]{16}$`)
	id := ServerID(linkTrojan)
	if !hex16.MatchString(id) {
		t.Fatalf("ServerID = %q, want 16 hex digits", id)
	}
	if ServerID(linkTrojan) != id {
		t.Error("ServerID is not stable")
	}
	for _, same := range []string{"  " + linkTrojan + "\r\n", "\n\n" + linkTrojan + "\t", "\xef\xbb\xbf" + linkTrojan} {
		if ServerID(same) != id {
			t.Errorf("ServerID(%q) differs from the trimmed link", same)
		}
	}
	if ServerID(strings.Replace(linkTrojan, "sni=example.com", "sni=other.example.com", 1)) == id || ServerID(linkSS) == id {
		t.Error("different servers share an id")
	}

	conf := ServerID(wgConf)
	variants := []string{
		strings.ReplaceAll(wgConf, "\n", "\r\n"),
		"\n\n" + strings.ReplaceAll(wgConf, "\n", "  \n\t") + "\n\n",
		strings.ReplaceAll(wgConf, "\n\n", "\n\n\n\n"),
	}
	for _, v := range variants {
		if ServerID(v) != conf {
			t.Errorf("whitespace changes the id of a .conf: %q", v)
		}
	}
	if ServerID(strings.Replace(wgConf, "MTU = 1420", "MTU = 1280", 1)) == conf {
		t.Error("a changed .conf keeps its id")
	}
}

// The name is not part of the id: a subscription that writes changing things into its names keeps
// its ids; any other change is a different server.
func TestServerIDLeavesTheNameOut(t *testing.T) {
	trojan := "trojan://Pa55-w0rd@example.com:443?sni=example.com"
	vmessJSON := func(ps, port string) string {
		return vmessLink(`{"v":"2","ps":"` + ps + `","add":"example.com","port":"` + port + `","id":"` + uuid1 + `","net":"ws","path":"/vm","tls":"tls"}`)
	}
	groups := [][]string{
		{trojan, trojan + "#A", trojan + "#%F0%9F%87%A9%F0%9F%87%AA%20Germany%20%7C%2012.3%20GB%20left", trojan + "#"},
		{vmessJSON("A", "443"), vmessJSON("B | 3 days left", "443"), vmessJSON("", "443") + "#C",
			vmessLinkURL(`{"v":"2","ps":"D","add":"example.com","port":"443","id":"` + uuid1 + `","net":"ws","path":"/vm","tls":"tls"}`),
			vmessLink(`{"id":"` + uuid1 + `","add":"example.com","PS":"E","v":"2","port":"443","tls":"tls","net":"ws","path":"/vm"}`)},
		{"vmess://" + uuid1 + "@example.com:443?type=ws#A", "vmess://" + uuid1 + "@example.com:443?type=ws#B"},
		{"ss://YWVzLTI1Ni1nY206cGFzcw@192.0.2.5:8388#A", "ss://YWVzLTI1Ni1nY206cGFzcw@192.0.2.5:8388#B"},
	}
	var ids []string
	for _, g := range groups {
		id := ServerID(g[0])
		for _, l := range g[1:] {
			if ServerID(l) != id {
				t.Errorf("%s: id differs from %s", l, g[0])
			}
			s, err := ParseLink(l)
			if err == nil && s.ID != id {
				t.Errorf("%s: parsed id differs", l)
			}
		}
		ids = append(ids, id)
	}
	different := []string{
		trojan + "&allowInsecure=1#A",
		"trojan://Pa55-w0rd@example.com:8443?sni=example.com#A",
		vmessJSON("A", "8443"),
		"vmess://" + uuid1 + "@example.com:443?type=ws&path=%2Fx#A",
		"ss://YWVzLTI1Ni1nY206cGFzcw@192.0.2.6:8388#A",
	}
	for _, l := range different {
		for _, id := range ids {
			if ServerID(l) == id {
				t.Errorf("%s shares an id with another server", l)
			}
		}
	}
	// the name is still kept and shown
	a, _ := ParseLink(trojan + "#A")
	b, _ := ParseLink(trojan + "#B")
	if a.Name != "A" || b.Name != "B" || a.Text == b.Text || a.ID != b.ID {
		t.Errorf("names %q %q, texts %q %q", a.Name, b.Name, a.Text, b.Text)
	}
}

func TestNames(t *testing.T) {
	cases := []struct{ link, want string }{
		{linkTrojan[:strings.IndexByte(linkTrojan, '#')] + "#%20%20Spaced%20%20", "Spaced"},
		{linkTrojan[:strings.IndexByte(linkTrojan, '#')] + "#Line%0Abreak%09tab%E2%80%AEevil", "Linebreaktabevil"},
		{linkTrojan[:strings.IndexByte(linkTrojan, '#')] + "#100%", "100%"},
		{linkTrojan[:strings.IndexByte(linkTrojan, '#')] + "#a+b", "a+b"},
		{linkTrojan[:strings.IndexByte(linkTrojan, '#')] + "#" + strings.Repeat("я", MaxNameLen+10), strings.Repeat("я", MaxNameLen)},
		{linkTrojan[:strings.IndexByte(linkTrojan, '#')] + "#%20", "example.com:443"},
		{linkTrojan[:strings.IndexByte(linkTrojan, '#')] + "#Сервер", "Сервер"},
	}
	for _, c := range cases {
		s, err := ParseLink(c.link)
		if err != nil {
			t.Fatalf("%s: %v", c.link, err)
		}
		if s.Name != c.want {
			t.Errorf("%s: name %q, want %q", c.link, s.Name, c.want)
		}
	}
}

func TestOneProtocolBlock(t *testing.T) {
	for _, c := range linkCases() {
		s, err := ParseLink(c.link)
		if err != nil {
			t.Fatal(err)
		}
		checkServer(t, s)
	}
	s, err := ParseWireGuardConf([]byte(wgConf), "")
	if err != nil {
		t.Fatal(err)
	}
	checkServer(t, s)
}

// checkServer holds the invariants every parsed server keeps (also used by the fuzz tests).
func checkServer(t *testing.T, s Server) {
	t.Helper()
	blocks := map[string]bool{
		ProtocolVLESS:       s.VLESS != nil,
		ProtocolVMess:       s.VMess != nil,
		ProtocolTrojan:      s.Trojan != nil,
		ProtocolShadowsocks: s.Shadowsocks != nil,
		ProtocolHysteria2:   s.Hysteria2 != nil,
		ProtocolTUIC:        s.TUIC != nil,
		ProtocolWireGuard:   s.WireGuard != nil,
	}
	n := 0
	for p, set := range blocks {
		if set {
			n++
			if p != s.Protocol {
				t.Errorf("protocol %q carries the %s block", s.Protocol, p)
			}
		}
	}
	if n != 1 {
		t.Errorf("%d protocol blocks in %s", n, dump(s))
	}
	if s.Port < 1 || s.Port > 65535 || s.Host == "" || s.Name == "" || len(s.ID) != 16 || s.ID != ServerID(s.Text) {
		t.Errorf("bad server %s", dump(s))
	}
	if (s.Protocol == ProtocolHysteria2 || s.Protocol == ProtocolTUIC) && (s.TLS == nil || s.Transport != nil) {
		t.Errorf("%s needs TLS and no transport: %s", s.Protocol, dump(s))
	}
	if (s.Protocol == ProtocolShadowsocks || s.Protocol == ProtocolWireGuard) && (s.TLS != nil || s.Transport != nil) {
		t.Errorf("%s has TLS or a transport: %s", s.Protocol, dump(s))
	}
	if s.Hysteria2 != nil {
		for _, r := range s.Hysteria2.ServerPorts {
			if r.From < 1 || r.To > 65535 || r.From > r.To {
				t.Errorf("bad port range %v", r)
			}
		}
	}
}
