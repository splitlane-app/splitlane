// Package serversetup sets up the user's own VPS over SSH (spec of stage 3 part 2, sections 3, 5
// and 8): it logs in with a password or a key, checks the system and the user's rights, uploads the
// server kit built into the helper, runs it, translates its JSON lines into steps for the window and
// waits until the subscription address answers. The server-setup subcommand runs it for the window,
// without elevation, with the credentials from its stdin.
package serversetup

import (
	"bytes"
	"context"
	"errors"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"splitlane/helper/internal/connector"
)

// Codes of a failed setup (spec, section 8).
const (
	CodeBadInput        = "bad_input"        // the request does not pass the checks
	CodeSSHUnreachable  = "ssh_unreachable"  // no TCP or SSH connection within ConnectTimeout, refused, a name that does not resolve
	CodeSSHAuth         = "ssh_auth"         // the server refused the login
	CodeSSHKey          = "ssh_key"          // the key cannot be used: detail ppk, passphrase or format
	CodePasswordExpired = "password_expired" // the login works, but the account must change its password first (no detail)
	CodeNoAdmin         = "no_admin"         // not root and no usable sudo: detail sudo_missing, sudo_tty, sudo_denied or sudo_needs_password
	CodeOSUnsupported   = "os_unsupported"   // not Debian 12+ or Ubuntu 22.04+, not x86_64 or aarch64, no systemd
	CodeDownloadFailed  = "download_failed"  // the kit could not download sing-box or packages
	CodePortsBusy       = "ports_busy"       // every port the kit tries for a protocol is taken
	CodeKitFailed       = "kit_failed"       // any other failure of the kit, or of SSH on a live connection
	CodeTimeout         = "timeout"          // the work over SSH took longer than Deps.Total
	CodeCancelled       = "cancelled"        // ctx was cancelled
	CodeConnectionLost  = "connection_lost"  // the SSH connection broke after the login
)

// WarnSubUnreachable: the subscription address did not answer within Deps.SubWait. The kit's own
// warnings (harden_failed, sub_port80_busy, sub_skipped) come before it in Done.Warnings.
const WarnSubUnreachable = "sub_unreachable"

// Event is a step of the setup for the window. Step is connect, system, install, harden or
// subscription; State is start, done, skip or warn; Detail is a short line without secrets.
type Event struct {
	Step   string `json:"step"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

// Done is a finished setup: the subscription address when it answered, the share links one per line
// (always some) and the warnings (never nil).
type Done struct {
	Subscription *string  `json:"subscription"`
	Links        string   `json:"links"`
	Warnings     []string `json:"warnings"`
}

// Failure is a failed setup. Detail is masked with connector.Mask and carries neither the password
// nor the passphrase.
type Failure struct {
	Code   string `json:"code"`
	Detail string `json:"detail,omitempty"`
}

// Deps are what Run uses from outside; tests replace them.
type Deps struct {
	Dial           func(ctx context.Context, network, addr string) (net.Conn, error)
	Fetch          connector.Fetcher // downloads the subscription
	ConnectTimeout time.Duration     // for the TCP connection, then again for the SSH handshake with the login
	Total          time.Duration     // for the whole setup; the subscription wait gets what is left of it
	SubWait        time.Duration     // how long the subscription address may take to answer
	SubEvery       time.Duration     // how often it is tried
}

// DefaultDeps are the Deps of the helper (spec, section 5). version is the helper's, for the
// User-Agent of the subscription download.
func DefaultDeps(version string) Deps {
	// net.Dialer turns TCP keep-alive on (every 15 s): a connection that dies silently while the kit
	// runs ends as connection_lost within minutes, not as a timeout at the end of Total.
	var dialer net.Dialer
	return Deps{
		Dial:           dialer.DialContext,
		Fetch:          connector.NewFetcher(version),
		ConnectTimeout: 15 * time.Second,
		Total:          10 * time.Minute,
		SubWait:        90 * time.Second,
		SubEvery:       5 * time.Second,
	}
}

// kitArgs are the flags SplitLane runs the kit with (plan, Global Constraints).
const kitArgs = "--json --default-protocols vless,hy2 --auto-ports --harden --subscription"

// tempName is what mktemp may answer. The name goes into command lines unquoted, so nothing else is
// accepted.
var tempName = regexp.MustCompile(`^/tmp/splitlane-kit\.[A-Za-z0-9]{6}$`)

// masked replaces a secret in a detail, as connector.Mask does.
const masked = "•••"

// maxDetail is the length of a detail in characters; a longer one is cut with "…".
const maxDetail = 300

// Run sets up the server of req (spec, section 5). It emits the steps through emit, one call at a
// time, and returns Done or a Failure. Everything runs within d.Total: the work over SSH that does
// not finish in it is a timeout; the subscription wait ends with it too, but then the kit has
// succeeded and its links are kept (Done with sub_unreachable, Ruling 13). A cancelled ctx is
// cancelled; an SSH connection that breaks after the login is connection_lost. The credentials go
// to the server only as SSH authentication and as the stdin of sudo -S, never in a command line;
// they and the secrets of the result (links, subscription address) are kept out of every Event and
// Failure. Run leaves req to the caller to wipe.
func Run(ctx context.Context, req Request, d Deps, emit func(Event)) (Done, *Failure) {
	runCtx, cancel := context.WithTimeout(ctx, d.Total)
	defer cancel()
	s := &setup{ctx: runCtx, caller: ctx, req: req, d: d, emitFn: emit}
	done, f := s.run()
	if f != nil {
		f.Detail = s.clean(f.Detail)
		return Done{}, f
	}
	return done, nil
}

type setup struct {
	ctx    context.Context // the caller's, within Total
	caller context.Context // the caller's alone: when it ends the run fails, even after the kit's success
	req    Request
	d      Deps
	emitFn func(Event)
	conn   *sshConn
}

// How the kit gets root.
type admin int

const (
	adminRoot         admin = iota // the user is root
	adminSudo                      // sudo without a password (NOPASSWD)
	adminSudoPassword              // sudo with the login password on its stdin
)

func (a admin) String() string {
	switch a {
	case adminSudo:
		return "sudo"
	case adminSudoPassword:
		return "sudo with the password"
	}
	return "root"
}

func (s *setup) run() (Done, *Failure) {
	addr := net.JoinHostPort(s.req.Host, strconv.Itoa(s.req.Port))
	s.emit("connect", "start", addr)
	conn, f := connect(s.ctx, s.req, s.d, addr)
	if f != nil {
		return Done{}, f
	}
	s.conn = conn
	defer conn.close()
	s.emit("connect", "done", "logged in as "+s.req.User)

	s.emit("system", "start", "checking the server")
	system, a, f := s.checkSystem()
	if f != nil {
		return Done{}, f
	}
	s.emit("system", "done", system+", "+a.String())

	s.emit("install", "start", "uploading the server kit")
	res, f := s.install(a)
	conn.close() // the server's part is over; the subscription is fetched from here
	if f != nil {
		return Done{}, f
	}
	return s.finish(res)
}

// checkSystem checks the system the kit needs (the kit checks it again, but a refusal here comes
// before anything is uploaded) and how the kit can get root. It returns the system as text.
func (s *setup) checkSystem() (string, admin, *Failure) {
	r, f := s.cmd("cat /etc/os-release", nil)
	if f != nil {
		return "", 0, f
	}
	if r.status != 0 {
		// The first command tells an account that must change its password (the login works, every
		// command fails) from a system that does not fit (Ruling 12).
		if passwordExpired(r.stderr) {
			return "", 0, &Failure{Code: CodePasswordExpired}
		}
		detail := "os-release"
		if t := s.stderrTail(r.stderr); t != "" {
			detail += ": " + t
		}
		return "", 0, unsupported(detail)
	}
	id, version := osRelease(string(r.stdout))
	if !supportedOS(id, version) {
		return "", 0, unsupported(strings.TrimSpace(clip(id, 64) + " " + clip(version, 64)))
	}
	if r, f = s.cmd("uname -m", nil); f != nil {
		return "", 0, f
	}
	arch := lastLine(r.stdout)
	if r.status != 0 || arch != "x86_64" && arch != "aarch64" {
		if arch == "" {
			arch = "uname"
		}
		return "", 0, unsupported(clip(arch, 64))
	}
	if r, f = s.cmd("test -d /run/systemd/system", nil); f != nil {
		return "", 0, f
	}
	if r.status != 0 {
		return "", 0, unsupported("systemd")
	}
	a, f := s.checkAdmin()
	return id + " " + version + ", " + arch, a, f
}

// checkAdmin finds how the kit can run as root: as the user, sudo without a password, or sudo with
// the login password. The password is sent only to sudo -S, and only when sudo -n says one is needed.
func (s *setup) checkAdmin() (admin, *Failure) {
	r, f := s.cmd("id -u", nil)
	if f != nil {
		return 0, f
	}
	if r.status == 0 && lastLine(r.stdout) == "0" {
		return adminRoot, nil
	}
	if r, f = s.cmd("sudo -n true", nil); f != nil {
		return 0, f
	}
	switch {
	case r.status == 0:
		return adminSudo, nil
	case r.status == 127: // the shell's "command not found"
		return 0, &Failure{Code: CodeNoAdmin, Detail: "sudo_missing"}
	case len(s.req.Password) == 0: // a key login has no password to give sudo
		return 0, &Failure{Code: CodeNoAdmin, Detail: "sudo_needs_password"}
	}
	// -S: the password from stdin; -p '': no prompt in the output. The lecture of a first sudo goes to
	// stderr and does not change the exit status.
	stdin := passwordLine(s.req.Password)
	defer clear(stdin)
	if r, f = s.cmd("sudo -S -p '' -v", stdin); f != nil {
		return 0, f
	}
	switch {
	case r.status == 0:
		return adminSudoPassword, nil
	case bytes.Contains(r.stderr, []byte("tty")) || bytes.Contains(r.stderr, []byte("terminal")):
		return 0, &Failure{Code: CodeNoAdmin, Detail: "sudo_tty"} // Defaults requiretty
	}
	return 0, &Failure{Code: CodeNoAdmin, Detail: "sudo_denied"}
}

// install uploads the kit to a temporary file, runs it and removes the file, also after a failure
// (not after a cancel, a timeout or a broken connection: then the connection is gone; the file is the
// public kit, mode 600, in /tmp).
func (s *setup) install(a admin) (*kitResult, *Failure) {
	r, f := s.cmd("mktemp /tmp/splitlane-kit.XXXXXX", nil)
	if f != nil {
		return nil, f
	}
	name := lastLine(r.stdout)
	if r.status != 0 || !tempName.MatchString(name) {
		return nil, kitFailed("mktemp: " + s.stderrTail(r.stderr))
	}
	defer func() {
		_, _ = s.conn.run("rm -f "+name, nil, nil) // a failure to remove changes nothing for the user
	}()
	if r, f = s.cmd("cat > "+name, Kit); f != nil {
		return nil, f
	}
	if r.status != 0 || r.stdinErr != nil {
		return nil, kitFailed("upload: " + s.stderrTail(r.stderr))
	}
	if r, f = s.cmd("chmod 600 "+name, nil); f != nil {
		return nil, f
	}
	if r.status != 0 {
		return nil, kitFailed("chmod: " + s.stderrTail(r.stderr))
	}
	return s.runKit(a, name)
}

// runKit runs the kit and reads its lines: steps go to the window, the result or the error line ends
// it. The kit runs from a file, so bash takes its script from there and stdin is only sudo's.
func (s *setup) runKit(a admin, name string) (*kitResult, *Failure) {
	cmd := "bash " + name + " " + kitArgs
	var stdin []byte
	switch a {
	case adminSudo:
		cmd = "sudo -n " + cmd // -n: never wait for a password
	case adminSudoPassword:
		// The password and a newline are the first bytes of stdin, then stdin closes. -k makes sudo
		// read them even though sudo -v has just cached the credentials: sshd starts every command of
		// a connection from the same process, so sudo's ticket (kept per parent process without a
		// terminal) would apply, sudo would read nothing, and the password would stay on the kit's
		// stdin for any program of the kit to read.
		cmd = "sudo -k -S -p '' " + cmd
		stdin = passwordLine(s.req.Password)
		defer clear(stdin)
	}
	var res *kitResult
	var fail *Failure
	r, err := s.conn.run(cmd, stdin, func(line []byte) bool {
		if res == nil && fail == nil {
			l := parseKitLine(line)
			switch {
			case l.event != nil:
				s.emit(l.event.Step, l.event.State, l.event.Detail)
			case l.result != nil:
				res = l.result
			case l.fail != nil:
				fail = l.fail
			}
		}
		return res != nil || fail != nil
	})
	// The first result or error line is the outcome, whatever came after it.
	switch {
	case res != nil:
		return res, nil
	case fail != nil:
		return nil, fail
	case err != nil:
		return nil, s.sshFailure(err)
	}
	if f := ctxFailure(s.ctx); f != nil {
		return nil, f
	}
	detail := s.stderrTail(r.stderr)
	if detail == "" {
		detail = "the kit ended without a result (exit code " + strconv.Itoa(r.status) + ")"
	}
	return nil, kitFailed(detail)
}

// finish makes Done of the kit's result and waits for the subscription address to answer.
func (s *setup) finish(res *kitResult) (Done, *Failure) {
	if len(res.Links) == 0 {
		return Done{}, kitFailed("the kit gave no share links")
	}
	done := Done{Links: strings.Join(res.Links, "\n"), Warnings: append([]string{}, res.Warnings...)}
	if res.Subscription == "" {
		s.emit("subscription", "skip", "no subscription address")
		return done, nil
	}
	s.emit("subscription", "start", "waiting for the subscription address to answer")
	if s.waitSubscription(res.Subscription) {
		sub := res.Subscription
		done.Subscription = &sub
		s.emit("subscription", "done", "the subscription address answers")
		return done, nil
	}
	// The wait ends at SubWait or with Total, whichever comes first. The kit has succeeded, so the end
	// of Total costs the subscription only (Ruling 13); a cancel still fails the run.
	if f := ctxFailure(s.caller); f != nil {
		return Done{}, f
	}
	done.Warnings = append(done.Warnings, WarnSubUnreachable)
	s.emit("subscription", "warn", "the subscription address did not answer")
	return done, nil
}

// waitSubscription tries the subscription address every SubEvery, for SubWait at most and never past
// Total, until it answers with at least one server: Caddy needs a while for its certificate. The
// download has the rules of the service's (connector.Fetcher: https only, public addresses, limits).
func (s *setup) waitSubscription(url string) bool {
	ctx, cancel := context.WithTimeout(s.ctx, s.d.SubWait)
	defer cancel()
	for {
		if body, err := s.d.Fetch.Get(ctx, url); err == nil {
			if servers, _ := connector.ParseSubscription(body); len(servers) > 0 {
				return true
			}
		}
		t := time.NewTimer(s.d.SubEvery)
		select {
		case <-ctx.Done():
			t.Stop()
			return false
		case <-t.C:
		}
	}
}

// cmd runs a command of the setup; an SSH failure becomes the Failure of the run.
func (s *setup) cmd(cmd string, stdin []byte) (cmdResult, *Failure) {
	r, err := s.conn.run(cmd, stdin, nil)
	if err != nil {
		return r, s.sshFailure(err)
	}
	return r, nil
}

// sshFailure tells why SSH failed: the context, a connection that is gone, or else the server (a
// session refused on a live connection).
func (s *setup) sshFailure(err error) *Failure {
	if f := ctxFailure(s.ctx); f != nil {
		return f
	}
	alive := s.conn.alive()
	if f := ctxFailure(s.ctx); f != nil {
		return f
	}
	if !alive {
		return &Failure{Code: CodeConnectionLost}
	}
	return kitFailed("ssh session: " + strings.TrimPrefix(err.Error(), "ssh: "))
}

func (s *setup) emit(step, state, detail string) {
	if s.emitFn != nil {
		s.emitFn(Event{Step: step, State: state, Detail: s.clean(detail)})
	}
}

// clean makes a detail safe and short: the password and the passphrase out, connector.Mask, one line
// of printable text, maxDetail characters. The password goes out first, before Mask or the cut can
// leave a piece of it.
func (s *setup) clean(text string) string {
	text = strings.ToValidUTF8(string(s.scrub([]byte(text))), string(utf8.RuneError))
	return clip(oneLine(connector.Mask(text)), maxDetail)
}

// scrub replaces the password and the passphrase in b with "•••". They are compared as bytes: a
// string of one could not be wiped.
func (s *setup) scrub(b []byte) []byte {
	for _, secret := range [][]byte{s.req.Password, s.req.Passphrase} {
		if len(secret) > 0 {
			b = bytes.ReplaceAll(b, secret, []byte(masked))
		}
	}
	return b
}

// stderrTail is the last two lines of a command's stderr for a detail. The secrets go out of the raw
// text first: lastLines makes each line one line of text, and a password with a tab or two spaces in
// a row would no longer match afterwards.
func (s *setup) stderrTail(stderr []byte) string {
	return lastLines(s.scrub(stderr), 2)
}

// passwordExpired tells whether a command's stderr says the account must change its password before
// anything runs: OpenSSH's "WARNING: Your password has expired." and "Password change required but
// no TTY available.", PAM's "You are required to change your password immediately".
func passwordExpired(stderr []byte) bool {
	text := strings.ToLower(string(stderr))
	for _, p := range []string{"password has expired", "password change required", "you are required to change your password"} {
		if strings.Contains(text, p) {
			return true
		}
	}
	return false
}

// ctxFailure is the Failure of a context that is over, or nil.
func ctxFailure(ctx context.Context) *Failure {
	switch err := ctx.Err(); {
	case err == nil:
		return nil
	case errors.Is(err, context.DeadlineExceeded):
		return &Failure{Code: CodeTimeout}
	default:
		return &Failure{Code: CodeCancelled}
	}
}

func unsupported(detail string) *Failure { return &Failure{Code: CodeOSUnsupported, Detail: detail} }

func kitFailed(detail string) *Failure { return &Failure{Code: CodeKitFailed, Detail: detail} }

// passwordLine is what sudo -S reads: the password and a newline, in a slice the caller clears.
func passwordLine(password []byte) []byte {
	b := make([]byte, 0, len(password)+1)
	return append(append(b, password...), '\n')
}

// lastLine is the last non-blank line of a command's output, trimmed. bash reads ~/.bashrc even for
// the command of an SSH session, and one that greets the user prints before every command; the
// answer of a command is its last line.
func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// osRelease reads ID and VERSION_ID of /etc/os-release, without their quotes.
func osRelease(text string) (id, version string) {
	for _, line := range strings.Split(text, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.Trim(v, `"'`)
		switch k {
		case "ID":
			id = v
		case "VERSION_ID":
			version = v
		}
	}
	return id, version
}

// supportedOS: Debian 12 or later, Ubuntu 22.04 or later, as the kit checks them (ID only: a system
// merely like Ubuntu is not one the kit was tried on).
func supportedOS(id, version string) bool {
	major, minor, ok := parseVersion(version)
	if !ok {
		return false
	}
	switch id {
	case "debian":
		return major >= 12
	case "ubuntu":
		return major > 22 || major == 22 && minor >= 4
	}
	return false
}

// parseVersion reads "12", "12.5" or "22.04".
func parseVersion(v string) (major, minor int, ok bool) {
	a, b, dotted := strings.Cut(v, ".")
	major, err := strconv.Atoi(a)
	if err != nil || major < 0 || a == "" || a[0] == '+' || a[0] == '-' {
		return 0, 0, false
	}
	if !dotted {
		return major, 0, true
	}
	minor, err = strconv.Atoi(b)
	if err != nil || minor < 0 || b == "" || b[0] == '+' || b[0] == '-' {
		return 0, 0, false
	}
	return major, minor, true
}

// oneLine turns spaces, control characters and line breaks into single spaces.
func oneLine(s string) string {
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }), " ")
}

// clip cuts s to n characters, marking the cut with "…".
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}
