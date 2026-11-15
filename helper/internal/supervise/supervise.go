// Package supervise is the supervisor of the product helper (spec 4): Task Scheduler does not restart a
// task whose program exits with a non-zero code, so the process the task starts runs the helper again
// as a worker and starts it anew after a failure.
package supervise

import (
	"fmt"
	"time"
)

const (
	Pause        = 5 * time.Second  // before a worker is started again
	Window       = 10 * time.Minute // the restarts are counted within it
	MaxRestarts  = 3                // at most so many restarts within Window
	exitInternal = 5                // the exit code of the helper for an internal error
)

// Final reports whether a worker's exit code is an answer, not a failure: OK, the pipe taken, a bad
// core, not installed, refused. The supervisor ends with the same code.
func Final(code int) bool {
	switch code {
	case 0, 3, 4, 6, 7:
		return true
	}
	return false
}

// Decide reports whether to start the worker again after it ended with code: not for a final code, and
// only while fewer than MaxRestarts restarts fall in (now-Window, now].
func Decide(code int, restarts []time.Time, now time.Time) bool {
	if Final(code) {
		return false
	}
	return len(within(restarts, now)) < MaxRestarts
}

func within(restarts []time.Time, now time.Time) []time.Time {
	var out []time.Time
	for _, t := range restarts {
		if t.After(now.Add(-Window)) && !t.After(now) {
			out = append(out, t)
		}
	}
	return out
}

// Worker is a running worker; Wait returns its exit code (a crash code as the unsigned number).
type Worker interface{ Wait() int }

// Clock is the time of the supervisor; the tests replace it.
type Clock interface {
	Now() time.Time
	After(time.Duration) <-chan time.Time
}

// codeText is an exit code for the log: a crash code (NTSTATUS, 0x80000000 and up) in hex, any other in decimal.
func codeText(code int) string {
	if code >= 0x80000000 {
		return fmt.Sprintf("0x%X", code)
	}
	return fmt.Sprint(code)
}

// Run starts the worker, waits for it and decides, until a final code or too many failures. It returns
// the exit code of the supervisor: the worker's last code, or 5 when the worker cannot be started. start
// gets restarted=false for the first worker and true for every one after a failure.
func Run(start func(restarted bool) (Worker, error), c Clock, logf func(format string, args ...any)) int {
	var restarts []time.Time
	for {
		w, err := start(len(restarts) > 0)
		if err != nil {
			logf("supervisor: cannot start the helper: %v", err)
			return exitInternal
		}
		code := w.Wait()
		now := c.Now()
		switch {
		case Final(code):
			logf("supervisor: the helper ended with code %s; not starting it again", codeText(code))
			return code
		case !Decide(code, restarts, now):
			logf("supervisor: the helper ended with code %s %d times within %s; giving up", codeText(code), MaxRestarts, windowText)
			return code
		}
		restarts = append(within(restarts, now), now)
		logf("supervisor: the helper ended with code %s; starting it again in %s (restart %d of %d in %s)", codeText(code), Pause, len(restarts), MaxRestarts, windowText)
		<-c.After(Pause)
	}
}

// windowText is Window as the log writes it.
var windowText = fmt.Sprintf("%dm", int(Window/time.Minute))
