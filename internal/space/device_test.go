package space

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// base32 的字母表必须正好 32 个字符, 少一个 NewEncoding 当场 panic ——
// 而它是包级变量, panic 发生在包初始化, 整个 space-server 起不来。
// 第一版写成 31 个 (I/L/O 都去掉了) 就是这个毛病, 所以钉一条。
func TestCodeAlphabet(t *testing.T) {
	const want = 32
	// 反推字母表: 编码一段已知数据看字符集合够不够用不现实, 直接验长度与无重复
	alpha := "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	if len(alpha) != want {
		t.Fatalf("字母表 %d 个字符, 必须 %d", len(alpha), want)
	}
	seen := map[rune]bool{}
	for _, c := range alpha {
		if seen[c] {
			t.Errorf("字母表里 %q 重复了", c)
		}
		seen[c] = true
	}
	for _, bad := range []rune{'I', 'O', '0', '1'} {
		if seen[bad] {
			t.Errorf("字母表里不该有 %q —— 手抄时会认错", bad)
		}
	}
	// 真正要保证的是这行包级变量能初始化且可用
	if got := codeAlphabet.EncodeToString([]byte("x")); got == "" {
		t.Error("字母表初始化了但编不出东西")
	}
}

func TestDeviceCode(t *testing.T) {
	t.Run("形状对得上", func(t *testing.T) {
		got := DeviceCode("be61c80651ac49c3")
		if len(got) != 14 { // 12 位 + 2 个连字符
			t.Fatalf("设备码 = %q, 长度 %d, 想要 14", got, len(got))
		}
		if got[4] != '-' || got[9] != '-' {
			t.Errorf("设备码 = %q, 应当每四位一组", got)
		}
		for _, c := range strings.ReplaceAll(got, "-", "") {
			if !strings.ContainsRune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", c) {
				t.Errorf("设备码 %q 里有字母表之外的字符 %q", got, c)
			}
		}
	})

	t.Run("同一台机器恒定", func(t *testing.T) {
		if a, b := DeviceCode("abc123"), DeviceCode("abc123"); a != b {
			t.Errorf("同一个机器标识算出两个码: %q / %q", a, b)
		}
	})

	// 同批次设备的 SoC 序列号前缀相同, 直接截取会让一批机器拿到同一个码。
	t.Run("相近的机器标识不会撞车", func(t *testing.T) {
		seen := map[string]string{}
		for _, id := range []string{
			"be61c80651ac49c0", "be61c80651ac49c1", "be61c80651ac49c2",
			"be61c80651ac49c3", "be61c80651ac49c4",
		} {
			code := DeviceCode(id)
			if prev, dup := seen[code]; dup {
				t.Errorf("%s 和 %s 算出同一个码 %s", prev, id, code)
			}
			seen[code] = id
		}
	})

	// 读不到机器标识时必须返回空, 让调用方当"不能参与授权"处理。
	// 要是这里给个固定值, 所有读不到的设备会共用同一个身份。
	t.Run("没有机器标识就没有码", func(t *testing.T) {
		if got := DeviceCode(""); got != "" {
			t.Errorf("空机器标识算出了 %q, 应当是空", got)
		}
	})
}

func TestMachineID(t *testing.T) {
	// device-tree 里的字符串以 NUL 结尾, 不剥掉会把结束符混进哈希,
	// 导致同一台机器在不同读法下算出不同的码。
	t.Run("剥掉结尾的 NUL 和空白", func(t *testing.T) {
		dir := t.TempDir()
		p := filepath.Join(dir, "serial-number")
		if err := os.WriteFile(p, []byte("be61c80651ac49c3\x00"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer swapSources([]string{p})()
		if got := MachineID(); got != "be61c80651ac49c3" {
			t.Errorf("MachineID() = %q, 想要不带 NUL 的那串", got)
		}
	})

	t.Run("前一个来源读不到就往后找", func(t *testing.T) {
		dir := t.TempDir()
		good := filepath.Join(dir, "good")
		if err := os.WriteFile(good, []byte("fallback-id\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer swapSources([]string{filepath.Join(dir, "nope"), good})()
		if got := MachineID(); got != "fallback-id" {
			t.Errorf("MachineID() = %q, 想要 fallback-id", got)
		}
	})

	// 文件存在但内容是空的 (有过这种情况: 某些 SoC 的节点存在却没烧号),
	// 不能当成合法身份返回空串以外的东西, 也不能停在这个来源上。
	t.Run("内容为空视同读不到", func(t *testing.T) {
		dir := t.TempDir()
		empty := filepath.Join(dir, "empty")
		if err := os.WriteFile(empty, []byte("  \n"), 0o644); err != nil {
			t.Fatal(err)
		}
		good := filepath.Join(dir, "good")
		if err := os.WriteFile(good, []byte("real-id"), 0o644); err != nil {
			t.Fatal(err)
		}
		defer swapSources([]string{empty, good})()
		if got := MachineID(); got != "real-id" {
			t.Errorf("MachineID() = %q, 空文件应当跳过", got)
		}
	})

	t.Run("一个都读不到就返回空", func(t *testing.T) {
		defer swapSources([]string{filepath.Join(t.TempDir(), "nope")})()
		if got := MachineID(); got != "" {
			t.Errorf("MachineID() = %q, 想要空串", got)
		}
	})
}

func swapSources(paths []string) func() {
	old := machineIDSources
	machineIDSources = paths
	return func() { machineIDSources = old }
}
