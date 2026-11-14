// Package activity counts the live activity of spec 8.4: the connections of the core, sampled once
// a second, become the rates of the two lanes and of every program, grouped by rule or by program
// folder, in the data of the activity event.
package activity

import (
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

// keep is how long a group stays in the summary after its last connection (spec 8.4).
const keep = 5 * time.Minute

// gap: a sample that comes more than this after the one before is a baseline, as the first one is (review
// F4-2): the counters moved for longer than its rates could tell.
const gap = 2 * time.Second

// Conn is one connection of the core at one moment: cumulative byte counters.
type Conn struct {
	ID          string
	ProcessPath string // "" when the core could not tell the owner
	Lane        string // protocol.LaneVPN or protocol.LaneDirect
	Host        string // name from the reverse DNS mapping, or the address
	Upload      int64  // bytes so far
	Download    int64
}

// Layout tells the rule of a process path (rules.Set satisfies it).
type Layout interface {
	RuleOf(path string) (protocol.AppRule, bool)
}

// Tracker turns the samples of the core's connections into the summary. It is safe for concurrent use.
type Tracker struct {
	env     rules.Env
	svchost string

	mu        sync.Mutex
	prev      map[string]counters // the counters of the previous sample, by connection id
	last      time.Time           // the time of the previous sample; zero before the first
	groups    map[string]*group   // by class.id
	lanes     protocol.ActivityLanes
	sysPinned bool              // the layout of the last sample pins "Windows services": the tile system is left out
	names     map[string]string // the description of each file read so far, by lower-case path; "" when it has none
}

type counters struct{ up, down int64 }

// group is one entry of the summary.
type group struct {
	key      string
	kind     string
	ruleID   string // "" for an unpinned group
	name     string // the rule's name; for an unpinned program, the name of exe
	named    string // the exe that name was taken from
	lane     string
	exe      string               // the executable with the most bytes since the group appeared
	exes     map[string]*exeBytes // every executable seen while the group is kept, by lower-case path
	lastSeen time.Time
	down, up int64
	conns    []protocol.ActivityConn // the connections of the last sample
}

type exeBytes struct {
	path  string
	bytes int64
}

// class is the group that a process path belongs to now.
type class struct {
	id     string // the index of the group: the key, and a lower-case folder for an unpinned program
	key    string
	kind   string
	ruleID string
	name   string
	lane   string
}

// New is a tracker with nothing seen yet; its summary has the two special tiles on the VPN lane,
// the default lane (spec 5.2), until the first sample names the main lane.
func New(env rules.Env) *Tracker {
	t := &Tracker{env: env, svchost: strings.TrimRight(env.WindowsDir, `\`) + `\System32\svchost.exe`}
	t.reset()
	return t
}

// Reset forgets everything (the core stopped): counters, groups, the files seen and their descriptions,
// the last sample's time.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reset()
}

func (t *Tracker) reset() {
	t.prev = map[string]counters{}
	t.last = time.Time{}
	t.sysPinned = false
	t.lanes = protocol.ActivityLanes{}
	t.groups = map[string]*group{}
	t.names = map[string]string{}
	for _, key := range []string{protocol.KeySystem, protocol.KeyUnattributed} {
		c := t.special(key, protocol.LaneVPN)
		t.groups[c.id] = newGroup(c)
	}
}

// special is the class of the tile system or unattributed: unpinned, always on the main lane.
func (t *Tracker) special(key, mainLane string) class {
	kind := protocol.KindSystem
	if key == protocol.KeyUnattributed {
		kind = protocol.KindUnattributed
	}
	return class{id: key, key: key, kind: kind, lane: mainLane}
}

// Sample takes the connections of one second. mainLane is the lane of unpinned programs now (the default lane,
// or direct in "all direct"). The rates are the bytes since the previous sample per second of the time
// between the two samples (a tick may come late), and at least one second: a tick right after a late one
// counts as one second. A connection first seen began since the sample before: it counts its counters.
// The first sample after New or Reset, and one more than gap after the one before (no window for a
// while), is a baseline (review F4-2): the counters of a connection count from its start, so every rate
// is 0 there and the counters are kept for the next sample; Sample returns false for it. The descriptions
// of the programs seen for the first time are read before, outside the lock (describeNew).
func (t *Tracker) Sample(now time.Time, conns []Conn, layout Layout, mainLane string) (counted bool) {
	t.describeNew(conns, layout, mainLane)
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, g := range t.groups {
		g.down, g.up, g.conns = 0, 0, nil
	}
	t.lanes = protocol.ActivityLanes{}
	t.sysPinned = pinsSystem(layout)
	baseline := t.last.IsZero() || now.Sub(t.last) > gap
	elapsed := time.Second
	if !t.last.IsZero() && now.Sub(t.last) > elapsed {
		elapsed = now.Sub(t.last)
	}
	t.last = now
	// A program has many connections: the rules are matched once per path and sample.
	classes := map[string]class{}
	classOf := func(path string) class {
		cl, ok := classes[path]
		if !ok {
			cl = t.classify(path, layout, mainLane)
			classes[path] = cl
		}
		return cl
	}
	next := make(map[string]counters, len(conns))
	for _, c := range conns {
		cur := counters{up: c.Upload, down: c.Download}
		was, seen := t.prev[c.ID]
		switch {
		case baseline:
			was = cur
		case !seen || cur.up < was.up || cur.down < was.down:
			was = counters{} // a new connection, or a new one under a reused id
		}
		next[c.ID] = cur
		bytes := cur.up - was.up + cur.down - was.down
		up, down := perSecond(cur.up-was.up, elapsed), perSecond(cur.down-was.down, elapsed)
		switch c.Lane {
		case protocol.LaneVPN:
			t.lanes.VPN.Up += up
			t.lanes.VPN.Down += down
		case protocol.LaneDirect:
			t.lanes.Direct.Up += up
			t.lanes.Direct.Down += down
		}
		cl := classOf(c.ProcessPath)
		g := t.groups[cl.id]
		if g == nil {
			g = newGroup(cl)
			t.groups[cl.id] = g
		}
		g.take(cl)
		g.lastSeen = now
		g.up += up
		g.down += down
		g.conns = append(g.conns, protocol.ActivityConn{Host: c.Host, Lane: c.Lane, Down: down, Up: up})
		if c.ProcessPath != "" {
			low := strings.ToLower(c.ProcessPath)
			e := g.exes[low]
			if e == nil {
				e = &exeBytes{path: c.ProcessPath}
				g.exes[low] = e
			}
			e.bytes += bytes
		}
	}
	t.prev = next
	for id, g := range t.groups {
		switch {
		case id == protocol.KeySystem || id == protocol.KeyUnattributed:
			g.lane = mainLane
		case len(g.conns) > 0:
			// its connections decided it above
		case now.Sub(g.lastSeen) > keep:
			delete(t.groups, id)
			continue
		default:
			// A kept group follows the layout and the main lane. When its program now belongs
			// to another group (it was pinned or unpinned), the group leaves at once: the
			// program is in the other one.
			cl := classOf(g.exe)
			if cl.id != id {
				delete(t.groups, id)
				continue
			}
			g.take(cl)
		}
		t.settle(g)
	}
	return !baseline
}

// perSecond is n bytes over elapsed (at least a second) as bytes per second; float64, so that the
// whole counters of a long connection seen for the first time cannot overflow.
func perSecond(n int64, elapsed time.Duration) int64 {
	if elapsed <= time.Second {
		return n
	}
	return int64(float64(n) * float64(time.Second) / float64(elapsed))
}

// pinsSystem: the layout has a rule of kind system ("Windows services" pinned, spec 8.1). Only the
// expressions of such a rule match the pseudo-path :System.
func pinsSystem(layout Layout) bool {
	if layout == nil {
		return false
	}
	r, ok := layout.RuleOf(":System")
	return ok && r.Matcher.Kind == protocol.KindSystem
}

// classify finds the group of a process path (spec 8.4): no owner, a pinned rule, the system
// tile, or the program folder.
func (t *Tracker) classify(path string, layout Layout, mainLane string) class {
	if path == "" {
		return t.special(protocol.KeyUnattributed, mainLane)
	}
	if layout != nil {
		if r, ok := layout.RuleOf(path); ok {
			kind := protocol.KindApp
			if r.Matcher.Kind == protocol.KindSystem {
				kind = protocol.KindSystem
			}
			return class{id: "rule:" + r.ID, key: r.ID, kind: kind, ruleID: r.ID, name: r.Name, lane: r.Lane}
		}
	}
	if strings.EqualFold(path, ":System") || strings.EqualFold(path, t.svchost) {
		return t.special(protocol.KeySystem, mainLane)
	}
	key := rules.ProgramFolder(path, t.env)
	return class{id: "folder:" + strings.ToLower(key), key: key, kind: protocol.KindApp, lane: mainLane}
}

func newGroup(c class) *group {
	g := &group{key: c.key, exes: map[string]*exeBytes{}}
	g.take(c)
	return g
}

// take updates what the layout and the main lane decide: the kind, the rule, the lane, a rule's name.
func (g *group) take(c class) {
	g.kind, g.ruleID, g.lane = c.kind, c.ruleID, c.lane
	if c.ruleID != "" {
		g.name = c.name
	}
}

// describeNew reads the file description of each unpinned program in conns that has not been read yet, once
// per path, outside the lock (review F4-1): it reads the file's resources from the disk, on a network share
// for as long as the SMB timeout, and a summary asked meanwhile must not wait for it. A path that Normalize
// refuses (a share, a pseudo-path, not an .exe) is never read: such a program is named after its file.
func (t *Tracker) describeNew(conns []Conn, layout Layout, mainLane string) {
	if t.env.Describe == nil {
		return
	}
	t.mu.Lock()
	var paths []string
	seen := map[string]bool{}
	for _, c := range conns {
		low := strings.ToLower(c.ProcessPath)
		if _, read := t.names[low]; c.ProcessPath != "" && !read && !seen[low] {
			seen[low] = true
			paths = append(paths, c.ProcessPath)
		}
	}
	t.mu.Unlock()
	for _, path := range paths {
		if cl := t.classify(path, layout, mainLane); cl.ruleID != "" || cl.kind != protocol.KindApp {
			continue // a pinned program has the name of its rule, a special tile none
		}
		d := ""
		if t.local(path) {
			d = strings.TrimSpace(t.env.Describe(path))
		}
		t.mu.Lock()
		t.names[strings.ToLower(path)] = d
		t.mu.Unlock()
	}
}

// local: Normalize takes the path, an .exe on a local drive; only such a file's resources are read.
func (t *Tracker) local(path string) bool {
	env := t.env
	env.Describe = nil
	_, err := rules.Normalize(protocol.AppRule{Exe: path, Matcher: protocol.Matcher{Kind: protocol.KindFile}}, env)
	return err == nil
}

// settle picks the executable with the most bytes; an unpinned program is named after it.
func (t *Tracker) settle(g *group) {
	top := ""
	for low, e := range g.exes {
		if top == "" || e.bytes > g.exes[top].bytes || (e.bytes == g.exes[top].bytes && low < top) {
			top = low
		}
	}
	if top != "" {
		g.exe = g.exes[top].path
	}
	if g.ruleID == "" && g.kind == protocol.KindApp && g.named != g.exe {
		g.name, g.named = t.describe(g.exe), g.exe
	}
}

// describe is the file description from the resources of exe, as describeNew read it, or its name without
// the extension (spec 8.4). It reads no file: it runs under the lock.
func (t *Tracker) describe(exe string) string {
	if d := t.names[strings.ToLower(exe)]; d != "" {
		return d
	}
	base := filepath.Base(exe)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Summary is the event of the last sample; detailFor "" gives no detail. Apps are ordered by
// down+up, the busiest first, then by key. While the layout pins "Windows services", the special
// tile system is left out (its traffic is the rule's); the tile unattributed is always there.
func (t *Tracker) Summary(detailFor string) protocol.Activity {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := protocol.Activity{Lanes: t.lanes, Apps: make([]protocol.ActivityApp, 0, len(t.groups))}
	for id, g := range t.groups {
		if id == protocol.KeySystem && t.sysPinned {
			continue
		}
		a := protocol.ActivityApp{Key: g.key, Kind: g.kind, Name: g.name, Exe: g.exe, Lane: g.lane,
			Down: g.down, Up: g.up, Connections: len(g.conns)}
		if g.ruleID != "" {
			id := g.ruleID
			a.RuleID = &id
		}
		out.Apps = append(out.Apps, a)
		if detailFor != "" && g.key == detailFor {
			out.Detail = g.detail()
		}
	}
	sort.Slice(out.Apps, func(i, j int) bool {
		a, b := out.Apps[i], out.Apps[j]
		if a.Down+a.Up != b.Down+b.Up {
			return a.Down+a.Up > b.Down+b.Up
		}
		return a.Key < b.Key
	})
	return out
}

// detail lists the files seen in the group, sorted, and its connections now, the busiest first.
func (g *group) detail() *protocol.ActivityDetail {
	d := &protocol.ActivityDetail{Key: g.key, Exes: make([]string, 0, len(g.exes)),
		Connections: append([]protocol.ActivityConn{}, g.conns...)}
	for _, e := range g.exes {
		d.Exes = append(d.Exes, e.path)
	}
	sort.Strings(d.Exes)
	sort.SliceStable(d.Connections, func(i, j int) bool {
		a, b := d.Connections[i], d.Connections[j]
		if a.Down+a.Up != b.Down+b.Up {
			return a.Down+a.Up > b.Down+b.Up
		}
		if a.Host != b.Host {
			return a.Host < b.Host
		}
		return a.Lane < b.Lane
	})
	return d
}
