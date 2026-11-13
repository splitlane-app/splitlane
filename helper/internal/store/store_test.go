package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"splitlane/helper/internal/protocol"
)

func TestStateRoundTripAndDefaults(t *testing.T) {
	d := Dir{Path: filepath.Join(t.TempDir(), "data")}
	s, err := d.LoadState()
	if err != nil {
		t.Fatal(err)
	}
	if s.DefaultLane != protocol.LaneVPN || s.ProbeURL != DefaultProbeURL || s.Apps == nil || s.WasOn {
		t.Fatalf("defaults %+v", s)
	}
	s.LoggedIn, s.WasOn, s.SelectedExitNode = true, true, "100.64.0.1"
	s.Core = &CoreIdentity{PID: 42, Created: 133000000000000000, Path: `C:\Program Files\SplitLane\sing-box.exe`}
	s.Apps = append(s.Apps, protocol.AppRule{ID: "a", Name: "Кириллица", Exe: `C:\x.exe`, Matcher: protocol.Matcher{Kind: "file", Value: `C:\x.exe`}, Lane: "direct"})
	if err := d.SaveState(s); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(d.Path, "state.json"))
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("state.json starts with a BOM")
	}
	got, err := d.LoadState()
	if err != nil || !got.WasOn || got.Core.PID != 42 || got.Apps[0].Name != "Кириллица" {
		t.Fatalf("round trip: %v %+v", err, got)
	}
}

func TestCorruptStateGivesDefaultsAndError(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	os.WriteFile(filepath.Join(d.Path, "state.json"), []byte("{broken"), 0o644)
	s, err := d.LoadState()
	if err == nil || s.DefaultLane != protocol.LaneVPN {
		t.Fatalf("want defaults and an error, got %+v %v", s, err)
	}
}

func TestJournalLifecycle(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	if j, err := d.LoadJournal(); j != nil || err != nil {
		t.Fatalf("no journal: %v %v", j, err)
	}
	if err := d.SaveJournal(Journal{ExitNode: "100.64.0.9", AllowLAN: true}); err != nil {
		t.Fatal(err)
	}
	j, err := d.LoadJournal()
	if err != nil || j.ExitNode != "100.64.0.9" || !j.AllowLAN {
		t.Fatalf("journal %+v %v", j, err)
	}
	if err := d.DeleteJournal(); err != nil {
		t.Fatal(err)
	}
	if err := d.DeleteJournal(); err != nil {
		t.Fatalf("deleting twice: %v", err)
	}
}

func TestUnreadableJournalIsSetAside(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	os.WriteFile(filepath.Join(d.Path, "restore.json"), []byte("{}"), 0o644)
	if j, err := d.LoadJournal(); j != nil || err == nil {
		t.Fatalf("want an error, got %v %v", j, err)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "restore.json.bad")); err != nil {
		t.Fatal("restore.json.bad missing")
	}
}

func TestRulesAndConfigFiles(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	if err := d.WriteRules([]byte("D\n"), []byte("V\n")); err != nil {
		t.Fatal(err)
	}
	path, err := d.WriteConfig([]byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	for file, want := range map[string]string{
		filepath.Join(d.Path, "rules", "lane-direct.json"): "D\n",
		filepath.Join(d.Path, "rules", "lane-vpn.json"):    "V\n",
		path: "{}\n",
	} {
		got, err := os.ReadFile(file)
		if err != nil || string(got) != want {
			t.Errorf("%s: %q %v", file, got, err)
		}
	}
	for _, sub := range []string{"logs", "tailscale"} {
		if _, err := os.Stat(filepath.Join(d.Path, sub)); err != nil {
			t.Errorf("folder %s missing", sub)
		}
	}
}
