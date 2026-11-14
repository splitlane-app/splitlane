// Package session tells the helper that the Windows session ends (logoff, restart, shutdown).
// A process that loaded user32.dll gets no console event for it; it gets WM_QUERYENDSESSION and
// WM_ENDSESSION in a top-level window (spec 5.4). A message-only window does not get them.
package session

import (
	"errors"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmQueryEndSession = 0x0011
	wmEndSession      = 0x0016
)

var (
	user32               = windows.NewLazySystemDLL("user32.dll")
	procRegisterClassExW = user32.NewProc("RegisterClassExW")
	procCreateWindowExW  = user32.NewProc("CreateWindowExW")
	procDefWindowProcW   = user32.NewProc("DefWindowProcW")
	procGetMessageW      = user32.NewProc("GetMessageW")
	procTranslateMessage = user32.NewProc("TranslateMessage")
	procDispatchMessageW = user32.NewProc("DispatchMessageW")
	procPostMessageW     = user32.NewProc("PostMessageW")
	procPostQuitMessage  = user32.NewProc("PostQuitMessage")
	procSendMessageW     = user32.NewProc("SendMessageW")
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type msg struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       [2]int32
	LPrivate uint32
}

// Watcher owns the hidden window.
type Watcher struct {
	hwnd uintptr
}

var (
	handlers sync.Map // hwnd -> *handler
	wndProc  = syscall.NewCallback(proc)
	class    sync.Once
	classErr error
)

type handler struct {
	onEnd func()
	log   func(string, ...any)
}

func proc(hwnd, message, wParam, lParam uintptr) uintptr {
	if v, ok := handlers.Load(hwnd); ok {
		h := v.(*handler)
		switch message {
		case wmQueryEndSession:
			h.log("session: WM_QUERYENDSESSION lParam=%#x", lParam)
			return 1 // the session may end
		case wmEndSession:
			h.log("session: WM_ENDSESSION ending=%v lParam=%#x", wParam != 0, lParam)
			if wParam != 0 {
				h.onEnd()
			}
			return 0
		case wmDestroy:
			handlers.Delete(hwnd)
			procPostQuitMessage.Call(0)
			return 0
		}
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, message, wParam, lParam)
	return r
}

var className = windows.StringToUTF16Ptr("SplitLaneSessionWatch")

// Watch creates the hidden window on a thread of its own and calls onEnd from WM_ENDSESSION,
// before Windows ends the process. onEnd must finish within a few seconds.
func Watch(onEnd func(), log func(string, ...any)) (*Watcher, error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	ready := make(chan error, 1)
	w := &Watcher{}
	go func() {
		runtime.LockOSThread()
		var instance windows.Handle
		if err := windows.GetModuleHandleEx(0, nil, &instance); err != nil {
			ready <- err
			return
		}
		class.Do(func() {
			wc := wndClassEx{WndProc: wndProc, Instance: instance, ClassName: className}
			wc.Size = uint32(unsafe.Sizeof(wc))
			if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
				classErr = err
			}
		})
		if classErr != nil {
			ready <- classErr
			return
		}
		// WS_OVERLAPPED, never shown: a top-level window without a taskbar entry
		hwnd, _, err := procCreateWindowExW.Call(0, uintptr(unsafe.Pointer(className)),
			uintptr(unsafe.Pointer(windows.StringToUTF16Ptr("SplitLane"))), 0, 0, 0, 0, 0, 0, 0, uintptr(instance), 0)
		if hwnd == 0 {
			ready <- err
			return
		}
		handlers.Store(hwnd, &handler{onEnd: onEnd, log: log})
		w.hwnd = hwnd
		ready <- nil
		var m msg
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				return
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()
	if err := <-ready; err != nil {
		return nil, errors.Join(errors.New("session watch window"), err)
	}
	return w, nil
}

// Stop closes the window and ends its thread.
func (w *Watcher) Stop() {
	if w != nil && w.hwnd != 0 {
		procPostMessageW.Call(w.hwnd, wmClose, 0, 0)
	}
}

// send delivers a message to the window and waits for its answer (tests).
func (w *Watcher) send(message, wParam, lParam uintptr) uintptr {
	r, _, _ := procSendMessageW.Call(w.hwnd, message, wParam, lParam)
	return r
}
