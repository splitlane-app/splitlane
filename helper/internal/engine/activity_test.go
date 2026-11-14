package engine

import (
	"context"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/activity"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

// checkedApps is a layout as lanes.set saves it in state.json: checked and normalized.
func checkedApps(t *testing.T, apps ...protocol.AppRule) []protocol.AppRule {
	t.Helper()
	checked, rejected, err := rules.Check(apps, testRules)
	if err != nil || rejected != nil {
		t.Fatalf("check: %v %v", err, rejected)
	}
	return checked
}

// windows sets the number of connected windows that the pipe gives with SetClients; set directly, so
// that no setup mode starts in off (spec 7).
func windows(h *harness, n int) { h.e.clients.Store(int32(n)) }

// countReads counts the calls of Connections from now on.
func countReads(h *harness) func() int {
	var mu sync.Mutex
	n := 0
	h.clash.mu.Lock()
	h.clash.onConns = func() {
		mu.Lock()
		defer mu.Unlock()
		n++
	}
	h.clash.mu.Unlock()
	return func() int {
		mu.Lock()
		defer mu.Unlock()
		return n
	}
}

// setConns makes conns the connections of the core.
func setConns(h *harness, conns ...Conn) {
	h.clash.mu.Lock()
	defer h.clash.mu.Unlock()
	h.clash.conns = conns
}

// appIn is the group key of a summary; ok false when it is not there.
func appIn(s protocol.Activity, key string) (protocol.ActivityApp, bool) {
	for _, a := range s.Apps {
		if a.Key == key {
			return a, true
		}
	}
	return protocol.ActivityApp{}, false
}

func summaryNow(t *testing.T, h *harness, detailFor string) protocol.Activity {
	t.Helper()
	s, ok := h.e.Activity(detailFor)
	if !ok {
		t.Fatalf("no summary in phase %s", h.e.State().Phase)
	}
	return s
}

// Spec 8.4: while the core with the adapter runs and a window is connected, the connections of the
// run are read once a second; two samples give the rates of the lanes and of each group: a pinned
// program by its rule, an unpinned one by its folder on the main lane, a connection without an owner
// in the tile unattributed, the detail of the group asked for with the host of each connection. The
// layout is compiled again when it changes; the main lane is direct in "all direct". Without a window
// nothing is read and there is no summary.
func TestActivityWhileOn(t *testing.T) {
	h := newHarness(t)
	h.store.state.Apps = checkedApps(t, fileRule(idA, curlA, protocol.LaneDirect))
	h.load()
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
	ctx := context.Background()
	reads := countReads(h)
	setConns(h,
		Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneDirect, Host: "a.example", Upload: 100, Download: 1000},
		Conn{ID: "2", ProcessPath: curlB, Lane: protocol.LaneVPN, Host: "203.0.113.7", Upload: 10, Download: 20})
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != 0 {
		t.Fatalf("no window: nothing read (%d), no summary", reads())
	}
	windows(h, 1)
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != 1 {
		t.Fatalf("the first sample is a baseline (TestActivityStartsFromABaseline): read %d times, no summary", reads())
	}
	h.clock.After(time.Second)
	setConns(h,
		Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneDirect, Host: "a.example", Upload: 150, Download: 1600},
		Conn{ID: "2", ProcessPath: curlB, Lane: protocol.LaneVPN, Host: "203.0.113.7", Upload: 12, Download: 25},
		Conn{ID: "3", Lane: protocol.LaneVPN, Host: "198.51.100.9", Upload: 7, Download: 9})
	h.e.sampleActivity(ctx)
	s := summaryNow(t, h, idA)
	if want := (protocol.ActivityLanes{VPN: protocol.Traffic{Down: 14, Up: 9}, Direct: protocol.Traffic{Down: 600, Up: 50}}); s.Lanes != want {
		t.Fatalf("lanes %+v, want %+v", s.Lanes, want)
	}
	if g, ok := appIn(s, idA); !ok || g.RuleID == nil || *g.RuleID != idA || g.Lane != protocol.LaneDirect ||
		g.Down != 600 || g.Up != 50 || g.Connections != 1 || g.Exe != curlA {
		t.Fatalf("the pinned program: %+v", g)
	}
	if g, ok := appIn(s, `C:\Tools\b\`); !ok || g.RuleID != nil || g.Kind != protocol.KindApp || g.Name != "curl-b" ||
		g.Lane != protocol.LaneVPN || g.Down != 5 || g.Up != 2 {
		t.Fatalf("the unpinned program on the main lane: %+v", g)
	}
	if g, ok := appIn(s, protocol.KeyUnattributed); !ok || g.Down != 9 || g.Up != 7 || g.Connections != 1 {
		t.Fatalf("no owner: %+v", g)
	}
	if _, ok := appIn(s, protocol.KeySystem); !ok {
		t.Fatalf("the special tiles are there: %+v", s.Apps)
	}
	if d := s.Detail; d == nil || d.Key != idA || len(d.Connections) != 1 || d.Connections[0] != (protocol.ActivityConn{Host: "a.example", Lane: protocol.LaneDirect, Down: 600, Up: 50}) {
		t.Fatalf("detail %+v", s.Detail)
	}
	// curl-b is pinned now and "all direct" is on: the new layout groups it, the main lane is direct
	h.e.mu.Lock()
	h.e.saved.Apps = checkedApps(t, fileRule(idA, curlA, protocol.LaneDirect), fileRule(idB, curlB, protocol.LaneDirect))
	h.e.st.AllDirect = true
	h.e.mu.Unlock()
	h.clock.After(time.Second)
	other := `C:\Tools\c\curl-c.exe`
	setConns(h,
		Conn{ID: "2", ProcessPath: curlB, Lane: protocol.LaneDirect, Upload: 12, Download: 30},
		Conn{ID: "4", ProcessPath: other, Lane: protocol.LaneDirect, Upload: 1, Download: 1})
	h.e.sampleActivity(ctx)
	s = summaryNow(t, h, "")
	if g, ok := appIn(s, idB); !ok || g.Lane != protocol.LaneDirect || g.Down != 5 {
		t.Fatalf("curl-b under its new rule: %+v", s.Apps)
	}
	if g, ok := appIn(s, `C:\Tools\c\`); !ok || g.Lane != protocol.LaneDirect {
		t.Fatalf("an unpinned program in all direct: %+v", s.Apps)
	}
	if g, ok := appIn(s, protocol.KeyUnattributed); !ok || g.Lane != protocol.LaneDirect {
		t.Fatalf("the special tile on the main lane: %+v", g)
	}
	windows(h, 0)
	before := reads()
	h.clock.After(time.Second)
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != before {
		t.Fatal("the last window went: nothing is read, the old summary is not offered")
	}
}

// Spec 8.4: the event goes only while the core with the adapter runs: on, degraded, blocked. In off,
// in starting and stopping, and after a disable, the connections are not read and there is no
// summary, with a window connected. A phase that leaves on, degraded and blocked starts the tracker
// again: back in on, the first sample is a baseline again (TestActivityStartsFromABaseline).
func TestNoActivityWhenOff(t *testing.T) {
	h := newHarness(t)
	h.load()
	windows(h, 1)
	ctx := context.Background()
	reads := countReads(h)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 100, Download: 1000})
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != 0 {
		t.Fatalf("off: read %d times", reads())
	}
	startEnable(h)
	h.waitPhase(protocol.PhaseOn)
	h.waitFor("the path read after phase on", func() bool { return h.e.State().Tailscale.Path != nil })
	h.e.sampleActivity(ctx) // the first sample of the run is a baseline, the next ones count what is new
	down := int64(1000)
	for _, phase := range []string{protocol.PhaseOn, protocol.PhaseDegraded, protocol.PhaseBlocked} {
		setPhase(h, phase)
		h.clock.After(time.Second)
		down += 10
		setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 100, Download: down})
		h.e.sampleActivity(ctx)
		if s := summaryNow(t, h, ""); s.Lanes.VPN.Down != 10 {
			t.Fatalf("%s: %+v", phase, s.Lanes)
		}
	}
	read := reads()
	for _, phase := range []string{protocol.PhaseStarting, protocol.PhaseStopping} {
		setPhase(h, phase)
		h.e.sampleActivity(ctx)
		if _, ok := h.e.Activity(""); ok || reads() != read {
			t.Fatalf("%s: a summary, or read", phase)
		}
	}
	setPhase(h, protocol.PhaseOn)
	h.clock.After(time.Second)
	h.e.sampleActivity(ctx)
	if s, ok := h.e.Activity(""); ok {
		t.Fatalf("back in on the tracker starts again, from a baseline: %+v", s.Lanes)
	}
	h.clock.After(time.Second)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 100, Download: down + 7})
	h.e.sampleActivity(ctx)
	if s := summaryNow(t, h, ""); s.Lanes.VPN.Down != 7 {
		t.Fatalf("back in on, a second after the baseline: %+v", s.Lanes)
	}
	if err := call(h, protocol.MethodDisable, nil); err != nil {
		t.Fatal(err)
	}
	h.waitPhase(protocol.PhaseOff)
	read = reads()
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok || reads() != read {
		t.Fatal("after the disable: a summary, or read")
	}
}

// Review F4-2: the first sample of a run, and the first after a gap (the last window went, one came back), is
// a baseline: no summary after it, neither the whole counters of a download under way as one second nor an
// event with every rate 0; the tick after it counts what is new.
func TestActivityStartsFromABaseline(t *testing.T) {
	h := enabled(t)
	windows(h, 1)
	ctx := context.Background()
	const size = 400 << 20 // a download under way: 400 MiB so far
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 100, Download: size})
	h.e.sampleActivity(ctx)
	if s, ok := h.e.Activity(""); ok {
		t.Fatalf("the first sample of the run is a baseline, no summary: %+v", s.Lanes)
	}
	h.clock.After(time.Second)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 150, Download: size + 1000})
	h.e.sampleActivity(ctx)
	if s := summaryNow(t, h, ""); s.Lanes.VPN != (protocol.Traffic{Down: 1000, Up: 50}) {
		t.Fatalf("a second later: %+v", s.Lanes)
	}
	windows(h, 0)
	h.e.sampleActivity(ctx)
	h.clock.After(5 * time.Second)
	windows(h, 1)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 400, Download: size + 6000})
	h.e.sampleActivity(ctx)
	if s, ok := h.e.Activity(""); ok {
		t.Fatalf("a window back after 5 s: a baseline, no summary: %+v", s.Lanes)
	}
	h.clock.After(time.Second)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 410, Download: size + 7000})
	h.e.sampleActivity(ctx)
	if s := summaryNow(t, h, ""); s.Lanes.VPN != (protocol.Traffic{Down: 1000, Up: 10}) {
		t.Fatalf("a second after the baseline: %+v", s.Lanes)
	}
}

// Review F4-1: the name of an unpinned program comes from its file's resources (on a network share a read can
// take the SMB timeout). The tick that reads it holds no lock of the engine's or the activity event's: while
// it waits, the state is there, and so is the summary of the sample before.
func TestActivityDoesNotWaitForADescription(t *testing.T) {
	h := enabled(t)
	windows(h, 1)
	entered, release := make(chan struct{}, 8), make(chan struct{})
	env := testRules
	env.Describe = func(string) string {
		entered <- struct{}{}
		<-release
		return "Curl A"
	}
	h.e.act.tracker = activity.New(env)
	ctx := context.Background()
	setConns(h, Conn{ID: "1", Lane: protocol.LaneVPN, Upload: 1, Download: 1}) // no owner: nothing to describe
	h.e.sampleActivity(ctx)
	h.clock.After(time.Second)
	h.e.sampleActivity(ctx)
	h.clock.After(time.Second)
	setConns(h, Conn{ID: "1", Lane: protocol.LaneVPN, Upload: 1, Download: 1},
		Conn{ID: "2", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 5, Download: 50})
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.e.sampleActivity(ctx)
	}()
	<-entered
	answered := make(chan bool, 1)
	go func() {
		_, ok := h.e.Activity("")
		h.e.State()
		answered <- ok
	}()
	select {
	case ok := <-answered:
		if !ok {
			t.Fatal("while the description is read, the summary of the sample before is offered")
		}
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("Engine.Activity waits for the file description")
	}
	close(release)
	<-done
	if g, ok := appIn(summaryNow(t, h, ""), `C:\Tools\a\`); !ok || g.Name != "Curl A" || g.Down != 50 {
		t.Fatalf("after the description: %+v", g)
	}
}

// Spec 8.4: the tracker starts again with each run of the core. After a restart (5.5) the first sample of
// the new core is a baseline (TestActivityStartsFromABaseline), and a program seen only by the core that is
// gone is not kept: its 5 minutes belonged to that run. The new run has no summary before its first
// sample, nor after that baseline.
func TestActivityResetsOnRestart(t *testing.T) {
	h := enabled(t)
	windows(h, 1)
	ctx := context.Background()
	setConns(h,
		Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 100, Download: 1000},
		Conn{ID: "2", ProcessPath: curlB, Lane: protocol.LaneVPN, Upload: 1, Download: 1})
	h.e.sampleActivity(ctx)
	h.clock.After(time.Second)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 150, Download: 1600})
	h.e.sampleActivity(ctx)
	s := summaryNow(t, h, "")
	if g, ok := appIn(s, `C:\Tools\a\`); !ok || g.Down != 600 || g.Up != 50 {
		t.Fatalf("first run: %+v", s.Apps)
	}
	if _, ok := appIn(s, `C:\Tools\b\`); !ok {
		t.Fatalf("first run: curl-b is kept for 5 minutes: %+v", s.Apps)
	}
	h.core.last().crash()
	h.waitFor("second core on", func() bool {
		return h.rec.count("core:start") == 2 && h.e.State().Phase == protocol.PhaseOn && h.idle()
	})
	if _, ok := h.e.Activity(""); ok {
		t.Fatal("the new run has no summary before its first sample")
	}
	h.clock.After(time.Second)
	h.e.sampleActivity(ctx)
	if _, ok := h.e.Activity(""); ok {
		t.Fatal("the first sample of the new run is a baseline")
	}
	h.clock.After(time.Second)
	setConns(h, Conn{ID: "1", ProcessPath: curlA, Lane: protocol.LaneVPN, Upload: 160, Download: 1700})
	h.e.sampleActivity(ctx)
	s = summaryNow(t, h, "")
	if g, ok := appIn(s, `C:\Tools\a\`); !ok || g.Down != 100 || g.Up != 10 {
		t.Fatalf("the new run counts from its baseline: %+v", s.Apps)
	}
	if _, ok := appIn(s, `C:\Tools\b\`); ok {
		t.Fatalf("a program of the run that is gone is not kept: %+v", s.Apps)
	}
}
