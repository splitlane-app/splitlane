package winhost

import (
	"os"
	"testing"
	"time"
)

// A burst of route notifications is reported once, 2 s after its last notification; the times are
// those of a clock of the test.
func TestRouteDebounce(t *testing.T) {
	t0 := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	at := func(ms int) time.Time { return t0.Add(time.Duration(ms) * time.Millisecond) }
	d := debounce{quiet: routeQuiet}
	if _, ok := d.wait(t0); ok || d.over(t0) {
		t.Fatal("no notification yet: nothing to wait for, nothing to report")
	}
	d.note(at(0))
	d.note(at(500))
	d.note(at(1900))
	if w, ok := d.wait(at(2000)); !ok || w != 1900*time.Millisecond {
		t.Fatalf("2 s after the last notification: wait %s %v", w, ok)
	}
	if d.over(at(3899)) {
		t.Fatal("reported before 2 s passed since the last notification")
	}
	if !d.over(at(3900)) {
		t.Fatal("the burst is over 2 s after its last notification")
	}
	if _, ok := d.wait(at(5000)); ok || d.over(at(10000)) {
		t.Fatal("a burst is reported once")
	}
	d.note(at(20000))
	if w, ok := d.wait(at(25000)); !ok || w != 0 || !d.over(at(25000)) {
		t.Fatalf("a late look reports the next burst at once: wait %s %v", w, ok)
	}
}

// TestLiveRouteWatchReadOnly registers the route notification of Windows and cancels it, twice; it
// changes nothing. It runs with SPLITLANE_LIVE_READONLY=1.
func TestLiveRouteWatchReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	for i := 0; i < 2; i++ {
		stop, err := WatchRoutes(func() { t.Log("LIVE routes changed") }, t.Logf)
		if err != nil {
			t.Fatalf("watch %d: %v", i, err)
		}
		routeMu.Lock()
		registered := routeHandle != 0 && routeCount == 1
		routeMu.Unlock()
		stop()
		stop()
		routeMu.Lock()
		cancelled := routeHandle == 0 && routeCount == 0
		routeMu.Unlock()
		if !registered || !cancelled {
			t.Fatalf("watch %d: registered %v, cancelled %v", i, registered, cancelled)
		}
	}
	t.Log("LIVE route notification registered and cancelled twice")
}
