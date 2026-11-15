package main

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/install"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/supervise"
)

func TestParse(t *testing.T) {
	if _, err := parse([]string{"--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}); err == nil {
		t.Fatal("the development paths need --dev")
	}
	if _, err := parse([]string{"--dev", "--data-dir", `data`, "--core", `C:\c\sing-box.exe`}); err == nil {
		t.Fatal("relative paths are refused")
	}
	o, err := parse([]string{"--dev", "--dev-pin", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`, "--pipe", `\\.\pipe\x`})
	if err != nil || !o.devPin || o.pipe != `\\.\pipe\x` {
		t.Fatalf("%+v %v", o, err)
	}
	if o, _ := parse([]string{"--dev", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}); o.pipe != `\\.\pipe\SplitLane.Helper` || o.devPin {
		t.Fatalf("defaults %+v", o)
	}
}

// --core-log-level is a development flag (spec 6.1): only with --dev, only info or debug; info by default.
func TestCoreLogLevelNeedsDev(t *testing.T) {
	paths := []string{"--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}
	for _, level := range []string{"debug", "info"} {
		if _, err := parse(append([]string{"--core-log-level", level}, paths...)); err == nil || !strings.Contains(err.Error(), "--core-log-level needs --dev") {
			t.Errorf("%s without --dev: %v", level, err)
		}
	}
	if _, err := parse(append([]string{"--dev", "--core-log-level", "trace"}, paths...)); err == nil || !strings.Contains(err.Error(), "info or debug") {
		t.Errorf("trace: %v", err)
	}
	o, err := parse(append([]string{"--dev", "--core-log-level", "debug"}, paths...))
	if err != nil || o.coreLogLevel != "debug" {
		t.Fatalf("debug: %+v %v", o, err)
	}
	if o, _ := parse(append([]string{"--dev"}, paths...)); o.coreLogLevel != "info" {
		t.Fatalf("default %q", o.coreLogLevel)
	}
}

// --dev-pin is development only (plan 1A, Global Constraints): without --dev it is a parse error, even
// as false, so the AI assistant's programs are never pinned to the VPN lane in the product.
func TestDevPinNeedsDev(t *testing.T) {
	for _, args := range [][]string{{"--dev-pin"}, {"--dev-pin=false"}, {"--dev-pin", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}} {
		if _, err := parse(args); err == nil || !strings.Contains(err.Error(), "--dev-pin needs --dev") {
			t.Errorf("%v: %v", args, err)
		}
	}
	o, err := parse([]string{"--dev", "--dev-pin", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`})
	if err != nil || !o.devPin {
		t.Fatalf("with --dev: %+v %v", o, err)
	}
}

// Without --dev the helper is the product (spec 4.2, 4.3): no flags at all; its folders and its pipe
// come from the system, never from the command line. The development flags are refused before
// anything is read or written.
func TestProductModeRejectsDevFlags(t *testing.T) {
	for _, args := range [][]string{{"--data-dir", `C:\d`}, {"--core", `C:\c\sing-box.exe`}, {"--pipe", `\\.\pipe\x`}, {"--pipe", pipe.DefaultName}} {
		if _, err := parse(args); err == nil || !strings.Contains(err.Error(), "need --dev") {
			t.Errorf("%v: %v", args, err)
		}
		if code := run(args); code != exitUsage {
			t.Errorf("run %v: exit code %d", args, code)
		}
	}
	o, err := parse(nil)
	if err != nil || o.dev || o.devPin || o.dataDir != "" || o.core != "" || o.pipe != pipe.DefaultName || o.coreLogLevel != "info" {
		t.Fatalf("no flags is the product: %+v %v", o, err)
	}
}

// No test reads the real places of the product: a test of the product mode fakes them (fakePlace), any
// other run without --dev stops at once. No test reaches the real system of install and uninstall.
func init() {
	programDir = func() (string, error) { return "", errors.New("the tests do not read the real program folder") }
	productDir = func() (string, error) { return "", errors.New("the tests do not read the real product folder") }
	userData = func() (string, error) { return "", errors.New("the tests do not read the real user folder") }
	productData = func() (string, error) { return "", errors.New("the tests do not read the real data folder") }
	newSys = func() install.Sys { return nil }
}

// fakePlace puts the product into temporary folders: the program folder with the helper's exe in it
// (the running exe is said to be that one) and the data folder that install made. It returns both.
func fakePlace(t *testing.T) (prog, data string) {
	t.Helper()
	root := t.TempDir()
	prog, data = filepath.Join(root, "Program Files", "SplitLane"), filepath.Join(root, "ProgramData", "SplitLane")
	os.MkdirAll(prog, 0o755)
	os.MkdirAll(data, 0o755)
	oldProg, oldData, oldExe, oldProd, oldUser := programDir, productData, executable, productDir, userData
	productDir = func() (string, error) { return filepath.Dir(prog), nil }
	userData = func() (string, error) { return filepath.Join(root, "Users", "tester", "SplitLane"), nil }
	programDir = func() (string, error) { return prog, nil }
	productData = func() (string, error) { return data, nil }
	executable = func() (string, error) { return filepath.Join(prog, "splitlane-helper.exe"), nil }
	t.Cleanup(func() {
		programDir, productData, executable, productDir, userData = oldProg, oldData, oldExe, oldProd, oldUser
	})
	return prog, data
}

func helperLog(t *testing.T, data string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(data, "logs", "helper.log"))
	return string(b)
}

// The product (spec 4.2, 4.3, 11): the data folder from place.DataDir, the core next to the helper in
// the program folder, the user of the pipe from setup.json. A helper that runs from another folder, a
// missing setup.json and a missing data folder are refused before the core is looked at; the data
// folder is never made here (install makes it with its access list).
func TestProductPaths(t *testing.T) {
	prog, data := fakePlace(t)
	const sid = "S-1-5-21-1000-2000-3000-1001"
	if err := place.WriteSetup(data, place.Setup{UserSID: sid}); err != nil {
		t.Fatal(err)
	}
	core, user, code := productPlace(filepath.Join(prog, "splitlane-helper.exe"), data, t.Logf)
	if code != exitOK || core != filepath.Join(prog, "sing-box.exe") || user != sid {
		t.Fatalf("core %q, user %q, code %d", core, user, code)
	}
	// The whole run, with a file in place of the core: the log in the data folder names the one refused.
	os.WriteFile(filepath.Join(prog, "sing-box.exe"), []byte("not the core"), 0o644)
	if code := run(worker); code != exitBadCore || !strings.Contains(helperLog(t, data), "core refused: "+filepath.Join(prog, "sing-box.exe")) {
		t.Fatalf("exit code %d, log:\n%s", code, helperLog(t, data))
	}

	executable = func() (string, error) { return `C:\Users\u\Downloads\splitlane-helper.exe`, nil }
	if code := run(worker); code != exitUsage || !strings.Contains(helperLog(t, data), `the product runs only as `+filepath.Join(prog, "splitlane-helper.exe")) {
		t.Fatalf("a helper outside the program folder: exit code %d, log:\n%s", code, helperLog(t, data))
	}
	executable = func() (string, error) { return strings.ToUpper(filepath.Join(prog, "splitlane-helper.exe")), nil }

	os.Remove(filepath.Join(data, "setup.json"))
	if code := run(worker); code != exitNotInstalled || !strings.Contains(helperLog(t, data), "not installed") {
		t.Fatalf("no setup.json: exit code %d, log:\n%s", code, helperLog(t, data))
	}

	os.RemoveAll(data)
	if code := run(worker); code != exitNotInstalled {
		t.Fatalf("no data folder: exit code %d", code)
	}
	if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the helper made the data folder")
	}
}

// worker is the command line of the worker copy: what the product does itself, without the supervisor.
var worker = []string{"--worker"}

// --worker is the product's: with --dev it is a usage error (spec 4).
func TestWorkerFlagNeedsProductMode(t *testing.T) {
	args := []string{"--dev", "--worker", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}
	if _, err := parse(args); err == nil || !strings.Contains(err.Error(), "--worker") {
		t.Fatalf("parse: %v", err)
	}
	if code := run(args); code != exitUsage {
		t.Fatalf("exit code %d", code)
	}
	if o, err := parse(worker); err != nil || !o.worker || o.dev {
		t.Fatalf("the product worker: %+v %v", o, err)
	}
}

type endedWorker struct{ code int }

func (w endedWorker) Wait() int { return w.code }

// The product without --worker is the supervisor (spec 4): after the place check it starts its own exe
// with --worker, opens no pipe and keeps no log file open; its lines are appended to helper.log. A
// refused place starts no worker and returns the code of the refusal.
func TestProductModeSupervisesItself(t *testing.T) {
	prog, data := fakePlace(t)
	if err := place.WriteSetup(data, place.Setup{UserSID: "S-1-5-21-1000-2000-3000-1001"}); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(data, "logs")
	os.MkdirAll(logs, 0o755)
	logPath := filepath.Join(logs, "helper.log")
	os.WriteFile(logPath, []byte("an earlier line\n"), 0o644)
	exe := filepath.Join(prog, "splitlane-helper.exe")

	var gotExe string
	var allArgs []string
	var codes []int
	old := startWorker
	startWorker = func(exe string, args []string) (supervise.Worker, error) {
		gotExe = exe
		allArgs = append(allArgs, strings.Join(args, " "))
		code := codes[0]
		codes = codes[1:]
		return endedWorker{code}, nil
	}
	oldClock := superviseClock
	superviseClock = instantClock{}
	t.Cleanup(func() { startWorker, superviseClock = old, oldClock })

	codes = []int{2, 7}
	if code := run(nil); code != exitRefused {
		t.Fatalf("exit code %d", code)
	}
	if gotExe != exe || len(allArgs) != 2 || allArgs[0] != "--worker" || allArgs[1] != "--worker --restarted" {
		t.Fatalf("the worker was started as %q %q", gotExe, allArgs)
	}
	b, _ := os.ReadFile(logPath)
	log := string(b)
	if !strings.HasPrefix(log, "an earlier line\n") || !strings.Contains(log, "supervisor: the helper ended with code 2; starting it again in 5s (restart 1 of 3 in 10m)") ||
		!strings.Contains(log, "supervisor: the helper ended with code 7; not starting it again") {
		t.Fatalf("log:\n%s", log)
	}
	if err := os.Remove(logPath); err != nil {
		t.Fatalf("the supervisor keeps the log open: %v", err)
	}
	if _, err := os.Stat(logPath + ".prev"); err == nil {
		t.Fatal("the supervisor rotated the log")
	}

	// A refused place at the very first start: its code, no worker, and its line is in helper.log
	// although there was no logs folder (the supervisor makes it, as the worker does).
	os.RemoveAll(logs)
	gotExe = ""
	executable = func() (string, error) { return `C:\Users\u\Downloads\splitlane-helper.exe`, nil }
	if code := run(nil); code != exitUsage || gotExe != "" {
		t.Fatalf("a helper outside the program folder: exit code %d, worker %q", code, gotExe)
	}
	if !strings.Contains(helperLog(t, data), "the product runs only as") {
		t.Fatalf("the refusal is not logged:\n%s", helperLog(t, data))
	}
	// The logs folder exists but helper.log does not: the first line makes the file.
	os.Remove(logPath)
	codes = []int{0}
	executable = func() (string, error) { return exe, nil }
	if code := run(nil); code != exitOK || !strings.Contains(helperLog(t, data), "not starting it again") {
		t.Fatalf("no helper.log: exit code %d, log:\n%s", code, helperLog(t, data))
	}
	executable = func() (string, error) { return exe, nil }
	gotExe = ""
	os.Remove(filepath.Join(data, place.SetupFile))
	if code := run(nil); code != exitNotInstalled || gotExe != "" {
		t.Fatalf("no setup.json: exit code %d, worker %q", code, gotExe)
	}
}

// --restarted is the supervisor's flag for a worker it starts after a failure: only with --worker.
func TestRestartedNeedsWorker(t *testing.T) {
	for _, args := range [][]string{{"--restarted"}, {"--dev", "--restarted", "--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}} {
		if _, err := parse(args); err == nil || !strings.Contains(err.Error(), "--restarted needs --worker") {
			t.Errorf("%v: %v", args, err)
		}
		if code := run(args); code != exitUsage {
			t.Errorf("run %v: exit code %d", args, code)
		}
	}
	if o, err := parse([]string{"--worker", "--restarted"}); err != nil || !o.worker || !o.restarted {
		t.Fatalf("%+v %v", o, err)
	}
}

// The first worker rotates helper.log as ever; a restarted one appends, so that the lines of the
// failure (and the supervisor's) are not rotated away by a crash loop.
func TestRestartedWorkerKeepsTheLog(t *testing.T) {
	prog, data := fakePlace(t)
	if err := place.WriteSetup(data, place.Setup{UserSID: "S-1-5-21-1000-2000-3000-1001"}); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(prog, "sing-box.exe"), []byte("not the core"), 0o644) // the run ends at the core check
	logs := filepath.Join(data, "logs")
	os.MkdirAll(logs, 0o755)
	logPath := filepath.Join(logs, "helper.log")

	os.WriteFile(logPath, []byte("the failure\n"), 0o644)
	if code := run([]string{"--worker", "--restarted"}); code != exitBadCore {
		t.Fatalf("exit code %d", code)
	}
	if !strings.HasPrefix(helperLog(t, data), "the failure\n") || !strings.Contains(helperLog(t, data), "core refused") {
		t.Fatalf("log:\n%s", helperLog(t, data))
	}
	if _, err := os.Stat(logPath + ".prev"); err == nil {
		t.Fatal("a restarted worker rotated the log")
	}

	if code := run([]string{"--worker"}); code != exitBadCore {
		t.Fatalf("exit code %d", code)
	}
	if b, _ := os.ReadFile(logPath + ".prev"); !strings.HasPrefix(string(b), "the failure\n") || strings.HasPrefix(helperLog(t, data), "the failure") {
		t.Fatalf("the first worker must rotate: prev %q, log:\n%s", b, helperLog(t, data))
	}
}

// instantClock has no pauses.
type instantClock struct{}

func (instantClock) Now() time.Time { return time.Now() }
func (instantClock) After(time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	ch <- time.Now()
	return ch
}

// setup.json at logon (spec 4.3): a read that fails for a moment (another program holds the file) is
// tried again, setupReads times, so the helper still starts and restores the journal after a reboot.
// Only a missing file is "not installed" (exit 6, at once); one that stays unreadable or does not
// parse is an internal error, never "not installed".
func TestProductSetupReadIsRetried(t *testing.T) {
	prog, data := fakePlace(t)
	const sid = "S-1-5-21-1000-2000-3000-1001"
	if err := place.WriteSetup(data, place.Setup{UserSID: sid}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(data, place.SetupFile)
	busy := &os.PathError{Op: "open", Path: file, Err: windows.ERROR_SHARING_VIOLATION}
	reads, fails := 0, 0
	oldRead, oldWait := readSetup, setupRetryWait
	readSetup = func(dir string) (place.Setup, error) {
		reads++
		if reads <= fails {
			return place.Setup{}, busy
		}
		return place.ReadSetup(dir)
	}
	setupRetryWait = time.Millisecond
	t.Cleanup(func() { readSetup, setupRetryWait = oldRead, oldWait })
	helper := filepath.Join(prog, "splitlane-helper.exe")
	var log strings.Builder
	logf := func(format string, a ...any) { fmt.Fprintf(&log, format+"\n", a...) }

	reads, fails = 0, setupReads-1
	if _, user, code := productPlace(helper, data, logf); code != exitOK || user != sid || reads != setupReads {
		t.Fatalf("busy %d times: user %q, exit code %d, %d reads, log:\n%s", fails, user, code, reads, log.String())
	}
	reads, fails = 0, setupReads
	log.Reset()
	if _, _, code := productPlace(helper, data, logf); code != exitInternal || reads != setupReads ||
		strings.Contains(log.String(), "not installed") || !strings.Contains(log.String(), file) {
		t.Fatalf("busy all the time: exit code %d, %d reads, log:\n%s", code, reads, log.String())
	}
	reads, fails = 0, 0
	log.Reset()
	os.WriteFile(file, []byte(`{broken`), 0o644)
	if _, _, code := productPlace(helper, data, logf); code != exitInternal || strings.Contains(log.String(), "not installed") {
		t.Fatalf("a broken setup.json: exit code %d, log:\n%s", code, log.String())
	}
	reads = 0
	log.Reset()
	os.Remove(file)
	if _, _, code := productPlace(helper, data, logf); code != exitNotInstalled || reads != 1 || !strings.Contains(log.String(), "not installed") {
		t.Fatalf("no setup.json: exit code %d, %d reads, log:\n%s", code, reads, log.String())
	}
}

// A file that is not the pinned core is refused before anything starts (spec 4.3).
func TestRunRefusesAWrongCore(t *testing.T) {
	dir := t.TempDir()
	core := filepath.Join(dir, "sing-box.exe")
	os.WriteFile(core, []byte("not the core"), 0o644)
	if code := run([]string{"--dev", "--data-dir", filepath.Join(dir, "data"), "--core", core}); code != exitBadCore {
		t.Fatalf("exit code %d", code)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "data", "logs", "helper.log"))
	if !strings.Contains(string(b), "core refused") {
		t.Fatalf("log:\n%s", b)
	}
	if code := run([]string{"--dev", "--data-dir", `data`, "--core", core}); code != exitUsage {
		t.Fatalf("a relative data folder: %d", code)
	}
}

// Review F12-1: the state line of helper.log carries the params of the reason whenever the reason changes (the
// error of restore_pending and its manual command, the output of sing-box check of config_invalid, ...): sorted by
// key, each value quoted on the one line, the IPv4 addresses masked and the login links hidden. The same reason
// again gives the line without them.
func TestStateLineHasReasonParams(t *testing.T) {
	pending := &protocol.Reason{Code: protocol.ReasonRestorePending, Params: map[string]string{
		protocol.ParamError:   "tailscale set: exit status 1\nvisit https://login.tailscale.com/a/1f2e3d4c",
		protocol.ParamCommand: "tailscale set --exit-node=100.101.102.103 --exit-node-allow-lan-access"}}
	stopping := protocol.State{Phase: protocol.PhaseStopping, Reason: pending}
	const head = "state: phase=stopping reason=restore_pending allDirect=false"
	want := head + ` params: command="tailscale set --exit-node=<ip> --exit-node-allow-lan-access" error="tailscale set: exit status 1\nvisit <login link>"`
	if got := stateLine(stopping, nil); got != want {
		t.Fatalf("a new reason:\n got %s\nwant %s", got, want)
	}
	if got := stateLine(stopping, &protocol.Reason{Code: pending.Code, Params: maps.Clone(pending.Params)}); got != head {
		t.Fatalf("the same reason again: %s", got)
	}
	other := &protocol.Reason{Code: pending.Code, Params: map[string]string{protocol.ParamCommand: "", protocol.ParamError: "access denied"}}
	if got, want := stateLine(protocol.State{Phase: protocol.PhaseStopping, Reason: other}, pending), head+` params: command="" error="access denied"`; got != want {
		t.Fatalf("the same code, other params:\n got %s\nwant %s", got, want)
	}
	crashed := &protocol.Reason{Code: protocol.ReasonCoreCrashed, Params: map[string]string{}}
	if got := stateLine(protocol.State{Phase: protocol.PhaseError, Reason: crashed}, pending); got != "state: phase=error reason=core_crashed allDirect=false" {
		t.Fatalf("a reason without params: %s", got)
	}
	if got := stateLine(protocol.State{Phase: protocol.PhaseOff, AllDirect: true}, crashed); got != "state: phase=off reason=- allDirect=true" {
		t.Fatalf("no reason: %s", got)
	}
}

// The subcommands install and uninstall (spec 4.3) come first, before any flag of the run mode:
// install takes --user-sid only and needs it, uninstall takes --purge only. The folders come from the
// system as in the product, never from the command line; the lines go to the output that the window
// reads, and a refusal exits with code 7 (a usage error with 2). The commands are replaced here.
func TestInstallSubcommandArgs(t *testing.T) {
	prog, data := fakePlace(t)
	const sid = "S-1-5-21-1000-2000-3000-1001"
	var got []install.Options
	var answer error
	old := adminCommands
	adminCommands = map[string]func(install.Options, install.Sys) error{}
	for name := range old {
		adminCommands[name] = func(o install.Options, s install.Sys) error {
			got = append(got, o)
			return answer
		}
	}
	t.Cleanup(func() { adminCommands = old })

	var out strings.Builder
	if code := admin("install", []string{"--user-sid", sid}, &out); code != exitOK || len(got) != 1 {
		t.Fatalf("exit code %d, %d calls, output:\n%s", code, len(got), out.String())
	}
	o := got[0]
	if o.UserSID != sid || o.Purge || o.ProgramDir != prog || o.DataDir != data || o.ProductDir != filepath.Dir(prog) || o.SessionUser || o.Exe != filepath.Join(prog, "splitlane-helper.exe") || o.Out != &out {
		t.Fatalf("options %+v", o)
	}
	if code := run([]string{"uninstall", "--purge"}); code != exitOK || len(got) != 2 || !got[1].Purge || got[1].UserSID != "" {
		t.Fatalf("run uninstall --purge: exit code %d, %+v", code, got)
	}
	if code := run([]string{"uninstall"}); code != exitOK || len(got) != 3 || got[2].Purge {
		t.Fatalf("run uninstall: exit code %d, %+v", code, got)
	}

	for _, c := range []struct {
		cmd  string
		args []string
		want string
	}{
		{"install", nil, "--user-sid or --session-user is required"},
		{"install", []string{"--user-sid"}, "flag needs an argument: -user-sid"},
		{"install", []string{"--user-sid", sid, "extra"}, `unexpected "extra"`},
		{"install", []string{"--user-sid", sid, "--purge"}, "flag provided but not defined: -purge"},
		{"install", []string{"--dev", "--user-sid", sid}, "flag provided but not defined: -dev"},
		{"uninstall", []string{"--user-sid", sid}, "flag provided but not defined: -user-sid"},
		{"uninstall", []string{"--purge", "--data-dir", `C:\d`}, "flag provided but not defined: -data-dir"},
	} {
		out.Reset()
		label := strings.ToUpper(c.cmd) + "-REFUSED: "
		if code := admin(c.cmd, c.args, &out); code != exitUsage || !strings.HasPrefix(out.String(), label) || !strings.Contains(out.String(), c.want) {
			t.Errorf("%s %v: exit code %d, output %q", c.cmd, c.args, code, out.String())
		}
	}

	out.Reset()
	answer = errors.New("refused")
	if code := admin("uninstall", nil, &out); code != exitRefused {
		t.Fatalf("a refused uninstall: exit code %d", code)
	}
	programDir = func() (string, error) { return "", errors.New("no program folder") }
	out.Reset()
	calls := len(got)
	if code := admin("install", []string{"--user-sid", sid}, &out); code != exitRefused || len(got) != calls || out.String() != "INSTALL-REFUSED: no program folder\n" {
		t.Fatalf("without the program folder: exit code %d, output %q", code, out.String())
	}
}

// prepare (spec 3) takes --session-user and nothing else; the user's folder of --purge is the process
// user's roaming folder, filled for uninstall only.
func TestPrepareFlags(t *testing.T) {
	prog, _ := fakePlace(t)
	var got []install.Options
	old := adminCommands
	adminCommands = map[string]func(install.Options, install.Sys) error{}
	for name := range old {
		adminCommands[name] = func(o install.Options, s install.Sys) error {
			got = append(got, o)
			return nil
		}
	}
	t.Cleanup(func() { adminCommands = old })

	var out strings.Builder
	if code := run([]string{"prepare", "--session-user"}); code != exitOK || len(got) != 1 || !got[0].SessionUser || got[0].UserSID != "" ||
		got[0].ProgramDir != prog || got[0].ProductDir != filepath.Dir(prog) {
		t.Fatalf("prepare --session-user: exit code %d, %+v", code, got)
	}
	if code := run([]string{"install", "--session-user"}); code != exitOK || len(got) != 2 || !got[1].SessionUser || got[1].UserSID != "" {
		t.Fatalf("install --session-user: exit code %d, %+v", code, got)
	}
	if code := run([]string{"uninstall", "--purge"}); code != exitOK || len(got) != 3 || !got[2].Purge || !strings.HasSuffix(got[2].UserDataDir, "SplitLane") {
		t.Fatalf("uninstall --purge: exit code %d, %+v", code, got)
	}
	for _, c := range []struct {
		cmd  string
		args []string
		want string
	}{
		{"prepare", nil, "--session-user is required"},
		{"prepare", []string{"--user-sid", "S-1-5-21-1000-2000-3000-1001"}, "flag provided but not defined: -user-sid"},
		{"prepare", []string{"--session-user", "--purge"}, "flag provided but not defined: -purge"},
		{"prepare", []string{"--session-user", "extra"}, `unexpected "extra"`},
		{"uninstall", []string{"--session-user"}, "flag provided but not defined: -session-user"},
	} {
		out.Reset()
		if code := admin(c.cmd, c.args, &out); code != exitUsage || !strings.HasPrefix(out.String(), strings.ToUpper(c.cmd)+"-REFUSED: ") || !strings.Contains(out.String(), c.want) {
			t.Errorf("%s %v: exit code %d, output %q", c.cmd, c.args, code, out.String())
		}
	}
	if len(got) != 3 {
		t.Fatalf("a refused command line ran a command: %d", len(got))
	}
}

// --session-user and --user-sid exclude each other: a usage error, nothing runs.
func TestSessionUserAndUserSidExclude(t *testing.T) {
	fakePlace(t)
	calls := 0
	old := adminCommands
	adminCommands = map[string]func(install.Options, install.Sys) error{
		"install": func(install.Options, install.Sys) error { calls++; return nil },
	}
	t.Cleanup(func() { adminCommands = old })
	var out strings.Builder
	code := admin("install", []string{"--session-user", "--user-sid", "S-1-5-21-1000-2000-3000-1001"}, &out)
	if code != exitUsage || calls != 0 || !strings.HasPrefix(out.String(), "INSTALL-REFUSED: ") || !strings.Contains(out.String(), "--session-user and --user-sid exclude each other") {
		t.Fatalf("exit code %d, %d calls, output %q", code, calls, out.String())
	}
}

// The pipe check is the product's alone: --dev passes none, the product passes the check of its folder,
// and a product folder that cannot be found is an error (the worker then exits with exitInternal).
func TestPipeCheckByMode(t *testing.T) {
	if c, err := pipeCheck(true); c != nil || err != nil {
		t.Fatalf("--dev: %v %v", c != nil, err)
	}
	fakePlace(t)
	if c, err := pipeCheck(false); c == nil || err != nil {
		t.Fatalf("product: %v %v", c != nil, err)
	}
	old := productDir
	defer func() { productDir = old }()
	productDir = func() (string, error) { return "", errors.New("no product folder") }
	if c, err := pipeCheck(false); c != nil || err == nil {
		t.Fatalf("a failing product folder is an error: %v %v", c != nil, err)
	}
}
