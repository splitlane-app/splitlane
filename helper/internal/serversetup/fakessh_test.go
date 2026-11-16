package serversetup

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"io"
	"net"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// The fake server is an SSH server of x/crypto/ssh on 127.0.0.1 that plays a VPS: it answers the
// commands of Run by their prefix from a script, plays sudo, keeps what "cat >" uploads, and records
// every command line, every byte a command got on stdin and every password a client sent, so a test
// can prove where the password went and where it never went.

// tempFile is the name the fake mktemp gives.
const tempFile = "/tmp/splitlane-kit.Ab12Cd"

// kitFlags are the flags SplitLane runs the kit with (plan, Global Constraints).
const kitFlags = " --json --default-protocols vless,hy2 --auto-ports --harden --subscription"

// hostFlag is what Run may add after kitFlags: --host with an IPv4 address.
var hostFlag = regexp.MustCompile(`^ --host \d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}$`)

const osDebian12 = `PRETTY_NAME="Debian GNU/Linux 12 (bookworm)"
NAME="Debian GNU/Linux"
VERSION_ID="12"
VERSION="12 (bookworm)"
VERSION_CODENAME=bookworm
ID=debian
HOME_URL="https://www.debian.org/"
`

// How sudo is set up for the user of the fake server.
const (
	sudoRoot       = iota // the user is root: no sudo needed
	sudoNoPasswd          // sudo without a password (NOPASSWD)
	sudoPassword          // sudo with the user's password
	sudoRequiretty        // sudo only from a terminal (Defaults requiretty)
	sudoMissing           // no sudo on the server
	sudoDenied            // sudo is there, the user is not in sudoers
)

const sudoLecture = "\nWe trust you have received the usual lecture from the local System\nAdministrator. It usually boils down to these three things:\n\n    #1) Respect the privacy of others.\n    #2) Think before you type.\n    #3) With great power comes great responsibility.\n\n"

// fakeReply is what a command prints and how it ends.
type fakeReply struct {
	stdout, stderr string
	status         int
	drop           bool                                     // after the output the connection breaks, with no exit status
	hang           bool                                     // after the output the command never ends, until the client goes away
	fn             func(cmd string, stdin []byte) fakeReply // the reply worked out from the command and its stdin
}

type execRecord struct {
	cmd   string
	stdin []byte
}

type kbdRound struct {
	instruction string
	questions   []string
	echos       []bool
}

type fakeServer struct {
	t    *testing.T
	ln   net.Listener
	port int

	// Set before start.
	user         string
	password     string     // the login password ("" refuses every password)
	sudoPassword string     // the password sudo wants; "" means password
	passwordAuth bool       // the password method is offered
	kbdAuth      bool       // keyboard-interactive is offered
	kbdRounds    []kbdRound // nil: one round, "Password: " without echo
	key          ssh.PublicKey
	sudo         int
	lecture      bool   // sudo prints its lecture on stderr
	refuse       int    // the session with this number (1 is the first) is refused, as sshd's MaxSessions does; 0: none
	greeting     string // printed before the stdout of every command, as by a ~/.bashrc that greets

	mu        sync.Mutex
	replies   map[string]fakeReply
	kit       fakeReply
	execs     []execRecord
	passwords []string // what clients sent as secrets: passwords of the password method, every keyboard-interactive answer
	files     map[string][]byte
	removed   []string
	conns     []net.Conn
	sudoTold  bool // the lecture was printed
	sessions  int  // sessions opened so far
	wg        sync.WaitGroup
}

// newFakeServer starts a fake Debian 12 x86_64 server with systemd where root logs in with password
// "Pa55-w0rd" (by the password method or keyboard-interactive) and the kit succeeds without a
// subscription. configure changes the setup before the server starts.
func newFakeServer(t *testing.T, configure func(f *fakeServer)) *fakeServer {
	t.Helper()
	f := &fakeServer{
		t:            t,
		user:         "root",
		password:     "Pa55-w0rd",
		passwordAuth: true,
		kbdAuth:      true,
		sudo:         sudoRoot,
		files:        map[string][]byte{},
		kit:          fakeReply{stdout: kitOutput(resultLine(t, []string{testLinkVLESS, testLinkHy2}, nil, nil))},
	}
	f.replies = map[string]fakeReply{
		"cat /etc/os-release":              {stdout: osDebian12},
		"uname -m":                         {stdout: "x86_64\n"},
		"test -d /run/systemd/system":      {},
		"id -u":                            {fn: f.idU},
		"sudo -n true":                     {fn: f.sudoN},
		"sudo -S -p '' -v":                 {fn: f.sudoV},
		"mktemp /tmp/splitlane-kit.XXXXXX": {stdout: tempFile + "\n"},
		"cat > ":                           {fn: f.upload},
		"chmod 600 ":                       {fn: f.chmod},
		"rm -f ":                           {fn: f.remove},
		"bash ":                            {fn: f.runKit},
		"sudo -n bash ":                    {fn: f.runKit},
		"sudo -k -S -p '' bash ":           {fn: f.runKit},
	}
	if configure != nil {
		configure(f)
	}
	if f.sudoPassword == "" {
		f.sudoPassword = f.password
	}
	f.start()
	return f
}

func (f *fakeServer) start() {
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		f.t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(hostKey)
	if err != nil {
		f.t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{}
	cfg.AddHostKey(signer)
	if f.passwordAuth {
		cfg.PasswordCallback = func(c ssh.ConnMetadata, pw []byte) (*ssh.Permissions, error) {
			f.mu.Lock()
			f.passwords = append(f.passwords, string(pw))
			f.mu.Unlock()
			if c.User() == f.user && f.password != "" && string(pw) == f.password {
				return nil, nil
			}
			return nil, errors.New("denied")
		}
	}
	if f.kbdAuth {
		cfg.KeyboardInteractiveCallback = func(c ssh.ConnMetadata, client ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
			rounds := f.kbdRounds
			if rounds == nil {
				rounds = []kbdRound{{questions: []string{"Password: "}, echos: []bool{false}}}
			}
			ok := false
			for _, r := range rounds {
				answers, err := client(c.User(), r.instruction, r.questions, r.echos)
				if err != nil {
					return nil, err
				}
				for i, a := range answers {
					f.mu.Lock()
					f.passwords = append(f.passwords, a) // an answer to a question shown as typed too
					f.mu.Unlock()
					ok = ok || !r.echos[i] && f.password != "" && a == f.password
				}
			}
			if ok && c.User() == f.user {
				return nil, nil
			}
			return nil, errors.New("denied")
		}
	}
	if f.key != nil {
		cfg.PublicKeyCallback = func(c ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if c.User() == f.user && string(key.Marshal()) == string(f.key.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("denied")
		}
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		f.t.Fatal(err)
	}
	f.ln = ln
	f.port = ln.Addr().(*net.TCPAddr).Port
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			f.conns = append(f.conns, c)
			f.mu.Unlock()
			f.wg.Add(1)
			go func() {
				defer f.wg.Done()
				f.serve(c, cfg)
			}()
		}
	}()
	f.t.Cleanup(func() {
		ln.Close()
		f.mu.Lock()
		for _, c := range f.conns {
			c.Close()
		}
		f.mu.Unlock()
		f.wg.Wait()
	})
}

func (f *fakeServer) serve(c net.Conn, cfg *ssh.ServerConfig) {
	sc, chans, reqs, err := ssh.NewServerConn(c, cfg)
	if err != nil {
		c.Close()
		return
	}
	gone := make(chan struct{})
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		sc.Wait()
		close(gone)
	}()
	f.wg.Add(1)
	go func() {
		defer f.wg.Done()
		ssh.DiscardRequests(reqs) // keepalives get an answer (false), as from OpenSSH
	}()
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "")
			continue
		}
		f.mu.Lock()
		f.sessions++
		refused := f.sessions == f.refuse
		f.mu.Unlock()
		if refused {
			nc.Reject(ssh.Prohibited, "open failed")
			continue
		}
		f.wg.Add(1)
		go func() {
			defer f.wg.Done()
			f.session(c, gone, nc)
		}()
	}
}

func (f *fakeServer) session(c net.Conn, gone <-chan struct{}, nc ssh.NewChannel) {
	ch, reqs, err := nc.Accept()
	if err != nil {
		return
	}
	defer ch.Close()
	started := false
	for req := range reqs {
		var p struct{ Command string }
		ok := req.Type == "exec" && !started && ssh.Unmarshal(req.Payload, &p) == nil
		if req.Type != "exec" {
			f.t.Errorf("a %q request", req.Type) // Run asks for no terminal, no environment
		}
		if req.WantReply {
			req.Reply(ok, nil)
		}
		if ok {
			started = true
			f.wg.Add(1)
			go func() {
				defer f.wg.Done()
				f.exec(c, gone, ch, p.Command)
			}()
		}
	}
}

func (f *fakeServer) exec(c net.Conn, gone <-chan struct{}, ch ssh.Channel, cmd string) {
	stdin, _ := io.ReadAll(ch) // until the client closes its stdin
	f.mu.Lock()
	f.execs = append(f.execs, execRecord{cmd, stdin})
	f.mu.Unlock()
	r := f.reply(cmd, stdin)
	io.WriteString(ch.Stderr(), r.stderr)
	io.WriteString(ch, f.greeting+r.stdout)
	switch {
	case r.drop:
		time.Sleep(20 * time.Millisecond) // the output goes out first
		c.Close()
		return
	case r.hang:
		<-gone
		return
	}
	ch.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{uint32(r.status)}))
	ch.Close()
}

// reply finds the reply of the longest prefix of cmd. A command nobody expects fails the test.
func (f *fakeServer) reply(cmd string, stdin []byte) fakeReply {
	f.mu.Lock()
	best, found := "", false
	for p := range f.replies {
		if strings.HasPrefix(cmd, p) && len(p) >= len(best) {
			best, found = p, true
		}
	}
	r := f.replies[best]
	f.mu.Unlock()
	if !found {
		f.t.Errorf("an unexpected command %q", cmd)
		return fakeReply{stderr: "bash: command not found\n", status: 127}
	}
	for r.fn != nil {
		r = r.fn(cmd, stdin)
	}
	return r
}

// setReply sets the reply of the commands that start with prefix.
func (f *fakeServer) setReply(prefix string, r fakeReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[prefix] = r
}

// setKit sets what the kit does once it runs with the right privileges.
func (f *fakeServer) setKit(r fakeReply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.kit = r
}

func (f *fakeServer) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, e := range f.execs {
		out = append(out, e.cmd)
	}
	return out
}

func (f *fakeServer) records() []execRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]execRecord(nil), f.execs...)
}

// stdinOf is the stdin of the first command that starts with prefix.
func (f *fakeServer) stdinOf(prefix string) ([]byte, bool) {
	for _, e := range f.records() {
		if strings.HasPrefix(e.cmd, prefix) {
			return e.stdin, true
		}
	}
	return nil, false
}

func (f *fakeServer) sentPasswords() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.passwords...)
}

func (f *fakeServer) file(name string) ([]byte, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.files[name]
	return b, ok
}

func (f *fakeServer) removedFiles() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.removed...)
}

// ---- The server's commands ----

func (f *fakeServer) idU(string, []byte) fakeReply {
	if f.sudo == sudoRoot {
		return fakeReply{stdout: "0\n"}
	}
	return fakeReply{stdout: "1000\n"}
}

func (f *fakeServer) sudoN(string, []byte) fakeReply {
	switch f.sudo {
	case sudoRoot, sudoNoPasswd:
		return fakeReply{}
	case sudoRequiretty:
		return fakeReply{stderr: "sudo: sorry, you must have a tty to run sudo\n", status: 1}
	case sudoMissing:
		return fakeReply{stderr: "bash: line 1: sudo: command not found\n", status: 127}
	}
	return fakeReply{stderr: "sudo: a password is required\n", status: 1}
}

// sudoS plays sudo -S: the password is the first line of stdin. It returns the failure or nil.
func (f *fakeServer) sudoS(stdin []byte) *fakeReply {
	switch f.sudo {
	case sudoRoot, sudoNoPasswd:
		return nil
	case sudoRequiretty:
		return &fakeReply{stderr: "sudo: sorry, you must have a tty to run sudo\n", status: 1}
	case sudoMissing:
		return &fakeReply{stderr: "bash: line 1: sudo: command not found\n", status: 127}
	}
	f.mu.Lock()
	lecture := f.lecture && !f.sudoTold
	f.sudoTold = f.sudoTold || lecture
	f.mu.Unlock()
	pre := ""
	if lecture {
		pre = sudoLecture
	}
	if string(stdin) != f.sudoPassword+"\n" {
		return &fakeReply{stderr: pre + "Sorry, try again.\nsudo: no password was provided\nsudo: 1 incorrect password attempt\n", status: 1}
	}
	if f.sudo == sudoDenied {
		return &fakeReply{stderr: pre + f.user + " is not in the sudoers file.\n", status: 1}
	}
	if lecture {
		return &fakeReply{stderr: pre} // status 0: the lecture is not a failure
	}
	return nil
}

func (f *fakeServer) sudoV(_ string, stdin []byte) fakeReply {
	if r := f.sudoS(stdin); r != nil {
		return *r
	}
	return fakeReply{}
}

func (f *fakeServer) upload(cmd string, stdin []byte) fakeReply {
	name := strings.TrimPrefix(cmd, "cat > ")
	if name != tempFile {
		f.t.Errorf("upload to %q", name)
	}
	f.mu.Lock()
	f.files[name] = append([]byte(nil), stdin...)
	f.mu.Unlock()
	return fakeReply{}
}

func (f *fakeServer) chmod(cmd string, _ []byte) fakeReply {
	if cmd != "chmod 600 "+tempFile {
		f.t.Errorf("chmod: %q", cmd)
	}
	return fakeReply{}
}

func (f *fakeServer) remove(cmd string, _ []byte) fakeReply {
	f.mu.Lock()
	f.removed = append(f.removed, strings.TrimPrefix(cmd, "rm -f "))
	f.mu.Unlock()
	return fakeReply{}
}

// runKit plays sudo in front of the kit, then the kit.
func (f *fakeServer) runKit(cmd string, stdin []byte) fakeReply {
	prefix, rest, _ := strings.Cut(cmd, "bash ")
	// the flags, and --host with a public IPv4 address typed for the server (TestRunKitHost)
	if host, ok := strings.CutPrefix(rest, tempFile+kitFlags); !ok || host != "" && !hostFlag.MatchString(host) {
		f.t.Errorf("the kit runs as %q", cmd)
	}
	switch prefix {
	case "":
		if f.sudo != sudoRoot {
			return fakeReply{stdout: `{"error":{"code":"kit_failed","message":"run it as root: sudo bash ` + tempFile + `"}}` + "\n", status: 1}
		}
	case "sudo -n ":
		if f.sudo != sudoRoot && f.sudo != sudoNoPasswd {
			return fakeReply{stderr: "sudo: a password is required\n", status: 1}
		}
	case "sudo -k -S -p '' ":
		if r := f.sudoS(stdin); r != nil && r.status != 0 {
			return *r
		} else if r != nil {
			k := f.kitReply()
			k.stderr = r.stderr + k.stderr
			return k
		}
	default:
		f.t.Errorf("the kit runs as %q", cmd)
	}
	return f.kitReply()
}

func (f *fakeServer) kitReply() fakeReply {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.kit
}

// ---- The kit's output ----

const (
	testLinkVLESS = "vless://11111111-2222-4333-8444-555555555555@198.51.100.7:443?security=reality&pbk=SbVKOEMjK0sIlbwg4akyBg5mL5KZwwB-ed4eEE7YnRc&sid=6ba85179e30d4fc2&sni=www.example.com&fp=chrome&flow=xtls-rprx-vision&type=tcp#SplitLane%20VLESS"
	testLinkHy2   = "hy2://Hy2Pa55w0rdSecret@198.51.100.7:443?sni=198-51-100-7.sslip.io#SplitLane%20Hysteria2"
	testSubURL    = "https://198-51-100-7.sslip.io:8443/Q2xvc2VkVG9rZW4xMjM0NTY3/sub"
)

// kitSteps are the step lines of a successful kit run.
var kitSteps = []string{
	`{"step":"system","state":"start","detail":"checking the system"}`,
	`{"step":"system","state":"done","detail":"debian 12, amd64; ports 443/tcp 443/udp 8443/tcp"}`,
	`{"step":"singbox","state":"start","detail":"downloading sing-box 1.14.4 (amd64)"}`,
	`{"step":"singbox","state":"done","detail":"installed sing-box 1.14.4"}`,
	`{"step":"config","state":"start","detail":"keys, the server config, the links"}`,
	`{"step":"config","state":"done","detail":"ways in: vless, hy2"}`,
	`{"step":"service","state":"start","detail":"the sing-box service"}`,
	`{"step":"service","state":"done","detail":"the sing-box service is running"}`,
	`{"step":"harden","state":"start","detail":"fail2ban for SSH, automatic security updates"}`,
	`{"step":"harden","state":"done","detail":"fail2ban guards SSH; security updates are on"}`,
	`{"step":"subscription","state":"start","detail":"the subscription over HTTPS (Caddy)"}`,
	`{"step":"subscription","state":"done","detail":"Caddy serves it on port 8443"}`,
}

// kitEvents are the events Run makes of kitSteps.
var kitEvents = []Event{
	{"install", "start", "downloading sing-box 1.14.4 (amd64)"},
	{"install", "done", "installed sing-box 1.14.4"},
	{"install", "start", "keys, the server config, the links"},
	{"install", "done", "ways in: vless, hy2"},
	{"install", "start", "the sing-box service"},
	{"install", "done", "the sing-box service is running"},
	{"harden", "start", "fail2ban for SSH, automatic security updates"},
	{"harden", "done", "fail2ban guards SSH; security updates are on"},
	{"subscription", "start", "the subscription over HTTPS (Caddy)"},
	{"subscription", "done", "Caddy serves it on port 8443"},
}

// kitOutput is the stdout of a kit run: the steps, then last.
func kitOutput(last ...string) string {
	return strings.Join(append(append([]string(nil), kitSteps...), last...), "\n") + "\n"
}

// resultLine is the result line of the kit. sub is the subscription address or nil.
func resultLine(t *testing.T, links []string, sub any, warnings []string) string {
	t.Helper()
	if warnings == nil {
		warnings = []string{}
	}
	ports := map[string]int{"vless": 443, "hy2": 443}
	if sub != nil {
		ports["sub"] = 8443
	}
	return mustJSON(t, map[string]any{"result": map[string]any{
		"links": links, "wireguard": nil, "subscription": sub, "ports": ports, "warnings": warnings,
	}})
}

func errorLine(t *testing.T, code, message string) string {
	t.Helper()
	return mustJSON(t, map[string]any{"error": map[string]any{"code": code, "message": message}})
}

// ---- Fakes of Deps ----

// loopbackDial connects only to 127.0.0.1: a test never reaches out of the machine.
func loopbackDial(t *testing.T) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil || host != "127.0.0.1" || network != "tcp" {
			t.Errorf("a connection to %s %s", network, addr)
			return nil, errors.New("not in a test")
		}
		var d net.Dialer
		return d.DialContext(ctx, network, addr)
	}
}

type fetchAnswer struct {
	body string
	err  error
}

// fakeFetch answers the downloads of the subscription: answers[i] is the answer to attempt i+1, the
// last one repeats; no answers: every attempt fails.
type fakeFetch struct {
	mu      sync.Mutex
	answers []fetchAnswer
	urls    []string
}

func (f *fakeFetch) Get(ctx context.Context, url string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(f.answers) == 0 {
		return nil, errors.New("fetch_failed")
	}
	a := f.answers[min(len(f.urls), len(f.answers))-1]
	if a.err != nil {
		return nil, a.err
	}
	return []byte(a.body), nil
}

func (f *fakeFetch) attempts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.urls...)
}

func testDeps(t *testing.T, fetch *fakeFetch) Deps {
	if fetch == nil {
		fetch = &fakeFetch{}
	}
	return Deps{
		Dial:           loopbackDial(t),
		Fetch:          fetch,
		ConnectTimeout: 5 * time.Second,
		Total:          20 * time.Second,
		SubWait:        300 * time.Millisecond,
		SubEvery:       10 * time.Millisecond,
	}
}
