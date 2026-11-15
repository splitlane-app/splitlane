// Package pipe is the named pipe of spec 5.1: JSON lines, hello first, events to every client.
package pipe

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"

	"splitlane/helper/internal/protocol"
)

// DefaultName is the pipe of the product.
const DefaultName = `\\.\pipe\SplitLane.Helper`

// Handler is what the server serves (the engine).
type Handler interface {
	Handle(ctx context.Context, method string, params json.RawMessage) (any, *protocol.Error)
	Hello() protocol.HelloResult
	SetClients(n int)
}

// SDDL gives read and write access to SYSTEM, the administrators and the setup user only (spec 5.1, 11).
// "P" protects the list from inherited entries.
func SDDL(userSID string) string {
	return "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;" + userSID + ")"
}

// Server serves one pipe name.
type Server struct {
	h         Handler
	l         net.Listener
	log       func(format string, args ...any)
	check     Check // nil: every client is served
	mu        sync.Mutex
	clients   map[*client]bool
	wg        sync.WaitGroup // the reader and writer of every client
	answering sync.WaitGroup // requests between their arrival and their queued answer
	closing   bool
	done      chan struct{} // closed by the first Close: RunActivity returns
}

// closeWait bounds each wait of Close: answers in flight, then the queues written out.
const closeWait = time.Second

type client struct {
	conn       net.Conn
	out        chan []byte
	ended      bool   // out is closed: nothing more is queued (s.mu)
	hello      bool   // a hello with the right protocol came
	mismatch   bool   // a hello with another protocol came: only helper.shutdown is accepted
	subscribed bool   // activity.subscribe came: the activity events go to this client (s.mu)
	detailFor  string // the detailFor of the subscription, "" for none (s.mu)
}

// Listen creates the pipe. It fails when the pipe already exists: another helper runs. A client
// that check refuses is closed without an answer; a nil check serves everyone (dev mode, tests).
func Listen(name, userSID string, h Handler, log func(string, ...any), check Check) (*Server, error) {
	l, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: SDDL(userSID),
		InputBufferSize:    64 * 1024,
		OutputBufferSize:   64 * 1024,
	})
	if err != nil {
		return nil, err
	}
	return newServer(l, h, log, check), nil
}

// newServer serves the connections of l; the tests give it a listener of their own.
func newServer(l net.Listener, h Handler, log func(string, ...any), check Check) *Server {
	if log == nil {
		log = func(string, ...any) {}
	}
	return &Server{h: h, l: l, log: log, check: check, clients: map[*client]bool{}, done: make(chan struct{})}
}

// acceptPause is the wait before Accept is tried again after an error.
const acceptPause = time.Second

// Serve accepts clients until Close and then returns the listener's error (net.ErrClosed). Any
// other Accept error (go-winio keeps its listener usable after it, for example after a failed
// CreateNamedPipe) is logged and Accept is tried again after a pause: one rare failure must not
// leave the helper holding the pipe name without taking new windows.
func (s *Server) Serve() error {
	for {
		conn, err := s.l.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) || s.isClosing() {
				return err
			}
			s.log("pipe: accept failed: %v; trying again in %s", err, acceptPause)
			time.Sleep(acceptPause)
			continue
		}
		c := &client{conn: conn, out: make(chan []byte, 64)}
		s.wg.Add(1)
		go s.admit(c)
	}
}

// admit runs the check in the goroutine of the client, before its first line is read: the Accept
// loop never waits for it. A refused client is closed and is never counted.
func (s *Server) admit(c *client) {
	if s.check != nil {
		if err := s.check(c.conn); err != nil {
			s.log("pipe: refused a client: %v", err)
			c.conn.Close()
			s.wg.Done()
			return
		}
	}
	s.mu.Lock()
	if s.closing { // Close came while the check ran: nobody is served any more
		s.mu.Unlock()
		c.conn.Close()
		s.wg.Done()
		return
	}
	s.wg.Add(1)
	s.mu.Unlock()
	go s.write(c)
	s.read(c)
}

// Close stops accepting and the activity events, lets the answers in flight reach their queues (the
// answer to helper.shutdown among them: the helper exits right after Close), writes every queue out
// and closes the clients. Each wait is bounded; a client that does not read is closed anyway.
func (s *Server) Close() {
	s.l.Close()
	s.mu.Lock()
	if !s.closing {
		close(s.done)
	}
	s.closing = true
	s.mu.Unlock()
	waitFor(&s.answering, closeWait)
	s.mu.Lock()
	var conns []net.Conn
	for c := range s.clients {
		conns = append(conns, c.conn)
		c.conn.SetWriteDeadline(time.Now().Add(closeWait))
		s.end(c, false)
	}
	s.mu.Unlock()
	if !waitFor(&s.wg, 2*closeWait) {
		for _, conn := range conns {
			conn.Close()
		}
	}
}

func (s *Server) isClosing() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closing
}

// waitFor waits for wg at most d; false when the time ran out.
func waitFor(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// Broadcast sends the state event to every client that said hello (spec 5.1).
func (s *Server) Broadcast(st protocol.State) {
	line := protocol.EncodeEvent("state", st)
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.clients {
		if c.hello {
			s.send(c, line)
		}
	}
}

// send queues a line; a client that does not read is dropped instead of blocking the helper.
// s.mu must be held.
func (s *Server) send(c *client, line []byte) {
	if c.ended {
		return
	}
	select {
	case c.out <- line:
	default:
		s.log("pipe: a client does not read its messages; dropping it")
		s.drop(c)
	}
}

// drop closes a client at once; s.mu must be held.
func (s *Server) drop(c *client) { s.end(c, true) }

// end closes the queue of a client once: at once with the connection (now), or after the writer
// has written what is queued (Close). s.mu must be held.
func (s *Server) end(c *client, now bool) {
	if !c.ended {
		c.ended = true
		close(c.out)
	}
	if now {
		c.conn.Close()
	}
	if s.clients[c] {
		delete(s.clients, c)
		s.h.SetClients(s.countHello())
	}
}

func (s *Server) countHello() int {
	n := 0
	for c := range s.clients {
		if c.hello {
			n++
		}
	}
	return n
}

func (s *Server) write(c *client) {
	defer s.wg.Done()
	defer c.conn.Close() // the queue was ended: everything in it is written
	for line := range c.out {
		if _, err := c.conn.Write(line); err != nil {
			s.mu.Lock()
			s.drop(c)
			s.mu.Unlock()
			return
		}
	}
}

func (s *Server) reply(c *client, line []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.clients[c] {
		s.send(c, line)
	}
}

func (s *Server) read(c *client) {
	defer s.wg.Done()
	s.mu.Lock()
	s.clients[c] = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.end(c, !s.closing) // while closing, the writer writes the queue out first
		s.mu.Unlock()
	}()
	sc := protocol.NewScanner(c.conn)
	for sc.Scan() {
		s.mu.Lock()
		if s.closing {
			s.mu.Unlock()
			return
		}
		s.answering.Add(1)
		s.mu.Unlock()
		var req protocol.Request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil || req.Method == "" {
			s.reply(c, protocol.EncodeError(req.ID, protocol.Errorf(protocol.CodeBadRequest, "not a request")))
		} else {
			s.reply(c, s.answer(c, req))
		}
		s.answering.Done()
	}
}

// answer applies the hello rules of 5.1, answers activity.subscribe itself (activity.go) and passes
// everything else to the handler.
func (s *Server) answer(c *client, req protocol.Request) []byte {
	s.mu.Lock()
	hello, mismatch := c.hello, c.mismatch
	s.mu.Unlock()
	if req.Method == protocol.MethodHello {
		var p protocol.HelloParams
		if err := protocol.DecodeParams(req.Params, &p); err != nil {
			return protocol.EncodeError(req.ID, err)
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if p.Protocol != protocol.Version {
			c.mismatch, c.hello = true, false
			s.h.SetClients(s.countHello())
			return protocol.EncodeError(req.ID, &protocol.Error{Code: protocol.CodeProtocolMismatch,
				Message: "the helper speaks another protocol version", Protocol: protocol.Version})
		}
		c.mismatch, c.hello = false, true
		s.h.SetClients(s.countHello())
		return protocol.EncodeResult(req.ID, s.h.Hello())
	}
	switch {
	case mismatch && req.Method != protocol.MethodShutdown:
		return protocol.EncodeError(req.ID, &protocol.Error{Code: protocol.CodeProtocolMismatch,
			Message: "only helper.shutdown is accepted after a protocol mismatch", Protocol: protocol.Version})
	case !hello && !mismatch:
		return protocol.EncodeError(req.ID, protocol.Errorf(protocol.CodeBadRequest, "the first message must be hello"))
	case req.Method == protocol.MethodActivitySubscribe:
		return s.subscribe(c, req)
	}
	res, err := s.h.Handle(context.Background(), req.Method, req.Params)
	if err != nil {
		return protocol.EncodeError(req.ID, err)
	}
	return protocol.EncodeResult(req.ID, res)
}
