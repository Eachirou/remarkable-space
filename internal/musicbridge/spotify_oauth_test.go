package musicbridge

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSpotifyPKCEAuthorizationAndRefresh(t *testing.T) {
	var calls int
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.String() != spotifyTokenURL {
			t.Fatalf("token endpoint = %s, want %s", req.URL, spotifyTokenURL)
		}
		if err := req.ParseForm(); err != nil {
			t.Fatal(err)
		}
		switch calls {
		case 0:
			if req.Form.Get("grant_type") != "authorization_code" || req.Form.Get("code_verifier") == "" {
				t.Fatalf("authorization exchange missing PKCE parameters: %v", req.Form)
			}
		case 1:
			if req.Form.Get("grant_type") != "refresh_token" || req.Form.Get("refresh_token") != "refresh-1" {
				t.Fatalf("refresh request has incorrect parameters: %v", req.Form)
			}
		default:
			t.Fatalf("unexpected token exchange %d", calls+1)
		}
		calls++
		response := `{"access_token":"access-1","refresh_token":"refresh-1","expires_in":3600}`
		if calls == 2 {
			response = `{"access_token":"access-2","expires_in":3600}`
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(response)),
			Request:    req,
		}, nil
	})}
	configPath := filepath.Join(t.TempDir(), "spotify.json")
	oauth, err := NewSpotifyOAuth("client-id", "http://127.0.0.1:9120/oauth/callback", configPath, client)
	if err != nil {
		t.Fatal(err)
	}

	authURL, err := oauth.AuthorizationURL()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("code_challenge_method") != "S256" || query.Get("code_challenge") == "" {
		t.Fatalf("authorization URL missing PKCE challenge: %s", authURL)
	}
	if query.Get("scope") != spotifyScopes {
		t.Fatalf("requested scopes = %q, want %q", query.Get("scope"), spotifyScopes)
	}
	if err := oauth.CompleteAuthorization(context.Background(), query.Get("state"), "authorization-code"); err != nil {
		t.Fatal(err)
	}

	if got, err := oauth.AccessToken(context.Background()); err != nil || got != "access-1" {
		t.Fatalf("AccessToken() = %q, %v; want access-1", got, err)
	}
	oauth.mu.Lock()
	oauth.token.ExpiresAt = time.Now().Add(-time.Second)
	oauth.mu.Unlock()
	if got, err := oauth.AccessToken(context.Background()); err != nil || got != "access-2" {
		t.Fatalf("refreshed AccessToken() = %q, %v; want access-2", got, err)
	}

	saved, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var persisted persistedSpotifyToken
	if err := json.Unmarshal(saved, &persisted); err != nil {
		t.Fatal(err)
	}
	if persisted.RefreshToken != "refresh-1" {
		t.Fatalf("saved refresh token = %q, want refresh-1", persisted.RefreshToken)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("Spotify token file permissions = %o, want 600", info.Mode().Perm())
	}
}

func TestSpotifyOAuthRejectsMismatchedState(t *testing.T) {
	oauth, err := NewSpotifyOAuth("client-id", "http://127.0.0.1:9120/oauth/callback", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := oauth.CompleteAuthorization(context.Background(), "wrong-state", "code"); err == nil {
		t.Fatal("CompleteAuthorization accepted an unknown state")
	}
}

func TestSpotifyAPIUnauthorizedRefreshesAndRetries(t *testing.T) {
	apiRequests := 0
	tokenRequests := 0
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/v1/me/player":
			apiRequests++
			if apiRequests == 1 {
				if got := req.Header.Get("Authorization"); got != "Bearer expired-access" {
					t.Fatalf("first API authorization = %q", got)
				}
				return &http.Response{
					StatusCode: http.StatusUnauthorized,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":{"message":"expired"}}`)),
					Request:    req,
				}, nil
			}
			if got := req.Header.Get("Authorization"); got != "Bearer refreshed-access" {
				t.Fatalf("retry API authorization = %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"is_playing":true,"item":{"name":"Test track"}}`)),
				Request:    req,
			}, nil
		case "/api/token":
			tokenRequests++
			if err := req.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if req.Form.Get("grant_type") != "refresh_token" {
				t.Fatalf("token grant_type = %q", req.Form.Get("grant_type"))
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"access_token":"refreshed-access","expires_in":3600}`)),
				Request:    req,
			}, nil
		default:
			t.Fatalf("unexpected request URL %s", req.URL)
			return nil, nil
		}
	})}
	oauth, err := NewSpotifyOAuth("client-id", "http://127.0.0.1:9120/oauth/callback", "", client)
	if err != nil {
		t.Fatal(err)
	}
	oauth.token = spotifyToken{
		AccessToken:  "expired-access",
		RefreshToken: "refresh-token",
		ExpiresAt:    time.Now().Add(time.Hour),
	}
	s := &Server{BridgeToken: "device-token", SpotifyOAuth: oauth, Client: client}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	req.Header.Set("Authorization", "Bearer device-token")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status response = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var status Status
	if err := json.Unmarshal(rec.Body.Bytes(), &status); err != nil {
		t.Fatal(err)
	}
	if !status.Playing || status.Track != "Test track" {
		t.Fatalf("status = %+v, want playing Test track", status)
	}
	if apiRequests != 2 || tokenRequests != 1 {
		t.Fatalf("API requests = %d, token refresh requests = %d; want 2 and 1", apiRequests, tokenRequests)
	}
}
