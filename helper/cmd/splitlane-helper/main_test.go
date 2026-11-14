package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
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
// other run without --dev stops at once.
func init() {
	programDir = func() (string, error) { return "", errors.New("the tests do not read the real program folder") }
	productData = func() (string, error) { return "", errors.New("the tests do not read the real data folder") }
}

// fakePlace puts the product into temporary folders: the program folder with the helper's exe in it
// (the running exe is said to be that one) and the data folder that install made. It returns both.
func fakePlace(t *testing.T) (prog, data string) {
	t.Helper()
	root := t.TempDir()
	prog, data = filepath.Join(root, "Program Files", "SplitLane"), filepath.Join(root, "ProgramData", "SplitLane")
	os.MkdirAll(prog, 0o755)
	os.MkdirAll(data, 0o755)
	oldProg, oldData, oldExe := programDir, productData, executable
	programDir = func() (string, error) { return prog, nil }
	productData = func() (string, error) { return data, nil }
	executable = func() (string, error) { return filepath.Join(prog, "splitlane-helper.exe"), nil }
	t.Cleanup(func() { programDir, productData, executable = oldProg, oldData, oldExe })
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
	if code := run(nil); code != exitBadCore || !strings.Contains(helperLog(t, data), "core refused: "+filepath.Join(prog, "sing-box.exe")) {
		t.Fatalf("exit code %d, log:\n%s", code, helperLog(t, data))
	}

	executable = func() (string, error) { return `C:\Users\u\Downloads\splitlane-helper.exe`, nil }
	if code := run(nil); code != exitUsage || !strings.Contains(helperLog(t, data), `the product runs only as `+filepath.Join(prog, "splitlane-helper.exe")) {
		t.Fatalf("a helper outside the program folder: exit code %d, log:\n%s", code, helperLog(t, data))
	}
	executable = func() (string, error) { return strings.ToUpper(filepath.Join(prog, "splitlane-helper.exe")), nil }

	os.Remove(filepath.Join(data, "setup.json"))
	if code := run(nil); code != exitNotInstalled || !strings.Contains(helperLog(t, data), "not installed") {
		t.Fatalf("no setup.json: exit code %d, log:\n%s", code, helperLog(t, data))
	}

	os.RemoveAll(data)
	if code := run(nil); code != exitNotInstalled {
		t.Fatalf("no data folder: exit code %d", code)
	}
	if _, err := os.Stat(data); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the helper made the data folder")
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
