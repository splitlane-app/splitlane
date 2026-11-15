package connector

import (
	"errors"
	"net/url"
	"strings"
	"unicode/utf8"
)

// ParseText reads the text of proxy.add. It is one of:
//   - a WireGuard .conf, when a line is "[Interface]": one server or one rejection;
//   - a subscription address: a single https:// line (with nothing else but blank and comment
//     lines), returned as subscriptionURL for the caller to fetch;
//   - share links, one per line.
//
// Blank lines and lines starting with '#' are skipped (comments, and the "#profile-title:" headers
// some subscriptions carry). A link that repeats an earlier one counts once. A rejection carries the
// 1-based line number; an https:// line among links is bad_link (an address goes alone).
func ParseText(text string) (servers []Server, rejected []Rejection, subscriptionURL string) {
	if len(text) > MaxTextLen {
		return nil, []Rejection{{Code: CodeTooLong, Detail: "text"}}, ""
	}
	text = strings.TrimPrefix(text, bom)
	lines := splitLines(text)
	for _, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l), "[interface]") {
			s, err := ParseWireGuardConf([]byte(text), "")
			if err != nil {
				var r *Rejection
				errors.As(err, &r)
				return nil, []Rejection{*r}, ""
			}
			return []Server{s}, nil, ""
		}
	}
	only, count := 0, 0
	for i, l := range lines {
		if l = strings.TrimSpace(l); l != "" && l[0] != '#' {
			only, count = i, count+1
		}
	}
	if count == 1 && isHTTPS(strings.TrimSpace(lines[only])) {
		u := strings.TrimSpace(lines[only])
		if !validSubscriptionURL(u) {
			return nil, []Rejection{{Line: only + 1, Code: CodeBadLink, Detail: "subscription address"}}, ""
		}
		return nil, nil, u
	}
	servers, rejected = parseLines(lines, true)
	return servers, rejected, ""
}

// ParseSubscription reads a subscription response: links one per line, either as plain text or the
// whole list in base64 (standard or URL alphabet, with or without padding, wrapped or not). Blank
// and '#' lines are skipped; a link that repeats an earlier one counts once; line numbers are those
// of the decoded list. A body with no link at all (Clash YAML, sing-box JSON, a .conf) is one
// unsupported_protocol rejection with line 0.
func ParseSubscription(body []byte) ([]Server, []Rejection) {
	if len(body) > MaxSubscriptionLen {
		return nil, []Rejection{{Code: CodeTooLong, Detail: "subscription"}}
	}
	text := strings.TrimPrefix(string(body), bom)
	if decoded, ok := decodeSubscription(text); ok {
		text = decoded
	}
	if !utf8.ValidString(text) {
		return nil, []Rejection{{Code: CodeBadLink, Detail: "not UTF-8"}}
	}
	lines := splitLines(text)
	content, links := false, false
	for _, l := range lines {
		if l = strings.TrimSpace(l); l == "" || l[0] == '#' {
			continue
		}
		content = true
		if strings.Contains(l, "://") {
			links = true
			break
		}
	}
	if content && !links {
		return nil, []Rejection{{Code: CodeUnsupportedProtocol, Detail: "not a list of links"}}
	}
	return parseLines(lines, false)
}

// parseLines parses one link per line. pasted: an https:// line is a misplaced subscription address
// rather than an unsupported link.
func parseLines(lines []string, pasted bool) ([]Server, []Rejection) {
	var servers []Server
	var rejected []Rejection
	seen := map[string]bool{}
	for i, l := range lines {
		if l = strings.TrimSpace(l); l == "" || l[0] == '#' {
			continue
		}
		if pasted && isHTTPS(l) {
			rejected = append(rejected, Rejection{Line: i + 1, Code: CodeBadLink, Detail: "a subscription address goes alone"})
			continue
		}
		s, r := parseLink(l)
		if r != nil {
			r.Line = i + 1
			rejected = append(rejected, *r)
			continue
		}
		if !seen[s.ID] {
			seen[s.ID] = true
			servers = append(servers, s)
		}
	}
	return servers, rejected
}

func isHTTPS(l string) bool {
	return len(l) >= 8 && strings.EqualFold(l[:8], "https://")
}

func validSubscriptionURL(u string) bool {
	if len(u) > 2048 {
		return false
	}
	for i := 0; i < len(u); i++ {
		if u[i] <= ' ' || u[i] == 0x7f {
			return false
		}
	}
	p, err := url.Parse(u)
	if err != nil || !strings.EqualFold(p.Scheme, "https") {
		return false
	}
	_, ok := cleanHost(p.Hostname())
	return ok
}

// decodeSubscription decodes a body that is base64 and nothing else (line breaks and spaces aside).
func decodeSubscription(text string) (string, bool) {
	s := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			return -1
		}
		return r
	}, text)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '+' || c == '/' || c == '-' || c == '_' || c == '=') {
			return "", false
		}
	}
	b, ok := decodeBase64(s)
	if !ok || !utf8.Valid(b) {
		return "", false
	}
	return string(b), true
}
