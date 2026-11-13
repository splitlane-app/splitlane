package probe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"splitlane/helper/internal/engine"
)

func service(status int, body string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
}

func TestDirect(t *testing.T) {
	p := New()
	cases := []struct {
		status int
		body   string
		want   engine.ProbeResult
	}{
		{200, "203.0.113.7\n", engine.ProbeResult{Reached: true, IP: "203.0.113.7"}},
		{503, "busy", engine.ProbeResult{Reached: true}},
		{200, "<html>", engine.ProbeResult{Reached: true}},
		{200, "2001:db8::1", engine.ProbeResult{Reached: true}},
	}
	for _, c := range cases {
		srv := service(c.status, c.body)
		if got := p.Direct(context.Background(), srv.URL); got != c.want {
			t.Errorf("%d %q: got %+v", c.status, c.body, got)
		}
		srv.Close()
	}
}

func TestFailureIsNotReached(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := l.Addr().String()
	l.Close()
	if got := New().Direct(context.Background(), "http://"+addr+"/"); got.Reached {
		t.Fatal("a refused connection is a failure")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { time.Sleep(time.Second) }))
	defer slow.Close()
	p := &Prober{Timeout: 100 * time.Millisecond}
	if got := p.Direct(context.Background(), slow.URL); got.Reached {
		t.Fatal("a timeout is a failure")
	}
}

// ViaProxy goes through the given local port, as through the probe inbounds of the core.
func TestViaProxy(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "address.example" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte("198.51.100.4"))
	}))
	defer proxy.Close()
	port := proxy.Listener.Addr().(*net.TCPAddr).Port
	got := New().ViaProxy(context.Background(), "http://address.example/", port)
	if got != (engine.ProbeResult{Reached: true, IP: "198.51.100.4"}) {
		t.Fatalf("got %+v", got)
	}
}
