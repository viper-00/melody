package melody

import (
	"net/http"
	"net/http/httptest"
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
	conn.SetPingHandler(func(string) error {
		return nil
	})
	defer conn.Close()

	if err != nil {
		t.Error(err)
	}

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
		conn, _ := NewDialer(http.URL)
		defer conn.Close()

		listeners := make([]*websocket.Conn, n)
		for i := 0; i < n; i++ {
			listener, _ := NewDialer(http.URL)
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

}

func TestMetadata(t *testing.T) {
}
