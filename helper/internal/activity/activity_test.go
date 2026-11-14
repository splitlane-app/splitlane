package activity

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

var testEnv = rules.Env{
	WindowsDir:      `C:\Windows`,
	ProgramFiles:    `C:\Program Files`,
	ProgramFilesX86: `C:\Program Files (x86)`,
	UserDirs:        []string{`C:\Users\u\Downloads`},
	Describe: func(exe string) string {
		switch strings.ToLower(exe) {
		case `c:\games\steam\steam.exe`:
			return "Steam Client"
		case `c:\apps\editor\editor.exe`:
			return "Text Editor"
		}
		return ""
	},
}

var start = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

const (
	steamID = "0b5c2a6e-6a43-4c55-9d43-2f1f4a0c9e11"
	gameID  = "5d0f3c1a-2b7e-4f60-8a9d-1c2e3f4a5b6c"
	otherID = "7a6b5c4d-3e2f-4a1b-9c8d-7e6f5a4b3c2d"
	sysID   = "9e8d7c6b-5a49-4f38-8e27-160f5e4d3c2b"

	discord = `C:\Users\u\AppData\Local\Discord\`
)

// layout is a checked and compiled layout, the way the engine builds it.
func layout(t *testing.T, apps ...protocol.AppRule) *rules.Set {
	t.Helper()
	checked, rejected, err := rules.Check(apps, testEnv)
	if err != nil || rejected != nil {
		t.Fatalf("check: %v %v", err, rejected)
	}
	set, err := rules.Compile(checked, testEnv)
	if err != nil {
		t.Fatal(err)
	}
	return set
}

func pinned(id, lane, kind, exe string) protocol.AppRule {
	return protocol.AppRule{ID: id, Exe: exe, Matcher: protocol.Matcher{Kind: kind}, Lane: lane}
}

func conn(id, path, lane string, up, down int64) Conn {
	return Conn{ID: id, ProcessPath: path, Lane: lane, Host: "host.example", Upload: up, Download: down}
}

func find(t *testing.T, s protocol.Activity, key string) protocol.ActivityApp {
	t.Helper()
	for _, a := range s.Apps {
		if a.Key == key {
			return a
		}
	}
	t.Fatalf("no group %q in %+v", key, s.Apps)
	return protocol.ActivityApp{}
}

func keys(s protocol.Activity) []string {
	out := []string{}
	for _, a := range s.Apps {
		out = append(out, a.Key)
	}
	return out
}

func idle(lane string) []protocol.ActivityApp {
	return []protocol.ActivityApp{
		{Key: protocol.KeySystem, Kind: protocol.KindSystem, Lane: lane},
		{Key: protocol.KeyUnattributed, Kind: protocol.KindUnattributed, Lane: lane},
	}
}

func TestRatesFromCounters(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	a, b := `C:\Apps\A\a.exe`, `C:\Apps\B\b.exe`
	// the first sample is a baseline (TestFirstSampleIsABaseline): it keeps the counters, every rate is 0
	tr.Sample(start, []Conn{conn("1", a, protocol.LaneVPN, 100, 1000), conn("2", b, protocol.LaneDirect, 10, 20)}, none, protocol.LaneVPN)
	s := tr.Summary("")
	if s.Lanes != (protocol.ActivityLanes{}) {
		t.Fatalf("the first sample: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\A\`); g.Down != 0 || g.Up != 0 || g.Connections != 1 {
		t.Fatalf("group a %+v", g)
	}
	// 1 goes on, 2 is gone, 3 is new: it began since the sample before and counts its whole counters
	tr.Sample(start.Add(time.Second), []Conn{conn("1", a, protocol.LaneVPN, 150, 1600), conn("3", b, protocol.LaneDirect, 5, 7)}, none, protocol.LaneVPN)
	s = tr.Summary("")
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 600, Up: 50}, Direct: protocol.Traffic{Down: 7, Up: 5}}); s.Lanes != want {
		t.Fatalf("second sample: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\A\`); g.Down != 600 || g.Up != 50 {
		t.Fatalf("group a %+v", g)
	}
	if g := find(t, s, `C:\Apps\B\`); g.Down != 7 || g.Up != 5 || g.Connections != 1 {
		t.Fatalf("group b %+v", g)
	}
	// id 1 now belongs to a new connection: its upload went down, so both counters count from 0;
	// 3 did not move
	tr.Sample(start.Add(2*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 30, 2000), conn("3", b, protocol.LaneDirect, 5, 7)}, none, protocol.LaneVPN)
	s = tr.Summary("")
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 2000, Up: 30}}); s.Lanes != want {
		t.Fatalf("a reused id: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\B\`); g.Down != 0 || g.Up != 0 || g.Connections != 1 {
		t.Fatalf("an idle connection: %+v", g)
	}
}

func TestGroupingByRuleAndFolder(t *testing.T) {
	set := layout(t,
		pinned(steamID, protocol.LaneDirect, protocol.KindFolder, `C:\Games\Steam\steam.exe`),
		pinned(gameID, protocol.LaneVPN, protocol.KindFile, `C:\Games\Steam\steamapps\common\Game\game.exe`))
	tr := New(testEnv)
	tr.Sample(start.Add(-time.Second), nil, set, protocol.LaneVPN) // the baseline: the connections below count their counters
	tr.Sample(start, []Conn{
		conn("1", `C:\Games\Steam\steam.exe`, protocol.LaneDirect, 10, 100),
		conn("2", `C:\Games\Steam\bin\cef\steamwebhelper.exe`, protocol.LaneDirect, 10, 300),
		conn("3", `C:\Games\Steam\steamapps\common\Game\game.exe`, protocol.LaneVPN, 50, 5000),
		conn("4", discord+`app-1.0.9\Discord.exe`, protocol.LaneVPN, 100, 900),
		conn("5", discord+`Update.exe`, protocol.LaneVPN, 0, 10),
		// Windows paths ignore case: the same folder written otherwise is the same program
		conn("6", `c:\users\u\appdata\local\discord\app-1.0.9\Squirrel.exe`, protocol.LaneVPN, 0, 1),
		conn("7", `C:\Users\u\Downloads\tool.exe`, protocol.LaneVPN, 1, 1),
		conn("8", `C:\Apps\Editor\editor.exe`, protocol.LaneVPN, 0, 2),
	}, set, protocol.LaneVPN)
	steam, game := steamID, gameID
	want := []protocol.ActivityApp{
		{Key: gameID, Kind: protocol.KindApp, Name: "game", Exe: `C:\Games\Steam\steamapps\common\Game\game.exe`,
			RuleID: &game, Lane: protocol.LaneVPN, Down: 5000, Up: 50, Connections: 1},
		{Key: discord, Kind: protocol.KindApp, Name: "Discord", Exe: discord + `app-1.0.9\Discord.exe`,
			Lane: protocol.LaneVPN, Down: 911, Up: 100, Connections: 3},
		// the name of a pinned program is the rule's, whichever file has the most traffic
		{Key: steamID, Kind: protocol.KindApp, Name: "Steam Client", Exe: `C:\Games\Steam\bin\cef\steamwebhelper.exe`,
			RuleID: &steam, Lane: protocol.LaneDirect, Down: 400, Up: 20, Connections: 2},
		{Key: `C:\Apps\Editor\`, Kind: protocol.KindApp, Name: "Text Editor", Exe: `C:\Apps\Editor\editor.exe`,
			Lane: protocol.LaneVPN, Down: 2, Connections: 1},
		// a shared folder: the file is the program
		{Key: `C:\Users\u\Downloads\tool.exe`, Kind: protocol.KindApp, Name: "tool", Exe: `C:\Users\u\Downloads\tool.exe`,
			Lane: protocol.LaneVPN, Down: 1, Up: 1, Connections: 1},
		{Key: protocol.KeySystem, Kind: protocol.KindSystem, Lane: protocol.LaneVPN},
		{Key: protocol.KeyUnattributed, Kind: protocol.KindUnattributed, Lane: protocol.LaneVPN},
	}
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, want) {
		t.Fatalf("summary:\n got %+v\nwant %+v", got, want)
	}
	// exe is the file with the most bytes since the group appeared (not in the last second),
	// a tie goes to the path that sorts first, and the name follows the file
	tr.Sample(start.Add(time.Second), []Conn{
		conn("4", discord+`app-1.0.9\Discord.exe`, protocol.LaneVPN, 100, 900),
		conn("5", discord+`Update.exe`, protocol.LaneVPN, 0, 1000),
	}, set, protocol.LaneVPN)
	if g := find(t, tr.Summary(""), discord); g.Exe != discord+`app-1.0.9\Discord.exe` || g.Name != "Discord" {
		t.Fatalf("1000 bytes of Update.exe against 1000 of Discord.exe so far: %+v", g)
	}
	tr.Sample(start.Add(2*time.Second), []Conn{
		conn("4", discord+`app-1.0.9\Discord.exe`, protocol.LaneVPN, 100, 900),
		conn("5", discord+`Update.exe`, protocol.LaneVPN, 0, 1001),
	}, set, protocol.LaneVPN)
	if g := find(t, tr.Summary(""), discord); g.Exe != discord+`Update.exe` || g.Name != "Update" || g.Connections != 2 {
		t.Fatalf("1001 bytes of Update.exe against 1000: %+v", g)
	}
}

func TestSystemAndUnattributedTiles(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, idle(protocol.LaneVPN)) {
		t.Fatalf("before the first sample: %+v", got)
	}
	tr.Sample(start, nil, none, protocol.LaneDirect)
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, idle(protocol.LaneDirect)) {
		t.Fatalf("idle, the main lane is direct: %+v", got)
	}
	tr.Sample(start.Add(time.Second), []Conn{
		conn("1", `:System`, protocol.LaneVPN, 1, 2),
		conn("2", `C:\WINDOWS\system32\svchost.exe`, protocol.LaneVPN, 3, 4),
		conn("3", "", protocol.LaneVPN, 10, 20),
		conn("4", "", protocol.LaneDirect, 1, 1),
		conn("5", `C:\Windows\explorer.exe`, protocol.LaneVPN, 0, 1),
		conn("6", `C:\Tools\svchost.exe`, protocol.LaneVPN, 0, 1),
	}, none, protocol.LaneVPN)
	s := tr.Summary("")
	if g := find(t, s, protocol.KeySystem); g.Kind != protocol.KindSystem || g.RuleID != nil || g.Name != "" ||
		g.Lane != protocol.LaneVPN || g.Down != 6 || g.Up != 4 || g.Connections != 2 {
		t.Errorf("system %+v", g)
	}
	if g := find(t, s, protocol.KeyUnattributed); g.Kind != protocol.KindUnattributed || g.RuleID != nil || g.Name != "" ||
		g.Exe != "" || g.Lane != protocol.LaneVPN || g.Down != 21 || g.Up != 11 || g.Connections != 2 {
		t.Errorf("unattributed %+v", g)
	}
	// another program of the Windows folder, and svchost.exe outside System32, are programs of their own
	if g := find(t, s, `C:\Windows\explorer.exe`); g.Kind != protocol.KindApp || g.Name != "explorer" {
		t.Errorf("explorer %+v", g)
	}
	if g := find(t, s, `C:\Tools\`); g.Kind != protocol.KindApp || g.Name != "svchost" {
		t.Errorf("svchost.exe elsewhere %+v", g)
	}
	// a pinned "Windows services" tile takes svchost.exe and :System; the special tile leaves the
	// summary while the rule is there (TestNoSystemTileWhenSystemIsPinned)
	tr.Sample(start.Add(2*time.Second), []Conn{
		conn("1", `:System`, protocol.LaneVPN, 2, 4),
		conn("2", `C:\WINDOWS\system32\svchost.exe`, protocol.LaneVPN, 6, 8),
	}, layout(t, pinned(sysID, protocol.LaneDirect, protocol.KindSystem, "")), protocol.LaneVPN)
	s = tr.Summary("")
	if g := find(t, s, sysID); g.Kind != protocol.KindSystem || g.RuleID == nil || *g.RuleID != sysID || g.Name != "svchost" ||
		g.Lane != protocol.LaneDirect || g.Down != 6 || g.Up != 4 || g.Connections != 2 {
		t.Errorf("pinned system %+v", g)
	}
	if got := keys(s); slices.Contains(got, protocol.KeySystem) || !slices.Contains(got, protocol.KeyUnattributed) {
		t.Errorf("the pinned rule instead of the special tile: %v", got)
	}
}

// Ruling of plan 1B: while the layout pins "Windows services" (a rule of kind system), its traffic is
// the rule's, and the unpinned tile system is left out of the summary, its detail too: two tiles of
// the same name would show one of them always idle. The tile unattributed stays in every summary.
// Once the rule goes, the special tile is back and the rule's group leaves at once.
func TestNoSystemTileWhenSystemIsPinned(t *testing.T) {
	tr := New(testEnv)
	sys := layout(t, pinned(sysID, protocol.LaneDirect, protocol.KindSystem, ""))
	tr.Sample(start.Add(-time.Second), nil, sys, protocol.LaneVPN) // the baseline
	tr.Sample(start, []Conn{conn("1", `:System`, protocol.LaneDirect, 1, 2), conn("2", "", protocol.LaneVPN, 3, 4)}, sys, protocol.LaneVPN)
	s := tr.Summary(protocol.KeySystem)
	if got, want := keys(s), []string{protocol.KeyUnattributed, sysID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pinned: %v, want %v", got, want)
	}
	if s.Detail != nil {
		t.Fatalf("no detail for a tile that is not in the summary: %+v", s.Detail)
	}
	tr.Sample(start.Add(time.Second), nil, sys, protocol.LaneVPN)
	if got, want := keys(tr.Summary("")), []string{sysID, protocol.KeyUnattributed}; !reflect.DeepEqual(got, want) {
		t.Fatalf("pinned, idle: %v, want %v", got, want)
	}
	tr.Sample(start.Add(2*time.Second), nil, layout(t), protocol.LaneVPN)
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, idle(protocol.LaneVPN)) {
		t.Fatalf("the rule is gone: %+v", got)
	}
}

// Ruling of plan 1B: a rate is the bytes since the previous sample per second of the real time between
// the two samples, so a tick that came late does not show a burst; a gap shorter than a second (the tick
// after a late one) counts as one second. The first sample is a baseline (TestFirstSampleIsABaseline). The
// executable of a group is still chosen by its bytes.
func TestRatesScaleWithElapsedTime(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	a, b := `C:\Apps\A\a.exe`, `C:\Apps\A\b.exe`
	tr.Sample(start, []Conn{conn("1", a, protocol.LaneVPN, 100, 1000)}, none, protocol.LaneVPN)
	if s := tr.Summary(""); s.Lanes != (protocol.ActivityLanes{}) {
		t.Fatalf("first sample: %+v", s.Lanes)
	}
	// 2 s later: 2000 and 200 more bytes are 1000 and 100 a second; a new connection counts its
	// whole counters over the same 2 s
	tr.Sample(start.Add(2*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 300, 3000), conn("2", b, protocol.LaneDirect, 40, 401)}, none, protocol.LaneVPN)
	s := tr.Summary(`C:\Apps\A\`)
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 1000, Up: 100}, Direct: protocol.Traffic{Down: 200, Up: 20}}); s.Lanes != want {
		t.Fatalf("2 s: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\A\`); g.Down != 1200 || g.Up != 120 || g.Exe != a {
		t.Fatalf("2 s: group %+v", g)
	}
	want := []protocol.ActivityConn{{Host: "host.example", Lane: protocol.LaneVPN, Down: 1000, Up: 100},
		{Host: "host.example", Lane: protocol.LaneDirect, Down: 200, Up: 20}}
	if s.Detail == nil || !reflect.DeepEqual(s.Detail.Connections, want) {
		t.Fatalf("2 s: detail %+v", s.Detail)
	}
	// 500 ms later counts as a second
	tr.Sample(start.Add(2500*time.Millisecond), []Conn{conn("1", a, protocol.LaneVPN, 350, 3500), conn("2", b, protocol.LaneDirect, 40, 401)}, none, protocol.LaneVPN)
	if s := tr.Summary(""); s.Lanes != (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 500, Up: 50}}) {
		t.Fatalf("500 ms: %+v", s.Lanes)
	}
}

// Review F4-2: the counters of a connection count from its start. The first sample after New or Reset, and
// the first after a gap of more than 2 s (no window for a while, the tracker kept), is a baseline: every
// rate is 0 there, for the connections seen before too, and the counters are kept for the next sample; a
// download under way never shows its whole size as one second. A connection first seen on a regular tick
// began since the tick before: it counts its counters.
func TestFirstSampleIsABaseline(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	a := `C:\Apps\A\a.exe`
	const size = 400 << 20 // a download under way: 400 MiB so far
	if tr.Sample(start, []Conn{conn("1", a, protocol.LaneVPN, 100, size)}, none, protocol.LaneVPN) {
		t.Fatal("the first sample counted")
	}
	s := tr.Summary("")
	if s.Lanes != (protocol.ActivityLanes{}) {
		t.Fatalf("the first sample is a baseline: %+v", s.Lanes)
	}
	if g := find(t, s, `C:\Apps\A\`); g.Down != 0 || g.Up != 0 || g.Connections != 1 || g.Exe != a {
		t.Fatalf("the first sample: %+v", g)
	}
	if !tr.Sample(start.Add(time.Second), []Conn{conn("1", a, protocol.LaneVPN, 150, size+1000), conn("2", a, protocol.LaneDirect, 5, 7)}, none, protocol.LaneVPN) {
		t.Fatal("a regular tick did not count")
	}
	if s := tr.Summary(""); s.Lanes != (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 1000, Up: 50}, Direct: protocol.Traffic{Down: 7, Up: 5}}) {
		t.Fatalf("a second later: %+v", s.Lanes)
	}
	// 3 s without a sample: a baseline again, for every connection
	if tr.Sample(start.Add(4*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 300, size+9000), conn("2", a, protocol.LaneDirect, 9, 9),
		conn("3", a, protocol.LaneVPN, 1, 20<<20)}, none, protocol.LaneVPN) {
		t.Fatal("the sample after a gap counted")
	}
	if s := tr.Summary(""); s.Lanes != (protocol.ActivityLanes{}) {
		t.Fatalf("after a gap of 3 s: %+v", s.Lanes)
	}
	tr.Sample(start.Add(5*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 310, size+10000), conn("3", a, protocol.LaneVPN, 2, 20<<20+100)}, none, protocol.LaneVPN)
	if s := tr.Summary(""); s.Lanes != (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 1100, Up: 11}}) {
		t.Fatalf("a second after the gap: %+v", s.Lanes)
	}
	tr.Reset()
	tr.Sample(start.Add(6*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 320, size+11000)}, none, protocol.LaneVPN)
	if s := tr.Summary(""); s.Lanes != (protocol.ActivityLanes{}) {
		t.Fatalf("the first sample after Reset: %+v", s.Lanes)
	}
}

func TestUnpinnedKeptFiveMinutes(t *testing.T) {
	tool, other, gameExe := `C:\Apps\Tool\tool.exe`, `C:\Apps\Other\other.exe`, `C:\Games\Game\game.exe`
	game := pinned(gameID, protocol.LaneVPN, protocol.KindFile, gameExe)
	set := layout(t, game)
	tr := New(testEnv)
	tr.Sample(start, []Conn{conn("1", tool, protocol.LaneVPN, 5, 5), conn("2", gameExe, protocol.LaneVPN, 1, 1)}, set, protocol.LaneVPN)
	tr.Sample(start.Add(time.Second), []Conn{conn("3", other, protocol.LaneVPN, 1, 1)}, set, protocol.LaneVPN)
	s := tr.Summary("")
	if g := find(t, s, `C:\Apps\Tool\`); g.Down != 0 || g.Up != 0 || g.Connections != 0 || g.Exe != tool || g.Name != "tool" {
		t.Fatalf("a program without connections stays, with zero rates: %+v", g)
	}
	if g := find(t, s, gameID); g.Connections != 0 {
		t.Fatalf("a pinned program too: %+v", g)
	}
	find(t, s, `C:\Apps\Other\`)
	// a kept program that gets pinned leaves at once: its traffic is the rule's now
	set = layout(t, game, pinned(otherID, protocol.LaneDirect, protocol.KindFolder, other))
	tr.Sample(start.Add(2*time.Second), nil, set, protocol.LaneVPN)
	if got, want := keys(tr.Summary("")), []string{gameID, `C:\Apps\Tool\`, protocol.KeySystem, protocol.KeyUnattributed}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after pinning other.exe: %v", got)
	}
	// 5 minutes after the last connection the program is still there, and follows the main lane
	tr.Sample(start.Add(5*time.Minute), nil, set, protocol.LaneDirect)
	s = tr.Summary("")
	if g := find(t, s, `C:\Apps\Tool\`); g.Lane != protocol.LaneDirect {
		t.Fatalf("the main lane moved: %+v", g)
	}
	if g := find(t, s, gameID); g.Lane != protocol.LaneVPN {
		t.Fatalf("a pinned program keeps the lane of its rule: %+v", g)
	}
	tr.Sample(start.Add(5*time.Minute+time.Second), nil, set, protocol.LaneDirect)
	if got := tr.Summary("").Apps; !reflect.DeepEqual(got, idle(protocol.LaneDirect)) {
		t.Fatalf("later than 5 minutes after the last connection: %+v", got)
	}
}

func TestDetailForAGroup(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	tr.Sample(start.Add(-time.Second), nil, none, protocol.LaneVPN) // the baseline
	tr.Sample(start, []Conn{
		{ID: "1", ProcessPath: discord + `app-1.0.9\Discord.exe`, Lane: protocol.LaneVPN, Host: "gateway.discord.example", Upload: 10, Download: 100},
		{ID: "2", ProcessPath: discord + `Update.exe`, Lane: protocol.LaneDirect, Host: "203.0.113.7", Upload: 1, Download: 1000},
		{ID: "3", ProcessPath: discord + `app-1.0.9\Discord.exe`, Lane: protocol.LaneVPN, Download: 5},
		{ID: "4", ProcessPath: `C:\Apps\Tool\tool.exe`, Lane: protocol.LaneVPN, Host: "tool.example", Upload: 1, Download: 1},
	}, none, protocol.LaneVPN)
	if d := tr.Summary("").Detail; d != nil {
		t.Fatalf("no detail without detailFor: %+v", d)
	}
	if d := tr.Summary(`C:\Apps\Nothing\`).Detail; d != nil {
		t.Fatalf("a key that is not in the summary gives no detail: %+v", d)
	}
	s := tr.Summary(discord)
	want := &protocol.ActivityDetail{Key: discord,
		Exes: []string{discord + `Update.exe`, discord + `app-1.0.9\Discord.exe`},
		Connections: []protocol.ActivityConn{
			{Host: "203.0.113.7", Lane: protocol.LaneDirect, Down: 1000, Up: 1},
			{Host: "gateway.discord.example", Lane: protocol.LaneVPN, Down: 100, Up: 10},
			{Host: "", Lane: protocol.LaneVPN, Down: 5},
		}}
	if !reflect.DeepEqual(s.Detail, want) {
		t.Fatalf("detail:\n got %+v\nwant %+v", s.Detail, want)
	}
	if !reflect.DeepEqual(s.Apps, tr.Summary("").Apps) {
		t.Fatal("the detail must not change the summary")
	}
	// kept without connections: the files stay, the connections are empty
	tr.Sample(start.Add(time.Second), nil, none, protocol.LaneVPN)
	if d := tr.Summary(discord).Detail; d == nil || len(d.Exes) != 2 || d.Connections == nil || len(d.Connections) != 0 {
		t.Fatalf("kept group: %+v", d)
	}
	if d := tr.Summary(protocol.KeyUnattributed).Detail; d == nil || d.Exes == nil || len(d.Exes) != 0 || d.Connections == nil {
		t.Fatalf("a special tile has a detail too: %+v", d)
	}
}

// Review F4-1: the file description comes from the file's resources on the disk (on a network share a
// read can take the SMB timeout), so the tracker reads it outside its lock: a summary asked while it is
// read is the one of the sample before, at once. The sample that reads it names the program after it.
func TestDescribeOutsideTheLock(t *testing.T) {
	entered, release := make(chan string, 8), make(chan struct{})
	env := testEnv
	env.Describe = func(exe string) string {
		entered <- exe
		<-release
		return "Slow Tool"
	}
	tr := New(env)
	none := layout(t)
	slow := `C:\Apps\Slow\slow.exe`
	tr.Sample(start, nil, none, protocol.LaneVPN)
	done := make(chan struct{})
	go func() {
		defer close(done)
		tr.Sample(start.Add(time.Second), []Conn{conn("1", slow, protocol.LaneVPN, 1, 1)}, none, protocol.LaneVPN)
	}()
	if exe := <-entered; exe != slow {
		t.Fatalf("described %q", exe)
	}
	got := make(chan protocol.Activity, 1)
	go func() { got <- tr.Summary("") }()
	select {
	case s := <-got:
		if !reflect.DeepEqual(s.Apps, idle(protocol.LaneVPN)) {
			t.Fatalf("while the description is read, the summary of the sample before: %+v", s.Apps)
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("Summary waits for the file description")
	}
	close(release)
	<-done
	if g := find(t, tr.Summary(""), `C:\Apps\Slow\`); g.Name != "Slow Tool" || g.Down != 1 {
		t.Fatalf("after the description: %+v", g)
	}
}

// Review F4-1: a file is described once (a cache by path, while the tracker keeps its groups), even when the
// group's executable goes to another file and back. A path that Normalize refuses is never described: a
// network share (its read is the slow one), a file that is not an .exe; the group is named after the file.
func TestDescribeOncePerLocalPath(t *testing.T) {
	var calls []string
	env := testEnv
	env.Describe = func(exe string) string {
		calls = append(calls, exe)
		return ""
	}
	tr := New(env)
	none := layout(t)
	a, b := `C:\Apps\Two\a.exe`, `C:\Apps\Two\b.exe`
	share, odd := `\\server\share\Tools\tool.exe`, `C:\Apps\Odd\odd.bin`
	tr.Sample(start, nil, none, protocol.LaneVPN)
	tr.Sample(start.Add(time.Second), []Conn{conn("1", a, protocol.LaneVPN, 0, 10), conn("3", share, protocol.LaneVPN, 0, 1),
		conn("4", odd, protocol.LaneVPN, 0, 1)}, none, protocol.LaneVPN)
	tr.Sample(start.Add(2*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 0, 10), conn("2", b, protocol.LaneVPN, 0, 20)}, none, protocol.LaneVPN)
	if g := find(t, tr.Summary(""), `C:\Apps\Two\`); g.Exe != b || g.Name != "b" {
		t.Fatalf("b.exe has the most bytes: %+v", g)
	}
	tr.Sample(start.Add(3*time.Second), []Conn{conn("1", a, protocol.LaneVPN, 0, 30), conn("2", b, protocol.LaneVPN, 0, 20)}, none, protocol.LaneVPN)
	s := tr.Summary("")
	if g := find(t, s, `C:\Apps\Two\`); g.Exe != a || g.Name != "a" {
		t.Fatalf("a.exe has the most bytes again: %+v", g)
	}
	if g := find(t, s, share); g.Name != "tool" {
		t.Fatalf("a program on a share: %+v", g)
	}
	if g := find(t, s, odd); g.Name != "odd" {
		t.Fatalf("not an .exe: %+v", g)
	}
	if want := []string{a, b}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("described %q, want %q", calls, want)
	}
}

func TestResetForgets(t *testing.T) {
	tr := New(testEnv)
	none := layout(t)
	tool := `C:\Apps\Tool\tool.exe`
	tr.Sample(start, []Conn{conn("1", tool, protocol.LaneVPN, 100, 200), conn("2", "", protocol.LaneDirect, 1, 1)}, none, protocol.LaneDirect)
	tr.Reset()
	s := tr.Summary(`C:\Apps\Tool\`)
	if s.Lanes != (protocol.ActivityLanes{}) || !reflect.DeepEqual(s.Apps, idle(protocol.LaneVPN)) || s.Detail != nil {
		t.Fatalf("after Reset: %+v", s)
	}
	// the first sample after Reset is a baseline again, the next counts from its counters
	if tr.Sample(start.Add(time.Second), []Conn{conn("1", tool, protocol.LaneVPN, 100, 200)}, none, protocol.LaneVPN) {
		t.Fatal("the first sample after Reset counted")
	}
	tr.Sample(start.Add(2*time.Second), []Conn{conn("1", tool, protocol.LaneVPN, 110, 230)}, none, protocol.LaneVPN)
	if g := find(t, tr.Summary(""), `C:\Apps\Tool\`); g.Down != 30 || g.Up != 10 {
		t.Fatalf("after Reset: %+v", g)
	}
}
