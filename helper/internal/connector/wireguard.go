package connector

import (
	"cmp"
	"encoding/base64"
	"encoding/binary"
	"net/netip"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"
)

// amneziaKey matches the keys AmneziaWG adds to [Interface]: junk packets (Jc, Jmin, Jmax), junk
// sizes (S1-S4), header values (H1-H4), signature packets (I1-I5, J1-J3, Itime). The core has none
// of them, and a server that expects them does not answer plain WireGuard.
var amneziaKey = regexp.MustCompile(`^(?:jc|jmin|jmax|itime|[shij][0-9]+)$`)

// ignoredInterfaceKeys: DNS (the lane resolves names through its own encrypted DNS), ListenPort
// (the core picks the local port), and the wg-quick keys for Linux routing and scripts (Table,
// PreUp, PostUp, PreDown, PostDown, SaveConfig, FwMark): SplitLane runs no scripts and routes by
// program, so none of them changes anything here.
var ignoredInterfaceKeys = map[string]bool{
	"dns": true, "listenport": true, "table": true, "preup": true, "postup": true,
	"predown": true, "postdown": true, "saveconfig": true, "fwmark": true,
}

// ParseWireGuardConf parses a WireGuard .conf with one [Interface] and exactly one [Peer]. Keys and
// section names are case-insensitive, '#' starts a comment, Address and AllowedIPs may repeat and
// hold comma lists, a bare address is a host prefix; the AllowedIPs must together cover all public
// IPv4 (wireguard_partial otherwise: the lane sends everything through the server; see
// notPublicIPv4 for the ranges a file may leave out). name is the file's
// base name (".conf" is cut, and so is anything up to a '/' or '\'); when it is empty the server is
// called host:port. The error is always a *Rejection; Line is the line of the problem, 0 when it is
// the whole file.
func ParseWireGuardConf(text []byte, name string) (Server, error) {
	s, r := parseConf(text, name)
	if r != nil {
		return Server{}, r
	}
	return s, nil
}

func parseConf(text []byte, name string) (Server, *Rejection) {
	if len(text) > MaxConfLen {
		return Server{}, reject(CodeTooLong, "conf")
	}
	if !utf8.Valid(text) {
		return Server{}, reject(CodeBadWireGuard, "not UTF-8")
	}
	src := strings.TrimPrefix(string(text), bom)
	fail := func(line int, code, d string) (Server, *Rejection) {
		return Server{}, &Rejection{Line: line, Code: code, Detail: d}
	}
	var (
		section           string
		interfaces, peers int
		seen              = map[string]bool{}
		wg                WireGuard
		host              string
		port              int
	)
	for i, l := range splitLines(src) {
		n := i + 1
		if j := strings.IndexByte(l, '#'); j >= 0 {
			l = l[:j]
		}
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		if l[0] == '[' {
			if l[len(l)-1] != ']' {
				return fail(n, CodeBadWireGuard, "section")
			}
			switch section = strings.ToLower(strings.TrimSpace(l[1 : len(l)-1])); section {
			case "interface":
				if interfaces++; interfaces > 1 {
					return fail(n, CodeBadWireGuard, "more than one [Interface]")
				}
			case "peer":
				if peers++; peers > 1 {
					return fail(n, CodeBadWireGuard, "more than one [Peer]")
				}
			default:
				return fail(n, CodeBadWireGuard, "unknown section")
			}
			continue
		}
		k, v, ok := strings.Cut(l, "=")
		if !ok {
			return fail(n, CodeBadWireGuard, "line without =")
		}
		key, v := strings.ToLower(strings.TrimSpace(k)), strings.TrimSpace(v)
		if amneziaKey.MatchString(key) {
			return fail(n, CodeAmneziaWG, detail("key", key))
		}
		if section == "" {
			return fail(n, CodeBadWireGuard, "key outside a section")
		}
		id := section + "." + key
		if seen[id] && key != "address" && key != "allowedips" && key != "dns" {
			return fail(n, CodeBadWireGuard, detail("repeated key", key))
		}
		seen[id] = true
		switch id {
		case "interface.privatekey":
			if wg.PrivateKey, ok = wgKey(v); !ok {
				return fail(n, CodeBadWireGuard, "PrivateKey")
			}
		case "interface.address":
			ps, ok := prefixList(v, false)
			if !ok {
				return fail(n, CodeBadWireGuard, "Address")
			}
			wg.Address = append(wg.Address, ps...)
		case "interface.mtu":
			m, ok := parseUint(v, 65535)
			if !ok || m < 576 {
				return fail(n, CodeBadWireGuard, "MTU")
			}
			wg.MTU = m
		case "peer.publickey":
			if wg.Peer.PublicKey, ok = wgKey(v); !ok {
				return fail(n, CodeBadWireGuard, "PublicKey")
			}
		case "peer.presharedkey":
			if wg.Peer.PresharedKey, ok = wgKey(v); !ok {
				return fail(n, CodeBadWireGuard, "PresharedKey")
			}
		case "peer.endpoint":
			h, p, r := splitHostPort(v)
			if r != nil || p == "" {
				return fail(n, CodeBadWireGuard, "Endpoint")
			}
			if port, ok = parsePort(p); !ok {
				return fail(n, CodeBadWireGuard, "Endpoint")
			}
			host = h
		case "peer.allowedips":
			ps, ok := prefixList(v, true)
			if !ok {
				return fail(n, CodeBadWireGuard, "AllowedIPs")
			}
			wg.Peer.AllowedIPs = append(wg.Peer.AllowedIPs, ps...)
		case "peer.persistentkeepalive":
			ka := 0
			if !strings.EqualFold(v, "off") {
				if ka, ok = parseUint(v, 65535); !ok {
					return fail(n, CodeBadWireGuard, "PersistentKeepalive")
				}
			}
			wg.Peer.PersistentKeepalive = ka
		default:
			if section != "interface" || !ignoredInterfaceKeys[key] {
				return fail(n, CodeBadWireGuard, detail("unknown key", key))
			}
		}
	}
	switch {
	case interfaces == 0:
		return fail(0, CodeBadWireGuard, "no [Interface]")
	case peers == 0:
		return fail(0, CodeBadWireGuard, "no [Peer]")
	case wg.PrivateKey == "":
		return fail(0, CodeMissingField, "PrivateKey")
	case len(wg.Address) == 0:
		return fail(0, CodeMissingField, "Address")
	case wg.Peer.PublicKey == "":
		return fail(0, CodeMissingField, "PublicKey")
	case host == "":
		return fail(0, CodeMissingField, "Endpoint")
	case len(wg.Peer.AllowedIPs) == 0:
		return fail(0, CodeMissingField, "AllowedIPs")
	case !coversPublicIPv4(wg.Peer.AllowedIPs):
		return fail(0, CodeWireGuardPartial, "AllowedIPs")
	}
	if name = confBaseName(name); name == "" {
		name = hostPortName(host, port)
	}
	return Server{Name: name, Protocol: ProtocolWireGuard, Host: host, Port: port, Text: normalize(src), WireGuard: &wg}, nil
}

// notPublicIPv4 are the IPv4 ranges a .conf need not send through the server: "this network",
// private, shared (CGNAT and Tailscale), loopback, link-local, multicast and reserved. SplitLane
// never sends them through the VPN lane anyway: private, loopback and link-local addresses go direct
// by a route rule (ip_is_private), 100.64.0.0/10 is kept off its adapter, and 0.0.0.0/8 and
// 224.0.0.0/3 are not reached over the internet. So a file made with "Exclude private IPs" of the
// WireGuard apps, which lists every public range but these, is full-tunnel for SplitLane.
var notPublicIPv4 = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("224.0.0.0/3"),
}

// coversPublicIPv4 tells whether the IPv4 prefixes among ps together cover all public IPv4, all of
// IPv4 but notPublicIPv4: the ranges of the prefixes and of notPublicIPv4, sorted by their first
// address and joined, must run from 0.0.0.0 to 255.255.255.255 without a gap (0.0.0.0/1 with
// 128.0.0.0/1 does; overlaps are fine). A split-tunnel file sends only some addresses through the
// server, and the lane's other traffic would fail its proof. IPv6 prefixes do not count: the lane
// needs no IPv6 through the server.
func coversPublicIPv4(ps []netip.Prefix) bool {
	type span struct{ from, to uint64 } // inclusive; uint64, so that the end of IPv4 plus one fits
	var spans []span
	for _, p := range append(slices.Clip(ps), notPublicIPv4...) {
		if !p.Addr().Is4() {
			continue
		}
		p = p.Masked()
		a := p.Addr().As4()
		from := uint64(binary.BigEndian.Uint32(a[:]))
		spans = append(spans, span{from, from + 1<<(32-p.Bits()) - 1})
	}
	slices.SortFunc(spans, func(a, b span) int { return cmp.Compare(a.from, b.from) })
	next := uint64(0) // the first address no span has covered yet
	for _, s := range spans {
		if s.from > next {
			return false
		}
		next = max(next, s.to+1)
	}
	return next == 1<<32
}

// wgKey checks a key of 32 bytes in standard base64 (44 characters), as wg writes it.
func wgKey(v string) (string, bool) {
	if len(v) != 44 {
		return "", false
	}
	b, err := base64.StdEncoding.DecodeString(v)
	return v, err == nil && len(b) == 32
}

// prefixList reads "10.0.0.2/32, fd00::2/128, 10.0.0.3"; masked zeroes the host bits.
func prefixList(v string, masked bool) ([]netip.Prefix, bool) {
	var out []netip.Prefix
	for _, it := range strings.Split(v, ",") {
		it = strings.TrimSpace(it)
		var p netip.Prefix
		if strings.Contains(it, "/") {
			var err error
			if p, err = netip.ParsePrefix(it); err != nil {
				return nil, false
			}
		} else {
			a, err := netip.ParseAddr(it)
			if err != nil || a.Zone() != "" {
				return nil, false
			}
			p = netip.PrefixFrom(a, a.BitLen())
		}
		if masked {
			p = p.Masked()
		}
		out = append(out, p)
	}
	return out, true
}
