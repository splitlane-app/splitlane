package serversetup

import (
	"bytes"
	"encoding/json"
	"net/netip"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Limits of a request. The line is what the server-setup subcommand reads from stdin; the window
// reads a key file of at most 16 KB (spec, section 3).
const (
	maxRequestLen = 64 << 10
	maxKeyLen     = 16 << 10
	defaultPort   = 22
)

// Request is a setup the window asks for: the server, the user and the credentials. The credentials
// are byte slices and never strings here, so Wipe can overwrite them when the setup is over.
type Request struct {
	Host string // an IP address (an IPv6 one without brackets) or a host name
	Port int
	User string

	Password   []byte // the login password, also the password of sudo; or
	Key        []byte // the text of an OpenSSH or PEM private key
	Passphrase []byte // the passphrase of Key, when it has one
}

// requestFields are the fields of a request line. Any other is refused: a field the window misnames
// is a bad_input, not a setting quietly dropped.
var requestFields = map[string]bool{"host": true, "port": true, "user": true, "password": true, "key": true, "passphrase": true}

// ParseRequest reads the request line of the window: a JSON object with host, port, user, and
// password or key with passphrase. host is an IPv4 address, an IPv6 address with or without
// brackets, or a host name (RFC 1123), spaces around it allowed; it has no port (Ruling 1: the window
// splits "host:port" as people type it and sends the port apart). port is 1..65535, 22 when it is
// absent or null. user is 1..32 characters of [a-z_][a-z0-9_-]*. Exactly one of password and key; a
// passphrase only with a key; an empty string counts as absent. Anything else is bad_input, whose
// detail names the field ("request" for the line as a whole), never a value.
//
// The secrets are copied out of line into fresh byte slices; the caller wipes line, the request's
// copies go with Wipe. They are unquoted by hand: a JSON string decoded into a Go string would leave
// a copy nothing can wipe.
func ParseRequest(line []byte) (Request, *Failure) {
	if len(line) > maxRequestLen || !json.Valid(line) {
		return Request{}, badInput("request")
	}
	// The names are checked before any value is copied. A name given twice is refused (Ruling 14):
	// json.Unmarshal would keep the last value and drop its copy of the first, which then could not
	// be wiped.
	names, ok := objectNames(line)
	if !ok {
		return Request{}, badInput("request")
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if !requestFields[n] || seen[n] {
			return Request{}, badInput("request")
		}
		seen[n] = true
	}
	// The values stay raw JSON, copies of line that are cleared on the way out.
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(line, &fields); err != nil || fields == nil {
		return Request{}, badInput("request")
	}
	defer func() {
		for _, v := range fields {
			clear(v)
		}
	}()

	var r Request
	var host string
	host, ok = textField(fields["host"])
	if !ok {
		return Request{}, badInput("host")
	}
	if r.Host, ok = parseHost(host); !ok {
		return Request{}, badInput("host")
	}
	r.Port = defaultPort
	if raw := fields["port"]; raw != nil && string(raw) != "null" {
		var port int
		if err := json.Unmarshal(raw, &port); err != nil || port < 1 || port > 65535 {
			return Request{}, badInput("port")
		}
		r.Port = port
	}
	user, ok := textField(fields["user"])
	if user = strings.TrimSpace(user); !ok || !validUser(user) {
		return Request{}, badInput("user")
	}
	r.User = user

	// The secrets come last: from here on a refusal wipes what was read.
	refuse := func(field string) (Request, *Failure) {
		r.Wipe()
		return Request{}, badInput(field)
	}
	for _, s := range []struct {
		name string
		dst  *[]byte
	}{{"password", &r.Password}, {"key", &r.Key}, {"passphrase", &r.Passphrase}} {
		if *s.dst, ok = secretField(fields[s.name]); !ok {
			return refuse(s.name)
		}
	}
	switch {
	case (len(r.Password) > 0) == (len(r.Key) > 0), len(r.Passphrase) > 0 && len(r.Key) == 0:
		return refuse("credentials")
	case bytes.ContainsAny(r.Password, "\x00\r\n"):
		// sudo -S reads the password up to the first newline: the rest would be the kit's stdin. A
		// password typed in the form has none of them, and PAM cuts a C string at NUL.
		return refuse("password")
	case len(r.Key) > maxKeyLen:
		return refuse("key")
	}
	return r, nil
}

// Wipe overwrites the credentials with zeros and drops them. The copies x/crypto/ssh makes (it takes
// a password as a string) are out of reach: the garbage collector frees them.
func (r *Request) Wipe() {
	clear(r.Password)
	clear(r.Key)
	clear(r.Passphrase)
	r.Password, r.Key, r.Passphrase = nil, nil, nil
}

func badInput(field string) *Failure { return &Failure{Code: CodeBadInput, Detail: field} }

// objectNames returns the member names of the JSON object b, in order, nested objects not
// included; false when b is not an object. b must be valid JSON (json.Valid). The values are only
// stepped over, never copied: the request's secrets are among them.
func objectNames(b []byte) ([]string, bool) {
	i := skipSpace(b, 0)
	if i >= len(b) || b[i] != '{' {
		return nil, false
	}
	var names []string
	for i = skipSpace(b, i+1); i < len(b) && b[i] != '}'; {
		end := endOfString(b, i)
		var name string // decoded, so an escaped spelling of a name is that name
		if b[i] != '"' || json.Unmarshal(b[i:end], &name) != nil {
			return nil, false
		}
		names = append(names, name)
		i = skipSpace(b, end)
		if i >= len(b) || b[i] != ':' {
			return nil, false
		}
		i = skipSpace(b, endOfValue(b, skipSpace(b, i+1)))
		if i < len(b) && b[i] == ',' {
			i = skipSpace(b, i+1)
		}
	}
	return names, i < len(b)
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\t' || b[i] == '\n' || b[i] == '\r') {
		i++
	}
	return i
}

// endOfString is the index after the string that starts with the quote at i.
func endOfString(b []byte, i int) int {
	for i++; i < len(b); i++ {
		switch b[i] {
		case '\\':
			i++
		case '"':
			return i + 1
		}
	}
	return len(b)
}

// endOfValue is the index after the JSON value that starts at i.
func endOfValue(b []byte, i int) int {
	if i >= len(b) {
		return i
	}
	switch b[i] {
	case '"':
		return endOfString(b, i)
	case '{', '[':
		depth := 0
		for ; i < len(b); i++ {
			switch b[i] {
			case '"':
				i = endOfString(b, i) - 1
			case '{', '[':
				depth++
			case '}', ']':
				if depth--; depth == 0 {
					return i + 1
				}
			}
		}
		return len(b)
	}
	for i < len(b) && !bytes.ContainsRune([]byte(",}] \t\r\n"), rune(b[i])) { // a number, true, false, null
		i++
	}
	return i
}

// textField reads a JSON string that is not a secret; a missing field or null is not one.
func textField(raw json.RawMessage) (string, bool) {
	if raw == nil || string(raw) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return "", false
	}
	return s, true
}

// secretField unquotes a JSON string into a fresh byte slice; a missing field, null and "" are nil.
func secretField(raw json.RawMessage) ([]byte, bool) {
	if raw == nil || string(raw) == "null" {
		return nil, true
	}
	b, ok := unquote(raw)
	if !ok {
		return nil, false
	}
	if len(b) == 0 {
		return nil, true
	}
	return b, true
}

// unquote decodes a JSON string literal, escapes and surrogate pairs included, into bytes. The result
// must be UTF-8; a lone surrogate is refused rather than replaced, so the bytes are the ones typed.
func unquote(raw []byte) ([]byte, bool) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, false
	}
	s := raw[1 : len(raw)-1]
	out := make([]byte, 0, len(s))
	fail := func() ([]byte, bool) {
		clear(out[:cap(out)])
		return nil, false
	}
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == '"' || c < 0x20:
			return fail()
		case c != '\\':
			out = append(out, c)
			i++
			continue
		}
		if i+1 >= len(s) {
			return fail()
		}
		switch e := s[i+1]; e {
		case '"', '\\', '/':
			out = append(out, e)
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'u':
			r, ok := hex4(s[i+2:])
			if !ok {
				return fail()
			}
			i += 6
			if utf16.IsSurrogate(r) {
				// A high surrogate and a low one make one character; anything else is not text.
				if i+1 >= len(s) || s[i] != '\\' || s[i+1] != 'u' {
					return fail()
				}
				low, ok := hex4(s[i+2:])
				if r = utf16.DecodeRune(r, low); !ok || r == utf8.RuneError {
					return fail()
				}
				i += 6
			}
			out = utf8.AppendRune(out, r)
			continue
		default:
			return fail()
		}
		i += 2
	}
	if !utf8.Valid(out) {
		return fail()
	}
	return out, true
}

// hex4 reads the four hex digits of a u-escape.
func hex4(s []byte) (rune, bool) {
	if len(s) < 4 {
		return 0, false
	}
	var r rune
	for _, c := range s[:4] {
		var d byte
		switch {
		case c >= '0' && c <= '9':
			d = c - '0'
		case c >= 'a' && c <= 'f':
			d = c - 'a' + 10
		case c >= 'A' && c <= 'F':
			d = c - 'A' + 10
		default:
			return 0, false
		}
		r = r<<4 | rune(d)
	}
	return r, true
}

// parseHost checks the host of a request and returns it without spaces around and without the
// brackets of an IPv6 address. A port, a scheme, a path or a zone is refused.
func parseHost(s string) (string, bool) {
	s = strings.TrimSpace(s)
	if inner, ok := strings.CutPrefix(s, "["); ok {
		inner, ok = strings.CutSuffix(inner, "]")
		a, err := netip.ParseAddr(inner)
		if !ok || err != nil || !a.Is6() || a.Zone() != "" {
			return "", false
		}
		return inner, true
	}
	if a, err := netip.ParseAddr(s); err == nil {
		return s, a.Zone() == ""
	}
	return s, validHostName(s)
}

// validHostName: RFC 1123 labels of letters, digits and '-' (not at either end), 1..63 characters,
// 253 in all, no trailing dot. The last label is not all digits (RFC 1123, 2.1), so a mistyped IPv4
// address such as 198.51.100.300 is not taken for a name.
func validHostName(s string) bool {
	if s == "" || len(s) > 253 {
		return false
	}
	labels := strings.Split(s, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return false
		}
		for i := 0; i < len(l); i++ {
			if c := l[i]; !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return strings.Trim(labels[len(labels)-1], "0123456789") != ""
}

// validUser: 1..32 characters, [a-z_][a-z0-9_-]* (the useradd default of Debian and Ubuntu). The user
// goes into no command line, but a strict check catches a field filled with something else.
func validUser(s string) bool {
	if s == "" || len(s) > 32 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z' || c == '_':
		case i > 0 && (c >= '0' && c <= '9' || c == '-'):
		default:
			return false
		}
	}
	return true
}
