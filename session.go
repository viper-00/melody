package melody

import (
	"net/http"
	"sync"

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
