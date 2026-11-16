//go:build e2e

// The end-to-end check of the server setup (spec, section 9): serversetup.Run over real SSH against
// test servers in containers (Debian and Ubuntu with systemd and sshd) that
// scripts/server/e2e/run-e2e.ps1 starts on Docker Desktop. Only for local runs: the script starts the
// containers, sets SPLITLANE_E2E and runs
//
//	go test -tags e2e -run '^TestE2E$' -count=1 -v -timeout 45m ./internal/serversetup/
//
// SPLITLANE_E2E is a JSON list of targets {name, scenario, host, port, user, password | keyfile}:
// logins to the test servers, each with the scenario it runs. A scenario that needs a fresh server
// has a container of its own; key shares the container of root.
//
//	root          root with its password on a fresh server: done; a second run gives the same links;
//	              the kit's service, state and fail2ban jail; a wrong password from client is counted
//	              by the jail
//	sudo          kit with its password and sudo on a fresh server: done
//	key           kit with its key: no_admin sudo_needs_password (sudo wants the password, and a key
//	              login has none)
//	busy          root on a fresh server where nc holds 443/tcp and 443/udp: VLESS on 2053, Hysteria2
//	              on 4443
//	preinstalled  root on a server where the kit ran first without the new flags: the four ways in
//	              stay (three links and the WireGuard .conf on the server) and the state gets no
//	              PROTOCOLS
//	client        no test of its own: a container on the same Docker network that tries a wrong
//	              password on the root servers, from an address that had no SSH session to them
//	              during the setup (the jail exempts those, and fail2ban ignores the server's own)
//
// The subscription download is faked (it records the address and fails) and the wait is 10 s, so a
// done setup ends with sub_unreachable (sub_skipped on Ubuntu 22.04, which has no Caddy package):
// inside the containers api.ipify.org answers with this PC's exit address, so a real download would
// go to whatever server that is. For the same reason the containers resolve the ACME hosts of Let's
// Encrypt to 127.0.0.1, and a target that does not is refused before anything runs. Nothing logged
// here carries a secret: links are shown as scheme:port, public addresses as •••.
package serversetup_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net"
	"net/netip"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"splitlane/helper/internal/connector"
	"splitlane/helper/internal/serversetup"
)

// target is one login of SPLITLANE_E2E.
type target struct {
	Name     string `json:"name"`
	Scenario string `json:"scenario"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Keyfile  string `json:"keyfile"`
}

// The hosts of Let's Encrypt's ACME API that every container must send to 127.0.0.1 (--add-host).
var acmeHosts = []string{"acme-v02.api.letsencrypt.org", "acme-staging-v02.api.letsencrypt.org"}

// The subscription address of the kit: the sslip.io name of the public address, port, token.
var subURL = regexp.MustCompile(`^https://\d{1,3}-\d{1,3}-\d{1,3}-\d{1,3}\.sslip\.io:(\d+)/[0-9a-f]{24}/sub$`)

// The password the client tries; no server has it.
const wrongPassword = "not-the-password-e2e"

func TestE2E(t *testing.T) {
	raw := os.Getenv("SPLITLANE_E2E")
	if raw == "" {
		t.Skip("SPLITLANE_E2E is not set: scripts/server/e2e/run-e2e.ps1 runs this test")
	}
	var targets []target
	if err := json.Unmarshal([]byte(raw), &targets); err != nil {
		t.Fatalf("SPLITLANE_E2E is not a JSON list of targets: %v", err)
	}
	var client *target
	for i := range targets {
		if targets[i].Scenario == "client" {
			client = &targets[i]
		}
	}
	for _, tg := range targets {
		if tg.Scenario == "client" {
			continue
		}
		t.Run(tg.Name, func(t *testing.T) {
			t.Parallel()
			switch tg.Scenario {
			case "root":
				scenarioRoot(t, tg, client)
			case "sudo":
				scenarioSudo(t, tg)
			case "key":
				scenarioKey(t, tg)
			case "busy":
				scenarioBusy(t, tg)
			case "preinstalled":
				scenarioPreinstalled(t, tg)
			default:
				t.Fatalf("unknown scenario %q", tg.Scenario)
			}
		})
	}
}

// scenarioRoot: a fresh server, root with its password; then the same again; then the jail.
func scenarioRoot(t *testing.T, tg target, client *target) {
	b := login(t, tg)
	sub := prepare(t, b)
	w := want{ports: map[string]int{"vless": 443, "hysteria2": 443}, admin: "root", sub: sub}

	first := setup(t, tg)
	checkDone(t, tg, first, w)
	checkServer(t, b, sub)
	if p := stateValue(t, b, "PROTOCOLS"); p != "vless,hy2" {
		t.Errorf("PROTOCOLS in the state is %q, want vless,hy2", p)
	}

	again := setup(t, tg)
	checkDone(t, tg, again, w)
	same := again.done.Links == first.done.Links
	if !same {
		t.Errorf("the second run gave other links: %s, then %s", linkText(first.done.Links), linkText(again.done.Links))
	}
	checkServer(t, b, sub)

	if client == nil {
		t.Fatal("SPLITLANE_E2E has no client target: the jail check needs one")
	}
	jail := checkJailCounts(t, b, *client)
	t.Logf("RESULT %s: %s; re-run %s, same links: %v; %s", tg.Name, first.summary(), again.took, same, jail)
}

// scenarioSudo: a fresh server, kit with its password and sudo.
func scenarioSudo(t *testing.T, tg target) {
	b := login(t, tg)
	sub := prepare(t, b)
	o := setup(t, tg)
	checkDone(t, tg, o, want{ports: map[string]int{"vless": 443, "hysteria2": 443}, admin: "sudo with the password", sub: sub})
	checkServer(t, b, sub)
	t.Logf("RESULT %s: %s", tg.Name, o.summary())
}

// scenarioKey: kit with its key; sudo wants a password the request does not have.
func scenarioKey(t *testing.T, tg target) {
	b := login(t, tg)
	prepare(t, b)
	o := setup(t, tg)
	noSecret(t, tg, o)
	if o.fail == nil {
		t.Fatalf("done (%s), want no_admin sudo_needs_password", linkText(o.done.Links))
	}
	if o.fail.Code != serversetup.CodeNoAdmin || o.fail.Detail != "sudo_needs_password" {
		t.Errorf("failed with %s %q, want no_admin sudo_needs_password", o.fail.Code, maskPublic(o.fail.Detail))
	}
	for _, e := range o.events {
		if e.Step == "install" {
			t.Errorf("the kit was uploaded: %+v", e)
		}
	}
	t.Logf("RESULT %s: %s", tg.Name, o.summary())
}

// scenarioBusy: a fresh server where another program holds 443/tcp and 443/udp.
func scenarioBusy(t *testing.T, tg target) {
	b := login(t, tg)
	sub := prepare(t, b)
	b.must(t, "systemd-run --quiet --unit=e2e-busy-tcp nc -lk 443 && systemd-run --quiet --unit=e2e-busy-udp nc -lku 443")
	if !eventually(20*time.Second, func() bool {
		_, _, code, err := b.run("ss -Hltn 'sport = :443' | grep -q . && ss -Hlun 'sport = :443' | grep -q .", nil)
		return err == nil && code == 0
	}) {
		t.Fatal("nc does not hold 443/tcp and 443/udp")
	}
	o := setup(t, tg)
	checkDone(t, tg, o, want{ports: map[string]int{"vless": 2053, "hysteria2": 4443}, admin: "root", sub: sub})
	checkServer(t, b, sub)
	if v, h := stateValue(t, b, "VLESS_PORT"), stateValue(t, b, "HY2_PORT"); v != "2053" || h != "4443" {
		t.Errorf("the state remembers VLESS_PORT=%s HY2_PORT=%s, want 2053 and 4443", v, h)
	}
	t.Logf("RESULT %s: %s", tg.Name, o.summary())
}

// scenarioPreinstalled: the kit ran first as before this part (no flags: all four ways in, no
// PROTOCOLS in the state); the setup keeps all four.
func scenarioPreinstalled(t *testing.T, tg target) {
	b := login(t, tg)
	sub := prepare(t, b)
	if _, errOut, code, err := b.run("cat >/root/e2e-kit.sh", serversetup.Kit); err != nil || code != 0 {
		t.Fatalf("uploading the kit: status %d, %v: %s", code, err, tailOf(errOut))
	}
	start := time.Now()
	_, _, code, err := b.run("bash /root/e2e-kit.sh </dev/null >/root/e2e-kit.log 2>&1; rc=$?; rm -f /root/e2e-kit.sh; exit $rc", nil)
	if err != nil || code != 0 {
		log, _, _, _ := b.run("tail -n 5 /root/e2e-kit.log", nil)
		t.Fatalf("the kit without flags: status %d, %v: %s", code, err, tailOf(log))
	}
	b.must(t, "rm -f /root/e2e-kit.log") // it holds the links
	if p := stateValue(t, b, "PROTOCOLS"); p != "" {
		t.Fatalf("the kit without flags wrote PROTOCOLS=%s", p)
	}
	t.Logf("the kit without flags: done in %s, no PROTOCOLS in the state", time.Since(start).Round(time.Second))

	o := setup(t, tg)
	checkDone(t, tg, o, want{ports: map[string]int{"vless": 443, "hysteria2": 443, "ss": 8388}, admin: "root", sub: sub})
	checkServer(t, b, sub)
	if p := stateValue(t, b, "PROTOCOLS"); p != "" {
		t.Errorf("the setup wrote PROTOCOLS=%s into the state of a server set up without it", p)
	}
	ways := b.must(t, `grep -o '"type": *"[a-z0-9]*"' /etc/splitlane-kit/config.json | sed 's/.*"\([a-z0-9]*\)"$/\1/' | sort -u | tr '\n' ' '`)
	for _, w := range []string{"vless", "hysteria2", "shadowsocks", "wireguard"} {
		if !slices.Contains(strings.Fields(ways), w) {
			t.Errorf("the server config has no %s (types: %s)", w, ways)
		}
	}
	if _, _, code, _ := b.run("test -s /root/splitlane-kit/splitlane-wg.conf", nil); code != 0 {
		t.Error("/root/splitlane-kit/splitlane-wg.conf is gone")
	}
	t.Logf("RESULT %s: %s; the server config has %s; splitlane-wg.conf kept; no PROTOCOLS in the state", tg.Name, o.summary(), strings.TrimSpace(ways))
}

// ---- The setup -------------------------------------------------------------------------------

// fetchLog is the subscription download of the test: it records the address and fails.
type fetchLog struct {
	mu   sync.Mutex
	urls []string
}

func (f *fetchLog) Get(_ context.Context, url string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	return nil, errors.New("e2e: the subscription is not downloaded here")
}

// outcome is one run of serversetup.Run.
type outcome struct {
	events  []serversetup.Event
	done    serversetup.Done
	fail    *serversetup.Failure
	fetched []string
	took    time.Duration
}

// summary is a line for the matrix, without secrets.
func (o outcome) summary() string {
	if o.fail != nil {
		return fmt.Sprintf("failed %s %q in %s", o.fail.Code, maskPublic(o.fail.Detail), o.took)
	}
	s := fmt.Sprintf("done in %s; links %s; warnings %v", o.took, linkText(o.done.Links), o.done.Warnings)
	if len(o.fetched) > 0 {
		s += fmt.Sprintf("; subscription tried %d times at %s", len(o.fetched), maskURL(o.fetched[0]))
	}
	return s
}

// setup runs serversetup.Run for tg with the fake download and a 10 s wait, and logs its events.
func setup(t *testing.T, tg target) outcome {
	t.Helper()
	req := serversetup.Request{Host: tg.Host, Port: tg.Port, User: tg.User}
	if tg.Keyfile != "" {
		key, err := os.ReadFile(tg.Keyfile)
		if err != nil {
			t.Fatalf("reading the key: %v", err)
		}
		req.Key = key
	} else {
		req.Password = []byte(tg.Password)
	}
	defer req.Wipe()
	fetch := &fetchLog{}
	d := serversetup.DefaultDeps("e2e")
	d.Fetch = fetch
	d.SubWait = 10 * time.Second
	d.SubEvery = 2 * time.Second

	var o outcome
	var mu sync.Mutex
	start := time.Now()
	done, fail := serversetup.Run(context.Background(), req, d, func(e serversetup.Event) {
		mu.Lock()
		o.events = append(o.events, e)
		mu.Unlock()
		t.Logf("  %s %s: %s", e.Step, e.State, maskPublic(e.Detail))
	})
	o.took = time.Since(start).Round(time.Second)
	mu.Lock()
	defer mu.Unlock()
	o.done, o.fail = done, fail
	fetch.mu.Lock()
	o.fetched = slices.Clone(fetch.urls)
	fetch.mu.Unlock()
	return o
}

// want is what a done setup must give.
type want struct {
	ports map[string]int // the scheme of each link and its port
	admin string         // how the kit got root, as the system step says it
	sub   bool           // the kit sets up the subscription (Caddy is in the packages)
}

func checkDone(t *testing.T, tg target, o outcome, w want) {
	t.Helper()
	noSecret(t, tg, o)
	if o.fail != nil {
		t.Fatalf("failed with %s %q, want done", o.fail.Code, maskPublic(o.fail.Detail))
	}
	if got := linkPorts(t, o.done.Links); !maps.Equal(got, w.ports) {
		t.Errorf("links %s, want %v", linkText(o.done.Links), w.ports)
	}
	if o.done.Subscription != nil {
		t.Errorf("the subscription answered, and the fake download never does")
	}
	warnings := []string{"sub_unreachable"}
	if !w.sub {
		warnings = []string{"sub_skipped"}
	}
	if !slices.Equal(o.done.Warnings, warnings) {
		t.Errorf("warnings %v, want %v", o.done.Warnings, warnings)
	}
	switch {
	case w.sub && len(o.fetched) == 0:
		t.Error("the subscription address was never tried")
	case !w.sub && len(o.fetched) > 0:
		t.Errorf("a subscription address was tried without a subscription: %s", maskURL(o.fetched[0]))
	}
	for _, u := range o.fetched {
		if m := subURL.FindStringSubmatch(u); m == nil || m[1] != "8443" {
			t.Errorf("the subscription address %s is not https://<a-b-c-d>.sslip.io:8443/<token>/sub", maskURL(u))
			break
		}
	}
	has := func(step, state string, ok func(string) bool) bool {
		return slices.ContainsFunc(o.events, func(e serversetup.Event) bool {
			return e.Step == step && e.State == state && (ok == nil || ok(e.Detail))
		})
	}
	if !has("system", "done", func(d string) bool { return strings.HasSuffix(d, ", "+w.admin) }) {
		t.Errorf("no system step that ends with %q", ", "+w.admin)
	}
	if !has("install", "done", nil) {
		t.Error("no install done step")
	}
	if !has("harden", "done", nil) {
		t.Error("no harden done step")
	}
	end := map[bool]string{true: "warn", false: "skip"}[w.sub]
	if n := len(o.events); n == 0 || o.events[n-1].Step != "subscription" || o.events[n-1].State != end {
		t.Errorf("the last step is not subscription %s: %+v", end, o.events)
	}
}

// noSecret fails the test when the password of tg is in an event, the failure or the warnings.
func noSecret(t *testing.T, tg target, o outcome) {
	t.Helper()
	if tg.Password == "" {
		return
	}
	b, _ := json.Marshal(struct {
		E []serversetup.Event
		F *serversetup.Failure
		W []string
	}{o.events, o.fail, o.done.Warnings})
	if bytes.Contains(b, []byte(tg.Password)) {
		t.Error("the password is in the output of the setup")
	}
}

// ---- The server ------------------------------------------------------------------------------

// box is an SSH connection of the test to a server, for preparing and checking it.
type box struct{ c *ssh.Client }

func login(t *testing.T, tg target) *box {
	t.Helper()
	var auth ssh.AuthMethod
	if tg.Keyfile != "" {
		pem, err := os.ReadFile(tg.Keyfile)
		if err != nil {
			t.Fatalf("reading the key: %v", err)
		}
		signer, err := ssh.ParsePrivateKey(pem)
		if err != nil {
			t.Fatalf("the key: %v", err)
		}
		auth = ssh.PublicKeys(signer)
	} else {
		auth = ssh.Password(tg.Password)
	}
	cfg := &ssh.ClientConfig{User: tg.User, Auth: []ssh.AuthMethod{auth}, HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 15 * time.Second}
	c, err := ssh.Dial("tcp", net.JoinHostPort(tg.Host, strconv.Itoa(tg.Port)), cfg)
	if err != nil {
		t.Fatalf("%s: login as %s: %v", tg.Name, tg.User, err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return &box{c}
}

// run runs cmd with stdin (nil: none) and returns its stdout, its stderr and its exit status.
func (b *box) run(cmd string, stdin []byte) (string, string, int, error) {
	s, err := b.c.NewSession()
	if err != nil {
		return "", "", -1, err
	}
	defer s.Close()
	var out, errOut bytes.Buffer
	s.Stdout, s.Stderr = &out, &errOut
	if stdin != nil {
		s.Stdin = bytes.NewReader(stdin)
	}
	err = s.Run(cmd)
	var exit *ssh.ExitError
	switch {
	case err == nil:
		return out.String(), errOut.String(), 0, nil
	case errors.As(err, &exit):
		return out.String(), errOut.String(), exit.ExitStatus(), nil
	}
	return out.String(), errOut.String(), -1, err
}

// must runs cmd and returns its stdout, trimmed; anything but status 0 ends the test.
func (b *box) must(t *testing.T, cmd string) string {
	t.Helper()
	out, errOut, code, err := b.run(cmd, nil)
	if err != nil || code != 0 {
		t.Fatalf("%s: status %d, %v: %s", cmd, code, err, tailOf(errOut))
	}
	return strings.TrimSpace(out)
}

// prepare refuses a server that would reach Let's Encrypt, logs its system and sudo, and tells
// whether the kit sets up the subscription there (Ubuntu 22.04 has no Caddy package).
func prepare(t *testing.T, b *box) bool {
	t.Helper()
	for _, h := range acmeHosts {
		out := b.must(t, "getent ahosts "+h+" || true")
		var addrs []string
		for _, l := range strings.Split(out, "\n") {
			if f := strings.Fields(l); len(f) > 0 && !slices.Contains(addrs, f[0]) {
				addrs = append(addrs, f[0])
			}
		}
		if !slices.Equal(addrs, []string{"127.0.0.1"}) {
			t.Fatalf("%s resolves to %v on the server, not to 127.0.0.1 alone: start the container with --add-host %s:127.0.0.1", h, addrs, h)
		}
	}
	system := b.must(t, `. /etc/os-release && echo "$ID $VERSION_ID"`)
	sudo := b.must(t, "sudo --version 2>/dev/null | head -n 1 || true")
	openssh := b.must(t, "ssh -V 2>&1 | head -n 1 || true")
	t.Logf("server: %s; %s; %s", system, sudo, openssh)
	return system != "ubuntu 22.04"
}

// checkServer: the kit's service runs, Caddy too when the subscription is set up, and the uploaded
// kit is gone from /tmp. Any user can ask.
func checkServer(t *testing.T, b *box, sub bool) {
	t.Helper()
	if n := b.must(t, "ls -d /tmp/splitlane-kit.* 2>/dev/null | wc -l"); n != "0" {
		t.Errorf("%s copies of the uploaded kit are left in /tmp", n)
	}
	units := []string{"splitlane-kit", "fail2ban"}
	if sub {
		units = append(units, "caddy")
	}
	for _, u := range units {
		if out, _, _, _ := b.run("systemctl is-active "+u, nil); strings.TrimSpace(out) != "active" {
			t.Errorf("%s is %q", u, strings.TrimSpace(out))
		}
	}
}

// stateValue is the value of key in the kit's state ("" when it is not there); root only. The state
// holds the keys of the server, so nothing else of it is read.
func stateValue(t *testing.T, b *box, key string) string {
	t.Helper()
	return b.must(t, "sed -n 's/^"+key+"=//p' /etc/splitlane-kit/state")
}

// checkJailCounts makes the client try a wrong password on the server (root) and waits until the
// kit's sshd jail counts it. The client's address must not be in the jail's ignoreip; fail2ban
// ignores the server's own addresses anyway (ignoreself), so the try cannot come from the server.
func checkJailCounts(t *testing.T, server *box, client target) string {
	t.Helper()
	jail := server.must(t, "sed -n 's/^ignoreip *= *//p' /etc/fail2ban/jail.d/splitlane-kit.local")
	ignore := strings.Fields(jail)
	serverIP := firstIPv4(t, server.must(t, "hostname -I"))
	c := login(t, client)
	clientIP := firstIPv4(t, c.must(t, "hostname -I"))
	if slices.Contains(ignore, clientIP) {
		t.Fatalf("the client %s is in the jail's ignoreip (%s): it had a session during the setup", clientIP, jail)
	}
	status := server.must(t, "fail2ban-client status sshd")
	before, matches := totalFailed(t, status), jailMatches(status)

	try := fmt.Sprintf(`f=$(mktemp) && printf '#!/bin/sh\necho %s\n' >"$f" && chmod 700 "$f" &&
SSH_ASKPASS="$f" SSH_ASKPASS_REQUIRE=force DISPLAY=none ssh -n -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null \
  -o LogLevel=ERROR -o PreferredAuthentications=password -o PubkeyAuthentication=no -o NumberOfPasswordPrompts=1 \
  -o ConnectTimeout=10 kit@%s true; rc=$?; rm -f "$f"; exit $rc`, wrongPassword, serverIP)
	_, errOut, code, err := c.run(try, nil)
	if err != nil || code != 255 || !strings.Contains(errOut, "Permission denied") {
		t.Fatalf("the wrong password from the client: status %d, %v: %s", code, err, tailOf(errOut))
	}
	after := before
	counted := eventually(30*time.Second, func() bool {
		out, _, _, _ := server.run("fail2ban-client status sshd", nil)
		after = totalFailed(t, out)
		return after > before
	})
	text := fmt.Sprintf("fail2ban sshd: Total failed %d -> %d after a wrong password from %s (ignoreip %s; journal matches %s)",
		before, after, clientIP, jail, matches)
	if !counted {
		t.Errorf("the jail did not count the failed login: %s", text)
	}
	return text
}

var (
	totalFailedLine = regexp.MustCompile(`Total failed:\s*(\d+)`)
	matchesLine     = regexp.MustCompile(`Journal matches:\s*(.*)`)
)

func totalFailed(t *testing.T, status string) int {
	t.Helper()
	m := totalFailedLine.FindStringSubmatch(status)
	if m == nil {
		t.Fatalf("no Total failed in fail2ban-client status sshd: %s", tailOf(status))
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

func jailMatches(status string) string {
	if m := matchesLine.FindStringSubmatch(status); m != nil {
		return strings.TrimSpace(m[1])
	}
	return "(none)"
}

func firstIPv4(t *testing.T, list string) string {
	t.Helper()
	for _, f := range strings.Fields(list) {
		if a, err := netip.ParseAddr(f); err == nil && a.Is4() {
			return f
		}
	}
	t.Fatalf("no IPv4 address in %q", list)
	return ""
}

// eventually calls ok every second until it is true or d is over.
func eventually(d time.Duration, ok func() bool) bool {
	for end := time.Now().Add(d); ; {
		if ok() {
			return true
		}
		if time.Now().After(end) {
			return false
		}
		time.Sleep(time.Second)
	}
}

// ---- Links and masks -------------------------------------------------------------------------

// linkPorts maps the scheme of each link to its port.
func linkPorts(t *testing.T, links string) map[string]int {
	t.Helper()
	got := map[string]int{}
	for _, l := range strings.Split(links, "\n") {
		scheme, port, ok := schemePort(l)
		if !ok {
			t.Errorf("a link without scheme, host and port: %s", connector.Mask(maskPublic(l)))
			continue
		}
		got[scheme] = port
	}
	return got
}

// schemePort reads the scheme and the port of a link: scheme://userinfo@host:port[/][?query][#name].
func schemePort(link string) (string, int, bool) {
	scheme, rest, ok := strings.Cut(link, "://")
	if !ok {
		return "", 0, false
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimSuffix(rest[strings.LastIndex(rest, "@")+1:], "/")
	_, p, err := net.SplitHostPort(rest)
	if err != nil {
		return "", 0, false
	}
	port, err := strconv.Atoi(p)
	return scheme, port, err == nil
}

// linkText is the links as "scheme:port ...", nothing secret.
func linkText(links string) string {
	var parts []string
	for _, l := range strings.Split(links, "\n") {
		if scheme, port, ok := schemePort(l); ok {
			parts = append(parts, scheme+":"+strconv.Itoa(port))
		} else if l != "" {
			parts = append(parts, "?")
		}
	}
	return strings.Join(parts, " ")
}

// publicIPv4 finds IPv4 addresses, dotted or dashed (sslip.io names).
var publicIPv4 = regexp.MustCompile(`\b\d{1,3}([.-])\d{1,3}[.-]\d{1,3}[.-]\d{1,3}\b`)

// maskPublic replaces public IPv4 addresses (this PC's exit address among them) with •••; loopback
// and private ones stay.
func maskPublic(s string) string {
	return publicIPv4.ReplaceAllStringFunc(s, func(m string) string {
		a, err := netip.ParseAddr(strings.ReplaceAll(m, "-", "."))
		if err == nil && (a.IsLoopback() || a.IsPrivate()) {
			return m
		}
		return "•••"
	})
}

// maskURL shows a subscription address as https://•••.sslip.io:<port>/•••/sub.
func maskURL(u string) string {
	if m := subURL.FindStringSubmatch(u); m != nil {
		return "https://•••.sslip.io:" + m[1] + "/•••/sub"
	}
	return connector.Mask(maskPublic(u))
}

// tailOf is the last three lines of a command's output, masked, for a failure.
func tailOf(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > 3 {
		lines = lines[len(lines)-3:]
	}
	return connector.Mask(maskPublic(strings.Join(lines, " | ")))
}
