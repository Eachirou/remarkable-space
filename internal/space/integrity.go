package space

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// SealPublicKey 商店签名私钥对应的公钥 (专属包 .integrity、贴纸包 .seal、空间自更新包共用一把)。
// 是变量只为测试能换。
var SealPublicKey = "102MVdUk+novne+iJwGaD54yHKe0NoKh6dm+n5unJ3M="

// VerifyIntegrity 验应用目录里商店打的 .integrity: 签名对、应用 id 对、设备码是本机。
// 格式见 remarkable-space-store/internal/store/integrity.go (应用侧同款在 apps 仓库 lib/integrity)。
func VerifyIntegrity(appDir, appID string) error {
	b, err := os.ReadFile(filepath.Join(appDir, ".integrity"))
	if err != nil {
		return errors.New("没有商店签名")
	}
	var r struct {
		V   int    `json:"v"`
		App string `json:"a"`
		Dev string `json:"c"`
		Sig string `json:"s"`
	}
	if json.Unmarshal(b, &r) != nil || r.V != 1 || r.App != appID || r.Dev == "" || r.Dev != DeviceCode(MachineID()) {
		return errors.New("商店签名不是这台设备的")
	}
	sig, err := base64.StdEncoding.DecodeString(r.Sig)
	pub, _ := base64.StdEncoding.DecodeString(SealPublicKey)
	if err != nil || !ed25519.Verify(ed25519.PublicKey(pub), []byte("rmkit-space-integrity-v1\x00"+r.App+"\x00"+r.Dev), sig) {
		return errors.New("商店签名不对")
	}
	return nil
}
