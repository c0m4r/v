package web

import (
	"bufio"
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/c0m4r/v/engine"
)

//go:embed static
var staticFS embed.FS

// Serve starts the web UI server.
func Serve(e *engine.Engine, args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:8080", "Listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}

	tok, err := loadOrGenerateToken(e.DataDir)
	if err != nil {
		return fmt.Errorf("auth token: %w", err)
	}

	mux := http.NewServeMux()
	registerRoutes(mux, e)

	handler := authMiddleware(tok, *listen, mux)
	srv := &http.Server{Addr: *listen, Handler: logMiddleware(handler)}

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-quit
		log.Printf("Shutting down — cleaning up tap interfaces...")
		e.CleanupTaps()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}()

	uiURL := buildUIURL(*listen, tok)
	log.Printf("Starting web UI on http://%s", *listen)
	log.Printf("Access URL: %s", uiURL)

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// buildUIURL constructs the browser URL with the embedded token.
// For 0.0.0.0/:: binds, substitutes "localhost" so the URL is usable.
func buildUIURL(listenAddr, token string) string {
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		host = listenAddr
		port = ""
	}
	if host == "0.0.0.0" || host == "::" || host == "" {
		host = "localhost"
	}
	var addr string
	if port != "" {
		addr = net.JoinHostPort(host, port)
	} else {
		addr = host
	}
	return fmt.Sprintf("http://%s/?token=%s", addr, token)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Hijack delegates to the underlying ResponseWriter for WebSocket upgrades.
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := w.ResponseWriter.(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, fmt.Errorf("underlying ResponseWriter does not support Hijack")
}

func logMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		// Sanitise path to prevent CRLF injection in logs.
		safePath := strings.NewReplacer("\n", "\\n", "\r", "\\r").Replace(r.URL.Path)
		log.Printf("%s %s %d %s", r.Method, safePath, sw.status, time.Since(start).Round(time.Millisecond))
	})
}

func registerRoutes(mux *http.ServeMux, e *engine.Engine) {
	// API routes
	mux.HandleFunc("GET /api/vms", handleListVMs(e))
	mux.HandleFunc("POST /api/vms", handleCreateVM(e))
	mux.HandleFunc("GET /api/vms/{id}", handleGetVM(e))
	mux.HandleFunc("PUT /api/vms/{id}", handleUpdateVM(e))
	mux.HandleFunc("POST /api/vms/{id}/start", handleStartVM(e))
	mux.HandleFunc("POST /api/vms/{id}/stop", handleStopVM(e))
	mux.HandleFunc("POST /api/vms/{id}/force-stop", handleForceStopVM(e))
	mux.HandleFunc("POST /api/vms/{id}/restart", handleRestartVM(e))
	mux.HandleFunc("PUT /api/vms/{id}/boot", handleSetBoot(e))
	mux.HandleFunc("PUT /api/vms/{id}/password", handleSetVMPassword(e))
	mux.HandleFunc("GET /api/vms/{id}/password", handleGetVMPassword(e))
	mux.HandleFunc("DELETE /api/vms/{id}", handleDeleteVM(e))
	mux.HandleFunc("GET /api/vms/{id}/console", handleConsole(e))

	mux.HandleFunc("GET /api/images", handleListImages(e))
	mux.HandleFunc("POST /api/images/pull", handlePullImage(e))

	mux.HandleFunc("GET /api/net/status", handleNetStatus(e))
	mux.HandleFunc("GET /api/info", handleInfo())

	mux.HandleFunc("GET /api/config", handleGetConfig(e))
	mux.HandleFunc("PUT /api/config", handleSetConfig(e))

	// Static files
	staticContent, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(staticContent)))
}
