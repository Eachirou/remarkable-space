package musicbridge

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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
