// Package probe asks the address service from which address a request arrives (spec 5.3 steps
// 5 and 8, 5.5). Every check is a new connection with a 5 s timeout.
package probe

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"splitlane/helper/internal/engine"
)

// Prober implements engine.Prober.
type Prober struct {
	Timeout time.Duration
}

var _ engine.Prober = (*Prober)(nil)

// New returns a prober with the 5 s timeout of 5.5.
func New() *Prober { return &Prober{Timeout: 5 * time.Second} }

// ViaProxy sends the request through a probe inbound of the core on 127.0.0.1:port.
func (p *Prober) ViaProxy(ctx context.Context, target string, port int) engine.ProbeResult {
	proxy := &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)}
	return p.get(ctx, target, &http.Transport{Proxy: http.ProxyURL(proxy), DisableKeepAlives: true})
}

// Direct is the helper's own request in the ordinary way, through the adapter when it is up.
func (p *Prober) Direct(ctx context.Context, target string) engine.ProbeResult {
	return p.get(ctx, target, &http.Transport{Proxy: nil, DisableKeepAlives: true})
}

// get: a timeout or a connection error is a failure; an HTTP error or a body without an IPv4
// address is a link that works with an unknown address (5.5).
func (p *Prober) get(ctx context.Context, target string, tr *http.Transport) engine.ProbeResult {
	defer tr.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, p.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return engine.ProbeResult{}
	}
	resp, err := (&http.Client{Transport: tr}).Do(req)
	if err != nil {
		return engine.ProbeResult{}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return engine.ProbeResult{Reached: true}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	text := strings.TrimSpace(string(body))
	if addr, err := netip.ParseAddr(text); err != nil || !addr.Is4() {
		return engine.ProbeResult{Reached: true}
	}
	return engine.ProbeResult{Reached: true, IP: text}
}
