package musicbridge

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const spotifyAPI = "https://api.spotify.com/v1"

type Server struct {
	BridgeToken  string
	SpotifyToken string
	DeviceID     string
	Client       *http.Client
	SpotifyOAuth *SpotifyOAuth
	PairCode     string
	PairExpires  time.Time
	PairFile     string

	pairMu       sync.Mutex
	pairHash     []byte
	pairAttempts int
	pairUsed     bool
}

type Status struct {
	Playing  bool   `json:"playing"`
	Track    string `json:"track,omitempty"`
	Artist   string `json:"artist,omitempty"`
	Album    string `json:"album,omitempty"`
	Device   string `json:"device,omitempty"`
	Progress int    `json:"progress_ms,omitempty"`
	Duration int    `json:"duration_ms,omitempty"`
}

type controlRequest struct {
	Action string `json:"action"`
}

type pairRequest struct {
	Code string `json:"code"`
}

type savedPair struct {
	TokenHash string `json:"token_hash"`
}

func NewPairingCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(100_000_000))
	if err != nil {
		return "", fmt.Errorf("generate pairing code: %w", err)
	}
	return fmt.Sprintf("%08d", n.Int64()), nil
}

func (s *Server) Init() error {
	if s.PairFile == "" {
		return nil
	}
	data, err := os.ReadFile(s.PairFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read paired-device credentials: %w", err)
	}
	var saved savedPair
	if err := json.Unmarshal(data, &saved); err != nil {
		return fmt.Errorf("parse paired-device credentials: %w", err)
	}
	hash, err := hex.DecodeString(saved.TokenHash)
	if err != nil || len(hash) != sha256.Size {
		return errors.New("paired-device credential file contains an invalid token hash")
	}
	s.pairHash = hash
	return nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("POST /api/pair", s.pair)
	mux.HandleFunc("GET /api/status", s.auth(s.status))
	mux.HandleFunc("POST /api/control", s.auth(s.control))
	mux.HandleFunc("GET /oauth/start", s.oauthStart)
	mux.HandleFunc("GET /oauth/callback", s.oauthCallback)
	return http.TimeoutHandler(mux, 10*time.Second, `{"error":"request timed out"}`)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		hash := sha256.Sum256([]byte(provided))
		s.pairMu.Lock()
		pairOK := len(s.pairHash) == sha256.Size && subtle.ConstantTimeCompare(hash[:], s.pairHash) == 1
		s.pairMu.Unlock()
		legacyOK := s.BridgeToken != "" && subtle.ConstantTimeCompare(
			[]byte(r.Header.Get("Authorization")), []byte("Bearer "+s.BridgeToken),
		) == 1
		if !pairOK && !legacyOK {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge token"})
			return
		}
		next(w, r)
	}
}

func (s *Server) pair(w http.ResponseWriter, r *http.Request) {
	var req pairRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return
	}
	s.pairMu.Lock()
	defer s.pairMu.Unlock()
	if s.pairUsed || s.PairCode == "" {
		writeJSON(w, http.StatusGone, map[string]string{"error": "pairing code is no longer available; restart the bridge"})
		return
	}
	if !s.PairExpires.IsZero() && time.Now().After(s.PairExpires) {
		writeJSON(w, http.StatusGone, map[string]string{"error": "pairing code expired; restart the bridge"})
		return
	}
	s.pairAttempts++
	if s.pairAttempts > 10 {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many pairing attempts; restart the bridge"})
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.Code), []byte(s.PairCode)) != 1 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "pairing code was rejected"})
		return
	}
	token, err := randomURLToken(32)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not generate a device token"})
		return
	}
	hash := sha256.Sum256([]byte(token))
	if err := s.persistPairHash(hash[:]); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not save paired-device credentials"})
		return
	}
	s.pairHash = hash[:]
	s.pairUsed = true
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Server) persistPairHash(hash []byte) error {
	if s.PairFile == "" {
		return nil
	}
	dir := filepath.Dir(s.PairFile)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create pairing config directory: %w", err)
	}
	data, err := json.Marshal(savedPair{TokenHash: hex.EncodeToString(hash)})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".paired-device-*")
	if err != nil {
		return fmt.Errorf("create paired-device file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.PairFile); err != nil {
		return fmt.Errorf("save paired-device file: %w", err)
	}
	return nil
}

func (s *Server) oauthStart(w http.ResponseWriter, r *http.Request) {
	if s.SpotifyOAuth == nil {
		http.Error(w, "Spotify OAuth is not configured; set MUSIC_BRIDGE_CLIENT_ID", http.StatusServiceUnavailable)
		return
	}
	target, err := s.SpotifyOAuth.AuthorizationURL()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (s *Server) oauthCallback(w http.ResponseWriter, r *http.Request) {
	if s.SpotifyOAuth == nil {
		http.Error(w, "Spotify OAuth is not configured", http.StatusServiceUnavailable)
		return
	}
	if r.URL.Query().Get("error") != "" {
		http.Error(w, "Spotify authorization was denied. Return to the bridge terminal and try again.", http.StatusBadRequest)
		return
	}
	if err := s.SpotifyOAuth.CompleteAuthorization(
		r.Context(), r.URL.Query().Get("state"), r.URL.Query().Get("code"),
	); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, "<!doctype html><meta charset=utf-8><title>Spotify connected</title><p>Spotify is connected. You can close this tab and return to the bridge terminal.</p>")
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	var player struct {
		IsPlaying bool `json:"is_playing"`
		Progress  int  `json:"progress_ms"`
		Item      *struct {
			Name     string `json:"name"`
			Duration int    `json:"duration_ms"`
			Album    struct {
				Name string `json:"name"`
			} `json:"album"`
			Artists []struct {
				Name string `json:"name"`
			} `json:"artists"`
		} `json:"item"`
		Device *struct {
			Name string `json:"name"`
		} `json:"device"`
	}
	code, err := s.spotify(r.Context(), http.MethodGet, "/me/player", nil, &player)
	if err != nil {
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	result := Status{Playing: player.IsPlaying, Progress: player.Progress}
	if player.Item != nil {
		result.Track = player.Item.Name
		result.Duration = player.Item.Duration
		result.Album = player.Item.Album.Name
		for i, artist := range player.Item.Artists {
			if i > 0 {
				result.Artist += ", "
			}
			result.Artist += artist.Name
		}
	}
	if player.Device != nil {
		result.Device = player.Device.Name
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	var req controlRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return
	}
	paths := map[string]string{
		"play":     "/me/player/play",
		"pause":    "/me/player/pause",
		"next":     "/me/player/next",
		"previous": "/me/player/previous",
	}
	path, ok := paths[req.Action]
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be play, pause, next, or previous"})
		return
	}
	code, err := s.spotify(r.Context(), map[string]string{
		"play": http.MethodPut, "pause": http.MethodPut,
		"next": http.MethodPost, "previous": http.MethodPost,
	}[req.Action], path, nil, nil)
	if err != nil {
		writeJSON(w, code, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) spotify(ctx context.Context, method, path string, body any, result any) (int, error) {
	if s.SpotifyOAuth == nil && s.SpotifyToken == "" {
		return http.StatusServiceUnavailable, errors.New("SPOTIFY_ACCESS_TOKEN is not configured")
	}
	var payload []byte
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		payload = b
	}
	endpoint := spotifyAPI + path
	if s.DeviceID != "" {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		endpoint += separator + url.Values{"device_id": []string{s.DeviceID}}.Encode()
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	accessToken := s.SpotifyToken
	var err error
	if s.SpotifyOAuth != nil {
		accessToken, err = s.SpotifyOAuth.AccessToken(ctx)
		if err != nil {
			return http.StatusUnauthorized, err
		}
	}
	resp, err := s.spotifyRequest(ctx, client, method, endpoint, payload, accessToken)
	if err != nil {
		return http.StatusBadGateway, fmt.Errorf("Spotify request failed: %w", err)
	}
	if resp.StatusCode == http.StatusUnauthorized && s.SpotifyOAuth != nil {
		resp.Body.Close()
		accessToken, err = s.SpotifyOAuth.RefreshAfterUnauthorized(ctx, accessToken)
		if err != nil {
			return http.StatusUnauthorized, err
		}
		resp, err = s.spotifyRequest(ctx, client, method, endpoint, payload, accessToken)
		if err != nil {
			return http.StatusBadGateway, fmt.Errorf("Spotify request failed: %w", err)
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var detail struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(io.LimitReader(resp.Body, 16<<10)).Decode(&detail)
		if detail.Error.Message == "" {
			detail.Error.Message = resp.Status
		}
		return resp.StatusCode, errors.New(detail.Error.Message)
	}
	if result != nil {
		if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(result); err != nil {
			return http.StatusBadGateway, fmt.Errorf("invalid Spotify response: %w", err)
		}
	}
	return resp.StatusCode, nil
}

func (s *Server) spotifyRequest(ctx context.Context, client *http.Client, method, endpoint string, payload []byte, token string) (*http.Response, error) {
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return client.Do(req)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
