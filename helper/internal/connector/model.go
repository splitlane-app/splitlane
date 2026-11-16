// Package connector holds the server model of the proxy connector (spec of stage 3 part 1,
// sections 1-3) and the strict parsers that fill it from share links, WireGuard .conf files and
// subscription bodies.
//
// The model is core-neutral: coreconfig renders it for sing-box. It names no file, no network
// interface and no other outbound, so nothing that comes from a link can make the core read a file
// or leave the lane. Every value is checked here; what a parser does not understand is refused with
// a code, never dropped silently.
package connector

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Protocols of a server (Server.Protocol).
const (
	ProtocolVLESS       = "vless"
	ProtocolVMess       = "vmess"
	ProtocolTrojan      = "trojan"
	ProtocolShadowsocks = "shadowsocks"
	ProtocolHysteria2   = "hysteria2"
	ProtocolTUIC        = "tuic"
	ProtocolWireGuard   = "wireguard"
)

// Transports of VLESS, VMess and Trojan (Transport.Type). No transport is plain TCP.
const (
	TransportWS          = "ws"
	TransportGRPC        = "grpc"
	TransportHTTPUpgrade = "httpupgrade"
	TransportHTTP        = "http" // HTTP/2: type=http or type=h2 in links
	TransportQUIC        = "quic"
)

// Rejection codes: the closed list the window has texts for.
const (
	CodeUnsupportedProtocol  = "unsupported_protocol"  // a scheme, cipher, VLESS encryption or flow the core lacks
	CodeUnsupportedTransport = "unsupported_transport" // a transport, header type or TLS kind the core lacks
	CodeBadLink              = "bad_link"              // malformed or unknown: a value, a parameter, an encoding
	CodeMissingField         = "missing_field"         // a required part is absent
	CodeBadWireGuard         = "bad_wireguard"         // a malformed .conf
	CodeAmneziaWG            = "amneziawg"             // AmneziaWG parameters in a .conf
	CodeWireGuardPartial     = "wireguard_partial"     // a .conf whose AllowedIPs leave part of IPv4 off the tunnel
	CodePlugin               = "plugin"                // a Shadowsocks plugin (SIP003)
	CodeTooLong              = "too_long"              // over a size limit
)

// Limits. MaxTextLen and MaxSubscriptionLen are the limits of proxy.add and of a subscription
// response; the others keep a single item to a sane size.
const (
	MaxLinkLen         = 8 << 10
	MaxTextLen         = 64 << 10
	MaxConfLen         = 64 << 10
	MaxSubscriptionLen = 1 << 20
	MaxNameLen         = 100 // characters
)

// Server is one imported server. Name, Protocol, Host and Port are not secret; everything else (Text and
// the protocol blocks) is, and lives only in servers.json. Its id is ServerID of Text under the key of the
// install: the caller, who holds the key, makes it.
type Server struct {
	Name     string // from the link (#name, vmess "ps"), the .conf file name, else host:port
	Protocol string // one of the Protocol* constants; exactly the matching block below is set
	Host     string // a DNS name or an IP address (IPv6 without brackets)
	Port     int    // 1-65535

	// Text is the normalized source text: one link, or a whole .conf. It is what servers.json
	// keeps; the server is parsed from it again whenever the core config is built.
	Text string

	TLS       *TLS       // nil: no TLS. Hysteria2 and TUIC always have it.
	Transport *Transport // nil: plain TCP (VLESS, VMess, Trojan), or the protocol's own (QUIC, UDP)

	VLESS       *VLESS
	VMess       *VMess
	Trojan      *Trojan
	Shadowsocks *Shadowsocks
	Hysteria2   *Hysteria2
	TUIC        *TUIC
	WireGuard   *WireGuard
}

// TLS of a server.
type TLS struct {
	ServerName  string   // SNI; "" = the core's default (Host)
	Insecure    bool     // skip the certificate check (allowInsecure=1 and the like)
	ALPN        []string // nil = the core's default
	Fingerprint string   // uTLS fingerprint (chrome, firefox, edge, safari, 360, qq, ios, android, random, randomized); "" = none
	Reality     *Reality // non-nil: REALITY instead of a certificate; Fingerprint is then always set
}

// Reality is the REALITY part of TLS.
type Reality struct {
	PublicKey string // 32 bytes, base64url without padding
	ShortID   string // lower-case hex, 0-16 characters, even length
}

// Transport of VLESS, VMess and Trojan.
type Transport struct {
	Type string // one of the Transport* constants

	// Host is the Host header of ws, the host of httpupgrade (at most one each) or the host list
	// of http.
	Host []string
	Path string // ws, httpupgrade, http; starts with "/" when set

	ServiceName string // grpc; "" = the core's default

	// Early data of ws: MaxEarlyData bytes of the first packet ride in the header
	// EarlyDataHeaderName (Sec-WebSocket-Protocol: the Xray convention of "?ed=" in the path).
	MaxEarlyData        int
	EarlyDataHeaderName string
}

// VLESS settings.
type VLESS struct {
	UUID string // lower case
	Flow string // "" or "xtls-rprx-vision" (TLS or REALITY over plain TCP only)

	// PacketEncoding of UDP: "" = the core's default (xudp), "xudp", "packetaddr", or "none"
	// for none at all.
	PacketEncoding string
}

// VMess settings.
type VMess struct {
	UUID     string // lower case
	Security string // auto, aes-128-gcm, chacha20-poly1305, none, zero
	AlterID  int    // 0 = AEAD; > 0 = the legacy protocol
}

// Trojan settings.
type Trojan struct {
	Password string
}

// Shadowsocks settings: AEAD and Shadowsocks 2022 methods only.
type Shadowsocks struct {
	Method   string // aes-128-gcm, aes-192-gcm, aes-256-gcm, chacha20-ietf-poly1305, xchacha20-ietf-poly1305, 2022-blake3-*
	Password string // for 2022: base64 keys of the method's size, "iPSK:uPSK" for a multi-user server
}

// Hysteria2 settings. The hop interval is not carried: no link dialect writes it, so the core's
// default applies.
type Hysteria2 struct {
	Password string // the auth string; "user:pass" for userpass auth

	// ServerPorts are port-hopping ranges (mport, or a port list in the address); when set the
	// client dials ports from them. Port is then the first port of the list or the port of the
	// address.
	ServerPorts []PortRange

	Obfs         string // "" or "salamander"
	ObfsPassword string // set exactly when Obfs is

	UpMbps, DownMbps int // 0 = unset (the core's congestion control); else Brutal at these rates
}

// PortRange is an inclusive range of ports; a single port has From == To.
type PortRange struct {
	From, To int
}

// TUIC v5 settings.
type TUIC struct {
	UUID              string // lower case
	Password          string
	CongestionControl string // "" (the core's default), cubic, new_reno, bbr
	UDPRelayMode      string // "" (the core's default), native, quic
	ZeroRTT           bool
}

// WireGuard settings of a .conf with one peer; the peer's endpoint is Server.Host and Port.
type WireGuard struct {
	PrivateKey string         // standard base64, 32 bytes
	Address    []netip.Prefix // the interface addresses, as written (10.0.0.2/24 stays so)
	MTU        int            // 0 = the core's default
	Peer       WireGuardPeer
}

// WireGuardPeer is the peer of a .conf.
type WireGuardPeer struct {
	PublicKey           string         // standard base64, 32 bytes
	PresharedKey        string         // "" or standard base64, 32 bytes
	AllowedIPs          []netip.Prefix // masked (10.0.0.1/24 becomes 10.0.0.0/24)
	PersistentKeepalive int            // seconds; 0 = off
}

// The closed sets of values the model holds. The parsers refuse anything else, and coreconfig checks
// a server against the same sets (the Known* functions) before it renders it. "" is in no set:
// whether a field may be empty is said by its comment above.
var (
	flows              = set("xtls-rprx-vision")
	packetEncodings    = set("xudp", "packetaddr", "none")
	vmessSecurities    = set("auto", "aes-128-gcm", "chacha20-poly1305", "none", "zero")
	fingerprints       = set("chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized") // of uTLS, all known to the core
	congestionControls = set("cubic", "new_reno", "bbr")
	udpRelayModes      = set("native", "quic")
)

// ssMethods: the AEAD methods (0) and the Shadowsocks 2022 methods with their key sizes. Stream
// ciphers and none are refused: they are broken or pointless over the internet.
var ssMethods = map[string]int{
	"aes-128-gcm": 0, "aes-192-gcm": 0, "aes-256-gcm": 0, "chacha20-ietf-poly1305": 0, "xchacha20-ietf-poly1305": 0,
	"2022-blake3-aes-128-gcm": 16, "2022-blake3-aes-256-gcm": 32, "2022-blake3-chacha20-poly1305": 32,
}

func set(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

// KnownFlow tells whether s is a VLESS flow of the model (VLESS.Flow).
func KnownFlow(s string) bool { return flows[s] }

// KnownPacketEncoding tells whether s is a VLESS packet encoding of the model (VLESS.PacketEncoding).
func KnownPacketEncoding(s string) bool { return packetEncodings[s] }

// KnownVMessSecurity tells whether s is a VMess security of the model (VMess.Security).
func KnownVMessSecurity(s string) bool { return vmessSecurities[s] }

// KnownShadowsocksMethod tells whether s is a Shadowsocks method of the model (Shadowsocks.Method).
func KnownShadowsocksMethod(s string) bool {
	_, ok := ssMethods[s]
	return ok
}

// KnownFingerprint tells whether s is a uTLS fingerprint of the model (TLS.Fingerprint).
func KnownFingerprint(s string) bool { return fingerprints[s] }

// KnownCongestionControl tells whether s is a TUIC congestion control of the model (TUIC.CongestionControl).
func KnownCongestionControl(s string) bool { return congestionControls[s] }

// KnownUDPRelayMode tells whether s is a TUIC UDP relay mode of the model (TUIC.UDPRelayMode).
func KnownUDPRelayMode(s string) bool { return udpRelayModes[s] }

// Rejection says why a link, a .conf or a line was refused. Detail is a short English note for
// logs (a parameter name, a field); it never repeats a value of the input.
type Rejection struct {
	Line   int // 1-based line of a multi-line text; 0 = the whole text
	Code   string
	Detail string
}

func (r *Rejection) Error() string {
	if r.Detail == "" {
		return r.Code
	}
	return r.Code + ": " + r.Detail
}

func reject(code, detail string) *Rejection {
	return &Rejection{Code: code, Detail: detail}
}

// ServerID is the id of a server: the first 8 bytes, in hex, of the HMAC-SHA256 under key (the random key
// of the install, kept in servers.json) of its identity: its normalized text without its name. Keyed, an id
// says nothing of the secrets in the text: ids go into state.json and the log, and so into diagnostics
// archives, where an unkeyed hash of a link with a weak password could be matched by guessing offline.
// Under the same key the same server always has the same id: leading and trailing spaces of every line,
// line endings, blank lines and a byte order mark do not change it, and neither does the name: a link is
// taken without the part from its first '#', a vmess:// base64 link as its JSON without "ps" (keys
// sorted). So a subscription that writes changing things into its names (traffic left, a load mark) keeps
// the ids of its servers across refreshes; the name itself is still kept in Text and shown. A .conf
// carries no name (the name is the file's), so it is taken whole.
func ServerID(key []byte, text string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(identity(text)))
	return hex.EncodeToString(mac.Sum(nil)[:8])
}

// identity is what ServerID hashes: the normalized text, for one link without its name. Two texts of the
// same server have the same identity, so the parsers count such a server once. It is as secret as the text.
func identity(text string) string {
	n := normalize(text)
	if !strings.Contains(n, "\n") {
		n = withoutName(n)
	}
	return n
}

// withoutName cuts the name out of one link for identity.
func withoutName(link string) string {
	link, _, _ = strings.Cut(link, "#")
	i := strings.Index(link, "://")
	if i <= 0 || !strings.EqualFold(link[:i], "vmess") || strings.ContainsAny(link[i+3:], "@?") {
		return link // the Xray URI form of vmess has its name in the fragment only
	}
	raw, ok := decodeBase64(link[i+3:])
	if !ok {
		return link
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if dec.Decode(&m) != nil || m == nil {
		return link
	}
	for k := range m {
		if strings.EqualFold(k, "ps") {
			delete(m, k)
		}
	}
	b, err := json.Marshal(m) // map keys come out sorted
	if err != nil {
		return link
	}
	return "vmess-json:" + string(b)
}

// normalize trims every line, drops blank lines and joins the rest with "\n".
func normalize(text string) string {
	var out []string
	for _, l := range splitLines(strings.TrimPrefix(text, bom)) {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

const bom = "\xef\xbb\xbf"

// splitLines splits on "\r\n", "\n" and a lone "\r".
func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.Split(strings.ReplaceAll(text, "\r", "\n"), "\n")
}

// cleanName makes a display name: valid UTF-8 without control characters and bidirectional
// overrides, trimmed, at most MaxNameLen characters.
func cleanName(s string) string {
	s = strings.ToValidUTF8(s, "")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) > MaxNameLen {
		s = strings.TrimSpace(string([]rune(s)[:MaxNameLen]))
	}
	return s
}

func hostPortName(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// safeWord returns s when it is a short plain word (a parameter name, a transport type) that may go
// into Rejection.Detail, else "".
func safeWord(s string) string {
	if len(s) == 0 || len(s) > 32 {
		return ""
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.') {
			return ""
		}
	}
	return s
}

// detail joins a fixed note with a word of the input that passed safeWord.
func detail(note, word string) string {
	if w := safeWord(word); w != "" {
		return note + " " + w
	}
	return note
}

// confBaseName makes the server name of a .conf from the name its caller gives (a file's base
// name): only the part after the last '/' or '\' counts (a path is never wanted here), ".conf" is
// cut.
func confBaseName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	name = strings.TrimSpace(name)
	if len(name) >= 5 && strings.EqualFold(name[len(name)-5:], ".conf") {
		name = name[:len(name)-5]
	}
	return cleanName(name)
}

// cleanHost checks a server address: an IP address without a zone, or an ASCII DNS name. It returns
// the canonical form (lower case; IPv6 compressed). The unspecified addresses (0.0.0.0, ::) and
// multicast are refused: panels put them into pseudo-entries that carry text, not a server.
// Loopback stays allowed (test servers).
func cleanHost(h string) (string, bool) {
	if a, err := netip.ParseAddr(h); err == nil {
		if u := a.Unmap(); a.Zone() != "" || u.IsUnspecified() || u.IsMulticast() {
			return "", false
		}
		return a.String(), true
	}
	h = strings.TrimSuffix(h, ".")
	if h == "" || len(h) > 253 {
		return "", false
	}
	labels := strings.Split(h, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return "", false
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
				return "", false
			}
		}
	}
	if allDigits(labels[len(labels)-1]) { // 1.2.3 is a broken address, not a name
		return "", false
	}
	return strings.ToLower(h), true
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return s != ""
}

// parsePort reads 1-65535 written in decimal digits only.
func parsePort(s string) (int, bool) {
	if !allDigits(s) || len(s) > 5 {
		return 0, false
	}
	n, _ := strconv.Atoi(s)
	return n, n >= 1 && n <= 65535
}

// parseUint reads a decimal number in [0, max].
func parseUint(s string, max int) (int, bool) {
	if !allDigits(s) || len(s) > 9 {
		return 0, false
	}
	n, _ := strconv.Atoi(s)
	return n, n <= max
}

// splitHostPort splits "host:port", "[v6]:port", "host" or "[v6]"; the port is returned as written.
func splitHostPort(hp string) (host, port string, r *Rejection) {
	if hp == "" {
		return "", "", reject(CodeMissingField, "host")
	}
	if hp[0] == '[' {
		end := strings.IndexByte(hp, ']')
		if end < 0 {
			return "", "", reject(CodeBadLink, "host")
		}
		rest := hp[end+1:]
		if rest != "" {
			if rest[0] != ':' {
				return "", "", reject(CodeBadLink, "host")
			}
			port = rest[1:]
			if port == "" {
				return "", "", reject(CodeBadLink, "port")
			}
		}
		a, err := netip.ParseAddr(hp[1:end])
		if err != nil || !a.Is6() {
			return "", "", reject(CodeBadLink, "host")
		}
		h, ok := cleanHost(a.String())
		if !ok {
			return "", "", reject(CodeBadLink, "host")
		}
		return h, port, nil
	}
	if strings.Count(hp, ":") > 1 {
		return "", "", reject(CodeBadLink, "IPv6 address without brackets")
	}
	host, port, hasPort := strings.Cut(hp, ":")
	if hasPort && port == "" {
		return "", "", reject(CodeBadLink, "port")
	}
	if host == "" {
		return "", "", reject(CodeMissingField, "host")
	}
	h, ok := cleanHost(host)
	if !ok {
		return "", "", reject(CodeBadLink, "host")
	}
	return h, port, nil
}

// parseUUID checks the 8-4-4-4-12 hex form and returns it in lower case.
func parseUUID(s string) (string, bool) {
	if len(s) != 36 {
		return "", false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch i {
		case 8, 13, 18, 23:
			if c != '-' {
				return "", false
			}
		default:
			if !isHex(c) {
				return "", false
			}
		}
	}
	return strings.ToLower(s), true
}

func isHex(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
}
