package singbox

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"splitlane/helper/internal/coreconfig"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/rules"
	"splitlane/helper/internal/store"
)

// TestMain lets the test binary play the core: started with SPLITLANE_FAKE_CORE=1 it reads the
// config, writes the start line into the log the config names, and waits for CTRL_BREAK.
func TestMain(m *testing.M) {
	if os.Getenv("SPLITLANE_FAKE_CORE") == "1" {
		fakeCore(os.Args[1:])
		return
	}
	os.Exit(m.Run())
}

func fakeCore(args []string) {
	config, _ := os.ReadFile(args[2])
	if args[0] == "check" {
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
	appendLine("INFO sing-box started (0.01s)")
	<-sig
	appendLine("INFO interrupted")
	os.Exit(0)
}

func fakeDriver(t *testing.T) (*Driver, string) {
	t.Helper()
	t.Setenv("SPLITLANE_FAKE_CORE", "1")
	dir := t.TempDir()
	p := coreconfig.PathsFor(dir)
	config := filepath.Join(dir, "config.json")
	b, _ := json.Marshal(map[string]any{"log": map[string]string{"output": filepath.ToSlash(p.CoreLog)}})
	os.WriteFile(config, b, 0o644)
	exe, _ := os.Executable()
	return &Driver{Exe: exe, DataDir: dir}, config
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
		ClashSecret: "0123456789abcdef", APISecret: "fedcba9876543210"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"default_mode": "vpn-localdns"`, `"exit_node": "100.64.0.1"`, `"listen_port": 40002`,
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
