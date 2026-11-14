package rules

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"splitlane/helper/internal/protocol"
)

// Stub keeps a rule file from ever being empty (spec 6.5).
const Stub = `^:splitlane-stub$`

// Check validates a whole layout (spec 8.2, 8.3 step 1). It returns the normalized
// rules, or the rejected ones, or an error for malformed input (bad_request).
// Nothing is returned as normalized when at least one rule is rejected.
func Check(apps []protocol.AppRule, env Env) ([]protocol.AppRule, []protocol.RuleRejection, error) {
	out := make([]protocol.AppRule, 0, len(apps))
	for _, a := range apps {
		n, err := Normalize(a, env)
		if err != nil {
			return nil, nil, err
		}
		out = append(out, n)
	}
	var rejected []protocol.RuleRejection
	reject := map[string]string{}
	for _, a := range out {
		switch {
		case tooBroad(a, env):
			reject[a.ID] = protocol.RejectTooBroad
		case protects(a, env):
			reject[a.ID] = protocol.RejectProtected
		}
	}
	for i, a := range out {
		for _, b := range out[i+1:] {
			if a.Lane != b.Lane && sameScope(a, b, env) {
				for _, id := range []string{a.ID, b.ID} {
					if reject[id] == "" {
						reject[id] = protocol.RejectDuplicate
					}
				}
			}
		}
	}
	for _, a := range out {
		if r := reject[a.ID]; r != "" {
			rejected = append(rejected, protocol.RuleRejection{ID: a.ID, Reason: r})
		}
	}
	if len(rejected) > 0 {
		return nil, rejected, nil
	}
	return out, nil, nil
}

// tooBroad: a rule may not cover a drive root, the Windows folder or a Program Files folder entirely (spec 8.2).
func tooBroad(a protocol.AppRule, env Env) bool {
	if a.Matcher.Kind != protocol.KindFolder {
		return false
	}
	d := strings.TrimRight(a.Matcher.Value, `\`)
	if isRoot(a.Matcher.Value) {
		return true
	}
	for _, s := range []string{env.WindowsDir, env.ProgramFiles, env.ProgramFilesX86} {
		if s != "" && underOrEqual(strings.TrimRight(s, `\`), d) {
			return true
		}
	}
	return false
}

// protects: the rule would catch the helper, the core or the system Tailscale processes (spec 8.2).
func protects(a protocol.AppRule, env Env) bool {
	for _, e := range Expressions(a, env) {
		re, err := regexp.Compile(e)
		if err != nil {
			return true
		}
		for _, p := range env.Protected {
			if p != "" && re.MatchString(p) {
				return true
			}
		}
	}
	return false
}

// scopes are the keys by which two rules have the same coverage.
func scopes(a protocol.AppRule, env Env) []string {
	v := strings.ToLower(a.Matcher.Value)
	switch a.Matcher.Kind {
	case protocol.KindSystem:
		return []string{"system", "file:" + strings.ToLower(svchost(env))}
	default:
		return []string{a.Matcher.Kind + ":" + v}
	}
}

func sameScope(a, b protocol.AppRule, env Env) bool {
	for _, x := range scopes(a, env) {
		for _, y := range scopes(b, env) {
			if x == y {
				return true
			}
		}
	}
	return false
}

// contains: outer is a folder rule and inner lies inside it (spec 8.2).
func contains(outer, inner protocol.AppRule, env Env) bool {
	if outer.Matcher.Kind != protocol.KindFolder {
		return false
	}
	o := outer.Matcher.Value
	switch inner.Matcher.Kind {
	case protocol.KindFolder:
		return hasPrefixFold(inner.Matcher.Value, o) && !strings.EqualFold(inner.Matcher.Value, o)
	case protocol.KindFile:
		return hasPrefixFold(inner.Matcher.Value, o)
	case protocol.KindPackage:
		return hasPrefixFold(inner.Exe, o)
	case protocol.KindSystem:
		return hasPrefixFold(svchost(env), o)
	}
	return false
}

// Headless is one rule object of a source rule-set file of sing-box (format version 3).
type Headless struct {
	Type             string     `json:"type,omitempty"`
	Mode             string     `json:"mode,omitempty"`
	Rules            []Headless `json:"rules,omitempty"`
	ProcessPathRegex []string   `json:"process_path_regex,omitempty"`
	Invert           bool       `json:"invert,omitempty"`
}

// RuleSet is a source rule-set file.
type RuleSet struct {
	Version int        `json:"version"`
	Rules   []Headless `json:"rules"`
}

// Documents builds the rule sets of the direct and the VPN lane from a checked layout.
// A broader rule gets the nested rules of the other lane as inverted parts of a
// logical "and" rule (spec 8.2). Every expression is compiled here first (spec 6.5).
// A rule without a usable expression (Expressions returns nil: it skipped Normalize)
// is an error, never an empty entry that matches nothing or excludes nothing.
func Documents(apps []protocol.AppRule, env Env) (direct, vpn RuleSet, err error) {
	direct = RuleSet{Version: 3, Rules: []Headless{{ProcessPathRegex: []string{Stub}}}}
	vpn = RuleSet{Version: 3, Rules: []Headless{{ProcessPathRegex: []string{Stub}}}}
	for _, a := range apps {
		own := Expressions(a, env)
		if len(own) == 0 {
			return direct, vpn, fmt.Errorf("rule %s: no usable expression", a.ID)
		}
		if err := compileAll(own); err != nil {
			return direct, vpn, fmt.Errorf("rule %s: %v", a.ID, err)
		}
		entry := Headless{ProcessPathRegex: own}
		var excluded []Headless
		for _, b := range apps {
			if b.Lane != a.Lane && contains(a, b, env) {
				nested := Expressions(b, env)
				if len(nested) == 0 {
					return direct, vpn, fmt.Errorf("rule %s: no usable expression", b.ID)
				}
				excluded = append(excluded, Headless{ProcessPathRegex: nested, Invert: true})
			}
		}
		if len(excluded) > 0 {
			entry = Headless{Type: "logical", Mode: "and", Rules: append([]Headless{{ProcessPathRegex: own}}, excluded...)}
		}
		if a.Lane == protocol.LaneDirect {
			direct.Rules = append(direct.Rules, entry)
		} else {
			vpn.Rules = append(vpn.Rules, entry)
		}
	}
	return direct, vpn, nil
}

// Files is Documents encoded as the bytes written to disk: UTF-8 without BOM, one trailing newline.
func Files(apps []protocol.AppRule, env Env) (direct, vpn []byte, err error) {
	d, v, err := Documents(apps, env)
	if err != nil {
		return nil, nil, err
	}
	if direct, err = json.MarshalIndent(d, "", "  "); err != nil {
		return nil, nil, err
	}
	if vpn, err = json.MarshalIndent(v, "", "  "); err != nil {
		return nil, nil, err
	}
	return append(direct, '\n'), append(vpn, '\n'), nil
}

func compileAll(expressions []string) error {
	for _, e := range expressions {
		if _, err := regexp.Compile(e); err != nil {
			return err
		}
	}
	return nil
}

// Set answers "which lane does this process belong to" by the precedence of spec 8.2:
// file, package and system rules beat folder rules, a deeper folder beats an outer one.
type Set struct {
	entries []entry
}

type entry struct {
	rule  protocol.AppRule
	rank  int
	depth int
	res   []*regexp.Regexp
}

// Compile prepares a checked layout for LaneOf. A rule without a usable expression
// is an error: it would never match, and LaneOf would report no lane for it.
func Compile(apps []protocol.AppRule, env Env) (*Set, error) {
	s := &Set{}
	for _, a := range apps {
		e := entry{rule: a, rank: 2, depth: len(a.Matcher.Value)}
		if a.Matcher.Kind == protocol.KindFolder {
			e.rank = 1
		}
		for _, x := range Expressions(a, env) {
			re, err := regexp.Compile(x)
			if err != nil {
				return nil, fmt.Errorf("rule %s: %v", a.ID, err)
			}
			e.res = append(e.res, re)
		}
		if len(e.res) == 0 {
			return nil, fmt.Errorf("rule %s: no usable expression", a.ID)
		}
		s.entries = append(s.entries, e)
	}
	return s, nil
}

// LaneOf returns the lane of the most specific matching rule, and false when no rule matches.
func (s *Set) LaneOf(path string) (string, bool) {
	r, ok := s.RuleOf(path)
	return r.Lane, ok
}

// RuleOf is the rule that decides the lane of path (the same precedence as LaneOf), if any.
// The activity groups a pinned program by it (spec 8.4). A nil Set has no rules.
func (s *Set) RuleOf(path string) (protocol.AppRule, bool) {
	if s == nil {
		return protocol.AppRule{}, false
	}
	best := -1
	for i, e := range s.entries {
		if !matchAny(e.res, path) {
			continue
		}
		if best < 0 || e.rank > s.entries[best].rank || (e.rank == s.entries[best].rank && e.depth > s.entries[best].depth) {
			best = i
		}
	}
	if best < 0 {
		return protocol.AppRule{}, false
	}
	return s.entries[best].rule, true
}

func matchAny(res []*regexp.Regexp, path string) bool {
	for _, re := range res {
		if re.MatchString(path) {
			return true
		}
	}
	return false
}
