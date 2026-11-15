// Package singbox is the one core driver of stage 1 (spec 4): the unmodified sing-box 1.14.4
// behind engine.CoreDriver. It builds the config (package coreconfig) and the lane files (package
// rules), names the modes, runs the processes, reads the log, talks to the Clash API (5.3–5.5) and
// to the API service (spec 7), and builds the config of the setup mode.
package singbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/connector"
	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
	"splitlane/helper/internal/winhost"
)

// CoreVersion is the only core version the configs are built for (spec 6).
const CoreVersion = "1.14.4"

// CoreSHA256 is the SHA-256 of sing-box.exe from sing-box-1.14.4-windows-amd64.zip (spec 4.1, 4.3).
const CoreSHA256 = "8f30682ee198855440d2a7fe070d5d3653793b2d01bb8c85eea88f93bbe7c8b9"

// maxLogRead bounds one read of the core log.
const maxLogRead = 1 << 20

// VerifyCore compares the SHA-256 of exe with CoreSHA256.
func VerifyCore(exe string) error {
	f, err := os.Open(exe)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != CoreSHA256 {
		return fmt.Errorf("%s has SHA-256 %s, want %s", exe, got, CoreSHA256)
	}
	return nil
}

// Driver implements engine.CoreDriver for one sing-box.exe and one data folder.
type Driver struct {
	Exe     string
	DataDir string
	// Environ is the environment of the core, its check and its CLI: nil is the system's own
	// (winhost.SystemEnviron). Tests add the variable that makes the test binary play the core.
	Environ func() ([]string, error)
}

var _ engine.CoreDriver = (*Driver)(nil)

func (d *Driver) environ() ([]string, error) {
	if d.Environ != nil {
		return d.Environ()
	}
	return winhost.SystemEnviron()
}

// BuildConfig is the config of spec 6 for the options, with the files of the driver's data folder.
func (d *Driver) BuildConfig(o engine.CoreOptions) ([]byte, error) {
	return coreconfig.Build(coreconfig.Options{
		Mode: o.Mode, DataDir: d.DataDir, HelperPath: o.HelperPath, TailscaleDir: o.TailscaleDir,
		Lane: lane(o.Lane), Ports: coreconfig.Ports(o.Ports),
		ClashSecret: o.ClashSecret, APISecret: o.APISecret, DevPin: o.DevPin, LogLevel: o.LogLevel,
	})
}

// lane is the engine's lane for coreconfig, field by field; coreconfig refuses a lane with no
// connector or two.
func lane(l engine.Lane) coreconfig.Lane {
	out := coreconfig.Lane{Proxy: l.Proxy}
	if l.Tailscale != nil {
		t := coreconfig.TailscaleLane(*l.Tailscale)
		out.Tailscale = &t
	}
	return out
}

// LaneFiles are the rule-set files (format 3) of both lanes (6.5, 8.2).
func (d *Driver) LaneFiles(apps []protocol.AppRule, env rules.Env) ([]byte, []byte, error) {
	return rules.Files(apps, env)
}

// Mode is the clash mode of spec 6.3.
func (d *Driver) Mode(defaultLane string, allDirect, degraded bool) string {
	return coreconfig.ModeFor(defaultLane, allDirect, degraded)
}

// Events reads the log after offset: the start line of this run, the login request of the
// embedded node and a lane file that could not be read again.
func (d *Driver) Events(offset int64) engine.CoreEvents {
	text := d.LogFrom(offset)
	return engine.CoreEvents{
		Started:      strings.Contains(text, coreconfig.StartLine),
		NeedLogin:    strings.Contains(text, coreconfig.LoginLine),
		ReloadFailed: strings.Contains(text, coreconfig.ReloadErrorLine),
	}
}

func hidden(flags uint32) *syscall.SysProcAttr {
	return &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
}

// Check runs `sing-box check`; the core reads the rule files already here (spec 12).
func (d *Driver) Check(ctx context.Context, configPath string) error {
	env, err := d.environ()
	if err != nil {
		return fmt.Errorf("sing-box check: the system environment: %w", err)
	}
	cmd := exec.CommandContext(ctx, d.Exe, "check", "-c", configPath, "-D", d.DataDir)
	cmd.Env = env
	cmd.SysProcAttr = hidden(windows.CREATE_NO_WINDOW)
	out, err := cmd.CombinedOutput()
	if err != nil {
		// masked before it is cut: a cut could leave the head of a secret that no rule matches
		text := connector.Mask(strings.TrimSpace(string(out)))
		if len(text) > 400 {
			text = text[:400]
		}
		return fmt.Errorf("sing-box check: %v: %s", err, text)
	}
	return nil
}

// Start sets the old log aside, then starts the core in a process group of its own, with a
// hidden console, the system's environment and its output in core-stdout.log (spec 5.3 step 4,
// 5.4 step 2).
func (d *Driver) Start(configPath string) (engine.CoreProc, error) {
	return d.start(configPath, coreconfig.PathsFor(d.DataDir).CoreLog)
}

// StartSetup is Start for the setup core (spec 7), whose config writes its own log: the log of a
// main core that failed stays for diagnostics.export.
func (d *Driver) StartSetup(configPath string) (engine.CoreProc, error) {
	return d.start(configPath, coreconfig.PathsFor(d.DataDir).SetupLog)
}

// SetupStarted: the setup core's log has its start line.
func (d *Driver) SetupStarted() bool {
	return strings.Contains(readLog(coreconfig.PathsFor(d.DataDir).SetupLog, 0), coreconfig.StartLine)
}

// start sets log, the one the config names, aside and starts the core.
func (d *Driver) start(configPath, log string) (engine.CoreProc, error) {
	env, err := d.environ()
	if err != nil {
		return nil, fmt.Errorf("the system environment for the core: %w", err)
	}
	p := coreconfig.PathsFor(d.DataDir)
	if err := os.MkdirAll(p.LogsDir, 0o755); err != nil {
		return nil, err
	}
	if err := setAside(log); err != nil {
		return nil, err
	}
	out, err := os.OpenFile(p.CoreStdout, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(d.Exe, "run", "-c", configPath, "-D", d.DataDir)
	cmd.Env = env
	cmd.Stdout, cmd.Stderr = out, out
	cmd.SysProcAttr = hidden(windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW)
	if err := cmd.Start(); err != nil {
		out.Close()
		return nil, err
	}
	pid := uint32(cmd.Process.Pid)
	id, err := identify(pid)
	if err != nil {
		id = store.CoreIdentity{PID: int(pid), Path: d.Exe}
	}
	proc := &process{id: id, done: make(chan struct{}), kill: cmd.Process.Kill}
	go func() {
		_ = cmd.Wait()
		out.Close()
		close(proc.done)
	}()
	return proc, nil
}

// setAside renames the previous log to .prev: the start line must come from this run (5.3 step 4).
func setAside(log string) error {
	if _, err := os.Stat(log); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	_ = os.Remove(log + ".prev")
	var err error
	for i := 0; i < 20; i++ {
		if err = os.Rename(log, log+".prev"); err == nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	if rmErr := os.Remove(log); rmErr == nil {
		return nil
	}
	return fmt.Errorf("the previous core log cannot be set aside: %w", err)
}

// Find opens the core of an earlier run by pid, creation time and image path (5.7): the image must be
// the one state.json names and the driver's own sing-box.exe, in the product the one in
// %ProgramFiles%\SplitLane\.
func (d *Driver) Find(id store.CoreIdentity) (engine.CoreProc, bool) {
	if id.PID <= 0 {
		return nil, false
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE|windows.PROCESS_TERMINATE, false, uint32(id.PID))
	if err != nil {
		return nil, false
	}
	created, path, err := processInfo(h)
	if err != nil || created != id.Created || !place.PathEqual(path, id.Path) || !place.PathEqual(path, d.Exe) {
		windows.CloseHandle(h)
		return nil, false
	}
	proc := &process{id: id, done: make(chan struct{})}
	var mu sync.Mutex
	closed := false
	proc.kill = func() error {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return nil
		}
		return windows.TerminateProcess(h, 1)
	}
	go func() {
		_, _ = windows.WaitForSingleObject(h, windows.INFINITE)
		mu.Lock()
		closed = true
		windows.CloseHandle(h)
		mu.Unlock()
		close(proc.done)
	}()
	return proc, true
}

// LogSize is the size of the current core log.
func (d *Driver) LogSize() int64 {
	fi, err := os.Stat(coreconfig.PathsFor(d.DataDir).CoreLog)
	if err != nil {
		return 0
	}
	return fi.Size()
}

// LogFrom is the text of the current core log after offset, at most 1 MiB.
func (d *Driver) LogFrom(offset int64) string {
	return readLog(coreconfig.PathsFor(d.DataDir).CoreLog, offset)
}

// readLog is the text of a log after offset, at most 1 MiB.
func readLog(path string, offset int64) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return ""
	}
	b, _ := io.ReadAll(io.LimitReader(f, maxLogRead))
	return string(b)
}

// Clash returns the Clash API client of the run that listens on port with secret.
func (d *Driver) Clash(port int, secret string) engine.ClashAPI {
	return NewClash(fmt.Sprintf("http://127.0.0.1:%d", port), secret)
}

// BuildSetupConfig is the config of the setup mode (spec 7), with the files of the driver's data folder.
func (d *Driver) BuildSetupConfig(o engine.SetupOptions) ([]byte, error) {
	return coreconfig.BuildSetup(coreconfig.SetupOptions{
		DataDir: d.DataDir, Hostname: o.Hostname, APIPort: o.APIPort, APISecret: o.APISecret, LogLevel: o.LogLevel,
	})
}

// Node returns the client of the API service of the run that listens on port with secret; its CLI
// gets the driver's environment.
func (d *Driver) Node(port int, secret string) engine.NodeAPI {
	n := NewNode(d.Exe, port, secret)
	n.Run = runIn(inHelperJob, d.environ)
	return n
}

type process struct {
	id   store.CoreIdentity
	done chan struct{}
	kill func() error
}

func (p *process) Identity() store.CoreIdentity { return p.id }
func (p *process) Done() <-chan struct{}        { return p.done }
func (p *process) Interrupt() error             { return CtrlBreak(uint32(p.id.PID)) }
func (p *process) Kill() error                  { return p.kill() }

func identify(pid uint32) (store.CoreIdentity, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return store.CoreIdentity{}, err
	}
	defer windows.CloseHandle(h)
	created, path, err := processInfo(h)
	if err != nil {
		return store.CoreIdentity{}, err
	}
	return store.CoreIdentity{PID: int(pid), Created: created, Path: path}, nil
}

// processInfo returns the creation time (FILETIME, 100 ns units) and the image path of a process.
func processInfo(h windows.Handle) (int64, string, error) {
	var created, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exit, &kernel, &user); err != nil {
		return 0, "", err
	}
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return 0, "", err
	}
	return int64(created.HighDateTime)<<32 | int64(created.LowDateTime), windows.UTF16ToString(buf[:size]), nil
}
