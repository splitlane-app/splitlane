package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
)

// Addresses of the fakes (documentation ranges). The exit node lives in the tailnet range.
const (
	exitNode   = "100.64.0.1"
	vpnAddr    = "203.0.113.10"
	directAddr = "198.51.100.20"
	portVPN    = 40001
	portDirect = 40002
)

// recorder keeps one ordered list of everything the fakes were asked to do.
type recorder struct {
	mu     sync.Mutex
	events []string
}

func (r *recorder) add(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, fmt.Sprintf(format, args...))
}

func (r *recorder) list() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string{}, r.events...)
}

// index of the first event that starts with prefix, or -1.
func (r *recorder) index(prefix string) int {
	for i, ev := range r.list() {
		if strings.HasPrefix(ev, prefix) {
			return i
		}
	}
	return -1
}

func (r *recorder) count(prefix string) int {
	n := 0
	for _, ev := range r.list() {
		if strings.HasPrefix(ev, prefix) {
			n++
		}
	}
	return n
}

// fakeClock never waits: After moves the time on and fires at once (after a real millisecond,
// so that a loop that waits for the user does not spin).
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.mu.Lock()
	c.now = c.now.Add(d)
	t := c.now
	c.mu.Unlock()
	time.Sleep(time.Millisecond)
	ch := make(chan time.Time, 1)
	ch <- t
	return ch
}

type fakeStore struct {
	mu              sync.Mutex
	rec             *recorder
	state           store.State
	journal         *store.Journal
	direct          []byte
	vpn             []byte
	config          []byte
	rulesErr        error
	journalErr      error
	onRules         func()  // runs after each write of the rule files
	stateReadFails  int     // so many next LoadState calls fail with a read error (store.ErrIO)
	journalReadErrs []error // the next LoadJournal calls fail with these, one by one
	saveErr         error   // SaveState fails with it; state.json keeps what it had
	stateReads      int     // calls of LoadState
	stateSaves      int     // calls of SaveState
	journalReads    int     // calls of LoadJournal
}

// readErr is how the store reports a file that is there but could not be read (store.ErrIO).
func readErr(file string) error {
	return fmt.Errorf("%w: open C:\\data\\%s: The process cannot access the file because it is being used by another process.", store.ErrIO, file)
}

// calls gives how often state.json was read and written and restore.json was read.
func (s *fakeStore) calls() (stateReads, stateSaves, journalReads int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stateReads, s.stateSaves, s.journalReads
}

func (s *fakeStore) LoadState() (store.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateReads++
	if s.stateReadFails > 0 {
		s.stateReadFails--
		return store.Defaults(), readErr("state.json")
	}
	return s.state, nil
}

func (s *fakeStore) SaveState(st store.State) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateSaves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.state = st
	return nil
}

func (s *fakeStore) LoadJournal() (*store.Journal, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.journalReads++
	if len(s.journalReadErrs) > 0 {
		err := s.journalReadErrs[0]
		s.journalReadErrs = s.journalReadErrs[1:]
		return nil, err
	}
	if s.journal == nil {
		return nil, nil
	}
	j := *s.journal
	return &j, nil
}

func (s *fakeStore) SaveJournal(j store.Journal) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journalErr != nil {
		return s.journalErr
	}
	s.rec.add("journal:save %s %v", j.ExitNode, j.AllowLAN)
	s.journal = &j
	return nil
}

func (s *fakeStore) DeleteJournal() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.add("journal:delete")
	s.journal = nil
	return nil
}

func (s *fakeStore) hasJournal() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.journal != nil
}

func (s *fakeStore) WriteRules(direct, vpn []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.rulesErr != nil {
		return s.rulesErr
	}
	s.rec.add("rules:write")
	s.direct, s.vpn = direct, vpn
	if s.onRules != nil {
		s.onRules()
	}
	return nil
}

func (s *fakeStore) WriteConfig(config []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = config
	return `C:\data\core-config.json`, nil
}

func (s *fakeStore) saved() store.State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

type fakeProc struct {
	rec             *recorder
	id              store.CoreIdentity
	done            chan struct{}
	once            sync.Once
	ignoreInterrupt bool
	interruptErr    error
	setup           bool // a core of the setup mode (spec 7): started from the setup config
}

func (p *fakeProc) Identity() store.CoreIdentity { return p.id }
func (p *fakeProc) Done() <-chan struct{}        { return p.done }
func (p *fakeProc) exit()                        { p.once.Do(func() { close(p.done) }) }
func (p *fakeProc) alive() bool                  { return !exited(p) }

func (p *fakeProc) Interrupt() error {
	p.rec.add("core:interrupt %d", p.id.PID)
	if p.interruptErr != nil {
		return p.interruptErr
	}
	if !p.ignoreInterrupt {
		p.exit()
	}
	return nil
}

func (p *fakeProc) Kill() error {
	p.rec.add("core:kill %d", p.id.PID)
	p.exit()
	return nil
}

// crash ends the process as if it failed on its own.
func (p *fakeProc) crash() {
	p.rec.add("core:crash %d", p.id.PID)
	p.exit()
}

// fakeCore is the core driver of the tests. It speaks like the sing-box driver (modes, log lines
// and rule files from coreconfig and rules); its processes are fakeProc and its config is the
// options as JSON, so the tests see what the engine asked for.
type fakeCore struct {
	mu         sync.Mutex
	rec        *recorder
	checkErr   error
	startErr   error
	procs      []*fakeProc
	log        string
	silent     int  // so many next starts never write the start line
	dieEarly   int  // so many next starts exit before the start line
	needLogin  bool // the node asks for a login after start
	leftover   *fakeProc
	clash      *fakeClash
	onStart    func(p *fakeProc)
	opts       CoreOptions  // of the last BuildConfig
	setupOpts  SetupOptions // of the last BuildSetupConfig
	node       *fakeNode    // the API service of every run
	setupNext  bool         // the last config built was the setup mode's: the next start is a setup core
	nodePort   int          // the port of the last call of Node
	nodeSecret string       // the secret of the last call of Node
}

func (c *fakeCore) BuildConfig(o CoreOptions) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.opts = o
	c.setupNext = false
	return json.Marshal(o)
}

func (c *fakeCore) options() CoreOptions {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.opts
}

func (c *fakeCore) LaneFiles(apps []protocol.AppRule, env rules.Env) ([]byte, []byte, error) {
	return rules.Files(apps, env)
}

func (c *fakeCore) Mode(defaultLane string, allDirect, degraded bool) string {
	return coreconfig.ModeFor(defaultLane, allDirect, degraded)
}

func (c *fakeCore) Events(offset int64) CoreEvents {
	text := c.logFrom(offset)
	return CoreEvents{
		Started:      strings.Contains(text, coreconfig.StartLine),
		NeedLogin:    strings.Contains(text, coreconfig.LoginLine),
		ReloadFailed: strings.Contains(text, coreconfig.ReloadErrorLine),
	}
}

func (c *fakeCore) Check(ctx context.Context, path string) error {
	c.rec.add("core:check")
	return c.checkErr
}

func (c *fakeCore) Start(path string) (CoreProc, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.startErr != nil {
		return nil, c.startErr
	}
	p := &fakeProc{rec: c.rec, id: store.CoreIdentity{PID: 1000 + len(c.procs), Created: int64(len(c.procs) + 1), Path: `C:\core\sing-box.exe`},
		done: make(chan struct{}), setup: c.setupNext}
	c.procs = append(c.procs, p)
	if p.setup {
		c.rec.add("core:start %d setup", p.id.PID)
	} else {
		c.rec.add("core:start %d", p.id.PID)
	}
	c.log = ""
	switch {
	case c.dieEarly > 0:
		c.dieEarly--
		p.exit()
	case c.silent > 0:
		c.silent--
	default:
		c.log = "INFO sing-box started (0.42s)\n"
		if c.needLogin {
			c.log += "INFO endpoint/tailscale[vpn]: Waiting for authentication: https://login.example/x\n"
		}
	}
	if c.onStart != nil {
		c.onStart(p)
	}
	return p, nil
}

func (c *fakeCore) Find(id store.CoreIdentity) (CoreProc, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.leftover != nil && c.leftover.id == id && c.leftover.alive() {
		return c.leftover, true
	}
	return nil, false
}

func (c *fakeCore) LogSize() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int64(len(c.log))
}

func (c *fakeCore) logFrom(offset int64) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if offset > int64(len(c.log)) {
		return ""
	}
	return c.log[offset:]
}

func (c *fakeCore) appendLog(line string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.log += line + "\n"
}

func (c *fakeCore) Clash(port int, secret string) ClashAPI { return c.clash }

func (c *fakeCore) BuildSetupConfig(o SetupOptions) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rec.add("core:setupconfig")
	c.setupOpts = o
	c.setupNext = true
	return json.Marshal(o)
}

// setupAlive tells whether a core of the setup mode runs.
func (c *fakeCore) setupAlive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range c.procs {
		if p.setup && p.alive() {
			return true
		}
	}
	return false
}

func (c *fakeCore) Node(port int, secret string) NodeAPI {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nodePort, c.nodeSecret = port, secret
	return c.node
}

// fakeNode is the API service of the core; all runs share it (h.core.node). By default the node
// is logged in and running, with its address, the harness's exit node in the list and a direct
// path. A logout that succeeds leaves it at needs_login without a link and without an address.
type fakeNode struct {
	mu      sync.Mutex
	rec     *recorder
	clock   *fakeClock
	state   NodeState
	nodes   []protocol.ExitNode
	path    string
	err     error         // every call fails with it
	pathErr error         // Path fails with it
	hold    chan struct{} // the next Path closes it and waits for the end of its context
	logouts int
	looks   int         // calls of State
	lists   []time.Time // the clock at each call of ExitNodes
}

func newFakeNode(rec *recorder, clock *fakeClock) *fakeNode {
	return &fakeNode{
		rec:   rec,
		clock: clock,
		state: NodeState{Backend: protocol.BackendRunning, HasAddress: true},
		nodes: []protocol.ExitNode{{ID: exitNode, Name: "exit-a", Online: true}},
		path:  protocol.PathDirect,
	}
}

// calls gives how often State was called and when ExitNodes was.
func (n *fakeNode) calls() (looks int, lists []time.Time) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.looks, append([]time.Time{}, n.lists...)
}

func (n *fakeNode) State(ctx context.Context) (NodeState, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.looks++
	if n.err != nil {
		return NodeState{}, n.err
	}
	return n.state, nil
}

func (n *fakeNode) ExitNodes(ctx context.Context) ([]protocol.ExitNode, error) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.lists = append(n.lists, n.clock.Now())
	if n.err != nil {
		return nil, n.err
	}
	return append([]protocol.ExitNode{}, n.nodes...), nil
}

func (n *fakeNode) Path(ctx context.Context, exitNode string) (string, error) {
	n.mu.Lock()
	hold := n.hold
	n.hold = nil
	n.mu.Unlock()
	if hold != nil {
		close(hold)
		<-ctx.Done()
		n.rec.add("node:path cancelled")
		return "", ctx.Err()
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	switch {
	case n.err != nil:
		return "", n.err
	case n.pathErr != nil:
		return "", n.pathErr
	}
	return n.path, nil
}

func (n *fakeNode) Logout(ctx context.Context) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.rec.add("node:logout")
	n.logouts++
	if n.err != nil {
		return n.err
	}
	n.state = NodeState{Backend: protocol.BackendNeedsLogin}
	return nil
}

func (n *fakeNode) set(f func(n *fakeNode)) {
	n.mu.Lock()
	defer n.mu.Unlock()
	f(n)
}

func (c *fakeCore) last() *fakeProc {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.procs) == 0 {
		return nil
	}
	return c.procs[len(c.procs)-1]
}

func (c *fakeCore) anyAlive() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, p := range append(c.procs, c.leftover) {
		if p != nil && p.alive() {
			return true
		}
	}
	return false
}

type fakeClash struct {
	mu        sync.Mutex
	rec       *recorder
	mode      string
	refuse    map[string]bool // modes the core does not take
	conns     []Conn
	connsErr  error
	reconnect func(c Conn) *Conn // a closed connection comes back like this (nil: it does not)
	onConns   func()             // runs at each call of Connections, before the answer, outside the lock
}

func (k *fakeClash) SetMode(ctx context.Context, mode string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.rec.add("clash:mode %s", mode)
	if k.refuse[mode] {
		return fmt.Errorf("mode %s not taken", mode)
	}
	k.mode = mode
	return nil
}

func (k *fakeClash) Connections(ctx context.Context) ([]Conn, error) {
	k.mu.Lock()
	hook := k.onConns
	k.mu.Unlock()
	if hook != nil {
		hook()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]Conn{}, k.conns...), k.connsErr
}

func (k *fakeClash) CloseConn(ctx context.Context, id string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.rec.add("clash:close %s", id)
	for i, c := range k.conns {
		if c.ID == id {
			k.conns = append(k.conns[:i], k.conns[i+1:]...)
			if k.reconnect != nil {
				if n := k.reconnect(c); n != nil {
					k.conns = append(k.conns, *n)
				}
			}
			break
		}
	}
	return nil
}

func (k *fakeClash) currentMode() string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.mode
}

type fakeSys struct {
	mu        sync.Mutex
	rec       *recorder
	store     *fakeStore
	installed bool
	on        bool
	ip        string
	allowLAN  bool
	statusErr error
	setErr    error
	ignoreSet bool // `tailscale set` reports success but nothing changes
	clearErr  error
	allowErr  error
}

func (s *fakeSys) Status(ctx context.Context) (SysStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.statusErr != nil {
		return SysStatus{Installed: s.installed}, s.statusErr
	}
	st := SysStatus{Installed: s.installed, ExitNodeOn: s.on}
	if s.on {
		st.ExitNodeIP = s.ip
	}
	return st, nil
}

func (s *fakeSys) AllowLAN(ctx context.Context) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.allowLAN, s.allowErr
}

func (s *fakeSys) SetExitNode(ctx context.Context, ip string, allowLAN bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rec.add("sys:set %s %v", ip, allowLAN)
	if s.setErr != nil {
		return s.setErr
	}
	if !s.ignoreSet {
		s.on, s.ip, s.allowLAN = true, ip, allowLAN
	}
	return nil
}

func (s *fakeSys) ClearExitNode(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.store.hasJournal() {
		s.rec.add("sys:clear WITHOUT JOURNAL")
	} else {
		s.rec.add("sys:clear")
	}
	if s.clearErr != nil {
		return s.clearErr
	}
	s.on = false
	return nil
}

func (s *fakeSys) RestoreCommand(ip string, allowLAN bool) string {
	return fmt.Sprintf("tailscale set --exit-node=%s --exit-node-allow-lan-access=%v", ip, allowLAN)
}

func (s *fakeSys) Dir() string { return `C:\Program Files\Tailscale` }

func (s *fakeSys) set(f func(s *fakeSys)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f(s)
}

// fakeProbe answers like the address service: through probe-vpn, probe-direct, and the helper's own request.
type fakeProbe struct {
	mu     sync.Mutex
	vpn    []ProbeResult // consumed one by one, the last one repeats
	direct []ProbeResult
	self   []ProbeResult
	hang   bool // every request hangs until its context ends
}

func next(list *[]ProbeResult) ProbeResult {
	r := (*list)[0]
	if len(*list) > 1 {
		*list = (*list)[1:]
	}
	return r
}

func (p *fakeProbe) ViaProxy(ctx context.Context, url string, port int) ProbeResult {
	p.mu.Lock()
	hang := p.hang
	p.mu.Unlock()
	if hang {
		<-ctx.Done()
		return noAnswer
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if port == portVPN {
		return next(&p.vpn)
	}
	return next(&p.direct)
}

func (p *fakeProbe) Direct(ctx context.Context, url string) ProbeResult {
	p.mu.Lock()
	defer p.mu.Unlock()
	return next(&p.self)
}

func (p *fakeProbe) script(vpn, direct, self []ProbeResult) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if vpn != nil {
		p.vpn = vpn
	}
	if direct != nil {
		p.direct = direct
	}
	if self != nil {
		p.self = self
	}
}

var (
	atVPN    = ProbeResult{Reached: true, IP: vpnAddr}
	atDirect = ProbeResult{Reached: true, IP: directAddr}
	noAnswer = ProbeResult{}
)

type fakeHost struct {
	mu           sync.Mutex
	rec          *recorder
	core         *fakeCore
	noRoute      int // so many next checks find no default route; -1: never a route
	forwarding   bool
	dnsOverride  bool
	otherCore    bool
	adapterStuck bool
	staleAdapter bool  // a forced end of a core left its Wintun device behind; the next removal takes it
	removeErr    error // the removal of the stale Wintun device fails
}

func (h *fakeHost) HasDefaultRoute() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.noRoute < 0 {
		return false
	}
	if h.noRoute > 0 {
		h.noRoute--
		return false
	}
	return true
}

func (h *fakeHost) IPForwarding() bool   { h.mu.Lock(); defer h.mu.Unlock(); return h.forwarding }
func (h *fakeHost) DNSOverride() bool    { h.mu.Lock(); defer h.mu.Unlock(); return h.dnsOverride }
func (h *fakeHost) AdapterPresent() bool { return h.adapterStuck || h.core.anyAlive() }

// CoreRunning sees a foreign sing-box.exe (otherCore) and, as the real check does, a core of the
// setup mode that still runs: step 1 of 5.3 must never meet one (spec 7).
func (h *fakeHost) CoreRunning() bool {
	h.mu.Lock()
	other := h.otherCore
	h.mu.Unlock()
	return other || h.core.setupAlive()
}

func (h *fakeHost) FlushDNS() error {
	h.rec.add("dns:flush")
	return nil
}

func (h *fakeHost) FreePorts(n int) ([]int, error) {
	return []int{portVPN, portDirect, 40003, 40004}[:n], nil
}

func (h *fakeHost) Secret() string { return "0123456789abcdef0123456789abcdef" }

func (h *fakeHost) RemoveStaleAdapter() (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rec.add("host:removestale")
	if h.removeErr != nil {
		return false, h.removeErr
	}
	removed := h.staleAdapter
	h.staleAdapter = false
	return removed, nil
}

// harness wires an engine to the fakes. By default: logged in, exit node selected,
// the system client on with the same exit node and LAN access allowed.
type harness struct {
	t      *testing.T
	e      *Engine
	rec    *recorder
	clock  *fakeClock
	store  *fakeStore
	core   *fakeCore
	clash  *fakeClash
	sys    *fakeSys
	probe  *fakeProbe
	host   *fakeHost
	mu     sync.Mutex
	states []protocol.State
	exited bool
}

var testRules = rules.Env{WindowsDir: `C:\Windows`, ProgramFiles: `C:\Program Files`, ProgramFilesX86: `C:\Program Files (x86)`}

func newHarness(t *testing.T) *harness {
	h := &harness{t: t, rec: &recorder{}, clock: &fakeClock{now: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)}}
	h.store = &fakeStore{rec: h.rec, state: store.Defaults()}
	h.store.state.LoggedIn = true
	h.store.state.SelectedExitNode = exitNode
	h.clash = &fakeClash{rec: h.rec, mode: "vpn", refuse: map[string]bool{}}
	h.core = &fakeCore{rec: h.rec, clash: h.clash, node: newFakeNode(h.rec, h.clock)}
	h.sys = &fakeSys{rec: h.rec, store: h.store, installed: true, on: true, ip: exitNode, allowLAN: true}
	h.probe = &fakeProbe{vpn: []ProbeResult{atVPN}, direct: []ProbeResult{atDirect}, self: []ProbeResult{atVPN}}
	h.host = &fakeHost{rec: h.rec, core: h.core}
	h.e = New(Deps{
		Clock: h.clock, Store: h.store, Core: h.core, Sys: h.sys, Probe: h.probe, Host: h.host,
		Rules: testRules,
		Settings: Settings{HelperPath: `C:\code\router\helper\bin\splitlane-helper.exe`,
			Hostname: "PC-splitlane", HelperVersion: "0.1.0", CoreVersion: "1.14.4"},
		Log: t.Logf,
		OnState: func(s protocol.State) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.states = append(h.states, s)
		},
		OnExit: func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.exited = true
		},
	})
	return h
}

// load puts the fake store into the engine, like the start of the helper does.
func (h *harness) load() { h.e.load() }

// phases is the sequence of published phases without repeats.
func (h *harness) phases() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []string
	for _, s := range h.states {
		if len(out) == 0 || out[len(out)-1] != s.Phase {
			out = append(out, s.Phase)
		}
	}
	return out
}

// sawReason reports whether a published state carried this reason code.
func (h *harness) sawReason(code string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, s := range h.states {
		if s.Reason != nil && s.Reason.Code == code {
			return true
		}
	}
	return false
}

// waitFor polls cond for up to 5 s of real time.
func (h *harness) waitFor(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s; phases %v; events %v", what, h.phases(), h.rec.list())
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func (h *harness) waitPhase(phase string) {
	h.t.Helper()
	h.waitFor("phase "+phase, func() bool { return h.e.State().Phase == phase && h.idle() })
}

func (h *harness) idle() bool {
	h.e.mu.Lock()
	defer h.e.mu.Unlock()
	return h.e.op == nil
}

// running puts the engine into phase on with a live core and a journal, as after step 9 of 5.3.
// The watcher of 5.5 is not started here; the tests of the watcher start it themselves.
func (h *harness) running() *fakeProc {
	h.t.Helper()
	p, _ := h.core.Start(`C:\data\core-config.json`)
	proc := p.(*fakeProc)
	h.store.journal = &store.Journal{ExitNode: exitNode, AllowLAN: true}
	h.sys.set(func(s *fakeSys) { s.on = false })
	h.load()
	h.e.mu.Lock()
	r := &coreRun{proc: proc, clash: h.clash, ports: portsForTest(), mode: "vpn"}
	h.e.run = r
	h.e.st.Addresses = protocol.Addresses{VPN: strPtr(vpnAddr), Direct: strPtr(directAddr)}
	h.e.finishOn()
	h.e.mu.Unlock()
	return proc
}

func portsForTest() Ports {
	return Ports{ProbeVPN: portVPN, ProbeDirect: portDirect, Clash: 40003, API: 40004}
}

// countSince counts the events from mark on that start with prefix.
func countSince(h *harness, mark int, prefix string) int {
	n := 0
	for _, ev := range h.rec.list()[mark:] {
		if strings.HasPrefix(ev, prefix) {
			n++
		}
	}
	return n
}

// systemExitNode is the exit node of the fake system client now.
func systemExitNode(h *harness) (bool, string) {
	h.sys.mu.Lock()
	defer h.sys.mu.Unlock()
	return h.sys.on, h.sys.ip
}

func strPtr(s string) *string { return &s }

var errBoom = errors.New("boom")
