package melody

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/gorilla/websocket"
)

type TestServer struct {
	m *Melody
}

func (s *TestServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.m.HandleRequest(w, r)
}

func NewTestServer() *TestServer {
	new := New()
	return &TestServer{
		m: new,
	}
}

func NewDialer(url string) (*websocket.Conn, error) {
	dialer := &websocket.Dialer{}
	conn, _, err := dialer.Dial(strings.Replace(url, "http", "ws", 1), nil)
	return conn, err
}

func NewTestServerHandler(handler handleMessageFunc) *TestServer {
	m := New()
	m.HandleMessage(handler)
	return &TestServer{
		m: m,
	}
}

func TestStop(t *testing.T) {
	new := NewTestServer()
	server := httptest.NewServer(new)
	defer server.Close()

	conn, err := NewDialer(server.URL)
	if err != nil {
		t.Error(err)
	}
	defer conn.Close()

	new.m.Close()
}

func TestPingPong(t *testing.T) {
	new := NewTestServer()
	new.m.Config.PongWait = time.Second
	new.m.Config.PingPeriod = time.Second * 9 / 10
	server := httptest.NewServer(new)
	defer server.Close()

	conn, err := NewDialer(server.URL)
	if err != nil {
		t.Error(err)
	}
	conn.SetPingHandler(func(string) error {
		return nil
	})
	defer conn.Close()

	conn.WriteMessage(websocket.TextMessage, []byte("test"))

	_, _, err = conn.ReadMessage()
	if err == nil {
		t.Error("There should be an error")
	}
}

func TestPong(t *testing.T) {
	server := NewTestServerHandler(func(session *Session, msg []byte) {
		session.Write(msg)
	})

	server.m.Config.PongWait = time.Second
	server.m.Config.PingPeriod = time.Second * 9 / 10
	http := httptest.NewServer(server)
	defer http.Close()

	conn, err := NewDialer(http.URL)
	if err != nil {
		t.Error(err)
	}
	defer conn.Close()

	fired := false
	server.m.HandlePong(func(s *Session) {
		fired = true
	})

	conn.WriteMessage(websocket.PongMessage, nil)

	time.Sleep(time.Millisecond)

	if !fired {
		t.Error("should have fired ping handler")
	}
}

func TestEcho(t *testing.T) {
	server := NewTestServerHandler(func(session *Session, msg []byte) {
		session.Write(msg)
	})

	http := httptest.NewServer(server)
	defer http.Close()

	fn := func(msg string) bool {
		conn, err := NewDialer(http.URL)
		if err != nil {
			t.Error(err)
			return false
		}
		defer conn.Close()

		conn.WriteMessage(websocket.TextMessage, []byte(msg))

		_, ret, err := conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return false
		}

		if msg != string(ret) {
			t.Errorf("%s should equal %s", msg, string(ret))
			return false
		}

		return true
	}

	if err := quick.Check(fn, nil); err != nil {
		t.Error(err)
	}
}

func TestHandlers(t *testing.T) {
	server := NewTestServer()
	server.m.HandleMessage(func(session *Session, msg []byte) {
		session.Write(msg)
	})

	http := httptest.NewServer(server)
	defer http.Close()

	var q *Session

	// connect the server
	server.m.HandleConnect(func(session *Session) {
		q = session
		session.Close()
	})

	// disconnect the server
	server.m.HandleDisconnect(func(session *Session) {
		if q != session {
			t.Error("disconnecting the session should be the same as connecting")
		}
	})

	NewDialer(http.URL)
}

func TestBroadcast(t *testing.T) {
	broadcast := NewTestServer()
	broadcast.m.HandleMessage(func(session *Session, msg []byte) {
		broadcast.m.Broadcast(msg)
	})
	http := httptest.NewServer(broadcast)
	defer http.Close()

	n := 10

	fn := func(msg string) bool {
		conn, err := NewDialer(http.URL)

		if err != nil {
			t.Error(err)
		}

		defer conn.Close()

		listeners := make([]*websocket.Conn, n)
		for i := 0; i < n; i++ {
			listener, err := NewDialer(http.URL)

			if err != nil {
				t.Error(err)
			}

			listeners[i] = listener
			defer listeners[i].Close()
		}

		conn.WriteMessage(websocket.TextMessage, []byte(msg))

		for i := 0; i < n; i++ {
			_, ret, err := listeners[i].ReadMessage()
			if err != nil {
				t.Error(err)
				return false
			}

			if msg != string(ret) {
				t.Errorf("%s should be equal %s", msg, string(ret))
				return false
			}
		}

		return true
	}

	if !fn("test") {
		t.Errorf("should not be false")
	}
}

func TestUpgrader(t *testing.T) {
	broadcast := NewTestServer()
	broadcast.m.HandleMessage(func(session *Session, msg []byte) {
		session.Write(msg)
	})

	server := httptest.NewServer(broadcast)
	defer server.Close()

	broadcast.m.Upgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin:     func(r *http.Request) bool { return false },
	}

	broadcast.m.HandleError(func(session *Session, err error) {
		if err == nil || err.Error() != "websocket: origin not allowed" {
			t.Error("there should be a origin error")
		}
	})

	_, err := NewDialer(server.URL)
	if err == nil || err.Error() != "websocket: bad handshake" {
		t.Error("there should be a bad handshake error")
	}
}

func TestMetadata(t *testing.T) {
	server := NewTestServer()
	server.m.HandleConnect(func(session *Session) {
		session.Set("stamp", time.Now().UnixNano())
	})
	server.m.HandleMessage(func(session *Session, msg []byte) {
		stamp := session.MustGet("stamp").(int64)
		session.Write([]byte(strconv.Itoa(int(stamp))))
	})

	http := httptest.NewServer(server)
	defer http.Close()

	fn := func(msg string) bool {
		conn, err := NewDialer(http.URL)
		if err != nil {
			t.Error(err)
			return false
		}
		defer conn.Close()

		conn.WriteMessage(websocket.TextMessage, []byte(msg))

		_, ret, err := conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return false
		}

		stamp, err := strconv.Atoi(string(ret))
		if err != nil {
			t.Error(err)
			return false
		}

		diff := int(time.Now().UnixNano()) - stamp

		if diff <= 0 {
			t.Errorf("diff should be above 0 %d", diff)
			return false
		}

		return true
	}

	if err := quick.Check(fn, nil); err != nil {
		t.Error(err)
	}
}

func TestBroadcastBinary(t *testing.T) {
	server := NewTestServer()
	server.m.HandleMessageBinary(func(session *Session, msg []byte) {
		server.m.BroadcastBinary(msg)
	})

	http := httptest.NewServer(server)
	defer http.Close()

	n := 10

	fn := func(msg []byte) bool {
		conn, err := NewDialer(http.URL)

		if err != nil {
			t.Error(err)
			return false
		}

		defer conn.Close()

		listeners := make([]*websocket.Conn, n)
		for i := 0; i < n; i++ {
			listener, err := NewDialer(http.URL)

			if err != nil {
				t.Error(err)
				return false
			}

			listeners[i] = listener
			defer listeners[i].Close()
		}

		conn.WriteMessage(websocket.BinaryMessage, []byte(msg))

		for i := 0; i < n; i++ {
			messageType, ret, err := listeners[i].ReadMessage()
			if err != nil {
				t.Error(err)
				return false
			}

			if messageType != websocket.BinaryMessage {
				t.Error("message type should be BinaryMessage")
				return false
			}

			if !bytes.Equal(msg, ret) {
				t.Errorf("%v should equal %v", msg, ret)
				return false
			}
		}
		return true
	}

	if !fn([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}) {
		t.Error("something error occur.")
	}
}
