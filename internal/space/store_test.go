package space

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

// 端到端: 编译期 -X 注入的密钥要一路走到请求头上。
// -X 的包路径或变量名写错时 Go 不报错、静默不注入, 光看编译通过发现不了,
// 所以这条链得真的跑一遍。平时不带密钥编译时自动跳过。
func TestInjectedKeyReachesRequest(t *testing.T) {
	if storeKey == "" {
		t.Skip("没有编译期注入的密钥, 跳过 (go test -ldflags \"-X ...storeKey=X\" 才测得到)")
	}
	var got string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("X-Space-Key")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apps":[]}`))
	}))
	defer up.Close()

	h := New(t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir(), "aarch64", "http://127.0.0.1:1")
	h.StoreURL = up.URL
	h.store(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/space/store", nil))

	if got != storeKey {
		t.Fatalf("请求头里的密钥 = %q, 想要 %q (注入没生效, 或 New 没把它接上)", got, storeKey)
	}
}

// 配了握手接口就走握手, 索引请求要带票。
func TestStoreUsesHandshakeWhenAPIConfigured(t *testing.T) {
	f := newFakeStore()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()

	h := &Handler{StoreAPI: srv.URL}
	h.SetupAuth(filepath.Join(t.TempDir(), "device.json"))
	h.auth.identity = DeviceIdentity{MachineID: "m1", Code: DeviceCode("m1")}

	rec := httptest.NewRecorder()
	h.store(rec, httptest.NewRequest(http.MethodGet, "/space/store", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("状态码 = %d (body: %s)", rec.Code, rec.Body)
	}
	if f.indexHits != 1 {
		t.Errorf("取了 %d 次索引, 想要 1 次", f.indexHits)
	}
	if !strings.HasPrefix(f.lastAuth, "Bearer ") {
		t.Errorf("索引请求没带票: %q", f.lastAuth)
	}
	if !strings.Contains(rec.Body.String(), "plan") {
		t.Errorf("索引内容没透出来: %s", rec.Body)
	}
}

// 没配握手接口 = 自建商店, 索引就是一份静态 JSON, 不该去握手。
func TestStoreStaysStaticWithoutAPI(t *testing.T) {
	var hits int
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"apps":[{"id":"mine"}]}`))
	}))
	defer up.Close()

	h := &Handler{StoreURL: up.URL}
	h.SetupAuth(filepath.Join(t.TempDir(), "device.json")) // 没有 StoreAPI, 应当什么也不装
	if h.auth != nil {
		t.Fatal("没配握手接口却装上了握手客户端")
	}

	rec := httptest.NewRecorder()
	h.store(rec, httptest.NewRequest(http.MethodGet, "/space/store", nil))
	if rec.Code != http.StatusOK || hits != 1 {
		t.Fatalf("状态码 = %d, 上游命中 %d 次", rec.Code, hits)
	}
}
