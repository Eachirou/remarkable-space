package space

import (
	"crypto/sha256"
	"encoding/base32"
	"os"
	"strings"
)

// 这台机器的身份。两个号，用途不同:
//
//	MachineID  内部主键, 取 SoC 烧录的序列号。出厂重置、退出账号、刷机都不变,
//	           和 reMarkable 账号完全无关 —— 账号在初始设置里可以 "Skip pairing"
//	           跳过, 跳过了 xochitl.conf 里就没有 devicetoken, 官方序列号也就读不到。
//	           拿账号里的东西当授权依据, 用户一退出登录授权就没了。
//	Code       给人看的短码, 由 MachineID 派生。用户买应用时抄这个号给我们,
//	           所以要好念好抄: 大写 base32 去掉易混的字符, 每四位一组。
type DeviceIdentity struct {
	MachineID string `json:"machineId"`
	Code      string `json:"code"`
	// 官方序列号 (形如 RM03A00522431KE), 仅作后台对账时认人用, 不参与授权判定。
	// 故意不自动读: 它藏在 xochitl.conf 的 devicetoken 里, 那是一个 JWT 账号凭证,
	// 除了序列号还带着账号标识 —— 为了一个不参与授权的备注字段去解析用户的账号
	// 凭证不划算。需要时让用户自己报 (机身上印着), 或在设置里手填。
	Serial string `json:"serial,omitempty"`
}

// machineIDSources 按可靠性排序。前两个是 SoC 烧录的, 最后一个是 systemd 生成的 ——
// 在 RMPP 上它恰好等于 soc0 那个值, 但别的机型未必, 所以只当兜底。
var machineIDSources = []string{
	"/proc/device-tree/serial-number",
	"/sys/devices/soc0/serial_number",
	"/etc/machine-id",
}

// MachineID 读这台机器的硬件标识。读不到任何一个来源时返回空字符串 ——
// 调用方要把它当"这台设备不能参与授权"处理, 而不是拿空串当一个合法身份。
func MachineID() string {
	for _, p := range machineIDSources {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		// device-tree 里的字符串以 NUL 结尾, 直接用会把结束符带进哈希
		if v := strings.TrimSpace(strings.TrimRight(string(b), "\x00")); v != "" {
			return v
		}
	}
	return ""
}

// 用户是照着屏幕手抄给我们的, 少一个歧义字符就少一次对不上号的客服, 所以去掉
// I 和 O (认成 1 和 0)。数字只用 2-9, 没有 0 和 1。
// 必须正好 32 个字符 —— 少一个 base32.NewEncoding 会 panic, 见 TestCodeAlphabet。
var codeAlphabet = base32.NewEncoding("ABCDEFGHJKLMNPQRSTUVWXYZ23456789").WithPadding(base32.NoPadding)

// DeviceCode 把机器标识压成一个 12 位短码, 形如 K7M2-9QX4-RT8N。
// 用哈希而不是直接截取机器标识: 机器标识是 SoC 序列号, 同一批次的设备前缀相同,
// 截出来会撞车; 过一遍 SHA-256 则均匀散开。
func DeviceCode(machineID string) string {
	if machineID == "" {
		return ""
	}
	sum := sha256.Sum256([]byte("rmkit-space-device-v1\x00" + machineID))
	s := codeAlphabet.EncodeToString(sum[:])[:12]
	return s[0:4] + "-" + s[4:8] + "-" + s[8:12]
}

// Identity 组装这台机器的身份。
func Identity() DeviceIdentity {
	id := MachineID()
	return DeviceIdentity{MachineID: id, Code: DeviceCode(id)}
}
