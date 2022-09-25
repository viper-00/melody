package melody

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
