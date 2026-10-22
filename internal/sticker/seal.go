package sticker

// 商店下发的贴纸包是按设备加密 + 签名的专属包 (格式见 remarkable-space-store/internal/store/sticker.go):
// 每个 .rm/.png 存成 <文件名>.enc (AES-256-GCM), 包目录里一个 .seal 记着设备码、包装后的密钥和商店签名。
// 这里用内置公钥验签、核对本机设备码, 解出密钥后用到哪个文件才在内存里解密。
// 不带 .seal 的包 (空间自带的手绘、数字日期) 照常明文读。验不过的包整个不显示。
// 上限: root 用户读得到 .seal、本仓库源码公开; 防的是「整包拷到别的机器」和「在电脑上直接打开图片」。

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/boangs/remarkable-space/internal/space"
)

// sealPublicKey 商店签名私钥对应的公钥 (和应用防拷 lib/integrity 是同一把)。是变量只为测试能换。
var sealPublicKey = "102MVdUk+novne+iJwGaD54yHKe0NoKh6dm+n5unJ3M="

// SealFile 包目录里的签名文件
const SealFile = ".seal"

// Incomplete 验不过时写进日志的话 (不提别的, 用户的要求)
const Incomplete = "贴纸包不完整，请在商店重新安装"

var errSeal = errors.New(Incomplete)

// 本机设备码 (机器不变, 算一次); 测试里可换
var localCode = sync.OnceValue(func() string { return space.DeviceCode(space.MachineID()) })

type sealRecord struct {
	V    int    `json:"v"`
	Pack string `json:"p"`
	Dev  string `json:"c"`
	K    string `json:"k"`
	Sig  string `json:"s"`
}

func stickerWrap(code string) []byte {
	s := sha256.Sum256([]byte("rmkit-space-sticker-wrap-v1\x00" + code))
	return s[:]
}

// openSeal 验一份 .seal, 通过就返回这个包的内容密钥
func openSeal(raw []byte, pack, code string, pub ed25519.PublicKey) ([]byte, error) {
	var r sealRecord
	if json.Unmarshal(raw, &r) != nil || r.V != 1 || code == "" || r.Pack != pack || r.Dev != code {
		return nil, errSeal
	}
	sig, err := base64.StdEncoding.DecodeString(r.Sig)
	if err != nil || !ed25519.Verify(pub, []byte("rmkit-space-sticker-v1\x00"+r.Pack+"\x00"+r.Dev+"\x00"+r.K), sig) {
		return nil, errSeal
	}
	kw, err := base64.StdEncoding.DecodeString(r.K)
	if err != nil || len(kw) != 32 {
		return nil, errSeal
	}
	w := stickerWrap(code)
	k := make([]byte, 32)
	for i := range k {
		k[i] = kw[i] ^ w[i]
	}
	return k, nil
}

type packSeal struct {
	mtime  time.Time
	sealed bool
	gcm    cipher.AEAD // 验过了才有
}

var (
	sealMu    sync.Mutex
	sealCache = map[string]*packSeal{}
)

// packState 包目录的加密状态: sealed=false 是明文包; sealed=true 且 gcm!=nil 是验过的专属包; gcm==nil 是验不过。
func packState(dir, pack string) *packSeal {
	p := filepath.Join(dir, pack, SealFile)
	st, err := os.Stat(p)
	sealMu.Lock()
	defer sealMu.Unlock()
	if err != nil {
		return &packSeal{}
	}
	if c := sealCache[p]; c != nil && c.mtime.Equal(st.ModTime()) {
		return c
	}
	c := &packSeal{mtime: st.ModTime(), sealed: true}
	raw, err := os.ReadFile(p)
	pub, _ := base64.StdEncoding.DecodeString(sealPublicKey)
	if err == nil {
		if k, err := openSeal(raw, pack, localCode(), ed25519.PublicKey(pub)); err == nil {
			if blk, err := aes.NewCipher(k); err == nil {
				c.gcm, _ = cipher.NewGCM(blk)
			}
		}
	}
	if c.gcm == nil {
		log.Printf("贴纸: %s %s", pack, Incomplete)
	}
	sealCache[p] = c
	return c
}

// usable 这个包能不能用 (明文包, 或验过的专属包)
func usable(dir, pack string) bool {
	s := packState(dir, pack)
	return !s.sealed || s.gcm != nil
}

// statFile 贴纸里的一个文件 (rel 形如 "包/名.png") 是否存在, 明文或加密的都算; 返回修改时间
func statFile(dir, rel string) (time.Time, bool) {
	for _, p := range []string{filepath.Join(dir, rel), filepath.Join(dir, rel) + ".enc"} {
		if st, err := os.Stat(p); err == nil {
			return st.ModTime(), true
		}
	}
	return time.Time{}, false
}

// ReadFile 读贴纸里的一个文件: 有明文读明文 (自带的包), 否则读 .enc 并用这台设备的包密钥解密
func ReadFile(dir, rel string) ([]byte, error) {
	rel = filepath.ToSlash(filepath.Clean(rel))
	pack, _, ok := strings.Cut(rel, "/")
	if !ok {
		return nil, os.ErrNotExist
	}
	s := packState(dir, pack)
	if !s.sealed {
		return os.ReadFile(filepath.Join(dir, rel))
	}
	if s.gcm == nil {
		return nil, errSeal
	}
	b, err := os.ReadFile(filepath.Join(dir, rel) + ".enc")
	if err != nil {
		return nil, err
	}
	n := s.gcm.NonceSize()
	if len(b) < n {
		return nil, errSeal
	}
	pt, err := s.gcm.Open(nil, b[:n], b[n:], []byte(rel))
	if err != nil {
		return nil, errSeal
	}
	return pt, nil
}

// Sealed 这个包是不是加密的专属包 (加密的包不往磁盘缓存原图尺寸的图片)
func Sealed(dir, pack string) bool { return packState(dir, pack).sealed }
