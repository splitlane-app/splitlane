package serversetup

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"splitlane/helper/internal/connector"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// setupRun runs Run and collects its events.
func setupRun(ctx context.Context, req Request, d Deps) ([]Event, Done, *Failure) {
	var mu sync.Mutex
	var events []Event
	done, fail := Run(ctx, req, d, func(e Event) {
		mu.Lock()
		events = append(events, e)
		mu.Unlock()
	})
	mu.Lock()
	defer mu.Unlock()
	return events, done, fail
}

func passwordRequest(f *fakeServer, password string) Request {
	return Request{Host: "127.0.0.1", Port: f.port, User: f.user, Password: []byte(password)}
}

// noSecrets fails the test when a secret is in an event, in the failure or in a command line of the
// server.
func noSecrets(t *testing.T, f *fakeServer, events []Event, fail *Failure, secrets ...string) {
	t.Helper()
	out := mustJSON(t, events) + mustJSON(t, fail)
	for _, e := range events {
		out += "\n" + e.Detail
	}
	if fail != nil {
		out += "\n" + fail.Detail
	}
	for _, s := range secrets {
		if s == "" {
			continue
		}
		if strings.Contains(out, s) {
			t.Errorf("a secret %q is in the events or the failure:\n%s", s, out)
		}
		if f == nil {
			continue
		}
		for _, c := range f.commands() {
			if strings.Contains(c, s) {
				t.Errorf("a secret %q is in the command line %q", s, c)
			}
		}
	}
}

func wantFailure(t *testing.T, fail *Failure, code, detail string) {
	t.Helper()
	if fail == nil {
		t.Fatalf("no failure, want %s %q", code, detail)
	}
	if fail.Code != code || detail != "*" && fail.Detail != detail {
		t.Fatalf("failure %+v, want %s %q", *fail, code, detail)
	}
}

// steps keeps step and state of the events.
func steps(events []Event) []string {
	var out []string
	for _, e := range events {
		out = append(out, e.Step+" "+e.State)
	}
	return out
}

func TestRunPasswordAsRoot(t *testing.T) {
	// Review Focus 1: the password reaches the server byte for byte, never through a shell.
	f := newFakeServer(t, func(f *fakeServer) { f.password = focusPassword })
	f.setKit(fakeReply{stdout: kitOutput(resultLine(t, []string{testLinkVLESS, testLinkHy2}, nil, []string{"harden_failed"}))})
	req := passwordRequest(f, focusPassword)
	events, done, fail := setupRun(context.Background(), req, testDeps(t, nil))
	if fail != nil {
		t.Fatalf("%+v", *fail)
	}
	if done.Links != testLinkVLESS+"\n"+testLinkHy2 || done.Subscription != nil || !reflect.DeepEqual(done.Warnings, []string{"harden_failed"}) {
		t.Fatalf("%+v", done)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(f.port))
	want := append([]Event{
		{"connect", "start", addr},
		{"connect", "done", "logged in as root"},
		{"system", "start", "checking the server"},
		{"system", "done", "debian 12, x86_64, root"},
		{"install", "start", "uploading the server kit"},
	}, kitEvents...)
	want = append(want, Event{"subscription", "skip", "no subscription address"})
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events\n%+v\nwant\n%+v", events, want)
	}
	if got := f.sentPasswords(); !reflect.DeepEqual(got, []string{focusPassword}) {
		t.Fatalf("passwords sent %q", got)
	}
	wantCmds := []string{
		"cat /etc/os-release",
		"uname -m",
		"test -d /run/systemd/system",
		"id -u",
		"mktemp /tmp/splitlane-kit.XXXXXX",
		"cat > " + tempFile,
		"chmod 600 " + tempFile,
		"bash " + tempFile + kitFlags,
		"rm -f " + tempFile,
	}
	if got := f.commands(); !reflect.DeepEqual(got, wantCmds) {
		t.Fatalf("commands\n%q\nwant\n%q", got, wantCmds)
	}
	if b, _ := f.file(tempFile); !bytes.Equal(b, Kit) {
		t.Fatalf("uploaded %d bytes, want the kit (%d bytes)", len(b), len(Kit))
	}
	for _, e := range f.records() {
		if e.cmd != "cat > "+tempFile && len(e.stdin) != 0 {
			t.Fatalf("%q got stdin %q", e.cmd, e.stdin)
		}
	}
	if got := f.removedFiles(); !reflect.DeepEqual(got, []string{tempFile}) {
		t.Fatalf("removed %q", got)
	}
	noSecrets(t, f, events, fail, focusPassword, testLinkVLESS, testLinkHy2)
}

func TestRunKeyboardInteractive(t *testing.T) {
	cases := []struct {
		name   string
		rounds []kbdRound
		code   string // "" means success
	}{
		{"one question", nil, ""},
		{"an empty round, then the question", []kbdRound{
			{instruction: "Welcome"},
			{questions: []string{"Password: "}, echos: []bool{false}},
		}, ""},
		{"three rounds", []kbdRound{
			{instruction: "a"},
			{instruction: "b"},
			{questions: []string{"Password: "}, echos: []bool{false}},
		}, ""},
		{"a question with echo", []kbdRound{
			{questions: []string{"Verification code: "}, echos: []bool{true}},
		}, CodeSSHAuth},
		{"four rounds", []kbdRound{
			{instruction: "a"},
			{instruction: "b"},
			{instruction: "c"},
			{questions: []string{"Password: "}, echos: []bool{false}},
		}, CodeSSHAuth},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, func(f *fakeServer) {
				f.password = focusPassword
				f.passwordAuth = false
				f.kbdRounds = c.rounds
			})
			events, _, fail := setupRun(context.Background(), passwordRequest(f, focusPassword), testDeps(t, nil))
			if c.code == "" {
				if fail != nil {
					t.Fatalf("%+v", *fail)
				}
				if got := f.sentPasswords(); !reflect.DeepEqual(got, []string{focusPassword}) {
					t.Fatalf("passwords sent %q", got)
				}
			} else {
				wantFailure(t, fail, c.code, "")
				if len(f.commands()) != 0 {
					t.Fatalf("commands %q", f.commands())
				}
				// The password is not typed into a question shown as typed, nor after the third round.
				if got := f.sentPasswords(); len(got) != 0 {
					t.Fatalf("answers sent %q", got)
				}
			}
			noSecrets(t, f, events, fail, focusPassword)
		})
	}
}

func TestRunWrongPassword(t *testing.T) {
	for _, password := range []bool{true, false} {
		t.Run("password method "+strconv.FormatBool(password), func(t *testing.T) {
			f := newFakeServer(t, func(f *fakeServer) { f.passwordAuth = password })
			events, _, fail := setupRun(context.Background(), passwordRequest(f, focusPassword), testDeps(t, nil))
			wantFailure(t, fail, CodeSSHAuth, "")
			// One try a run: a refused password is not tried again by keyboard-interactive, so
			// fail2ban and MaxAuthTries count one failure.
			if got := f.sentPasswords(); !reflect.DeepEqual(got, []string{focusPassword}) {
				t.Fatalf("passwords sent %q", got)
			}
			if !reflect.DeepEqual(steps(events), []string{"connect start"}) {
				t.Fatalf("%v", steps(events))
			}
			noSecrets(t, f, events, fail, focusPassword)
		})
	}
	t.Run("another user", func(t *testing.T) {
		f := newFakeServer(t, nil)
		req := passwordRequest(f, f.password)
		req.User = "admin"
		_, _, fail := setupRun(context.Background(), req, testDeps(t, nil))
		wantFailure(t, fail, CodeSSHAuth, "")
	})
}

// utf8BOM is the byte order mark Notepad puts in front of a text file.
func utf8BOM() []byte { return []byte{0xef, 0xbb, 0xbf} }

func newTestKey(t *testing.T) (ed25519.PrivateKey, ssh.PublicKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return priv, sp
}

// keyText is the OpenSSH text of a private key, encrypted with passphrase unless it is empty.
func keyText(t *testing.T, priv ed25519.PrivateKey, passphrase string) []byte {
	t.Helper()
	var block *pem.Block
	var err error
	if passphrase == "" {
		block, err = ssh.MarshalPrivateKey(priv, "test@example.com")
	} else {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, "test@example.com", []byte(passphrase))
	}
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}

func TestRunKey(t *testing.T) {
	priv, pub := newTestKey(t)
	other, _ := newTestKey(t)
	plain := keyText(t, priv, "")
	locked := keyText(t, priv, focusPassword)
	ppk := []byte("PuTTY-User-Key-File-3: ssh-ed25519\r\nEncryption: none\r\nComment: test\r\nPublic-Lines: 2\r\nAAAAC3NzaC1lZDI1NTE5\r\nAAAA\r\n")
	cases := []struct {
		name       string
		key        []byte
		passphrase string
		code       string // "" means success
		detail     string
		dial       bool // the server is reached
	}{
		{"a key", plain, "", "", "", true},
		{"a key with its passphrase", locked, focusPassword, "", "", true},
		{"a passphrase for a key without one", plain, "ignored", "", "", true},
		{"a key with a BOM and CRLF", append(utf8BOM(), bytes.ReplaceAll(plain, []byte("\n"), []byte("\r\n"))...), "", "", "", true},
		{"no passphrase", locked, "", CodeSSHKey, "passphrase", false},
		{"a wrong passphrase", locked, "wrong", CodeSSHKey, "passphrase", false},
		{"a PuTTY key", ppk, "", CodeSSHKey, "ppk", false},
		{"a PuTTY key after spaces", append(append(utf8BOM(), " \r\n"...), ppk...), "", CodeSSHKey, "ppk", false},
		{"not a key", []byte("hello\n"), "", CodeSSHKey, "format", false},
		{"a public key", []byte(ssh.MarshalAuthorizedKey(pub)), "", CodeSSHKey, "format", false},
		{"a key the server does not know", keyText(t, other, ""), "", CodeSSHAuth, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, func(f *fakeServer) {
				f.key = pub
				f.password = ""
			})
			d := testDeps(t, nil)
			dialed := false
			dial := d.Dial
			d.Dial = func(ctx context.Context, network, addr string) (net.Conn, error) {
				dialed = true
				return dial(ctx, network, addr)
			}
			req := Request{Host: "127.0.0.1", Port: f.port, User: "root", Key: c.key, Passphrase: []byte(c.passphrase)}
			events, done, fail := setupRun(context.Background(), req, d)
			if c.code == "" {
				if fail != nil {
					t.Fatalf("%+v", *fail)
				}
				if done.Links == "" {
					t.Fatalf("%+v", done)
				}
			} else {
				wantFailure(t, fail, c.code, c.detail)
			}
			if dialed != c.dial {
				t.Fatalf("dialed %v, want %v", dialed, c.dial)
			}
			if len(f.sentPasswords()) != 0 {
				t.Fatalf("passwords sent %q", f.sentPasswords())
			}
			noSecrets(t, f, events, fail, focusPassword, string(c.key[min(len(c.key), 40):]))
		})
	}
}

func TestRunUnreachable(t *testing.T) {
	closed, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	closedPort := closed.Addr().(*net.TCPAddr).Port
	closed.Close()

	// A server that accepts and says nothing, and one that is not SSH.
	silent := listen(t, func(c net.Conn) { io.Copy(io.Discard, c) }) // silent until the client goes
	web := listen(t, func(c net.Conn) { c.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\n")) })

	cases := []struct {
		name   string
		port   int
		dial   func(context.Context, string, string) (net.Conn, error)
		detail string
	}{
		{"nobody listens", closedPort, nil, "*"},
		{"no answer to the connection", 22, func(ctx context.Context, _, _ string) (net.Conn, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		}, "timeout"},
		{"a name that does not resolve", 22, func(context.Context, string, string) (net.Conn, error) {
			return nil, &net.DNSError{Err: "no such host", Name: "vps.example.com", IsNotFound: true}
		}, "name"},
		{"a silent server", silent, nil, "timeout"},
		{"not an SSH server", web, nil, "*"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := testDeps(t, nil)
			d.ConnectTimeout = 300 * time.Millisecond
			if c.dial != nil {
				d.Dial = c.dial
			}
			start := time.Now()
			req := Request{Host: "127.0.0.1", Port: c.port, User: "root", Password: []byte(focusPassword)}
			events, _, fail := setupRun(context.Background(), req, d)
			wantFailure(t, fail, CodeSSHUnreachable, c.detail)
			if !reflect.DeepEqual(steps(events), []string{"connect start"}) {
				t.Fatalf("%v", steps(events))
			}
			if time.Since(start) > 4*time.Second {
				t.Fatalf("took %v", time.Since(start))
			}
			noSecrets(t, nil, events, fail, focusPassword)
		})
	}
}

// listen serves each connection with serve on 127.0.0.1 and returns the port.
func listen(t *testing.T, serve func(net.Conn)) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				serve(c)
			}()
		}
	}()
	t.Cleanup(func() {
		ln.Close()
		wg.Wait()
	})
	return ln.Addr().(*net.TCPAddr).Port
}

func TestRunSystemUnsupported(t *testing.T) {
	cases := []struct {
		name    string
		replies map[string]fakeReply
		detail  string
	}{
		{"Debian 11", map[string]fakeReply{"cat /etc/os-release": {stdout: "ID=debian\nVERSION_ID=\"11\"\n"}}, "debian 11"},
		{"CentOS", map[string]fakeReply{"cat /etc/os-release": {stdout: "NAME=\"CentOS Stream\"\nID=\"centos\"\nID_LIKE=\"rhel fedora\"\nVERSION_ID=\"9\"\n"}}, "centos 9"},
		{"Ubuntu 20.04", map[string]fakeReply{"cat /etc/os-release": {stdout: "ID=ubuntu\nVERSION_ID=\"20.04\"\n"}}, "ubuntu 20.04"},
		{"Ubuntu 21.10", map[string]fakeReply{"cat /etc/os-release": {stdout: "ID=ubuntu\nVERSION_ID=\"21.10\"\n"}}, "ubuntu 21.10"},
		{"Debian testing without a version", map[string]fakeReply{"cat /etc/os-release": {stdout: "ID=debian\nVERSION_CODENAME=trixie\n"}}, "debian"},
		{"Mint, like Ubuntu", map[string]fakeReply{"cat /etc/os-release": {stdout: "ID=linuxmint\nID_LIKE=\"ubuntu debian\"\nVERSION_ID=\"21.3\"\n"}}, "linuxmint 21.3"},
		{"a version that is not a number", map[string]fakeReply{"cat /etc/os-release": {stdout: "ID=debian\nVERSION_ID=\"12x\"\n"}}, "debian 12x"},
		{"no os-release", map[string]fakeReply{"cat /etc/os-release": {stderr: "cat: /etc/os-release: No such file or directory\n", status: 1}}, "os-release"},
		{"armv7l", map[string]fakeReply{"uname -m": {stdout: "armv7l\n"}}, "armv7l"},
		{"i686", map[string]fakeReply{"uname -m": {stdout: "i686\n"}}, "i686"},
		{"no systemd", map[string]fakeReply{"test -d /run/systemd/system": {status: 1}}, "systemd"},
		{"a long odd name", map[string]fakeReply{"cat /etc/os-release": {stdout: "ID=" + strings.Repeat("x", 500) + "\x1b\nVERSION_ID=1\n"}}, strings.Repeat("x", 64) + "… 1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, nil)
			for p, r := range c.replies {
				f.setReply(p, r)
			}
			events, _, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
			wantFailure(t, fail, CodeOSUnsupported, c.detail)
			if !reflect.DeepEqual(steps(events), []string{"connect start", "connect done", "system start"}) {
				t.Fatalf("%v", steps(events))
			}
			for _, cmd := range f.commands() {
				if strings.HasPrefix(cmd, "mktemp") || strings.HasPrefix(cmd, "id ") {
					t.Fatalf("ran %q", cmd)
				}
			}
		})
	}
}

func TestRunSystemSupported(t *testing.T) {
	cases := []struct {
		name, osRelease, arch, detail string
	}{
		{"Debian 12", osDebian12, "x86_64", "debian 12, x86_64, root"},
		{"Debian 13", "ID=debian\nVERSION_ID=\"13\"\n", "aarch64", "debian 13, aarch64, root"},
		{"Ubuntu 22.04", "ID=ubuntu\nVERSION_ID=\"22.04\"\n", "x86_64", "ubuntu 22.04, x86_64, root"},
		{"Ubuntu 24.04", "NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\nVERSION_ID='24.04'\n", "aarch64", "ubuntu 24.04, aarch64, root"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, nil)
			f.setReply("cat /etc/os-release", fakeReply{stdout: c.osRelease})
			f.setReply("uname -m", fakeReply{stdout: c.arch + "\n"})
			events, _, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
			if fail != nil {
				t.Fatalf("%+v", *fail)
			}
			if events[3] != (Event{"system", "done", c.detail}) {
				t.Fatalf("%+v", events[3])
			}
		})
	}
}

// TestRunGreetingShell: a ~/.bashrc that prints a greeting before every command does not confuse the
// answers of uname, id and mktemp, nor the kit's lines.
func TestRunGreetingShell(t *testing.T) {
	for _, sudo := range []int{sudoRoot, sudoPassword} {
		t.Run(strconv.Itoa(sudo), func(t *testing.T) {
			f := newFakeServer(t, func(f *fakeServer) {
				f.greeting = "Welcome to your VPS!\n\n"
				f.sudo = sudo
				if sudo != sudoRoot {
					f.user = "kit"
				}
			})
			events, done, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
			if fail != nil {
				t.Fatalf("%+v", *fail)
			}
			if done.Links == "" {
				t.Fatalf("%+v", done)
			}
			want := "debian 12, x86_64, root"
			if sudo != sudoRoot {
				want = "debian 12, x86_64, sudo with the password"
			}
			if events[3] != (Event{"system", "done", want}) {
				t.Fatalf("%+v", events[3])
			}
			if b, _ := f.file(tempFile); !bytes.Equal(b, Kit) {
				t.Fatal("the kit was not uploaded")
			}
		})
	}
}

func TestRunPrivileges(t *testing.T) {
	type check func(t *testing.T, f *fakeServer)
	// sentOnlyTo: the password went on stdin only to the commands named, as the whole stdin.
	sentOnlyTo := func(prefixes ...string) check {
		return func(t *testing.T, f *fakeServer) {
			t.Helper()
			for _, e := range f.records() {
				want := false
				for _, p := range prefixes {
					want = want || strings.HasPrefix(e.cmd, p)
				}
				switch {
				case want && string(e.stdin) != focusPassword+"\n":
					t.Errorf("%q got stdin %q, want the password and a newline", e.cmd, e.stdin)
				case !want && bytes.Contains(e.stdin, []byte(focusPassword)):
					t.Errorf("%q got the password", e.cmd)
				}
			}
		}
	}
	cases := []struct {
		name    string
		sudo    int
		lecture bool
		sudoPw  string // the password sudo wants; "" means the login password
		key     bool   // log in with a key
		cmds    []string
		code    string // "" means success
		detail  string
		check   check
		sysText string
	}{
		{name: "root", sudo: sudoRoot, cmds: []string{"id -u", "bash " + tempFile + kitFlags}, check: sentOnlyTo(), sysText: "root"},
		{name: "sudo without a password", sudo: sudoNoPasswd,
			cmds:  []string{"id -u", "sudo -n true", "sudo -n bash " + tempFile + kitFlags},
			check: sentOnlyTo(), sysText: "sudo"},
		{name: "sudo with the password", sudo: sudoPassword,
			cmds:  []string{"id -u", "sudo -n true", "sudo -S -p '' -v", "sudo -k -S -p '' bash " + tempFile + kitFlags},
			check: sentOnlyTo("sudo -S -p '' -v", "sudo -k -S -p '' bash "), sysText: "sudo with the password"},
		{name: "sudo with its lecture", sudo: sudoPassword, lecture: true,
			cmds:  []string{"id -u", "sudo -n true", "sudo -S -p '' -v", "sudo -k -S -p '' bash " + tempFile + kitFlags},
			check: sentOnlyTo("sudo -S -p '' -v", "sudo -k -S -p '' bash "), sysText: "sudo with the password"},
		{name: "requiretty", sudo: sudoRequiretty,
			cmds: []string{"id -u", "sudo -n true", "sudo -S -p '' -v"}, code: CodeNoAdmin, detail: "sudo_tty",
			check: sentOnlyTo("sudo -S -p '' -v")},
		{name: "a wrong sudo password", sudo: sudoPassword, sudoPw: "another password",
			cmds: []string{"id -u", "sudo -n true", "sudo -S -p '' -v"}, code: CodeNoAdmin, detail: "sudo_denied",
			check: sentOnlyTo("sudo -S -p '' -v")},
		{name: "not in sudoers", sudo: sudoDenied, lecture: true,
			cmds: []string{"id -u", "sudo -n true", "sudo -S -p '' -v"}, code: CodeNoAdmin, detail: "sudo_denied",
			check: sentOnlyTo("sudo -S -p '' -v")},
		{name: "no sudo", sudo: sudoMissing,
			cmds: []string{"id -u", "sudo -n true"}, code: CodeNoAdmin, detail: "sudo_missing", check: sentOnlyTo()},
		{name: "a key and sudo that wants a password", sudo: sudoPassword, key: true,
			cmds: []string{"id -u", "sudo -n true"}, code: CodeNoAdmin, detail: "sudo_needs_password", check: sentOnlyTo()},
		{name: "a key and sudo without a password", sudo: sudoNoPasswd, key: true,
			cmds: []string{"id -u", "sudo -n true", "sudo -n bash " + tempFile + kitFlags}, check: sentOnlyTo(), sysText: "sudo"},
	}
	priv, pub := newTestKey(t)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, func(f *fakeServer) {
				f.user = "kit"
				if c.sudo == sudoRoot {
					f.user = "root"
				}
				f.password = focusPassword
				f.sudoPassword = c.sudoPw
				f.sudo = c.sudo
				f.lecture = c.lecture
				f.key = pub
			})
			req := passwordRequest(f, focusPassword)
			if c.key {
				req.Password = nil
				req.Key = keyText(t, priv, "")
			}
			events, done, fail := setupRun(context.Background(), req, testDeps(t, nil))
			var got []string
			for _, cmd := range f.commands() {
				if strings.HasPrefix(cmd, "id ") || strings.HasPrefix(cmd, "sudo ") || strings.HasPrefix(cmd, "bash ") {
					got = append(got, cmd)
				}
			}
			if !reflect.DeepEqual(got, c.cmds) {
				t.Errorf("commands\n%q\nwant\n%q", got, c.cmds)
			}
			if c.code == "" {
				if fail != nil {
					t.Fatalf("%+v", *fail)
				}
				if done.Links == "" {
					t.Fatalf("%+v", done)
				}
				if want := "debian 12, x86_64, " + c.sysText; events[3] != (Event{"system", "done", want}) {
					t.Errorf("%+v, want %q", events[3], want)
				}
			} else {
				wantFailure(t, fail, c.code, c.detail)
				if !reflect.DeepEqual(steps(events), []string{"connect start", "connect done", "system start"}) {
					t.Errorf("%v", steps(events))
				}
			}
			c.check(t, f)
			noSecrets(t, f, events, fail, focusPassword)
		})
	}
}

func TestRunKitResult(t *testing.T) {
	body := testLinkHy2 + "\n"
	if servers, _ := connector.ParseSubscription([]byte(body)); len(servers) == 0 {
		t.Fatal("the test subscription has no server")
	}
	t.Run("with a subscription", func(t *testing.T) {
		f := newFakeServer(t, nil)
		f.setKit(fakeReply{stdout: kitOutput(resultLine(t, []string{testLinkVLESS, testLinkHy2}, testSubURL, nil))})
		fetch := &fakeFetch{answers: []fetchAnswer{{body: body}}}
		events, done, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, fetch))
		if fail != nil {
			t.Fatalf("%+v", *fail)
		}
		if done.Subscription == nil || *done.Subscription != testSubURL || done.Links != testLinkVLESS+"\n"+testLinkHy2 ||
			done.Warnings == nil || len(done.Warnings) != 0 {
			t.Fatalf("%+v", done)
		}
		if got := fetch.attempts(); !reflect.DeepEqual(got, []string{testSubURL}) {
			t.Fatalf("fetched %q", got)
		}
		tail := events[len(events)-2:]
		want := []Event{
			{"subscription", "start", "waiting for the subscription address to answer"},
			{"subscription", "done", "the subscription address answers"},
		}
		if !reflect.DeepEqual(tail, want) {
			t.Fatalf("%+v", tail)
		}
		noSecrets(t, f, events, fail, f.password, testSubURL, "Q2xvc2VkVG9rZW4xMjM0NTY3", testLinkVLESS, testLinkHy2)
		if b, _ := json.Marshal(done); !strings.Contains(string(b), `"subscription":"`+testSubURL+`"`) {
			t.Fatalf("%s", b)
		}
	})
	t.Run("without a subscription", func(t *testing.T) {
		f := newFakeServer(t, nil)
		f.setKit(fakeReply{stdout: kitOutput(resultLine(t, []string{testLinkVLESS}, nil, []string{"sub_port80_busy", "harden_failed"}))})
		fetch := &fakeFetch{}
		events, done, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, fetch))
		if fail != nil {
			t.Fatalf("%+v", *fail)
		}
		if done.Subscription != nil || done.Links != testLinkVLESS || !reflect.DeepEqual(done.Warnings, []string{"sub_port80_busy", "harden_failed"}) {
			t.Fatalf("%+v", done)
		}
		if len(fetch.attempts()) != 0 {
			t.Fatalf("fetched %q", fetch.attempts())
		}
		if last := events[len(events)-1]; last != (Event{"subscription", "skip", "no subscription address"}) {
			t.Fatalf("%+v", last)
		}
		if b, _ := json.Marshal(done); !strings.Contains(string(b), `"subscription":null`) {
			t.Fatalf("%s", b)
		}
	})
	t.Run("no links", func(t *testing.T) {
		f := newFakeServer(t, nil)
		f.setKit(fakeReply{stdout: kitOutput(resultLine(t, []string{}, nil, nil))})
		_, _, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
		wantFailure(t, fail, CodeKitFailed, "the kit gave no share links")
		if got := f.removedFiles(); !reflect.DeepEqual(got, []string{tempFile}) {
			t.Fatalf("removed %q", got)
		}
	})
	t.Run("a result with a failed exit", func(t *testing.T) {
		// The result line is the outcome, whatever the exit status after it.
		f := newFakeServer(t, nil)
		f.setKit(fakeReply{stdout: kitOutput(resultLine(t, []string{testLinkVLESS}, nil, nil)), status: 1})
		_, done, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
		if fail != nil || done.Links != testLinkVLESS {
			t.Fatalf("%+v %+v", done, fail)
		}
	})
	t.Run("the kit lingers after its result", func(t *testing.T) {
		old := kitExitGrace
		kitExitGrace = 100 * time.Millisecond
		defer func() { kitExitGrace = old }()
		f := newFakeServer(t, nil)
		f.setKit(fakeReply{stdout: kitOutput(resultLine(t, []string{testLinkVLESS}, nil, nil)), hang: true})
		start := time.Now()
		_, done, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
		if fail != nil || done.Links != testLinkVLESS {
			t.Fatalf("%+v %+v", done, fail)
		}
		if time.Since(start) > 5*time.Second {
			t.Fatalf("took %v", time.Since(start))
		}
		if got := f.removedFiles(); !reflect.DeepEqual(got, []string{tempFile}) {
			t.Fatalf("removed %q", got)
		}
	})
}

func TestRunKitErrors(t *testing.T) {
	cases := []struct {
		code, message, want, detail string
	}{
		{"os_unsupported", "this system (alpine) is not supported", CodeOSUnsupported, "this system (alpine) is not supported"},
		{"download_failed", "could not download sing-box; nothing was installed", CodeDownloadFailed, "could not download sing-box; nothing was installed"},
		{"ports_busy", "another program already uses 443/tcp", CodePortsBusy, "another program already uses 443/tcp"},
		{"kit_failed", "Caddy refused the site", CodeKitFailed, "Caddy refused the site"},
		{"something_new", "a new failure", CodeKitFailed, "a new failure"},
		{"kit_failed", "bad link " + testLinkHy2 + " at " + testSubURL, CodeKitFailed,
			"bad link hy2://•••@198.51.100.7:443?sni=198-51-100-7.sslip.io#SplitLane%20Hysteria2 at https://198-51-100-7.sslip.io:8443/•••/sub"},
		{"kit_failed", "the password " + focusPassword + " leaked", CodeKitFailed, "the password ••• leaked"},
		{"kit_failed", "line one\nline two\ttabbed " + strings.Repeat("y", 400), CodeKitFailed, "line one line two tabbed " + strings.Repeat("y", 275) + "…"},
	}
	for _, c := range cases {
		t.Run(c.code, func(t *testing.T) {
			f := newFakeServer(t, func(f *fakeServer) { f.password = focusPassword })
			f.setKit(fakeReply{stdout: kitOutput(errorLine(t, c.code, c.message)), status: 1})
			events, _, fail := setupRun(context.Background(), passwordRequest(f, focusPassword), testDeps(t, nil))
			wantFailure(t, fail, c.want, c.detail)
			if got := f.removedFiles(); !reflect.DeepEqual(got, []string{tempFile}) {
				t.Fatalf("removed %q", got)
			}
			noSecrets(t, f, events, fail, focusPassword, "Q2xvc2VkVG9rZW4xMjM0NTY3", "Hy2Pa55w0rdSecret")
		})
	}
}

// TestRunKitNoise (Review Focus 4): lines that are not the kit's JSON are skipped; only a result or
// an error line ends the kit; without one the run fails with kit_failed.
func TestRunKitNoise(t *testing.T) {
	noise := []string{
		"",
		"Reading package lists... Done",
		"debconf: unable to initialize frontend: Dialog",
		"dpkg: warning: while removing caddy, directory '/etc/caddy' not empty so not removed",
		`{"step":"singbox","state":"start","detail":"downlo`,
		`{"unknown":"object"}`,
		`{"step":"reboot","state":"start","detail":"no"}`,
		"   ",
	}
	t.Run("noise around the steps", func(t *testing.T) {
		f := newFakeServer(t, nil)
		var lines []string
		for i, s := range kitSteps {
			lines = append(lines, noise[i%len(noise)], s)
		}
		lines = append(lines, noise...)
		lines = append(lines, resultLine(t, []string{testLinkVLESS}, nil, nil), "trailing noise")
		f.setKit(fakeReply{stdout: strings.Join(lines, "\n") + "\n", stderr: "W: apt noise\n"})
		events, done, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
		if fail != nil {
			t.Fatalf("%+v", *fail)
		}
		if done.Links != testLinkVLESS {
			t.Fatalf("%+v", done)
		}
		if got := events[5 : len(events)-1]; !reflect.DeepEqual(got, kitEvents) {
			t.Fatalf("%+v", got)
		}
	})
	stderr := "E: Unable to fetch some archives\nsee " + testSubURL + "\n"
	ends := []struct {
		name, stdout, detail string
		status               int
	}{
		{"noise only", strings.Join(noise, "\n") + "\n", "E: Unable to fetch some archives | see https://198-51-100-7.sslip.io:8443/•••/sub", 0},
		{"a cut result line", kitOutput(`{"result":{"links":["` + testLinkVLESS + `"],"subscr`), "E: Unable to fetch some archives | see https://198-51-100-7.sslip.io:8443/•••/sub", 1},
		{"no output", "", "E: Unable to fetch some archives | see https://198-51-100-7.sslip.io:8443/•••/sub", 2},
	}
	for _, c := range ends {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, nil)
			f.setKit(fakeReply{stdout: c.stdout, stderr: stderr, status: c.status})
			events, _, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
			wantFailure(t, fail, CodeKitFailed, c.detail)
			if got := f.removedFiles(); !reflect.DeepEqual(got, []string{tempFile}) {
				t.Fatalf("removed %q", got)
			}
			noSecrets(t, f, events, fail, "Q2xvc2VkVG9rZW4xMjM0NTY3", testLinkVLESS)
		})
	}
	t.Run("sudo refuses the kit", func(t *testing.T) {
		// The password worked for sudo -v, then not for the kit: sudo's words are the detail.
		f := newFakeServer(t, func(f *fakeServer) {
			f.user, f.sudo, f.password = "kit", sudoPassword, focusPassword
		})
		f.setReply("sudo -k -S -p '' bash ", fakeReply{stderr: "Sorry, try again.\nsudo: 1 incorrect password attempt\n", status: 1})
		events, _, fail := setupRun(context.Background(), passwordRequest(f, focusPassword), testDeps(t, nil))
		wantFailure(t, fail, CodeKitFailed, "Sorry, try again. | sudo: 1 incorrect password attempt")
		noSecrets(t, f, events, fail, focusPassword)
	})
}

// TestRunConnectionLost (Review Focus 5): the SSH connection breaks after connect.
func TestRunConnectionLost(t *testing.T) {
	cases := []struct {
		name, prefix string
		reply        fakeReply
	}{
		{"in the middle of the kit's output", "", fakeReply{stdout: strings.Join(kitSteps[:5], "\n") + "\n" + `{"step":"config","st`, drop: true}},
		{"during the system check", "uname -m", fakeReply{drop: true}},
		{"during the upload", "cat > ", fakeReply{drop: true}},
		{"during the sudo check", "id -u", fakeReply{stdout: "10", drop: true}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, nil)
			if c.prefix == "" {
				f.setKit(c.reply)
			} else {
				f.setReply(c.prefix, c.reply)
			}
			start := time.Now()
			events, _, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
			wantFailure(t, fail, CodeConnectionLost, "")
			if time.Since(start) > 5*time.Second {
				t.Fatalf("took %v", time.Since(start))
			}
			for _, e := range events {
				if e.Step == "subscription" && e.State != "start" && e.State != "done" {
					t.Fatalf("%+v", events)
				}
			}
		})
	}
}

// TestRunSessionRefused: a session the server refuses on a live connection is not a lost connection.
func TestRunSessionRefused(t *testing.T) {
	cases := []struct {
		name    string
		session int
		removed []string
	}{
		{"during the system check", 2, nil},          // uname -m
		{"during the upload", 6, []string{tempFile}}, // cat >
		{"the kit's session", 8, []string{tempFile}}, // bash
		{"the removal", 9, nil},                      // rm -f: the setup still succeeds
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, func(f *fakeServer) { f.refuse = c.session })
			_, done, fail := setupRun(context.Background(), passwordRequest(f, f.password), testDeps(t, nil))
			if c.session == 9 {
				if fail != nil || done.Links == "" {
					t.Fatalf("%+v %+v", done, fail)
				}
				return
			}
			wantFailure(t, fail, CodeKitFailed, `ssh session: rejected: administratively prohibited ("open failed")`)
			if got := f.removedFiles(); !reflect.DeepEqual(got, c.removed) {
				t.Fatalf("removed %q", got)
			}
		})
	}
}

func TestRunCancel(t *testing.T) {
	t.Run("while the kit runs", func(t *testing.T) {
		f := newFakeServer(t, nil)
		f.setKit(fakeReply{stdout: strings.Join(kitSteps[:3], "\n") + "\n", hang: true})
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var mu sync.Mutex
		var events []Event
		start := time.Now()
		_, fail := Run(ctx, passwordRequest(f, f.password), testDeps(t, nil), func(e Event) {
			mu.Lock()
			events = append(events, e)
			mu.Unlock()
			if e.Step == "install" && strings.Contains(e.Detail, "sing-box") {
				cancel()
			}
		})
		wantFailure(t, fail, CodeCancelled, "")
		if time.Since(start) > 5*time.Second {
			t.Fatalf("took %v", time.Since(start))
		}
		if b, _ := json.Marshal(fail); string(b) != `{"code":"cancelled"}` {
			t.Fatalf("%s", b)
		}
	})
	t.Run("before the start", func(t *testing.T) {
		f := newFakeServer(t, nil)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, _, fail := setupRun(ctx, passwordRequest(f, f.password), testDeps(t, nil))
		wantFailure(t, fail, CodeCancelled, "")
	})
	t.Run("while connecting", func(t *testing.T) {
		port := listen(t, func(c net.Conn) { io.Copy(io.Discard, c) }) // silent until the client goes
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(100*time.Millisecond, cancel)
		req := Request{Host: "127.0.0.1", Port: port, User: "root", Password: []byte("x")}
		_, _, fail := setupRun(ctx, req, testDeps(t, nil))
		wantFailure(t, fail, CodeCancelled, "")
	})
	t.Run("while waiting for the subscription", func(t *testing.T) {
		f := newFakeServer(t, nil)
		f.setKit(fakeReply{stdout: kitOutput(resultLine(t, []string{testLinkVLESS}, testSubURL, nil))})
		d := testDeps(t, nil)
		d.SubWait = 10 * time.Second
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, fail := Run(ctx, passwordRequest(f, f.password), d, func(e Event) {
			if e.Step == "subscription" && strings.HasPrefix(e.Detail, "waiting") {
				time.AfterFunc(50*time.Millisecond, cancel)
			}
		})
		wantFailure(t, fail, CodeCancelled, "")
	})
}

func TestRunTimeout(t *testing.T) {
	f := newFakeServer(t, nil)
	f.setKit(fakeReply{stdout: kitSteps[2] + "\n", hang: true})
	d := testDeps(t, nil)
	d.Total = time.Second
	start := time.Now()
	events, _, fail := setupRun(context.Background(), passwordRequest(f, f.password), d)
	wantFailure(t, fail, CodeTimeout, "")
	if el := time.Since(start); el < time.Second || el > 5*time.Second {
		t.Fatalf("took %v", el)
	}
	if steps(events)[len(events)-1] != "install start" {
		t.Fatalf("%v", steps(events))
	}
}

func TestRunSubscriptionWait(t *testing.T) {
	body := testLinkHy2 + "\n"
	cases := []struct {
		name     string
		answers  []fetchAnswer
		ok       bool
		attempts int // 0: at least two
	}{
		{"the first attempt", []fetchAnswer{{body: body}}, true, 1},
		{"the second attempt", []fetchAnswer{{err: errors.New("fetch_http_status")}, {body: body}}, true, 2},
		{"no answer", nil, false, 0},
		{"an answer without servers", []fetchAnswer{{body: "<html>Caddy</html>"}}, false, 0},
		{"an empty answer", []fetchAnswer{{body: ""}}, false, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFakeServer(t, nil)
			f.setKit(fakeReply{stdout: kitOutput(resultLine(t, []string{testLinkVLESS, testLinkHy2}, testSubURL, []string{"harden_failed"}))})
			fetch := &fakeFetch{answers: c.answers}
			d := testDeps(t, fetch)
			d.SubWait = 200 * time.Millisecond
			d.SubEvery = 20 * time.Millisecond
			start := time.Now()
			events, done, fail := setupRun(context.Background(), passwordRequest(f, f.password), d)
			if fail != nil {
				t.Fatalf("%+v", *fail)
			}
			n := len(fetch.attempts())
			if c.ok {
				if done.Subscription == nil || *done.Subscription != testSubURL || !reflect.DeepEqual(done.Warnings, []string{"harden_failed"}) {
					t.Fatalf("%+v", done)
				}
				if n != c.attempts {
					t.Fatalf("%d attempts", n)
				}
				if last := events[len(events)-1]; last != (Event{"subscription", "done", "the subscription address answers"}) {
					t.Fatalf("%+v", last)
				}
			} else {
				if done.Subscription != nil || !reflect.DeepEqual(done.Warnings, []string{"harden_failed", "sub_unreachable"}) {
					t.Fatalf("%+v", done)
				}
				if done.Links != testLinkVLESS+"\n"+testLinkHy2 {
					t.Fatalf("%+v", done)
				}
				if n < 2 || n > 12 {
					t.Fatalf("%d attempts in 200 ms, every 20 ms", n)
				}
				if el := time.Since(start); el < 200*time.Millisecond {
					t.Fatalf("gave up after %v", el)
				}
				if last := events[len(events)-1]; last != (Event{"subscription", "warn", "the subscription address did not answer"}) {
					t.Fatalf("%+v", last)
				}
			}
			for _, u := range fetch.attempts() {
				if u != testSubURL {
					t.Fatalf("fetched %q", u)
				}
			}
			noSecrets(t, f, events, fail, testSubURL, "Q2xvc2VkVG9rZW4xMjM0NTY3", f.password)
		})
	}
}

func TestDefaultDeps(t *testing.T) {
	d := DefaultDeps("1.2.3")
	if d.Dial == nil || d.Fetch == nil {
		t.Fatal("no Dial or Fetch")
	}
	if d.ConnectTimeout != 15*time.Second || d.Total != 10*time.Minute || d.SubWait != 90*time.Second || d.SubEvery != 5*time.Second {
		t.Fatalf("%+v", d)
	}
}

func TestOutcomeJSON(t *testing.T) {
	sub := "https://h.example.com/x"
	cases := []struct {
		v    any
		want string
	}{
		{Event{"install", "start", "x"}, `{"step":"install","state":"start","detail":"x"}`},
		{Event{"install", "done", ""}, `{"step":"install","state":"done","detail":""}`},
		{Done{Subscription: &sub, Links: "a\nb", Warnings: []string{"sub_unreachable"}}, `{"subscription":"https://h.example.com/x","links":"a\nb","warnings":["sub_unreachable"]}`},
		{Done{Links: "a", Warnings: []string{}}, `{"subscription":null,"links":"a","warnings":[]}`},
		{Failure{Code: "cancelled"}, `{"code":"cancelled"}`},
		{Failure{Code: "kit_failed", Detail: "x"}, `{"code":"kit_failed","detail":"x"}`},
	}
	for _, c := range cases {
		if got := mustJSON(t, c.v); got != c.want {
			t.Errorf("%s, want %s", got, c.want)
		}
	}
}
