package connector

import (
	"errors"
	"net/netip"
	"reflect"
	"strings"
	"testing"
)

// Made-up keys: 32 bytes each, standard base64.
const (
	wgPriv = "lhw1Rezx8+44v1b7GrtW7HxgVikNwpLsRNzZk0ZbBXA="
	wgPub  = "ZcQzkt7Yzwta15biT7QDOx+0JlXlF1K5Mnq0qUyDgLY="
	wgPSK  = "G7sxQw6Upoi06QWKlQg49YeU7FjgUsa0xJx95nCpBDk="
)

const wgConf = `[Interface]
PrivateKey = ` + wgPriv + `
Address = 10.8.0.2/32, fd00:8::2/128
DNS = 1.1.1.1, 1.0.0.1
MTU = 1420

[Peer]
PublicKey = ` + wgPub + `
PresharedKey = ` + wgPSK + `
Endpoint = vpn.example.com:51820
AllowedIPs = 0.0.0.0/0, ::/0
PersistentKeepalive = 25
`

func prefixes(s ...string) []netip.Prefix {
	var out []netip.Prefix
	for _, p := range s {
		out = append(out, netip.MustParsePrefix(p))
	}
	return out
}

func TestParseWireGuardConf(t *testing.T) {
	cases := []struct {
		name, conf, file string
		want             Server
	}{
		{"typical client file", wgConf, "office",
			Server{Name: "office", Protocol: ProtocolWireGuard, Host: "vpn.example.com", Port: 51820,
				WireGuard: &WireGuard{PrivateKey: wgPriv, Address: prefixes("10.8.0.2/32", "fd00:8::2/128"), MTU: 1420,
					Peer: WireGuardPeer{PublicKey: wgPub, PresharedKey: wgPSK, AllowedIPs: prefixes("0.0.0.0/0", "::/0"), PersistentKeepalive: 25}}}},
		{"ipv6 endpoint, lower-case keys, comments, crlf, bare addresses, repeated lines",
			"\xef\xbb\xbf# exported by a server panel\r\n[interface]\r\nprivatekey=" + wgPriv + "\r\naddress = 10.0.0.2\r\naddress = 10.0.0.3/24 # second\r\nListenPort = 51000\r\n\r\n[PEER]\r\npublickey = " + wgPub +
				"\r\nendpoint = [2001:db8::1]:51820\r\nallowedips = 0.0.0.0/1\r\nAllowedIPs = 128.0.0.0/1, 2001:db8::/33\r\nPersistentKeepalive = off\r\n",
			"",
			Server{Name: "[2001:db8::1]:51820", Protocol: ProtocolWireGuard, Host: "2001:db8::1", Port: 51820,
				WireGuard: &WireGuard{PrivateKey: wgPriv, Address: prefixes("10.0.0.2/32", "10.0.0.3/24"),
					Peer: WireGuardPeer{PublicKey: wgPub, AllowedIPs: prefixes("0.0.0.0/1", "128.0.0.0/1", "2001:db8::/33")}}}},
		{"name from the file name, ipv4 endpoint, wg-quick keys ignored",
			"[Interface]\nPrivateKey = " + wgPriv + "\nAddress = 172.16.0.2/32\nTable = off\nPostUp = echo up\nPreDown = echo down\nSaveConfig = false\nFwMark = 0x1234\n[Peer]\nPublicKey = " + wgPub +
				"\nEndpoint = 192.0.2.60:443\nAllowedIPs = 0.0.0.0/0\n",
			"home.conf",
			Server{Name: "home", Protocol: ProtocolWireGuard, Host: "192.0.2.60", Port: 443,
				WireGuard: &WireGuard{PrivateKey: wgPriv, Address: prefixes("172.16.0.2/32"),
					Peer: WireGuardPeer{PublicKey: wgPub, AllowedIPs: prefixes("0.0.0.0/0")}}}},
		{"allowed ips are masked", strings.Replace(wgConf, "0.0.0.0/0, ::/0", "10.8.0.1/24", 1), "",
			Server{Name: "vpn.example.com:51820", Protocol: ProtocolWireGuard, Host: "vpn.example.com", Port: 51820,
				WireGuard: &WireGuard{PrivateKey: wgPriv, Address: prefixes("10.8.0.2/32", "fd00:8::2/128"), MTU: 1420,
					Peer: WireGuardPeer{PublicKey: wgPub, PresharedKey: wgPSK, AllowedIPs: prefixes("10.8.0.0/24"), PersistentKeepalive: 25}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseWireGuardConf([]byte(c.conf), c.file)
			if err != nil {
				t.Fatalf("ParseWireGuardConf: %v", err)
			}
			if got.ID != ServerID(c.conf) || got.Text != normalize(c.conf) {
				t.Errorf("ID %q / Text %q do not follow the normalized text", got.ID, got.Text)
			}
			got.ID, got.Text = "", ""
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %s\nwant %s", dump(got), dump(c.want))
			}
		})
	}
}

func TestParseWireGuardConfRejects(t *testing.T) {
	iface := "[Interface]\nPrivateKey = " + wgPriv + "\nAddress = 10.0.0.2/32\n"
	peer := "[Peer]\nPublicKey = " + wgPub + "\nEndpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0\n"
	cases := []struct {
		name, conf, code string
		line             int
	}{
		{"amneziawg junk", iface + "Jc = 4\nJmin = 40\nJmax = 70\nS1 = 0\nS2 = 0\nH1 = 1\nH2 = 2\nH3 = 3\nH4 = 4\n" + peer, CodeAmneziaWG, 4},
		{"amneziawg 1.5 signature", iface + "I1 = <b 0xc200000001>\n" + peer, CodeAmneziaWG, 4},
		{"amneziawg 2 header range", iface + "S3 = 10\n" + peer, CodeAmneziaWG, 4},
		{"two peers", iface + peer + peer, CodeBadWireGuard, 8},
		{"no peer", iface, CodeBadWireGuard, 0},
		{"no interface", peer, CodeBadWireGuard, 0},
		{"two interfaces", iface + iface + peer, CodeBadWireGuard, 4},
		{"short key", strings.Replace(iface, wgPriv, "c2hvcnQ=", 1) + peer, CodeBadWireGuard, 2},
		{"key in url alphabet", strings.Replace(iface+peer, wgPub, strings.NewReplacer("+", "-", "/", "_").Replace(wgPub), 1), CodeBadWireGuard, 5},
		{"bad preshared key", iface + peer + "PresharedKey = nope\n", CodeBadWireGuard, 8},
		{"no private key", "[Interface]\nAddress = 10.0.0.2/32\n" + peer, CodeMissingField, 0},
		{"no address", "[Interface]\nPrivateKey = " + wgPriv + "\n" + peer, CodeMissingField, 0},
		{"no public key", iface + "[Peer]\nEndpoint = vpn.example.com:51820\nAllowedIPs = 0.0.0.0/0\n", CodeMissingField, 0},
		{"no endpoint", iface + "[Peer]\nPublicKey = " + wgPub + "\nAllowedIPs = 0.0.0.0/0\n", CodeMissingField, 0},
		{"no allowed ips", iface + "[Peer]\nPublicKey = " + wgPub + "\nEndpoint = vpn.example.com:51820\n", CodeMissingField, 0},
		{"unknown key", iface + "Junk = 1\n" + peer, CodeBadWireGuard, 4},
		{"unknown section", iface + "[Relay]\n" + peer, CodeBadWireGuard, 4},
		{"key outside a section", "PrivateKey = " + wgPriv + "\n" + iface + peer, CodeBadWireGuard, 1},
		{"line without equals", iface + "Address\n" + peer, CodeBadWireGuard, 4},
		{"duplicate private key", iface + "PrivateKey = " + wgPriv + "\n" + peer, CodeBadWireGuard, 4},
		{"bad endpoint port", strings.Replace(iface+peer, ":51820", ":0", 1), CodeBadWireGuard, 6},
		{"endpoint without port", strings.Replace(iface+peer, ":51820", "", 1), CodeBadWireGuard, 6},
		{"bad address", strings.Replace(iface, "10.0.0.2/32", "10.0.0.300/32", 1) + peer, CodeBadWireGuard, 3},
		{"empty address item", strings.Replace(iface, "10.0.0.2/32", "10.0.0.2/32,,", 1) + peer, CodeBadWireGuard, 3},
		{"bad allowed ips", strings.Replace(iface+peer, "0.0.0.0/0", "0.0.0.0/33", 1), CodeBadWireGuard, 7},
		{"bad mtu", iface + "MTU = 10\n" + peer, CodeBadWireGuard, 4},
		{"bad keepalive", iface + peer + "PersistentKeepalive = soon\n", CodeBadWireGuard, 8},
		{"unclosed section", "[Interface\n" + peer, CodeBadWireGuard, 1},
		{"not utf-8", iface + "# \xff\n" + peer, CodeBadWireGuard, 0},
		{"too long", iface + peer + "# " + strings.Repeat("x", MaxConfLen), CodeTooLong, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, err := ParseWireGuardConf([]byte(c.conf), "x")
			if err == nil {
				t.Fatalf("accepted: %s", dump(s))
			}
			var r *Rejection
			if !errors.As(err, &r) {
				t.Fatalf("error %v is not a *Rejection", err)
			}
			if r.Code != c.code || r.Line != c.line {
				t.Errorf("got %s line %d (%s), want %s line %d", r.Code, r.Line, r.Detail, c.code, c.line)
			}
			if strings.Contains(r.Error(), wgPriv) || strings.Contains(r.Error(), wgPub) {
				t.Errorf("rejection repeats a key: %v", r)
			}
		})
	}
}
