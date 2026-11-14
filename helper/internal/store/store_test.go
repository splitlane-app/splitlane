package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

// errSharing is the text Windows gives for a file another process holds open without sharing.
var errSharing = errors.New("The process cannot access the file because it is being used by another process.")

// failRead makes readFile fail with cause for the file with this name, as Windows does for a file held
// open by another process; other files are read. The returned function, and the end of the test,
// undo it.
func failRead(t *testing.T, name string, cause error) func() {
	old := readFile
	readFile = func(path string) ([]byte, error) {
		if filepath.Base(path) == name {
			return nil, &os.PathError{Op: "open", Path: path, Err: cause}
		}
		return old(path)
	}
	undo := func() { readFile = old }
	t.Cleanup(undo)
	return undo
}

// An intact state.json that cannot be read gives the defaults and ErrIO; the file stays as it was and
// nothing is set aside, so the caller can keep it from being written over.
func TestStateIOErrorKeepsTheFile(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	s := Defaults()
	s.WasOn, s.SelectedExitNode = true, "100.64.0.1"
	if err := d.SaveState(s); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(d.Path, "state.json"))
	undo := failRead(t, "state.json", errSharing)
	got, err := d.LoadState()
	if !errors.Is(err, ErrIO) || !errors.Is(err, errSharing) || got.WasOn || got.SelectedExitNode != "" || got.DefaultLane != protocol.LaneVPN {
		t.Fatalf("want the defaults and ErrIO with the cause, got %+v %v", got, err)
	}
	undo()
	if after, _ := os.ReadFile(filepath.Join(d.Path, "state.json")); string(after) != string(before) {
		t.Fatalf("state.json changed:\n%s", after)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "state.json.bad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nothing may be set aside: %v", err)
	}
	if got, err := d.LoadState(); err != nil || !got.WasOn || got.SelectedExitNode != "100.64.0.1" {
		t.Fatalf("the file must read again as it was: %+v %v", got, err)
	}
}

// Unreadable content of state.json is set aside the way a journal is: state.json.bad, then the first
// free state.json.bad.<n>. The error gives the cause and the new name and is not ErrIO; defaults follow.
func TestCorruptStateIsSetAside(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	for _, content := range []string{"{first", "{second"} {
		os.WriteFile(filepath.Join(d.Path, "state.json"), []byte(content), 0o644)
		s, err := d.LoadState()
		if err == nil || errors.Is(err, ErrIO) || !strings.Contains(err.Error(), "invalid character") ||
			!strings.Contains(err.Error(), "was set aside as state.json.bad") {
			t.Fatalf("%s: want a set-aside error with the cause, got %v", content, err)
		}
		if s.DefaultLane != protocol.LaneVPN || len(s.Apps) != 0 || s.ProbeURL != DefaultProbeURL {
			t.Fatalf("%s: want the defaults, got %+v", content, s)
		}
	}
	for file, want := range map[string]string{"state.json.bad": "{first", "state.json.bad.1": "{second"} {
		got, err := os.ReadFile(filepath.Join(d.Path, file))
		if err != nil || string(got) != want {
			t.Errorf("%s: %q %v, want %q", file, got, err, want)
		}
	}
	if _, err := os.Stat(filepath.Join(d.Path, "state.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("state.json must be gone after it was set aside: %v", err)
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
	for n := 1; n < maxBadFiles; n++ {
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

// An intact restore.json that cannot be read (a sharing violation, access denied) is not corrupt: the
// error is ErrIO with the cause, the journal stays where it is and nothing is set aside, so the caller
// cannot take it for "no journal". Unreadable content is never ErrIO.
func TestJournalIOErrorIsNotCorrupt(t *testing.T) {
	d := Dir{Path: t.TempDir()}
	if err := d.SaveJournal(Journal{ExitNode: "100.64.0.9", AllowLAN: true}); err != nil {
		t.Fatal(err)
	}
	undo := failRead(t, "restore.json", errSharing)
	j, err := d.LoadJournal()
	if j != nil || !errors.Is(err, ErrIO) || !errors.Is(err, errSharing) || strings.Contains(err.Error(), "set aside") {
		t.Fatalf("want ErrIO with the cause, nothing set aside, got %v %v", j, err)
	}
	if _, err := os.Stat(filepath.Join(d.Path, "restore.json.bad")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("nothing may be set aside: %v", err)
	}
	undo()
	if j, err := d.LoadJournal(); err != nil || j == nil || j.ExitNode != "100.64.0.9" || !j.AllowLAN {
		t.Fatalf("the journal must read again as it was: %+v %v", j, err)
	}
	os.WriteFile(filepath.Join(d.Path, "restore.json"), []byte("{broken"), 0o644)
	if _, err := d.LoadJournal(); err == nil || errors.Is(err, ErrIO) {
		t.Fatalf("unreadable content is not ErrIO: %v", err)
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

// diagnostics.export (spec 5.1, 4.2): the archives go into export\ of the data folder; the logs it takes are
// the helper's and the core's in logs\, each with the one of the run before. Nothing is created.
func TestExportPaths(t *testing.T) {
	d := Dir{Path: `C:\data`}
	if got := d.ExportDir(); got != `C:\data\export` {
		t.Fatalf("export folder %q", got)
	}
	want := []string{`C:\data\logs\helper.log`, `C:\data\logs\helper.log.prev`, `C:\data\logs\core.log`, `C:\data\logs\core.log.prev`}
	if got := d.Logs(); !slices.Equal(got, want) {
		t.Fatalf("logs %q", got)
	}
}
