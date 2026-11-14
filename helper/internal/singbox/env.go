package singbox

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// systemEnviron is the environment of the core, its check and its CLI (review F13-2). They run
// elevated, so they get the system's own variables (CreateEnvironmentBlock without a token) and
// never the user's, whose TS_* knobs, proxies or GODEBUG would steer them. The block takes
// SystemRoot and SystemDrive from the calling process, and the helper, a task started as the user,
// takes them from HKCU\Environment too; they are set to the system's own first
// (GetSystemWindowsDirectory), otherwise Path, TEMP and ComSpec of the block would point where the
// user said.
func systemEnviron() ([]string, error) {
	root, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return nil, err
	}
	for _, v := range [][2]string{{"SystemRoot", root}, {"SystemDrive", filepath.VolumeName(root)}} {
		if !strings.EqualFold(os.Getenv(v[0]), v[1]) {
			if err := os.Setenv(v[0], v[1]); err != nil {
				return nil, err
			}
		}
	}
	return windows.Token(0).Environ(false)
}
