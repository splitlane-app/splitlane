package pipe

import (
	"errors"
	"fmt"
	"math/rand"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func TestAllowedFolderBoundary(t *testing.T) {
	dir := `C:\Program Files\SplitLane`
	cases := []struct {
		image string
		want  bool
	}{
		{`C:\Program Files\SplitLane\SplitLane.exe`, true},
		{`C:\Program Files\SplitLane\helper\splitlane-helper.exe`, true},
		{`c:\program files\splitlane\SPLITLANE.EXE`, true},
		{`C:\Program Files\SplitLaneX\a.exe`, false},
		{`C:\Program Files\SplitLane`, false},
		{`SplitLane\SplitLane.exe`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := Allowed(Caller{PID: 1, Image: c.image}, dir); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v", c.image, got, c.want)
		}
	}
	if Allowed(Caller{PID: 1, Image: `C:\x\a.exe`}, "") {
		t.Error("an empty product folder allows nothing")
	}
}

func TestAllowedElevated(t *testing.T) {
	if !Allowed(Caller{PID: 1, Image: `C:\Temp\Setup.exe`, Elevated: true}, `C:\Program Files\SplitLane`) {
		t.Fatal("an elevated client is allowed from anywhere")
	}
	if Allowed(Caller{PID: 1, Image: `C:\Temp\Setup.exe`}, `C:\Program Files\SplitLane`) {
		t.Fatal("a plain client outside the product folder is refused")
	}
}

func TestClientOfThisProcess(t *testing.T) {
	var got Caller
	var gotErr error
	var mu sync.Mutex
	done := make(chan struct{})
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
	l, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: SDDL(currentSID(t))})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		mu.Lock()
		got, gotErr = ClientOf(conn)
		mu.Unlock()
		close(done)
	}()
	c := dial(t, name)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("no client")
	}
	_ = c
	mu.Lock()
	defer mu.Unlock()
	if gotErr != nil {
		t.Fatal(gotErr)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.EqualFold(got.Image, exe) {
		t.Errorf("image %q, want %q", got.Image, exe)
	}
	if got.PID != uint32(os.Getpid()) {
		t.Errorf("pid %d, want %d", got.PID, os.Getpid())
	}
}

type noFdConn struct{ net.Conn }

func TestClientOfWithoutFd(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	if _, err := ClientOf(noFdConn{a}); err == nil {
		t.Fatal("a connection without a handle is an error")
	}
}

func servePolicy(t *testing.T, check Check) (*Server, *fakeHandler, string, *logLines) {
	t.Helper()
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
	h := &fakeHandler{}
	ll := &logLines{}
	s, err := Listen(name, currentSID(t), h, ll.add, check)
	if err != nil {
		t.Fatal(err)
	}
	go s.Serve()
	t.Cleanup(s.Close)
	return s, h, name, ll
}

type logLines struct {
	mu    sync.Mutex
	lines []string
}

func (l *logLines) add(format string, a ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.lines = append(l.lines, fmt.Sprintf(format, a...))
}

func (l *logLines) has(sub string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, s := range l.lines {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func TestCheckErrorRejects(t *testing.T) {
	_, h, name, ll := servePolicy(t, func(net.Conn) error { return errors.New("not part of SplitLane: evil.exe (process 7)") })
	c := dial(t, name)
	c.conn.Write([]byte(`{"id":1,"method":"hello","params":{"protocol":1}}` + "\n"))
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := c.r.ReadString('\n'); err == nil {
		t.Fatal("a refused client must get no answer")
	}
	deadline := time.Now().Add(2 * time.Second)
	for !ll.has("pipe: refused a client: not part of SplitLane: evil.exe (process 7)") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !ll.has("pipe: refused a client: not part of SplitLane: evil.exe (process 7)") {
		t.Fatalf("log: %v", ll.lines)
	}
	if h.count() != 0 {
		t.Fatal("a refused client is not counted")
	}
}

func TestRejectedClientDoesNotStopTheServer(t *testing.T) {
	var mu sync.Mutex
	n := 0
	_, _, name, _ := servePolicy(t, func(net.Conn) error {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return errors.New("refused")
		}
		return nil
	})
	first := dial(t, name)
	first.conn.Write([]byte(`{"id":1,"method":"hello","params":{"protocol":1}}` + "\n"))
	first.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := first.r.ReadString('\n'); err == nil {
		t.Fatal("the first client was refused")
	}
	second := dial(t, name)
	m := second.send(t, `{"id":2,"method":"hello","params":{"protocol":1}}`)
	if _, ok := m["result"]; !ok {
		t.Fatalf("hello: %v", m)
	}
}

func TestNilCheckAcceptsAll(t *testing.T) {
	_, _, name, _ := servePolicy(t, nil)
	c := dial(t, name)
	if m := c.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`); m["result"] == nil {
		t.Fatalf("hello: %v", m)
	}
}

func TestProductCheckAllowsOwnProcessAsElevatedOrInside(t *testing.T) {
	// The test process is not inside a product folder named after nothing, so only elevation can admit it.
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
	l, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: SDDL(currentSID(t))})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	exe, _ := os.Executable()
	dir := exe[:strings.LastIndex(exe, `\`)]
	errs := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			errs <- err
			return
		}
		defer conn.Close()
		errs <- ProductCheck(dir)(conn)
	}()
	dial(t, name)
	if err := <-errs; err != nil {
		t.Fatalf("an image inside the folder is allowed: %v", err)
	}
}

func TestProductCheckRefusalNamesOnlyTheBaseName(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
	l, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: SDDL(currentSID(t))})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	errs := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			errs <- err
			return
		}
		defer conn.Close()
		errs <- ProductCheck(`C:\nowhere\SplitLane`)(conn)
	}()
	dial(t, name)
	err = <-errs
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("the test process is elevated: it would be allowed")
	}
	if err == nil {
		t.Fatal("refused expected")
	}
	exe, _ := os.Executable()
	base := exe[strings.LastIndex(exe, `\`)+1:]
	want := fmt.Sprintf("not part of SplitLane: %s (process %d)", base, os.Getpid())
	if !strings.EqualFold(err.Error(), want) {
		t.Fatalf("%q, want %q", err, want)
	}
}
