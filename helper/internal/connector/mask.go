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
	// Secret fields of core JSON (sing-box outbounds and endpoints) and of servers.json.
	maskJSON = regexp.MustCompile(`(?i)("(?:password|uuid|private_key|public_key|pre_shared_key|short_id|auth|auth_str|token|key|psk|url|text)"\s*:\s*")(?:[^"\\]|\\.)*"`)
	// The user part of any URL: a UUID, a password, uuid:password, base64 of method:password.
	maskUser = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://)[^\s@"'<>]*@`)
	// The base64 body of vmess:// (JSON) and of a legacy ss://.
	maskBlob = regexp.MustCompile(`(?i)\b((?:vmess|ss)://)[A-Za-z0-9+/=_-]+`)
	// Secret query values of links and subscription addresses. path and serviceName are on the
	// list because servers use secret paths as a second password.
	maskQuery = regexp.MustCompile(`(?i)([?&;](?:password|passwd|pass|pwd|obfs-password|auth|token|access_token|key|api_key|apikey|pbk|sid|uuid|psk|secret|path|servicename)=)[^&\s#"'<>]*`)
	// http(s) addresses, for tokens written as a path segment.
	maskURL  = regexp.MustCompile(`(?i)\bhttps?://[^\s"'<>]+`)
	maskUUID = regexp.MustCompile(`(?i)[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	// A bare 32-byte key in standard base64 (WireGuard, Shadowsocks 2022).
	maskKey = regexp.MustCompile(`[A-Za-z0-9+/]{43}=`)
)

// Mask hides the secrets of servers and subscriptions in a text bound for a log, an error or the
// diagnostics archive: UUIDs, the user part of links and URLs (passwords of Trojan, Shadowsocks,
// Hysteria2, TUIC; userinfo of subscription addresses), vmess:// and legacy ss:// bodies, secret
// query values (pbk, sid, obfs-password, token, key, path, …), WireGuard and Shadowsocks 2022 keys,
// secret fields of core JSON, and path segments of http(s) addresses that look like tokens (16 or
// more letters, digits, '-', '_' or '=' with both letters and digits). Each becomes "•••". Host
// names, ports and other parameters stay, so the log still says which server failed.
func Mask(s string) string {
	s = maskConf.ReplaceAllString(s, "${1}"+masked)
	s = maskJSON.ReplaceAllString(s, `${1}`+masked+`"`)
	s = maskUser.ReplaceAllString(s, "${1}"+masked+"@")
	s = maskBlob.ReplaceAllString(s, "${1}"+masked)
	s = maskQuery.ReplaceAllString(s, "${1}"+masked)
	s = maskURL.ReplaceAllStringFunc(s, maskPath)
	s = maskUUID.ReplaceAllString(s, masked)
	return maskKey.ReplaceAllString(s, masked)
}

// maskPath masks the token-like segments of the path of an http(s) address.
func maskPath(u string) string {
	i := strings.Index(u, "://") + 3
	slash := strings.IndexByte(u[i:], '/')
	if slash < 0 {
		return u
	}
	head, path, tail := u[:i+slash], u[i+slash:], ""
	if j := strings.IndexAny(path, "?#"); j >= 0 {
		path, tail = path[:j], path[j:]
	}
	segs := strings.Split(path, "/")
	for k, seg := range segs {
		if tokenLike(seg) {
			segs[k] = masked
		}
	}
	return head + strings.Join(segs, "/") + tail
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
