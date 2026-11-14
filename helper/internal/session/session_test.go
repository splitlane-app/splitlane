package session

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// The hidden window answers WM_QUERYENDSESSION with "yes" and calls the handler on WM_ENDSESSION
// with wParam TRUE only, before the message returns.
func TestEndSessionCallsTheHandler(t *testing.T) {
	var calls atomic.Int32
	w, err := Watch(Handlers{OnEnd: func() { calls.Add(1) }}, t.Logf)
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

// Section 10: the same hidden window gets WM_POWERBROADCAST. PBT_APMSUSPEND is "suspend"; the wake is
// "resume" once, although Windows sends PBT_APMRESUMEAUTOMATIC and, when the user is back,
// PBT_APMRESUMESUSPEND; other power events are not reported. Every one is answered TRUE.
func TestPowerBroadcastCallsTheHandler(t *testing.T) {
	var mu sync.Mutex
	var got []string
	w, err := Watch(Handlers{OnPower: func(event string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, event)
	}}, t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer w.Stop()
	const pbtPowerStatusChange = 0x000A
	for _, event := range []uintptr{pbtAPMSuspend, pbtAPMResumeAutomatic, pbtAPMResumeSuspend, pbtPowerStatusChange,
		pbtAPMSuspend, pbtAPMResumeSuspend} {
		if r := w.send(wmPowerBroadcast, event, 0); r != 1 {
			t.Fatalf("WM_POWERBROADCAST %#x answered %d", event, r)
		}
	}
	w.send(wmEndSession, 1, 0) // no OnEnd: nothing to call
	mu.Lock()
	defer mu.Unlock()
	if s := strings.Join(got, ","); s != "suspend,resume,suspend,resume" {
		t.Fatalf("events %s", s)
	}
}

func TestTwoWatchersAndStop(t *testing.T) {
	a, err := Watch(Handlers{OnEnd: func() {}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Watch(Handlers{OnEnd: func() {}}, nil)
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
