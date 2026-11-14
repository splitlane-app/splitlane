package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	if _, err := parse([]string{"--data-dir", `C:\d`, "--core", `C:\c\sing-box.exe`}); err == nil {
		t.Fatal("this build needs --dev")
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
	if code := run(nil); code != exitUsage {
		t.Fatalf("no arguments: %d", code)
	}
}
