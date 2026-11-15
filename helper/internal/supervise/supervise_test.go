package supervise

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeWorker struct{ code int }

func (w fakeWorker) Wait() int { return w.code }

// fakeClock: virtual time that moves only by the pauses asked for.
type fakeClock struct {
	now    time.Time
	pauses []time.Duration
}

func (c *fakeClock) Now() time.Time { return c.now }
func (c *fakeClock) After(d time.Duration) <-chan time.Time {
	c.pauses = append(c.pauses, d)
	c.now = c.now.Add(d)
	ch := make(chan time.Time, 1)
	ch <- c.now
	return ch
}

// script starts workers that end with the given codes in turn; it counts the starts.
type script struct {
	codes  []int
	starts int
	err    error
}

func (s *script) start() (Worker, error) {
	if s.err != nil {
		return nil, s.err
	}
	s.starts++
	return fakeWorker{s.codes[s.starts-1]}, nil
}

func newClock() *fakeClock { return &fakeClock{now: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)} }

type lines struct{ got []string }

func (l *lines) logf(format string, a ...any) { l.got = append(l.got, fmt.Sprintf(format, a...)) }

func TestFinalCodes(t *testing.T) {
	for code, want := range map[int]bool{0: true, 3: true, 4: true, 6: true, 7: true, 1: false, 2: false, 5: false, -1: false, 0xC0000005: false} {
		if Final(code) != want {
			t.Errorf("Final(%d) = %v", code, !want)
		}
	}
}

func TestDecideRestartsAFailure(t *testing.T) {
	now := newClock().now
	if !Decide(2, nil, now) || !Decide(0xC0000005, []time.Time{now, now}, now) {
		t.Fatal("a failure with fewer than three restarts is restarted")
	}
	if Decide(3, nil, now) || Decide(0, nil, now) {
		t.Fatal("a final code is never restarted")
	}
	if Decide(2, []time.Time{now, now, now}, now) {
		t.Fatal("three restarts in the window stop it")
	}
}

func TestDecideForgetsOldRestarts(t *testing.T) {
	now := newClock().now
	old := now.Add(-11 * time.Minute)
	if !Decide(2, []time.Time{old, old, old}, now) {
		t.Fatal("restarts 11 minutes ago must not count")
	}
	if Decide(2, []time.Time{old, now.Add(-9 * time.Minute), now.Add(-time.Minute), now}, now) {
		t.Fatal("three recent restarts count")
	}
}

func TestRunExitsWithAFinalCode(t *testing.T) {
	for _, code := range []int{0, 3, 4, 6, 7} {
		s := &script{codes: []int{code}}
		c := newClock()
		if got := Run(s.start, c, (&lines{}).logf); got != code || s.starts != 1 || len(c.pauses) != 0 {
			t.Errorf("code %d: exit %d, %d starts, pauses %v", code, got, s.starts, c.pauses)
		}
	}
}

func TestRunRestartsAfterAFailureAndPause(t *testing.T) {
	s := &script{codes: []int{2, 0}}
	c := newClock()
	if got := Run(s.start, c, (&lines{}).logf); got != 0 || s.starts != 2 {
		t.Fatalf("exit %d, %d starts", got, s.starts)
	}
	if len(c.pauses) != 1 || c.pauses[0] != 5*time.Second {
		t.Fatalf("pauses %v", c.pauses)
	}
}

func TestRunStopsAfterThreeRestartsInTenMinutes(t *testing.T) {
	s := &script{codes: []int{2, 2, 2, 0xC0000005, 0}}
	c := newClock()
	if got := Run(s.start, c, (&lines{}).logf); got != 0xC0000005 || s.starts != 4 || len(c.pauses) != 3 {
		t.Fatalf("exit %d, %d starts, pauses %v", got, s.starts, c.pauses)
	}
}

func TestRunLogsEveryDecision(t *testing.T) {
	l := &lines{}
	Run((&script{codes: []int{2, 2, 2, 2}}).start, newClock(), l.logf)
	want := []string{
		"supervisor: the helper ended with code 2; starting it again in 5s (restart 1 of 3 in 10m)",
		"supervisor: the helper ended with code 2; starting it again in 5s (restart 2 of 3 in 10m)",
		"supervisor: the helper ended with code 2; starting it again in 5s (restart 3 of 3 in 10m)",
		"supervisor: the helper ended with code 2 3 times within 10m; giving up",
	}
	if strings.Join(l.got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("log:\n%s", strings.Join(l.got, "\n"))
	}
	l = &lines{}
	Run((&script{codes: []int{7}}).start, newClock(), l.logf)
	if len(l.got) != 1 || l.got[0] != "supervisor: the helper ended with code 7; not starting it again" {
		t.Fatalf("log: %q", l.got)
	}
}

func TestRunStartFailureIsInternal(t *testing.T) {
	l := &lines{}
	s := &script{err: errors.New("no such file")}
	if got := Run(s.start, newClock(), l.logf); got != 5 {
		t.Fatalf("exit %d", got)
	}
	if len(l.got) != 1 || !strings.Contains(l.got[0], "supervisor: cannot start the helper: no such file") {
		t.Fatalf("log: %q", l.got)
	}
}
