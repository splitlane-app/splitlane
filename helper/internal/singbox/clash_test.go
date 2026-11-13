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
		w.Write([]byte(`{"downloadTotal":1,"uploadTotal":2,"connections":[
 {"id":"a1","metadata":{"processPath":"C:\\Tools\\curl-a.exe","host":"example.org"},"chains":["direct"],"rule":"rule_set=lane-direct => route(direct)"},
 {"id":"b2","metadata":{"processPath":""},"chains":["vpn"],"rule":"final"},
 {"id":"c3","metadata":{"processPath":""},"chains":["vpn"],"rule":"inbound=[probe-vpn] => route(vpn)"},
 {"id":"d4","metadata":{"processPath":"C:\\Tools\\x.exe"},"chains":["direct"],"rule":"clash_mode=alldirect => route(direct)"}],"memory":0}`))
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
