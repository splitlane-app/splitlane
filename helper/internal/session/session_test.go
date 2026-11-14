package session

import (
	"sync/atomic"
	"testing"
	"time"
)

// The hidden window answers WM_QUERYENDSESSION with "yes" and calls the handler on WM_ENDSESSION
// with wParam TRUE only, before the message returns.
func TestEndSessionCallsTheHandler(t *testing.T) {
	var calls atomic.Int32
	w, err := Watch(func() { calls.Add(1) }, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	if r := w.send(wmQueryEndSession, 0, 0); r != 1 {
		t.Fatalf("WM_QUERYENDSESSION answered %d", r)
	}
	w.send(wmEndSession, 0, 0) // the session does not end after all
	if calls.Load() != 0 {
		t.Fatal("WM_ENDSESSION with FALSE must not call the handler")
	}
	w.send(wmEndSession, 1, 0)
	if calls.Load() != 1 {
		t.Fatal("WM_ENDSESSION with TRUE must call the handler before it returns")
	}
}

func TestTwoWatchersAndStop(t *testing.T) {
	a, err := Watch(func() {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Watch(func() {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	a.Stop()
	b.Stop()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		_, okA := handlers.Load(a.hwnd)
		_, okB := handlers.Load(b.hwnd)
		if !okA && !okB {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("windows not destroyed after Stop")
}
