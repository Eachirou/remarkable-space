package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/boangs/remarkable-space/internal/musicbridge"
)

func main() {
	addr := flag.String("addr", env("MUSIC_BRIDGE_ADDR", "127.0.0.1:9120"), "listen address")
	token := flag.String("token", os.Getenv("MUSIC_BRIDGE_TOKEN"), "shared token required by the RM2 app")
	spotifyToken := flag.String("spotify-token", os.Getenv("SPOTIFY_ACCESS_TOKEN"), "Spotify access token")
	device := flag.String("device", os.Getenv("SPOTIFY_DEVICE_ID"), "optional Spotify device id")
	flag.Parse()

	if *token == "" {
		log.Fatal("MUSIC_BRIDGE_TOKEN or -token is required")
	}
	server := &musicbridge.Server{
		BridgeToken:  *token,
		SpotifyToken: *spotifyToken,
		DeviceID:     *device,
		Client:       &http.Client{Timeout: 8 * time.Second},
	}
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("music bridge listening on %s", *addr)
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
