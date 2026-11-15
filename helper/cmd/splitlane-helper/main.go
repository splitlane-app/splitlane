// Command splitlane-helper is the elevated helper of SplitLane (spec 5). It is built without a
// console window (-H=windowsgui) and starts the core without a window.
//
// Without flags it is the product (spec 4.2, 4.3): the scheduled task "SplitLane Helper" starts
// <ProgramDir>\splitlane-helper.exe (ProgramDir is %ProgramFiles%\SplitLane\helper, the window is at the
// root of %ProgramFiles%\SplitLane); the data folder is %ProgramData%\SplitLane, the core is
// <ProgramDir>\sing-box.exe, the user of the pipe comes from setup.json, and a helper that no window
// reaches ends 60 s after the start-up. For development it runs from an elevated terminal with paths
// in the repository: --dev --data-dir <folder> --core <sing-box.exe>.
//
// The window runs the setup with UAC from its own copy, dist\helper (package install, spec 4.3):
//
//	splitlane-helper.exe install --user-sid <SID>
//	splitlane-helper.exe uninstall [--purge]
//
// Their lines go to the standard output (the window redirects it); a refusal exits with code 7.
//
// The product is two processes (spec 4): the scheduled task starts the supervisor, which runs this exe
// again with --worker and starts it anew after a failure (package supervise); the worker is what the
// text above describes. The supervisor opens no pipe, starts no core and keeps no file open.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"maps"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/diag"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/install"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/probe"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/session"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
	"splitlane/helper/internal/supervise"
	"splitlane/helper/internal/systs"
	"splitlane/helper/internal/winhost"
)

var version = "0.1.0-dev"

// Exit codes; the process has no console, so they and helper.log are its only output.
const (
	exitOK           = 0
	exitUsage        = 2
	exitPipeTaken    = 3
	exitBadCore      = 4
	exitInternal     = 5
	exitNotInstalled = 6 // the product without its data folder or setup.json: install has not run
	exitRefused      = 7 // install or uninstall refused; the reason is on its -REFUSED line
)

const pipeTakenLabel = "the pipe already exists: another helper runs"

// idleExit: a product helper that no window reaches ends so long after the start-up (spec 4.3).
const idleExit = 60 * time.Second

// The places of the product (package place) and the running exe; the tests replace them, so that no
// test reads the registry or the known folders or touches %ProgramData%.
var (
	programDir  = place.ProgramDir
	productData = place.DataDir
	readSetup   = place.ReadSetup
	executable  = os.Executable
)

// The supervisor (spec 4): startWorker runs this exe with the arguments, superviseClock paces the
// restarts; the tests replace both, so that no test starts a process or waits.
var (
	startWorker    = startWorkerProcess
	superviseClock supervise.Clock = realClock{}
)

// setupReads is how many times the product reads setup.json, setupRetryWait apart (the tests shorten
// it): at logon another program may hold the file for a moment, and a helper that ends there leaves the
// journal unrestored and the system exit node off after a reboot. Only a missing file is "not installed".
const setupReads = 3

var setupRetryWait = 200 * time.Millisecond

// adminCommands are the subcommands of the setup (spec 4.3) and newSys the system they change; the
// tests replace both.
var (
	adminCommands = map[string]func(install.Options, install.Sys) error{
		"install":   install.Install,
		"uninstall": install.Uninstall,
	}
	newSys = func() install.Sys { return install.NewSys() }
)

type options struct {
	dev          bool
	dataDir      string
	core         string
	pipe         string
	devPin       bool
	worker       bool
	coreLogLevel string
}

func parse(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("splitlane-helper", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.dev, "dev", false, "development mode: the paths below; without it the helper is the product")
	fs.StringVar(&o.dataDir, "data-dir", "", "development only: data folder (absolute)")
	fs.BoolVar(&o.worker, "worker", false, "product only: run as the worker the supervisor starts")
	fs.StringVar(&o.core, "core", "", "development only: full path of sing-box.exe 1.14.4")
	fs.StringVar(&o.pipe, "pipe", pipe.DefaultName, "development only: pipe name")
	fs.BoolVar(&o.devPin, "dev-pin", false, "development only: keep the AI assistant's programs on the VPN lane")
	fs.StringVar(&o.coreLogLevel, "core-log-level", coreconfig.LogInfo, "development only: the core's log level, info or debug (spec 6.1)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	given := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { given[f.Name] = true })
	switch {
	case o.worker && o.dev:
		return o, fmt.Errorf("--worker needs the product: it cannot go with --dev, which has no supervisor")
	case given["core-log-level"] && !o.dev:
		return o, fmt.Errorf("--core-log-level needs --dev: the product always writes the core log at level info")
	case given["dev-pin"] && !o.dev:
		return o, fmt.Errorf("--dev-pin needs --dev: the product never pins the AI assistant's programs to a lane")
	case o.coreLogLevel != coreconfig.LogInfo && o.coreLogLevel != coreconfig.LogDebug:
		return o, fmt.Errorf("--core-log-level must be info or debug")
	case !o.dev && (given["data-dir"] || given["core"] || given["pipe"]):
		return o, fmt.Errorf("--data-dir, --core and --pipe need --dev: the product takes its folders and its pipe from the system (spec 4.2)")
	case !o.dev:
		return o, nil // the product: run finds its folders (spec 4.2, 4.3)
	case !filepath.IsAbs(o.dataDir) || !filepath.IsAbs(o.core):
		return o, fmt.Errorf("--data-dir and --core must be absolute paths")
	}
	return o, nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

type realClock struct{}

func (realClock) Now() time.Time                         { return time.Now() }
func (realClock) After(d time.Duration) <-chan time.Time { return time.After(d) }

func run(args []string) int {
	if len(args) > 0 && adminCommands[args[0]] != nil {
		return admin(args[0], args[1:], os.Stdout)
	}
	o, err := parse(args)
	if err != nil {
		return exitUsage
	}
	if !o.dev && !o.worker {
		return supervisor()
	}
	if !o.dev {
		// The product: the data folder is the one install made with its access list (spec 4.2); the
		// helper never makes it, so without it there is nowhere to write even the log.
		if o.dataDir, err = productData(); err != nil {
			return exitInternal
		}
		if fi, err := os.Stat(o.dataDir); err != nil || !fi.IsDir() {
			return exitNotInstalled
		}
	}
	paths := coreconfig.PathsFor(o.dataDir)
	if err := os.MkdirAll(paths.LogsDir, 0o755); err != nil {
		return exitUsage
	}
	logPath := filepath.Join(paths.LogsDir, "helper.log")
	_ = os.Rename(logPath, logPath+".prev")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return exitUsage
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags|log.Lmicroseconds)
	logf := func(format string, a ...any) { logger.Printf(format, a...) }
	logf("splitlane-helper %s starting, dev=%v dev-pin=%v core-log-level=%s", version, o.dev, o.devPin, o.coreLogLevel)

	helperPath, err := executable()
	if err != nil {
		logf("own path: %v", err)
		return exitInternal
	}
	userSID := ""
	if !o.dev {
		var code int
		if o.core, userSID, code = productPlace(helperPath, o.dataDir, logf); code != exitOK {
			return code
		}
	}
	if err := singbox.VerifyCore(o.core); err != nil {
		logf("core refused: %v", err)
		return exitBadCore
	}
	if o.dev {
		user, err := windows.GetCurrentProcessToken().GetTokenUser()
		if err != nil {
			logf("own user: %v", err)
			return exitInternal
		}
		userSID = user.User.Sid.String()
	}
	sys := systs.New()
	protected := []string{helperPath, o.core, filepath.Join(sys.Dir(), "tailscaled.exe"), filepath.Join(sys.Dir(), "tailscale-ipn.exe")}
	host, _ := os.Hostname()
	exit := make(chan struct{})
	var srv *pipe.Server
	var lastReason *protocol.Reason // the engine publishes under its lock, one state at a time
	eng := engine.New(engine.Deps{
		Clock: realClock{},
		Store: store.Dir{Path: o.dataDir},
		Core:  &singbox.Driver{Exe: o.core, DataDir: o.dataDir},
		Sys:   sys,
		Probe: probe.New(),
		Host:  winhost.Host{},
		Rules: winhost.RulesEnv(protected),
		Settings: engine.Settings{HelperPath: helperPath, Hostname: host + "-splitlane",
			DevPin: o.devPin, HelperVersion: version, CoreVersion: singbox.CoreVersion, CoreLogLevel: o.coreLogLevel},
		Log: logf,
		OnState: func(s protocol.State) {
			logf("%s", stateLine(s, lastReason))
			lastReason = s.Reason
			srv.Broadcast(s)
		},
		OnExit: func() { close(exit) },
	})
	srv, err = pipe.Listen(o.pipe, userSID, eng, logf)
	if err != nil {
		logf("%s (%v)", pipeTakenLabel, err)
		return exitPipeTaken
	}
	// A stray console event must never end the helper: it is handled here, not by the default handler.
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	go func() {
		for range interrupts {
			logf("console interrupt ignored")
		}
	}()
	// Section 10: sleep and wake come to the hidden window, whose procedure must answer at once; route
	// and interface changes come a burst at a time. The engine follows them only while routing is on.
	watcher, err := session.Watch(session.Handlers{
		OnEnd:   eng.SessionEnding,
		OnPower: func(event string) { go eng.NetworkChanged(event) },
	}, logf)
	if err != nil {
		logf("session end and sleep will not be noticed: %v", err)
	}
	stopRoutes, err := winhost.WatchRoutes(func() { eng.NetworkChanged("route") }, logf)
	if err != nil {
		logf("network changes will not be noticed: %v", err)
		stopRoutes = func() {}
	}
	ctx, cancel := context.WithCancel(context.Background())
	eng.Start()
	go eng.Run(ctx)
	go srv.Serve()
	go srv.RunActivity(eng, time.Second) // the activity event once a second (spec 8.4)
	if !o.dev {
		go eng.ExitIfIdle(idleExit) // spec 4.3: started at logon, the helper ends when no window comes
	}
	<-exit
	logf("helper.shutdown done; exiting")
	cancel()
	srv.Close()
	watcher.Stop()
	stopRoutes()
	return exitOK
}

// appendWriter writes every line to the end of the file at path: it opens the file for the line and closes
// it at once, so that nothing stays open; a file that cannot be opened (no logs folder) skips the line.
type appendWriter struct{ path string }

func (w appendWriter) Write(p []byte) (int, error) {
	f, err := os.OpenFile(w.path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return len(p), nil
	}
	defer f.Close()
	f.Write(p)
	return len(p), nil
}

// supervisor is the product without --worker (spec 4): the process the scheduled task starts. After the
// check of the place it runs this exe with --worker and starts it again after a failure. It opens no pipe,
// starts no core and neither makes nor rotates helper.log: its lines are appended to the worker's log in
// the same format, and without the logs folder they are skipped.
func supervisor() int {
	dataDir, err := productData()
	if err != nil {
		return exitInternal
	}
	if fi, err := os.Stat(dataDir); err != nil || !fi.IsDir() {
		return exitNotInstalled
	}
	logger := log.New(appendWriter{filepath.Join(coreconfig.PathsFor(dataDir).LogsDir, "helper.log")}, "", log.LstdFlags|log.Lmicroseconds)
	logf := func(format string, a ...any) { logger.Printf(format, a...) }
	helperPath, err := executable()
	if err != nil {
		logf("supervisor: own path: %v", err)
		return exitInternal
	}
	if _, _, code := productPlace(helperPath, dataDir, logf); code != exitOK {
		return code
	}
	return supervise.Run(func() (supervise.Worker, error) {
		return startWorker(helperPath, []string{"--worker"})
	}, superviseClock, logf)
}

// procWorker is a worker process the supervisor waits for.
type procWorker struct{ cmd *exec.Cmd }

func (w procWorker) Wait() int {
	w.cmd.Wait()
	if w.cmd.ProcessState == nil {
		return exitInternal
	}
	return w.cmd.ProcessState.ExitCode() // a crash code (NTSTATUS) comes as the unsigned number
}

// startWorkerProcess starts exe with args without a console window and with the standard streams on NUL.
// There is no job object (spec 4).
func startWorkerProcess(exe string, args []string) (supervise.Worker, error) {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return procWorker{cmd}, nil
}

// stateLine is the line of helper.log for a state the engine published: the phase, the code of its reason
// and allDirect, and the reason's params whenever the reason differs from last, the one of the state before
// (review F12-1): sorted, masked, the login links hidden (diag.ReasonParams). So the cause of
// restore_pending, config_invalid, core_start_timeout or switch_check_failed is in the archive.
func stateLine(s protocol.State, last *protocol.Reason) string {
	code := "-"
	if s.Reason != nil {
		code = s.Reason.Code
	}
	line := fmt.Sprintf("state: phase=%s reason=%s allDirect=%v", s.Phase, code, s.AllDirect)
	if r := s.Reason; r != nil && len(r.Params) > 0 && (last == nil || last.Code != r.Code || !maps.Equal(last.Params, r.Params)) {
		line += " params: " + diag.ReasonParams(r.Params)
	}
	return line
}

// productPlace checks that this helper is the product's own copy and finds the rest of the product
// (spec 4.2, 4.3, 11): the helper must run as <ProgramDir>\splitlane-helper.exe, where only
// administrators write, since it runs elevated; the core is <ProgramDir>\sing-box.exe; the user of
// the pipe is the one setup.json names. A refusal is logged and gives the exit code.
func productPlace(helperPath, dataDir string, logf func(string, ...any)) (core, userSID string, code int) {
	dir, err := programDir()
	if err != nil {
		logf("the program folder: %v", err)
		return "", "", exitInternal
	}
	want := filepath.Join(dir, "splitlane-helper.exe")
	if !strings.EqualFold(filepath.Clean(helperPath), want) {
		logf("refused: the product runs only as %s, this is %s (spec 11); a development run takes --dev", want, helperPath)
		return "", "", exitUsage
	}
	s, err := readSetup(dataDir)
	for i := 1; i < setupReads && err != nil && !errors.Is(err, os.ErrNotExist); i++ {
		time.Sleep(setupRetryWait)
		s, err = readSetup(dataDir)
	}
	switch {
	case errors.Is(err, os.ErrNotExist):
		logf("not installed: %v; the window runs install first (spec 4.3)", err)
		return "", "", exitNotInstalled
	case err != nil:
		logf("%s cannot be read (%d tries): %v", place.SetupFile, setupReads, err)
		return "", "", exitInternal
	}
	return filepath.Join(dir, "sing-box.exe"), s.UserSID, exitOK
}

var adminUsage = map[string]string{
	"install":   "splitlane-helper install --user-sid <SID>",
	"uninstall": "splitlane-helper uninstall [--purge]",
}

// parseAdmin reads the flags of a subcommand: --user-sid of install, which it needs, or --purge of
// uninstall; nothing else, no flag of the run mode.
func parseAdmin(cmd string, args []string) (install.Options, error) {
	var o install.Options
	fs := flag.NewFlagSet("splitlane-helper "+cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if cmd == "install" {
		fs.StringVar(&o.UserSID, "user-sid", "", "the SID of the user who runs the setup, S-1-...")
	} else {
		fs.BoolVar(&o.Purge, "purge", false, "remove the data folder too")
	}
	if err := fs.Parse(args); err != nil {
		return o, fmt.Errorf("%v; usage: %s", err, adminUsage[cmd])
	}
	switch {
	case fs.NArg() != 0:
		return o, fmt.Errorf("unexpected %q; usage: %s", fs.Arg(0), adminUsage[cmd])
	case cmd == "install" && o.UserSID == "":
		return o, fmt.Errorf("--user-sid is required; usage: %s", adminUsage[cmd])
	}
	return o, nil
}

// admin runs install or uninstall with its lines to out. The program and data folders come from the
// system as in the product (package place); the source of install is the folder of the running exe.
func admin(cmd string, args []string, out io.Writer) int {
	refused := strings.ToUpper(cmd) + "-REFUSED"
	o, err := parseAdmin(cmd, args)
	if err != nil {
		fmt.Fprintf(out, "%s: %v\n", refused, err)
		return exitUsage
	}
	if o.ProgramDir, err = programDir(); err == nil {
		if o.DataDir, err = productData(); err == nil {
			o.Exe, err = executable()
		}
	}
	if err != nil {
		fmt.Fprintf(out, "%s: %v\n", refused, err)
		return exitRefused
	}
	o.Hostname, _ = os.Hostname()
	o.Out = out
	if adminCommands[cmd](o, newSys()) != nil {
		return exitRefused // the command printed its -REFUSED line
	}
	return exitOK
}
