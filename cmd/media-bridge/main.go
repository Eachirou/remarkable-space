package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/boangs/remarkable-space/internal/mediabridge"
)

func main() {
	addr := flag.String("addr", env("MEDIA_BRIDGE_ADDR", "127.0.0.1:9130"), "listen address")
	flag.Parse()
	server, err := mediabridge.FromEnv()
	if err != nil {
		log.Fatal(err)
	}
	httpServer := &http.Server{Addr: *addr, Handler: server.Handler(), ReadHeaderTimeout: 3 * time.Second, IdleTimeout: 30 * time.Second}
	log.Printf("media bridge listening on %s", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
