package space

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// 一个按协议走的假商店, 够用来验客户端这边的行为。
type fakeStore struct {
	mu sync.Mutex
	// 每台设备当前认的 secret。轮换宽限期内旧的也认, 所以存两个。
	secret, prev map[string]string
	nonce        map[string]string
	registers    int
	tickets      int
	ttl          int
	// 下一次 /ticket 不下发新 secret (模拟服务端不轮换)
	noRotate bool
	// /challenge 直接拒
	challengeStatus int
	challengeBody   string
	indexHits       int
	lastAuth        string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		secret: map[string]string{}, prev: map[string]string{},
		nonce: map[string]string{}, ttl: 300,
	}
}

func (f *fakeStore) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /register", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Code, MachineID string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		defer f.mu.Unlock()
		f.registers++
		s := "secret-" + in.Code + "-" + strconv.Itoa(f.registers)
		f.secret[in.Code], f.prev[in.Code] = s, ""
		writeJSON(w, 200, map[string]any{"secret": s})
	})

	mux.HandleFunc("POST /challenge", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Code string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.challengeStatus != 0 {
			writeJSON(w, f.challengeStatus, map[string]any{"error": f.challengeBody})
			return
		}
		if f.secret[in.Code] == "" {
			writeJSON(w, 401, map[string]any{"error": "没注册过"})
			return
		}
		n := "nonce-" + strconv.Itoa(len(f.nonce)+1)
		f.nonce[in.Code] = n
		writeJSON(w, 200, map[string]any{"nonce": n})
	})

	mux.HandleFunc("POST /ticket", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Code, Nonce, Sig string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.nonce[in.Code] != in.Nonce || in.Nonce == "" {
			writeJSON(w, 400, map[string]any{"error": "挑战码对不上"})
			return
		}
		cur, prev := f.secret[in.Code], f.prev[in.Code]
		switch {
		case in.Sig == sign(cur, in.Nonce):
		case prev != "" && in.Sig == sign(prev, in.Nonce):
			// 宽限期: 设备上次没把新 secret 写下来, 认旧的, 并退回去
			f.secret[in.Code], f.prev[in.Code] = prev, ""
		default:
			writeJSON(w, 401, map[string]any{"error": "签名不对"})
			return
		}
		delete(f.nonce, in.Code) // 一次性
		f.tickets++
		out := map[string]any{"ticket": "tk-" + strconv.Itoa(f.tickets), "expiresIn": f.ttl}
		if !f.noRotate {
			ns := "secret-rotated-" + strconv.Itoa(f.tickets)
			f.prev[in.Code], f.secret[in.Code] = f.secret[in.Code], ns
			out["newSecret"] = ns
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("GET /index", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.indexHits++
		f.lastAuth = r.Header.Get("Authorization")
		if !strings.HasPrefix(f.lastAuth, "Bearer tk-") {
			writeJSON(w, 401, map[string]any{"error": "票不对"})
			return
		}
		writeJSON(w, 200, map[string]any{"apps": []any{map[string]any{"id": "plan"}}})
	})
	return mux
}

func newTestAuth(t *testing.T, srv *httptest.Server) (*StoreAuth, string) {
	t.Helper()
	cred := filepath.Join(t.TempDir(), "device.json")
	id := DeviceIdentity{MachineID: "machine-1", Code: DeviceCode("machine-1")}
	return newStoreAuth(srv.URL, cred, "", id), cred
}

func TestHandshake(t *testing.T) {
	f := newFakeStore()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, credPath := newTestAuth(t, srv)

	tk, err := a.Ticket()
	if err != nil {
		t.Fatalf("握手失败: %v", err)
	}
	if tk == "" {
		t.Fatal("握手成功却没拿到票")
	}
	if f.registers != 1 {
		t.Errorf("注册了 %d 次, 想要 1 次", f.registers)
	}

	// 凭据要落盘, 而且只能 root 自己读 —— 它是冒充这台设备的全部所需
	st, err := os.Stat(credPath)
	if err != nil {
		t.Fatalf("凭据没落盘: %v", err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Errorf("凭据权限 %o, 想要 600", perm)
	}
}

func TestTicketReusedUntilExpiry(t *testing.T) {
	f := newFakeStore()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, _ := newTestAuth(t, srv)

	first, err := a.Ticket()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		got, err := a.Ticket()
		if err != nil {
			t.Fatal(err)
		}
		if got != first {
			t.Fatalf("票在有效期内被换掉了: %q → %q", first, got)
		}
	}
	if f.tickets != 1 {
		t.Errorf("换了 %d 次票, 有效期内应该只换 1 次", f.tickets)
	}
}

// 票快到期时要提前换, 不能卡着最后一秒 —— 取索引的请求在路上就过期了。
func TestTicketRenewedBeforeExpiry(t *testing.T) {
	f := newFakeStore()
	f.ttl = 10 // 比 ticketSkew (30s) 还短, 拿到手就该算过期
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, _ := newTestAuth(t, srv)

	if _, err := a.Ticket(); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Ticket(); err != nil {
		t.Fatal(err)
	}
	if f.tickets != 2 {
		t.Errorf("换了 %d 次票, 短效票每次都该重换", f.tickets)
	}
}

func TestSecretRotates(t *testing.T) {
	f := newFakeStore()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, credPath := newTestAuth(t, srv)

	if _, err := a.Ticket(); err != nil {
		t.Fatal(err)
	}
	first := readCred(t, credPath)
	if first.Secret == "" {
		t.Fatal("凭据里没有 secret")
	}

	a.Invalidate()
	if _, err := a.Ticket(); err != nil {
		t.Fatal(err)
	}
	second := readCred(t, credPath)

	if second.Secret == first.Secret {
		t.Error("握手之后 secret 没有轮换")
	}
	if second.Prev != first.Secret {
		t.Errorf("上一个 secret = %q, 想要 %q —— 丢了它, 轮换没落盘时就再也握不上手",
			second.Prev, first.Secret)
	}
	if f.registers != 1 {
		t.Errorf("注册了 %d 次, 轮换不该触发重新注册", f.registers)
	}
}

// 轮换天然有个缝: 服务端换完了, 设备写盘前被杀。下次要能拿上一个 secret 握上。
func TestFallsBackToPrevSecret(t *testing.T) {
	f := newFakeStore()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, credPath := newTestAuth(t, srv)

	if _, err := a.Ticket(); err != nil {
		t.Fatal(err)
	}
	// 伪造"新 secret 没写下来": 本地还停在上一个上, 服务端已经往前走了
	c := readCred(t, credPath)
	stale := deviceCredential{Code: c.Code, Secret: c.Prev, Prev: c.Prev}
	writeCred(t, credPath, stale)

	a2 := newStoreAuth(srv.URL, credPath, "", a.identity)
	if _, err := a2.Ticket(); err != nil {
		t.Fatalf("拿上一个 secret 没能握上手: %v", err)
	}
	if f.registers != 1 {
		t.Errorf("注册了 %d 次 —— 本该用上一个 secret 接上, 而不是重新注册", f.registers)
	}
}

// 本地凭据彻底对不上 (服务端重置过, 或这份是从别处拷来的) 时重新注册, 且只注册一次。
func TestReregistersOnBadSecret(t *testing.T) {
	f := newFakeStore()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, credPath := newTestAuth(t, srv)

	writeCred(t, credPath, deviceCredential{Code: a.identity.Code, Secret: "完全不对的 secret"})
	// 服务端也不认这台设备, 于是 /challenge 先给 401
	if _, err := a.Ticket(); err != nil {
		t.Fatalf("应当自动重新注册后握上手, 却失败了: %v", err)
	}
	if f.registers != 1 {
		t.Errorf("注册了 %d 次, 想要 1 次 (不能反复重试把服务端刷爆)", f.registers)
	}
}

// 凭据属于别的设备 —— 整个 space 目录被拷到另一台机器上就是这种情况。
func TestRejectsCredentialOfAnotherDevice(t *testing.T) {
	f := newFakeStore()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, credPath := newTestAuth(t, srv)

	writeCred(t, credPath, deviceCredential{Code: "XXXX-YYYY-ZZZZ", Secret: "别人的"})
	if _, err := a.Ticket(); err != nil {
		t.Fatalf("应当忽略别人的凭据重新注册: %v", err)
	}
	got := readCred(t, credPath)
	if got.Code != a.identity.Code {
		t.Errorf("凭据里的设备码 = %q, 想要 %q", got.Code, a.identity.Code)
	}
}

// 读不到机器标识时必须明确失败。退而求其次编个固定身份的话,
// 所有读不到的设备会共用一个身份, 封一台等于封一片。
func TestNoMachineIDNoTicket(t *testing.T) {
	f := newFakeStore()
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a := newStoreAuth(srv.URL, filepath.Join(t.TempDir(), "d.json"), "", DeviceIdentity{})

	_, err := a.Ticket()
	if err == nil {
		t.Fatal("没有机器标识却拿到了票")
	}
	if !strings.Contains(err.Error(), "机器标识") {
		t.Errorf("错误信息没说清原因: %v", err)
	}
	if f.registers != 0 {
		t.Error("没有身份就不该去注册")
	}
}

// 服务端的拒绝理由要原样透给用户, 不能只剩一个状态码。
func TestSurfacesServerReason(t *testing.T) {
	f := newFakeStore()
	f.challengeStatus, f.challengeBody = http.StatusForbidden, "这台设备的授权已停用"
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, credPath := newTestAuth(t, srv)
	writeCred(t, credPath, deviceCredential{Code: a.identity.Code, Secret: "x"})

	_, err := a.Ticket()
	if err == nil {
		t.Fatal("被停用却拿到了票")
	}
	if !strings.Contains(err.Error(), "授权已停用") {
		t.Errorf("没把服务端的理由带出来: %v", err)
	}
}

// 服务端不下发新 secret 时保持原样, 别把 Prev 搞乱。
func TestNoRotationKeepsSecret(t *testing.T) {
	f := newFakeStore()
	f.noRotate = true
	srv := httptest.NewServer(f.handler())
	defer srv.Close()
	a, credPath := newTestAuth(t, srv)

	if _, err := a.Ticket(); err != nil {
		t.Fatal(err)
	}
	before := readCred(t, credPath)
	a.Invalidate()
	if _, err := a.Ticket(); err != nil {
		t.Fatal(err)
	}
	after := readCred(t, credPath)
	if after.Secret != before.Secret || after.Prev != before.Prev {
		t.Errorf("服务端没轮换, 本地却动了: %+v → %+v", before, after)
	}
}

func readCred(t *testing.T, p string) deviceCredential {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("读凭据: %v", err)
	}
	var c deviceCredential
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatalf("解析凭据: %v", err)
	}
	return c
}

func writeCred(t *testing.T, p string, c deviceCredential) {
	t.Helper()
	c.Updated = time.Now()
	b, _ := json.Marshal(c)
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
}
