// Command slctl is the test client of the helper's pipe (Plan 1): every method of spec 5.1, waiting
// for phases, a lane check with renamed copies of curl.exe, and a development seed of state.json.
// Addresses are masked in the output unless -show-addresses is given.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/store"
)

const usage = `usage: slctl [-pipe NAME] [-timeout 150s] [-show-addresses] [-url URL] COMMAND [ARGS]
  state                          print the state
  enable                         routing.enable, wait for on or error
  disable [-skip-restore]        routing.disable, wait for off or error
  alldirect on|off               routing.allDirect, wait for the flag
  lanes FILE                     lanes.set with the params in FILE (JSON)
  exitnode IP                    tailscale.setExitNode
  clear-system-exit-node         systemTailscale.clearExitNode
  shutdown                       helper.shutdown, wait until the pipe closes
  call METHOD [PARAMS|@FILE]     any method, params as JSON or from a file
  wait PHASE[,PHASE...]          wait for one of the phases; success when it is the first one
  watch [SECONDS]                print events
  expect-lanes EXE=LANE ...      run copies of curl.exe, compare their address with state.addresses
  seed DATA-DIR EXIT-NODE [HOSTNAME]   development only: state.json of a logged-in node
  reset-dev DATA-DIR             development only: clear "was on", the core identity and restore.json
A pipe that does not exist yet is waited for, up to -timeout and at most 30 s.
exit codes: 0 ok, 1 error response or wrong final phase, 2 usage or no pipe, 3 timeout, 4 pipe broken or closed`

// Exit codes.
const (
	exitOK      = 0
	exitFailed  = 1 // an error response, a final phase error, a lane on the wrong address
	exitUsage   = 2 // wrong arguments, no pipe
	exitTimeout = 3 // no answer, or the awaited phase did not come, within -timeout
	exitBroken  = 4 // the pipe broke or the helper closed it while a command was running
)

// exitCodeFor is the exit code of the error that ended a command, and the one place that decides on
// 3: only a deadline is a timeout. An error response (a *protocol.Error, already printed by call)
// is a failure; anything else is a pipe that broke or was closed.
func exitCodeFor(err error) int {
	var perr *protocol.Error
	switch {
	case errors.Is(err, os.ErrDeadlineExceeded):
		return exitTimeout
	case errors.As(err, &perr):
		return exitFailed
	}
	return exitBroken
}

// pipeCommands are the commands that talk to the helper.
var pipeCommands = []string{"state", "enable", "disable", "alldirect", "lanes", "exitnode", "clear-system-exit-node",
	"shutdown", "call", "wait", "watch", "expect-lanes"}

// A helper that was just started opens its pipe a moment later (its log, the SHA-256 of the core,
// the engine). go-winio retries only a busy pipe, so slctl tries a missing one again itself.
const (
	dialWaitMax = 30 * time.Second
	dialEvery   = 200 * time.Millisecond
)

// dial connects to the pipe; while the pipe does not exist it tries again every 200 ms for wait.
func dial(name string, wait time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(wait)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		conn, err := winio.DialPipeContext(ctx, name)
		cancel()
		if err == nil || !errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || !time.Now().Add(dialEvery).Before(deadline) {
			return conn, err
		}
		time.Sleep(dialEvery)
	}
}

func main() { os.Exit(run(os.Args[1:], os.Stdout)) }

type client struct {
	conn    net.Conn
	r       *bufio.Reader
	out     io.Writer
	show    bool
	nextID  int
	state   *protocol.State
	timeout time.Time
	lastErr error
}

func (c *client) print(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if !c.show {
		line = protocol.MaskAddresses(line)
	}
	fmt.Fprintln(c.out, line)
}

// read returns the next message; events update the known state and are printed.
func (c *client) read() (map[string]json.RawMessage, error) {
	c.conn.SetReadDeadline(c.timeout)
	text, err := c.r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &m); err != nil {
		return nil, err
	}
	if ev, ok := m["event"]; ok {
		var name string
		json.Unmarshal(ev, &name)
		if name == "state" {
			var s protocol.State
			json.Unmarshal(m["data"], &s)
			c.state = &s
			c.print("EVENT state %s", summary(s))
		} else {
			c.print("EVENT %s", name)
		}
	}
	return m, nil
}

// call sends a request and waits for its response.
func (c *client) call(method string, params any) (json.RawMessage, *protocol.Error, error) {
	c.nextID++
	req := map[string]any{"id": c.nextID, "method": method}
	if params != nil {
		req["params"] = params
	}
	b, err := json.Marshal(req)
	if err != nil {
		return nil, nil, err
	}
	if _, err := c.conn.Write(append(b, '\n')); err != nil {
		return nil, nil, err
	}
	for {
		m, err := c.read()
		if err != nil {
			return nil, nil, err
		}
		var id int
		if raw, ok := m["id"]; !ok || json.Unmarshal(raw, &id) != nil || id != c.nextID {
			continue
		}
		if raw, ok := m["error"]; ok {
			var e protocol.Error
			json.Unmarshal(raw, &e)
			c.print("ERROR %s %s%s", e.Code, e.Message, rulesText(e.Rules))
			return nil, &e, nil
		}
		c.print("RESULT %s", m["result"])
		return m["result"], nil, nil
	}
}

func rulesText(rules []protocol.RuleRejection) string {
	var parts []string
	for _, r := range rules {
		parts = append(parts, r.ID+"="+r.Reason)
	}
	if len(parts) == 0 {
		return ""
	}
	return " rules: " + strings.Join(parts, ",")
}

func summary(s protocol.State) string {
	reason := "-"
	if s.Reason != nil {
		reason = s.Reason.Code
		for k, v := range s.Reason.Params {
			reason += fmt.Sprintf(" %s=%q", k, v)
		}
	}
	return fmt.Sprintf("phase=%s reason=%s allDirect=%v defaultLane=%s apps=%d vpn=%s direct=%s systemExitNode=%v restoreOnDisable=%v",
		s.Phase, reason, s.AllDirect, s.DefaultLane, len(s.Apps), str(s.Addresses.VPN), str(s.Addresses.Direct),
		s.SystemTailscale.ExitNodeOn, s.SystemTailscale.RestoreOnDisable)
}

func str(p *string) string {
	if p == nil {
		return "null"
	}
	return *p
}

// fetchState asks for the state now.
func (c *client) fetchState() (protocol.State, error) {
	raw, perr, err := c.call(protocol.MethodStateGet, nil)
	if err != nil {
		return protocol.State{}, err
	}
	if perr != nil {
		return protocol.State{}, perr
	}
	var s protocol.State
	json.Unmarshal(raw, &s)
	c.state = &s
	return s, nil
}

// waitFor reads events until cond holds for the state, after asking for it once.
func (c *client) waitFor(cond func(protocol.State) bool) (protocol.State, error) {
	s, err := c.fetchState()
	if err != nil {
		return s, err
	}
	for !cond(s) {
		if _, err := c.read(); err != nil {
			return s, err
		}
		s = *c.state
	}
	return s, nil
}

func phaseIn(phases ...string) func(protocol.State) bool {
	return func(s protocol.State) bool {
		for _, p := range phases {
			if s.Phase == p {
				return true
			}
		}
		return false
	}
}

func run(args []string, out io.Writer) int {
	fs := flag.NewFlagSet("slctl", flag.ContinueOnError)
	fs.SetOutput(out)
	name := fs.String("pipe", pipe.DefaultName, "pipe name")
	timeout := fs.Duration("timeout", 150*time.Second, "how long to wait for an answer or a phase")
	show := fs.Bool("show-addresses", false, "print addresses as they are")
	url := fs.String("url", store.DefaultProbeURL, "address service for expect-lanes")
	fs.Usage = func() { fmt.Fprintln(out, usage) }
	if err := fs.Parse(args); err != nil || fs.NArg() == 0 {
		fmt.Fprintln(out, usage)
		return exitUsage
	}
	cmd, rest := fs.Arg(0), fs.Args()[1:]
	switch {
	case cmd == "seed":
		return seed(rest, out)
	case cmd == "reset-dev":
		return resetDev(rest, out)
	case !slices.Contains(pipeCommands, cmd):
		fmt.Fprintln(out, usage)
		return exitUsage
	}
	conn, err := dial(*name, min(*timeout, dialWaitMax))
	if err != nil {
		fmt.Fprintf(out, "SLCTL: cannot connect to %s: %v\n", *name, err)
		return exitUsage
	}
	defer conn.Close()
	c := &client{conn: conn, r: bufio.NewReader(conn), out: out, show: *show, timeout: time.Now().Add(*timeout)}
	var code int
	if _, perr, err := c.call(protocol.MethodHello, protocol.HelloParams{Protocol: protocol.Version}); err != nil {
		code = c.fail(err)
	} else if perr != nil {
		c.print("SLCTL: hello failed: %v", perr) // call printed its ERROR line
		code = exitFailed
	} else {
		code = command(c, cmd, rest, *url)
	}
	if c.lastErr != nil { // whatever the command returned, the error that ended it decides on 3 and 4
		code = exitCodeFor(c.lastErr)
	}
	return code
}

// fail records the error that ended the command, says what happened and returns its exit code.
// An error response was printed by call already (its ERROR line).
func (c *client) fail(err error) int {
	c.lastErr = err
	code := exitCodeFor(err)
	last := ""
	if c.state != nil {
		last = ", last state " + summary(*c.state)
	}
	switch code {
	case exitTimeout:
		c.print("SLCTL: TIMEOUT, no answer or phase within -timeout (%v)%s", err, last)
	case exitBroken:
		c.print("SLCTL: the pipe is broken or closed (%v)%s", err, last)
	}
	return code
}

// finish prints the final line; an error ends the command through fail.
func (c *client) finish(s protocol.State, err error, ok func(protocol.State) bool) int {
	if err != nil {
		return c.fail(err)
	}
	c.print("SLCTL: %s", summary(s))
	if !ok(s) {
		return exitFailed
	}
	return exitOK
}

func command(c *client, cmd string, args []string, url string) int {
	simple := func(method string, params any) int {
		_, perr, err := c.call(method, params)
		switch {
		case err != nil:
			return c.fail(err)
		case perr != nil:
			return exitFailed // call printed its ERROR line
		}
		return exitOK
	}
	switch cmd {
	case "state":
		s, err := c.fetchState()
		return c.finish(s, err, func(protocol.State) bool { return true })
	case "enable":
		if code := simple(protocol.MethodEnable, nil); code != exitOK {
			return code
		}
		s, err := c.waitFor(phaseIn(protocol.PhaseOn, protocol.PhaseError, protocol.PhaseDegraded, protocol.PhaseBlocked))
		return c.finish(s, err, phaseIn(protocol.PhaseOn))
	case "disable":
		fs := flag.NewFlagSet("disable", flag.ContinueOnError)
		skip := fs.Bool("skip-restore", false, "disable without restoring the system exit node")
		if fs.Parse(args) != nil {
			return exitUsage
		}
		if code := simple(protocol.MethodDisable, protocol.DisableParams{SkipRestore: *skip}); code != exitOK {
			return code
		}
		s, err := c.waitFor(func(s protocol.State) bool {
			return phaseIn(protocol.PhaseOff, protocol.PhaseError)(s) ||
				(s.Phase == protocol.PhaseStopping && s.Reason != nil && s.Reason.Code == protocol.ReasonRestorePending)
		})
		return c.finish(s, err, phaseIn(protocol.PhaseOff))
	case "alldirect":
		if len(args) != 1 || (args[0] != "on" && args[0] != "off") {
			return exitUsage
		}
		on := args[0] == "on"
		before, err := c.fetchState()
		if err != nil {
			return c.finish(before, err, nil)
		}
		if code := simple(protocol.MethodAllDirect, map[string]bool{"on": on}); code != exitOK {
			return code
		}
		// the apply takes about 3 s; the next state event carries its result
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			time.Sleep(500 * time.Millisecond)
			if s, err := c.fetchState(); err != nil || s.AllDirect == on {
				return c.finish(s, err, func(s protocol.State) bool { return s.AllDirect == on })
			}
		}
		s, _ := c.fetchState()
		return c.finish(s, nil, func(s protocol.State) bool { return s.AllDirect == on })
	case "lanes":
		if len(args) != 1 {
			return exitUsage
		}
		b, err := os.ReadFile(args[0])
		if err != nil {
			c.print("SLCTL: %v", err)
			return exitUsage
		}
		return simple(protocol.MethodLanesSet, json.RawMessage(b))
	case "exitnode":
		if len(args) != 1 {
			return exitUsage
		}
		return simple(protocol.MethodSetExitNode, protocol.SetExitNodeParams{ID: args[0]})
	case "clear-system-exit-node":
		return simple(protocol.MethodClearSystemExitNode, nil)
	case "shutdown":
		if code := simple(protocol.MethodShutdown, nil); code != exitOK {
			return code
		}
		for {
			if _, err := c.read(); err != nil {
				if errors.Is(err, os.ErrDeadlineExceeded) {
					c.lastErr = err
					c.print("SLCTL: TIMEOUT the helper did not exit")
					return exitTimeout
				}
				c.print("SLCTL: the helper closed the pipe")
				return exitOK
			}
		}
	case "call":
		if len(args) < 1 || len(args) > 2 {
			return exitUsage
		}
		var params any
		if len(args) == 2 {
			raw := []byte(args[1])
			if strings.HasPrefix(args[1], "@") {
				b, err := os.ReadFile(args[1][1:])
				if err != nil {
					c.print("SLCTL: %v", err)
					return exitUsage
				}
				raw = b
			}
			if !json.Valid(raw) {
				// Windows PowerShell 5.1 drops the double quotes of a native argument: use @FILE there
				c.print("SLCTL: params are not valid JSON; write them to a file and pass @FILE")
				return exitUsage
			}
			params = json.RawMessage(raw)
		}
		return simple(args[0], params)
	case "wait":
		if len(args) != 1 {
			return exitUsage
		}
		phases := strings.Split(args[0], ",")
		s, err := c.waitFor(phaseIn(phases...))
		return c.finish(s, err, phaseIn(phases[0]))
	case "watch":
		seconds := 60
		if len(args) == 1 {
			fmt.Sscan(args[0], &seconds)
		}
		c.timeout = time.Now().Add(time.Duration(seconds) * time.Second)
		for {
			if _, err := c.read(); err != nil {
				return exitOK
			}
		}
	case "expect-lanes":
		return expectLanes(c, args, url)
	}
	fmt.Fprintln(c.out, usage)
	return exitUsage
}

// laneName says which lane an address belongs to.
func laneName(ip string, s protocol.State) string {
	switch {
	case ip == "":
		return "none"
	case s.Addresses.VPN != nil && ip == *s.Addresses.VPN:
		return protocol.LaneVPN
	case s.Addresses.Direct != nil && ip == *s.Addresses.Direct:
		return protocol.LaneDirect
	}
	return "other"
}

// verdict judges answers only (Plan 0 rule): the first answer is right, or two of three are.
// More requests are needed while it returns done=false.
func verdict(seen []string, want string) (ok, done bool) {
	var answers []string
	for _, s := range seen {
		if s != "none" {
			answers = append(answers, s)
		}
	}
	switch {
	case len(answers) > 0 && answers[0] == want:
		return true, true
	case len(answers) >= 3:
		n := 0
		for _, a := range answers[:3] {
			if a == want {
				n++
			}
		}
		return n >= 2, true
	case len(seen) >= 6:
		return false, true
	}
	return false, false
}

// egress runs a copy of curl.exe against the address service, past any proxy.
var egress = func(exe, url string) string {
	cmd := exec.Command(exe, "-4", "-s", "--noproxy", "*", "--max-time", "10", url)
	b, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func expectLanes(c *client, args []string, url string) int {
	s, err := c.fetchState()
	if err != nil {
		return c.finish(s, err, nil)
	}
	if s.Addresses.VPN == nil || s.Addresses.Direct == nil || *s.Addresses.VPN == *s.Addresses.Direct {
		c.print("SLCTL: lane addresses are unknown or equal; nothing to compare")
		return exitFailed
	}
	code := exitOK
	for _, a := range args {
		exe, want, ok := strings.Cut(a, "=")
		if !ok || (want != protocol.LaneVPN && want != protocol.LaneDirect) {
			return exitUsage
		}
		var seen []string
		good, done := false, false
		for !done {
			seen = append(seen, laneName(egress(exe, url), s))
			good, done = verdict(seen, want)
		}
		if good {
			c.print("LANE-OK %s %s", filepath.Base(exe), want)
		} else {
			c.print("LANE-WRONG %s want=%s seen=%v", filepath.Base(exe), want, seen)
			code = exitFailed
		}
	}
	return code
}

// programData is the root of the product's data folder: the known folder ProgramData, never the
// environment variable. The tests point it at a temporary folder.
var programData = func() string {
	if p, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0); err == nil && p != "" {
		return p
	}
	return `C:\ProgramData`
}

// isDrive says whether a volume name is a plain drive letter, "C:".
func isDrive(vol string) bool { return len(vol) == 2 && vol[1] == ':' }

// finalPath asks the system for the real name of an existing path: links and junctions followed, the
// 8.3 short names and the case of every name as stored, the \\?\ prefix removed.
func finalPath(p string) (string, error) {
	name, err := windows.UTF16PtrFromString(p)
	if err != nil {
		return "", err
	}
	h, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(h, &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if int(n) < len(buf) {
			return strings.TrimPrefix(windows.UTF16ToString(buf[:n]), `\\?\`), nil
		}
		buf = make([]uint16, n+1) // n is the size needed, with the terminating zero
	}
}

// realPath is dir with "." and ".." folded and every part that exists resolved by the system (see
// finalPath); the part that does not exist yet is appended as written. ok is false when nothing can be
// said: an existing part cannot be resolved (a junction to nothing, no access), no part exists, or the
// result is not on a drive letter. The caller then refuses the folder.
func realPath(dir string) (path string, ok bool) {
	p := filepath.Clean(dir)
	var tail []string
	for {
		if real, err := finalPath(p); err == nil {
			real = filepath.Join(append([]string{real}, tail...)...)
			return real, isDrive(filepath.VolumeName(real))
		}
		if _, err := os.Lstat(p); !errors.Is(err, fs.ErrNotExist) {
			return "", false // there, but not resolvable
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", false // not even the drive is there
		}
		tail = append([]string{filepath.Base(p)}, tail...)
		p = parent
	}
}

// devFolder: an absolute folder on a drive letter that is not the root of the product data
// (%ProgramData%, the known folder) or below it. Whatever the spelling (forward slashes, ".." segments,
// short 8.3 names, other case, links and junctions), the real folder is compared with the real root by
// path elements; \\?\, \\.\ and UNC spellings are refused outright. It only reads.
func devFolder(dir string) bool {
	if !filepath.IsAbs(dir) || !isDrive(filepath.VolumeName(filepath.Clean(dir))) {
		return false
	}
	real, ok := realPath(dir)
	if !ok {
		return false
	}
	root, ok := realPath(programData())
	if !ok {
		return false
	}
	if !strings.EqualFold(filepath.VolumeName(real), filepath.VolumeName(root)) {
		return true // another drive
	}
	rel, err := filepath.Rel(root, real)
	if err != nil {
		return false
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) // outside, or an ancestor
}

// seed writes state.json for a node that is already logged in (development only, Plan 1A has no
// login yet). It refuses the product data folder.
func seed(args []string, out io.Writer) int {
	if len(args) < 2 || len(args) > 3 {
		fmt.Fprintln(out, usage)
		return exitUsage
	}
	dir := args[0]
	if !devFolder(dir) {
		fmt.Fprintln(out, "SLCTL: seed takes an absolute development folder on a drive letter, never %ProgramData% or below it")
		return exitUsage
	}
	if err := protocol.ValidateExitNodeID(args[1], nil); err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitUsage
	}
	d := store.Dir{Path: dir}
	s, err := d.LoadState()
	if err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	s.LoggedIn, s.SelectedExitNode = true, args[1]
	if len(args) == 3 {
		s.DeviceName = args[2]
	}
	if err := d.SaveState(s); err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	fmt.Fprintln(out, "SEED-OK state.json written")
	return exitOK
}

// resetDev clears what a run that the guard ended while on leaves in a development data folder:
// "was on", the core identity and the restore journal. Without it the next helper would restore
// and enable again on its own at start (5.7). Only after the system was found restored (Task 23).
func resetDev(args []string, out io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(out, usage)
		return exitUsage
	}
	dir := args[0]
	if !devFolder(dir) {
		fmt.Fprintln(out, "SLCTL: reset-dev takes an absolute development folder on a drive letter, never %ProgramData% or below it")
		return exitUsage
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err != nil {
		fmt.Fprintf(out, "SLCTL: no state.json in %s\n", dir)
		return exitUsage
	}
	d := store.Dir{Path: dir}
	s, err := d.LoadState()
	if err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	s.WasOn, s.Core = false, nil
	if err := d.SaveState(s); err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	if err := d.DeleteJournal(); err != nil {
		fmt.Fprintf(out, "SLCTL: %v\n", err)
		return exitFailed
	}
	fmt.Fprintln(out, "RESET-OK wasOn=false core=none restore.json absent")
	return exitOK
}
