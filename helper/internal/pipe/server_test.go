package pipe

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"splitlane/helper/internal/protocol"
)

type fakeHandler struct {
	mu      sync.Mutex
	methods []string
	clients int
	calls   []int // every SetClients

	// When hold is set, proxy.refresh plays a download: it sends on entered, then waits for hold to be
	// closed or for its context to end, which it reports on cancelled.
	hold      chan struct{}
	entered   chan struct{}
	cancelled chan struct{}
}

func (f *fakeHandler) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	f.mu.Lock()
	f.methods = append(f.methods, method)
	hold, entered, cancelled := f.hold, f.entered, f.cancelled
	f.mu.Unlock()
	switch {
	case method == "nope":
		return nil, protocol.Errorf(protocol.CodeUnknownMethod, "unknown")
	case method == protocol.MethodProxyRefresh && hold != nil:
		entered <- struct{}{}
		select {
		case <-hold:
		case <-ctx.Done():
			cancelled <- struct{}{}
			return nil, protocol.Errorf(protocol.CodeInternal, "cancelled")
		}
	}
	return map[string]string{"ok": method}, nil
}

// holdDownloads makes proxy.refresh wait (fakeHandler.hold); release lets the waiting ones go.
func (f *fakeHandler) holdDownloads() (entered, cancelled chan struct{}, release func()) {
	f.mu.Lock()
	defer f.mu.Unlock()
	hold := make(chan struct{}) // this call's own: a later holdDownloads must not change what release closes
	f.hold, f.entered, f.cancelled = hold, make(chan struct{}, 8), make(chan struct{}, 8)
	return f.entered, f.cancelled, sync.OnceFunc(func() { close(hold) })
}

func (f *fakeHandler) Hello() protocol.HelloResult {
	return protocol.HelloResult{Protocol: protocol.Version, Helper: "0.1.0", Core: "1.14.4"}
}

func (f *fakeHandler) SetClients(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.clients = n
	f.calls = append(f.calls, n)
}

func (f *fakeHandler) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.clients
}

func currentSID(t *testing.T) string {
	t.Helper()
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return u.User.Sid.String()
}

// serveOn starts a server with the log and the check on a pipe name of its own; the pipe disappears with
// the test.
func serveOn(t *testing.T, log func(string, ...any), check Check) (*Server, *fakeHandler, string) {
	t.Helper()
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
	h := &fakeHandler{}
	s, err := Listen(name, currentSID(t), h, log, check)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	t.Cleanup(s.Close)
	return s, h, name
}

// serve starts a server without a check.
func serve(t *testing.T) (*Server, *fakeHandler, string) {
	t.Helper()
	return serveOn(t, t.Logf, nil)
}

type testClient struct {
	conn net.Conn
	r    *bufio.Reader
}

func dial(t *testing.T, name string) *testClient {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := winio.DialPipeContext(ctx, name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	return &testClient{conn: conn, r: bufio.NewReader(conn)}
}

func (c *testClient) send(t *testing.T, line string) map[string]any {
	t.Helper()
	if _, err := c.conn.Write([]byte(line + "\n")); err != nil {
		t.Fatal(err)
	}
	return c.next(t)
}

func (c *testClient) next(t *testing.T) map[string]any {
	t.Helper()
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	text, err := c.r.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	json.Unmarshal([]byte(text), &m)
	return m
}

func errCode(m map[string]any) string {
	e, _ := m["error"].(map[string]any)
	code, _ := e["code"].(string)
	return code
}

func TestSDDL(t *testing.T) {
	if got := SDDL("S-1-5-21-1-2-3-1001"); got != "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;S-1-5-21-1-2-3-1001)" {
		t.Fatal(got)
	}
}

func TestHelloFirst(t *testing.T) {
	_, h, name := serve(t)
	c := dial(t, name)
	if code := errCode(c.send(t, `{"id":1,"method":"state.get"}`)); code != protocol.CodeBadRequest {
		t.Fatalf("before hello: %s", code)
	}
	m := c.send(t, `{"id":2,"method":"hello","params":{"protocol":3}}`)
	res, _ := m["result"].(map[string]any)
	if m["id"].(float64) != 2 || res["core"] != "1.14.4" || res["protocol"].(float64) != protocol.Version {
		t.Fatalf("hello: %v", m)
	}
	if code := errCode(c.send(t, `{"id":3,"method":"nope"}`)); code != protocol.CodeUnknownMethod {
		t.Fatalf("unknown method: %s", code)
	}
	if code := errCode(c.send(t, `not json`)); code != protocol.CodeBadRequest {
		t.Fatalf("garbage: %s", code)
	}
	if h.count() != 1 {
		t.Fatalf("clients %d", h.count())
	}
}

// Stage 3 part 1: proxy.add and proxy.refresh may download a subscription for up to 20 s, and
// diagnostics.export writes an archive: they are answered in a goroutine of their own, so the requests that
// come after one on the same connection (routing.disable, "Повторить") are answered at once; the answers
// are matched by id. The download is given up when its client goes and when the helper closes the pipe.
func TestLongRequestsAreAnsweredConcurrently(t *testing.T) {
	s, h, name := serve(t)
	entered, cancelled, release := h.holdDownloads()
	defer release()
	c := dial(t, name)
	c.send(t, `{"id":1,"method":"hello","params":{"protocol":3}}`)
	c.conn.Write([]byte(`{"id":2,"method":"proxy.refresh","params":{"id":"sub-0123456789abcdef"}}` + "\n"))
	<-entered
	if m := c.send(t, `{"id":3,"method":"routing.disable"}`); m["id"].(float64) != 3 || m["result"] == nil {
		t.Fatalf("routing.disable behind the download: %v", m)
	}
	if m := c.send(t, `{"id":4,"method":"diagnostics.export"}`); m["id"].(float64) != 4 {
		t.Fatalf("diagnostics.export: %v", m)
	}
	release()
	if m := c.next(t); m["id"].(float64) != 2 || m["result"] == nil {
		t.Fatalf("the answer of the download: %v", m)
	}

	entered, cancelled, release = h.holdDownloads()
	defer release()
	gone := dial(t, name)
	gone.send(t, `{"id":1,"method":"hello","params":{"protocol":3}}`)
	gone.conn.Write([]byte(`{"id":2,"method":"proxy.add","params":{"text":"https://example.com/s"}}` + "\n"))
	gone.conn.Write([]byte(`{"id":3,"method":"proxy.refresh","params":{"id":"sub-0123456789abcdef"}}` + "\n"))
	<-entered
	gone.conn.Close()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the download of a client that went is not given up")
	}

	entered, cancelled, _ = h.holdDownloads()
	last := dial(t, name)
	last.send(t, `{"id":1,"method":"hello","params":{"protocol":3}}`)
	last.conn.Write([]byte(`{"id":2,"method":"proxy.refresh","params":{"id":"sub-0123456789abcdef"}}` + "\n"))
	<-entered
	s.Close()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("Close does not give the download up")
	}
}

// The hello rules hold for the methods answered concurrently too, in the order of the lines: before a hello
// "the first message must be hello", after a mismatch protocol_mismatch; the handler never sees them.
func TestConcurrentMethodsWaitForHello(t *testing.T) {
	_, h, name := serve(t)
	c := dial(t, name)
	for i, m := range []string{protocol.MethodProxyAdd, protocol.MethodProxyRefresh, protocol.MethodDiagnosticsExport} {
		line := fmt.Sprintf(`{"id":%d,"method":"%s","params":{}}`, i+1, m)
		if r := c.send(t, line); errCode(r) != protocol.CodeBadRequest || !strings.Contains(fmt.Sprint(r["error"]), "the first message must be hello") {
			t.Fatalf("%s before hello: %v", m, r)
		}
	}
	c.send(t, `{"id":9,"method":"hello","params":{"protocol":1}}`)
	if r := c.send(t, `{"id":10,"method":"proxy.add","params":{"text":"x"}}`); errCode(r) != protocol.CodeProtocolMismatch {
		t.Fatalf("after a mismatch: %v", r)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.methods) != 0 {
		t.Fatalf("the handler was called: %v", h.methods)
	}
}

// After a protocol mismatch only helper.shutdown is accepted; the error carries the helper's version.
func TestProtocolMismatch(t *testing.T) {
	_, h, name := serve(t)
	c := dial(t, name)
	m := c.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`) // the window of stage 2
	e, _ := m["error"].(map[string]any)
	if errCode(m) != protocol.CodeProtocolMismatch || e["protocol"].(float64) != protocol.Version {
		t.Fatalf("mismatch: %v", m)
	}
	if code := errCode(c.send(t, `{"id":2,"method":"state.get"}`)); code != protocol.CodeProtocolMismatch {
		t.Fatalf("after mismatch: %s", code)
	}
	if m := c.send(t, `{"id":3,"method":"helper.shutdown"}`); m["result"] == nil {
		t.Fatalf("shutdown after mismatch: %v", m)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if strings.Join(h.methods, ",") != "helper.shutdown" {
		t.Fatalf("handler saw %v", h.methods)
	}
}

// Events go to every client that said hello; several clients at once (spec 5.1).
func TestBroadcastToEveryClient(t *testing.T) {
	s, h, name := serve(t)
	a, b, silent := dial(t, name), dial(t, name), dial(t, name)
	a.send(t, `{"id":1,"method":"hello","params":{"protocol":3}}`)
	b.send(t, `{"id":1,"method":"hello","params":{"protocol":3}}`)
	_ = silent
	deadline := time.Now().Add(5 * time.Second)
	for h.count() != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	s.Broadcast(protocol.State{Phase: protocol.PhaseOn, Apps: []protocol.AppRule{}})
	for _, c := range []*testClient{a, b} {
		m := c.next(t)
		data, _ := m["data"].(map[string]any)
		if m["event"] != "state" || data["phase"] != "on" {
			t.Fatalf("event %v", m)
		}
	}
	a.conn.Close()
	for h.count() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if h.count() != 1 {
		t.Fatalf("clients after a close: %d", h.count())
	}
}

// Review Focus 4: a window that hangs and stops reading is dropped; the helper does not block
// and the other clients keep getting events.
func TestStalledClientIsDropped(t *testing.T) {
	s, h, name := serve(t)
	stalled, live := dial(t, name), dial(t, name)
	stalled.conn.Write([]byte(`{"id":1,"method":"hello","params":{"protocol":3}}` + "\n")) // never reads again
	live.send(t, `{"id":1,"method":"hello","params":{"protocol":3}}`)
	big := protocol.State{Phase: protocol.PhaseOn}
	for i := 0; i < 40; i++ {
		big.Apps = append(big.Apps, protocol.AppRule{ID: fmt.Sprint(i), Name: strings.Repeat("x", 250), Exe: strings.Repeat("y", 250)})
	}
	got := make(chan struct{}, 1)
	go func() {
		for {
			live.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
			if _, err := live.r.ReadString('\n'); err != nil {
				close(got)
				return
			}
			got <- struct{}{}
		}
	}()
	// ~20 KB per event: the stalled client's pipe buffer (64 KB) and queue (64) fill up long before 150.
	// Events go out one at a time, as the helper sends them, so the reading client keeps up.
	finished := make(chan string, 1)
	go func() {
		for i := 0; i < 150; i++ {
			s.Broadcast(big)
			select {
			case _, ok := <-got:
				if !ok {
					finished <- fmt.Sprintf("the reading client lost the pipe after %d events", i)
					return
				}
			case <-time.After(5 * time.Second):
				finished <- fmt.Sprintf("the reading client got no event %d", i)
				return
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
		t.Fatal("Broadcast blocked on a client that does not read")
	}
	if h.count() != 1 {
		t.Fatalf("clients %d: the stalled one must be dropped", h.count())
	}
}

// exitOnShutdown is the helper's exit: on helper.shutdown it closes the server at once, while the
// answer is still on its way (cmd/splitlane-helper: OnExit, then Close).
type exitOnShutdown struct {
	fakeHandler
	srv *Server
}

func (h *exitOnShutdown) Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error) {
	if method == protocol.MethodShutdown {
		go h.srv.Close()
	}
	return h.fakeHandler.Handle(ctx, method, params)
}

// The answer to helper.shutdown reaches the client although the helper closes the pipe right
// away: Close writes the queues out before it closes the connections (slctl shutdown, Task 22, 23).
func TestShutdownReplyIsNotLost(t *testing.T) {
	for i := 0; i < 200; i++ {
		name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
		h := &exitOnShutdown{}
		s, err := Listen(name, currentSID(t), h, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		h.srv = s
		go s.Serve()
		c := dial(t, name)
		c.send(t, `{"id":1,"method":"hello","params":{"protocol":3}}`)
		c.conn.Write([]byte(`{"id":2,"method":"helper.shutdown"}` + "\n"))
		c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		text, err := c.r.ReadString('\n')
		if err != nil || !strings.Contains(text, `"id":2`) || !strings.Contains(text, `"result"`) {
			t.Fatalf("round %d: the answer was lost: %q %v", i, text, err)
		}
		if _, err := c.r.ReadString('\n'); err == nil {
			t.Fatalf("round %d: the pipe must close after the answer", i)
		}
		c.conn.Close()
		s.Close()
	}
}

// flakyListener fails its first Accept calls with errs, then hands out the connections sent on conns
// until it is closed, like go-winio's listener: an error other than net.ErrClosed leaves it usable.
type flakyListener struct {
	mu     sync.Mutex
	errs   []error
	conns  chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newFlakyListener(errs ...error) *flakyListener {
	return &flakyListener{errs: errs, conns: make(chan net.Conn), closed: make(chan struct{})}
}

func (l *flakyListener) Accept() (net.Conn, error) {
	l.mu.Lock()
	if len(l.errs) > 0 {
		err := l.errs[0]
		l.errs = l.errs[1:]
		l.mu.Unlock()
		return nil, err
	}
	l.mu.Unlock()
	select {
	case c := <-l.conns:
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}

func (l *flakyListener) Close() error {
	l.once.Do(func() { close(l.closed) })
	return nil
}

func (l *flakyListener) Addr() net.Addr { return &net.UnixAddr{Name: "flaky", Net: "unix"} }

// An Accept error that is not the close of the listener (a failed CreateNamedPipe) does not end the
// server: it is logged, Accept is tried again after a pause, the next client is served. Serve returns
// only after Close.
func TestAcceptErrorDoesNotStopServing(t *testing.T) {
	l := newFlakyListener(errors.New("CreateNamedPipe: All pipe instances are busy."))
	s := newServer(l, &fakeHandler{}, t.Logf, nil)
	served := make(chan error, 1)
	began := time.Now()
	go func() { served <- s.Serve() }()
	conn, peer := net.Pipe()
	t.Cleanup(func() { peer.Close() })
	select {
	case l.conns <- conn:
	case err := <-served:
		t.Fatalf("Serve returned after an accept error: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("Accept was not tried again after the error")
	}
	if waited := time.Since(began); waited < acceptPause*9/10 {
		t.Fatalf("Accept was tried again after %s, want a pause of %s", waited, acceptPause)
	}
	c := &testClient{conn: peer, r: bufio.NewReader(peer)}
	if m := c.send(t, `{"id":1,"method":"hello","params":{"protocol":3}}`); m["result"] == nil {
		t.Fatalf("the client after the error is not served: %v", m)
	}
	select {
	case err := <-served:
		t.Fatalf("Serve returned before Close: %v", err)
	default:
	}
	s.Close()
	select {
	case err := <-served:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("Serve ended with %v, want the close of the listener", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after Close")
	}
}

// A second helper cannot take the same pipe: one instance at a time.
func TestSecondListenFails(t *testing.T) {
	_, _, name := serve(t)
	if _, err := Listen(name, currentSID(t), &fakeHandler{}, nil, nil); err == nil {
		t.Fatal("the pipe name is taken")
	}
}

func (f *fakeHandler) setCalls() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.calls...)
}
