package melody

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

func TestStop(t *testing.T) {
	test := NewTestServer()
	server := httptest.NewServer(test)
	defer server.Close()

}
