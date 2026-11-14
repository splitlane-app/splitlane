// Command splitlane-helper is the elevated helper of SplitLane (spec 5). It is built without a
// console window (-H=windowsgui) and starts the core without a window.
//
// Without flags it is the product (spec 4.2, 4.3): the scheduled task "SplitLane Helper" starts
// <ProgramDir>\splitlane-helper.exe; the data folder is %ProgramData%\SplitLane, the core is
// <ProgramDir>\sing-box.exe, the user of the pipe comes from setup.json, and a helper that no window
// reaches ends 60 s after the start-up. For development it runs from an elevated terminal with paths
// in the repository: --dev --data-dir <folder> --core <sing-box.exe>.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/probe"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/session"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
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
)

const pipeTakenLabel = "the pipe already exists: another helper runs"

// idleExit: a product helper that no window reaches ends so long after the start-up (spec 4.3).
const idleExit = 60 * time.Second

// The places of the product (package place) and the running exe; the tests replace them, so that no
// test reads the registry or the known folders or touches %ProgramData%.
var (
	programDir  = place.ProgramDir
	productData = place.DataDir
	executable  = os.Executable
)

type options struct {
	dev          bool
	dataDir      string
	core         string
	pipe         string
	devPin       bool
	coreLogLevel string
}

func parse(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("splitlane-helper", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&o.dev, "dev", false, "development mode: the paths below; without it the helper is the product")
	fs.StringVar(&o.dataDir, "data-dir", "", "development only: data folder (absolute)")
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
	o, err := parse(args)
	if err != nil {
		return exitUsage
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
			code := "-"
			if s.Reason != nil {
				code = s.Reason.Code
			}
			logf("state: phase=%s reason=%s allDirect=%v", s.Phase, code, s.AllDirect)
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
	// changes come a burst at a time. The engine follows them only while routing is on.
	watcher, err := session.Watch(session.Handlers{
		OnEnd:   eng.SessionEnding,
		OnPower: func(event string) { go eng.NetworkChanged(event) },
	}, logf)
	if err != nil {
		logf("session end and sleep will not be noticed: %v", err)
	}
	stopRoutes, err := winhost.WatchRoutes(func() { eng.NetworkChanged("route") }, logf)
	if err != nil {
		logf("route changes will not be noticed: %v", err)
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
	s, err := place.ReadSetup(dataDir)
	if err != nil {
		logf("not installed: %v; the window runs install first (spec 4.3)", err)
		return "", "", exitNotInstalled
	}
	return filepath.Join(dir, "sing-box.exe"), s.UserSID, exitOK
}
