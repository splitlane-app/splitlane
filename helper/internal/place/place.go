// Package place knows where the product lives (spec 4.2) and what its setup recorded (4.3): the
// program folder in Program Files, the data folder in ProgramData and setup.json with the SID of the
// user who ran the setup. The helper runs elevated with the user's environment, which the user may
// change (HKCU\Environment): no path here comes from an environment variable.
package place

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"splitlane/helper/internal/atomicfile"
)

// Name is the product's folder in Program Files and in ProgramData.
const Name = "SplitLane"

// SetupFile is the name of the setup data in the data folder.
const SetupFile = "setup.json"

// The lookups of the system; the tests replace them.
var (
	// programFiles reads ProgramFilesDir of HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion, 64-bit view.
	programFiles = func() (string, error) {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion`,
			registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			return "", err
		}
		defer k.Close()
		v, _, err := k.GetStringValue("ProgramFilesDir")
		return v, err
	}
	// programDataValue reads ProgramData of HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList,
	// 64-bit view, as it is written there: %SystemDrive%\ProgramData, not expanded.
	programDataValue = func() (string, error) {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList`,
			registry.QUERY_VALUE|registry.WOW64_64KEY)
		if err != nil {
			return "", err
		}
		defer k.Close()
		v, _, err := k.GetStringValue("ProgramData")
		return v, err
	}
	// windowsDir is the Windows directory as the system knows it, C:\Windows; no environment variable.
	windowsDir = windows.GetSystemWindowsDirectory
)

// HelperName is the folder of the helper and the core inside the product folder.
const HelperName = "helper"

// ProductDir is %ProgramFiles%\SplitLane (spec 4.2): the window and the uninstaller at its root, the
// helper in helper\. Written only by administrators. Program Files is the folder the registry names,
// never the environment variable: only binaries that administrators write may run elevated (spec 11).
func ProductDir() (string, error) {
	dir, err := programFiles()
	if err != nil {
		return "", fmt.Errorf("the Program Files folder: %w", err)
	}
	return inside(dir, "the Program Files folder")
}

// ProgramDir is <ProductDir>\helper: the helper, the core and the licence texts.
func ProgramDir() (string, error) {
	dir, err := ProductDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, HelperName), nil
}

// DataDir is %ProgramData%\SplitLane (spec 4.2). ProgramData is the value the registry names, never the
// known folder or the environment variable: both expand %SystemDrive% from the environment of the
// process, and the helper started at logon has the user's (HKCU\Environment), so a program of the user
// could point the elevated helper at a folder it owns. Only a leading %SystemDrive% is put in, from the
// volume of the Windows directory.
func DataDir() (string, error) {
	dir, err := programDataValue()
	if err != nil {
		return "", fmt.Errorf("the ProgramData folder: %w", err)
	}
	const systemDrive = "%SystemDrive%"
	if len(dir) >= len(systemDrive) && strings.EqualFold(dir[:len(systemDrive)], systemDrive) {
		win, err := windowsDir()
		if err != nil {
			return "", fmt.Errorf("the Windows directory: %w", err)
		}
		vol := filepath.VolumeName(win)
		if len(vol) != 2 || vol[1] != ':' || !filepath.IsAbs(win) {
			return "", fmt.Errorf("the Windows directory is not on a drive: %q", win)
		}
		dir = vol + dir[len(systemDrive):]
	}
	return inside(dir, "the ProgramData folder")
}

// inside is the product's folder in dir; a relative dir or one that would need the environment to
// expand is refused.
func inside(dir, what string) (string, error) {
	if strings.Contains(dir, "%") || !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%s is not an absolute path: %q", what, dir)
	}
	return filepath.Join(dir, Name), nil
}

// Setup is setup.json (spec 4.3): the user who ran the setup. The pipe gives this user read and
// write (5.1), the folder export\ gives him read (4.2).
type Setup struct {
	UserSID string `json:"userSid"`
}

// ReadSetup reads setup.json of dataDir. A missing file gives an error that wraps os.ErrNotExist: the
// product is not installed. A file that does not parse or holds no valid SID is an error too.
func ReadSetup(dataDir string) (Setup, error) {
	b, err := os.ReadFile(filepath.Join(dataDir, SetupFile))
	if err != nil {
		return Setup{}, err
	}
	var s Setup
	if err := json.Unmarshal(b, &s); err != nil {
		return Setup{}, fmt.Errorf("%s: %w", SetupFile, err)
	}
	if err := checkSID(s.UserSID); err != nil {
		return Setup{}, fmt.Errorf("%s: %w", SetupFile, err)
	}
	return s, nil
}

// WriteSetup writes setup.json into dataDir atomically, UTF-8 without a BOM. The folder must be there:
// install makes it first, with its access list (spec 4.2); a folder made here would inherit another.
func WriteSetup(dataDir string, s Setup) error {
	if err := checkSID(s.UserSID); err != nil {
		return err
	}
	if fi, err := os.Stat(dataDir); err != nil || !fi.IsDir() {
		return fmt.Errorf("the data folder %s is not there", dataDir)
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return atomicfile.Write(filepath.Join(dataDir, SetupFile), append(b, '\n'))
}

// checkSID takes a SID only in the plain form S-1-... that windows.StringToSid reads and gives back
// unchanged: the SID goes into the access list of the pipe (5.1), where an alias such as WD (Everyone)
// or extra text would give access to others.
func checkSID(sid string) error {
	s, err := windows.StringToSid(sid)
	if err != nil {
		return fmt.Errorf("user SID %q: %w", sid, err)
	}
	if s.String() != sid {
		return fmt.Errorf("user SID %q is not in the form S-1-...", sid)
	}
	return nil
}

// asciiFold lower-cases the ASCII letters only. NTFS compares names with its own case table, and Go's
// folding differs from it (U+0130 becomes "i", U+017F and U+212A fold to "s" and "k"): a folder named
// with such a letter is another folder, yet would pass a Go comparison. Every other rune stays exact,
// so a doubtful name never matches.
func asciiFold(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// PathEqual tells that a and b name the same file or folder: the paths are cleaned and compared with
// ASCII-only case folding; a non-ASCII letter must match exactly (a mismatch fails closed).
func PathEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return asciiFold(filepath.Clean(a)) == asciiFold(filepath.Clean(b))
}

// PathInside tells that path lies strictly below the folder dir (dir itself is not inside; SplitLaneX is
// not inside SplitLane). The same ASCII-only folding as PathEqual: non-ASCII letters must match exactly.
func PathInside(path, dir string) bool {
	if path == "" || dir == "" {
		return false
	}
	p, d := asciiFold(filepath.Clean(path)), asciiFold(filepath.Clean(dir))
	if !strings.HasSuffix(d, `\`) {
		d += `\`
	}
	return len(p) > len(d) && strings.HasPrefix(p, d)
}
