package winhost

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"splitlane/helper/internal/coreconfig"
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

// A change of a route on the core's own adapter or on the system Tailscale's is not a change of the
// network: the helper's own steps (the TUN of 5.3, the exit node switched off in step 6) and the tailnet
// change them. The set is the one HasDefaultRoute leaves out; a change on any other adapter, or on one
// that cannot be read any more, counts.
func TestRouteChangeFilter(t *testing.T) {
	gone := errors.New("The system cannot find the file specified.")
	for _, c := range []struct {
		name  string
		i     Iface
		err   error
		count bool
	}{
		{"the core's adapter", Iface{Alias: coreconfig.AdapterName, Type: 53}, nil, false},
		{"the core's adapter, other case", Iface{Alias: "splitlane", Type: 53}, nil, false},
		{"the system Tailscale", Iface{Alias: "Tailscale", Type: 53}, nil, false},
		{"loopback", Iface{Alias: "Loopback Pseudo-Interface 1", Type: windows.IF_TYPE_SOFTWARE_LOOPBACK}, nil, false},
		{"Ethernet", Iface{Alias: "Ethernet", Type: windows.IF_TYPE_ETHERNET_CSMACD}, nil, true},
		{"Wi-Fi", Iface{Alias: "Wi-Fi", Type: windows.IF_TYPE_IEEE80211}, nil, true},
		{"WSL", Iface{Alias: "vEthernet (WSL)", Type: windows.IF_TYPE_ETHERNET_CSMACD}, nil, true},
		{"an adapter that is gone", Iface{}, gone, true},
	} {
		if got := routeCounts(c.i, c.err); got != c.count {
			t.Errorf("%s: counts %v, want %v", c.name, got, c.count)
		}
	}
}

// The callback of NotifyRouteChange2 wakes the watchers only for a change that counts: the routes the
// helper's own TUN and step 6 change right after phase on wake none (each would start a network change
// of section 10 again), a route on another adapter wakes them.
func TestRouteChangedSkipsOwnAdapters(t *testing.T) {
	ifaces := map[uint64]Iface{
		1: {LUID: 1, Alias: coreconfig.AdapterName, Type: 53},
		2: {LUID: 2, Alias: "Tailscale", Type: 53},
		3: {LUID: 3, Alias: "Ethernet", Type: windows.IF_TYPE_ETHERNET_CSMACD},
	}
	old := routeIface
	routeIface = func(luid uint64) (Iface, error) {
		if i, ok := ifaces[luid]; ok {
			return i, nil
		}
		return Iface{}, errors.New("The system cannot find the file specified.")
	}
	defer func() { routeIface = old }()
	w := &routeWatch{kicks: make(chan struct{}, 1), done: make(chan struct{})}
	routeWatchers.Store(w, struct{}{})
	defer routeWatchers.Delete(w)
	for _, c := range []struct {
		luid  uint64
		wakes bool
	}{{1, false}, {2, false}, {3, true}, {1, false}, {9, true}} {
		row := windows.MibIpForwardRow2{InterfaceLuid: c.luid}
		routeChanged(0, &row, 0)
		woke := false
		select {
		case <-w.kicks:
			woke = true
		default:
		}
		if woke != c.wakes {
			t.Fatalf("a change of a route on LUID %d (%q): woke the watchers %v, want %v", c.luid, ifaces[c.luid].Alias, woke, c.wakes)
		}
	}
}

// TestLiveRouteIfaceReadOnly reads every interface of Windows by its LUID as the callback does: the alias
// and the type are those of the adapter list. It changes nothing. It runs with SPLITLANE_LIVE_READONLY=1.
func TestLiveRouteIfaceReadOnly(t *testing.T) {
	if os.Getenv("SPLITLANE_LIVE_READONLY") != "1" {
		t.Skip("SPLITLANE_LIVE_READONLY is not set")
	}
	list, err := Interfaces()
	if err != nil || len(list) == 0 {
		t.Fatalf("interfaces: %d, %v", len(list), err)
	}
	for _, want := range list {
		got, err := ifaceOf(want.LUID)
		if err != nil || got.Alias != want.Alias || got.Type != want.Type {
			t.Fatalf("LUID %d: %+v %v, want alias %q type %d", want.LUID, got, err, want.Alias, want.Type)
		}
	}
	t.Logf("LIVE %d interfaces read by LUID", len(list))
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
