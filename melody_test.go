package melody

import (
	"bytes"
	"math/rand"
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

func TestBroadcastBinaryOthers(t *testing.T) {
	broadcast := NewTestServer()
	broadcast.m.HandleMessageBinary(func(session *Session, msg []byte) {
		broadcast.m.BroadcastBinaryOthers(msg, session)
	})
	broadcast.m.Config.PongWait = time.Second
	broadcast.m.Config.PingPeriod = time.Second * 9 / 10
	server := httptest.NewServer(broadcast)
	defer server.Close()

	n := 10

	fn := func(msg []byte) bool {
		conn, _ := NewDialer(server.URL)
		defer conn.Close()

		listeners := make([]*websocket.Conn, n)
		for i := 0; i < n; i++ {
			listener, _ := NewDialer(server.URL)
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
				t.Errorf("message type should be BinaryMessage")
				return false
			}

			if !bytes.Equal(msg, ret) {
				t.Errorf("%v should equal %v", msg, ret)
				return false
			}
		}

		return true
	}

	if !fn([]byte{2, 3, 5, 7, 11}) {
		t.Errorf("should not be false")
	}
}

func TestWriteClosed(t *testing.T) {
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

		conn.WriteMessage(websocket.TextMessage, []byte(msg))

		server.m.HandleConnect(func(s *Session) {
			s.Close()
		})

		server.m.HandleDisconnect(func(s *Session) {
			err := s.Write([]byte("hello world"))
			if err == nil {
				t.Error("There should be an error")
			}
		})

		return true
	}

	if err := quick.Check(fn, nil); err != nil {
		t.Error(err)
	}
}

func TestLen(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	connect := int(rand.Int31n(100))
	disconnect := rand.Float32()
	conns := make([]*websocket.Conn, connect)
	defer func() {
		for _, conn := range conns {
			if conn != nil {
				conn.Close()
			}
		}
	}()

	server := NewTestServerHandler(func(session *Session, msg []byte) {})
	http := httptest.NewServer(server)

	disconnected := 0
	for i := 0; i < connect; i++ {
		conn, err := NewDialer(http.URL)
		if err != nil {
			t.Error(err)
		}

		if rand.Float32() < disconnect {
			conns[i] = nil
			disconnected++
			conn.Close()
			continue
		}

		conns[i] = conn
	}

	time.Sleep(time.Millisecond)

	connected := connect - disconnected

	if server.m.Len() != connected {
		t.Errorf("melody length %d should equal %d", server.m.Len(), connected)
	}
}

func TestGetSessions(t *testing.T) {
	rand.Seed(time.Now().UnixNano())
	connect := int(rand.Int31n(100))
	disconnect := rand.Float32()
	conns := make([]*websocket.Conn, connect)
	defer func() {
		for _, conn := range conns {
			if conn != nil {
				conn.Close()
			}
		}
	}()

	server := NewTestServerHandler(func(session *Session, msg []byte) {})
	http := httptest.NewServer(server)
	defer http.Close()

	disconnected := 0
	for i := 0; i < connect; i++ {
		conn, err := NewDialer(http.URL)
		if err != nil {
			t.Error(err)
		}

		if rand.Float32() < disconnect {
			conns[i] = nil
			disconnected++
			conn.Close()
			continue
		}

		conns[i] = conn
	}

	time.Sleep(time.Millisecond)

	connected := connect - disconnected

	sessions, err := server.m.Sessions()
	if err != nil {
		t.Fatalf("error retrieving sessions: %v", err.Error())
	}

	if len(sessions) != connected {
		t.Errorf("melody sessions %d should equal %d", len(sessions), connected)
	}
}

func TestEchoBinary(t *testing.T) {
	server := NewTestServer()
	server.m.HandleMessageBinary(func(session *Session, msg []byte) {
		session.WriteBinary(msg)
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

		conn.WriteMessage(websocket.BinaryMessage, []byte(msg))

		_, ret, err := conn.ReadMessage()
		if err != nil {
			t.Error(err)
			return false
		}

		if msg != string(ret) {
			t.Errorf("%s should be equal %s", msg, string(ret))
			return false
		}

		return true
	}

	if err := quick.Check(fn, nil); err != nil {
		t.Error(err)
	}
}

func TestBroadcastFilter(t *testing.T) {
	broadcast := NewTestServer()
	broadcast.m.HandleMessage(func(session *Session, msg []byte) {
		broadcast.m.BroadcastFilter(msg, func(q *Session) bool {
			return session == q
		})
	})

	server := httptest.NewServer(broadcast)
	defer server.Close()

	fn := func(msg string) bool {
		conn, err := NewDialer(server.URL)
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

	if !fn("test") {
		t.Errorf("should not be false")
	}
}

func TestSmallMessageBuffer(t *testing.T) {
	server := NewTestServerHandler(func(session *Session, msg []byte) {
		session.Write(msg)
	})

	server.m.Config.MessageBufferSize = 0
	server.m.HandleError(func(s *Session, err error) {
		if err == nil {
			t.Error("there should be a buffer full error here")
		}
	})

	http := httptest.NewServer(server)
	defer http.Close()

	conn, err := NewDialer(http.URL)
	if err != nil {
		t.Error(err)
	}
	defer conn.Close()

	conn.WriteMessage(websocket.TextMessage, []byte("abcdef"))
}

func BenchmarkSessionWrite(b *testing.B) {
	server := NewTestServerHandler(func(session *Session, msg []byte) {
		session.Write(msg)
	})

	http := httptest.NewServer(server)
	conn, _ := NewDialer(http.URL)
	defer http.Close()
	defer conn.Close()

	for n := 0; n < b.N; n++ {
		conn.WriteMessage(websocket.TextMessage, []byte("test"))
		conn.ReadMessage()
	}
}

func BenchmarkBroadcast(b *testing.B) {
	server := NewTestServerHandler(func(session *Session, msg []byte) {
		session.Write(msg)
	})

	http := httptest.NewServer(server)
	defer http.Close()

	conns := make([]*websocket.Conn, 0)
	num := 100

	for i := 0; i < num; i++ {
		conn, _ := NewDialer(http.URL)
		conns = append(conns, conn)
	}

	for n := 0; n < b.N; n++ {
		server.m.Broadcast([]byte("test"))

		for i := 0; i < num; i++ {
			conns[i].ReadMessage()
		}
	}

	for i := 0; i < num; i++ {
		conns[i].Close()
	}
}
