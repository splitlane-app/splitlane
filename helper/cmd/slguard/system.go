package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// tsTimeout bounds one call of tailscale.exe: a hung CLI must not stall the guard in tripping.
const tsTimeout = 30 * time.Second

// winSystem is the real System. It shares no code with the helper on purpose.
type winSystem struct {
	tailscale string
}

func newSystem() *winSystem {
	return &winSystem{tailscale: filepath.Join(programFiles(), "Tailscale", "tailscale.exe")}
}

// programFiles is the 64-bit Program Files folder named by HKLM. The guard runs elevated with the
// user's environment, where %ProgramFiles% is the user's to change: it is never read.
func programFiles() string {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err == nil {
		defer k.Close()
		if v, _, err := k.GetStringValue("ProgramFilesDir"); err == nil && filepath.IsAbs(v) && !strings.Contains(v, "%") {
			return v
		}
	}
	return `C:\Program Files`
}

func (s *winSystem) ts(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), tsTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.tailscale, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd.Output()
}

func (s *winSystem) ExitNode() (bool, string, error) {
	out, err := s.ts("status", "--json")
	if err != nil {
		return false, "", err
	}
	var st struct {
		BackendState   string
		ExitNodeStatus *struct{ TailscaleIPs []string }
	}
	if err := json.Unmarshal(out, &st); err != nil {
		return false, "", err
	}
	if st.BackendState != "Running" || st.ExitNodeStatus == nil {
		return false, "", nil
	}
	for _, p := range st.ExitNodeStatus.TailscaleIPs {
		if ip := strings.SplitN(p, "/", 2)[0]; !strings.Contains(ip, ":") {
			return true, ip, nil
		}
	}
	return true, "", nil
}

func (s *winSystem) AllowLAN() (bool, error) {
	out, err := s.ts("debug", "prefs")
	if err != nil {
		return false, err
	}
	var p struct{ ExitNodeAllowLANAccess *bool }
	if err := json.Unmarshal(out, &p); err != nil || p.ExitNodeAllowLANAccess == nil {
		return false, fmt.Errorf("no ExitNodeAllowLANAccess in debug prefs (%v)", err)
	}
	return *p.ExitNodeAllowLANAccess, nil
}

func (s *winSystem) SetExitNode(ip string, allowLAN bool) error {
	_, err := s.ts("set", "--exit-node="+ip, "--exit-node-allow-lan-access="+strconv.FormatBool(allowLAN))
	return err
}

// proc is one entry of the process snapshot.
type proc struct {
	pid, parent uint32
	image       string
}

func snapshot() []proc {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(snap)
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	var out []proc
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		out = append(out, proc{pid: e.ProcessID, parent: e.ParentProcessID, image: windows.UTF16ToString(e.ExeFile[:])})
	}
	return out
}

// withImage: the ids of the processes with this image name.
func withImage(ps []proc, image string) []uint32 {
	var out []uint32
	for _, p := range ps {
		if strings.EqualFold(p.image, image) {
			out = append(out, p.pid)
		}
	}
	return out
}

// childrenOf: the ids of the processes with this image name that one of parents started. A child
// keeps its parent's id after the parent has ended.
func childrenOf(ps []proc, parents []uint32, image string) []uint32 {
	var out []uint32
	for _, p := range ps {
		if strings.EqualFold(p.image, image) && slices.Contains(parents, p.parent) {
			out = append(out, p.pid)
		}
	}
	return out
}

// terminate ends the processes and returns the ids of those that ended.
func terminate(ids []uint32) []uint32 {
	var ended []uint32
	for _, pid := range ids {
		h, err := windows.OpenProcess(windows.PROCESS_TERMINATE|windows.SYNCHRONIZE, false, pid)
		if err != nil {
			continue
		}
		if windows.TerminateProcess(h, 1) == nil {
			windows.WaitForSingleObject(h, 5000)
			ended = append(ended, pid)
		}
		windows.CloseHandle(h)
	}
	return ended
}

func (s *winSystem) Count(image string) int { return len(withImage(snapshot(), image)) }

func (s *winSystem) Kill(image string) []uint32 { return terminate(withImage(snapshot(), image)) }

func (s *winSystem) KillChildren(parents []uint32, image string) int {
	return len(terminate(childrenOf(snapshot(), parents, image)))
}

func (s *winSystem) AdapterPresent() bool {
	_, err := net.InterfaceByName("SplitLane")
	return err == nil
}

func (s *winSystem) Launch(command []string) error {
	cmd := exec.Command(command[0], command[1:]...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
