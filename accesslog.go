package main

import (
	"bufio"
	"errors"
	"log"
	"net"
	"net/http"
	"time"

	"nautix/internal/auth"
)

// statusWriter records the status code of a response. It forwards Hijack and
// Flush, and exposes the wrapped writer through Unwrap, so WebSocket upgrades
// keep working.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("connection cannot be hijacked")
	}
	w.status = http.StatusSwitchingProtocols
	return h.Hijack()
}

func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logRequests writes one line per request to the standard log, which is what
// "docker compose logs" shows:
//
//	203.0.113.7 GET /api/state 200 4ms
//
// The address is the one used for the login rate limit. When a proxy is in
// front, the peer address and the full X-Forwarded-For chain are appended, to
// check what the proxy really sends. Only the path is logged, never the query
// string, the body or cookies. A WebSocket is logged when it closes.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			log.Printf("%s %s %s %d %s (peer=%s xff=%q)", auth.ClientIP(r), r.Method, r.URL.Path, sw.status,
				time.Since(start).Round(time.Millisecond), r.RemoteAddr, xff)
			return
		}
		log.Printf("%s %s %s %d %s", auth.ClientIP(r), r.Method, r.URL.Path, sw.status,
			time.Since(start).Round(time.Millisecond))
	})
}
