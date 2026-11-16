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
		{"allowed ips are masked and may overlap", strings.Replace(wgConf, "0.0.0.0/0, ::/0", "10.8.0.1/24, 128.0.0.1/1, 0.0.0.0/1", 1), "",
			Server{Name: "vpn.example.com:51820", Protocol: ProtocolWireGuard, Host: "vpn.example.com", Port: 51820,
				WireGuard: &WireGuard{PrivateKey: wgPriv, Address: prefixes("10.8.0.2/32", "fd00:8::2/128"), MTU: 1420,
					Peer: WireGuardPeer{PublicKey: wgPub, PresharedKey: wgPSK, AllowedIPs: prefixes("10.8.0.0/24", "128.0.0.0/1", "0.0.0.0/1"), PersistentKeepalive: 25}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseWireGuardConf([]byte(c.conf), c.file)
			if err != nil {
				t.Fatalf("ParseWireGuardConf: %v", err)
			}
			if got.Text != normalize(c.conf) {
				t.Errorf("Text %q does not follow the normalized text", got.Text)
			}
			got.Text = ""
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %s\nwant %s", dump(got), dump(c.want))
			}
		})
	}
}

// The AllowedIPs of a .conf must together cover all of IPv4: the ranges are joined, not matched as text.
func TestCoversIPv4(t *testing.T) {
	for _, c := range []struct {
		name     string
		prefixes []string
		want     bool
	}{
		{"all of IPv4", []string{"0.0.0.0/0"}, true},
		{"with IPv6", []string{"::/0", "0.0.0.0/0"}, true},
		{"the /1 halves", []string{"0.0.0.0/1", "128.0.0.0/1"}, true},
		{"the /1 halves the other way round", []string{"128.0.0.0/1", "0.0.0.0/1"}, true},
		{"the quarters", []string{"192.0.0.0/2", "0.0.0.0/2", "128.0.0.0/2", "64.0.0.0/2"}, true},
		{"overlapping prefixes", []string{"0.0.0.0/1", "10.0.0.0/8", "64.0.0.0/2", "128.0.0.0/1", "255.255.255.255/32"}, true},
		{"everything but 10.0.0.0/8", []string{"0.0.0.0/5", "8.0.0.0/7", "11.0.0.0/8", "12.0.0.0/6",
			"16.0.0.0/4", "32.0.0.0/3", "64.0.0.0/2", "128.0.0.0/1"}, false},
		{"one half", []string{"0.0.0.0/1"}, false},
		{"the last address left out", []string{"0.0.0.0/1", "128.0.0.0/2", "192.0.0.0/3", "224.0.0.0/4", "240.0.0.0/5", "248.0.0.0/6",
			"252.0.0.0/7", "254.0.0.0/8", "255.0.0.0/9", "255.128.0.0/10", "255.192.0.0/11", "255.224.0.0/12", "255.240.0.0/13",
			"255.248.0.0/14", "255.252.0.0/15", "255.254.0.0/16", "255.255.0.0/17", "255.255.128.0/18", "255.255.192.0/19",
			"255.255.224.0/20", "255.255.240.0/21", "255.255.248.0/22", "255.255.252.0/23", "255.255.254.0/24", "255.255.255.0/25",
			"255.255.255.128/26", "255.255.255.192/27", "255.255.255.224/28", "255.255.255.240/29", "255.255.255.248/30",
			"255.255.255.252/31", "255.255.255.254/32"}, false},
		{"the same with the last address", []string{"0.0.0.0/1", "128.0.0.0/2", "192.0.0.0/3", "224.0.0.0/4", "240.0.0.0/5",
			"248.0.0.0/6", "252.0.0.0/7", "254.0.0.0/8", "255.0.0.0/9", "255.128.0.0/10", "255.192.0.0/11", "255.224.0.0/12",
			"255.240.0.0/13", "255.248.0.0/14", "255.252.0.0/15", "255.254.0.0/16", "255.255.0.0/17", "255.255.128.0/18",
			"255.255.192.0/19", "255.255.224.0/20", "255.255.240.0/21", "255.255.248.0/22", "255.255.252.0/23", "255.255.254.0/24",
			"255.255.255.0/25", "255.255.255.128/26", "255.255.255.192/27", "255.255.255.224/28", "255.255.255.240/29",
			"255.255.255.248/30", "255.255.255.252/31", "255.255.255.254/32", "255.255.255.255/32"}, true},
		{"a private network", []string{"10.0.0.0/8"}, false},
		{"IPv6 only", []string{"::/0"}, false},
		{"IPv4 written as IPv6", []string{"::ffff:0.0.0.0/96"}, false},
		{"none", nil, false},
	} {
		if got := coversIPv4(prefixes(c.prefixes...)); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
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
		{"unspecified endpoint", strings.Replace(iface+peer, "vpn.example.com", "0.0.0.0", 1), CodeBadWireGuard, 6},
		{"multicast endpoint", strings.Replace(iface+peer, "vpn.example.com", "[ff05::2]", 1), CodeBadWireGuard, 6},
		{"bad address", strings.Replace(iface, "10.0.0.2/32", "10.0.0.300/32", 1) + peer, CodeBadWireGuard, 3},
		{"empty address item", strings.Replace(iface, "10.0.0.2/32", "10.0.0.2/32,,", 1) + peer, CodeBadWireGuard, 3},
		{"bad allowed ips", strings.Replace(iface+peer, "0.0.0.0/0", "0.0.0.0/33", 1), CodeBadWireGuard, 7},
		{"split tunnel", strings.Replace(iface+peer, "0.0.0.0/0", "10.8.0.0/24, 192.168.0.0/16", 1), CodeWireGuardPartial, 0},
		{"one half of IPv4", strings.Replace(iface+peer, "0.0.0.0/0", "0.0.0.0/1, ::/0", 1), CodeWireGuardPartial, 0},
		{"IPv6 only", strings.Replace(iface+peer, "0.0.0.0/0", "::/0", 1), CodeWireGuardPartial, 0},
		{"the halves on two lines with a gap", strings.Replace(iface+peer, "AllowedIPs = 0.0.0.0/0", "AllowedIPs = 0.0.0.0/1\nAllowedIPs = 128.0.0.0/2", 1),
			CodeWireGuardPartial, 0},
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
