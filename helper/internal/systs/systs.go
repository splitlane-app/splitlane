// Package systs talks to the official Tailscale client through tailscale.exe, always by its
// full path from the install folder, never through PATH (spec 5.6, 11).
package systs

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"splitlane/helper/internal/engine"
	"splitlane/helper/internal/winhost"
)

// fallbackProgramFiles is used when the registry value cannot be read or is not a plain path.
const fallbackProgramFiles = `C:\Program Files`

// DefaultExe is the CLI in the install folder of the official client (spec 5.6): the folder of
// tailscaled.exe that the Tailscale service names (its ImagePath in HKLM\SYSTEM), else the
// Program Files folder that the registry names (HKLM, 64-bit view). Never %ProgramFiles%: the
// helper runs elevated with the user's environment, and HKCU\Environment can set ProgramFiles to
// a folder the user writes. Only binaries that administrators write may run elevated (spec 11).
func DefaultExe() string { return exeFrom(serviceImagePath, programFilesDir) }

// serviceImagePath reads ImagePath of HKLM\SYSTEM\CurrentControlSet\Services\Tailscale.
func serviceImagePath() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Services\Tailscale`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("ImagePath")
	return v, err
}

// serviceDir is the folder of tailscaled.exe in an ImagePath: a quoted path up to its closing
// quote, an unquoted one up to tailscaled.exe. Another program, a relative path or a value that
// would need the environment to expand is not used.
func serviceDir(imagePath string) (string, bool) {
	v := strings.TrimSpace(imagePath)
	if strings.HasPrefix(v, `"`) {
		end := strings.Index(v[1:], `"`)
		if end < 0 {
			return "", false
		}
		v = v[1 : 1+end]
	} else if i := strings.Index(strings.ToLower(v), "tailscaled.exe"); i >= 0 {
		v = v[:i+len("tailscaled.exe")]
	}
	if !filepath.IsAbs(v) || strings.Contains(v, "%") || !strings.EqualFold(filepath.Base(v), "tailscaled.exe") {
		return "", false
	}
	return filepath.Dir(v), true
}

// exeFrom takes the CLI next to the service's tailscaled.exe, else from the Program Files folder.
func exeFrom(service, programFiles func() (string, error)) string {
	if v, err := service(); err == nil {
		if dir, ok := serviceDir(v); ok {
			return filepath.Join(dir, "tailscale.exe")
		}
	}
	return exeIn(programFiles)
}

// programFilesDir reads ProgramFilesDir from HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion.
func programFilesDir() (string, error) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion`,
		registry.QUERY_VALUE|registry.WOW64_64KEY)
	if err != nil {
		return "", err
	}
	defer k.Close()
	v, _, err := k.GetStringValue("ProgramFilesDir")
	return v, err
}

// exeIn builds the path from the folder read; an error, a relative path or a value that would need
// the environment to expand gives the fallback.
func exeIn(read func() (string, error)) string {
	dir, err := read()
	if err != nil || !filepath.IsAbs(dir) || strings.Contains(dir, "%") {
		dir = fallbackProgramFiles
	}
	return filepath.Join(dir, "Tailscale", "tailscale.exe")
}

// Runner runs exe with args and returns its standard output.
type Runner func(ctx context.Context, exe string, args ...string) ([]byte, error)

// Client implements engine.SystemTailscale.
type Client struct {
	Exe string
	Run Runner
}

var _ engine.SystemTailscale = (*Client)(nil)

// New uses the default install path and runs tailscale.exe without a window, with the system's
// environment (winhost.SystemEnviron).
func New() *Client { return &Client{Exe: DefaultExe(), Run: run} }

func run(ctx context.Context, exe string, args ...string) ([]byte, error) {
	env, err := winhost.SystemEnviron()
	if err != nil {
		return nil, fmt.Errorf("%s %s: the system environment: %w", filepath.Base(exe), strings.Join(args, " "), err)
	}
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s %s: %v: %s", filepath.Base(exe), strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

func (c *Client) installed() bool {
	_, err := os.Stat(c.Exe)
	return err == nil
}

// Status reads `tailscale status --json`. Not installed: Installed false. A stopped service,
// a missing login or any other error: no exit node (5.6), with the error for the log.
func (c *Client) Status(ctx context.Context) (engine.SysStatus, error) {
	if !c.installed() {
		return engine.SysStatus{}, nil
	}
	out, err := c.Run(ctx, c.Exe, "status", "--json")
	if err != nil {
		return engine.SysStatus{Installed: true}, err
	}
	return ParseStatus(out)
}

// ParseStatus takes BackendState and ExitNodeStatus.TailscaleIPs (first IPv4, without "/32").
func ParseStatus(out []byte) (engine.SysStatus, error) {
	var s struct {
		BackendState   string
		ExitNodeStatus *struct {
			TailscaleIPs []string
		}
	}
	if err := json.Unmarshal(out, &s); err != nil {
		return engine.SysStatus{Installed: true}, fmt.Errorf("tailscale status --json: %w", err)
	}
	st := engine.SysStatus{Installed: true}
	if s.BackendState != "Running" || s.ExitNodeStatus == nil {
		return st, nil
	}
	st.ExitNodeOn = true
	for _, p := range s.ExitNodeStatus.TailscaleIPs {
		ip := strings.SplitN(p, "/", 2)[0]
		if !strings.Contains(ip, ":") {
			st.ExitNodeIP = ip
			break
		}
	}
	return st, nil
}

// Prefs reads WantRunning and the exit node of the settings from `tailscale debug prefs` (5.4 step 1):
// a client the user disconnected keeps the exit node there, by address (ExitNodeIP) until it has
// resolved it, by id (ExitNodeID) after, and never shows it in its status.
func (c *Client) Prefs(ctx context.Context) (engine.SysPrefs, error) {
	out, err := c.Run(ctx, c.Exe, "debug", "prefs")
	if err != nil {
		return engine.SysPrefs{}, err
	}
	var p struct {
		WantRunning *bool
		ExitNodeID  string
		ExitNodeIP  string
	}
	if err := json.Unmarshal(out, &p); err != nil || p.WantRunning == nil {
		return engine.SysPrefs{}, fmt.Errorf("tailscale debug prefs: no WantRunning (%v)", err)
	}
	return engine.SysPrefs{WantRunning: *p.WantRunning, ExitNode: p.ExitNodeID != "" || p.ExitNodeIP != ""}, nil
}

// AllowLAN reads ExitNodeAllowLANAccess from `tailscale debug prefs` (it is not in status, 5.3 step 2).
func (c *Client) AllowLAN(ctx context.Context) (bool, error) {
	out, err := c.Run(ctx, c.Exe, "debug", "prefs")
	if err != nil {
		return false, err
	}
	var p struct {
		ExitNodeAllowLANAccess *bool
	}
	if err := json.Unmarshal(out, &p); err != nil || p.ExitNodeAllowLANAccess == nil {
		return false, fmt.Errorf("tailscale debug prefs: no ExitNodeAllowLANAccess (%v)", err)
	}
	return *p.ExitNodeAllowLANAccess, nil
}

// SetExitNode is the restore command of 5.4 step 1.
func (c *Client) SetExitNode(ctx context.Context, ip string, allowLAN bool) error {
	_, err := c.Run(ctx, c.Exe, "set", "--exit-node="+ip, "--exit-node-allow-lan-access="+strconv.FormatBool(allowLAN))
	return err
}

// ClearExitNode is step 6 of 5.3.
func (c *Client) ClearExitNode(ctx context.Context) error {
	_, err := c.Run(ctx, c.Exe, "set", "--exit-node=")
	return err
}

// RestoreCommand is the manual command shown with restore_pending (5.2).
func (c *Client) RestoreCommand(ip string, allowLAN bool) string {
	return fmt.Sprintf(`"%s" set --exit-node=%s --exit-node-allow-lan-access=%v`, c.Exe, ip, allowLAN)
}

// Dir is the install folder (rule 5 of 6.2).
func (c *Client) Dir() string { return filepath.Dir(c.Exe) }
