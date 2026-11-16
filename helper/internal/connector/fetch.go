package connector

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Fetcher downloads the body of a subscription address.
type Fetcher interface {
	Get(ctx context.Context, url string) ([]byte, error)
}

// Limits of a subscription download (spec of stage 3 part 1, section 3). The body limit is
// MaxSubscriptionLen.
const (
	FetchTimeout = 20 * time.Second // the whole download: resolution, connection, redirects, body
	MaxRedirects = 5
)

// Codes of a failed download (FetchError.Code).
const (
	CodeFetchBlockedAddress   = "fetch_blocked_address"    // the address or a redirect leads to the machine itself or a local network
	CodeFetchNotHTTPS         = "fetch_not_https"          // the address or a redirect is not https
	CodeFetchTooManyRedirects = "fetch_too_many_redirects" // more than MaxRedirects
	CodeFetchTooLarge         = "fetch_too_large"          // the body is over MaxSubscriptionLen
	CodeFetchTimeout          = "fetch_timeout"            // over FetchTimeout
	CodeFetchHTTPStatus       = "fetch_http_status"        // the server answered with a status other than 200
	CodeFetchFailed           = "fetch_failed"             // anything else: a bad address, DNS, TLS, the connection
)

// FetchError is a failed download. Its text never carries the address, which holds the token of a
// subscription: Detail is a fixed word or the status, never text from the address or the server.
type FetchError struct {
	Code   string // one of the CodeFetch* constants
	Detail string
	Status int // the HTTP status of CodeFetchHTTPStatus

	cause error // context.DeadlineExceeded or context.Canceled, else nil
}

func (e *FetchError) Error() string {
	if e.Detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

// Unwrap lets errors.Is find context.DeadlineExceeded and context.Canceled; nothing else is kept.
func (e *FetchError) Unwrap() error { return e.cause }

func fetchError(code, detail string) *FetchError { return &FetchError{Code: code, Detail: detail} }

// deadlineKey is the context key of the deadline of Get, for dial.
type deadlineKey struct{}

// fetchOptions are for tests only; NewFetcher uses none of them.
type fetchOptions struct {
	allowLoopback bool           // the test server listens on 127.0.0.1; no other blocked address opens
	lookup        lookupFunc     // the resolver; nil: the system's
	timeout       time.Duration  // 0: FetchTimeout
	roots         *x509.CertPool // nil: the system's
}

// lookupFunc resolves a name as net.Resolver.LookupNetIP does.
type lookupFunc func(ctx context.Context, network, host string) ([]netip.Addr, error)

type httpFetcher struct {
	userAgent     string
	timeout       time.Duration
	allowLoopback bool
	lookup        lookupFunc
	dialer        net.Dialer
	client        *http.Client
}

// NewFetcher returns the Fetcher of the helper. version is the helper's, for the User-Agent.
//
// The helper runs elevated, so an address a user pastes must not reach the machine itself or a
// network behind it: the address must be https, and no address it leads to may be loopback,
// link-local (169.254.169.254 among them), private, CGNAT (the tailnet, 100.64.0.0/10), multicast,
// unspecified or reserved, in any IPv6 form that carries such an IPv4 address. It is checked three
// times: the host of the address and of every redirect when it is an IP address, before anything is
// sent; every address a name resolves to, before connecting (a name with one bad address is refused
// whole); and the address of the socket itself, right before it connects, so what is checked is what
// is connected. No proxy of the environment is used: the helper has the user's environment. HTTP/1.1
// only, without compression; at most MaxRedirects redirects, a body of MaxSubscriptionLen bytes as
// downloaded, and FetchTimeout for the whole download, its connections included.
func NewFetcher(version string) Fetcher { return newFetcher(version, fetchOptions{}) }

func newFetcher(version string, o fetchOptions) Fetcher {
	f := &httpFetcher{
		userAgent:     "SplitLane/" + version,
		timeout:       o.timeout,
		allowLoopback: o.allowLoopback,
		lookup:        o.lookup,
	}
	if f.timeout <= 0 {
		f.timeout = FetchTimeout
	}
	if f.lookup == nil {
		f.lookup = net.DefaultResolver.LookupNetIP
	}
	f.dialer = net.Dialer{Control: f.control}
	var http1 http.Protocols // HTTP/1.1 only: one connection per request, and the path the tests run
	http1.SetHTTP1(true)
	f.client = &http.Client{
		Transport: &http.Transport{
			Proxy:                  nil, // never HTTP(S)_PROXY: the helper has the user's environment
			DialContext:            f.dial,
			TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: o.roots},
			Protocols:              &http1,
			DisableKeepAlives:      true, // one download at a time; nothing stays open in the helper
			DisableCompression:     true, // the body as sent: the limit is of the bytes downloaded
			TLSHandshakeTimeout:    10 * time.Second,
			MaxResponseHeaderBytes: 64 << 10,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > MaxRedirects {
				return fetchError(CodeFetchTooManyRedirects, "")
			}
			if fe := f.checkURL(req.URL); fe != nil {
				return fe
			}
			return nil
		},
	}
	return f
}

// Get downloads the body of rawURL, which must be https. A failure is a *FetchError.
func (f *httpFetcher) Get(ctx context.Context, rawURL string) ([]byte, error) {
	if len(rawURL) > 2048 || strings.ContainsFunc(rawURL, func(r rune) bool { return r <= ' ' || r == 0x7f }) {
		return nil, fetchError(CodeFetchFailed, "bad address")
	}
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	deadline, _ := ctx.Deadline()
	// The transport dials under context.WithoutCancel of the request, which keeps values only: the
	// deadline goes to dial as a value.
	ctx = context.WithValue(ctx, deadlineKey{}, deadline)
	defer f.client.CloseIdleConnections()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fetchError(CodeFetchFailed, "bad address")
	}
	if fe := f.checkURL(req.URL); fe != nil {
		return nil, fe
	}
	req.Header.Set("User-Agent", f.userAgent)
	resp, err := f.client.Do(req)
	if err != nil {
		return nil, classify(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &FetchError{Code: CodeFetchHTTPStatus, Detail: "status " + strconv.Itoa(resp.StatusCode), Status: resp.StatusCode}
	}
	if resp.ContentLength > MaxSubscriptionLen {
		return nil, fetchError(CodeFetchTooLarge, "")
	}
	// One byte over the limit tells a body over it.
	body, err := io.ReadAll(io.LimitReader(resp.Body, MaxSubscriptionLen+1))
	// The deadline is the last word: a read of a chunked body that races it can end without an error
	// and with only part of the body (TestFetchTimeout), so a body read past it is not trusted.
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return nil, classify(ctx, err)
	}
	if len(body) > MaxSubscriptionLen {
		return nil, fetchError(CodeFetchTooLarge, "")
	}
	return body, nil
}

// checkURL checks the address of a request or a redirect before anything is sent to it: https, a
// host that is a sane name or an IP address, and that address not a blocked one.
func (f *httpFetcher) checkURL(u *url.URL) *FetchError {
	if u.Scheme != "https" {
		return fetchError(CodeFetchNotHTTPS, "")
	}
	h := u.Hostname()
	if a, err := netip.ParseAddr(h); err == nil {
		if f.blocked(a) {
			return fetchError(CodeFetchBlockedAddress, "")
		}
		return nil
	}
	if _, ok := cleanHost(h); !ok {
		return fetchError(CodeFetchFailed, "bad address")
	}
	return nil
}

// dial is the DialContext of the transport: it resolves the name itself, refuses it when any of its
// addresses is blocked, and connects only to the addresses it checked, one after another.
func (f *httpFetcher) dial(ctx context.Context, network, address string) (net.Conn, error) {
	// The transport dials without the deadline of the request (a dial outlives a canceled request,
	// for the next one to use): the dial ends at the deadline of Get, which comes as a value.
	deadline, ok := ctx.Value(deadlineKey{}).(time.Time)
	if !ok || deadline.IsZero() {
		deadline = time.Now().Add(f.timeout)
	}
	ctx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	host, p, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	port, err := strconv.ParseUint(p, 10, 16)
	if err != nil {
		return nil, err
	}
	var addrs []netip.Addr
	if a, err := netip.ParseAddr(host); err == nil {
		addrs = []netip.Addr{a}
	} else {
		ipNetwork := "ip"
		switch network {
		case "tcp4":
			ipNetwork = "ip4"
		case "tcp6":
			ipNetwork = "ip6"
		}
		if addrs, err = f.lookup(ctx, ipNetwork, host); err != nil {
			return nil, err
		}
		if len(addrs) == 0 {
			return nil, &net.DNSError{Err: "no address", Name: host, IsNotFound: true}
		}
	}
	for _, a := range addrs {
		if f.blocked(a) {
			return nil, fetchError(CodeFetchBlockedAddress, "")
		}
	}
	var first error
	for i, a := range addrs {
		dctx, dcancel := partialContext(ctx, len(addrs)-i)
		c, err := f.dialer.DialContext(dctx, network, netip.AddrPortFrom(a.Unmap(), uint16(port)).String())
		dcancel()
		if err == nil {
			return c, nil
		}
		if first == nil {
			first = err
		}
		if ctx.Err() != nil {
			break
		}
	}
	return nil, first
}

// partialContext gives one of the remaining addresses its share of the time left, at least 2 s (as
// the dialer of net does), so one silent address does not use up the whole download.
func partialContext(ctx context.Context, remaining int) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok || remaining <= 1 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, max(time.Until(deadline)/time.Duration(remaining), 2*time.Second))
}

// control is the Control of the dialer: the last check, on the address the socket connects to.
func (f *httpFetcher) control(network, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil || f.blocked(ap.Addr()) {
		return fetchError(CodeFetchBlockedAddress, "")
	}
	return nil
}

func (f *httpFetcher) blocked(a netip.Addr) bool { return blockedAddr(a, f.allowLoopback) }

// blockedPrefixes are blocked beside what the methods of netip.Addr name in blockedAddr.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network": 0.x.x.x reaches the machine itself on some systems
	netip.MustParsePrefix("100.64.0.0/10"),   // CGNAT, the tailnet of Tailscale among it
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking; the fake-IP range of local proxy clients
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, the broadcast 255.255.255.255 among it
	netip.MustParsePrefix("::/96"),           // IPv4-compatible (deprecated): an IPv4 address inside
	netip.MustParsePrefix("::ffff:0:0:0/96"), // IPv4-translated (SIIT): an IPv4 address inside
	netip.MustParsePrefix("64:ff9b:1::/48"),  // local-use NAT64
	netip.MustParsePrefix("100::/64"),        // discard-only
	netip.MustParsePrefix("2001::/32"),       // Teredo: an IPv4 address inside
	netip.MustParsePrefix("2002::/16"),       // 6to4: an IPv4 address inside
	netip.MustParsePrefix("fec0::/10"),       // site-local (deprecated)
}

// nat64 is the well-known NAT64 prefix: the IPv4 address in its last 32 bits is checked instead.
var nat64 = netip.MustParsePrefix("64:ff9b::/96")

// blockedAddr reports whether a download may not connect to a: loopback (unless allowLoopback, for
// tests), link-local, private (10/8, 172.16/12, 192.168/16, fc00::/7), multicast, unspecified,
// blockedPrefixes, an address with a zone, and any of them as an IPv4-mapped or NAT64 address.
func blockedAddr(a netip.Addr, allowLoopback bool) bool {
	if !a.IsValid() || a.Zone() != "" {
		return true
	}
	a = a.Unmap()
	if nat64.Contains(a) {
		b := a.As16()
		a = netip.AddrFrom4([4]byte(b[12:]))
	}
	if a.IsLoopback() {
		return !allowLoopback
	}
	if a.IsUnspecified() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsMulticast() {
		return true
	}
	for _, p := range blockedPrefixes {
		if p.Contains(a) {
			return true
		}
	}
	return false
}

// classify turns an error of the transport into a *FetchError without its text: the text of
// *url.Error carries the address, and that of other errors may carry a redirect address.
func classify(ctx context.Context, err error) *FetchError {
	var fe *FetchError
	if errors.As(err, &fe) {
		return fe
	}
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return &FetchError{Code: CodeFetchTimeout, cause: context.DeadlineExceeded}
	case errors.Is(ctx.Err(), context.Canceled):
		return &FetchError{Code: CodeFetchFailed, Detail: "canceled", cause: context.Canceled}
	}
	var dnsErr *net.DNSError
	var certErr *tls.CertificateVerificationError
	var netErr net.Error
	var opErr *net.OpError
	switch {
	case errors.As(err, &dnsErr):
		if dnsErr.IsTimeout {
			return &FetchError{Code: CodeFetchTimeout, Detail: "dns", cause: context.DeadlineExceeded}
		}
		return fetchError(CodeFetchFailed, "dns")
	case errors.As(err, &certErr):
		return fetchError(CodeFetchFailed, "certificate")
	case errors.As(err, &netErr) && netErr.Timeout():
		return &FetchError{Code: CodeFetchTimeout, cause: context.DeadlineExceeded}
	case errors.As(err, &opErr) && opErr.Op == "dial":
		return fetchError(CodeFetchFailed, "connection")
	}
	return fetchError(CodeFetchFailed, "network")
}
