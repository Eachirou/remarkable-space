package sticker

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// 和商店服务端 internal/store/sticker_test.go 同一组固定向量: 种子 0..31, 设备码 ABCD-EFGH-JKLM, 包 nature
const vecK = "S34OI2OjVv9CM2ahbPBnsI/zW1MacmHF20H/2XiSoHo="

func testKey() ed25519.PrivateKey {
	seed := make([]byte, 32)
	for i := range seed {
		seed[i] = byte(i)
	}
	return ed25519.NewKeyFromSeed(seed)
}

func testRecord(key ed25519.PrivateKey, code, pack, kb64 string) []byte {
	sig := ed25519.Sign(key, []byte("rmkit-space-sticker-v1\x00"+pack+"\x00"+code+"\x00"+kb64))
	b, _ := json.Marshal(sealRecord{V: 1, Pack: pack, Dev: code, K: kb64, Sig: base64.StdEncoding.EncodeToString(sig)})
	return b
}

func TestOpenSealVector(t *testing.T) {
	key := testKey()
	pub := key.Public().(ed25519.PublicKey)
	k, err := openSeal(testRecord(key, "ABCD-EFGH-JKLM", "nature", vecK), "nature", "ABCD-EFGH-JKLM", pub)
	if err != nil {
		t.Fatal(err)
	}
	// 解出来的密钥必须等于商店那边的派生公式
	m := hmac.New(sha256.New, key.Seed())
	m.Write([]byte("rmkit-space-sticker-key-v1\x00ABCD-EFGH-JKLM\x00nature"))
	if !hmac.Equal(k, m.Sum(nil)) {
		t.Fatal("密钥和商店派生的不一致")
	}
	// 换设备 / 换包 / 改字段都要拒
	if _, err := openSeal(testRecord(key, "ABCD-EFGH-JKLM", "nature", vecK), "nature", "ZZZZ-EFGH-JKLM", pub); err == nil {
		t.Fatal("别的设备不该通过")
	}
	if _, err := openSeal(testRecord(key, "ABCD-EFGH-JKLM", "nature", vecK), "plan", "ABCD-EFGH-JKLM", pub); err == nil {
		t.Fatal("别的包不该通过")
	}
	other := ed25519.NewKeyFromSeed(make([]byte, 32))
	if _, err := openSeal(testRecord(other, "ABCD-EFGH-JKLM", "nature", vecK), "nature", "ABCD-EFGH-JKLM", pub); err == nil {
		t.Fatal("别的私钥签的不该通过")
	}
}

// 往返: 按商店格式加密一个包, ReadFile 能解; 拷到别的设备码就读不到
func TestSealedPackRoundTrip(t *testing.T) {
	key := testKey()
	dir := t.TempDir()
	code := "ABCD-EFGH-JKLM"
	pack := "nature"
	m := hmac.New(sha256.New, key.Seed())
	m.Write([]byte("rmkit-space-sticker-key-v1\x00" + code + "\x00" + pack))
	k := m.Sum(nil)
	w := stickerWrap(code)
	kw := make([]byte, 32)
	for i := range kw {
		kw[i] = k[i] ^ w[i]
	}
	blk, _ := aes.NewCipher(k)
	gcm, _ := cipher.NewGCM(blk)
	plain := []byte("hello sticker")
	nonce := make([]byte, gcm.NonceSize())
	rand.Read(nonce)
	os.MkdirAll(filepath.Join(dir, pack), 0o755)
	os.WriteFile(filepath.Join(dir, pack, "leaf.png.enc"), gcm.Seal(nonce, nonce, plain, []byte(pack+"/leaf.png")), 0o644)
	os.WriteFile(filepath.Join(dir, pack, SealFile), testRecord(key, code, pack, base64.StdEncoding.EncodeToString(kw)), 0o644)

	old, oldCode := sealPublicKey, localCode
	defer func() { sealPublicKey, localCode = old, oldCode }()
	sealPublicKey = base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))

	localCode = func() string { return code }
	sealCache = map[string]*packSeal{}
	got, err := ReadFile(dir, pack+"/leaf.png")
	if err != nil || string(got) != string(plain) {
		t.Fatalf("解密失败: %v %q", err, got)
	}
	localCode = func() string { return "ZZZZ-YYYY-XXXX" }
	sealCache = map[string]*packSeal{}
	if _, err := ReadFile(dir, pack+"/leaf.png"); err == nil {
		t.Fatal("拷到别的设备不该读得到")
	}
	if usable(dir, pack) {
		t.Fatal("别的设备上这个包应该整个不显示")
	}
}

// 列表里加密贴纸的路径不能带 .enc (面板拿它去要缩略图/图片, 后端只认 .rm)
func TestListSealedRel(t *testing.T) {
	key := testKey()
	dir := t.TempDir()
	code, pack := "ABCD-EFGH-JKLM", "nature"
	m := hmac.New(sha256.New, key.Seed())
	m.Write([]byte("rmkit-space-sticker-key-v1\x00" + code + "\x00" + pack))
	k := m.Sum(nil)
	w := stickerWrap(code)
	kw := make([]byte, 32)
	for i := range kw {
		kw[i] = k[i] ^ w[i]
	}
	os.MkdirAll(filepath.Join(dir, pack), 0o755)
	os.WriteFile(filepath.Join(dir, pack, "leaf.rm.enc"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, pack, SealFile), testRecord(key, code, pack, base64.StdEncoding.EncodeToString(kw)), 0o644)
	old, oldCode := sealPublicKey, localCode
	defer func() { sealPublicKey, localCode = old, oldCode }()
	sealPublicKey = base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	localCode = func() string { return code }
	sealCache = map[string]*packSeal{}
	packs, err := List(dir)
	if err != nil || len(packs) != 1 || len(packs[0].Stickers) != 1 {
		t.Fatalf("列表不对: %v %+v", err, packs)
	}
	if got := packs[0].Stickers[0].Rel; got != "nature/leaf.rm" {
		t.Fatalf("Rel = %q, 应为 nature/leaf.rm", got)
	}
	if packs[0].Icon != "nature/leaf.rm" {
		t.Fatalf("Icon = %q", packs[0].Icon)
	}
}
