package connector

import (
	"regexp"
	"strings"
)

// masked replaces a secret.
const masked = "•••"

var (
	// WireGuard keys in a .conf: PrivateKey = …, PublicKey = …, PresharedKey = ….
	maskConf = regexp.MustCompile(`(?im)^(\s*(?:privatekey|publickey|presharedkey)\s*=\s*)\S+`)
	// Secret fields of core JSON (sing-box outbounds, endpoints, transports) and of servers.json.
	maskJSON = regexp.MustCompile(`(?i)("(?:password|uuid|private_key|public_key|pre_shared_key|short_id|auth|auth_str|token|key|psk|url|text|path|service_name)"\s*:\s*")(?:[^"\\]|\\.)*"`)
	// The user part of any URL: a UUID, a password, uuid:password, base64 of method:password.
	maskUser = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^\s@"'<>]*@`)
	// A link whose body is base64 (vmess:// JSON, legacy ss://, Shadowrocket vless:// and vmess://,
	// ssr://): 16 or more base64 characters right after scheme://, up to '?', '#', a space or the end.
	maskBlob = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[A-Za-z0-9+/=_-]{16,}([?#\s"'<>]|$)`)
	// Secret query values of links. path and serviceName are on the list because servers use
	// secret paths as a second password.
	maskQuery = regexp.MustCompile(`(?i)([?&;](?:password|passwd|pass|pwd|obfs-password|auth|token|access_token|key|api_key|apikey|pbk|sid|uuid|psk|secret|path|servicename)=)[^&\s#"'<>]*`)
	// http(s) addresses: their query values and token-like path segments.
	maskURL  = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>]+`)
	maskUUID = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
)

// keepQuery: the query parameters of http(s) addresses that stay readable (the format a client asks
// a subscription for). Every other value is masked: subscription addresses carry their tokens under
// any name.
var keepQuery = map[string]bool{"format": true, "flag": true, "target": true}

// Mask hides the secrets of servers and subscriptions in a text bound for a log, an error or the
// diagnostics archive: UUIDs, the user part of links and URLs (passwords of Trojan, Shadowsocks,
// Hysteria2, TUIC; userinfo of subscription addresses), base64 link bodies, secret query values of
// links (pbk, sid, obfs-password, path, …), every query value of an http(s) address but format,
// flag and target, path segments of http(s) addresses that look like tokens (16 or more letters,
// digits, '-', '_' or '=' with both letters and digits), WireGuard and Shadowsocks keys (a run of
// standard base64 of 22 characters or more ending in padding, masked whole), and secret fields of
// core JSON. Each becomes "•••". Host names, ports and other parameters stay, so a log still says
// which server failed. Mask(Mask(s)) == Mask(s): the rules are applied until nothing changes.
func Mask(s string) string {
	for range 8 {
		next := maskOnce(s)
		if next == s {
			break
		}
		s = next
	}
	return s
}

func maskOnce(s string) string {
	s = maskConf.ReplaceAllString(s, "${1}"+masked)
	s = maskJSON.ReplaceAllString(s, `${1}`+masked+`"`)
	s = maskUser.ReplaceAllString(s, "${1}"+masked+"@")
	s = maskBlob.ReplaceAllString(s, "${1}"+masked+"${2}")
	s = maskQuery.ReplaceAllString(s, "${1}"+masked)
	s = maskURL.ReplaceAllStringFunc(s, maskAddress)
	s = maskUUID.ReplaceAllString(s, masked)
	return maskKeys(s)
}

// maskAddress masks the query values (but keepQuery) and the token-like path segments of an http(s)
// address.
func maskAddress(u string) string {
	i := strings.Index(u, "://") + 3
	end := len(u)
	if j := strings.IndexAny(u[i:], "/?#"); j >= 0 {
		end = i + j
	}
	head, rest := u[:end], u[end:]
	rest, fragment, hasFragment := strings.Cut(rest, "#")
	path, query, hasQuery := strings.Cut(rest, "?")
	segs := strings.Split(path, "/")
	for k, seg := range segs {
		if tokenLike(seg) {
			segs[k] = masked
		}
	}
	out := head + strings.Join(segs, "/")
	if hasQuery {
		items := strings.Split(query, "&")
		for k, it := range items {
			key, value, hasValue := strings.Cut(it, "=")
			switch {
			case keepQuery[strings.ToLower(key)]:
			case hasValue && value != "":
				items[k] = key + "=" + masked
			case !hasValue && it != "":
				items[k] = masked // a bare token
			}
		}
		out += "?" + strings.Join(items, "&")
	}
	if hasFragment {
		out += "#" + fragment
	}
	return out
}

func tokenLike(s string) bool {
	if len(s) < 16 {
		return false
	}
	letter, digit := false, false
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			letter = true
		case c >= '0' && c <= '9':
			digit = true
		case c == '-' || c == '_' || c == '=':
		default:
			return false
		}
	}
	return letter && digit
}

// maskKeys masks every whole run of standard base64 characters that is 22 characters or longer
// and ends in padding: a 16-byte key ("…==", Shadowsocks 2022 AES-128) or a 32-byte one ("…=",
// WireGuard, Shadowsocks 2022), and longer runs whole, so no prefix of them leaks.
func maskKeys(s string) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); {
		if !isBase64(s[i]) {
			i++
			continue
		}
		j := i
		for j < len(s) && isBase64(s[j]) {
			j++
		}
		k := j
		for k < len(s) && k-j < 2 && s[k] == '=' {
			k++
		}
		if k > j && j-i >= 22 {
			b.WriteString(s[last:i])
			b.WriteString(masked)
			last = k
		}
		i = k
	}
	if last == 0 {
		return s
	}
	b.WriteString(s[last:])
	return b.String()
}

func isBase64(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '/'
}
