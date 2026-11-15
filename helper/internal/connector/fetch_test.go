package connector

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

// token is the secret part of the test subscription addresses: no error may carry it.
const token = "s3cr3tT0ken"

// hosts is a resolver for the tests: names map to fixed addresses, any other name does not exist.
// No test asks a real DNS server.
func hosts(m map[string][]string) func(context.Context, string, string) ([]netip.Addr, error) {
	return func(_ context.Context, _, host string) ([]netip.Addr, error) {
		var out []netip.Addr
		for _, s := range m[strings.ToLower(host)] {
			out = append(out, netip.MustParseAddr(s))
		}
		if len(out) == 0 {
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
		return out, nil
	}
}

// testFetcher trusts the certificate of ts (nil: none) and resolves names with hosts (nil: none).
// It fails closed: after the real control, a socket to anything but 127.0.0.1 (where the test
// servers listen) fails the test unconnected, so a broken check never reaches out of the test.
func testFetcher(t *testing.T, ts *httptest.Server, o fetchOptions) *httpFetcher {
	if ts != nil {
		o.roots = x509.NewCertPool()
		o.roots.AddCert(ts.Certificate())
	}
	if o.lookup == nil {
		o.lookup = hosts(nil)
	}
	f := newFetcher("1.2.3", o).(*httpFetcher)
	f.dialer.Control = func(network, address string, c syscall.RawConn) error {
		if err := f.control(network, address, c); err != nil {
			return err
		}
		if ap, err := netip.ParseAddrPort(address); err != nil || ap.Addr() != netip.MustParseAddr("127.0.0.1") {
			t.Errorf("a socket to %s", address)
			return errors.New("stop")
		}
		return nil
	}
	return f
}

// port is the port of the test server, for addresses that name it under another host.
func port(t *testing.T, ts *httptest.Server) string {
	t.Helper()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	return u.Port()
}

// wantCode checks that err is a *FetchError with code and that its text names no part of the
// address but the host.
func wantCode(t *testing.T, err error, code string) *FetchError {
	t.Helper()
	var fe *FetchError
	if !errors.As(err, &fe) {
		t.Fatalf("error %v (%T), want *FetchError %s", err, err, code)
	}
	if fe.Code != code {
		t.Fatalf("code %s (%v), want %s", fe.Code, err, code)
	}
	if s := err.Error(); strings.Contains(s, token) || strings.Contains(s, "/sub") || strings.Contains(s, "://") {
		t.Fatalf("error text %q carries the address", s)
	}
	return fe
}

// wait holds a handler until the client goes away (or 5 s), so the server closes cleanly.
func wait(r *http.Request) {
	select {
	case <-r.Context().Done():
	case <-time.After(5 * time.Second):
	}
}

func TestFetchSuccess(t *testing.T) {
	// The helper runs elevated with the user's environment: a proxy named there is not used.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	body := base64.StdEncoding.EncodeToString([]byte(linkTrojan + "\n" + linkHy2 + "\n"))
	var seen atomic.Value // written by the handler goroutine
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sub/"+token {
			http.NotFound(w, r)
			return
		}
		seen.Store(r.Header.Get("User-Agent") + "|" + r.Header.Get("Accept-Encoding") + "|" + r.Proto)
		fmt.Fprint(w, body)
	}))
	ts.EnableHTTP2 = true // the server offers h2; the fetcher speaks HTTP/1.1 only
	ts.StartTLS()
	defer ts.Close()
	f := testFetcher(t, ts, fetchOptions{allowLoopback: true, lookup: hosts(map[string][]string{"sub.example.com": {"127.0.0.1"}})})
	got, err := f.Get(context.Background(), "https://sub.example.com:"+port(t, ts)+"/sub/"+token+"?flag=x")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != body {
		t.Fatalf("body %q, want %q", got, body)
	}
	// No Accept-Encoding: the body comes as is, and the limit is of the bytes downloaded.
	if seen.Load() != "SplitLane/1.2.3||HTTP/1.1" {
		t.Fatalf("User-Agent|Accept-Encoding|protocol %v", seen.Load())
	}
	servers, rejected := ParseSubscription(got)
	if len(servers) != 2 || len(rejected) != 0 {
		t.Fatalf("servers %d, rejected %v", len(servers), rejected)
	}
}

// The configuration of the real fetcher: what the tests run with a test option is checked here.
func TestFetchProductionConfig(t *testing.T) {
	f := NewFetcher("1.2.3").(*httpFetcher)
	tr, ok := f.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport %T", f.client.Transport)
	}
	switch {
	case tr.Proxy != nil:
		t.Error("a proxy function: the helper has the user's environment")
	case f.client.Jar != nil:
		t.Error("a cookie jar")
	case !tr.DisableKeepAlives:
		t.Error("keep-alives")
	case !tr.DisableCompression:
		t.Error("compression: the limit is of the bytes downloaded")
	case tr.ForceAttemptHTTP2 || tr.Protocols == nil || tr.Protocols.HTTP2() || !tr.Protocols.HTTP1():
		t.Error("not HTTP/1.1 only")
	case tr.TLSClientConfig == nil || tr.TLSClientConfig.MinVersion != tls.VersionTLS12 ||
		tr.TLSClientConfig.InsecureSkipVerify || tr.TLSClientConfig.RootCAs != nil:
		t.Error("TLS: want 1.2 or later, verified against the system roots")
	case tr.DialContext == nil || f.dialer.Control == nil:
		t.Error("the dialer or its control is not wired")
	case f.allowLoopback:
		t.Error("loopback allowed")
	case f.timeout != FetchTimeout:
		t.Errorf("timeout %v", f.timeout)
	}
	// The control of the real dialer refuses on its own, before any connection.
	_, err := f.dialer.DialContext(context.Background(), "tcp", "127.0.0.1:1")
	wantCode(t, err, CodeFetchBlockedAddress)
}

// The real fetcher never reaches the local test server: loopback is open only to tests.
func TestFetchDefaultRefusesLoopback(t *testing.T) {
	var hits atomic.Int32
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer ts.Close()
	_, err := NewFetcher("1.2.3").Get(context.Background(), ts.URL+"/sub/"+token)
	wantCode(t, err, CodeFetchBlockedAddress)
	// Names the resolver maps to loopback, without the loopback option.
	f := testFetcher(t, ts, fetchOptions{lookup: hosts(map[string][]string{
		"localhost":       {"127.0.0.1", "::1"},
		"sub.example.com": {"127.0.0.1"},
	})})
	for _, h := range []string{"localhost", "sub.example.com"} {
		_, err := f.Get(context.Background(), "https://"+h+":"+port(t, ts)+"/sub/"+token)
		wantCode(t, err, CodeFetchBlockedAddress)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("the server got %d requests", n)
	}
}

// The certificate is checked against the name of the address, not the address dialed: a name the
// certificate lacks fails, though the certificate is for 127.0.0.1.
func TestFetchCertificateName(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Config.ErrorLog = log.New(io.Discard, "", 0) // the refused handshake
	ts.StartTLS()
	defer ts.Close()
	f := testFetcher(t, ts, fetchOptions{allowLoopback: true, lookup: hosts(map[string][]string{"sub.example.net": {"127.0.0.1"}})})
	_, err := f.Get(context.Background(), "https://sub.example.net:"+port(t, ts)+"/sub/"+token)
	if fe := wantCode(t, err, CodeFetchFailed); fe.Detail != "certificate" {
		t.Fatalf("detail %q", fe.Detail)
	}
}

func TestFetchBlockedTargets(t *testing.T) {
	f := testFetcher(t, nil, fetchOptions{lookup: hosts(map[string][]string{
		"loop.example.com":   {"127.0.0.1"},
		"loop6.example.com":  {"::1"},
		"meta.example.com":   {"169.254.169.254"},
		"lan.example.com":    {"192.168.1.1"},
		"cgnat.example.com":  {"100.100.100.100"},
		"mapped.example.com": {"::ffff:10.0.0.1"},
		"mixed.example.com":  {"198.51.100.7", "10.0.0.1"},
		"ula.example.com":    {"fd7a:115c:a1e0::1"},
		"fake.example.com":   {"198.18.0.1"},
	})})
	for _, h := range []string{
		"127.0.0.1", "127.0.0.1:8443", "10.0.0.1", "169.254.169.254", "100.100.100.100", "[::1]",
		"[::ffff:127.0.0.1]", "[::ffff:169.254.169.254]", "0.0.0.0", "172.16.0.1", "192.168.1.1",
		"224.0.0.1", "255.255.255.255", "[::]", "[fe80::1]", "[fc00::1]", "[ff02::1]", "[64:ff9b::a00:1]",
		"198.18.0.1", "[::ffff:0:a00:1]", "fake.example.com",
		"loop.example.com", "loop6.example.com", "meta.example.com", "lan.example.com",
		"cgnat.example.com", "mapped.example.com", "mixed.example.com", "ula.example.com",
	} {
		t.Run(h, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := f.Get(ctx, "https://"+h+"/sub/"+token)
			wantCode(t, err, CodeFetchBlockedAddress)
		})
	}
}

// Each layer refuses on its own, with the later ones out of the way: the address check before
// anything is sent, the dialer after resolution, and the socket control on the address connected.
func TestFetchLayers(t *testing.T) {
	f := newFetcher("1.2.3", fetchOptions{lookup: hosts(map[string][]string{
		"lan.example.com":   {"192.168.1.1"},
		"mixed.example.com": {"198.51.100.7", "fd00::1"},
	})}).(*httpFetcher)
	for _, s := range []string{"https://10.0.0.1/", "https://[::ffff:127.0.0.1]:8443/", "https://100.100.100.100/"} {
		u, _ := url.Parse(s)
		if fe := f.checkURL(u); fe == nil || fe.Code != CodeFetchBlockedAddress {
			t.Fatalf("checkURL %s: %v", s, fe)
		}
	}
	u, _ := url.Parse("https://lan.example.com/") // a name is for the dialer
	if fe := f.checkURL(u); fe != nil {
		t.Fatalf("checkURL of a name: %v", fe)
	}
	// The dialer alone: were it to let an address through, the control would fail the test before
	// any connection is attempted.
	f.dialer.Control = func(network, address string, _ syscall.RawConn) error {
		t.Errorf("the dialer let %s through", address)
		return errors.New("stop")
	}
	for _, a := range []string{"10.0.0.1:443", "[::1]:443", "lan.example.com:443", "mixed.example.com:443"} {
		_, err := f.dial(context.Background(), "tcp", a)
		wantCode(t, err, CodeFetchBlockedAddress)
	}
	if _, err := f.dial(context.Background(), "tcp", "unknown.example.com:443"); err == nil || errors.As(err, new(*FetchError)) {
		t.Fatalf("dial of an unknown name: %v", err)
	}
	for _, a := range []string{"127.0.0.1:443", "169.254.169.254:80", "[::ffff:10.0.0.1]:443", "[fe80::1%5]:443", "bad"} {
		if err := f.control("tcp4", a, nil); err == nil {
			t.Fatalf("control %s: no error", a)
		}
	}
	if err := f.control("tcp4", "198.51.100.7:443", nil); err != nil {
		t.Fatalf("control of a public address: %v", err)
	}
}

func TestFetchNotHTTPS(t *testing.T) {
	f := testFetcher(t, nil, fetchOptions{})
	for _, u := range []string{
		"http://127.0.0.1/sub/" + token, "http://example.com/sub/" + token, "HTTP://example.com/sub/" + token,
		"ftp://example.com/sub/" + token, "example.com/sub/" + token,
	} {
		_, err := f.Get(context.Background(), u)
		wantCode(t, err, CodeFetchNotHTTPS)
	}
	for _, u := range []string{"https://exa mple.com/sub/" + token, "https://example.com/sub/\n" + token, "https:///sub/" + token, "https://1.2.3/sub/" + token} {
		_, err := f.Get(context.Background(), u)
		wantCode(t, err, CodeFetchFailed)
	}
}

func TestFetchRedirects(t *testing.T) {
	var p string
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/hop/"):
			n, _ := strconv.Atoi(strings.TrimPrefix(r.URL.Path, "/hop/"))
			if n > 0 {
				http.Redirect(w, r, "/hop/"+strconv.Itoa(n-1), http.StatusFound)
				return
			}
			if r.Header.Get("User-Agent") != "SplitLane/1.2.3" {
				http.Error(w, "user agent", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, linkTrojan)
		case r.URL.Path == "/to-meta":
			http.Redirect(w, r, "https://169.254.169.254/latest/meta-data/"+token, http.StatusFound)
		case r.URL.Path == "/to-mapped":
			http.Redirect(w, r, "https://[::ffff:10.0.0.1]/sub/"+token, http.StatusMovedPermanently)
		case r.URL.Path == "/to-lan":
			http.Redirect(w, r, "https://lan.example.com:"+p+"/sub/"+token, http.StatusTemporaryRedirect)
		case r.URL.Path == "/to-loop":
			http.Redirect(w, r, "https://127.0.0.1:"+p+"/hop/0", http.StatusFound)
		case r.URL.Path == "/to-http":
			http.Redirect(w, r, "http://sub.example.com:"+p+"/sub/"+token, http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()
	p = port(t, ts)
	f := testFetcher(t, ts, fetchOptions{allowLoopback: true, lookup: hosts(map[string][]string{
		"sub.example.com": {"127.0.0.1"},
		"lan.example.com": {"192.168.1.1"},
	})})
	base := "https://sub.example.com:" + p
	for _, c := range []struct{ path, code string }{
		{"/hop/0", ""},
		{"/hop/5", ""},
		{"/to-loop", ""},
		{"/hop/6", CodeFetchTooManyRedirects},
		{"/to-meta", CodeFetchBlockedAddress},
		{"/to-mapped", CodeFetchBlockedAddress},
		{"/to-lan", CodeFetchBlockedAddress},
		{"/to-http", CodeFetchNotHTTPS},
	} {
		t.Run(c.path, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			body, err := f.Get(ctx, base+c.path)
			if c.code == "" {
				if err != nil || string(body) != linkTrojan {
					t.Fatalf("body %q, error %v", body, err)
				}
				return
			}
			wantCode(t, err, c.code)
		})
	}
}

func TestFetchTooLarge(t *testing.T) {
	chunk := strings.Repeat("a", 64<<10)
	stream := func(w http.ResponseWriter, n int) {
		w.(http.Flusher).Flush() // headers out: the body goes chunked, without a length
		for ; n > 0; n -= len(chunk) {
			if _, err := fmt.Fprint(w, chunk[:min(n, len(chunk))]); err != nil {
				return
			}
		}
	}
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/declared":
			// A length over the limit is refused before any of the body is read: the body never comes.
			w.Header().Set("Content-Length", strconv.Itoa(MaxSubscriptionLen+1))
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			wait(r)
		case "/streamed":
			stream(w, MaxSubscriptionLen+1)
		case "/exact":
			stream(w, MaxSubscriptionLen)
		}
	}))
	defer ts.Close()
	f := testFetcher(t, ts, fetchOptions{allowLoopback: true, timeout: 3 * time.Second})
	for _, p := range []string{"/declared", "/streamed"} {
		_, err := f.Get(context.Background(), ts.URL+p)
		wantCode(t, err, CodeFetchTooLarge)
	}
	body, err := f.Get(context.Background(), ts.URL+"/exact")
	if err != nil || len(body) != MaxSubscriptionLen {
		t.Fatalf("exact limit: %d bytes, error %v", len(body), err)
	}
}

func TestFetchTimeout(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/body" {
			fmt.Fprint(w, "trojan://")
			w.(http.Flusher).Flush()
		}
		wait(r)
	}))
	defer ts.Close()
	f := testFetcher(t, ts, fetchOptions{allowLoopback: true, timeout: 300 * time.Millisecond})
	for _, p := range []string{"/headers", "/body"} {
		start := time.Now()
		_, err := f.Get(context.Background(), ts.URL+p)
		fe := wantCode(t, err, CodeFetchTimeout)
		if !errors.Is(fe, context.DeadlineExceeded) {
			t.Fatalf("%s: %v is not DeadlineExceeded", p, err)
		}
		if d := time.Since(start); d > 3*time.Second {
			t.Fatalf("%s took %v", p, d)
		}
	}
}

// The transport dials without the deadline of the request, so a hung resolution or connection would
// outlive Get: the dial ends with Get, at the deadline of Get (here the caller's, the earlier one).
func TestFetchDialBounded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	want, _ := ctx.Deadline()
	done := make(chan time.Time, 1)
	f := testFetcher(t, nil, fetchOptions{lookup: func(ctx context.Context, _, _ string) ([]netip.Addr, error) {
		deadline, _ := ctx.Deadline()
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
		}
		done <- deadline
		return nil, ctx.Err()
	}})
	_, err := f.Get(ctx, "https://slow.example.com/sub/"+token)
	wantCode(t, err, CodeFetchTimeout)
	select {
	case got := <-done:
		if !got.Equal(want) {
			t.Fatalf("the dial's deadline %v, want that of Get %v", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("the dial outlived Get")
	}
}

func TestFetchCanceled(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { wait(r) }))
	defer ts.Close()
	f := testFetcher(t, ts, fetchOptions{allowLoopback: true})
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	_, err := f.Get(ctx, ts.URL+"/sub/"+token)
	fe := wantCode(t, err, CodeFetchFailed)
	if !errors.Is(fe, context.Canceled) {
		t.Fatalf("%v is not Canceled", err)
	}
}

func TestFetchHTTPStatus(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sub/" + token:
			http.Error(w, "gone", http.StatusNotFound)
		case "/err":
			http.Error(w, "oops", http.StatusInternalServerError)
		case "/empty-redirect":
			w.WriteHeader(http.StatusFound) // no Location: the response itself
		}
	}))
	defer ts.Close()
	f := testFetcher(t, ts, fetchOptions{allowLoopback: true})
	for _, c := range []struct {
		path   string
		status int
	}{{"/sub/" + token, 404}, {"/err", 500}, {"/empty-redirect", 302}} {
		_, err := f.Get(context.Background(), ts.URL+c.path)
		if fe := wantCode(t, err, CodeFetchHTTPStatus); fe.Status != c.status {
			t.Fatalf("%s: status %d, want %d", c.path, fe.Status, c.status)
		}
	}
}

// A certificate the roots do not trust fails the download: verification is never skipped. The roots
// are an empty pool rather than the system's, which on Windows may go online for a root update.
func TestFetchUntrustedCertificate(t *testing.T) {
	ts := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Config.ErrorLog = log.New(io.Discard, "", 0) // the refused handshake
	ts.StartTLS()
	defer ts.Close()
	f := testFetcher(t, nil, fetchOptions{allowLoopback: true, roots: x509.NewCertPool()})
	_, err := f.Get(context.Background(), ts.URL+"/sub/"+token)
	if fe := wantCode(t, err, CodeFetchFailed); fe.Detail != "certificate" {
		t.Fatalf("detail %q", fe.Detail)
	}
}

func TestBlockedAddr(t *testing.T) {
	blocked := []string{
		"0.0.0.0", "0.1.2.3", "10.0.0.1", "10.255.255.255", "100.64.0.1", "100.100.100.100",
		"100.127.255.255", "127.0.0.1", "127.255.255.254", "169.254.169.254", "169.254.0.1",
		"172.16.0.1", "172.31.255.255", "192.0.0.8", "192.168.1.1", "224.0.0.1", "239.255.255.250",
		"240.0.0.1", "255.255.255.255",
		"::", "::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "::ffff:100.64.0.1", "::127.0.0.1",
		"::a00:1", "fe80::1", "fe80::1%5", "fec0::1", "fc00::1", "fd7a:115c:a1e0::1", "ff02::1",
		"ff05::2", "64:ff9b::a00:1", "64:ff9b::7f00:1", "64:ff9b:1::1", "2002:7f00:1::1", "2001::1",
		"100::1", "198.18.0.1", "198.19.255.255", "::ffff:0:a00:1", "::ffff:0:c633:6407",
	}
	allowed := []string{
		"198.51.100.7", "192.0.2.1", "100.63.255.255", "100.128.0.0", "172.32.0.1", "11.0.0.1",
		"198.17.255.255", "198.20.0.1", "::ffff:198.51.100.7", "2001:db8::1", "64:ff9b::c633:6407",
	}
	for _, s := range blocked {
		if !blockedAddr(netip.MustParseAddr(s), false) {
			t.Errorf("%s is allowed", s)
		}
	}
	for _, s := range allowed {
		if blockedAddr(netip.MustParseAddr(s), false) {
			t.Errorf("%s is blocked", s)
		}
	}
	if blockedAddr(netip.MustParseAddr("127.0.0.1"), true) || blockedAddr(netip.MustParseAddr("::1"), true) {
		t.Error("loopback is blocked with the test option")
	}
	if !blockedAddr(netip.MustParseAddr("10.0.0.1"), true) || !blockedAddr(netip.Addr{}, true) {
		t.Error("the test option opens more than loopback")
	}
}
