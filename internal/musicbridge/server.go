package musicbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const spotifyAPI = "https://api.spotify.com/v1"

type Server struct {
	BridgeToken  string
	SpotifyToken string
	DeviceID     string
	Client       *http.Client
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

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /api/status", s.auth(s.status))
	mux.HandleFunc("POST /api/control", s.auth(s.control))
	return http.TimeoutHandler(mux, 10*time.Second, `{"error":"request timed out"}`)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.BridgeToken == "" || r.Header.Get("Authorization") != "Bearer "+s.BridgeToken {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge token"})
			return
		}
		next(w, r)
	}
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
	if s.SpotifyToken == "" {
		return http.StatusServiceUnavailable, errors.New("SPOTIFY_ACCESS_TOKEN is not configured")
	}
	var payload io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return http.StatusInternalServerError, err
		}
		payload = bytes.NewReader(b)
	}
	endpoint := spotifyAPI + path
	if s.DeviceID != "" {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		endpoint += separator + url.Values{"device_id": []string{s.DeviceID}}.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return http.StatusInternalServerError, err
	}
	req.Header.Set("Authorization", "Bearer "+s.SpotifyToken)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return http.StatusBadGateway, fmt.Errorf("Spotify request failed: %w", err)
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

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
