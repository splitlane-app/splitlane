package place

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveDataDirIgnoresSystemDrive: the scheduled task starts the helper as the user, with
// HKCU\Environment loaded, so a program of that user can set SystemDrive to a folder it owns (here with
// a ProgramData in it). The data folder of the elevated helper must not follow it. It is the first test
// of the file: a lookup that went through the environment would keep its first answer for the process.
// Reads the registry and the Windows directory only; runs with SPLITLANE_LIVE_READONLY=1.
func TestLiveDataDirIgnoresSystemDrive(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	planted := filepath.Join(t.TempDir(), "sd")
	if err := os.MkdirAll(filepath.Join(planted, "ProgramData"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SystemDrive", planted)
	got, err := DataDir()
	if err != nil || strings.HasPrefix(strings.ToLower(got), strings.ToLower(planted)) {
		t.Fatalf("with SystemDrive planted: data folder %q %v", got, err)
	}
	os.Unsetenv("SystemDrive") // the cleanup of t.Setenv puts the real one back
	if want, err := DataDir(); err != nil || got != want {
		t.Fatalf("with SystemDrive planted %q, without it %q %v", got, want, err)
	}
	t.Logf("LIVE data folder %s with SystemDrive planted and without it", got)
}

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
	if dir, err := ProgramDir(); err != nil || dir != `D:\Apps\SplitLane\helper` {
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

// The product folder is <Program Files>\SplitLane, Program Files from the registry value, never from the
// environment (the window and the uninstaller live there).
func TestProductDirIsUnderProgramFiles(t *testing.T) {
	t.Setenv("ProgramFiles", `C:\Users\u\AppData\Local\Fake`)
	fake(t, &programFiles, `D:\Apps`, nil)
	if dir, err := ProductDir(); err != nil || dir != `D:\Apps\SplitLane` {
		t.Fatalf("%q %v", dir, err)
	}
	for _, bad := range []string{`%ProgramFiles%`, `Program Files`, ``} {
		fake(t, &programFiles, bad, nil)
		if dir, err := ProductDir(); err == nil {
			t.Errorf("%q gave %q", bad, dir)
		}
	}
	fake(t, &programFiles, "", errors.New("The system cannot find the file specified."))
	if _, err := ProductDir(); err == nil || !strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("a failed read: %v", err)
	}
}

// The program folder, where the helper and the core live, is the helper folder of the product folder.
func TestProgramDirIsTheHelperFolder(t *testing.T) {
	fake(t, &programFiles, `D:\Apps`, nil)
	if dir, err := ProgramDir(); err != nil || dir != `D:\Apps\SplitLane\helper` {
		t.Fatalf("%q %v", dir, err)
	}
	fake(t, &programFiles, `%ProgramFiles%`, nil)
	if dir, err := ProgramDir(); err == nil {
		t.Fatalf("an unexpanded value gave %q", dir)
	}
	fake(t, &programFiles, "", errors.New("The system cannot find the file specified."))
	if _, err := ProgramDir(); err == nil || !strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("a failed read: %v", err)
	}
}

// The data folder is ProgramData of HKLM ProfileList as it is written there (spec 4.2, 11), with only a
// leading %SystemDrive% put in from the volume of the Windows directory, never from the environment of
// the elevated process (the user can set SystemDrive and ProgramData in HKCU\Environment). Any other
// variable, a relative path or a failed read gives an error.
func TestDataDirFromRegistryValue(t *testing.T) {
	t.Setenv("SystemDrive", t.TempDir())
	t.Setenv("ProgramData", `C:\Users\u\AppData\Local\Fake`)
	fake(t, &windowsDir, `D:\Windows`, nil)
	for raw, want := range map[string]string{
		`%SystemDrive%\ProgramData`: `D:\ProgramData\SplitLane`,
		`%systemdrive%\ProgramData`: `D:\ProgramData\SplitLane`,
		`E:\ProgramData`:            `E:\ProgramData\SplitLane`,
	} {
		fake(t, &programDataValue, raw, nil)
		if dir, err := DataDir(); err != nil || dir != want {
			t.Errorf("%q: %q %v", raw, dir, err)
		}
	}
	for _, bad := range []string{`%ProgramData%`, `%SystemRoot%\..\ProgramData`, `%SystemDrive%\%USERNAME%`,
		`%SystemDrive%`, `%SystemDrive%ProgramData`, `X%SystemDrive%\ProgramData`, `ProgramData`, `\ProgramData`, ``} {
		fake(t, &programDataValue, bad, nil)
		if dir, err := DataDir(); err == nil {
			t.Errorf("%q gave %q", bad, dir)
		}
	}
	fake(t, &programDataValue, `%SystemDrive%\ProgramData`, nil)
	for _, bad := range []string{``, `Windows`, `\Windows`, `\\server\share\Windows`, `%SystemDrive%\Windows`} {
		fake(t, &windowsDir, bad, nil)
		if dir, err := DataDir(); err == nil {
			t.Errorf("Windows directory %q gave %q", bad, dir)
		}
	}
	fake(t, &windowsDir, "", errors.New("The parameter is incorrect."))
	if _, err := DataDir(); err == nil || !strings.Contains(err.Error(), "parameter is incorrect") {
		t.Fatalf("a failed Windows directory: %v", err)
	}
	fake(t, &windowsDir, `D:\Windows`, nil)
	fake(t, &programDataValue, "", errors.New("The system cannot find the file specified."))
	if _, err := DataDir(); err == nil || !strings.Contains(err.Error(), "cannot find") {
		t.Fatalf("a failed read: %v", err)
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

// TestLivePlacesReadOnly reads the real registry values and Windows directory and changes nothing. It runs
// with SPLITLANE_LIVE_READONLY=1.
func TestLivePlacesReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	prod, err := ProductDir()
	if err != nil || !strings.EqualFold(filepath.Base(prod), "SplitLane") {
		t.Fatalf("product folder %q %v", prod, err)
	}
	prog, err := ProgramDir()
	if err != nil || !strings.EqualFold(filepath.Base(prog), "helper") || filepath.Dir(prog) != prod {
		t.Fatalf("program folder %q %v", prog, err)
	}
	data, err := DataDir()
	if err != nil || !strings.EqualFold(filepath.Base(data), "SplitLane") {
		t.Fatalf("data folder %q %v", data, err)
	}
	t.Logf("LIVE program folder %s, data folder %s", prog, data)
}
