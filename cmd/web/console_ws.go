package web

import (
	"log"
	"net"
	"net/http"

	"github.com/c0m4r/v/engine"
	"golang.org/x/net/websocket"
)

func handleConsole(e *engine.Engine) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")

		sockPath, err := e.ConsoleSocketPath(id)
		if err != nil {
			jsonError(w, 400, err.Error())
			return
		}

		handler := websocket.Handler(func(ws *websocket.Conn) {
			conn, err := net.Dial("unix", sockPath)
			if err != nil {
				log.Printf("console connect error: %v", err)
				return
			}
			defer func() { _ = conn.Close() }()
			defer func() { _ = ws.Close() }()

			done := make(chan struct{})

			// Console socket -> WebSocket (as binary messages)
			go func() {
				defer func() {
					select {
					case <-done:
					default:
						close(done)
					}
				}()
				buf := make([]byte, 4096)
				for {
					n, err := conn.Read(buf)
					if err != nil {
						return
					}
					if n > 0 {
						if err := websocket.Message.Send(ws, buf[:n]); err != nil {
							return
						}
					}
				}
			}()

			// WebSocket -> Console socket (as binary messages)
			go func() {
				defer func() {
					select {
					case <-done:
					default:
						close(done)
					}
				}()
				for {
					var msg []byte
					if err := websocket.Message.Receive(ws, &msg); err != nil {
						return
					}
					if _, err := conn.Write(msg); err != nil {
						return
					}
				}
			}()

			<-done
		})

		handler.ServeHTTP(w, r)
	}
}
