package systs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Shapes of the real output of Tailscale 1.104 (values replaced by documentation ones).
const (
	statusOn = `{"Version":"1.104.2","BackendState":"Running","ExitNodeStatus":{"ID":"nX","Online":true,
 "TailscaleIPs":["100.64.0.1/32","fd7a:115c:a1e0::1/128"]},"Self":{"HostName":"pc"},"Peer":{}}`
	statusOff     = `{"BackendState":"Running","ExitNodeStatus":null}`
	statusStopped = `{"BackendState":"Stopped","ExitNodeStatus":{"TailscaleIPs":["100.64.0.1/32"]}}`
	prefs         = `{"ControlURL":"https://controlplane.tailscale.com","ExitNodeID":"nX","ExitNodeIP":"","ExitNodeAllowLANAccess":true,"CorpDNS":false}`
)

type call struct{ args []string }

func fake(t *testing.T, outputs map[string]string, fail error) (*Client, *[]call) {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "tailscale.exe")
	os.WriteFile(exe, []byte("x"), 0o644)
	var calls []call
	c := &Client{Exe: exe, Run: func(ctx context.Context, path string, args ...string) ([]byte, error) {
		if path != exe {
			t.Fatalf("tailscale.exe must be called by its full path, got %s", path)
		}
		calls = append(calls, call{args})
		if fail != nil {
			return nil, fail
		}
		return []byte(outputs[strings.Join(args, " ")]), nil
	}}
	return c, &calls
}

// Spec 5.6, 11: the path never follows the environment the elevated helper inherits.
func TestExeIgnoresTheEnvironment(t *testing.T) {
	user := t.TempDir()
	t.Setenv("ProgramFiles", user)
	cases := map[string]struct {
		dir  string
		err  error
		want string
	}{
		"registry":   {`D:\Apps`, nil, `D:\Apps\Tailscale\tailscale.exe`},
		"unreadable": {"", errors.New("no value"), `C:\Program Files\Tailscale\tailscale.exe`},
		"relative":   {`Program Files`, nil, `C:\Program Files\Tailscale\tailscale.exe`},
		"unexpanded": {`%SystemDrive%\Program Files`, nil, `C:\Program Files\Tailscale\tailscale.exe`},
	}
	for name, c := range cases {
		if got := exeIn(func() (string, error) { return c.dir, c.err }); got != c.want {
			t.Errorf("%s: %s", name, got)
		}
	}
	// The folder of the service's tailscaled.exe comes first (a client installed elsewhere).
	programFiles := func() (string, error) { return `D:\Apps`, nil }
	services := map[string]struct {
		image string
		err   error
		want  string
	}{
		"quoted":       {`"E:\Net\Tailscale\tailscaled.exe"`, nil, `E:\Net\Tailscale\tailscale.exe`},
		"with args":    {`E:\Net\Tailscale\tailscaled.exe -port 41641`, nil, `E:\Net\Tailscale\tailscale.exe`},
		"no service":   {"", errors.New("not found"), `D:\Apps\Tailscale\tailscale.exe`},
		"unexpanded":   {`"%ProgramFiles%\Tailscale\tailscaled.exe"`, nil, `D:\Apps\Tailscale\tailscale.exe`},
		"other binary": {`"E:\Net\other.exe"`, nil, `D:\Apps\Tailscale\tailscale.exe`},
		"relative":     {`Tailscale\tailscaled.exe`, nil, `D:\Apps\Tailscale\tailscale.exe`},
	}
	for name, c := range services {
		if got := exeFrom(func() (string, error) { return c.image, c.err }, programFiles); got != c.want {
			t.Errorf("service %s: %s", name, got)
		}
	}
	// The real registry values (read only).
	if exe := New().Exe; !filepath.IsAbs(exe) || strings.HasPrefix(strings.ToLower(exe), strings.ToLower(user)) {
		t.Fatalf("the path follows %%ProgramFiles%%: %s", exe)
	}
}

func TestStatus(t *testing.T) {
	cases := map[string]struct {
		out string
		on  bool
		ip  string
	}{
		"on":      {statusOn, true, "100.64.0.1"},
		"off":     {statusOff, false, ""},
		"stopped": {statusStopped, false, ""},
	}
	for name, c := range cases {
		client, _ := fake(t, map[string]string{"status --json": c.out}, nil)
		st, err := client.Status(context.Background())
		if err != nil || !st.Installed || st.ExitNodeOn != c.on || st.ExitNodeIP != c.ip {
			t.Errorf("%s: %+v %v", name, st, err)
		}
	}
}

func TestStatusNotInstalledOrFailing(t *testing.T) {
	c := &Client{Exe: filepath.Join(t.TempDir(), "missing", "tailscale.exe"), Run: func(context.Context, string, ...string) ([]byte, error) {
		t.Fatal("must not run a missing client")
		return nil, nil
	}}
	if st, err := c.Status(context.Background()); err != nil || st.Installed {
		t.Fatalf("%+v %v", st, err)
	}
	failing, _ := fake(t, nil, errors.New("service stopped"))
	st, err := failing.Status(context.Background())
	if err == nil || !st.Installed || st.ExitNodeOn {
		t.Fatalf("a failing client counts as no exit node: %+v %v", st, err)
	}
}

func TestAllowLANAndCommands(t *testing.T) {
	c, calls := fake(t, map[string]string{"debug prefs": prefs}, nil)
	if lan, err := c.AllowLAN(context.Background()); err != nil || !lan {
		t.Fatalf("%v %v", lan, err)
	}
	c.SetExitNode(context.Background(), "100.64.0.1", true)
	c.ClearExitNode(context.Background())
	got := []string{}
	for _, x := range *calls {
		got = append(got, strings.Join(x.args, " "))
	}
	want := "debug prefs|set --exit-node=100.64.0.1 --exit-node-allow-lan-access=true|set --exit-node="
	if strings.Join(got, "|") != want {
		t.Fatalf("calls %v", got)
	}
	if !strings.Contains(c.RestoreCommand("100.64.0.1", false), `set --exit-node=100.64.0.1 --exit-node-allow-lan-access=false`) {
		t.Fatal(c.RestoreCommand("100.64.0.1", false))
	}
	broken, _ := fake(t, map[string]string{"debug prefs": `{"CorpDNS":false}`}, nil)
	if _, err := broken.AllowLAN(context.Background()); err == nil {
		t.Fatal("a missing flag must be an error, not false")
	}
}
