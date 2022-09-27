package melody

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type handleMessageFunc func(*Session, []byte)
type filterFunc func(*Session) bool
type handleErrorFunc func(*Session, error)
type handleCloseFunc func(*Session, int, string) error
type handleSessionFunc func(*Session)

// Melody implements a websocket manager.
type Melody struct {
	Config                   *Config
	Upgrader                 *websocket.Upgrader
	messageHandler           handleMessageFunc
	messageHandlerBinary     handleMessageFunc
	messageSentHandler       handleMessageFunc
	messageSentHandlerBinary handleMessageFunc
	errorHandler             handleErrorFunc
	closeHandler             handleCloseFunc
	connectHandler           handleSessionFunc
	disconnectHandler        handleSessionFunc
	pongHandler              handleSessionFunc
	hub                      *hub
}

// New creates a new melody instance with default Upgrader and Config.
func New() *Melody {
	upgrader := &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return true },
	}

	hub := newHub()

	// running hub in the background
	go hub.run()

	return &Melody{
		Config:                   newConfig(),
		Upgrader:                 upgrader,
		messageHandler:           func(*Session, []byte) {},
		messageHandlerBinary:     func(*Session, []byte) {},
		messageSentHandler:       func(*Session, []byte) {},
		messageSentHandlerBinary: func(*Session, []byte) {},
		errorHandler:             func(*Session, error) {},
		closeHandler:             nil,
		connectHandler:           func(*Session) {},
		disconnectHandler:        func(*Session) {},
		pongHandler:              func(*Session) {},
		hub:                      hub,
	}
}

// HandleRequest upgrades http requests to websocket connections and dispatches them to be handled by the melody instance.
func (m *Melody) HandleRequest(w http.ResponseWriter, r *http.Request) error {
	return m.HandleRequestWithKeys(w, r, nil)
}

// HandleRequestWithKeys does the same as the HandleRequest but populates session.Keys with keys.
func (m *Melody) HandleRequestWithKeys(w http.ResponseWriter, r *http.Request, keys map[string]interface{}) error {
	if m.hub.closed() {
		return ErrClosed
	}

	conn, err := m.Upgrader.Upgrade(w, r, w.Header())
	if err != nil {
		return err
	}

	session := &Session{
		Request:    r,
		Keys:       keys,
		conn:       conn,
		output:     make(chan *envelope, m.Config.MessageBufferSize),
		outputDone: make(chan struct{}),
		melody:     m,
		open:       true,
		rwmutex:    &sync.RWMutex{},
	}

	m.hub.register <- session

	m.connectHandler(session)

	go session.writePump()

	session.readPump()

	if !m.hub.closed() {
		m.hub.unregister <- session
	}

	session.close()

	m.disconnectHandler(session)

	return nil
}

// Close closes the melody instance and all connected sessions.
func (m *Melody) Close() error {
	if m.hub.closed() {
		return ErrClosed
	}

	m.hub.exit <- &envelope{t: websocket.CloseMessage, msg: []byte{}}

	return nil
}

// HandleMessage fires fn when a text message comes in.
func (m *Melody) HandleMessage(fn func(*Session, []byte)) {
	m.messageHandler = fn
}

// HandlePong fires fn when a pong is received from a session.
func (m *Melody) HandlePong(fn func(*Session)) {
	m.pongHandler = fn
}

// HandleConnect fires fn when a session connects.
func (m *Melody) HandleConnect(fn func(session *Session)) {
	m.connectHandler = fn
}

// HandleDisconnect fires fn when a session disconnects.
func (m *Melody) HandleDisconnect(fn func(session *Session)) {
	m.disconnectHandler = fn
}

// Broadcast broadcasts a text message to all sessions.
func (m *Melody) Broadcast(msg []byte) error {
	if m.hub.closed() {
		return ErrClosed
	}

	message := &envelope{t: websocket.TextMessage, msg: msg}
	m.hub.broadcast <- message

	return nil
}

// HandleError fires fn when a session has an error.
func (m *Melody) HandleError(fn func(*Session, error)) {
	m.errorHandler = fn
}

// HandleMessageBinary fires fn when binary message comes in.
func (m *Melody) HandleMessageBinary(fn func(*Session, []byte)) {
	m.messageHandlerBinary = fn
}

// BroadcastBinary broadcasts a binary message to all sessions.
func (m *Melody) BroadcastBinary(msg []byte) error {
	if m.hub.closed() {
		return ErrClosed
	}

	message := &envelope{t: websocket.BinaryMessage, msg: msg}
	m.hub.broadcast <- message

	return nil
}
