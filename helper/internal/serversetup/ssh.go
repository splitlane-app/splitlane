package serversetup

import (
	"bytes"
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/crypto/ssh"
)

const (
	maxOutput       = 64 << 10 // the stdout a command of the checks may print; the tail of stderr kept of any
	maxKitLine      = 1 << 20  // a longer line of the kit is skipped (its result line is a few KB)
	maxPromptRounds = 3        // keyboard-interactive rounds answered before the login counts as refused
)

// kitExitGrace: once the kit has printed its result or error line, how long its session may stay
// open before it is closed (a program the kit started could hold the output open). A variable for the
// tests.
var kitExitGrace = 10 * time.Second

var (
	// errPrompt: the server asked something the password is not the answer to (a question shown as
	// typed, such as a one-time code) or kept asking.
	errPrompt = errors.New("ssh: a keyboard-interactive question the password does not answer")
	// errPasswordRefused stops the login once the password method has failed: keyboard-interactive
	// would send the same password again, a second failure for fail2ban and MaxAuthTries.
	errPasswordRefused = errors.New("ssh: the server refused the password")
	// errNoPassword, errNoKey: the server offers no method the login can use (a password to a server
	// that takes keys only, a key to one that takes passwords only); nothing is sent.
	errNoPassword = errors.New("ssh: the server does not accept passwords")
	errNoKey      = errors.New("ssh: the server does not accept keys")
)

// sshConn is the SSH connection of a setup.
type sshConn struct {
	client *ssh.Client
	lost   chan struct{} // closed when the connection is gone
	stop   func() bool   // ends the watch of the context
	once   sync.Once
}

// connect makes the TCP connection within ConnectTimeout, then the SSH handshake with the login within
// ConnectTimeout again. A key is read before anything is dialled, so ssh_key comes without network.
// From here to close, the end of ctx closes the connection: whatever waits on it fails at once and
// the caller reports the context's code.
func connect(ctx context.Context, req Request, d Deps, addr string) (*sshConn, *Failure) {
	auth, authCallback, f := authMethods(req)
	if f != nil {
		return nil, f
	}
	dialCtx, cancel := context.WithTimeout(ctx, d.ConnectTimeout)
	raw, err := d.Dial(dialCtx, "tcp", addr)
	cancel()
	if err != nil {
		if f := ctxFailure(ctx); f != nil {
			return nil, f
		}
		return nil, &Failure{Code: CodeSSHUnreachable, Detail: dialDetail(err)}
	}
	stop := context.AfterFunc(ctx, func() { raw.Close() })
	var keyed atomic.Bool // the host key has come: what fails after it is the login
	cfg := &ssh.ClientConfig{
		User:         req.User,
		Auth:         auth,
		AuthCallback: authCallback,
		// The host key is not checked (spec, section 3): a VPS rented a minute ago has no key to
		// compare with, and the access is one-off and not remembered. The known risk, written in the
		// README and PRIVACY: whoever stands in for the server at the moment of the setup learns the
		// password.
		HostKeyCallback: func(host string, remote net.Addr, key ssh.PublicKey) error {
			keyed.Store(true)
			return ssh.InsecureIgnoreHostKey()(host, remote, key)
		},
	}
	_ = raw.SetDeadline(time.Now().Add(d.ConnectTimeout))
	sc, chans, reqs, err := ssh.NewClientConn(raw, addr, cfg)
	if err != nil {
		stop()
		raw.Close()
		if f := ctxFailure(ctx); f != nil {
			return nil, f
		}
		switch {
		case errors.Is(err, errNoPassword):
			return nil, &Failure{Code: CodeSSHAuth, Detail: "no_password"}
		case errors.Is(err, errNoKey):
			return nil, &Failure{Code: CodeSSHAuth, Detail: "no_key"}
		case isTimeout(err):
			return nil, &Failure{Code: CodeSSHUnreachable, Detail: "timeout"}
		case !keyed.Load():
			return nil, &Failure{Code: CodeSSHUnreachable, Detail: "ssh"} // not SSH, or no common algorithm
		}
		return nil, &Failure{Code: CodeSSHAuth}
	}
	_ = raw.SetDeadline(time.Time{})
	c := &sshConn{client: ssh.NewClient(sc, chans, reqs), lost: make(chan struct{}), stop: stop}
	go func() {
		_ = c.client.Wait()
		close(c.lost)
	}()
	return c, nil
}

// authMethods: a key, or the password by the password method and by keyboard-interactive (each
// question shown without echo gets the password; one shown with echo, or a fourth round, refuses the
// login). Keyboard-interactive is used only when the server does not take passwords by the password
// method (the callback), so a wrong password is one failure a run.
func authMethods(req Request) ([]ssh.AuthMethod, ssh.ClientAuthCallback, *Failure) {
	if len(req.Key) > 0 {
		signer, f := parseKey(req.Key, req.Passphrase)
		if f != nil {
			return nil, nil, f
		}
		onlyKey := func(c *ssh.ClientAuthContext) (ssh.AuthMethod, error) {
			if !slices.Contains(c.AllowedMethods, "publickey") {
				return nil, errNoKey
			}
			return nil, nil
		}
		return []ssh.AuthMethod{ssh.PublicKeys(signer)}, onlyKey, nil
	}
	// x/crypto/ssh takes the password as a string: that copy cannot be wiped, the garbage collector
	// frees it after the run.
	password := string(req.Password)
	rounds := 0
	kbd := ssh.KeyboardInteractive(func(_, _ string, questions []string, echos []bool) ([]string, error) {
		if rounds++; rounds > maxPromptRounds {
			return nil, errPrompt
		}
		answers := make([]string, len(questions))
		for i := range questions {
			if echos[i] {
				return nil, errPrompt
			}
			answers[i] = password
		}
		return answers, nil
	})
	once := func(c *ssh.ClientAuthContext) (ssh.AuthMethod, error) {
		if !slices.Contains(c.AllowedMethods, "password") && !slices.Contains(c.AllowedMethods, "keyboard-interactive") {
			return nil, errNoPassword
		}
		if slices.Contains(c.TriedMethods, "password") {
			return nil, errPasswordRefused
		}
		return nil, nil // the next method of Auth the server allows
	}
	return []ssh.AuthMethod{ssh.Password(password), kbd}, once, nil
}

// parseKey reads an OpenSSH or PEM private key, with its passphrase when it has one. A PuTTY key is
// ssh_key "ppk" (PuTTYgen exports OpenSSH); a missing or wrong passphrase is "passphrase"; anything
// else "format". A passphrase given for a key without one is ignored. The detail never quotes the key.
func parseKey(key, passphrase []byte) (ssh.Signer, *Failure) {
	key = bytes.TrimLeft(bytes.TrimPrefix(key, []byte{0xef, 0xbb, 0xbf}), " \t\r\n") // Notepad's BOM
	if bytes.HasPrefix(key, []byte("PuTTY-User-Key-File")) {
		return nil, &Failure{Code: CodeSSHKey, Detail: "ppk"}
	}
	signer, err := ssh.ParsePrivateKey(key)
	var missing *ssh.PassphraseMissingError
	switch {
	case err == nil:
		return signer, nil
	case !errors.As(err, &missing):
		return nil, &Failure{Code: CodeSSHKey, Detail: "format"}
	case len(passphrase) == 0:
		return nil, &Failure{Code: CodeSSHKey, Detail: "passphrase"}
	}
	signer, err = ssh.ParsePrivateKeyWithPassphrase(key, passphrase)
	switch {
	case err == nil:
		return signer, nil
	case errors.Is(err, x509.IncorrectPasswordError):
		return nil, &Failure{Code: CodeSSHKey, Detail: "passphrase"}
	}
	return nil, &Failure{Code: CodeSSHKey, Detail: "format"}
}

// dialDetail says why the TCP connection failed, in a fixed word.
func dialDetail(err error) string {
	var dns *net.DNSError
	switch {
	case errors.As(err, &dns):
		return "name"
	case isTimeout(err) || errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	return "connect"
}

func isTimeout(err error) bool {
	var ne net.Error
	return errors.Is(err, os.ErrDeadlineExceeded) || errors.As(err, &ne) && ne.Timeout()
}

// cmdResult is what a command left.
type cmdResult struct {
	stdout   []byte // the first maxOutput bytes; nil when the lines went to onLine
	stderr   []byte // the last maxOutput bytes
	status   int    // the exit status
	stdinErr error  // stdin did not go through whole
}

// run runs cmd in a session of its own: stdin is written and closed, stdout and stderr are read to
// their end. With onLine, stdout goes to it line by line instead of into the result; once onLine
// returns true (the kit's last line), the session gets kitExitGrace to end and is then closed. The
// error is a failure of SSH (a refused session, a connection gone, no exit status); a command that
// fails is a status.
func (c *sshConn) run(cmd string, stdin []byte, onLine func([]byte) bool) (cmdResult, error) {
	var res cmdResult
	sess, err := c.client.NewSession()
	if err != nil {
		return res, err
	}
	defer sess.Close()
	in, err := sess.StdinPipe()
	if err != nil {
		return res, err
	}
	out, err := sess.StdoutPipe()
	if err != nil {
		return res, err
	}
	errOut, err := sess.StderrPipe()
	if err != nil {
		return res, err
	}
	if err := sess.Start(cmd); err != nil {
		return res, err
	}
	stderr := &tail{max: maxOutput}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stderr, errOut)
	}()
	go func() {
		defer wg.Done()
		if onLine == nil {
			res.stdout, _ = io.ReadAll(io.LimitReader(out, maxOutput))
			_, _ = io.Copy(io.Discard, out) // the rest, so the command is not stuck on a full window
			return
		}
		var grace *time.Timer
		_ = eachLine(out, maxKitLine, func(line []byte) {
			if onLine(line) && grace == nil {
				grace = time.AfterFunc(kitExitGrace, func() { _ = sess.Close() })
			}
		})
		if grace != nil {
			grace.Stop()
		}
	}()
	if len(stdin) > 0 {
		if _, err := in.Write(stdin); err != nil {
			res.stdinErr = err
		}
	}
	if err := in.Close(); err != nil && len(stdin) > 0 && res.stdinErr == nil {
		res.stdinErr = err
	}
	wg.Wait()
	err = sess.Wait()
	res.stderr = stderr.bytes()
	var exit *ssh.ExitError
	switch {
	case err == nil:
	case errors.As(err, &exit):
		res.status = exit.ExitStatus()
	default:
		return res, err
	}
	return res, nil
}

// alive tells whether the connection still works: a keepalive request gets an answer (any answer:
// OpenSSH refuses the request it does not know, which is an answer).
func (c *sshConn) alive() bool {
	select {
	case <-c.lost:
		return false
	default:
	}
	_, _, err := c.client.SendRequest("keepalive@openssh.com", true, nil)
	return err == nil
}

func (c *sshConn) close() {
	c.once.Do(func() {
		c.stop()
		_ = c.client.Close()
	})
}
