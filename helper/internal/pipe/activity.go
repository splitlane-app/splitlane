package pipe

import (
	"time"

	"splitlane/helper/internal/protocol"
)

// ActivitySource gives the data of the activity event (spec 8.4): the engine. false while there is
// nothing to send: no core with the adapter runs.
type ActivitySource interface {
	Activity(detailFor string) (protocol.Activity, bool)
}

// subscribe is activity.subscribe (5.1): the server keeps the subscription of each client itself, until
// the connection closes; a repeated one replaces it. Params that do not decode change nothing.
func (s *Server) subscribe(c *client, req protocol.Request) []byte {
	detailFor, err := protocol.DecodeSubscribe(req.Params)
	if err != nil {
		return protocol.EncodeError(req.ID, err)
	}
	s.mu.Lock()
	c.subscribed, c.detailFor = true, detailFor
	s.mu.Unlock()
	return protocol.EncodeResult(req.ID, struct{}{})
}

// RunActivity sends the activity event (5.1, 8.4) every period to each client that said hello and
// subscribed, until Close.
func (s *Server) RunActivity(src ActivitySource, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-s.done:
			return
		case <-t.C:
			s.sendActivity(src)
		}
	}
}

// sendActivity is one tick of RunActivity. src is asked once per distinct detailFor, outside the
// server's lock: the engine publishes its state under its own lock, and Broadcast takes this one.
// Nothing goes to a client whose source has no summary. The events go through the queue of the state
// events, so a client that does not read is dropped and the others go on (Review Focus 3).
func (s *Server) sendActivity(src ActivitySource) {
	s.mu.Lock()
	want := map[string]bool{}
	for c := range s.clients {
		if c.hello && c.subscribed {
			want[c.detailFor] = true
		}
	}
	s.mu.Unlock()
	lines := map[string][]byte{}
	for detailFor := range want {
		if a, ok := src.Activity(detailFor); ok {
			lines[detailFor] = protocol.EncodeEvent(protocol.EventActivity, a)
		}
	}
	if len(lines) == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		if line := lines[c.detailFor]; line != nil && c.hello && c.subscribed {
			s.send(c, line)
		}
	}
}
