package winhost

import (
	"fmt"
	"sync"
	"time"

	"golang.org/x/sys/windows"
)

// routeQuiet: a burst of route notifications is over this long after its last notification.
const routeQuiet = 2 * time.Second

// debounce coalesces a burst of route notifications: it is over once quiet has passed since its last
// notification, and then it is reported once. It keeps no clock: the times come from the caller.
type debounce struct {
	quiet time.Duration
	last  time.Time // the last notification of the burst under way; zero when none is under way
}

// note records a notification at now.
func (d *debounce) note(now time.Time) { d.last = now }

// wait is how long after now the burst under way may be over; false when none is under way.
func (d *debounce) wait(now time.Time) (time.Duration, bool) {
	if d.last.IsZero() {
		return 0, false
	}
	return max(d.last.Add(d.quiet).Sub(now), 0), true
}

// over reports, once per burst, that the burst under way is over at now.
func (d *debounce) over(now time.Time) bool {
	if d.last.IsZero() || now.Before(d.last.Add(d.quiet)) {
		return false
	}
	d.last = time.Time{}
	return true
}

// One notification of route changes serves every watcher of the process: Windows calls routeChanged on
// a thread of its own, which only reads the interface of the route and wakes the watchers. NewCallback
// is called once: a process has only so many callbacks.
var (
	routeCallback = windows.NewCallback(routeChanged)
	routeMu       sync.Mutex     // registers and cancels the notification; the callback never takes it
	routeHandle   windows.Handle // the notification of NotifyRouteChange2; 0 while no watcher runs
	routeCount    int            // the watchers that use routeHandle
	routeWatchers sync.Map       // *routeWatch -> struct{}: every watcher the callback wakes
)

// routeIface reads the interface of a route as the decisions see it; the tests replace it.
var routeIface = ifaceOf

// ifaceOf reads the alias and the type of the interface with this LUID.
func ifaceOf(luid uint64) (Iface, error) {
	row := windows.MibIfRow2{InterfaceLuid: luid}
	if err := windows.GetIfEntry2Ex(windows.MibIfTableNormalWithoutStatistics, &row); err != nil {
		return Iface{}, err
	}
	return Iface{LUID: luid, Alias: windows.UTF16ToString(row.Alias[:]), Type: row.Type}, nil
}

// routeCounts: a change of a route on this interface is a change of the network (section 10). The routes
// of the core's own adapter and the system Tailscale's change with the helper's own steps (the TUN of
// 5.3, the exit node switched off in step 6) and with the tailnet, not with the network: they are left
// out, the set HasDefaultRoute leaves out. An interface that cannot be read (gone already) counts.
func routeCounts(i Iface, err error) bool { return err != nil || !ignored(i) }

// routeChanged is the callback of NotifyRouteChange2 (callerContext and the kind of change are not used):
// a change of a route, IPv4 or IPv6, that counts (routeCounts) wakes every watcher.
func routeChanged(callerContext uintptr, row *windows.MibIpForwardRow2, notificationType uintptr) uintptr {
	if row != nil && !routeCounts(routeIface(row.InterfaceLuid)) {
		return 0
	}
	routeWatchers.Range(func(w, _ any) bool {
		w.(*routeWatch).kick()
		return true
	})
	return 0
}

// routeWatch is one watcher: its goroutine (loop) turns the wakes into one call of onChange per burst.
type routeWatch struct {
	kicks chan struct{} // a notification came; one pending wake is enough
	done  chan struct{} // closed by stop
}

func (w *routeWatch) kick() {
	select {
	case w.kicks <- struct{}{}:
	default:
	}
}

func (w *routeWatch) loop(onChange func(), log func(string, ...any)) {
	d := debounce{quiet: routeQuiet}
	timer := time.NewTimer(routeQuiet)
	timer.Stop()
	defer timer.Stop()
	for {
		select {
		case <-w.done:
			return
		case <-w.kicks:
			d.note(time.Now())
		case <-timer.C:
			if d.over(time.Now()) {
				log("routes changed")
				onChange()
			}
		}
		if wait, ok := d.wait(time.Now()); ok {
			timer.Reset(wait)
		}
	}
}

// WatchRoutes calls onChange when the routes of Windows change (section 10): once per burst, 2 s after
// its last notification, on a goroutine of its own; changes on the core's own adapter and the system
// Tailscale's are left out (routeCounts). The notification is NotifyRouteChange2 of
// iphlpapi.dll for IPv4 and IPv6, without the initial notification; stop ends the watcher and, with the
// last watcher, cancels the notification (CancelMibChangeNotify2). stop may be called more than once.
// Only the notification is registered: nothing in the system changes.
func WatchRoutes(onChange func(), log func(string, ...any)) (stop func(), err error) {
	if log == nil {
		log = func(string, ...any) {}
	}
	w := &routeWatch{kicks: make(chan struct{}, 1), done: make(chan struct{})}
	routeMu.Lock()
	defer routeMu.Unlock()
	if routeCount == 0 {
		var h windows.Handle
		if err := windows.NotifyRouteChange2(windows.AF_UNSPEC, routeCallback, nil, false, &h); err != nil {
			return nil, fmt.Errorf("NotifyRouteChange2: %w", err)
		}
		routeHandle = h
	}
	routeCount++
	routeWatchers.Store(w, struct{}{})
	go w.loop(onChange, log)
	var once sync.Once
	return func() {
		once.Do(func() {
			routeWatchers.Delete(w)
			close(w.done)
			routeMu.Lock()
			defer routeMu.Unlock()
			routeCount--
			if routeCount == 0 {
				if err := windows.CancelMibChangeNotify2(routeHandle); err != nil {
					log("route watch: CancelMibChangeNotify2: %v", err)
				}
				routeHandle = 0
			}
		})
	}, nil
}
