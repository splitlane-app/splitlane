package coreconfig

import (
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"

	"splitlane/helper/internal/connector"
)

// Lane is the connector of the VPN lane (spec of stage 3 part 1, section 5): exactly one of its
// fields is set. Either way the lane is the outbound or the endpoint tagged TagVPN, so the probes,
// the DNS over HTTPS through the lane and the rules of spec 6.2 do not depend on the connector.
type Lane struct {
	Tailscale *TailscaleLane    // the embedded Tailscale node with an exit node
	Proxy     *connector.Server // one server of a link or a WireGuard .conf, as package connector parsed it
}

// TailscaleLane is the embedded Tailscale node (spec 7).
type TailscaleLane struct {
	Hostname string // <PC>-splitlane
	ExitNode string // IPv4 address of the exit node in the tailnet (spec 7)
}

// check refuses an exit node that is not an IPv4 address and an empty hostname.
func (t *TailscaleLane) check() error {
	if addr, err := netip.ParseAddr(t.ExitNode); err != nil || !addr.Is4() {
		return fmt.Errorf("coreconfig: exit node must be an IPv4 address")
	}
	if t.Hostname == "" {
		return fmt.Errorf("coreconfig: hostname is empty")
	}
	return nil
}

// wireGuardNodeRule: whether rule 0 of spec 6.2 is written for a WireGuard endpoint. The embedded
// Tailscale node hands the router, under the inbound TagVPN, the flows that tailnet peers open to
// the node's own address (rejectNodeInbound). A WireGuard endpoint might do the same with flows the
// server's side opens to the interface address; until the live check (task 9 of the plan of stage 3
// part 1) shows whether it does, the rule is not written for it. This is the one place to switch it
// on. Proxy outbounds have no such flows and never get the rule.
const wireGuardNodeRule = false

// check refuses a lane with no connector or two, and a Tailscale lane without its values. A server
// is checked while it is rendered (proxyLane): the checks and the fields stay together.
func (l Lane) check() error {
	switch {
	case l.Tailscale == nil && l.Proxy == nil:
		return fmt.Errorf("coreconfig: the VPN lane has no connector")
	case l.Tailscale != nil && l.Proxy != nil:
		return fmt.Errorf("coreconfig: the VPN lane has two connectors, Tailscale and a server")
	case l.Tailscale != nil:
		return l.Tailscale.check()
	}
	return nil
}

// vpnLane renders the lane: the outbounds and endpoints tagged TagVPN, whether rule 0 of spec 6.2
// is needed (nodeRule) and whether the API service of the embedded node is (api). A proxy outbound
// gets the destination names as they are, for its server to resolve. No lane sets a resolver of its
// own: the name of a server or a WireGuard peer resolves through route.default_domain_resolver
// (dns-local), as for the Tailscale endpoint, and the DNS section is the same for every lane.
func vpnLane(l Lane, p Paths) (outbounds, endpoints []obj, nodeRule, api bool, err error) {
	if l.Tailscale != nil {
		return nil, []obj{{
			"type":            "tailscale",
			"tag":             TagVPN,
			"state_directory": filepath.ToSlash(p.StateDir),
			"hostname":        l.Tailscale.Hostname,
			"exit_node":       l.Tailscale.ExitNode,
		}}, true, true, nil
	}
	lane, endpoint, err := proxyLane(l.Proxy)
	if err != nil {
		return nil, nil, false, false, err
	}
	if endpoint {
		return nil, []obj{lane}, wireGuardNodeRule, false, nil
	}
	return []obj{lane}, nil, false, false, nil
}

// proxyLane renders a server as the core's outbound, or for WireGuard as its endpoint (sing-box
// 1.11 and later have WireGuard only as an endpoint). The field names are those of sing-box 1.14.4.
// Optional fields the server does not set are left out: the core's defaults apply. Errors name the
// field, never its value: they reach config_invalid and the logs.
func proxyLane(s *connector.Server) (lane obj, endpoint bool, err error) {
	switch {
	case s.Host == "":
		return nil, false, fmt.Errorf("coreconfig: the server has no address")
	case s.Port < 1 || s.Port > 65535:
		return nil, false, fmt.Errorf("coreconfig: the server port must be 1..65535")
	}
	n := 0
	for _, set := range []bool{s.VLESS != nil, s.VMess != nil, s.Trojan != nil, s.Shadowsocks != nil,
		s.Hysteria2 != nil, s.TUIC != nil, s.WireGuard != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		return nil, false, fmt.Errorf("coreconfig: the server must have the settings of its one protocol")
	}
	missing := fmt.Errorf("coreconfig: the server lacks the settings of its protocol")
	o := obj{"tag": TagVPN, "server": s.Host, "server_port": s.Port}
	switch s.Protocol {
	case connector.ProtocolVLESS:
		v := s.VLESS
		if v == nil {
			return nil, false, missing
		}
		o["type"], o["uuid"] = "vless", v.UUID
		if v.Flow != "" {
			o["flow"] = v.Flow
		}
		// The core takes no packet_encoding as its default, xudp, and an empty one as none at all.
		switch v.PacketEncoding {
		case "":
		case "none":
			o["packet_encoding"] = ""
		case "xudp", "packetaddr":
			o["packet_encoding"] = v.PacketEncoding
		default:
			return nil, false, fmt.Errorf("coreconfig: unknown VLESS packet encoding")
		}
		err = stream(o, s)
	case connector.ProtocolVMess:
		v := s.VMess
		if v == nil {
			return nil, false, missing
		}
		o["type"], o["uuid"] = "vmess", v.UUID
		if v.Security != "" {
			o["security"] = v.Security
		}
		if v.AlterID > 0 {
			o["alter_id"] = v.AlterID
		}
		err = stream(o, s)
	case connector.ProtocolTrojan:
		if s.Trojan == nil {
			return nil, false, missing
		}
		o["type"], o["password"] = "trojan", s.Trojan.Password
		err = stream(o, s)
	case connector.ProtocolShadowsocks:
		if s.Shadowsocks == nil {
			return nil, false, missing
		}
		o["type"], o["method"], o["password"] = "shadowsocks", s.Shadowsocks.Method, s.Shadowsocks.Password
		err = bare(s, false)
	case connector.ProtocolHysteria2:
		h := s.Hysteria2
		if h == nil {
			return nil, false, missing
		}
		o["type"], o["password"] = "hysteria2", h.Password
		ports, perr := hopPorts(s.Port, h.ServerPorts)
		if perr != nil {
			return nil, false, perr
		}
		if ports != nil {
			delete(o, "server_port")
			o["server_ports"] = ports
		}
		switch h.Obfs {
		case "":
		case "salamander":
			o["obfs"] = obj{"type": "salamander", "password": h.ObfsPassword}
		default:
			return nil, false, fmt.Errorf("coreconfig: unknown Hysteria2 obfuscation")
		}
		if h.UpMbps > 0 {
			o["up_mbps"] = h.UpMbps
		}
		if h.DownMbps > 0 {
			o["down_mbps"] = h.DownMbps
		}
		err = quic(o, s)
	case connector.ProtocolTUIC:
		u := s.TUIC
		if u == nil {
			return nil, false, missing
		}
		o["type"], o["uuid"], o["password"] = "tuic", u.UUID, u.Password
		if u.CongestionControl != "" {
			o["congestion_control"] = u.CongestionControl
		}
		if u.UDPRelayMode != "" {
			o["udp_relay_mode"] = u.UDPRelayMode
		}
		if u.ZeroRTT {
			o["zero_rtt_handshake"] = true
		}
		err = quic(o, s)
	case connector.ProtocolWireGuard:
		if s.WireGuard == nil {
			return nil, false, missing
		}
		if err := bare(s, false); err != nil {
			return nil, false, err
		}
		ep, err := wireGuardEndpoint(s)
		return ep, true, err
	default:
		return nil, false, fmt.Errorf("coreconfig: unknown protocol of the server")
	}
	if err != nil {
		return nil, false, err
	}
	return o, false, nil
}

// bare refuses a transport on a protocol without one, and TLS unless tlsAllowed.
func bare(s *connector.Server, tlsAllowed bool) error {
	if s.Transport != nil {
		return fmt.Errorf("coreconfig: the protocol of the server has no transport")
	}
	if s.TLS != nil && !tlsAllowed {
		return fmt.Errorf("coreconfig: the protocol of the server has no TLS")
	}
	return nil
}

// quic adds the TLS that Hysteria2 and TUIC always have; they carry no transport.
func quic(o obj, s *connector.Server) error {
	if err := bare(s, true); err != nil {
		return err
	}
	if s.TLS == nil {
		return fmt.Errorf("coreconfig: the protocol of the server needs TLS")
	}
	t, err := renderTLS(s.TLS, nil)
	if err != nil {
		return err
	}
	o["tls"] = t
	return nil
}

// stream adds the TLS and the transport of VLESS, VMess and Trojan.
func stream(o obj, s *connector.Server) error {
	if s.TLS != nil {
		t, err := renderTLS(s.TLS, s.Transport)
		if err != nil {
			return err
		}
		o["tls"] = t
	}
	if s.Transport != nil {
		t, err := renderTransport(s.Transport)
		if err != nil {
			return err
		}
		if s.Transport.Type == connector.TransportQUIC && s.TLS == nil {
			return fmt.Errorf("coreconfig: the QUIC transport needs TLS")
		}
		o["transport"] = t
	}
	return nil
}

// renderTLS is the core's TLS of a server. tr is the transport over it, nil for none.
func renderTLS(t *connector.TLS, tr *connector.Transport) (obj, error) {
	o := obj{"enabled": true}
	if t.ServerName != "" {
		o["server_name"] = t.ServerName
	}
	if t.Insecure {
		o["insecure"] = true
	}
	if alpn := laneALPN(t.ALPN, tr); len(alpn) > 0 {
		o["alpn"] = alpn
	}
	if t.Fingerprint != "" {
		o["utls"] = obj{"enabled": true, "fingerprint": t.Fingerprint}
	}
	if r := t.Reality; r != nil {
		// The core's REALITY client speaks through uTLS only.
		if t.Fingerprint == "" {
			return nil, fmt.Errorf("coreconfig: REALITY needs a uTLS fingerprint")
		}
		if r.PublicKey == "" {
			return nil, fmt.Errorf("coreconfig: REALITY needs a public key")
		}
		ro := obj{"enabled": true, "public_key": r.PublicKey}
		if r.ShortID != "" {
			ro["short_id"] = r.ShortID
		}
		o["reality"] = ro
	}
	return o, nil
}

// laneALPN is the ALPN list the core offers. WebSocket and HTTPUpgrade are upgrades of an HTTP/1.1
// request: offered h2, a server that speaks HTTP/2 (nginx, a CDN) picks it and the upgrade then fails.
// sing-box 1.14.4 offers the list as given (checked on 127.0.0.1: with h2,http/1.1 it negotiated h2
// and sent the upgrade over it, with uTLS too) and offers http/1.1 itself when there is no list. So
// for those two transports a list from the link becomes http/1.1; every other list stays as it is.
func laneALPN(alpn []string, tr *connector.Transport) []string {
	if len(alpn) == 0 {
		return nil
	}
	if tr != nil && (tr.Type == connector.TransportWS || tr.Type == connector.TransportHTTPUpgrade) {
		return []string{"http/1.1"}
	}
	return alpn
}

// renderTransport is the core's V2Ray transport. gRPC modes gun and multi are both the core's grpc
// (gun): the parser keeps no mode.
func renderTransport(tr *connector.Transport) (obj, error) {
	oneHost := func() (string, error) {
		switch len(tr.Host) {
		case 0:
			return "", nil
		case 1:
			return tr.Host[0], nil
		}
		return "", fmt.Errorf("coreconfig: the transport of the server takes one host")
	}
	o := obj{}
	switch tr.Type {
	case connector.TransportWS:
		o["type"] = "ws"
		host, err := oneHost()
		if err != nil {
			return nil, err
		}
		if host != "" {
			o["headers"] = obj{"Host": host} // the core's ws has no host field of its own
		}
		if tr.Path != "" {
			o["path"] = tr.Path
		}
		if tr.MaxEarlyData > 0 {
			o["max_early_data"] = tr.MaxEarlyData
			if tr.EarlyDataHeaderName != "" {
				o["early_data_header_name"] = tr.EarlyDataHeaderName
			}
		}
	case connector.TransportGRPC:
		o["type"] = "grpc"
		if tr.ServiceName != "" {
			o["service_name"] = tr.ServiceName
		}
	case connector.TransportHTTPUpgrade:
		o["type"] = "httpupgrade"
		host, err := oneHost()
		if err != nil {
			return nil, err
		}
		if host != "" {
			o["host"] = host
		}
		if tr.Path != "" {
			o["path"] = tr.Path
		}
	case connector.TransportHTTP:
		o["type"] = "http"
		if len(tr.Host) > 0 {
			o["host"] = tr.Host
		}
		if tr.Path != "" {
			o["path"] = tr.Path
		}
	case connector.TransportQUIC:
		o["type"] = "quic"
	default:
		return nil, fmt.Errorf("coreconfig: unknown transport of the server")
	}
	return o, nil
}

// hopPorts is server_ports of a Hysteria2 server with port hopping, each item "from:to" (the core
// refuses a bare port), or nil without hopping. With server_ports set the core dials only those ports
// and never server_port (checked on 127.0.0.1 with sing-box 1.14.4: no packet went to server_port),
// so server_port is left out and the port of the address, which the server listens on for sure (the
// mport dialect of v2rayN and NekoBox writes the hopping ranges beside it), is added first unless a
// range holds it.
func hopPorts(port int, ranges []connector.PortRange) ([]string, error) {
	if len(ranges) == 0 {
		return nil, nil
	}
	held := false
	out := []string{}
	for _, r := range ranges {
		if r.From < 1 || r.To > 65535 || r.From > r.To {
			return nil, fmt.Errorf("coreconfig: a Hysteria2 port range must be 1..65535, from the lower port")
		}
		held = held || (port >= r.From && port <= r.To)
		out = append(out, strconv.Itoa(r.From)+":"+strconv.Itoa(r.To))
	}
	if !held {
		out = append([]string{strconv.Itoa(port) + ":" + strconv.Itoa(port)}, out...)
	}
	return out, nil
}

// wireGuardEndpoint is the WireGuard endpoint of a .conf with one peer, the server's Host and Port.
func wireGuardEndpoint(s *connector.Server) (obj, error) {
	w := s.WireGuard
	address, ok1 := prefixes(w.Address)
	allowed, ok2 := prefixes(w.Peer.AllowedIPs)
	if w.PrivateKey == "" || w.Peer.PublicKey == "" || !ok1 || !ok2 {
		return nil, fmt.Errorf("coreconfig: the WireGuard server needs its keys, addresses and allowed IPs")
	}
	peer := obj{"address": s.Host, "port": s.Port, "public_key": w.Peer.PublicKey, "allowed_ips": allowed}
	if w.Peer.PresharedKey != "" {
		peer["pre_shared_key"] = w.Peer.PresharedKey
	}
	if w.Peer.PersistentKeepalive > 0 {
		peer["persistent_keepalive_interval"] = w.Peer.PersistentKeepalive
	}
	ep := obj{"type": "wireguard", "tag": TagVPN, "address": address, "private_key": w.PrivateKey, "peers": []any{peer}}
	if w.MTU > 0 {
		ep["mtu"] = w.MTU
	}
	return ep, nil
}

// prefixes writes a non-empty list of valid prefixes as text; ok is false otherwise.
func prefixes(ps []netip.Prefix) ([]string, bool) {
	if len(ps) == 0 {
		return nil, false
	}
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		if !p.IsValid() {
			return nil, false
		}
		out = append(out, p.String())
	}
	return out, true
}
