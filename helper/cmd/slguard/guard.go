package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Image names the guard ends. TailscaleCLI is only the command line program: the service is
// tailscaled.exe and the tray app tailscale-ipn.exe, the guard never touches them.
const (
	HelperImage  = "splitlane-helper.exe"
	CoreImage    = "sing-box.exe"
	TailscaleCLI = "tailscale.exe"
)

// Times of the restore: the exit node must read on settleReads times in a row, settleEvery
// apart, within settleFor; before the core is ended and again after.
const (
	settleFor   = 60 * time.Second
	settleEvery = 2 * time.Second
	settleReads = 3
)

// No VPN path at all: the system exit node off and no sing-box.exe. No step of a live run
// passes through this state (5.3 starts the core before the exit node goes off, 5.4 restores
// before it stops the core, a killed helper leaves its core running), so it trips the guard
// once it lasts noPathGrace; it is read every pathEvery ticks.
const (
	noPathGrace = 10 * time.Second
	pathEvery   = 2
)

// System is everything the guard does to Windows; the tests replace it.
type System interface {
	ExitNode() (on bool, ip string, err error) // tailscale status --json
	AllowLAN() (bool, error)                   // tailscale debug prefs
	SetExitNode(ip string, allowLAN bool) error
	Kill(image string) []uint32                      // ends every process with this image name, returns their ids
	KillChildren(parents []uint32, image string) int // ends the processes of image that one of parents started
	Pids(image string) []uint32                      // the ids of the running processes with this image name, nothing ended
	Count(image string) int                          // running processes with this image name
	AdapterPresent() bool                            // the SplitLane adapter
	Launch(command []string) error
}

// Guard is the dead-man switch. Files in Dir: deadline.txt (unix seconds, written by the
// developer's assistant), stop.flag, cmd.json, status.json, guard-state.json, guard.log.
type Guard struct {
	Dir     string
	Sys     System
	Now     func() time.Time
	Sleep   func(time.Duration)
	Launch  []string      // the helper's command line; empty: the guard starts nothing
	HardCap time.Duration // the guard trips after this long whatever happens
	MaxLead time.Duration // a deadline never reaches further than this past its own write
	Tick    time.Duration
	RunID   string

	exitIP   string
	allowLAN bool
	started  time.Time
	deadline time.Time
	seq      int
	ticks    int
	noPathAt time.Time // first read of the current run of reads without a VPN path
}

func (g *Guard) path(name string) string { return filepath.Join(g.Dir, name) }

func (g *Guard) logf(format string, args ...any) {
	f, err := os.OpenFile(g.path("guard.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", g.Now().Format("15:04:05"), fmt.Sprintf(format, args...))
}

// A reader that has the target open (PowerShell polls status.json every 500 ms) makes the rename
// fail with "Access is denied" until it lets go. So a write is tried again: writeAttempts times,
// retrySleep apart (a variable so that the tests do not wait), like Write-P1Atomic in p1lib.ps1.
const (
	writeAttempts   = 20
	writeRetryEvery = 100 * time.Millisecond
)

var retrySleep = time.Sleep

// writeAtomic: temp file in the same folder, then rename (the reader never sees half a file).
// Every attempt writes the temp file anew; the last error comes back and no temp file stays.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	var err error
	for attempt := 1; attempt <= writeAttempts; attempt++ {
		if attempt > 1 {
			retrySleep(writeRetryEvery)
		}
		if err = os.WriteFile(tmp, data, 0o644); err != nil {
			continue
		}
		if err = os.Rename(tmp, path); err == nil {
			return nil
		}
	}
	os.Remove(tmp)
	return fmt.Errorf("%w (%d attempts)", err, writeAttempts)
}

func (g *Guard) status(state string, fields map[string]any) {
	g.seq++
	s := map[string]any{"run": g.RunID, "seq": g.seq, "state": state, "time": g.Now().Unix()}
	for k, v := range fields {
		s[k] = v
	}
	b, _ := json.Marshal(s)
	if err := writeAtomic(g.path("status.json"), b); err != nil {
		g.logf("status not written (state %s, seq %d): %v", state, g.seq, maskIP(err.Error()))
		return
	}
	g.logf("status %s", maskIP(string(b)))
}

// readDeadline: the last good value stays when the file is half written; never later than mtime+MaxLead.
func (g *Guard) readDeadline() (time.Time, bool) {
	p := g.path("deadline.txt")
	b, err := os.ReadFile(p)
	fi, statErr := os.Stat(p)
	if err == nil && statErr == nil {
		if n, perr := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); perr == nil {
			d := time.Unix(n, 0)
			if limit := fi.ModTime().Add(g.MaxLead); d.After(limit) {
				d = limit
			}
			g.deadline = d
		}
	}
	return g.deadline, !g.deadline.IsZero()
}

// arm checks the starting point: a fresh heartbeat, no core and no helper, the system exit node on.
// It returns the reason of a refusal, or "".
func (g *Guard) arm() string {
	if d, ok := g.readDeadline(); !ok || !d.After(g.Now()) {
		return "heartbeat missing or stale: refresh deadline.txt first"
	}
	if g.Sys.Count(CoreImage) > 0 {
		return "a sing-box.exe is running"
	}
	if g.Sys.Count(HelperImage) > 0 {
		return "a splitlane-helper.exe is running"
	}
	on, ip, err := g.Sys.ExitNode()
	if err != nil || !on || ip == "" {
		return fmt.Sprintf("the system exit node is not on (%v)", err)
	}
	lan, err := g.Sys.AllowLAN()
	if err != nil {
		return fmt.Sprintf("the LAN access flag of the system client is unknown (%v)", err)
	}
	g.exitIP, g.allowLAN = ip, lan
	b, _ := json.Marshal(map[string]any{"exitNode": ip, "allowLan": lan})
	if err := writeAtomic(g.path("guard-state.json"), b); err != nil {
		return fmt.Sprintf("guard-state.json not written: %v", err)
	}
	return ""
}

// Run arms the guard and watches until it trips or finishes; it returns the final state.
func (g *Guard) Run() string {
	g.started = g.Now()
	os.Remove(g.path("stop.flag"))
	os.Remove(g.path("cmd.json"))
	g.status("starting", nil)
	if why := g.arm(); why != "" {
		g.status("refused", map[string]any{"reason": why})
		return "refused"
	}
	g.status("armed", nil)
	if len(g.Launch) > 0 {
		if err := g.Sys.Launch(g.Launch); err != nil {
			return g.trip(fmt.Sprintf("helper launch failed: %v", err))
		}
		g.logf("helper launched")
	}
	for {
		g.Sleep(g.Tick)
		now := g.Now()
		if _, err := os.Stat(g.path("stop.flag")); err == nil {
			return g.trip("stop requested")
		}
		if d, _ := g.readDeadline(); !now.Before(d) {
			return g.trip("heartbeat deadline expired")
		}
		if now.Sub(g.started) > g.HardCap {
			return g.trip("hard time cap reached")
		}
		if final := g.command(); final != "" {
			return final
		}
		if g.ticks++; g.ticks%pathEvery == 0 && g.noVPNPath(now) {
			return g.trip("no VPN path: the system exit node is off and no sing-box.exe runs")
		}
	}
}

// noVPNPath reports a lasting state without any VPN path: the exit node off and no core for at
// least noPathGrace. A read that fails changes nothing.
func (g *Guard) noVPNPath(now time.Time) bool {
	on, _, err := g.Sys.ExitNode()
	if err != nil {
		return false
	}
	if on || g.Sys.Count(CoreImage) > 0 {
		g.noPathAt = time.Time{}
		return false
	}
	if g.noPathAt.IsZero() {
		g.noPathAt = now
		g.logf("no VPN path: the system exit node is off and no sing-box.exe runs")
	}
	return now.Sub(g.noPathAt) >= noPathGrace
}

// command runs one command from cmd.json: kill-helper, kill-worker, kill-core, launch-helper, finish.
func (g *Guard) command() string {
	b, err := os.ReadFile(g.path("cmd.json"))
	if err != nil {
		return ""
	}
	var cmd struct {
		Action string `json:"action"`
		ID     string `json:"id"`
	}
	if json.Unmarshal(b, &cmd) != nil {
		return "" // half written: read it again on the next tick
	}
	if os.Remove(g.path("cmd.json")) != nil {
		return "" // never run a command twice
	}
	g.logf("command %s id=%s", cmd.Action, cmd.ID)
	switch cmd.Action {
	case "kill-helper":
		n := len(g.Sys.Kill(HelperImage))
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": fmt.Sprintf("helper processes ended: %d", n)})
	case "kill-worker":
		// The product helper is a supervisor and its worker copy (spec 4): the worker is the helper whose
		// parent is a helper. The supervisor stays and starts a new worker.
		n := g.Sys.KillChildren(g.Sys.Pids(HelperImage), HelperImage)
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": fmt.Sprintf("worker processes ended: %d", n)})
	case "kill-core":
		// A crash of the core while the helper runs (spec 5.5, 12): the helper gives the exit node back and restarts.
		n := len(g.Sys.Kill(CoreImage))
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": fmt.Sprintf("core processes ended: %d", n)})
	case "launch-helper":
		result := "helper launched"
		if len(g.Launch) == 0 {
			result = "no helper command line"
		} else if err := g.Sys.Launch(g.Launch); err != nil {
			result = "launch failed: " + err.Error()
		}
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "result": result})
	case "finish":
		if g.Sys.Count(HelperImage) == 0 && g.systemOK() {
			g.status("finished", map[string]any{"cmd_id": cmd.ID, "system_ok": true})
			return "finished"
		}
		return g.trip("finish found the system not restored or the helper still running")
	default:
		g.status("armed", map[string]any{"cmd_id": cmd.ID, "cmd_error": "unknown action"})
	}
	return ""
}

func (g *Guard) systemOK() bool {
	on, _, err := g.Sys.ExitNode()
	return err == nil && on && g.Sys.Count(CoreImage) == 0 && !g.Sys.AdapterPresent()
}

// trip puts the system back. The helper goes first: it is the one that switches the exit node off.
// A `tailscale.exe set --exit-node=` it had just started is not in a job and outlives it: it goes
// too. Then the exit node, confirmed, tried every 5 s without end. A command that landed late is
// caught while the core still runs (the exit node on with the adapter up blocks traffic, it does
// not leak): the exit node must read on three times in a row before the core is ended, and again
// after; system_ok needs those reads, no core and no adapter.
func (g *Guard) trip(reason string) string {
	g.status("tripping", map[string]any{"reason": reason})
	helpers := g.Sys.Kill(HelperImage)
	g.logf("helper processes ended: %d", len(helpers))
	if len(helpers) > 0 {
		g.logf("tailscale.exe started by the helper ended: %d", g.Sys.KillChildren(helpers, TailscaleCLI))
	}
	g.restoreExitNode(reason)
	g.settle(reason, "before the core was ended")
	g.logf("core processes ended: %d", len(g.Sys.Kill(CoreImage)))
	for i := 0; i < 30 && g.Sys.AdapterPresent(); i++ {
		g.Sleep(500 * time.Millisecond)
	}
	ok := g.settle(reason, "after the core was ended") && g.systemOK()
	g.status("tripped", map[string]any{"reason": reason, "system_ok": ok})
	return "tripped"
}

// settle reads the exit node every 2 s until it reads on three times in a row (at most 60 s);
// an off reading restores it again. False when the reads did not settle.
func (g *Guard) settle(reason, when string) bool {
	good := 0
	for end := g.Now().Add(settleFor); good < settleReads && g.Now().Before(end); {
		g.Sleep(settleEvery)
		if on, _, err := g.Sys.ExitNode(); err == nil && on {
			good++
			continue
		}
		good = 0
		g.logf("the exit node went off %s: restoring it again", when)
		g.restoreExitNode(reason)
	}
	return good >= settleReads
}

// restoreExitNode sets the exit node back and returns only after `tailscale status` confirms it;
// attempts every 5 s without end (status restore_blocked with the manual command meanwhile).
func (g *Guard) restoreExitNode(reason string) {
	manual := fmt.Sprintf("tailscale set --exit-node=%s --exit-node-allow-lan-access=%v", g.exitIP, g.allowLAN)
	for attempt := 1; ; attempt++ {
		if on, _, err := g.Sys.ExitNode(); err == nil && on {
			return
		}
		g.logf("set the exit node back (attempt %d)", attempt)
		err := g.Sys.SetExitNode(g.exitIP, g.allowLAN)
		if on, _, serr := g.Sys.ExitNode(); serr == nil && on {
			return
		}
		g.status("restore_blocked", map[string]any{"reason": reason, "attempt": attempt, "error": fmt.Sprint(err), "manual": manual})
		g.Sleep(5 * time.Second)
	}
}

var ipv4 = regexp.MustCompile(`\b\d{1,3}(\.\d{1,3}){3}\b`)

// maskIP keeps addresses out of the log.
func maskIP(s string) string { return ipv4.ReplaceAllString(s, "<ip>") }
