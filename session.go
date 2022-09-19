package melody

import (
	"net/http"
	"sync"

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
