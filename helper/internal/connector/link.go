package connector

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ParseLink parses one share link. The error is always a *Rejection.
//
// Schemes: vless://; vmess:// (the v2rayN base64 JSON and the Xray URI form vmess://uuid@host:port?…);
// trojan://; ss:// (SIP002 with the user part in base64 or percent-encoded, as Shadowsocks 2022
// requires, and the legacy ss://base64(method:password@host:port)); hysteria2:// and hy2://;
// tuic:// (v5). Any other scheme is unsupported_protocol.
//
// Parameters. Names are case-insensitive; a parameter with an empty value says nothing and is
// skipped (clients write host=&path= freely); a repeated name is bad_link. Every parameter a scheme
// knows is listed by the function of that scheme. Known parameters that do not apply to the chosen
// transport or security (path with grpc, pbk with tls, fp with QUIC protocols) are ignored, as Xray
// does. Ignored everywhere: remarks, remark (names). Ignored on purpose: spx (REALITY spiderX, the
// page the client crawls to look like a browser; the core has no such field and the tunnel does
// not depend on it), outline=1 (Outline's mark on its keys). Any other parameter is bad_link: an
// unknown parameter may change how the server must be reached (fragment, pinSHA256, pqv, ech, mux,
// prefix, …), so the link is refused rather than half-understood.
//
// Dialects:
//   - security=tls without sni takes the Host of ws, httpupgrade or a single-host http (v2rayN and
//     NekoBox do the same for CDN links);
//   - security=reality needs pbk (base64url; the standard alphabet and padding are accepted and
//     rewritten), takes sid (hex, even length, at most 16) and the fingerprint chrome when fp is
//     empty (Xray requires one); on an IP address without sni it is missing_field;
//   - ws early data comes from "?ed=N" inside the path (Xray) or from ed=N, and rides in
//     Sec-WebSocket-Protocol unless eh names another header; "?ed=N" is cut from an httpupgrade
//     path too (an Xray server ignores the query and the core has no early data there);
//   - grpc mode gun and multi are both accepted and the core speaks gun: an Xray server serves both
//     methods (Tun and TunMulti) whatever its own multiMode says; on an IP address with tls and no
//     sni the authority (v2rayN's "host") is the server name; a custom path (serviceName starting
//     with "/") and an authority other than the server name are unsupported_transport;
//   - type tcp and raw are plain TCP; headerType must be none (the http header is not supported);
//     h2 and http are HTTP/2; xhttp, splithttp, kcp and anything else are unsupported_transport;
//   - the name after '#' may hold raw spaces; anywhere else a space is bad_link, and a control
//     character is bad_link everywhere, as is a path with one after percent-decoding, or any text
//     that is not UTF-8 (raw or decoded);
//   - the address may be a name or an IP address, loopback included; 0.0.0.0, :: and multicast
//     are bad_link (pseudo-entries of panels).
func ParseLink(line string) (Server, error) {
	s, r := parseLink(line)
	if r != nil {
		return Server{}, r
	}
	return s, nil
}

func parseLink(line string) (Server, *Rejection) {
	line = strings.TrimSpace(line)
	if len(line) > MaxLinkLen {
		return Server{}, reject(CodeTooLong, "link")
	}
	if line == "" {
		return Server{}, reject(CodeBadLink, "empty")
	}
	if !utf8.ValidString(line) {
		return Server{}, reject(CodeBadLink, "not UTF-8")
	}
	// Spaces are allowed in the name only (some clients write it unescaped); control characters
	// nowhere.
	name := strings.IndexByte(line, '#')
	if name < 0 {
		name = len(line)
	}
	for i := 0; i < len(line); i++ {
		if c := line[i]; c < ' ' || c == 0x7f || (c == ' ' && i < name) {
			return Server{}, reject(CodeBadLink, "space or control character")
		}
	}
	i := strings.Index(line, "://")
	if i <= 0 || !validScheme(line[:i]) {
		return Server{}, reject(CodeBadLink, "not a link")
	}
	scheme, body := strings.ToLower(line[:i]), line[i+3:]
	var s Server
	var r *Rejection
	switch scheme {
	case "vless":
		s, r = parseVLESS(body)
	case "vmess":
		s, r = parseVMess(body)
	case "trojan":
		s, r = parseTrojan(body)
	case "ss":
		s, r = parseShadowsocks(body)
	case "hysteria2", "hy2":
		s, r = parseHysteria2(body)
	case "tuic":
		s, r = parseTUIC(body)
	default:
		return Server{}, reject(CodeUnsupportedProtocol, detail("scheme", scheme))
	}
	if r != nil {
		return Server{}, r
	}
	s.Text = line
	if s.Name == "" {
		s.Name = hostPortName(s.Host, s.Port)
	}
	return s, nil
}

func validScheme(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		letter := c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		if !letter && (i == 0 || !(c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.')) {
			return false
		}
	}
	return s != ""
}

// parts of a link body (after "scheme://"). They are cut by hand, not by net/url: links carry
// unescaped '/', '+' and '=' in the user part and port lists in the address.
type parts struct {
	user     string // raw, before the last '@'
	hasUser  bool
	hostport string
	path     string
	query    string
	fragment string
}

func split(body string) parts {
	var p parts
	body, p.fragment, _ = strings.Cut(body, "#")
	body, p.query, _ = strings.Cut(body, "?")
	if at := strings.LastIndexByte(body, '@'); at >= 0 {
		p.user, p.hasUser, body = body[:at], true, body[at+1:]
	}
	if i := strings.IndexByte(body, '/'); i >= 0 {
		body, p.path = body[:i], body[i:]
	}
	p.hostport = body
	return p
}

// noPath: the address of a link may end with "/" and nothing more.
func noPath(path string) *Rejection {
	if path != "" && path != "/" {
		return reject(CodeBadLink, "path in the address")
	}
	return nil
}

func hostPort(hp string, defaultPort int) (string, int, *Rejection) {
	host, port, r := splitHostPort(hp)
	if r != nil {
		return "", 0, r
	}
	if port == "" {
		if defaultPort > 0 {
			return host, defaultPort, nil
		}
		return "", 0, reject(CodeMissingField, "port")
	}
	n, ok := parsePort(port)
	if !ok {
		return "", 0, reject(CodeBadLink, "port")
	}
	return host, n, nil
}

func linkName(fragment string) string {
	s, err := url.PathUnescape(fragment)
	if err != nil {
		s = fragment
	}
	return cleanName(s)
}

// unescape percent-decodes ('+' stays '+': links carry base64 values) and wants UTF-8 back: the
// value goes into core JSON, which would silently rewrite invalid bytes.
func unescape(s string) (string, bool) {
	d, err := url.PathUnescape(s)
	return d, err == nil && utf8.ValidString(d)
}

func userUUID(p parts) (string, *Rejection) {
	if !p.hasUser || p.user == "" {
		return "", reject(CodeMissingField, "uuid")
	}
	u, ok := unescape(p.user)
	if !ok {
		return "", reject(CodeBadLink, "uuid")
	}
	id, ok := parseUUID(u)
	if !ok {
		return "", reject(CodeBadLink, "uuid")
	}
	return id, nil
}

func userPassword(p parts) (string, *Rejection) {
	if !p.hasUser || p.user == "" {
		return "", reject(CodeMissingField, "password")
	}
	pw, ok := unescape(p.user)
	if !ok {
		return "", reject(CodeBadLink, "password")
	}
	return pw, nil
}

// params are the query parameters of a link; get marks a parameter as understood.
type params struct {
	m    map[string]string
	used map[string]bool
}

func newParams() *params {
	return &params{m: map[string]string{}, used: map[string]bool{}}
}

func parseParams(q string) (*params, *Rejection) {
	p := newParams()
	seen := map[string]bool{}
	for _, kv := range strings.Split(q, "&") {
		if kv == "" {
			continue
		}
		k, v, _ := strings.Cut(kv, "=")
		k, ok1 := unescape(k)
		v, ok2 := unescape(v)
		if !ok1 || !ok2 {
			return nil, reject(CodeBadLink, "percent-encoding")
		}
		k = strings.ToLower(k)
		if seen[k] {
			return nil, reject(CodeBadLink, detail("repeated parameter", k))
		}
		seen[k] = true
		p.set(k, v)
	}
	return p, nil
}

func (p *params) get(k string) string {
	p.used[k] = true
	return p.m[k]
}

func (p *params) set(k, v string) {
	if v != "" {
		p.m[k] = v
	}
}

// harmless parameters, ignored by every scheme.
var harmless = map[string]bool{"remarks": true, "remark": true}

// unknown refuses the first parameter nobody asked for.
func (p *params) unknown() *Rejection {
	var keys []string
	for k := range p.m {
		if !p.used[k] && !harmless[k] {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	return reject(CodeBadLink, detail("unknown parameter", keys[0]))
}

func parseBool(v string) (bool, bool) {
	switch strings.ToLower(v) {
	case "", "0", "false":
		return false, true
	case "1", "true":
		return true, true
	}
	return false, false
}

// insecure: allowInsecure (Xray links), insecure (Hysteria2), allow_insecure (TUIC); any of them.
func insecure(q *params) (bool, *Rejection) {
	on := false
	for _, k := range []string{"allowinsecure", "insecure", "allow_insecure"} {
		b, ok := parseBool(q.get(k))
		if !ok {
			return false, reject(CodeBadLink, k)
		}
		on = on || b
	}
	return on, nil
}

// serverName reads sni, or peer (its old name in Shadowrocket and Clash conversions).
func serverName(q *params) (string, *Rejection) {
	sni, peer := q.get("sni"), q.get("peer")
	if sni == "" {
		sni = peer
	}
	if sni == "" {
		return "", nil
	}
	n, ok := cleanHost(sni)
	if !ok {
		return "", reject(CodeBadLink, "sni")
	}
	return n, nil
}

func parseALPN(s string) ([]string, *Rejection) {
	if s == "" {
		return nil, nil
	}
	var out []string
	for _, a := range strings.Split(s, ",") {
		a = strings.TrimSpace(a)
		if a == "" || len(a) > 64 || !printable(a) {
			return nil, reject(CodeBadLink, "alpn")
		}
		out = append(out, a)
	}
	return out, nil
}

func printable(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] <= ' ' || s[i] >= 0x7f {
			return false
		}
	}
	return true
}

func fingerprint(s string) (string, *Rejection) {
	s = strings.ToLower(s)
	if s == "" || s == "none" {
		return "", nil
	}
	if !fingerprints[s] {
		return "", reject(CodeBadLink, "fp")
	}
	return s, nil
}

func realityKey(s string) (string, bool) {
	s = strings.NewReplacer("+", "-", "/", "_").Replace(strings.TrimRight(s, "="))
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil || len(b) != 32 {
		return "", false
	}
	return base64.RawURLEncoding.EncodeToString(b), true
}

func shortID(s string) bool {
	if len(s) > 16 || len(s)%2 != 0 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !isHex(s[i]) {
			return false
		}
	}
	return true
}

// stream reads the transport and the security of the Xray share-link standard (VLESS, VMess, Trojan).
func stream(q *params, host, defaultSecurity string) (*TLS, *Transport, *Rejection) {
	tr, authority, r := transport(q)
	if r != nil {
		return nil, nil, r
	}
	tls, r := security(q, host, defaultSecurity, tr)
	if r != nil {
		return nil, nil, r
	}
	if tr != nil && tr.Type == TransportQUIC && tls == nil {
		return nil, nil, reject(CodeBadLink, "quic without tls")
	}
	if tr != nil && tr.Type == TransportGRPC && authority != "" {
		a, ok := cleanHost(authority)
		if !ok {
			return nil, nil, reject(CodeBadLink, "authority")
		}
		// v2rayN writes the domain of a gRPC server on an address into "host" (the authority)
		// and leaves sni empty: the authority is then the server name.
		if tls != nil && tls.Reality == nil && tls.ServerName == "" {
			if _, err := netip.ParseAddr(host); err == nil {
				tls.ServerName = a
			}
		}
		name := host
		if tls != nil && tls.ServerName != "" {
			name = tls.ServerName
		}
		if a != name {
			return nil, nil, reject(CodeUnsupportedTransport, "grpc authority")
		}
	}
	return tls, tr, nil
}

const maxEarlyData = 1 << 16

// transport reads type, headerType, host, path, serviceName, mode, authority, ed, eh,
// quicSecurity, key, seed (kcp) and extra (xhttp). It returns the gRPC authority for stream.
func transport(q *params) (*Transport, string, *Rejection) {
	typ := strings.ToLower(q.get("type"))
	header := strings.ToLower(q.get("headertype"))
	host, path := q.get("host"), q.get("path")
	service, mode, authority := q.get("servicename"), strings.ToLower(q.get("mode")), q.get("authority")
	ed, eh := q.get("ed"), q.get("eh")
	quicSecurity := strings.ToLower(q.get("quicsecurity"))
	q.get("key")   // the key of QUIC encryption, which must be none
	q.get("seed")  // kcp, refused below
	q.get("extra") // xhttp, refused below
	if !cleanText(path) {
		return nil, "", reject(CodeBadLink, "path")
	}

	switch typ {
	case "", "tcp", "raw":
		if header != "" && header != "none" {
			return nil, "", reject(CodeUnsupportedTransport, detail("tcp header", header))
		}
		return nil, "", nil
	case "ws":
		h, r := oneHost(host)
		if r != nil {
			return nil, "", r
		}
		p, n, r := earlyDataPath(path)
		if r != nil {
			return nil, "", r
		}
		if ed != "" {
			v, ok := parseUint(ed, maxEarlyData)
			if !ok {
				return nil, "", reject(CodeBadLink, "ed")
			}
			n = v
		}
		t := &Transport{Type: TransportWS, Host: h, Path: p}
		if n > 0 {
			t.MaxEarlyData, t.EarlyDataHeaderName = n, "Sec-WebSocket-Protocol"
			if eh != "" {
				if !headerName(eh) {
					return nil, "", reject(CodeBadLink, "eh")
				}
				t.EarlyDataHeaderName = eh
			}
		}
		return t, "", nil
	case "grpc":
		switch mode {
		case "", "gun", "multi", "none":
		default:
			return nil, "", reject(CodeBadLink, "mode")
		}
		if strings.HasPrefix(service, "/") {
			return nil, "", reject(CodeUnsupportedTransport, "grpc custom path")
		}
		if !printable(service) {
			return nil, "", reject(CodeBadLink, "serviceName")
		}
		return &Transport{Type: TransportGRPC, ServiceName: service}, authority, nil
	case "httpupgrade":
		h, r := oneHost(host)
		if r != nil {
			return nil, "", r
		}
		p, _, r := earlyDataPath(path)
		if r != nil {
			return nil, "", r
		}
		return &Transport{Type: TransportHTTPUpgrade, Host: h, Path: p}, "", nil
	case "http", "h2":
		hs, r := hostList(host)
		if r != nil {
			return nil, "", r
		}
		return &Transport{Type: TransportHTTP, Host: hs, Path: cleanPath(path)}, "", nil
	case "quic":
		if quicSecurity != "" && quicSecurity != "none" {
			return nil, "", reject(CodeUnsupportedTransport, "quic encryption")
		}
		if header != "" && header != "none" {
			return nil, "", reject(CodeUnsupportedTransport, "quic header")
		}
		return &Transport{Type: TransportQUIC}, "", nil
	}
	return nil, "", reject(CodeUnsupportedTransport, detail("transport", typ))
}

// cleanText: valid UTF-8 without control characters (a path goes into an HTTP request line).
func cleanText(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func cleanPath(p string) string {
	if p != "" && p[0] != '/' {
		return "/" + p
	}
	return p
}

// earlyDataPath cuts "ed=N" out of the query of a path ("/ws?ed=2048" is the Xray way to ask for
// early data).
func earlyDataPath(path string) (string, int, *Rejection) {
	path = cleanPath(path)
	base, query, ok := strings.Cut(path, "?")
	if !ok {
		return path, 0, nil
	}
	n := 0
	var keep []string
	for _, kv := range strings.Split(query, "&") {
		if v, isED := strings.CutPrefix(kv, "ed="); isED {
			x, ok := parseUint(v, maxEarlyData)
			if !ok {
				return "", 0, reject(CodeBadLink, "ed")
			}
			n = x
		} else if kv != "" {
			keep = append(keep, kv)
		}
	}
	if len(keep) > 0 {
		base += "?" + strings.Join(keep, "&")
	}
	return base, n, nil
}

func headerName(s string) bool {
	if len(s) > 64 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return s != ""
}

func oneHost(host string) ([]string, *Rejection) {
	if host == "" {
		return nil, nil
	}
	if strings.Contains(host, ",") {
		return nil, reject(CodeBadLink, "host list")
	}
	h, ok := cleanHost(host)
	if !ok {
		return nil, reject(CodeBadLink, "host")
	}
	return []string{h}, nil
}

func hostList(host string) ([]string, *Rejection) {
	if host == "" {
		return nil, nil
	}
	var out []string
	for _, h := range strings.Split(host, ",") {
		c, ok := cleanHost(strings.TrimSpace(h))
		if !ok {
			return nil, reject(CodeBadLink, "host")
		}
		out = append(out, c)
	}
	return out, nil
}

// security reads security, sni, peer, alpn, fp, pbk, sid, spx and the insecure flags.
func security(q *params, host, defaultSecurity string, tr *Transport) (*TLS, *Rejection) {
	sec := strings.ToLower(q.get("security"))
	sni, r := serverName(q)
	if r != nil {
		return nil, r
	}
	alpn, r := parseALPN(q.get("alpn"))
	if r != nil {
		return nil, r
	}
	fp, r := fingerprint(q.get("fp"))
	if r != nil {
		return nil, r
	}
	pbk, sid := q.get("pbk"), q.get("sid")
	q.get("spx")
	skip, r := insecure(q)
	if r != nil {
		return nil, r
	}
	if sec == "" {
		sec = defaultSecurity
	}
	switch sec {
	case "none":
		return nil, nil
	case "tls":
		if sni == "" && tr != nil && len(tr.Host) == 1 {
			sni = tr.Host[0]
		}
		return &TLS{ServerName: sni, Insecure: skip, ALPN: alpn, Fingerprint: fp}, nil
	case "reality":
		if pbk == "" {
			return nil, reject(CodeMissingField, "pbk")
		}
		key, ok := realityKey(pbk)
		if !ok {
			return nil, reject(CodeBadLink, "pbk")
		}
		if !shortID(sid) {
			return nil, reject(CodeBadLink, "sid")
		}
		if sni == "" {
			if _, err := netip.ParseAddr(host); err == nil {
				return nil, reject(CodeMissingField, "sni")
			}
		}
		if fp == "" {
			fp = "chrome"
		}
		return &TLS{ServerName: sni, ALPN: alpn, Fingerprint: fp,
			Reality: &Reality{PublicKey: key, ShortID: strings.ToLower(sid)}}, nil
	}
	return nil, reject(CodeUnsupportedTransport, detail("security", sec))
}

// parseVLESS: vless://uuid@host:port?… with encryption (none only), flow, packetEncoding and the
// parameters of stream.
func parseVLESS(body string) (Server, *Rejection) {
	p := split(body)
	q, r := parseParams(p.query)
	if r != nil {
		return Server{}, r
	}
	id, r := userUUID(p)
	if r != nil {
		return Server{}, r
	}
	host, port, r := hostPort(p.hostport, 0)
	if r != nil {
		return Server{}, r
	}
	if r := noPath(p.path); r != nil {
		return Server{}, r
	}
	if enc := strings.ToLower(q.get("encryption")); enc != "" && enc != "none" {
		return Server{}, reject(CodeUnsupportedProtocol, "vless encryption")
	}
	flow := strings.ToLower(q.get("flow"))
	pe := strings.ToLower(q.get("packetencoding"))
	if pe != "" && !packetEncodings[pe] {
		return Server{}, reject(CodeBadLink, "packetEncoding")
	}
	tls, tr, r := stream(q, host, "none")
	if r != nil {
		return Server{}, r
	}
	switch {
	case flow == "":
	case flows[flow]:
		if tls == nil || tr != nil {
			return Server{}, reject(CodeBadLink, "flow needs tls over plain tcp")
		}
	default:
		return Server{}, reject(CodeUnsupportedProtocol, "flow")
	}
	if r := q.unknown(); r != nil {
		return Server{}, r
	}
	return Server{Name: linkName(p.fragment), Protocol: ProtocolVLESS, Host: host, Port: port, TLS: tls, Transport: tr,
		VLESS: &VLESS{UUID: id, Flow: flow, PacketEncoding: pe}}, nil
}

func vmessSecurity(s string) (string, *Rejection) {
	switch s = strings.ToLower(s); {
	case s == "":
		return "auto", nil
	case vmessSecurities[s]:
		return s, nil
	}
	return "", reject(CodeUnsupportedProtocol, "vmess security")
}

func parseVMess(body string) (Server, *Rejection) {
	head := body
	if i := strings.IndexAny(head, "?#"); i >= 0 {
		head = head[:i]
	}
	if strings.Contains(head, "@") {
		return parseVMessURI(body)
	}
	return parseVMessJSON(body)
}

// parseVMessURI: vmess://uuid@host:port?… (Xray share-link standard) with encryption (the VMess
// security) and the parameters of stream.
func parseVMessURI(body string) (Server, *Rejection) {
	p := split(body)
	q, r := parseParams(p.query)
	if r != nil {
		return Server{}, r
	}
	id, r := userUUID(p)
	if r != nil {
		return Server{}, r
	}
	host, port, r := hostPort(p.hostport, 0)
	if r != nil {
		return Server{}, r
	}
	if r := noPath(p.path); r != nil {
		return Server{}, r
	}
	sec, r := vmessSecurity(q.get("encryption"))
	if r != nil {
		return Server{}, r
	}
	tls, tr, r := stream(q, host, "none")
	if r != nil {
		return Server{}, r
	}
	if r := q.unknown(); r != nil {
		return Server{}, r
	}
	return Server{Name: linkName(p.fragment), Protocol: ProtocolVMess, Host: host, Port: port, TLS: tls, Transport: tr,
		VMess: &VMess{UUID: id, Security: sec}}, nil
}

// vmessFields of the v2rayN JSON. allowInsecure and insecure are not v2rayN's but some panels add them.
var vmessFields = map[string]bool{
	"v": true, "ps": true, "add": true, "port": true, "id": true, "aid": true, "scy": true, "net": true,
	"type": true, "host": true, "path": true, "tls": true, "sni": true, "alpn": true, "fp": true,
	"allowinsecure": true, "insecure": true,
}

// parseVMessJSON: vmess://base64(JSON) of v2rayN, base64 standard or URL, with or without padding.
// Values may be strings or numbers; an unknown field with a value is bad_link.
func parseVMessJSON(body string) (Server, *Rejection) {
	b64, fragment, _ := strings.Cut(body, "#")
	if strings.Contains(b64, "?") {
		return Server{}, reject(CodeBadLink, "parameters after vmess base64")
	}
	raw, ok := decodeBase64(b64)
	if !ok || !utf8.Valid(raw) {
		return Server{}, reject(CodeBadLink, "vmess base64")
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil || m == nil {
		return Server{}, reject(CodeBadLink, "vmess json")
	}
	if _, err := dec.Token(); err != io.EOF {
		return Server{}, reject(CodeBadLink, "vmess json")
	}
	f := map[string]string{}
	var unknown []string
	for k, v := range m {
		var s string
		switch v := v.(type) {
		case string:
			s = v
		case json.Number:
			s = v.String()
		case bool:
			s = strconv.FormatBool(v)
		case nil:
		default:
			return Server{}, reject(CodeBadLink, detail("vmess field", k))
		}
		k, s = strings.ToLower(k), strings.TrimSpace(s)
		if _, dup := f[k]; dup {
			return Server{}, reject(CodeBadLink, detail("repeated vmess field", k))
		}
		f[k] = s
		if s != "" && !vmessFields[k] {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return Server{}, reject(CodeBadLink, detail("unknown vmess field", unknown[0]))
	}
	if v := f["v"]; v != "" && v != "2" {
		return Server{}, reject(CodeBadLink, "vmess version")
	}
	add := strings.TrimSuffix(strings.TrimPrefix(f["add"], "["), "]")
	if add == "" {
		return Server{}, reject(CodeMissingField, "host")
	}
	host, ok := cleanHost(add)
	if !ok {
		return Server{}, reject(CodeBadLink, "host")
	}
	if f["port"] == "" {
		return Server{}, reject(CodeMissingField, "port")
	}
	port, ok := parsePort(f["port"])
	if !ok {
		return Server{}, reject(CodeBadLink, "port")
	}
	if f["id"] == "" {
		return Server{}, reject(CodeMissingField, "uuid")
	}
	id, ok := parseUUID(f["id"])
	if !ok {
		return Server{}, reject(CodeBadLink, "uuid")
	}
	aid := 0
	if a := f["aid"]; a != "" {
		if aid, ok = parseUint(a, 65535); !ok {
			return Server{}, reject(CodeBadLink, "aid")
		}
	}
	sec, r := vmessSecurity(f["scy"])
	if r != nil {
		return Server{}, r
	}

	q := newParams()
	network := strings.ToLower(f["net"])
	q.set("type", network)
	switch network {
	case "grpc": // v2rayN keeps the service name in "path", the mode in "type", the authority in "host"
		q.set("servicename", f["path"])
		q.set("mode", f["type"])
		q.set("authority", f["host"])
	case "quic": // and the QUIC encryption in "host", its key in "path"
		q.set("quicsecurity", f["host"])
		q.set("key", f["path"])
		q.set("headertype", f["type"])
	default:
		q.set("headertype", f["type"])
		q.set("host", f["host"])
		q.set("path", f["path"])
	}
	for _, k := range []string{"sni", "alpn", "fp", "allowinsecure", "insecure"} {
		q.set(k, f[k])
	}
	q.set("security", f["tls"])
	tls, tr, r := stream(q, host, "none")
	if r != nil {
		return Server{}, r
	}
	name := cleanName(f["ps"])
	if name == "" {
		name = linkName(fragment)
	}
	return Server{Name: name, Protocol: ProtocolVMess, Host: host, Port: port, TLS: tls, Transport: tr,
		VMess: &VMess{UUID: id, Security: sec, AlterID: aid}}, nil
}

// parseTrojan: trojan://password@host:port?… with the parameters of stream; TLS unless security says
// otherwise.
func parseTrojan(body string) (Server, *Rejection) {
	p := split(body)
	q, r := parseParams(p.query)
	if r != nil {
		return Server{}, r
	}
	pw, r := userPassword(p)
	if r != nil {
		return Server{}, r
	}
	host, port, r := hostPort(p.hostport, 0)
	if r != nil {
		return Server{}, r
	}
	if r := noPath(p.path); r != nil {
		return Server{}, r
	}
	tls, tr, r := stream(q, host, "tls")
	if r != nil {
		return Server{}, r
	}
	if r := q.unknown(); r != nil {
		return Server{}, r
	}
	return Server{Name: linkName(p.fragment), Protocol: ProtocolTrojan, Host: host, Port: port, TLS: tls, Transport: tr,
		Trojan: &Trojan{Password: pw}}, nil
}

// parseShadowsocks: ss://userinfo@host:port[/]?…#name (SIP002), userinfo being base64(method:password)
// or the percent-encoded method:password, or ss://base64(method:password@host:port)#name. Parameters:
// plugin (refused with the code plugin), outline (ignored).
func parseShadowsocks(body string) (Server, *Rejection) {
	main, fragment, _ := strings.Cut(body, "#")
	main, query, _ := strings.Cut(main, "?")
	q, r := parseParams(query)
	if r != nil {
		return Server{}, r
	}
	if q.get("plugin") != "" {
		return Server{}, reject(CodePlugin, "")
	}
	q.get("outline")
	var user, hp, path string
	plain := false
	if at := strings.LastIndexByte(main, '@'); at >= 0 {
		user, hp = main[:at], main[at+1:]
		if i := strings.IndexByte(hp, '/'); i >= 0 {
			hp, path = hp[:i], hp[i:]
		}
	} else {
		raw, ok := decodeBase64(strings.TrimSuffix(main, "/"))
		if !ok || !utf8.Valid(raw) {
			return Server{}, reject(CodeBadLink, "shadowsocks base64")
		}
		s := string(raw)
		at := strings.LastIndexByte(s, '@')
		if at < 0 {
			return Server{}, reject(CodeBadLink, "shadowsocks address")
		}
		user, hp, plain = s[:at], s[at+1:], true
	}
	method, password, r := ssUser(user, plain)
	if r != nil {
		return Server{}, r
	}
	host, port, r := hostPort(hp, 0)
	if r != nil {
		return Server{}, r
	}
	if r := noPath(path); r != nil {
		return Server{}, r
	}
	size, ok := ssMethods[method]
	if !ok {
		return Server{}, reject(CodeUnsupportedProtocol, "shadowsocks method")
	}
	if size > 0 {
		for _, k := range strings.Split(password, ":") {
			b, err := base64.StdEncoding.DecodeString(k)
			if err != nil || len(b) != size {
				return Server{}, reject(CodeBadLink, "shadowsocks 2022 key")
			}
		}
	}
	if r := q.unknown(); r != nil {
		return Server{}, r
	}
	return Server{Name: linkName(fragment), Protocol: ProtocolShadowsocks, Host: host, Port: port,
		Shadowsocks: &Shadowsocks{Method: method, Password: password}}, nil
}

func ssUser(u string, plain bool) (string, string, *Rejection) {
	if u == "" {
		return "", "", reject(CodeMissingField, "method")
	}
	s := u
	if !plain {
		if strings.Contains(s, "%") {
			d, ok := unescape(s)
			if !ok {
				return "", "", reject(CodeBadLink, "userinfo")
			}
			s = d
		}
		if !strings.Contains(s, ":") { // base64 never holds ':'
			raw, ok := decodeBase64(s)
			if !ok || !utf8.Valid(raw) {
				return "", "", reject(CodeBadLink, "userinfo")
			}
			s = string(raw)
		}
	}
	method, password, _ := strings.Cut(s, ":")
	if method == "" {
		return "", "", reject(CodeMissingField, "method")
	}
	if password == "" {
		return "", "", reject(CodeMissingField, "password")
	}
	return strings.ToLower(method), password, nil
}

// parseHysteria2: hysteria2://auth@host[:port][/]?… where the port may be a list ("443,5000-6000",
// the official URI) and defaults to 443. Parameters: sni (peer), insecure, alpn, obfs (salamander),
// obfs-password, mport (v2rayN, NekoBox: hopping ranges), upmbps, downmbps; fp is ignored (no uTLS
// over QUIC). pinSHA256 is not supported and so refused as unknown.
func parseHysteria2(body string) (Server, *Rejection) {
	p := split(body)
	q, r := parseParams(p.query)
	if r != nil {
		return Server{}, r
	}
	pw, r := userPassword(p)
	if r != nil {
		return Server{}, r
	}
	host, spec, r := splitHostPort(p.hostport)
	if r != nil {
		return Server{}, r
	}
	port := 443
	var ports []PortRange
	if spec != "" {
		if n, ok := parsePort(spec); ok {
			port = n
		} else {
			if ports, ok = parsePortRanges(spec); !ok {
				return Server{}, reject(CodeBadLink, "port")
			}
			port = ports[0].From
		}
	}
	if mp := q.get("mport"); mp != "" {
		if ports != nil {
			return Server{}, reject(CodeBadLink, "mport and a port list")
		}
		var ok bool
		if ports, ok = parsePortRanges(mp); !ok {
			return Server{}, reject(CodeBadLink, "mport")
		}
	}
	if r := noPath(p.path); r != nil {
		return Server{}, r
	}
	tls, r := quicTLS(q)
	if r != nil {
		return Server{}, r
	}
	obfs, obfsPw := strings.ToLower(q.get("obfs")), q.get("obfs-password")
	switch obfs {
	case "", "none":
		obfs = ""
		if obfsPw != "" {
			return Server{}, reject(CodeBadLink, "obfs-password without obfs")
		}
	case "salamander":
		if obfsPw == "" {
			return Server{}, reject(CodeMissingField, "obfs-password")
		}
	default:
		return Server{}, reject(CodeBadLink, "obfs")
	}
	up, ok1 := mbps(q.get("upmbps"))
	down, ok2 := mbps(q.get("downmbps"))
	if !ok1 || !ok2 {
		return Server{}, reject(CodeBadLink, "bandwidth")
	}
	if r := q.unknown(); r != nil {
		return Server{}, r
	}
	return Server{Name: linkName(p.fragment), Protocol: ProtocolHysteria2, Host: host, Port: port, TLS: tls,
		Hysteria2: &Hysteria2{Password: pw, ServerPorts: ports, Obfs: obfs, ObfsPassword: obfsPw, UpMbps: up, DownMbps: down}}, nil
}

// quicTLS reads the TLS of Hysteria2 and TUIC: sni (peer), the insecure flags, alpn; fp is ignored.
func quicTLS(q *params) (*TLS, *Rejection) {
	sni, r := serverName(q)
	if r != nil {
		return nil, r
	}
	skip, r := insecure(q)
	if r != nil {
		return nil, r
	}
	alpn, r := parseALPN(q.get("alpn"))
	if r != nil {
		return nil, r
	}
	q.get("fp")
	return &TLS{ServerName: sni, Insecure: skip, ALPN: alpn}, nil
}

func mbps(s string) (int, bool) {
	if s == "" {
		return 0, true
	}
	return parseUint(s, 100000)
}

// parsePortRanges reads "443", "5000-6000" and comma lists of them (at most 16).
func parsePortRanges(s string) ([]PortRange, bool) {
	items := strings.Split(s, ",")
	if len(items) > 16 {
		return nil, false
	}
	var out []PortRange
	for _, it := range items {
		a, b, isRange := strings.Cut(strings.TrimSpace(it), "-")
		from, ok := parsePort(a)
		if !ok {
			return nil, false
		}
		to := from
		if isRange {
			if to, ok = parsePort(b); !ok || to < from {
				return nil, false
			}
		}
		out = append(out, PortRange{From: from, To: to})
	}
	return out, true
}

// parseTUIC: tuic://uuid:password@host:port?… (v5). Parameters: congestion_control, udp_relay_mode,
// sni (peer), alpn, allow_insecure (allowInsecure, insecure), disable_sni (0 only: the model always
// sends SNI), reduce_rtt and zero_rtt_handshake (0-RTT); fp is ignored.
func parseTUIC(body string) (Server, *Rejection) {
	p := split(body)
	q, r := parseParams(p.query)
	if r != nil {
		return Server{}, r
	}
	if !p.hasUser || p.user == "" {
		return Server{}, reject(CodeMissingField, "uuid")
	}
	rawID, rawPw, _ := strings.Cut(p.user, ":")
	u, ok := unescape(rawID)
	if !ok {
		return Server{}, reject(CodeBadLink, "uuid")
	}
	id, ok := parseUUID(u)
	if !ok {
		return Server{}, reject(CodeBadLink, "uuid")
	}
	pw, ok := unescape(rawPw)
	if !ok {
		return Server{}, reject(CodeBadLink, "password")
	}
	if pw == "" {
		return Server{}, reject(CodeMissingField, "password")
	}
	host, port, r := hostPort(p.hostport, 0)
	if r != nil {
		return Server{}, r
	}
	if r := noPath(p.path); r != nil {
		return Server{}, r
	}
	cc := strings.ToLower(q.get("congestion_control"))
	if cc != "" && !congestionControls[cc] {
		return Server{}, reject(CodeBadLink, "congestion_control")
	}
	mode := strings.ToLower(q.get("udp_relay_mode"))
	if mode != "" && !udpRelayModes[mode] {
		return Server{}, reject(CodeBadLink, "udp_relay_mode")
	}
	if off, ok := parseBool(q.get("disable_sni")); !ok || off {
		return Server{}, reject(CodeBadLink, "disable_sni")
	}
	zero := false
	for _, k := range []string{"reduce_rtt", "zero_rtt_handshake"} {
		b, ok := parseBool(q.get(k))
		if !ok {
			return Server{}, reject(CodeBadLink, k)
		}
		zero = zero || b
	}
	tls, r := quicTLS(q)
	if r != nil {
		return Server{}, r
	}
	if r := q.unknown(); r != nil {
		return Server{}, r
	}
	return Server{Name: linkName(p.fragment), Protocol: ProtocolTUIC, Host: host, Port: port, TLS: tls,
		TUIC: &TUIC{UUID: id, Password: pw, CongestionControl: cc, UDPRelayMode: mode, ZeroRTT: zero}}, nil
}

// decodeBase64 reads base64 in the standard or the URL alphabet, with or without padding.
func decodeBase64(s string) ([]byte, bool) {
	s = strings.TrimRight(s, "=")
	if s == "" {
		return nil, false
	}
	enc := base64.RawStdEncoding
	if strings.ContainsAny(s, "-_") {
		if strings.ContainsAny(s, "+/") {
			return nil, false
		}
		enc = base64.RawURLEncoding
	}
	b, err := enc.DecodeString(s)
	return b, err == nil
}
