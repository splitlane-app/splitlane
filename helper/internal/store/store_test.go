package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	if _, err := os.Stat(filepath.Join(d.Path, "restore.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore.json must be gone after it was set aside: %v", err)
	}
}

func TestUnreadableJournalErrorNamesCauseAndFile(t *testing.T) {
	for name, tc := range map[string]struct{ content, cause string }{
		"broken json":  {"{broken", "invalid character"},
		"empty target": {"{}", "empty exitNode"},
	} {
		t.Run(name, func(t *testing.T) {
			d := Dir{Path: t.TempDir()}
			os.WriteFile(filepath.Join(d.Path, "restore.json"), []byte(tc.content), 0o644)
			_, err := d.LoadJournal()
			if err == nil || !strings.Contains(err.Error(), tc.cause) || !strings.Contains(err.Error(), "restore.json.bad") {
				t.Fatalf("want the cause %q and the new file name, got %v", tc.cause, err)
			}
		})
	}
}

func TestSecondUnreadableJournalKeepsEarlierEvidence(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	for _, content := range []string{"{first", "{second"} {
		os.WriteFile(filepath.Join(d.Path, "restore.json"), []byte(content), 0o644)
		if _, err := d.LoadJournal(); err == nil {
			t.Fatalf("%s: want an error", content)
		}
	}
	for file, want := range map[string]string{"restore.json.bad": "{first", "restore.json.bad.1": "{second"} {
		got, err := os.ReadFile(filepath.Join(d.Path, file))
		if err != nil || string(got) != want {
			t.Errorf("%s: %q %v, want %q", file, got, err, want)
		}
	}
}

func TestJournalThatCannotBeSetAsideSaysSo(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	os.WriteFile(filepath.Join(d.Path, "restore.json"), []byte("{broken"), 0o644)
	locked := errors.New("locked by antivirus")
	old := renameFile
	renameFile = func(string, string) error { return locked }
	defer func() { renameFile = old }()
	j, err := d.LoadJournal()
	if j != nil || !errors.Is(err, locked) {
		t.Fatalf("want the rename error, got %v %v", j, err)
	}
	if !strings.Contains(err.Error(), "could not be set aside") || strings.Contains(err.Error(), "was set aside") ||
		!strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("message must say it was not set aside and give the cause: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "restore.json")); err != nil {
		t.Fatalf("the journal must stay in place: %v", err)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "restore.json.bad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no .bad file expected: %v", err)
	}
}

func TestJournalSetAsideGivesUpWhenAllNamesAreTaken(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	os.WriteFile(filepath.Join(d.Path, "restore.json"), []byte("{broken"), 0o644)
	os.WriteFile(filepath.Join(d.Path, "restore.json.bad"), []byte("0"), 0o644)
	for n := 1; n < maxBadJournals; n++ {
		os.WriteFile(filepath.Join(d.Path, "restore.json.bad."+strconv.Itoa(n)), []byte("x"), 0o644)
	}
	if _, err := d.LoadJournal(); err == nil || !strings.Contains(err.Error(), "could not be set aside") {
		t.Fatalf("want a could-not-be-set-aside error, got %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(d.Path, "restore.json.bad")); string(b) != "0" {
		t.Fatalf("restore.json.bad was overwritten: %q", b)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "restore.json")); err != nil {
		t.Fatalf("the journal must stay in place: %v", err)
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
