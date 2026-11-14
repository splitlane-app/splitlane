package install

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/protocol"
)

// readDACL reads the access list of path back: its SDDL and whether it is protected from inheritance.
func readDACL(t *testing.T, path string) (string, bool) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil {
		t.Fatal(err)
	}
	return sd.String(), control&windows.SE_DACL_PROTECTED != 0
}

// The access lists of spec 4.2 with the real API, on a temporary folder and with an SDDL that keeps the
// current user's full access. A folder that is not there is made with the list; one that is there gets
// it, protected, with exactly its entries, and a file in it gets them as inherited entries. A junction
// in place of the folder is refused and its target keeps its list (the list would go to the target); so
// is a folder whose owner is not one of the owners given (one that a user made before the setup).
func TestApplyACLOnTempDir(t *testing.T) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	me := u.User.Sid.String()
	owners := []string{me, "S-1-5-32-544"} // the folders of this test belong to the user, or to the administrators when it runs elevated
	sddl := "D:PAI(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;" + me + ")"
	root := t.TempDir()

	made := filepath.Join(root, "made")
	if err := secureDir(made, sddl, owners); err != nil {
		t.Fatal(err)
	}
	if got, protected := readDACL(t, made); got != sddl || !protected {
		t.Fatalf("a new folder: %s protected=%v", got, protected)
	}

	there := filepath.Join(root, "there")
	if err := os.Mkdir(there, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(there, "state.json")
	if err := os.WriteFile(file, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, protected := readDACL(t, there); got == sddl || protected {
		t.Fatalf("before: %s protected=%v", got, protected)
	}
	if err := secureDir(there, sddl, owners); err != nil {
		t.Fatal(err)
	}
	if got, protected := readDACL(t, there); got != sddl || !protected {
		t.Fatalf("a folder that was there: %s protected=%v", got, protected)
	}
	if got, protected := readDACL(t, file); got != "D:AI(A;ID;FA;;;SY)(A;ID;FA;;;BA)(A;ID;FA;;;"+me+")" || protected {
		t.Fatalf("the file in it: %s protected=%v", got, protected)
	}

	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	before, _ := readDACL(t, target)
	link := filepath.Join(root, "link")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	if err := secureDir(link, sddl, owners); err == nil || !strings.Contains(err.Error(), "is not a plain folder") {
		t.Fatalf("a junction: %v", err)
	}
	if after, _ := readDACL(t, target); after != before {
		t.Fatalf("the junction's target changed: %s -> %s", before, after)
	}

	other := filepath.Join(root, "other")
	if err := os.Mkdir(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := secureDir(other, sddl, []string{"S-1-5-18"}); err == nil || !strings.Contains(err.Error(), "was not made by an administrator") {
		t.Fatalf("a folder of another owner: %v", err)
	}
	if got, _ := readDACL(t, other); got == sddl {
		t.Fatal("a folder of another owner got the list")
	}
}

// The check of the data folder before its files are read (review F14-1) with the real API, on temporary
// folders: a folder that is not there is no error; a plain folder of one of the owners passes; a folder
// of another owner, a junction and a file are refused.
func TestCheckDirOnTempDir(t *testing.T) {
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	owners := []string{u.User.Sid.String(), "S-1-5-32-544"} // the user, or the administrators when the test runs elevated
	root := t.TempDir()

	if there, err := checkDir(filepath.Join(root, "missing"), owners); there || err != nil {
		t.Fatalf("a folder that is not there: %v %v", there, err)
	}
	dir := filepath.Join(root, "dir")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if there, err := checkDir(dir, owners); !there || err != nil {
		t.Fatalf("a folder of the owners: %v %v", there, err)
	}
	if there, err := checkDir(dir, []string{"S-1-5-18"}); !there || err == nil || !strings.Contains(err.Error(), "was not made by an administrator") {
		t.Fatalf("a folder of another owner: %v %v", there, err)
	}
	link := filepath.Join(root, "link")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", link, dir).CombinedOutput(); err != nil {
		t.Fatalf("mklink: %v %s", err, out)
	}
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{link, file} {
		if _, err := checkDir(p, owners); err == nil || !strings.Contains(err.Error(), "is not a plain folder") {
			t.Fatalf("%s: %v", p, err)
		}
	}
}

// The pipe client of install on a test pipe whose server is this process (the helper's process, which
// never ends here). No pipe: no helper. A helper that answers helper.shutdown and stays is reported
// after the wait with its process and the phase and reason of its last state event (restore_pending,
// 5.4); an error answer to helper.shutdown is reported at once.
func TestShutdownHelperOnATestPipe(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, time.Now().UnixNano())
	s := &System{Pipe: name}
	if found, err := s.ShutdownHelper(time.Second); found || err != nil {
		t.Fatalf("no pipe: %v %v", found, err)
	}
	l, err := winio.ListenPipe(name, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	refuse := make(chan bool, 2)
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				sc := protocol.NewScanner(conn)
				for sc.Scan() {
					var req protocol.Request
					json.Unmarshal(sc.Bytes(), &req)
					switch req.Method {
					case protocol.MethodHello:
						conn.Write(protocol.EncodeResult(req.ID, protocol.HelloResult{Protocol: protocol.Version}))
					case protocol.MethodShutdown:
						if <-refuse {
							conn.Write(protocol.EncodeError(req.ID, protocol.Errorf(protocol.CodeInternal, "test")))
							continue
						}
						conn.Write(protocol.EncodeResult(req.ID, struct{}{}))
						conn.Write(protocol.EncodeEvent("state", protocol.State{Phase: protocol.PhaseStopping, Reason: &protocol.Reason{
							Code: protocol.ReasonRestorePending, Params: map[string]string{protocol.ParamError: "exit node not confirmed"}}}))
					}
				}
			}(conn)
		}
	}()

	refuse <- false
	start := time.Now()
	found, err := s.ShutdownHelper(time.Second)
	want := fmt.Sprintf("the helper (process %d) did not end within 1s; its last state: phase=stopping reason=restore_pending (exit node not confirmed)", os.Getpid())
	if !found || err == nil || err.Error() != want || time.Since(start) < time.Second {
		t.Fatalf("a helper that stays: %v %v", found, err)
	}

	refuse <- true
	found, err = s.ShutdownHelper(time.Minute)
	if !found || err == nil || !strings.Contains(err.Error(), "refused helper.shutdown: internal: test") {
		t.Fatalf("a refused shutdown: %v %v", found, err)
	}
}
