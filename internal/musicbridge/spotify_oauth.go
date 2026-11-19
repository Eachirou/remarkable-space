package musicbridge

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	spotifyAuthorizeURL = "https://accounts.spotify.com/authorize"
	spotifyTokenURL     = "https://accounts.spotify.com/api/token"
	spotifyScopes       = "user-read-playback-state user-modify-playback-state"
)

type SpotifyOAuth struct {
	ClientID    string
	RedirectURI string
	ConfigPath  string
	Client      *http.Client

	mu      sync.Mutex
	token   spotifyToken
	pending map[string]pendingAuthorization
}

type spotifyToken struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
}

type persistedSpotifyToken struct {
	RefreshToken string `json:"refresh_token"`
}

type pendingAuthorization struct {
	Verifier  string
	ExpiresAt time.Time
}

type spotifyTokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
	Description  string `json:"error_description"`
}

func NewSpotifyOAuth(clientID, redirectURI, configPath string, client *http.Client) (*SpotifyOAuth, error) {
	o := &SpotifyOAuth{
		ClientID:    clientID,
		RedirectURI: redirectURI,
		ConfigPath:  configPath,
		Client:      client,
		pending:     make(map[string]pendingAuthorization),
	}
	if configPath == "" {
		return o, nil
	}
	data, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return o, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Spotify token file: %w", err)
	}
	var saved persistedSpotifyToken
	if err := json.Unmarshal(data, &saved); err != nil {
		return nil, fmt.Errorf("parse Spotify token file: %w", err)
	}
	o.token.RefreshToken = saved.RefreshToken
	return o, nil
}

func (o *SpotifyOAuth) AuthorizationURL() (string, error) {
	if o.ClientID == "" {
		return "", errors.New("Spotify client ID is not configured")
	}
	verifier, err := randomURLToken(32)
	if err != nil {
		return "", err
	}
	state, err := randomURLToken(24)
	if err != nil {
		return "", err
	}
	challenge := sha256.Sum256([]byte(verifier))
	params := url.Values{
		"client_id":             {o.ClientID},
		"response_type":         {"code"},
		"redirect_uri":          {o.RedirectURI},
		"scope":                 {spotifyScopes},
		"state":                 {state},
		"code_challenge_method": {"S256"},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	now := time.Now()
	for key, pending := range o.pending {
		if now.After(pending.ExpiresAt) {
			delete(o.pending, key)
		}
	}
	o.pending[state] = pendingAuthorization{Verifier: verifier, ExpiresAt: now.Add(10 * time.Minute)}
	return spotifyAuthorizeURL + "?" + params.Encode(), nil
}

func (o *SpotifyOAuth) CompleteAuthorization(ctx context.Context, state, code string) error {
	o.mu.Lock()
	pending, ok := o.pending[state]
	delete(o.pending, state)
	o.mu.Unlock()
	if !ok || time.Now().After(pending.ExpiresAt) {
		return errors.New("Spotify authorization expired or state did not match")
	}
	if code == "" {
		return errors.New("Spotify authorization did not return a code")
	}
	form := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {o.ClientID},
		"code":          {code},
		"redirect_uri":  {o.RedirectURI},
		"code_verifier": {pending.Verifier},
	}
	token, err := o.exchange(ctx, form)
	if err != nil {
		return err
	}
	if token.RefreshToken == "" {
		return errors.New("Spotify did not return a refresh token; authorize again")
	}
	if err := o.persistRefreshToken(token.RefreshToken); err != nil {
		return err
	}
	o.mu.Lock()
	o.token = token
	o.mu.Unlock()
	return nil
}

func (o *SpotifyOAuth) AccessToken(ctx context.Context) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.token.AccessToken != "" && time.Until(o.token.ExpiresAt) > 30*time.Second {
		return o.token.AccessToken, nil
	}
	if o.token.RefreshToken == "" {
		return "", errors.New("Spotify is not authorized; open the Spotify authorization link on the computer")
	}
	return o.refreshLocked(ctx)
}

func (o *SpotifyOAuth) RefreshAfterUnauthorized(ctx context.Context, failedToken string) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.token.AccessToken != failedToken && o.token.AccessToken != "" &&
		time.Until(o.token.ExpiresAt) > 30*time.Second {
		return o.token.AccessToken, nil
	}
	if o.token.RefreshToken == "" {
		return "", errors.New("Spotify is not authorized; open the Spotify authorization link on the computer")
	}
	o.token.AccessToken = ""
	return o.refreshLocked(ctx)
}

func (o *SpotifyOAuth) refreshLocked(ctx context.Context) (string, error) {
	form := url.Values{
		"grant_type":    {"refresh_token"},
		"client_id":     {o.ClientID},
		"refresh_token": {o.token.RefreshToken},
	}
	token, err := o.exchange(ctx, form)
	if err != nil {
		return "", err
	}
	if token.RefreshToken == "" {
		token.RefreshToken = o.token.RefreshToken
	}
	if err := o.persistRefreshToken(token.RefreshToken); err != nil {
		return "", err
	}
	o.token = token
	return o.token.AccessToken, nil
}

func (o *SpotifyOAuth) exchange(ctx context.Context, form url.Values) (spotifyToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, spotifyTokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return spotifyToken{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := o.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return spotifyToken{}, fmt.Errorf("Spotify token request failed: %w", err)
	}
	defer resp.Body.Close()
	var result spotifyTokenResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<10)).Decode(&result); err != nil {
		return spotifyToken{}, fmt.Errorf("read Spotify token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message := result.Description
		if message == "" {
			message = result.Error
		}
		if message == "" {
			message = resp.Status
		}
		return spotifyToken{}, fmt.Errorf("Spotify authorization failed: %s", message)
	}
	if result.AccessToken == "" || result.ExpiresIn <= 0 {
		return spotifyToken{}, errors.New("Spotify returned an incomplete access token response")
	}
	return spotifyToken{
		AccessToken:  result.AccessToken,
		RefreshToken: result.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(result.ExpiresIn) * time.Second),
	}, nil
}

func (o *SpotifyOAuth) persistRefreshToken(refreshToken string) error {
	if o.ConfigPath == "" {
		return nil
	}
	dir := filepath.Dir(o.ConfigPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create Spotify config directory: %w", err)
	}
	data, err := json.Marshal(persistedSpotifyToken{RefreshToken: refreshToken})
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".spotify-token-*")
	if err != nil {
		return fmt.Errorf("create Spotify token file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("secure Spotify token file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write Spotify token file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync Spotify token file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close Spotify token file: %w", err)
	}
	if err := os.Rename(tmpName, o.ConfigPath); err != nil {
		return fmt.Errorf("save Spotify token file: %w", err)
	}
	return nil
}

func randomURLToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", fmt.Errorf("generate secure random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}
