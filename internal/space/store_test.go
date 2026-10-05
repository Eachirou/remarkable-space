package space

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 官方商店靠一个请求头认客户端, 不带就 404。这里钉住两件事:
// 带了密钥要原样发出去, 没配密钥时不能凭空造一个头 (自建商店不该被这道门挡住)。
func TestStoreSendsKeyHeader(t *testing.T) {
	for _, tc := range []struct {
		name, key, want string
	}{
		{"配了密钥就带上", "K123", "K123"},
		{"没配就不带头", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			var seen bool
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got, seen = r.Header.Get("X-Space-Key"), true
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"apps":[]}`))
			}))
			defer upstream.Close()

			h := &Handler{StoreURL: upstream.URL, StoreKey: tc.key}
			rec := httptest.NewRecorder()
			h.store(rec, httptest.NewRequest(http.MethodGet, "/space/store", nil))

			if rec.Code != http.StatusOK {
				t.Fatalf("状态码 = %d, 想要 200 (body: %s)", rec.Code, rec.Body)
			}
			if !seen {
				t.Fatal("上游没收到请求")
			}
			if got != tc.want {
				t.Errorf("X-Space-Key = %q, 想要 %q", got, tc.want)
			}
		})
	}
}

// 密钥没对上时商店返回 404, 得让用户看到一句能看懂的话, 而不是干巴巴一个状态码。
func TestStoreExplains404(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer upstream.Close()

	h := &Handler{StoreURL: upstream.URL, StoreKey: "wrong"}
	rec := httptest.NewRecorder()
	h.store(rec, httptest.NewRequest(http.MethodGet, "/space/store", nil))

	if body := rec.Body.String(); !contains(body, "商店密钥") {
		t.Errorf("404 的提示没提到密钥, 用户看不懂: %s", body)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
