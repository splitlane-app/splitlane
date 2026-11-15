package pipe

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"splitlane/helper/internal/protocol"
)

// fakeSource plays the engine's activity: a summary, with the detail of detailFor when one is asked
// for, while ok; it records the detailFor of every call. With pace, each call first waits (at most
// 5 s) for a token that the reading client gives back for each event it got.
type fakeSource struct {
	mu    sync.Mutex
	ok    bool
	apps  []protocol.ActivityApp
	calls []string
	pace  chan struct{}
}

var _ ActivitySource = (*fakeSource)(nil)

func (f *fakeSource) Activity(detailFor string) (protocol.Activity, bool) {
	f.mu.Lock()
	f.calls = append(f.calls, detailFor)
	ok, apps, pace := f.ok, f.apps, f.pace
	f.mu.Unlock()
	if pace != nil {
		select {
		case <-pace:
		case <-time.After(5 * time.Second):
		}
	}
	a := protocol.Activity{Lanes: protocol.ActivityLanes{VPN: protocol.Traffic{Down: 1000, Up: 100}}, Apps: apps}
	if detailFor != "" {
		a.Detail = &protocol.ActivityDetail{Key: detailFor}
	}
	return a, ok
}

func (f *fakeSource) set(ok bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ok = ok
}

func (f *fakeSource) asked() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.calls...)
}

// request sends a request and returns its answer; the events that come before it are skipped.
func (c *testClient) request(t *testing.T, line string) map[string]any {
	t.Helper()
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	for {
		if m := c.next(t); m["event"] == nil {
			return m
		}
	}
}

// activity is the next message, which must be an activity event; it returns its detail key, "" for none.
func (c *testClient) activity(t *testing.T) string {
	t.Helper()
	m := c.next(t)
	data, _ := m["data"].(map[string]any)
	if m["event"] != protocol.EventActivity || data["lanes"] == nil {
		t.Fatalf("not an activity event: %v", m)
	}
	d, _ := data["detail"].(map[string]any)
	key, _ := d["key"].(string)
	return key
}

func hello(t *testing.T, cs ...*testClient) {
	t.Helper()
	for _, c := range cs {
		if m := c.send(t, `{"id":1,"method":"hello","params":{"protocol":2}}`); m["result"] == nil {
			t.Fatalf("hello: %v", m)
		}
	}
}

// Spec 5.1, 8.4: the server answers activity.subscribe itself, with {} (after hello only), and sends the
// event to every client that subscribed; a client with detailFor gets the detail, the others do not, and
// a client that did not subscribe gets nothing. The source is asked once per detailFor and tick, not
// once per client. RunActivity returns at Close.
func TestActivitySubscribe(t *testing.T) {
	s, h, name := serve(t)
	early := dial(t, name)
	if code := errCode(early.send(t, `{"id":1,"method":"activity.subscribe"}`)); code != protocol.CodeBadRequest {
		t.Fatalf("before hello: %s", code)
	}
	a, a2, b, quiet := dial(t, name), dial(t, name), dial(t, name), dial(t, name)
	hello(t, a, a2, b, quiet)
	key := `C:\Apps\Tool\`
	for _, c := range []*testClient{a, a2} {
		if m := c.send(t, `{"id":2,"method":"activity.subscribe"}`); fmt.Sprint(m["result"]) != "map[]" {
			t.Fatalf("subscribe: %v", m)
		}
	}
	if m := b.send(t, `{"id":2,"method":"activity.subscribe","params":{"detailFor":"C:\\Apps\\Tool\\"}}`); fmt.Sprint(m["result"]) != "map[]" {
		t.Fatalf("subscribe with detailFor: %v", m)
	}
	h.mu.Lock()
	seen := strings.Join(h.methods, ",")
	h.mu.Unlock()
	if strings.Contains(seen, protocol.MethodActivitySubscribe) {
		t.Fatalf("the handler got activity.subscribe: %s", seen)
	}
	src := &fakeSource{ok: true}
	returned := make(chan struct{})
	go func() {
		s.RunActivity(src, 20*time.Millisecond)
		close(returned)
	}()
	for i := 0; i < 3; i++ {
		for _, c := range []*testClient{a, a2} {
			if got := c.activity(t); got != "" {
				t.Fatalf("a subscription without detailFor got the detail of %q", got)
			}
		}
		if got := b.activity(t); got != key {
			t.Fatalf("detail %q, want %q", got, key)
		}
	}
	// had it been sent an event, the event would come before the answer
	if m := quiet.send(t, `{"id":2,"method":"state.get"}`); m["event"] != nil || m["id"] == nil {
		t.Fatalf("a client that did not subscribe got %v", m)
	}
	calls := src.asked()
	for i := 0; i+1 < len(calls); i += 2 {
		if pair := []string{calls[i], calls[i+1]}; !(pair[0] == "" && pair[1] == key) && !(pair[0] == key && pair[1] == "") {
			t.Fatalf("calls %q: once per detailFor and tick", calls)
		}
	}
	s.Close()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		t.Fatal("RunActivity did not return at Close")
	}
}

// Spec 5.1: a repeated activity.subscribe replaces the subscription; one with bad params is bad_request
// and leaves the subscription as it was.
func TestResubscribeReplaces(t *testing.T) {
	s, _, name := serve(t)
	c := dial(t, name)
	hello(t, c)
	c.send(t, `{"id":2,"method":"activity.subscribe","params":{"detailFor":"k"}}`)
	go s.RunActivity(&fakeSource{ok: true}, 20*time.Millisecond)
	if got := c.activity(t); got != "k" {
		t.Fatalf("detail %q", got)
	}
	if m := c.request(t, `{"id":3,"method":"activity.subscribe","params":{}}`); fmt.Sprint(m["result"]) != "map[]" {
		t.Fatalf("subscribe again: %v", m)
	}
	for i := 0; i < 3; i++ {
		if got := c.activity(t); got != "" {
			t.Fatalf("after the second subscribe: detail %q", got)
		}
	}
	if code := errCode(c.request(t, `{"id":4,"method":"activity.subscribe","params":{"detailFor":"k","more":1}}`)); code != protocol.CodeBadRequest {
		t.Fatalf("unknown field: %s", code)
	}
	for i := 0; i < 3; i++ {
		if got := c.activity(t); got != "" {
			t.Fatalf("after a refused subscribe: detail %q", got)
		}
	}
}

// Review Focus 3: a client that subscribed and stopped reading does not block the helper or the other
// clients: it is dropped as for state events, and the reading client keeps getting the events.
func TestStalledSubscriberIsDropped(t *testing.T) {
	s, h, name := serve(t)
	stalled, reading := dial(t, name), dial(t, name)
	stalled.conn.Write([]byte(`{"id":1,"method":"hello","params":{"protocol":2}}` + "\n" +
		`{"id":2,"method":"activity.subscribe"}` + "\n")) // never reads again
	hello(t, reading)
	reading.send(t, `{"id":2,"method":"activity.subscribe"}`)
	deadline := time.Now().Add(5 * time.Second)
	for h.count() != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	// ~20 KB per event: the stalled client's pipe buffer (64 KB) and queue (64) fill up long before 150.
	// Each event waits until the reading client has the one before (pace), as the 1 s ticks would allow.
	src := &fakeSource{ok: true, pace: make(chan struct{}, 1)}
	for i := 0; i < 40; i++ {
		src.apps = append(src.apps, protocol.ActivityApp{Key: fmt.Sprint(i), Kind: protocol.KindApp,
			Name: strings.Repeat("x", 250), Exe: strings.Repeat("y", 250), Lane: protocol.LaneVPN})
	}
	src.pace <- struct{}{}
	go s.RunActivity(src, time.Millisecond)
	finished := make(chan string, 1)
	go func() {
		for i := 0; i < 150; i++ {
			reading.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			if _, err := reading.r.ReadString('\n'); err != nil {
				finished <- fmt.Sprintf("the reading client got no event %d: %v", i, err)
				return
			}
			select {
			case src.pace <- struct{}{}:
			default:
			}
		}
		finished <- ""
	}()
	select {
	case msg := <-finished:
		if msg != "" {
			t.Fatal(msg)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("RunActivity blocked on a client that does not read")
	}
	if h.count() != 1 {
		t.Fatalf("clients %d: the stalled one must be dropped", h.count())
	}
}

// Spec 8.4: no event while the source has no summary (no core with the adapter runs); events come once
// it has one.
func TestNoActivityEventWhenSourceSaysNo(t *testing.T) {
	s, _, name := serve(t)
	c := dial(t, name)
	hello(t, c)
	c.send(t, `{"id":2,"method":"activity.subscribe"}`)
	src := &fakeSource{}
	go s.RunActivity(src, 10*time.Millisecond)
	deadline := time.Now().Add(5 * time.Second)
	for len(src.asked()) < 5 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if n := len(src.asked()); n < 5 {
		t.Fatalf("the source was asked %d times", n)
	}
	if m := c.send(t, `{"id":3,"method":"state.get"}`); m["event"] != nil || m["id"] == nil {
		t.Fatalf("an event without a summary: %v", m)
	}
	src.set(true)
	c.activity(t)
}
