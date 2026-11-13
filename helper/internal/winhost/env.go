package winhost

import (
	"fmt"
	"os"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/rules"
)

// RulesEnv describes this computer for package rules; protected are the full paths of the
// helper, the core and the system Tailscale processes (spec 8.2).
func RulesEnv(protected []string) rules.Env {
	win, _ := windows.GetWindowsDirectory()
	known := func(id *windows.KNOWNFOLDERID) string {
		p, err := windows.KnownFolderPath(id, 0)
		if err != nil {
			return ""
		}
		return p
	}
	return rules.Env{
		WindowsDir:      win,
		ProgramFiles:    known(windows.FOLDERID_ProgramFiles),
		ProgramFilesX86: known(windows.FOLDERID_ProgramFilesX86),
		UserDirs: []string{known(windows.FOLDERID_Desktop), known(windows.FOLDERID_Downloads),
			known(windows.FOLDERID_Documents), strings.TrimRight(os.TempDir(), `\`)},
		Protected: protected,
		Describe:  FileDescription,
	}
}

// FileDescription is the FileDescription of the version resources of path, or "".
func FileDescription(path string) string {
	size, err := windows.GetFileVersionInfoSize(path, nil)
	if err != nil || size == 0 {
		return ""
	}
	buf := make([]byte, size)
	if err := windows.GetFileVersionInfo(path, 0, size, unsafe.Pointer(&buf[0])); err != nil {
		return ""
	}
	lang := "040904b0"
	var trans *[2]uint16
	var n uint32
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\VarFileInfo\Translation`, unsafe.Pointer(&trans), &n) == nil && n >= 4 {
		lang = fmt.Sprintf("%04x%04x", trans[0], trans[1])
	}
	var text *uint16
	if windows.VerQueryValue(unsafe.Pointer(&buf[0]), `\StringFileInfo\`+lang+`\FileDescription`, unsafe.Pointer(&text), &n) != nil || n == 0 {
		return ""
	}
	return strings.TrimSpace(windows.UTF16PtrToString(text))
}
