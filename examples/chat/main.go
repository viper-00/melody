package main

import (
	"melody"
	"net/http"

	"github.com/gin-gonic/gin"
)

func main() {
	g := gin.Default()
	m := melody.New()

	g.GET("/", func(c *gin.Context) {
		http.ServeFile(c.Writer, c.Request, "index.html")
	})

	g.GET("/ws", func(c *gin.Context) {
		m.HandleRequest(c.Writer, c.Request)
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		m.Broadcast(msg)
	})

	g.Run(":5000")
}
