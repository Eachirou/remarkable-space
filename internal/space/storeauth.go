package space

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 商店握手。设备取索引之前要走三步, 每一步验的都是服务端拿得出判断依据的东西,
// 而不是"客户端说自己是什么" —— 后者改个字段就伪造了, 分几步发也一样。
//
//	① 注册   POST {api}/register  {machineId, code, model, os}
//	         服务端记下这台设备, 返回它专属的 secret。只在第一次 (或 secret 失效时) 做。
//	② 挑战   POST {api}/challenge {code}
//	         服务端查: 注册过吗 / 封了吗 / 超限了吗 → 返回一次性 nonce。
//	③ 换票   POST {api}/ticket    {code, nonce, sig}   sig = HMAC-SHA256(secret, nonce)
//	         验签通过 → 返回短期票, 并下发新 secret (轮换)。
//	④ 取索引 GET  {api}/index     Authorization: Bearer <票>
//
// secret 永远不出现在请求里, 只用来算 sig —— 这是三步握手唯一有意义的地方。
// 它落在本地文件里: 冒充的门槛因此从"知道设备码"(公开可读) 抬到"能访问这台设备的
// 文件系统"。再加上每次握手后轮换, 拷走的副本会和真设备互相顶替, 几小时内必然撞车,
// 服务端看到同一设备码两份 secret 交替出现就知道被复制了。
//
// 挡不住的: 有真设备的人导出 secret 自己用。那只能靠服务端的归因与限速去治,
// 客户端做不了 —— 它整个运行环境都在对方手里。

const (
	ticketSkew    = 30 * time.Second // 票快到期就提前换, 别卡着最后一秒用
	handshakeWait = 20 * time.Second
)

// deviceCredential 落盘的那份。
type deviceCredential struct {
	Code   string `json:"code"`
	Secret string `json:"secret"`
	// 上一个 secret。轮换有个天然的缝: 服务端换完了, 设备写盘前断电/进程被杀,
	// 两边就对不上, 这台设备再也握不上手。留着上一个, 新的不行时拿它重试一次。
	// 服务端那边也要在宽限期内同时认这两个 —— 协议见 store-server 的 README。
	Prev    string    `json:"prev,omitempty"`
	Updated time.Time `json:"updated"`
}

// StoreAuth 管一台设备在商店这边的身份与票。零值不可用, 走 newStoreAuth。
type StoreAuth struct {
	api      string // 形如 https://boangs.com/rmkit-space/api
	credPath string
	key      string // 最外层的粗筛头, 和握手无关
	identity DeviceIdentity
	client   *http.Client

	mu        sync.Mutex
	cred      *deviceCredential
	ticket    string
	ticketExp time.Time
}

func newStoreAuth(api, credPath, key string, id DeviceIdentity) *StoreAuth {
	return &StoreAuth{
		api: api, credPath: credPath, key: key, identity: id,
		client: &http.Client{Timeout: handshakeWait},
	}
}

// Ticket 返回一张能用的票, 必要时现握手换一张。
func (a *StoreAuth) Ticket() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.ticket != "" && time.Now().Add(ticketSkew).Before(a.ticketExp) {
		return a.ticket, nil
	}
	if a.identity.MachineID == "" {
		// 读不到机器标识就没有身份可言。这里绝不能退而求其次编一个固定值 ——
		// 那会让所有读不到的设备共用同一个身份, 封一台等于封一片。
		return "", fmt.Errorf("读不到这台设备的机器标识, 无法连接应用商店")
	}

	if err := a.loadCred(); err != nil {
		return "", err
	}
	tk, err := a.handshake()
	if err == nil {
		return tk, nil
	}
	// secret 对不上 —— 可能是服务端那边重置了, 也可能本地这份是从别处拷来的旧货。
	// 重新注册一次, 只试这一次, 免得反复失败时把服务端刷爆。
	if isAuthErr(err) {
		if rerr := a.register(); rerr != nil {
			return "", rerr
		}
		return a.handshake()
	}
	return "", err
}

// Invalidate 丢掉手上的票。取索引时拿到 401 就调它, 下次自然重新握手。
func (a *StoreAuth) Invalidate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ticket, a.ticketExp = "", time.Time{}
}

func (a *StoreAuth) handshake() (string, error) {
	if a.cred == nil || a.cred.Secret == "" {
		if err := a.register(); err != nil {
			return "", err
		}
	}

	var ch struct {
		Nonce string `json:"nonce"`
	}
	if err := a.post("/challenge", map[string]any{"code": a.identity.Code}, &ch); err != nil {
		return "", err
	}
	if ch.Nonce == "" {
		return "", fmt.Errorf("应用商店没有下发挑战码")
	}

	var tk struct {
		Ticket    string `json:"ticket"`
		ExpiresIn int    `json:"expiresIn"` // 秒
		NewSecret string `json:"newSecret"`
	}
	body := map[string]any{
		"code":  a.identity.Code,
		"nonce": ch.Nonce,
		"sig":   sign(a.cred.Secret, ch.Nonce),
	}
	err := a.post("/ticket", body, &tk)
	if err != nil && isAuthErr(err) && a.cred.Prev != "" {
		// 上回轮换没落盘成功, 服务端还停在上一个 secret 上。拿它再试一次。
		body["sig"] = sign(a.cred.Prev, ch.Nonce)
		if err2 := a.post("/ticket", body, &tk); err2 == nil {
			a.cred.Secret, err = a.cred.Prev, nil
		}
	}
	if err != nil {
		return "", err
	}
	if tk.Ticket == "" {
		return "", fmt.Errorf("应用商店没有下发票据")
	}

	if tk.NewSecret != "" && tk.NewSecret != a.cred.Secret {
		a.cred.Prev, a.cred.Secret = a.cred.Secret, tk.NewSecret
		if err := a.saveCred(); err != nil {
			// 写盘失败不该让这次握手白费: 票是好的, 先用着。下次拿 Prev 还能握上。
			logStore("新 secret 没能写进 %s: %v", a.credPath, err)
		}
	}

	ttl := time.Duration(tk.ExpiresIn) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	a.ticket, a.ticketExp = tk.Ticket, time.Now().Add(ttl)
	return a.ticket, nil
}

func (a *StoreAuth) register() error {
	var out struct {
		Secret string `json:"secret"`
	}
	req := map[string]any{
		"machineId": a.identity.MachineID,
		"code":      a.identity.Code,
		"shell":     ShellVersion,
	}
	if a.identity.Serial != "" {
		req["serial"] = a.identity.Serial
	}
	if err := a.post("/register", req, &out); err != nil {
		return err
	}
	if out.Secret == "" {
		return fmt.Errorf("应用商店注册没有返回凭据")
	}
	a.cred = &deviceCredential{Code: a.identity.Code, Secret: out.Secret, Updated: time.Now()}
	if err := a.saveCred(); err != nil {
		logStore("注册凭据没能写进 %s: %v", a.credPath, err)
	}
	return nil
}

type storeHTTPError struct {
	status int
	msg    string
}

func (e *storeHTTPError) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("应用商店返回 HTTP %d", e.status)
}

func isAuthErr(err error) bool {
	var he *storeHTTPError
	if errors.As(err, &he) {
		return he.status == http.StatusUnauthorized || he.status == http.StatusForbidden
	}
	return false
}

func logStore(format string, args ...any) {
	log.Printf("space/store: "+format, args...)
}

func (a *StoreAuth) post(path string, body any, out any) error {
	buf, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, a.api+path, bytes.NewReader(buf))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if a.key != "" {
		req.Header.Set("X-Space-Key", a.key)
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("连不上应用商店: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return &storeHTTPError{status: resp.StatusCode, msg: storeErrMsg(resp.StatusCode, raw)}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("应用商店返回的内容看不懂: %w", err)
	}
	return nil
}

// 服务端的拒绝理由要原样带给用户 —— "这台设备被停用了" 和 "连不上" 得分得清,
// 否则用户只能看到一个状态码, 我们也无从判断该让他做什么。
func storeErrMsg(status int, raw []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(raw, &e) == nil && e.Error != "" {
		return e.Error
	}
	switch status {
	case http.StatusTooManyRequests:
		return "这台设备访问应用商店太频繁, 过一会儿再试"
	case http.StatusForbidden:
		return "这台设备的应用商店访问已被停用"
	}
	return ""
}

func sign(secret, nonce string) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(nonce))
	return hex.EncodeToString(m.Sum(nil))
}

func (a *StoreAuth) loadCred() error {
	if a.cred != nil {
		return nil
	}
	b, err := os.ReadFile(a.credPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 还没注册过, handshake 里会补
		}
		return err
	}
	var c deviceCredential
	if err := json.Unmarshal(b, &c); err != nil {
		logStore("%s 内容坏了, 当作没注册过重来: %v", a.credPath, err)
		return nil
	}
	// 换了台机器 (或者这份是从别的设备拷来的): 本地这份对不上当前设备码, 不能用。
	if c.Code != a.identity.Code {
		logStore("本地凭据属于设备 %s, 当前设备是 %s, 重新注册", c.Code, a.identity.Code)
		return nil
	}
	a.cred = &c
	return nil
}

// 先写临时文件再改名: 轮换时如果直接截断原文件写到一半断电, 两边的 secret 就都没了,
// 这台设备从此握不上手。改名是原子的, 要么旧的要么新的, 不会出半截。
func (a *StoreAuth) saveCred() error {
	if a.cred == nil {
		return nil
	}
	a.cred.Updated = time.Now()
	b, err := json.MarshalIndent(a.cred, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(a.credPath), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(a.credPath), ".device-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil { // 不 fsync 的话 rename 可能先于数据落盘
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), a.credPath)
}
