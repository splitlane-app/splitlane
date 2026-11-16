package connector

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"reflect"
	"slices"
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

// androidExcludePrivate is the AllowedIPs the WireGuard apps write with "Exclude private IPs": every
// public range, without 10/8, 172.16/12, 192.168/16, 224/3 and the like.
var androidExcludePrivate = []string{"0.0.0.0/5", "8.0.0.0/7", "11.0.0.0/8", "12.0.0.0/6", "16.0.0.0/4", "32.0.0.0/3",
	"64.0.0.0/2", "128.0.0.0/3", "160.0.0.0/5", "168.0.0.0/6", "172.0.0.0/12", "172.32.0.0/11", "172.64.0.0/10",
	"172.128.0.0/9", "173.0.0.0/8", "174.0.0.0/7", "176.0.0.0/4", "192.0.0.0/9", "192.128.0.0/11", "192.160.0.0/13",
	"192.169.0.0/16", "192.170.0.0/15", "192.172.0.0/14", "192.176.0.0/12", "192.192.0.0/10", "193.0.0.0/8",
	"194.0.0.0/7", "196.0.0.0/6", "200.0.0.0/5", "208.0.0.0/4"}

// reversed is a reversed copy of list.
func reversed(list []string) []string {
	out := slices.Clone(list)
	slices.Reverse(out)
	return out
}

// without is list with the prefix p left out.
func without(list []string, p string) []string {
	var out []string
	for _, x := range list {
		if x != p {
			out = append(out, x)
		}
	}
	return out
}

// allBut is the fewest prefixes that cover all of IPv4 but the one address a.
func allBut(a string) []netip.Prefix {
	x := uint64(binary.BigEndian.Uint32(netip.MustParseAddr(a).AsSlice()))
	var out []netip.Prefix
	if x > 0 {
		out = append(out, spanPrefixes(0, x-1)...)
	}
	if x < 1<<32-1 {
		out = append(out, spanPrefixes(x+1, 1<<32-1)...)
	}
	return out
}

// spanPrefixes is the fewest prefixes that cover the addresses from..to.
func spanPrefixes(from, to uint64) []netip.Prefix {
	var out []netip.Prefix
	for from <= to {
		bits := 32
		for bits > 0 {
			size := uint64(1) << (33 - bits)
			if from%size != 0 || from+size-1 > to {
				break
			}
			bits--
		}
		var b [4]byte
		binary.BigEndian.PutUint32(b[:], uint32(from))
		out = append(out, netip.PrefixFrom(netip.AddrFrom4(b), bits))
		from += 1 << (32 - bits)
	}
	return out
}

// The AllowedIPs of a .conf must together cover all public IPv4: the ranges are joined, not matched as
// text, and the private, shared, loopback, link-local, multicast and reserved ranges may be left out.
func TestCoversPublicIPv4(t *testing.T) {
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
		{`"Exclude private IPs" of the WireGuard apps`, androidExcludePrivate, true},
		{"the same, the other way round, with IPv6", append([]string{"::/1", "8000::/1"}, reversed(androidExcludePrivate)...), true},
		{"the same without a public /7", without(androidExcludePrivate, "8.0.0.0/7"), false},
		{"the same without a public /8", without(androidExcludePrivate, "193.0.0.0/8"), false},
		{"the same without 169.254-171.255", without(androidExcludePrivate, "168.0.0.0/6"), false},
		{"everything but 10.0.0.0/8", []string{"0.0.0.0/5", "8.0.0.0/7", "11.0.0.0/8", "12.0.0.0/6",
			"16.0.0.0/4", "32.0.0.0/3", "64.0.0.0/2", "128.0.0.0/1"}, true},
		{"everything below 224.0.0.0", []string{"0.0.0.0/1", "128.0.0.0/2", "192.0.0.0/3"}, true},
		{"one half", []string{"0.0.0.0/1"}, false},
		{"the upper half", []string{"128.0.0.0/1"}, false},
		{"a private network", []string{"10.0.0.0/8"}, false},
		{"all the private networks", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}, false},
		{"IPv6 only", []string{"::/0"}, false},
		{"IPv4 written as IPv6", []string{"::ffff:0.0.0.0/96"}, false},
		{"none", nil, false},
	} {
		if got := coversPublicIPv4(prefixes(c.prefixes...)); got != c.want {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
	// one address left out: a public one is a gap, one of the ranges that need not go through the server is not
	for _, c := range []struct {
		address string
		want    bool
	}{
		{"8.8.8.8", false},
		{"1.0.0.0", false},
		{"9.255.255.255", false},
		{"11.0.0.0", false},
		{"100.63.255.255", false},
		{"100.128.0.0", false},
		{"126.255.255.255", false},
		{"128.0.0.0", false},
		{"169.253.255.255", false},
		{"169.255.0.0", false},
		{"172.15.255.255", false},
		{"172.32.0.0", false},
		{"192.167.255.255", false},
		{"192.169.0.0", false},
		{"223.255.255.255", false},
		{"0.0.0.0", true},
		{"0.255.255.255", true},
		{"10.1.2.3", true},
		{"100.64.0.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
		{"172.16.0.0", true},
		{"172.31.255.255", true},
		{"192.168.1.1", true},
		{"224.0.0.1", true},
		{"240.0.0.1", true},
		{"255.255.255.255", true},
	} {
		ps := allBut(c.address)
		if got := coversPublicIPv4(ps); got != c.want {
			t.Errorf("all but %s (%d prefixes): %v, want %v", c.address, len(ps), got, c.want)
		}
	}
	if got := coversPublicIPv4(spanPrefixes(0, 1<<32-1)); !got {
		t.Error("the helper of this test: all of IPv4")
	}
}

// A file made with "Exclude private IPs" of the WireGuard apps is full-tunnel for SplitLane: it is taken
// with its AllowedIPs as written.
func TestParseWireGuardConfExcludePrivateIPs(t *testing.T) {
	conf := strings.Replace(wgConf, "AllowedIPs = 0.0.0.0/0, ::/0", "AllowedIPs = "+strings.Join(androidExcludePrivate, ", ")+", ::/0", 1)
	s, err := ParseWireGuardConf([]byte(conf), "phone.conf")
	if err != nil {
		t.Fatal(err)
	}
	if got := s.WireGuard.Peer.AllowedIPs; len(got) != len(androidExcludePrivate)+1 || got[0] != netip.MustParsePrefix("0.0.0.0/5") {
		t.Fatalf("AllowedIPs %v", got)
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
		{`"Exclude private IPs" without 8.0.0.0/7`,
			strings.Replace(iface+peer, "0.0.0.0/0", strings.Join(without(androidExcludePrivate, "8.0.0.0/7"), ", "), 1), CodeWireGuardPartial, 0},
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
