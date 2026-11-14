package singbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// fakeAPI behaves like the Clash API of sing-box 1.14.4: 204 for any PATCH, unknown modes ignored.
type fakeAPI struct {
	mu      sync.Mutex
	mode    string
	deleted []string
}

func (f *fakeAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") != "Bearer s3cret" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch {
	case r.Method == http.MethodPatch && r.URL.Path == "/configs":
		var body struct {
			Mode string `json:"mode"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		for _, m := range []string{"vpn", "vpn-localdns", "direct", "direct-localdns", "alldirect"} {
			if strings.EqualFold(m, body.Mode) {
				f.mode = m
			}
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && r.URL.Path == "/configs":
		json.NewEncoder(w).Encode(map[string]any{"mode": f.mode, "mode-list": []string{"vpn"}})
	case r.Method == http.MethodGet && r.URL.Path == "/connections":
		// The shape of experimental/clashapi/connections.go in sing-box 1.14.4: host is the domain
		// (the reverse mapping of DNS answers gives one, 6.4), destinationIP is "" for a destination
		// without an address, the counters are cumulative byte counts.
		w.Write([]byte(`{"connections":[
 {"chains":["direct"],"download":5000000000,"id":"a1","metadata":{"destinationIP":"203.0.113.5","destinationPort":"443","dnsMode":"normal","host":"example.org","network":"tcp","processPath":"C:\\Tools\\curl-a.exe","sourceIP":"192.0.2.10","sourcePort":"50001","type":"tun/tun-in"},"rule":"rule_set=lane-direct => route(direct)","rulePayload":"","start":"2026-11-14T12:00:00.5+03:00","upload":1024},
 {"chains":["vpn"],"download":0,"id":"b2","metadata":{"destinationIP":"198.51.100.7","destinationPort":"443","dnsMode":"normal","host":"","network":"udp","processPath":"","sourceIP":"192.0.2.10","sourcePort":"50002","type":"tun/tun-in"},"rule":"final","rulePayload":"","start":"2026-11-14T12:00:01+03:00","upload":0},
 {"chains":["vpn"],"download":10,"id":"c3","metadata":{"destinationIP":"","destinationPort":"443","dnsMode":"normal","host":"api.ipify.org","network":"tcp","processPath":"","sourceIP":"127.0.0.1","sourcePort":"50003","type":"mixed/probe-vpn"},"rule":"inbound=[probe-vpn] => route(vpn)","rulePayload":"","start":"2026-11-14T12:00:02+03:00","upload":20},
 {"chains":["direct"],"download":7,"id":"d4","metadata":{"destinationIP":"","destinationPort":"0","dnsMode":"normal","host":"","network":"tcp","processPath":"C:\\Tools\\x.exe","sourceIP":"192.0.2.10","sourcePort":"50004","type":"tun/tun-in"},"rule":"clash_mode=alldirect => route(direct)","rulePayload":"","start":"2026-11-14T12:00:03+03:00","upload":3}],
 "downloadTotal":5000000017,"memory":0,"uploadTotal":1047}`))
	case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/connections/"):
		f.deleted = append(f.deleted, strings.TrimPrefix(r.URL.Path, "/connections/"))
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestClashModeIsReadBack(t *testing.T) {
	api := &fakeAPI{mode: "vpn"}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := NewClash(srv.URL, "s3cret")
	if err := c.SetMode(context.Background(), "direct-localdns"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetMode(context.Background(), "global"); err == nil || !strings.Contains(err.Error(), "did not take") {
		t.Fatalf("an unknown mode must fail on the read back, got %v", err)
	}
	if err := NewClash(srv.URL, "wrong").SetMode(context.Background(), "vpn"); err == nil {
		t.Fatal("a wrong secret must fail")
	}
}

func TestClashConnectionsAndClose(t *testing.T) {
	api := &fakeAPI{}
	srv := httptest.NewServer(api)
	defer srv.Close()
	c := NewClash(srv.URL, "s3cret")
	conns, err := c.Connections(context.Background())
	if err != nil || len(conns) != 4 {
		t.Fatalf("%v %v", conns, err)
	}
	if conns[0].ProcessPath != `C:\Tools\curl-a.exe` || conns[0].Lane != "direct" || !conns[0].ByLanes {
		t.Fatalf("first %+v", conns[0])
	}
	// rules 8–11 of 6.2 (a lane file, the final route, a mode) count; a built-in rule does not
	if !conns[1].ByLanes || conns[2].ByLanes || !conns[3].ByLanes || conns[2].Lane != "vpn" {
		t.Fatalf("%+v", conns)
	}
	if err := c.CloseConn(context.Background(), "a1"); err != nil {
		t.Fatal(err)
	}
	if err := c.CloseConn(context.Background(), ""); err == nil {
		t.Fatal("an empty id must never become DELETE /connections")
	}
	if len(api.deleted) != 1 || api.deleted[0] != "a1" {
		t.Fatalf("deleted %v", api.deleted)
	}
}

// Spec 8.4: the connections give the activity its counters and hosts. upload and download are the
// cumulative bytes (more than 4 GiB fit); the host is metadata.host, the name from the reverse mapping
// of DNS answers (6.4), else metadata.destinationIP, else "".
func TestConnectionsHaveCounters(t *testing.T) {
	srv := httptest.NewServer(&fakeAPI{})
	defer srv.Close()
	conns, err := NewClash(srv.URL, "s3cret").Connections(context.Background())
	if err != nil || len(conns) != 4 {
		t.Fatalf("%v %v", conns, err)
	}
	want := []struct {
		host             string
		upload, download int64
	}{
		{"example.org", 1024, 5000000000},
		{"198.51.100.7", 0, 0},
		{"api.ipify.org", 20, 10},
		{"", 3, 7},
	}
	for i, w := range want {
		if c := conns[i]; c.Host != w.host || c.Upload != w.upload || c.Download != w.download {
			t.Errorf("connection %d: %+v, want host %q up %d down %d", i, c, w.host, w.upload, w.download)
		}
	}
}
