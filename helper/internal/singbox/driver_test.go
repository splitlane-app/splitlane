package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
	"splitlane/helper/internal/winhost"
)

// TestMain lets the test binary play the core: started with SPLITLANE_FAKE_CORE=1 it reads the
// config, writes the start line into the log the config names, and waits for CTRL_BREAK; as
// `api ...` it plays the CLI of the API service (fakeAPICommand); as `helper` it plays the helper
// that pings through the real runner (fakeHelper).
func TestMain(m *testing.M) {
	if os.Getenv("SPLITLANE_FAKE_CORE") == "1" {
		fakeCore(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

func fakeCore(args []string) {
	if args[0] == "api" {
		fakeAPICommand(args[1:])
		return
	}
	if args[0] == "helper" {
		fakeHelper()
		return
	}
	config, _ := os.ReadFile(args[2])
	if args[0] == "check" {
		// the environment the check got, for TestCoreAndCLIGetTheSystemEnvironment
		os.WriteFile(args[2]+".env", []byte(strings.Join(os.Environ(), "\n")), 0o644)
		if bytes.Contains(config, []byte("bad")) {
			os.Stdout.WriteString("FATAL decode config: bad field\n")
			os.Exit(1)
		}
		os.Exit(0)
	}
	var c struct {
		Log struct {
			Output string `json:"output"`
		} `json:"log"`
	}
	json.Unmarshal(config, &c)
	appendLine := func(s string) {
		f, _ := os.OpenFile(c.Log.Output, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		f.WriteString(s + "\n")
		f.Close()
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	for _, kv := range os.Environ() {
		appendLine("ENV " + kv)
	}
	appendLine("INFO sing-box started (0.01s)")
	<-sig
	appendLine("INFO interrupted")
	os.Exit(0)
}

// fakeEnviron is the system's environment (winhost.SystemEnviron) plus the variable that makes the test
// binary play the core, and extra: the children get no other variable of the test process.
func fakeEnviron(extra ...string) func() ([]string, error) {
	return func() ([]string, error) {
		env, err := winhost.SystemEnviron()
		return append(append(env, "SPLITLANE_FAKE_CORE=1"), extra...), err
	}
}

func fakeDriver(t *testing.T) (*Driver, string) {
	t.Helper()
	dir := t.TempDir()
	p := coreconfig.PathsFor(dir)
	config := filepath.Join(dir, "config.json")
	b, _ := json.Marshal(map[string]any{"log": map[string]string{"output": filepath.ToSlash(p.CoreLog)}})
	os.WriteFile(config, b, 0o644)
	exe, _ := os.Executable()
	return &Driver{Exe: exe, DataDir: dir, Environ: fakeEnviron()}, config
}

func waitLog(t *testing.T, d *Driver, text string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(d.LogFrom(0), text) {
		if time.Now().After(deadline) {
			t.Fatalf("log never showed %q: %q", text, d.LogFrom(0))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestStartRotatesTheLogAndIdentifiesTheProcess(t *testing.T) {
	d, config := fakeDriver(t)
	p := coreconfig.PathsFor(d.DataDir)
	os.MkdirAll(p.LogsDir, 0o755)
	os.WriteFile(p.CoreLog, []byte("INFO sing-box started (old run)\n"), 0o644)
	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proc.Kill(); <-proc.Done() })
	if old, _ := os.ReadFile(p.CoreLog + ".prev"); !strings.Contains(string(old), "old run") {
		t.Fatal("the old log must be set aside")
	}
	waitLog(t, d, "sing-box started (0.01s)")
	if strings.Contains(d.LogFrom(0), "old run") {
		t.Fatal("the start line of an earlier run must not count")
	}
	id := proc.Identity()
	exe, _ := os.Executable()
	if id.PID <= 0 || id.Created == 0 || !strings.EqualFold(id.Path, exe) {
		t.Fatalf("identity %+v", id)
	}
	size := d.LogSize()
	if size == 0 || d.LogFrom(size) != "" {
		t.Fatalf("size %d, text after the end %q", size, d.LogFrom(size))
	}
}

// A setup core sets aside its own log only (spec 6.1): the log of a main core that failed stays for
// diagnostics.export however many setup runs follow (review of plan 1B, final, minor 4).
func TestStartSetupKeepsTheMainLog(t *testing.T) {
	d, _ := fakeDriver(t)
	p := coreconfig.PathsFor(d.DataDir)
	os.MkdirAll(p.LogsDir, 0o755)
	os.WriteFile(p.CoreLog, []byte("ERROR the main core failed\n"), 0o644)
	os.WriteFile(p.SetupLog, []byte("INFO sing-box started (old setup run)\n"), 0o644)
	config := filepath.Join(d.DataDir, "setup.json")
	b, _ := json.Marshal(map[string]any{"log": map[string]string{"output": filepath.ToSlash(p.SetupLog)}})
	os.WriteFile(config, b, 0o644)
	for run := 0; run < 2; run++ {
		proc, err := d.StartSetup(config)
		if err != nil {
			t.Fatal(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for !d.SetupStarted() {
			if time.Now().After(deadline) {
				t.Fatal("the setup core's start line never came")
			}
			time.Sleep(20 * time.Millisecond)
		}
		proc.Kill()
		<-proc.Done()
	}
	if main, _ := os.ReadFile(p.CoreLog); string(main) != "ERROR the main core failed\n" {
		t.Fatalf("the main core's log %q", main)
	}
	if _, err := os.Stat(p.CoreLog + ".prev"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a setup run sets the main log aside: %v", err)
	}
	if prev, _ := os.ReadFile(p.SetupLog + ".prev"); strings.Contains(string(prev), "old setup run") || !strings.Contains(string(prev), "sing-box started (0.01s)") {
		t.Fatalf("the setup log of the run before %q", prev)
	}
	if d.Events(0).Started {
		t.Fatal("the start line of a setup core is not one of the main core")
	}
}

func TestFindOnlyTheSameProcess(t *testing.T) {
	d, config := fakeDriver(t)
	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	waitLog(t, d, "started")
	id := proc.Identity()
	other := store.CoreIdentity{PID: id.PID, Created: id.Created + 1, Path: id.Path}
	if _, ok := d.Find(other); ok {
		t.Fatal("another creation time is another process")
	}
	found, ok := d.Find(id)
	if !ok {
		t.Fatal("own core not found")
	}
	if err := found.Kill(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-found.Done():
	case <-time.After(10 * time.Second):
		t.Fatal("found process did not end")
	}
	<-proc.Done()
	if _, ok := d.Find(id); ok {
		t.Fatal("an ended process must not be found")
	}
}

// 5.7: the own core is also the image of the driver's sing-box.exe, in the product the one in
// %ProgramFiles%\SplitLane\. The same process seen by a driver of another sing-box.exe is not its core;
// the driver's path written with forward slashes is the same file.
func TestFindOnlyTheDriversCore(t *testing.T) {
	d, config := fakeDriver(t)
	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proc.Kill(); <-proc.Done() })
	waitLog(t, d, "started")
	other := &Driver{Exe: filepath.Join(t.TempDir(), "sing-box.exe"), DataDir: d.DataDir}
	if _, ok := other.Find(proc.Identity()); ok {
		t.Fatal("a process of another image is not the driver's core")
	}
	slashes := &Driver{Exe: filepath.ToSlash(d.Exe), DataDir: d.DataDir}
	if _, ok := slashes.Find(proc.Identity()); !ok {
		t.Fatal("the driver's own exe written with forward slashes")
	}
}

func TestCheckReportsTheCoreMessage(t *testing.T) {
	d, config := fakeDriver(t)
	if err := d.Check(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(config, []byte(`{"bad":true}`), 0o644)
	if err := d.Check(context.Background(), config); err == nil || !strings.Contains(err.Error(), "bad field") {
		t.Fatalf("got %v", err)
	}
}

// Review F13-2: the core, its check and its CLI run elevated, so they get the system's own
// environment and what they need (BOX_API_* for the CLI), never the user's: a TS_* knob, a proxy or
// GODEBUG of the user would steer them. SystemRoot is the system's even when the helper's is not:
// a task started as the user takes it from HKCU\Environment too.
func TestCoreAndCLIGetTheSystemEnvironment(t *testing.T) {
	d, config := fakeDriver(t)
	t.Setenv("TS_DEBUG_PLANTED", "user")
	t.Setenv("SystemRoot", `X:\planted`)
	windowsDir, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		t.Fatal(err)
	}
	clean := func(who string, env []string) map[string]string {
		t.Helper()
		got := map[string]string{}
		for _, kv := range env {
			k, v, _ := strings.Cut(kv, "=")
			got[strings.ToUpper(k)] = v
		}
		if _, ok := got["TS_DEBUG_PLANTED"]; ok {
			t.Errorf("%s: the user's TS_DEBUG_PLANTED reached it", who)
		}
		if !strings.EqualFold(got["SYSTEMROOT"], windowsDir) || strings.Contains(strings.Join(env, "\n"), `X:\planted`) {
			t.Errorf("%s: SystemRoot %q, want %q and no trace of the planted one", who, got["SYSTEMROOT"], windowsDir)
		}
		return got
	}

	out, err := d.Node(40004, testAPISecret).(*Node).tailscale(context.Background(), "env")
	if err != nil {
		t.Fatal(err)
	}
	cli := clean("the CLI", lines(out))
	if cli["BOX_API_URL"] != "127.0.0.1:40004" || cli["BOX_API_SECRET"] != testAPISecret {
		t.Errorf("the CLI lacks BOX_API_*: %q %q", cli["BOX_API_URL"], cli["BOX_API_SECRET"])
	}

	if err := d.Check(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	checked, err := os.ReadFile(config + ".env")
	if err != nil {
		t.Fatal(err)
	}
	clean("the check", lines(checked))

	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { proc.Kill(); <-proc.Done() })
	waitLog(t, d, "sing-box started")
	var core []string
	for _, line := range lines([]byte(d.LogFrom(0))) {
		if kv, ok := strings.CutPrefix(line, "ENV "); ok {
			core = append(core, kv)
		}
	}
	clean("the core", core)
}

// The engine's operations in sing-box terms (spec 4): the modes of 6.3, the log lines of 5.3 and
// 8.3, the config of spec 6 in the driver's data folder, the rule files of 6.5.
func TestDriverSpeaksSingBox(t *testing.T) {
	d := &Driver{Exe: `C:\core\sing-box.exe`, DataDir: t.TempDir()}
	for _, c := range []struct {
		lane          string
		all, degraded bool
		want          string
	}{
		{"vpn", false, false, "vpn"}, {"vpn", false, true, "vpn-localdns"}, {"direct", false, false, "direct"},
		{"direct", false, true, "direct-localdns"}, {"direct", true, true, "alldirect"},
	} {
		if got := d.Mode(c.lane, c.all, c.degraded); got != c.want {
			t.Errorf("%+v: %s", c, got)
		}
	}
	p := coreconfig.PathsFor(d.DataDir)
	os.MkdirAll(p.LogsDir, 0o755)
	os.WriteFile(p.CoreLog, []byte("INFO sing-box started (0.42s)\n"), 0o644)
	mark := d.LogSize()
	if ev := d.Events(0); !ev.Started || ev.NeedLogin || ev.ReloadFailed {
		t.Fatalf("start: %+v", ev)
	}
	f, _ := os.OpenFile(p.CoreLog, os.O_APPEND|os.O_WRONLY, 0o644)
	f.WriteString("INFO endpoint/tailscale[vpn]: Waiting for authentication: https://login.example/x\n" +
		"ERROR router: reload rule-set lane-direct: parse error\n")
	f.Close()
	if ev := d.Events(mark); ev.Started || !ev.NeedLogin || !ev.ReloadFailed {
		t.Fatalf("after the mark: %+v", ev)
	}
	config, err := d.BuildConfig(engine.CoreOptions{Mode: "vpn-localdns", HelperPath: `C:\SplitLane\splitlane-helper.exe`,
		TailscaleDir: `C:\Program Files\Tailscale`, Hostname: "pc-splitlane", ExitNode: "100.64.0.1",
		Ports:       engine.Ports{ProbeVPN: 40001, ProbeDirect: 40002, Clash: 40003, API: 40004},
		ClashSecret: "0123456789abcdef", APISecret: "fedcba9876543210", LogLevel: "debug"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"default_mode": "vpn-localdns"`, `"exit_node": "100.64.0.1"`, `"listen_port": 40002`, `"level": "debug"`,
		`"output": "` + filepath.ToSlash(p.CoreLog) + `"`, `"path": "` + filepath.ToSlash(p.RulesDirect) + `"`} {
		if !strings.Contains(string(config), want) {
			t.Errorf("config lacks %s", want)
		}
	}
	direct, vpn, err := d.LaneFiles(nil, rules.Env{})
	if err != nil || !strings.Contains(string(direct), rules.Stub) || !strings.Contains(string(vpn), rules.Stub) {
		t.Fatalf("lane files: %v\n%s\n%s", err, direct, vpn)
	}
}

func TestVerifyCore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sing-box.exe")
	os.WriteFile(path, []byte("not the core"), 0o644)
	if err := VerifyCore(path); err == nil {
		t.Fatal("a wrong file must fail")
	}
	if real := os.Getenv("SPLITLANE_CORE"); real != "" {
		if err := VerifyCore(real); err != nil {
			t.Fatal(err)
		}
	}
}

// TestInterruptStopsTheCore sends the real CTRL_BREAK through the core's hidden console. The test
// process lets its own console go for a moment, so it runs only with SPLITLANE_CONSOLE_TEST=1.
func TestInterruptStopsTheCore(t *testing.T) {
	if os.Getenv("SPLITLANE_CONSOLE_TEST") != "1" {
		t.Skip("SPLITLANE_CONSOLE_TEST is not set")
	}
	d, config := fakeDriver(t)
	proc, err := d.Start(config)
	if err != nil {
		t.Fatal(err)
	}
	waitLog(t, d, "started")
	if err := proc.Interrupt(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-proc.Done():
	case <-time.After(5 * time.Second):
		proc.Kill()
		t.Fatal("the core did not stop within 5 s after CTRL_BREAK")
	}
	waitLog(t, d, "interrupted")
}
