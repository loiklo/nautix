// Nautix: skill tracking for the students of a diving club.
// A single binary serves the JSON API, the WebSocket and the embedded frontend
// over plain HTTP; TLS is expected to be handled by a reverse proxy.
package main

import (
	"embed"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"time"

	"nautix/internal/api"
	"nautix/internal/auth"
	"nautix/internal/db"
	"nautix/internal/hub"
)

//go:embed web
var webFiles embed.FS

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	conn, err := db.Open(db.Config{
		Host:     env("DB_HOST", "127.0.0.1"),
		Port:     env("DB_PORT", "3306"),
		User:     env("DB_USER", "nautix"),
		Password: env("DB_PASSWORD", "nautix"),
		Name:     env("DB_NAME", "nautix"),
	})
	if err != nil {
		log.Fatal(err)
	}

	srv := &api.Server{
		DB:   conn,
		Hub:  hub.New(),
		Auth: auth.New(conn, env("COOKIE_SECURE", "true") != "false"),
	}
	mux := http.NewServeMux()
	srv.Routes(mux)

	// Content types that the host system may not know or may get wrong.
	mime.AddExtensionType(".js", "text/javascript")
	mime.AddExtensionType(".webmanifest", "application/manifest+json")
	static, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatal(err)
	}
	files := http.FileServerFS(static)
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		// Always revalidate so a new version is picked up without manual cache clearing.
		w.Header().Set("Cache-Control", "no-cache")
		files.ServeHTTP(w, r)
	})

	addr := env("LISTEN_ADDR", ":8080")
	log.Printf("nautix listening on %s (plain HTTP)", addr)
	server := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(server.ListenAndServe())
}
