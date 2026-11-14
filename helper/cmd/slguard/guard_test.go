package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSystem: the exit node is off while "routing is on"; the processes are counters.
type fakeSystem struct {
	mu             sync.Mutex
	events         []string
	exitOn         bool
	exitIP         string
	ignoreSet      int // so many SetExitNode calls change nothing
	helpers        int
	cores          int
	adapter        bool
	launchErr      error
	loseOnCoreKill bool             // a late `set --exit-node=` of the helper lands just when the core is ended
	clock          func() time.Time // the guard's virtual clock, for the time of the trip
	helperKilledAt time.Time        // the first `kill splitlane-helper.exe` on that clock
}

func (f *fakeSystem) add(s string) { f.events = append(f.events, s) }

func (f *fakeSystem) ExitNode() (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.exitOn {
		return true, f.exitIP, nil
	}
	return false, "", nil
}

func (f *fakeSystem) AllowLAN() (bool, error) { return true, nil }

func (f *fakeSystem) SetExitNode(ip string, lan bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add(fmt.Sprintf("set %s %v", ip, lan))
	if f.ignoreSet > 0 {
		f.ignoreSet--
		return errors.New("tailscaled not ready")
	}
	f.exitOn, f.exitIP = true, ip
	return nil
}

func ids(n int) []uint32 {
	out := []uint32{}
	for i := 0; i < n; i++ {
		out = append(out, uint32(100+i))
	}
	return out
}

func (f *fakeSystem) Kill(image string) []uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add("kill " + image)
	switch image {
	case HelperImage:
		if f.clock != nil && f.helperKilledAt.IsZero() {
			f.helperKilledAt = f.clock()
		}
		n := f.helpers
		f.helpers = 0
		return ids(n)
	case CoreImage:
		n := f.cores
		f.cores, f.adapter = 0, false
		if f.loseOnCoreKill {
			f.exitOn, f.loseOnCoreKill = false, false
		}
		return ids(n)
	}
	return nil
}

func (f *fakeSystem) KillChildren(parents []uint32, image string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add(fmt.Sprintf("kill %s children of %d", image, len(parents)))
	return 0
}

func (f *fakeSystem) Count(image string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	if image == HelperImage {
		return f.helpers
	}
	return f.cores
}

func (f *fakeSystem) AdapterPresent() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.adapter
}

func (f *fakeSystem) Launch(cmd []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.add("launch " + strings.Join(cmd, " "))
	if f.launchErr != nil {
		return f.launchErr
	}
	f.helpers = 1
	return nil
}

func (f *fakeSystem) list() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.events, "|")
}

// newGuard: virtual time that moves only in Sleep; script runs at every tick (it plays the world).
func newGuard(t *testing.T, sys *fakeSystem, script func(g *Guard, tick int)) *Guard {
	t.Helper()
	dir := t.TempDir()
	now := time.Now()
	g := &Guard{Dir: dir, Sys: sys, HardCap: time.Hour, MaxLead: 900 * time.Second, Tick: 2 * time.Second, RunID: "test"}
	g.Now = func() time.Time { return now }
	tick := 0
	g.Sleep = func(d time.Duration) {
		now = now.Add(d)
		tick++
		if script != nil {
			script(g, tick)
		}
		if tick > 100000 {
			t.Fatal("the guard never ended")
		}
	}
	heartbeat(g, 400)
	return g
}

// heartbeat writes deadline.txt as the assistant does; its modification time follows the virtual clock.
func heartbeat(g *Guard, seconds int) {
	p := filepath.Join(g.Dir, "deadline.txt")
	os.WriteFile(p, []byte(fmt.Sprint(g.Now().Unix()+int64(seconds))), 0o644)
	os.Chtimes(p, g.Now(), g.Now())
}

func command(g *Guard, action string) {
	os.WriteFile(filepath.Join(g.Dir, "cmd.json"), []byte(`{"action":"`+action+`","id":"c1"}`), 0o644)
}

func lastStatus(t *testing.T, g *Guard) map[string]any {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(g.Dir, "status.json"))
	if err != nil {
		t.Fatal(err)
	}
	var s map[string]any
	json.Unmarshal(b, &s)
	return s
}

// engaged: what the helper does after the launch: a core, the adapter, the exit node off.
func engaged(sys *fakeSystem) func(g *Guard, tick int) {
	return func(g *Guard, tick int) {
		if tick == 1 {
			sys.mu.Lock()
			sys.cores, sys.adapter, sys.exitOn = 1, true, false
			sys.mu.Unlock()
		}
	}
}

func TestArmRefusals(t *testing.T) {
	cases := map[string]func(s *fakeSystem, g *Guard){
		"no heartbeat":   func(s *fakeSystem, g *Guard) { os.Remove(filepath.Join(g.Dir, "deadline.txt")) },
		"old heartbeat":  func(s *fakeSystem, g *Guard) { heartbeat(g, -1) },
		"core running":   func(s *fakeSystem, g *Guard) { s.cores = 1 },
		"helper running": func(s *fakeSystem, g *Guard) { s.helpers = 1 },
		"exit node off":  func(s *fakeSystem, g *Guard) { s.exitOn = false },
	}
	for name, change := range cases {
		sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
		g := newGuard(t, sys, nil)
		g.Launch = []string{"helper.exe"}
		change(sys, g)
		if got := g.Run(); got != "refused" {
			t.Errorf("%s: %s", name, got)
		}
		if strings.Contains(sys.list(), "kill") || strings.Contains(sys.list(), "set") || strings.Contains(sys.list(), "launch") {
			t.Errorf("%s: a refusal changes nothing: %s", name, sys.list())
		}
	}
}

// A stale heartbeat trips: helper first, then the exit node (confirmed), then the core.
func TestStaleHeartbeatTrips(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, engaged(sys))
	g.Launch = []string{"helper.exe", "--dev"}
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	want := "launch helper.exe --dev|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true|kill sing-box.exe"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
	s := lastStatus(t, g)
	if s["state"] != "tripped" || s["reason"] != "heartbeat deadline expired" || s["system_ok"] != true {
		t.Fatalf("status %v", s)
	}
}

// A `set --exit-node=` the helper started lands after the core was ended: the exit node goes off
// with no core. The guard sees it, sets the exit node again and only then reports the system ok.
func TestExitNodeLostAfterCoreKillIsSetAgain(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1", loseOnCoreKill: true}
	g := newGuard(t, sys, engaged(sys))
	g.Launch = []string{"helper.exe"}
	os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	want := "launch helper.exe|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true|kill sing-box.exe|set 100.64.0.1 true"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
	if s := lastStatus(t, g); s["state"] != "tripped" || s["system_ok"] != true {
		t.Fatalf("status %v", s)
	}
}

// A late `set --exit-node=` lands while the guard checks the exit node and the core still runs:
// the exit node is set again before the core is ended (blocked traffic, not a leak).
func TestExitNodeLostBeforeCoreKillIsSetFirst(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		engaged(sys)(g, tick)
		switch tick {
		case 1:
			os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
		case 2: // the first read of the trip after the restore: the core still runs
			sys.mu.Lock()
			sys.exitOn = false
			sys.mu.Unlock()
		}
	})
	g.Launch = []string{"helper.exe"}
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	want := "launch helper.exe|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true|set 100.64.0.1 true|kill sing-box.exe"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
	if s := lastStatus(t, g); s["state"] != "tripped" || s["system_ok"] != true {
		t.Fatalf("status %v", s)
	}
}

// No VPN path at all (the exit node off and no core) trips once it lasted 10 s, not before; the
// exit node off while the core runs (a killed helper, Task 23 step 7) does not.
func TestNoVPNPathTrips(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	var goneAt time.Time
	g := newGuard(t, sys, func(g *Guard, tick int) {
		if tick == 2 { // a read tick (pathEvery): the path is gone when the grace starts to count
			sys.mu.Lock()
			sys.exitOn = false // the core is gone, the exit node stayed off
			sys.mu.Unlock()
			goneAt = g.Now()
		}
	})
	sys.clock = g.Now
	g.Launch = []string{"helper.exe"}
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	s := lastStatus(t, g)
	if !strings.HasPrefix(fmt.Sprint(s["reason"]), "no VPN path") || s["system_ok"] != true {
		t.Fatalf("status %v", s)
	}
	// the trip starts with the helper's end: not before the path was gone for 10 s (the literal,
	// so that a shorter noPathGrace fails too)
	if after := sys.helperKilledAt.Sub(goneAt); goneAt.IsZero() || after < 10*time.Second || after > time.Minute {
		t.Fatalf("tripped %s after the VPN path was gone", after)
	}
	if !strings.HasPrefix(sys.list(), "launch helper.exe|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true") {
		t.Fatalf("events %s", sys.list())
	}

	sys = &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g = newGuard(t, sys, func(g *Guard, tick int) {
		engaged(sys)(g, tick) // a core runs, the exit node is off
		if tick == 60 {
			os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
		}
	})
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	if s := lastStatus(t, g); s["reason"] != "stop requested" {
		t.Fatalf("the exit node off with a core running is no trip: %v", s)
	}
}

// The process filters of the real system: by image name, and the children of ended helpers.
func TestProcessFilters(t *testing.T) {
	ps := []proc{
		{pid: 10, parent: 1, image: "splitlane-helper.exe"},
		{pid: 11, parent: 10, image: "tailscale.exe"},
		{pid: 12, parent: 77, image: "tailscale.exe"},
		{pid: 13, parent: 10, image: "sing-box.exe"},
		{pid: 14, parent: 1, image: "TAILSCALE.EXE"},
	}
	if got := fmt.Sprint(withImage(ps, "tailscale.exe")); got != "[11 12 14]" {
		t.Fatalf("by image: %s", got)
	}
	if got := fmt.Sprint(childrenOf(ps, []uint32{10}, "tailscale.exe")); got != "[11]" {
		t.Fatalf("children: %s", got)
	}
}

// %ProgramFiles% from the user's environment never decides which tailscale.exe runs elevated.
func TestTailscalePathIgnoresTheEnvironment(t *testing.T) {
	user := t.TempDir()
	t.Setenv("ProgramFiles", user)
	if exe := newSystem().tailscale; !filepath.IsAbs(exe) || strings.HasPrefix(strings.ToLower(exe), strings.ToLower(user)) {
		t.Fatalf("the path follows %%ProgramFiles%%: %s", exe)
	}
}

// The core is never ended before the exit node is confirmed; attempts go on every 5 s.
func TestRestoreRetriesBeforeTheCoreGoes(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1", ignoreSet: 3}
	g := newGuard(t, sys, engaged(sys))
	os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
	want := "kill splitlane-helper.exe|set 100.64.0.1 true|set 100.64.0.1 true|set 100.64.0.1 true|set 100.64.0.1 true|kill sing-box.exe"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
}

// A heartbeat that is refreshed keeps the guard armed; finish ends it when the system is back.
func TestHeartbeatAndFinish(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		if tick%100 == 0 {
			heartbeat(g, 400)
		}
		if tick == 1000 {
			command(g, "finish")
		}
	})
	if got := g.Run(); got != "finished" {
		t.Fatalf("%s, events %s", got, sys.list())
	}
	if strings.Contains(sys.list(), "kill") || strings.Contains(sys.list(), "set") {
		t.Fatalf("nothing to do: %s", sys.list())
	}
}

// finish while the core still runs is a trip, not a quiet end.
func TestFinishWithCoreRunningTrips(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		if tick == 1 {
			sys.mu.Lock()
			sys.cores, sys.adapter = 1, true
			sys.mu.Unlock()
			command(g, "finish")
		}
	})
	if got := g.Run(); got != "tripped" || !strings.HasSuffix(sys.list(), "kill sing-box.exe") {
		t.Fatalf("%s %s", got, sys.list())
	}
}

// kill-helper simulates a failure of the helper; launch-helper starts it again; the core stays.
func TestKillAndLaunchCommands(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		switch tick {
		case 1:
			sys.mu.Lock()
			sys.cores, sys.adapter, sys.exitOn = 1, true, false
			sys.mu.Unlock()
			command(g, "kill-helper")
		case 2:
			command(g, "launch-helper")
		case 3:
			os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
		}
	})
	g.Launch = []string{"helper.exe"}
	g.Run()
	want := "launch helper.exe|kill splitlane-helper.exe|launch helper.exe|kill splitlane-helper.exe|kill tailscale.exe children of 1|set 100.64.0.1 true|kill sing-box.exe"
	if sys.list() != want {
		t.Fatalf("events %s", sys.list())
	}
}

// A deadline far in the future counts only up to 900 s after the file was written.
func TestDeadlineIsCapped(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, nil)
	heartbeat(g, 100000)
	start := g.Now()
	g.Run()
	if waited := g.Now().Sub(start); waited < 890*time.Second || waited > 930*time.Second { // the trip reads for 12 s
		t.Fatalf("tripped after %s", waited)
	}
}

func TestHardCap(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1"}
	g := newGuard(t, sys, func(g *Guard, tick int) {
		if tick%100 == 0 {
			heartbeat(g, 400)
		}
	})
	g.HardCap = 10 * time.Minute
	g.Run()
	if s := lastStatus(t, g); s["reason"] != "hard time cap reached" {
		t.Fatalf("status %v", s)
	}
}

func TestLaunchFailureTrips(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1", launchErr: errors.New("no file")}
	g := newGuard(t, sys, nil)
	g.Launch = []string{"missing.exe"}
	if got := g.Run(); got != "tripped" {
		t.Fatal(got)
	}
}

func TestLogMasksAddresses(t *testing.T) {
	sys := &fakeSystem{exitOn: true, exitIP: "100.64.0.1", ignoreSet: 1}
	g := newGuard(t, sys, engaged(sys))
	os.WriteFile(filepath.Join(g.Dir, "stop.flag"), nil, 0o644)
	g.Run()
	b, _ := os.ReadFile(filepath.Join(g.Dir, "guard.log"))
	if strings.Contains(string(b), "100.64.0.1") || !strings.Contains(string(b), "<ip>") {
		t.Fatalf("log:\n%s", b)
	}
}

// holdOpen opens the file the way PowerShell's Get-Content does: the rename of another file over
// it then fails with "Access is denied" until the handle is closed.
func holdOpen(t *testing.T, path string) *os.File {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// A reader that has status.json open at that moment must not lose the write: the guard tries
// again, as Write-P1Atomic does (the terminal state is what Stop-P1Guard and Send-P1Guard wait for).
func TestStatusWriteRetriesWhileAReaderHoldsTheFile(t *testing.T) {
	g := newGuard(t, &fakeSystem{}, nil)
	g.status("armed", nil)
	held := holdOpen(t, filepath.Join(g.Dir, "status.json"))
	sleeps := 0
	old := retrySleep
	retrySleep = func(time.Duration) {
		if sleeps++; sleeps == 3 {
			held.Close() // the reader is done after the third failed attempt
		}
	}
	t.Cleanup(func() { retrySleep = old })

	g.status("tripped", map[string]any{"reason": "test", "system_ok": true})

	if s := lastStatus(t, g); s["state"] != "tripped" || s["system_ok"] != true {
		t.Fatalf("status.json after the retries: %v", s)
	}
	if sleeps != 3 {
		t.Fatalf("slept %d times between attempts, want 3", sleeps)
	}
	if _, err := os.Stat(filepath.Join(g.Dir, "status.json.tmp")); err == nil {
		t.Fatal("the temp file is left behind")
	}
	b, _ := os.ReadFile(filepath.Join(g.Dir, "guard.log"))
	if strings.Contains(string(b), "not written") || !strings.Contains(string(b), `"state":"tripped"`) {
		t.Fatalf("log:\n%s", b)
	}
}

// When every attempt fails the guard says so and does not log the state as if it were written.
func TestStatusWriteFailureIsLoggedHonestly(t *testing.T) {
	g := newGuard(t, &fakeSystem{}, nil)
	g.status("armed", nil)
	holdOpen(t, filepath.Join(g.Dir, "status.json"))
	sleeps := 0
	old := retrySleep
	retrySleep = func(time.Duration) { sleeps++ }
	t.Cleanup(func() { retrySleep = old })

	g.status("tripped", map[string]any{"reason": "test", "system_ok": true})

	if s := lastStatus(t, g); s["state"] != "armed" {
		t.Fatalf("status.json changed although the rename never worked: %v", s)
	}
	if sleeps != writeAttempts-1 {
		t.Fatalf("slept %d times, want %d (%d attempts)", sleeps, writeAttempts-1, writeAttempts)
	}
	if _, err := os.Stat(filepath.Join(g.Dir, "status.json.tmp")); err == nil {
		t.Fatal("the temp file is left behind")
	}
	b, _ := os.ReadFile(filepath.Join(g.Dir, "guard.log"))
	if !strings.Contains(string(b), "status not written") || !strings.Contains(string(b), "tripped") ||
		strings.Contains(string(b), `"state":"tripped"`) {
		t.Fatalf("log:\n%s", b)
	}
}
