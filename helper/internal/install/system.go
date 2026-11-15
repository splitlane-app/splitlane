package install

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/atomicfile"
	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/pipe"
	"splitlane/helper/internal/place"
	"splitlane/helper/internal/protocol"
	"splitlane/helper/internal/singbox"
	"splitlane/helper/internal/store"
	"splitlane/helper/internal/systs"
)

// System is the real Sys: the token of this process, the files, the helper's pipe, the system
// Tailscale client by its full path, the core's process, the access lists and schtasks.exe of Windows.
type System struct {
	Pipe      string        // the helper's pipe
	Tailscale *systs.Client // the official client by its full path (spec 5.6)
}

var _ Sys = (*System)(nil)

// NewSys is the System of the product: the pipe \\.\pipe\SplitLane.Helper, tailscale.exe from the
// install folder of the official client.
func NewSys() *System { return &System{Pipe: pipe.DefaultName, Tailscale: systs.New()} }

// Token: the user of this process's token and whether UAC elevated it.
func (*System) Token() (string, bool, error) {
	t := windows.GetCurrentProcessToken()
	u, err := t.GetTokenUser()
	if err != nil {
		return "", false, err
	}
	return u.User.Sid.String(), t.IsElevated(), nil
}

var (
	wtsapi32         = windows.NewLazySystemDLL("wtsapi32.dll")
	procWTSQuerySess = wtsapi32.NewProc("WTSQuerySessionInformationW")
)

const (
	wtsCurrentServerHandle = 0 // WTS_CURRENT_SERVER_HANDLE
	wtsUserName            = 5 // WTS_INFO_CLASS: WTSUserName
	wtsDomainName          = 7 // WTS_INFO_CLASS: WTSDomainName
)

// wtsString is one string of the session sess of this computer (WTSQuerySessionInformationW).
func wtsString(sess uint32, class uintptr) (string, error) {
	var buf *uint16
	var n uint32
	r, _, err := procWTSQuerySess.Call(wtsCurrentServerHandle, uintptr(sess), class, uintptr(unsafe.Pointer(&buf)), uintptr(unsafe.Pointer(&n)))
	if r == 0 {
		return "", err
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buf)))
	return windows.UTF16PtrToString(buf), nil
}

// SessionUserSID: the user and domain of this process's session (WTSUserName, WTSDomainName), then their
// SID (LookupAccountName). A session without a user (session 0 of services) gives "".
func (*System) SessionUserSID() (string, error) {
	var sess uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &sess); err != nil {
		return "", fmt.Errorf("the session of this process: %w", err)
	}
	user, err := wtsString(sess, wtsUserName)
	if err != nil {
		return "", fmt.Errorf("the user of session %d: %w", sess, err)
	}
	if user == "" {
		return "", nil
	}
	domain, err := wtsString(sess, wtsDomainName)
	if err != nil {
		return "", fmt.Errorf("the domain of session %d: %w", sess, err)
	}
	account := user
	if domain != "" {
		account = domain + `\` + user
	}
	sid, _, _, err := windows.LookupSID("", account)
	if err != nil {
		return "", fmt.Errorf("the SID of the session user: %w", err)
	}
	return sid.String(), nil
}

// productImages are the images of the processes that hold the files of the program folder.
var productImages = []string{"splitlane-helper.exe", "sing-box.exe"}

// ProductProcesses: the snapshot of the processes (as cmd/slguard does), those named as one of
// productImages, other than this one, whose image (QueryFullProcessImageName) is under productDir. A
// process that has such a name and cannot be opened is returned without an image: it may be the product's,
// and the files it holds cannot be replaced.
func (*System) ProductProcesses(productDir string) ([]Proc, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("the list of processes: %w", err)
	}
	defer windows.CloseHandle(snap)
	self := windows.GetCurrentProcessId()
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []Proc
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		name := windows.UTF16ToString(e.ExeFile[:])
		if e.ProcessID == self || !slices.ContainsFunc(productImages, func(i string) bool { return strings.EqualFold(i, name) }) {
			continue
		}
		image, ok := imagePath(e.ProcessID)
		switch {
		case !ok:
			out = append(out, Proc{PID: e.ProcessID})
		case insideDir(image, productDir):
			out = append(out, Proc{PID: e.ProcessID, Image: image})
		}
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return nil, fmt.Errorf("the list of processes: %w", err)
	}
	return out, nil
}

// imagePath is the full path of the image of a process; ok is false when it cannot be found out.
func imagePath(pid uint32) (string, bool) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", false
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return "", false
	}
	return windows.UTF16ToString(buf[:n]), true
}

// insideDir tells that path is below dir; names compare with ASCII-only case folding (place.PathInside).
func insideDir(path, dir string) bool {
	return place.PathInside(path, dir)
}

// AccountName is DOMAIN\name of the SID, the user of the task.
func (*System) AccountName(sid string) (string, error) {
	s, err := windows.StringToSid(sid)
	if err != nil {
		return "", err
	}
	account, domain, _, err := s.LookupAccount("")
	if err != nil {
		return "", err
	}
	return domain + `\` + account, nil
}

func (*System) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (*System) StatFile(path string) error {
	_, err := os.Stat(path)
	return err
}

// dialTimeout bounds the connection to a pipe that is there and busy.
const dialTimeout = 5 * time.Second

// ShutdownHelper: see Sys. The helper's process is the server of the pipe (GetNamedPipeServerProcessId);
// while it is waited for, the answer to helper.shutdown and the state events are read: the last state
// says why a helper that does not end stays (restore_pending, 5.4).
func (s *System) ShutdownHelper(wait time.Duration) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	conn, err := winio.DialPipeContext(ctx, s.Pipe)
	cancel()
	if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return true, fmt.Errorf("the pipe %s is there but does not answer: %w", s.Pipe, err)
	}
	defer conn.Close()
	f, ok := conn.(interface{ Fd() uintptr })
	if !ok {
		return true, errors.New("the pipe gives no handle")
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(windows.Handle(f.Fd()), &pid); err != nil {
		return true, fmt.Errorf("the process of the pipe: %w", err)
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return true, fmt.Errorf("the helper's process %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)
	w := &watch{}
	go w.read(conn)
	msgs := append(request(1, protocol.MethodHello, protocol.HelloParams{Protocol: protocol.Version}), request(2, protocol.MethodShutdown, nil)...)
	if _, err := conn.Write(msgs); err != nil {
		if ev, _ := windows.WaitForSingleObject(h, 0); ev == windows.WAIT_OBJECT_0 {
			return true, nil
		}
		return true, fmt.Errorf("helper.shutdown could not be sent to the helper (process %d): %w", pid, err)
	}
	deadline := time.Now().Add(wait)
	for {
		ev, err := windows.WaitForSingleObject(h, 500)
		switch {
		case err != nil:
			return true, fmt.Errorf("waiting for the helper (process %d): %w", pid, err)
		case ev == windows.WAIT_OBJECT_0:
			return true, nil
		}
		if fail := w.failed(); fail != nil {
			return true, fmt.Errorf("the helper (process %d) refused helper.shutdown: %v", pid, fail)
		}
		if !time.Now().Before(deadline) {
			return true, fmt.Errorf("the helper (process %d) did not end within %s; its last state: %s", pid, wait, w.state())
		}
	}
}

// request is one request line of spec 5.1.
func request(id int, method string, params any) []byte {
	m := map[string]any{"id": id, "method": method}
	if params != nil {
		m["params"] = params
	}
	b, _ := json.Marshal(m)
	return append(b, '\n')
}

// watch keeps what the helper sends after helper.shutdown: an error answer to it (request 2) and the
// phase and reason of the last state event.
type watch struct {
	mu   sync.Mutex
	last string
	fail error
}

func (w *watch) read(r io.Reader) {
	sc := protocol.NewScanner(r)
	for sc.Scan() {
		var m struct {
			ID    int             `json:"id"`
			Error *protocol.Error `json:"error"`
			Event string          `json:"event"`
			Data  json.RawMessage `json:"data"`
		}
		if json.Unmarshal(sc.Bytes(), &m) != nil {
			continue
		}
		w.mu.Lock()
		switch {
		case m.Event == "state":
			var st protocol.State
			if json.Unmarshal(m.Data, &st) == nil {
				w.last = "phase=" + st.Phase
				if st.Reason != nil {
					w.last += " reason=" + st.Reason.Code
					if text := st.Reason.Params[protocol.ParamError]; text != "" {
						w.last += " (" + text + ")"
					}
				}
			}
		case m.ID == 2 && m.Error != nil:
			w.fail = m.Error
		}
		w.mu.Unlock()
	}
}

func (w *watch) failed() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.fail
}

func (w *watch) state() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.last == "" {
		return "no state event came"
	}
	return w.last
}

func (s *System) SystemStatus(ctx context.Context) (engine.SysStatus, error) {
	return s.Tailscale.Status(ctx)
}

func (s *System) SetExitNode(ctx context.Context, ip string, allowLAN bool) error {
	return s.Tailscale.SetExitNode(ctx, ip, allowLAN)
}

func (s *System) RestoreCommand(ip string, allowLAN bool) string {
	return s.Tailscale.RestoreCommand(ip, allowLAN)
}

// FindCore is singbox.Driver.Find with the driver's core exe: the image of the process must be that file.
func (*System) FindCore(exe string, id store.CoreIdentity) (engine.CoreProc, bool) {
	return (&singbox.Driver{Exe: exe}).Find(id)
}

func (*System) After(d time.Duration) <-chan time.Time { return time.After(d) }

// adminOwners may own a folder that is there already: what install or the helper made (the objects of
// an elevated administrator belong to the administrators) or the system.
var adminOwners = []string{"S-1-5-32-544", "S-1-5-18"}

// SecureDir: see secureDir; a folder that is there must belong to the administrators or to SYSTEM.
func (*System) SecureDir(dir, sddl string) error { return secureDir(dir, sddl, adminOwners) }

// CheckDir: see checkDir; a folder that is there must belong to the administrators or to SYSTEM.
func (*System) CheckDir(dir string) (bool, error) { return checkDir(dir, adminOwners) }

// checkDir tells whether the folder dir is there. One that is there must be a plain folder, not a link
// or a junction (what is done in it would be done in its target), and belong to one of owners: a user
// could have made it before the first install, with links and files of his own inside.
func checkDir(dir string, owners []string) (bool, error) {
	fi, err := os.Lstat(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return false, nil
	case err != nil:
		return false, err
	case fi.Mode().Type() != fs.ModeDir:
		return true, fmt.Errorf("%s is not a plain folder (a file, a link or a junction): it is not touched", dir)
	}
	return true, checkOwner(dir, owners)
}

// secureDir gives the folder dir the access list of sddl (spec 4.2). A folder that is not there is made
// with it at once, so it never has another list: ProgramData lets users make folders. A folder that is
// there must pass checkDir (the list would go to the target of a link); it then gets the list,
// protected, and Windows passes the inherited entries on to everything inside it.
func secureDir(dir, sddl string, owners []string) error {
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("the access list %q: %w", sddl, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("the access list %q: %w", sddl, err)
	}
	there, err := checkDir(dir, owners)
	if err != nil {
		return err
	}
	if !there {
		p, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			return err
		}
		sa := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
		if err := windows.CreateDirectory(p, sa); err != nil {
			return fmt.Errorf("make %s: %w", dir, err)
		}
	}
	err = windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
	if err != nil {
		return fmt.Errorf("the access list of %s: %w", dir, err)
	}
	return nil
}

// checkOwner: the owner of dir is one of owners.
func checkOwner(dir string, owners []string) error {
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.OWNER_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("the owner of %s: %w", dir, err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("the owner of %s: %w", dir, err)
	}
	if !slices.Contains(owners, owner.String()) {
		return fmt.Errorf("%s is there and belongs to %s: it was not made by an administrator; remove it first", dir, owner)
	}
	return nil
}

func (*System) WriteSetup(dataDir string, s place.Setup) error { return place.WriteSetup(dataDir, s) }

func (*System) WriteResult(path string, data []byte) error { return atomicfile.Write(path, data) }

// RegisterTask: the XML in UTF-16 LE with a BOM in dir\task.xml, schtasks /create /tn ... /xml ... /f
// (the task is replaced when it is there), the file removed after.
func (*System) RegisterTask(xml, dir string) error {
	file := filepath.Join(dir, "task.xml")
	if err := os.WriteFile(file, utf16File(xml), 0o600); err != nil {
		return err
	}
	defer os.Remove(file)
	return schtasks("/create", "/tn", TaskName, "/xml", file, "/f")
}

// DeleteTask: schtasks /delete when schtasks /query finds the task.
func (*System) DeleteTask() error {
	if schtasks("/query", "/tn", TaskName) != nil {
		return nil
	}
	return schtasks("/delete", "/tn", TaskName, "/f")
}

func (*System) RemoveAll(dir string) error { return os.RemoveAll(dir) }

// schtasks runs schtasks.exe of the system folder, never one found through PATH, without a window.
func schtasks(args ...string) error {
	dir, err := windows.GetSystemDirectory()
	if err != nil {
		return err
	}
	cmd := exec.Command(filepath.Join(dir, "schtasks.exe"), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("schtasks %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}
