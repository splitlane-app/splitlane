package place

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fake makes the lookup *f answer value and err for the test.
func fake(t *testing.T, f *func() (string, error), value string, err error) {
	t.Helper()
	old := *f
	*f = func() (string, error) { return value, err }
	t.Cleanup(func() { *f = old })
}

// The program folder is ProgramFilesDir of HKLM as it is written there (spec 4.2, 11), never the
// environment of the elevated process: a value that would need the environment to expand, a relative
// one or a failed read gives an error.
func TestProgramDirFromRegistryValue(t *testing.T) {
	t.Setenv("ProgramFiles", `C:\Users\u\AppData\Local\Fake`)
	fake(t, &programFiles, `D:\Apps`, nil)
	if dir, err := ProgramDir(); err != nil || dir != `D:\Apps\SplitLane` {
		t.Fatalf("%q %v", dir, err)
	}
	for _, bad := range []string{`%ProgramFiles%`, `%SystemDrive%\Program Files`, `Program Files`, `\Program Files`, ``} {
		fake(t, &programFiles, bad, nil)
		if dir, err := ProgramDir(); err == nil {
			t.Errorf("%q gave %q", bad, dir)
		}
	}
	fake(t, &programFiles, "", errors.New("The system cannot find the file specified."))
	if _, err := ProgramDir(); err == nil || !strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("a failed read: %v", err)
	}
}

// The data folder is the known folder ProgramData (spec 4.2), with the same checks.
func TestDataDirFromKnownFolder(t *testing.T) {
	t.Setenv("ProgramData", `C:\Users\u\AppData\Local\Fake`)
	fake(t, &programData, `C:\ProgramData`, nil)
	if dir, err := DataDir(); err != nil || dir != `C:\ProgramData\SplitLane` {
		t.Fatalf("%q %v", dir, err)
	}
	for _, bad := range []string{`%ProgramData%`, `ProgramData`, ``} {
		fake(t, &programData, bad, nil)
		if dir, err := DataDir(); err == nil {
			t.Errorf("%q gave %q", bad, dir)
		}
	}
	fake(t, &programData, "", errors.New("Element not found."))
	if _, err := DataDir(); err == nil {
		t.Fatal("a failed lookup must be an error")
	}
}

const testSID = "S-1-5-21-1000-2000-3000-1001"

// setup.json is UTF-8 without a BOM and reads back; a missing file is os.ErrNotExist (not installed).
// WriteSetup never makes the data folder: install makes it with its access list (spec 4.2).
func TestSetupRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadSetup(dir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no setup.json: %v", err)
	}
	if err := WriteSetup(dir, Setup{UserSID: testSID}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "setup.json"))
	if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) || !bytes.Contains(raw, []byte(`"userSid": "`+testSID+`"`)) {
		t.Fatalf("setup.json:\n%s", raw)
	}
	s, err := ReadSetup(dir)
	if err != nil || s.UserSID != testSID {
		t.Fatalf("%+v %v", s, err)
	}
	missing := filepath.Join(dir, "missing")
	if err := WriteSetup(missing, Setup{UserSID: testSID}); err == nil {
		t.Fatal("the data folder must already be there")
	}
	if _, err := os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("WriteSetup made the data folder")
	}
}

// The SID goes into the access list of the pipe (5.1): only the plain form S-1-... that
// windows.StringToSid reads and gives back unchanged is taken. An alias such as WD (Everyone) or text
// that would add an entry is refused, on write and on read.
func TestSetupRejectsBadSID(t *testing.T) {
	dir := t.TempDir()
	for _, bad := range []string{"", "not-a-sid", "WD", "BU", "s-1-5-21-1000-2000-3000-1001", testSID + ")(A;;GA;;;WD", " " + testSID} {
		if err := WriteSetup(dir, Setup{UserSID: bad}); err == nil {
			t.Errorf("WriteSetup took %q", bad)
		}
		if _, err := os.Stat(filepath.Join(dir, "setup.json")); err == nil {
			t.Fatalf("setup.json written for %q", bad)
		}
	}
	for _, text := range []string{`{"userSid":"WD"}`, `{"userSid":""}`, `{}`, `{broken`} {
		os.WriteFile(filepath.Join(dir, "setup.json"), []byte(text), 0o644)
		if s, err := ReadSetup(dir); err == nil || errors.Is(err, os.ErrNotExist) {
			t.Errorf("ReadSetup took %s: %+v %v", text, s, err)
		}
	}
}

// TestLivePlacesReadOnly reads the real registry value and known folder and changes nothing. It runs
// with SPLITLANE_LIVE_READONLY=1.
func TestLivePlacesReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	prog, err := ProgramDir()
	if err != nil || !strings.EqualFold(filepath.Base(prog), "SplitLane") {
		t.Fatalf("program folder %q %v", prog, err)
	}
	data, err := DataDir()
	if err != nil || !strings.EqualFold(filepath.Base(data), "SplitLane") {
		t.Fatalf("data folder %q %v", data, err)
	}
	t.Logf("LIVE program folder %s, data folder %s", prog, data)
}
