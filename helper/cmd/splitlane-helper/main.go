// Command splitlane-helper is the elevated helper of SplitLane (spec 5). It is built without a
// console window (-H=windowsgui) and starts the core without a window.
//
// This build (Plan 1A) runs only in development mode, from an elevated process, with paths in the
// repository: --dev --data-dir <folder> --core <sing-box.exe>. The install command and the
// scheduled task of spec 4.3 come with Plan 1B.
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
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/pipe"
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
	exitOK        = 0
	exitUsage     = 2
	exitPipeTaken = 3
	exitBadCore   = 4
	exitInternal  = 5
)

const pipeTakenLabel = "the pipe already exists: another helper runs"

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
	fs.BoolVar(&o.dev, "dev", false, "development mode (required in this build)")
	fs.StringVar(&o.dataDir, "data-dir", "", "data folder (absolute)")
	fs.StringVar(&o.core, "core", "", "full path of sing-box.exe 1.14.4")
	fs.StringVar(&o.pipe, "pipe", pipe.DefaultName, "pipe name")
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
	case o.coreLogLevel != coreconfig.LogInfo && o.coreLogLevel != coreconfig.LogDebug:
		return o, fmt.Errorf("--core-log-level must be info or debug")
	case !o.dev:
		return o, fmt.Errorf("this build runs only with --dev")
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

	if err := singbox.VerifyCore(o.core); err != nil {
		logf("core refused: %v", err)
		return exitBadCore
	}
	helperPath, err := os.Executable()
	if err != nil {
		logf("own path: %v", err)
		return exitInternal
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		logf("own user: %v", err)
		return exitInternal
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
	srv, err = pipe.Listen(o.pipe, user.User.Sid.String(), eng, logf)
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
	<-exit
	logf("helper.shutdown done; exiting")
	cancel()
	srv.Close()
	watcher.Stop()
	stopRoutes()
	return exitOK
}
