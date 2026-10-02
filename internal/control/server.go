package control

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"sync"
	"time"

	"github.com/sglavoie/muse/internal/safego"
)

// maxLineBytes bounds one request line, so a host that never writes a newline
// cannot grow muse's memory without limit.
const maxLineBytes = 1 << 20

// outboxDepth is how many lines a connection may have queued before events
// for it are dropped. Replies are never dropped: a request waits for its own.
const outboxDepth = 64

// DefaultReplyTimeout is how long a request waits for the TUI to answer before
// the host is told it did not.
const DefaultReplyTimeout = 10 * time.Second

// flushTimeout bounds how long Close spends writing the lines still queued for
// a connection, such as the quitting event, before it ends the connection.
const flushTimeout = time.Second

// quittingMessage is the refusal a request gets once Drain has run.
const quittingMessage = "muse is quitting"

// Dispatch hands one request to the TUI and returns the channel its reply will
// arrive on. It must not block: the TUI answers from inside its own Update.
type Dispatch func(Request) <-chan Reply

// Server listens on one Unix socket for the life of a TUI session. Listen
// opens the socket and Serve starts accepting, so the server can be handed to
// the model as its event publisher before the program that dispatches requests
// exists.
type Server struct {
	path         string
	listener     net.Listener
	replyTimeout time.Duration

	mu        sync.Mutex
	dispatch  Dispatch
	conns     map[*conn]struct{}
	hello     []byte
	lastState []byte
	quitting  []byte
	serving   bool
	closed    bool
	wg        sync.WaitGroup
}

type conn struct {
	net.Conn
	out      chan []byte
	done     chan struct{}
	stop     chan struct{}
	once     sync.Once
	stopOnce sync.Once
}

func (c *conn) close() {
	c.once.Do(func() {
		close(c.done)
		_ = c.Close()
	})
}

// requestStop asks the writer to flush what is queued and then end the
// connection. The write deadline it sets also interrupts a write already
// blocked on a host that stopped reading, so the flush ends within
// flushTimeout either way.
func (c *conn) requestStop() {
	c.stopOnce.Do(func() {
		_ = c.SetWriteDeadline(time.Now().Add(flushTimeout))
		close(c.stop)
	})
}

// Listen opens the socket at path, replacing a stale file a crashed session
// left there. Only the user who runs muse may connect. Nothing is accepted
// until Serve.
func Listen(path string) (*Server, error) {
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return nil, fmt.Errorf("control: %s exists and is not a socket", path)
		}
		if err := os.Remove(path); err != nil {
			return nil, fmt.Errorf("control: remove stale socket: %w", err)
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("control: listen on %s: %w", path, err)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(path)
		return nil, fmt.Errorf("control: restrict %s: %w", path, err)
	}
	return &Server{
		path:         path,
		listener:     listener,
		replyTimeout: DefaultReplyTimeout,
		conns:        map[*conn]struct{}{},
	}, nil
}

// Path is the socket's path.
func (s *Server) Path() string { return s.path }

// Serve starts accepting connections, handing each request to dispatch. It
// returns at once; the server runs until Close.
func (s *Server) Serve(dispatch Dispatch) error {
	if dispatch == nil {
		return errors.New("control: a dispatch function is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch {
	case s.closed:
		return errors.New("control: server is closed")
	case s.serving:
		return errors.New("control: server is already serving")
	}
	s.dispatch = dispatch
	s.serving = true
	s.wg.Add(1)
	safego.Go("control.accept", s.accept)
	return nil
}

// SetHello sets the hello event every new connection hears first.
func (s *Server) SetHello(hello Hello) {
	line, ok := encodeEvent(Event{Event: EventHello, Hello: &hello})
	if !ok {
		return
	}
	s.mu.Lock()
	s.hello = line
	s.mu.Unlock()
}

// Publish sends an event to every connection without blocking: a connection
// whose queue is full misses it. A state event is also remembered for the next
// connection, so it may be published before Serve.
func (s *Server) Publish(event Event) {
	line, ok := encodeEvent(event)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return
	}
	if event.Event == EventState {
		s.lastState = line
	}
	s.broadcastLocked(event.Event, line)
}

// Drain refuses every request from now on and tells each connection muse is
// quitting. A connection that arrives later hears it too. It is idempotent.
func (s *Server) Drain() {
	line, ok := encodeEvent(Event{Event: EventQuitting})
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.quitting != nil {
		return
	}
	s.quitting = line
	s.broadcastLocked(EventQuitting, line)
}

func (s *Server) broadcastLocked(name string, line []byte) {
	for c := range s.conns {
		select {
		case c.out <- line:
		default:
			slog.Warn("control: dropped an event for a slow host", "event", name)
		}
	}
}

// Close stops accepting, writes what is still queued for each connection,
// ends every connection, and removes the socket. It is idempotent.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	conns := make([]*conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()

	err := s.listener.Close()
	for _, c := range conns {
		c.requestStop()
	}
	s.wg.Wait()
	if rmErr := os.Remove(s.path); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) && err == nil {
		err = rmErr
	}
	return err
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		raw, err := s.listener.Accept()
		if err != nil {
			return
		}
		if !s.register(raw) {
			return
		}
	}
}

// register adds a fresh connection, queues its replay, and starts its reader
// and writer. It reports false once the server is closed.
func (s *Server) register(raw net.Conn) bool {
	c := &conn{
		Conn: raw,
		out:  make(chan []byte, outboxDepth),
		done: make(chan struct{}),
		stop: make(chan struct{}),
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		c.close()
		return false
	}
	s.conns[c] = struct{}{}
	for _, line := range [][]byte{s.hello, s.lastState, s.quitting} {
		if line != nil {
			c.out <- line
		}
	}
	s.wg.Add(2)
	s.mu.Unlock()
	safego.Go("control.write", func() { s.write(c) })
	safego.Go("control.read", func() { s.read(c) })
	return true
}

func (s *Server) drop(c *conn) {
	s.mu.Lock()
	delete(s.conns, c)
	s.mu.Unlock()
	c.close()
}

func (s *Server) write(c *conn) {
	defer s.wg.Done()
	for {
		select {
		case <-c.done:
			return
		case <-c.stop:
			s.flush(c)
			s.drop(c)
			return
		case line := <-c.out:
			if _, err := c.Write(line); err != nil {
				s.drop(c)
				return
			}
		}
	}
}

// flush writes the lines already queued for c, bounded by the write deadline
// requestStop set.
func (s *Server) flush(c *conn) {
	for {
		select {
		case line := <-c.out:
			if _, err := c.Write(line); err != nil {
				return
			}
		default:
			return
		}
	}
}

func (s *Server) read(c *conn) {
	defer s.wg.Done()
	defer s.drop(c)
	scanner := bufio.NewScanner(c)
	scanner.Buffer(make([]byte, 0, 4096), maxLineBytes)
	for scanner.Scan() {
		if len(scanner.Bytes()) == 0 {
			continue
		}
		var req Request
		if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
			s.send(c, Reply{OK: false, Message: "malformed request: " + err.Error()})
			continue
		}
		s.send(c, s.answer(req, c.done))
	}
}

// answer waits for the TUI's reply to req, bounded by the reply timeout and by
// the connection ending. Once Drain has run the TUI is not asked at all.
func (s *Server) answer(req Request, done <-chan struct{}) Reply {
	s.mu.Lock()
	dispatch, quitting := s.dispatch, s.quitting != nil
	s.mu.Unlock()
	if quitting {
		return Reply{ID: req.ID, OK: false, Message: quittingMessage}
	}
	replies := dispatch(req)
	timer := time.NewTimer(s.replyTimeout)
	defer timer.Stop()
	select {
	case reply := <-replies:
		reply.ID = req.ID
		return reply
	case <-timer.C:
		return Reply{ID: req.ID, OK: false, Message: "muse did not answer in time"}
	case <-done:
		return Reply{ID: req.ID}
	}
}

// send queues a reply, waiting for room rather than dropping it.
func (s *Server) send(c *conn, reply Reply) {
	line, err := json.Marshal(reply)
	if err != nil {
		slog.Warn("control: encode reply", "err", err)
		return
	}
	select {
	case c.out <- append(line, '\n'):
	case <-c.done:
	}
}

func encodeEvent(event Event) ([]byte, bool) {
	line, err := json.Marshal(event)
	if err != nil {
		slog.Warn("control: encode event", "event", event.Event, "err", err)
		return nil, false
	}
	return append(line, '\n'), true
}
