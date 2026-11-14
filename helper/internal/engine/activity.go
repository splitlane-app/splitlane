package engine

import (
	"context"
	"slices"
	"time"

	"splitlane/helper/internal/activity"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
)

// activityEvery: the connections of the core are read once a second (spec 8.4).
const activityEvery = time.Second

// liveActivity is the live activity of spec 8.4: the tracker of the current run of the core and the
// layout its samples are grouped by.
type liveActivity struct {
	tracker *activity.Tracker
	run     *coreRun // guarded by Engine.mu: the run the tracker's samples are of; nil after a reset
	fresh   bool     // guarded by Engine.mu: the last tick sampled run while a window was connected, not as a baseline
	resets  int      // guarded by Engine.mu: the resets of tracker so far (Activity reads it without Engine.mu)

	// Owned by sampleActivity.
	apps   []protocol.AppRule // the layout that set was compiled from
	set    *rules.Set         // nil until the first sample
	failed string             // the last error of the connection list, masked; "" while it works
}

// sampleActivity is one tick of spec 8.4, once a second from Run: while the core with the adapter runs
// (on, degraded, blocked) and a window is connected, the connections of the run go into the tracker,
// grouped by the saved layout (compiled again when it changes), the unpinned programs on the main lane:
// direct in "all direct", else the default lane. The tracker starts again from nothing for each run of
// the core and when the phase leaves on, degraded and blocked. Without a window nothing is read, and
// the summary of the last sample is no longer offered: it would be old when a window comes back. The
// first sample of a run, and the first after a gap (a window back after a while), is a baseline (review
// F4-2): no summary is offered after it, the next tick's counts. A failed read is logged masked, once
// while it repeats; the summary of that tick is not offered either.
func (e *Engine) sampleActivity(ctx context.Context) {
	a := &e.act
	e.mu.Lock()
	r := e.run
	if r == nil || r.stopping || !live(e.st.Phase) {
		if a.run != nil {
			a.reset()
		}
		a.run, a.fresh = nil, false
		e.mu.Unlock()
		return
	}
	if a.run != r {
		a.reset()
		a.run, a.fresh = r, false
	}
	if e.clients.Load() == 0 {
		a.fresh = false
		e.mu.Unlock()
		return
	}
	apps, mainLane := slices.Clone(e.saved.Apps), e.saved.DefaultLane
	if e.st.AllDirect {
		mainLane = protocol.LaneDirect
	}
	e.mu.Unlock()
	if a.set == nil || !slices.Equal(a.apps, apps) {
		set, err := rules.Compile(apps, e.d.Rules)
		if err != nil {
			e.d.Log("activity: the layout does not compile, every program is grouped by its folder: %v", err)
			set = &rules.Set{}
		}
		a.apps, a.set = apps, set
	}
	conns, err := r.clash.Connections(ctx)
	now := e.d.Clock.Now()
	if err != nil {
		e.mu.Lock()
		a.fresh = false
		gone := e.run != r || r.stopping
		e.mu.Unlock()
		if text := protocol.MaskAddresses(err.Error()); !gone && text != a.failed {
			a.failed = text
			e.d.Log("activity: the connections of the core: %s", text)
		}
		return
	}
	a.failed = ""
	sample := make([]activity.Conn, 0, len(conns))
	for _, c := range conns {
		sample = append(sample, activity.Conn{ID: c.ID, ProcessPath: c.ProcessPath, Lane: c.Lane, Host: c.Host,
			Upload: c.Upload, Download: c.Download})
	}
	counted := a.tracker.Sample(now, sample, a.set, mainLane)
	e.mu.Lock()
	a.fresh = e.run == r && counted
	e.mu.Unlock()
}

// reset starts the tracker again from nothing; Engine.mu must be held.
func (a *liveActivity) reset() {
	a.tracker.Reset()
	a.resets++
}

// Activity is the summary of the last sample (spec 8.4) with the detail of the group detailFor ("" for
// none): the data of the activity event. false while no core with the adapter runs (any phase but on,
// degraded and blocked), and unless the last tick sampled the run with a window connected and counted
// its rates (not a baseline: the run's first sample, the first after a gap). The summary is
// taken without e.mu, under the tracker's own lock (review F4-1: nothing of the engine waits for it);
// the run is checked before and again after it, and a reset in between (a new run of the core) gives
// false, never the empty tracker of the new run.
func (e *Engine) Activity(detailFor string) (protocol.Activity, bool) {
	e.mu.Lock()
	ok, resets := e.activityLive(), e.act.resets
	e.mu.Unlock()
	if !ok {
		return protocol.Activity{}, false
	}
	s := e.act.tracker.Summary(detailFor)
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.activityLive() || e.act.resets != resets {
		return protocol.Activity{}, false
	}
	return s, true
}

// activityLive: the core with the adapter runs and the last tick sampled its run with a window connected;
// e.mu must be held.
func (e *Engine) activityLive() bool {
	r := e.run
	return r != nil && !r.stopping && r == e.act.run && e.act.fresh && live(e.st.Phase)
}
