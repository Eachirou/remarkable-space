package mediabridge

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseLists(t *testing.T) {
	feeds := ParseList("News=https://example.com/news.xml,Invalid,Talk=https://example.com/talk.xml")
	if len(feeds) != 2 || feeds[0].Name != "News" || feeds[1].URL != "https://example.com/talk.xml" {
		t.Fatalf("unexpected feeds: %#v", feeds)
	}
	stations := ParseStations("Jazz=https://radio.example/jazz.mp3")
	if len(stations) != 1 || stations[0].Name != "Jazz" {
		t.Fatalf("unexpected stations: %#v", stations)
	}
}

func TestHealthAndAuthentication(t *testing.T) {
	s := &Server{Token: "secret"}
	server := httptest.NewServer(s.Handler())
	defer server.Close()
	resp, err := http.Get(server.URL + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("health: %v, %v", err, resp.Status)
	}
	_ = resp.Body.Close()
	resp, err = http.Get(server.URL + "/api/radio/stations")
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("auth: %v, %v", err, resp.Status)
	}
	_ = resp.Body.Close()
}
