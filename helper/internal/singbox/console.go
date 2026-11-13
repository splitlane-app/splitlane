package singbox

import (
	"errors"
	"fmt"
	"sync"

	"golang.org/x/sys/windows"
)

var (
	kernel32          = windows.NewLazySystemDLL("kernel32.dll")
	procAttachConsole = kernel32.NewProc("AttachConsole")
	procFreeConsole   = kernel32.NewProc("FreeConsole")
)

// attachParentProcess is ATTACH_PARENT_PROCESS, (DWORD)-1.
const attachParentProcess = ^uint32(0)

var consoleMu sync.Mutex

func attachConsole(pid uint32) error {
	if r, _, err := procAttachConsole.Call(uintptr(pid)); r == 0 {
		return err
	}
	return nil
}

func freeConsole() { procFreeConsole.Call() }

// CtrlBreak sends CTRL_BREAK_EVENT to the process group whose id is pid: the core is started in
// a group of its own, with a hidden console (spec 5.4). The helper is built without a console;
// for the moment of the signal it attaches to the core's console and then lets it go. A caller
// that has a console of its own (go test, for example) gets it back afterwards.
// The caller is not in the core's group, so the event does not reach it.
func CtrlBreak(pid uint32) error {
	consoleMu.Lock()
	defer consoleMu.Unlock()
	hadConsole := false
	if err := attachConsole(pid); err != nil {
		if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return fmt.Errorf("attach to the console of %d: %w", pid, err)
		}
		hadConsole = true // already attached to a console: let it go first
		freeConsole()
		if err := attachConsole(pid); err != nil {
			attachConsole(attachParentProcess)
			return fmt.Errorf("attach to the console of %d: %w", pid, err)
		}
	}
	defer func() {
		freeConsole()
		if hadConsole {
			attachConsole(attachParentProcess)
		}
	}()
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, pid)
}
