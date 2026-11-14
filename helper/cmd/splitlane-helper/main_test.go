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
