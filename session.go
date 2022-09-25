package melody

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Session wrapper around websocket connections.
type Session struct {
	Request    *http.Request
	Keys       map[string]interface{}
	conn       *websocket.Conn
	output     chan *envelope
	outputDone chan struct{}
	melody     *Melody
	open       bool
	rwmutex    *sync.RWMutex
}

func (s *Session) writeMessage(message *envelope) {
	if s.closed() {
		s.melody.errorHandler(s, ErrWriteClosed)
		return
	}

	select {
	case s.output <- message:
	default:
		s.melody.errorHandler(s, ErrMessageBufferFull)
	}
}

// clsoe the session if exist
func (s *Session) close() {
	s.rwmutex.Lock()
	open := s.open
	s.open = false
	s.rwmutex.Unlock()
	if open {
		s.conn.Close()
		close(s.outputDone)
	}
}

func (s *Session) closed() bool {
	s.rwmutex.RLock()
	defer s.rwmutex.RUnlock()

	return !s.open
}

func (s *Session) ping() {
	s.writeRaw(&envelope{t: websocket.PingMessage, msg: []byte{}})
}

func (s *Session) writePump() {
	ticker := time.NewTicker(s.melody.Config.PingPeriod)
	defer ticker.Stop()

loop:
	for {
		select {
		case msg := <-s.output:
			err := s.writeRaw(msg)
			if err != nil {
				s.melody.errorHandler(s, err)
				break loop
			}

			// websocket message type: TextMessage, BinaryMessage, CloseMessage, PingMessage, PongMessage
			if msg.t == websocket.CloseMessage {
				break loop
			}

			if msg.t == websocket.TextMessage {
				s.melody.messageSentHandler(s, msg.msg)
			}

			if msg.t == websocket.BinaryMessage {
				s.melody.messageSentHandlerBinary(s, msg.msg)
			}
		case <-ticker.C:
			s.ping()
		case _, ok := <-s.outputDone:
			if !ok {
				break loop
			}
		}
	}
}

func (s *Session) readPump() {
	s.conn.SetReadLimit(s.melody.Config.MaxMessageSize)
	s.conn.SetReadDeadline(time.Now().Add(s.melody.Config.PongWait))

	s.conn.SetPongHandler(func(string) error {
		s.conn.SetReadDeadline(time.Now().Add(s.melody.Config.PongWait))
		s.melody.pongHandler(s)
		return nil
	})

	if s.melody.closeHandler != nil {
		s.conn.SetCloseHandler(func(code int, text string) error {
			return s.melody.closeHandler(s, code, text)
		})
	}

	for {

		t, message, err := s.conn.ReadMessage()
		if err != nil {
			s.melody.errorHandler(s, err)
			break
		}

		if t == websocket.TextMessage {
			s.melody.messageHandler(s, message)
		}

		if t == websocket.BinaryMessage {
			s.melody.messageHandlerBinary(s, message)
		}
	}
}

func (s *Session) writeRaw(msg *envelope) error {
	if s.closed() {
		return ErrWriteClosed
	}

	s.conn.SetWriteDeadline(time.Now().Add(s.melody.Config.WriteWait))

	err := s.conn.WriteMessage(msg.t, msg.msg)
	if err != nil {
		return err
	}

	return nil
}

// IsClosed returns the status of the connection.
func (s *Session) IsClosed() bool {
	return s.closed()
}

// Close the session.
func (s *Session) Close() error {
	if s.closed() {
		return ErrSessionClosed
	}

	s.writeMessage(&envelope{t: websocket.CloseMessage, msg: []byte{}})

	return nil
}
