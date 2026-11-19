package main

import (
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/boangs/remarkable-space/internal/musicbridge"
)

func main() {
	addr := flag.String("addr", env("MUSIC_BRIDGE_ADDR", "127.0.0.1:9120"), "listen address")
	token := flag.String("token", os.Getenv("MUSIC_BRIDGE_TOKEN"), "optional legacy shared token")
	spotifyToken := flag.String("spotify-token", os.Getenv("SPOTIFY_ACCESS_TOKEN"), "Spotify access token")
	device := flag.String("device", os.Getenv("SPOTIFY_DEVICE_ID"), "optional Spotify device id")
	clientID := flag.String("client-id", os.Getenv("MUSIC_BRIDGE_CLIENT_ID"), "Spotify app client ID (enables PKCE authorization)")
	redirectURI := flag.String("redirect-uri", os.Getenv("MUSIC_BRIDGE_REDIRECT_URI"), "exact callback URL registered in the Spotify dashboard")
	configDir := flag.String("config-dir", "", "directory for private bridge credentials")
	flag.Parse()
	if *redirectURI == "" {
		*redirectURI = defaultRedirectURI(*addr)
	}

	if *configDir == "" {
		userConfig, err := os.UserConfigDir()
		if err != nil {
			log.Fatalf("cannot find user config directory: %v", err)
		}
		*configDir = filepath.Join(userConfig, "remarkable-space", "music-bridge")
	}
	if err := os.MkdirAll(*configDir, 0o700); err != nil {
		log.Fatalf("cannot create private config directory: %v", err)
	}
	if err := os.Chmod(*configDir, 0o700); err != nil {
		log.Fatalf("cannot secure private config directory: %v", err)
	}

	client := &http.Client{Timeout: 8 * time.Second}
	var oauth *musicbridge.SpotifyOAuth
	if *clientID != "" {
		var err error
		oauth, err = musicbridge.NewSpotifyOAuth(
			*clientID, *redirectURI, filepath.Join(*configDir, "spotify.json"), client,
		)
		if err != nil {
			log.Fatalf("cannot initialize Spotify authorization: %v", err)
		}
	}
	pairCode, err := musicbridge.NewPairingCode()
	if err != nil {
		log.Fatalf("cannot initialize device pairing: %v", err)
	}
	server := &musicbridge.Server{
		BridgeToken:  *token,
		SpotifyToken: *spotifyToken,
		DeviceID:     *device,
		Client:       client,
		SpotifyOAuth: oauth,
		PairCode:     pairCode,
		PairExpires:  time.Now().Add(10 * time.Minute),
		PairFile:     filepath.Join(*configDir, "paired-device.json"),
	}
	if err := server.Init(); err != nil {
		log.Fatalf("cannot load paired-device credentials: %v", err)
	}
	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           server.Handler(),
		ReadHeaderTimeout: 3 * time.Second,
		IdleTimeout:       30 * time.Second,
	}
	log.Printf("Pairing code (valid for 10 minutes): %s", pairCode)
	if oauth != nil {
		log.Printf("Authorize Spotify in a browser on this computer: http://127.0.0.1%s/oauth/start", portSuffix(*addr))
	} else if *spotifyToken == "" {
		log.Printf("Spotify OAuth is not configured; set MUSIC_BRIDGE_CLIENT_ID to enable it")
	}
	log.Printf("music bridge listening on %s", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func defaultRedirectURI(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "9120"
	}
	return "http://127.0.0.1:" + port + "/oauth/callback"
}

func portSuffix(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		port = "9120"
	}
	return ":" + port
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
