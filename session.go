package melody

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var ()

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
	if s.isClosed() {
		s.melody.errorHandler(s, ErrWriteClosed)
		return
	}

	select {
	case s.output <- message:
	default:
		s.melody.errorHandler(s, ErrMessageBufferFull)
	}
}

// Clsoe the session if exist
func (s *Session) Close() error {
	if s.isClosed() {
		return ErrSessionClosed
	}

	s.writeMessage(&envelope{t: websocket.CloseMessage, msg: []byte{}})

	return nil
}

func (s *Session) isClosed() bool {
	s.rwmutex.Lock()
	defer s.rwmutex.Unlock()

	return !s.open
}

func (s *Session) ping() {

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

}

func (s *Session) writeRaw(msg *envelope) error {
	return nil
}
