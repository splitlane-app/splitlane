// Package rules turns pinned programs (spec 8.1) into matchers, regular
// expressions and the two rule-set files of the core (spec 6.5, 8.2).
package rules

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"splitlane/helper/internal/protocol"
)

// Env is what the rules need to know about this computer.
type Env struct {
	WindowsDir      string   // C:\Windows
	ProgramFiles    string   // C:\Program Files
	ProgramFilesX86 string   // C:\Program Files (x86)
	UserDirs        []string // Desktop, Downloads, Documents and the temp folder of the setup user
	Protected       []string // full paths: the helper, the core, tailscaled.exe, tailscale-ipn.exe
	// Describe returns the file description from the version resources of exe, or "".
	Describe func(exe string) string
}

// versionDir is a folder named like a version: optional "app-" and two to four numbers (spec 8.2).
var versionDir = regexp.MustCompile(`(?i)^(app-)?\d+(\.\d+){1,3}$`)

// Normalize computes matcher.value (spec 8.1, 8.2) and fills an empty name.
// A folder that is shared by many programs turns the rule into kind file.
// An error means the input itself is malformed (bad_request).
func Normalize(a protocol.AppRule, env Env) (protocol.AppRule, error) {
	n := a
	if a.Matcher.Kind == protocol.KindSystem {
		if a.Matcher.Value != "" {
			return n, fmt.Errorf("rule %s: a system rule has an empty value", a.ID)
		}
		if n.Name == "" {
			n.Name = displayName(svchost(env), env)
		}
		return n, nil
	}
	exe, err := cleanExe(a.Exe)
	if err != nil {
		return n, fmt.Errorf("rule %s: %v", a.ID, err)
	}
	n.Exe = exe
	kind, value := a.Matcher.Kind, ""
	switch kind {
	case protocol.KindFile:
		value = exe
	case protocol.KindFolder:
		dir := filepath.Dir(exe)
		if versionDir.MatchString(filepath.Base(dir)) && !isRoot(dir) {
			dir = filepath.Dir(dir)
		}
		if shared(dir, env) {
			kind, value = protocol.KindFile, exe
		} else {
			value = strings.TrimRight(dir, `\`) + `\`
		}
	case protocol.KindPackage:
		v, ok := packageValue(exe)
		if !ok {
			return n, fmt.Errorf("rule %s: %s is not inside a WindowsApps package folder", a.ID, exe)
		}
		value = v
	default:
		return n, fmt.Errorf("rule %s: unknown kind %q", a.ID, kind)
	}
	if a.Matcher.Value != "" && a.Matcher.Value != value {
		return n, fmt.Errorf("rule %s: matcher.value differs from the one the helper computes", a.ID)
	}
	n.Matcher = protocol.Matcher{Kind: kind, Value: value}
	if n.Name == "" {
		n.Name = displayName(exe, env)
	}
	return n, nil
}

func cleanExe(exe string) (string, error) {
	if exe == "" || !filepath.IsAbs(exe) || filepath.VolumeName(exe) == "" || strings.HasPrefix(exe, `\\`) {
		return "", fmt.Errorf("exe must be an absolute local path")
	}
	exe = filepath.Clean(exe)
	if !strings.EqualFold(filepath.Ext(exe), ".exe") {
		return "", fmt.Errorf("exe must end with .exe")
	}
	return exe, nil
}

func isRoot(dir string) bool {
	return strings.TrimRight(dir, `\`) == strings.TrimRight(filepath.VolumeName(dir), `\`)
}

// shared reports the folders that many programs share: there a folder rule becomes a file rule (spec 8.2).
func shared(dir string, env Env) bool {
	d := strings.TrimRight(dir, `\`)
	if isRoot(dir) || underOrEqual(d, env.WindowsDir) {
		return true
	}
	for _, s := range append([]string{env.ProgramFiles, env.ProgramFilesX86}, env.UserDirs...) {
		if s != "" && strings.EqualFold(d, strings.TrimRight(s, `\`)) {
			return true
		}
	}
	return false
}

func underOrEqual(path, root string) bool {
	if root == "" {
		return false
	}
	root = strings.TrimRight(root, `\`)
	return strings.EqualFold(path, root) || hasPrefixFold(path, root+`\`)
}

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// packageValue is "<package name>_<publisher id>/<file name>" for an exe inside
// ...\WindowsApps\<Name>_<Version>_<Arch>_<ResourceId>_<PublisherId>\...
func packageValue(exe string) (string, bool) {
	parts := strings.Split(exe, `\`)
	for i := 0; i+2 < len(parts); i++ {
		if !strings.EqualFold(parts[i], "WindowsApps") {
			continue
		}
		fields := strings.Split(parts[i+1], "_")
		if len(fields) != 5 || fields[0] == "" || fields[4] == "" {
			return "", false
		}
		return fields[0] + "_" + fields[4] + "/" + parts[len(parts)-1], true
	}
	return "", false
}

func splitPackage(value string) (name, publisher, file string) {
	slash := strings.LastIndex(value, "/")
	id := value[:slash]
	under := strings.LastIndex(id, "_")
	return id[:under], id[under+1:], value[slash+1:]
}

func svchost(env Env) string {
	return strings.TrimRight(env.WindowsDir, `\`) + `\System32\svchost.exe`
}

func displayName(exe string, env Env) string {
	if env.Describe != nil {
		if d := strings.TrimSpace(env.Describe(exe)); d != "" {
			return truncate(d)
		}
	}
	base := filepath.Base(exe)
	return truncate(strings.TrimSuffix(base, filepath.Ext(base)))
}

func truncate(s string) string {
	for len(s) > protocol.MaxNameLen {
		r := []rune(s)
		s = string(r[:len(r)-1])
	}
	return s
}

// Expressions are the case-insensitive process_path_regex expressions of a normalized rule (spec 8.1).
func Expressions(a protocol.AppRule, env Env) []string {
	v := a.Matcher.Value
	switch a.Matcher.Kind {
	case protocol.KindFolder:
		return []string{`(?i)^` + regexp.QuoteMeta(v)}
	case protocol.KindFile:
		return []string{`(?i)^` + regexp.QuoteMeta(v) + `$`}
	case protocol.KindPackage:
		name, publisher, file := splitPackage(v)
		return []string{`(?i)^.*\\WindowsApps\\` + regexp.QuoteMeta(name) + `_[^\\]*_` +
			regexp.QuoteMeta(publisher) + `\\(?:[^\\]*\\)*` + regexp.QuoteMeta(file) + `$`}
	case protocol.KindSystem:
		return []string{`(?i)^` + regexp.QuoteMeta(svchost(env)) + `$`, `(?i)^:System$`}
	}
	return nil
}

// BuiltinExpression is the expression of the built-in rules 5 and 6 of spec 6.2: one exact path.
func BuiltinExpression(path string) string {
	return `(?i)^` + regexp.QuoteMeta(filepath.Clean(path)) + `$`
}
