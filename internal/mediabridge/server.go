package mediabridge

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Feed struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Episode struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Station struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Server struct {
	Token    string
	Feeds    []Feed
	Stations []Station
	Client   *http.Client
	mu       sync.Mutex
	player   *exec.Cmd
	playing  string
}

type rss struct {
	Channel struct {
		Items []struct {
			Title     string `xml:"title"`
			Enclosure struct {
				URL string `xml:"url,attr"`
			} `xml:"enclosure"`
			Link string `xml:"link"`
		} `xml:"item"`
	} `xml:"channel"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /api/podcast/feeds", s.auth(s.feeds))
	mux.HandleFunc("GET /api/podcast/episodes", s.auth(s.episodes))
	mux.HandleFunc("POST /api/podcast/play", s.auth(s.play))
	mux.HandleFunc("GET /api/radio/stations", s.auth(s.stations))
	mux.HandleFunc("POST /api/radio/play", s.auth(s.play))
	mux.HandleFunc("GET /api/media/status", s.auth(s.status))
	mux.HandleFunc("POST /api/media/control", s.auth(s.control))
	return http.TimeoutHandler(mux, 15*time.Second, `{"error":"request timed out"}`)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.Token == "" || r.Header.Get("Authorization") != "Bearer "+s.Token {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid bridge token"})
			return
		}
		next(w, r)
	}
}

func (s *Server) feeds(w http.ResponseWriter, _ *http.Request) { writeJSON(w, http.StatusOK, s.Feeds) }
func (s *Server) stations(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Stations)
}

func (s *Server) episodes(w http.ResponseWriter, r *http.Request) {
	feedURL := r.URL.Query().Get("feed")
	if feedURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "feed is required"})
		return
	}
	client := s.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, feedURL, nil)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": resp.Status})
		return
	}
	var doc rss
	if err := xml.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&doc); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "invalid RSS feed"})
		return
	}
	out := make([]Episode, 0, len(doc.Channel.Items))
	for _, item := range doc.Channel.Items {
		url := item.Enclosure.URL
		if url == "" {
			url = item.Link
		}
		if item.Title != "" && url != "" {
			out = append(out, Episode{Title: item.Title, URL: url})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) play(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URL   string `json:"url"`
		Title string `json:"title"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 16<<10)).Decode(&req); err != nil || req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}
	if err := s.startPlayer(r.Context(), req.URL, req.Title); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) status(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	playing := s.player != nil && s.player.Process != nil
	if playing && s.player.ProcessState != nil {
		playing = !s.player.ProcessState.Exited()
	}
	writeJSON(w, http.StatusOK, map[string]any{"playing": playing, "title": s.playing})
}

func (s *Server) control(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action string `json:"action"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<10)).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "body must be JSON"})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.Action == "stop" {
		if s.player != nil && s.player.Process != nil {
			_ = s.player.Process.Kill()
		}
		s.player = nil
		s.playing = ""
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
		return
	}
	writeJSON(w, http.StatusBadRequest, map[string]string{"error": "action must be stop"})
}

func (s *Server) startPlayer(ctx context.Context, mediaURL, title string) error {
	if !strings.HasPrefix(mediaURL, "http://") && !strings.HasPrefix(mediaURL, "https://") {
		return errors.New("media URL must use http or https")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.player != nil && s.player.Process != nil {
		_ = s.player.Process.Kill()
	}
	_ = ctx
	cmd := exec.Command("mpv", "--no-video", "--really-quiet", mediaURL)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mpv: %w", err)
	}
	s.player, s.playing = cmd, title
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func ParseList(value string) []Feed {
	var out []Feed
	for _, item := range strings.Split(value, ",") {
		name, url, ok := strings.Cut(item, "=")
		if ok && strings.TrimSpace(url) != "" {
			out = append(out, Feed{Name: strings.TrimSpace(name), URL: strings.TrimSpace(url)})
		}
	}
	return out
}

func ParseStations(value string) []Station {
	var out []Station
	for _, item := range strings.Split(value, ",") {
		name, url, ok := strings.Cut(item, "=")
		if ok && strings.TrimSpace(url) != "" {
			out = append(out, Station{Name: strings.TrimSpace(name), URL: strings.TrimSpace(url)})
		}
	}
	return out
}

func FromEnv() (*Server, error) {
	token := os.Getenv("MEDIA_BRIDGE_TOKEN")
	if token == "" {
		return nil, errors.New("MEDIA_BRIDGE_TOKEN is required")
	}
	return &Server{Token: token, Feeds: ParseList(os.Getenv("PODCAST_FEEDS")),
		Stations: ParseStations(os.Getenv("RADIO_STATIONS")), Client: &http.Client{Timeout: 10 * time.Second}}, nil
}
