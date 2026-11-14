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

	mu     sync.Mutex
	prev   map[string]counters // the counters of the previous sample, by connection id
	groups map[string]*group   // by class.id
	lanes  protocol.ActivityLanes
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

// Reset forgets everything (the core stopped): counters, groups and the files seen.
func (t *Tracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.reset()
}

func (t *Tracker) reset() {
	t.prev = map[string]counters{}
	t.lanes = protocol.ActivityLanes{}
	t.groups = map[string]*group{}
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
// or direct in "all direct").
func (t *Tracker) Sample(now time.Time, conns []Conn, layout Layout, mainLane string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, g := range t.groups {
		g.down, g.up, g.conns = 0, 0, nil
	}
	t.lanes = protocol.ActivityLanes{}
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
		if !seen || cur.up < was.up || cur.down < was.down {
			was = counters{} // a new connection, or a new one under a reused id
		}
		next[c.ID] = cur
		up, down := cur.up-was.up, cur.down-was.down
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
			e.bytes += up + down
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

// describe is the file description from the resources of exe, or its name without the extension (spec 8.4).
func (t *Tracker) describe(exe string) string {
	if t.env.Describe != nil {
		if d := strings.TrimSpace(t.env.Describe(exe)); d != "" {
			return d
		}
	}
	base := filepath.Base(exe)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// Summary is the event of the last sample; detailFor "" gives no detail. Apps are ordered by
// down+up, the busiest first, then by key.
func (t *Tracker) Summary(detailFor string) protocol.Activity {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := protocol.Activity{Lanes: t.lanes, Apps: make([]protocol.ActivityApp, 0, len(t.groups))}
	for _, g := range t.groups {
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
