package musicbridge

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHandlerRequiresToken(t *testing.T) {
	s := &Server{BridgeToken: "secret"}
	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestControlRejectsUnknownAction(t *testing.T) {
	s := &Server{BridgeToken: "secret"}
	req := httptest.NewRequest(http.MethodPost, "/api/control", strings.NewReader(`{"action":"seek"}`))
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPairingIssuesPersistentDeviceToken(t *testing.T) {
	pairFile := filepath.Join(t.TempDir(), "paired-device.json")
	s := &Server{PairCode: "01234567", PairExpires: time.Now().Add(time.Minute), PairFile: pairFile}
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/pair", strings.NewReader(`{"code":"01234567"}`))
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("pair status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var response struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Token == "" {
		t.Fatal("pair response did not contain a device token")
	}

	data, err := os.ReadFile(pairFile)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), response.Token) {
		t.Fatal("paired-device file stores the bearer token instead of its hash")
	}
	info, err := os.Stat(pairFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("paired-device file permissions = %o, want 600", info.Mode().Perm())
	}

	restarted := &Server{PairFile: pairFile}
	if err := restarted.Init(); err != nil {
		t.Fatal(err)
	}
	authReq := httptest.NewRequest(http.MethodPost, "/api/control", strings.NewReader(`{"action":"play"}`))
	authReq.Header.Set("Authorization", "Bearer "+response.Token)
	authRec := httptest.NewRecorder()
	restarted.Handler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("authenticated control status = %d, want %d (authentication should pass)", authRec.Code, http.StatusServiceUnavailable)
	}

	reuseReq := httptest.NewRequest(http.MethodPost, "/api/pair", strings.NewReader(`{"code":"01234567"}`))
	reuseRec := httptest.NewRecorder()
	s.Handler().ServeHTTP(reuseRec, reuseReq)
	if reuseRec.Code != http.StatusGone {
		t.Fatalf("reused pairing code status = %d, want %d", reuseRec.Code, http.StatusGone)
	}
}

func TestPairingRejectsExpiredAndIncorrectCodes(t *testing.T) {
	for _, tc := range []struct {
		name        string
		code        string
		expires     time.Time
		wantStatus  int
		requestCode string
	}{
		{name: "incorrect", code: "12345678", expires: time.Now().Add(time.Minute), requestCode: "87654321", wantStatus: http.StatusUnauthorized},
		{name: "expired", code: "12345678", expires: time.Now().Add(-time.Minute), requestCode: "12345678", wantStatus: http.StatusGone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{PairCode: tc.code, PairExpires: tc.expires}
			req := httptest.NewRequest(http.MethodPost, "/api/pair", strings.NewReader(`{"code":"`+tc.requestCode+`"}`))
			rec := httptest.NewRecorder()
			s.Handler().ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d: %s", rec.Code, tc.wantStatus, rec.Body.String())
			}
		})
	}
}
