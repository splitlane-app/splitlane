package pipe

import (
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// Only the window itself is served without elevation: the other programs of the product folder (the core,
// elevate.exe, the uninstaller) run with any arguments a user gives them (final review, item 2).
func TestAllowedOnlyTheWindow(t *testing.T) {
	window := `C:\Program Files\SplitLane\SplitLane.exe`
	cases := []struct {
		image string
		want  bool
	}{
		{`C:\Program Files\SplitLane\SplitLane.exe`, true},
		{`c:\program files\splitlane\SPLITLANE.EXE`, true},
		{`C:\Program Files\SplitLane\.\SplitLane.exe`, true},
		{`C:\Program Files\SplitLane\helper\sing-box.exe`, false},
		{`C:\Program Files\SplitLane\helper\splitlane-helper.exe`, false},
		{`C:\Program Files\SplitLane\resources\elevate.exe`, false},
		{`C:\Program Files\SplitLane\Uninstall SplitLane.exe`, false},
		{`C:\Program Files\SplitLane\helper\SplitLane.exe`, false},
		{`C:\Program Files\SplitLane\SplitLane.exe\x.exe`, false},
		{`C:\Program Files\SplitLaneX\SplitLane.exe`, false},
		{`C:\Program Files\SplitLane`, false},
		{"C:\\Program F\u0130les\\SplitLane\\SplitLane.exe", false}, // U+0130: another folder on NTFS
		{"C:\\Program File\u017f\\SplitLane\\SplitLane.exe", false}, // U+017F folds to "s" in Go
		{`SplitLane\SplitLane.exe`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := Allowed(Caller{PID: 1, Image: c.image}, window); got != c.want {
			t.Errorf("Allowed(%q) = %v, want %v", c.image, got, c.want)
		}
	}
	// U+212A (Kelvin) folds to "k" in Go, yet is another letter on NTFS.
	if !Allowed(Caller{PID: 1, Image: `c:\work\kit\SplitLane.exe`}, `C:\Work\Kit\SplitLane.exe`) ||
		Allowed(Caller{PID: 1, Image: "C:\\Work\\\u212ait\\SplitLane.exe"}, `C:\Work\Kit\SplitLane.exe`) {
		t.Error("the Kelvin sign is not a K")
	}
	for _, w := range []string{"", "SplitLane.exe"} {
		if Allowed(Caller{PID: 1, Image: `SplitLane.exe`}, w) || Allowed(Caller{PID: 1, Image: `C:\x\SplitLane.exe`}, w) {
			t.Errorf("the window %q is not a full path: it allows nothing", w)
		}
	}
}

func TestAllowedElevated(t *testing.T) {
	if !Allowed(Caller{PID: 1, Image: `C:\Temp\Setup.exe`, Elevated: true}, `C:\Program Files\SplitLane\SplitLane.exe`) {
		t.Fatal("an elevated client is allowed from anywhere")
	}
	if Allowed(Caller{PID: 1, Image: `C:\Temp\Setup.exe`}, `C:\Program Files\SplitLane\SplitLane.exe`) {
		t.Fatal("a plain client outside the product folder is refused")
	}
}

// ProductCheck builds the window's path from the product folder: <productDir>\SplitLane.exe.
func TestProductCheckNamesTheWindow(t *testing.T) {
	if got := WindowExe(`C:\Program Files\SplitLane`); got != `C:\Program Files\SplitLane\SplitLane.exe` {
		t.Fatalf("WindowExe: %q", got)
	}
	if got := WindowExe(""); got != "" {
		t.Fatalf("WindowExe without a product folder: %q, want none", got)
	}
}

// accepted runs f on the server side of a test pipe connection from this very process, while a client
// is connected to it; an Accept that fails fails the test.
func accepted(t *testing.T, f func(conn net.Conn)) {
	t.Helper()
	name := fmt.Sprintf(`\\.\pipe\SplitLane.Test.%d`, rand.Int63())
	l, err := winio.ListenPipe(name, &winio.PipeConfig{SecurityDescriptor: SDDL(currentSID(t))})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	done := make(chan error, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			done <- err
			return
		}
		defer conn.Close()
		f(conn)
		done <- nil
	}()
	dial(t, name)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no client came")
	}
}

func ownExe(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestClientOfThisProcess(t *testing.T) {
	var got Caller
	var gotErr error
	accepted(t, func(conn net.Conn) { got, gotErr = ClientOf(conn) })
	if gotErr != nil {
		t.Fatal(gotErr)
	}
	if exe := ownExe(t); !strings.EqualFold(got.Image, exe) {
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

// This test exe plays the window: its own path is the window's.
func TestWindowCheckAdmitsTheWindow(t *testing.T) {
	var err error
	accepted(t, func(conn net.Conn) { err = windowCheck(ownExe(t))(conn) })
	if err != nil {
		t.Fatalf("the window is allowed: %v", err)
	}
}

// The folder of this test exe is the product folder: the test exe lies inside it, but it is not
// SplitLane.exe, so it is refused.
func TestProductCheckRefusesAnotherProgramOfTheFolder(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("the test process is elevated: it would be allowed")
	}
	var err error
	accepted(t, func(conn net.Conn) { err = ProductCheck(filepath.Dir(ownExe(t)))(conn) })
	if err == nil {
		t.Fatal("a program of the product folder other than the window is refused")
	}
}

func TestProductCheckRefusalNamesOnlyTheBaseName(t *testing.T) {
	if windows.GetCurrentProcessToken().IsElevated() {
		t.Skip("the test process is elevated: it would be allowed")
	}
	var err error
	accepted(t, func(conn net.Conn) { err = ProductCheck(`C:\nowhere\SplitLane`)(conn) })
	if err == nil {
		t.Fatal("refused expected")
	}
	want := fmt.Sprintf("not the SplitLane window: %s (process %d)", filepath.Base(ownExe(t)), os.Getpid())
	if !strings.EqualFold(err.Error(), want) {
		t.Fatalf("%q, want %q", err, want)
	}
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

func (l *logLines) all() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.lines...)
}

func (l *logLines) has(sub string) bool {
	for _, s := range l.all() {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// serveWith starts a server with the check; its log lines are kept.
func serveWith(t *testing.T, check Check) (*Server, *fakeHandler, string, *logLines) {
	t.Helper()
	ll := &logLines{}
	s, h, name := serveOn(t, ll.add, check)
	return s, h, name, ll
}

const helloLine = `{"id":1,"method":"hello","params":{"protocol":1}}` + "\n"

// expectEOF sends hello and waits for the end of the stream: a refused client gets no answer.
func expectEOF(t *testing.T, c *testClient) {
	t.Helper()
	c.conn.Write([]byte(helloLine))
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	line, err := c.r.ReadString('\n')
	if !errors.Is(err, io.EOF) {
		t.Fatalf("a refused client must see the end of the stream, got %q %v", line, err)
	}
}

func TestCheckErrorRejects(t *testing.T) {
	const refusal = "not the SplitLane window: evil.exe (process 7)"
	_, h, name, ll := serveWith(t, func(net.Conn) error { return errors.New(refusal) })
	expectEOF(t, dial(t, name))
	deadline := time.Now().Add(2 * time.Second)
	for !ll.has("pipe: refused a client: "+refusal) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !ll.has("pipe: refused a client: " + refusal) {
		t.Fatalf("log: %v", ll.all())
	}
	if calls := h.setCalls(); len(calls) != 0 {
		t.Fatalf("a refused client does not change SetClients: %v", calls)
	}
}

func TestRejectedClientDoesNotStopTheServer(t *testing.T) {
	var mu sync.Mutex
	n := 0
	_, _, name, _ := serveWith(t, func(net.Conn) error {
		mu.Lock()
		defer mu.Unlock()
		n++
		if n == 1 {
			return errors.New("refused")
		}
		return nil
	})
	expectEOF(t, dial(t, name))
	second := dial(t, name)
	m := second.send(t, `{"id":2,"method":"hello","params":{"protocol":1}}`)
	if _, ok := m["result"]; !ok {
		t.Fatalf("hello: %v", m)
	}
}

func TestNilCheckAcceptsAll(t *testing.T) {
	_, _, name, _ := serveWith(t, nil)
	c := dial(t, name)
	if m := c.send(t, `{"id":1,"method":"hello","params":{"protocol":1}}`); m["result"] == nil {
		t.Fatalf("hello: %v", m)
	}
}

// Close while a client is still in its check: the client is not served afterwards and Close returns.
func TestCloseDuringTheCheckServesNobody(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	s, h, name := serveOn(t, nil, func(net.Conn) error {
		close(entered)
		<-release
		return nil
	})
	c := dial(t, name)
	<-entered
	closed := make(chan struct{})
	go func() { s.Close(); close(closed) }()
	for deadline := time.Now().Add(5 * time.Second); !s.isClosing() && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
	}
	if !s.isClosing() {
		t.Fatal("Close did not start")
	}
	close(release)
	select {
	case <-closed:
	case <-time.After(5 * time.Second):
		t.Fatal("Close did not return")
	}
	c.conn.Write([]byte(helloLine))
	c.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if line, err := c.r.ReadString('\n'); !errors.Is(err, io.EOF) {
		t.Fatalf("the client was served after Close: %q %v", line, err)
	}
	if calls := h.setCalls(); len(calls) != 0 {
		t.Fatalf("SetClients: %v", calls)
	}
}
