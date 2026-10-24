package space

import (
	"os"
	"path/filepath"
	"testing"
)

// 外壳与组件按版本放 rel/<版本>/ 并切链接; 第一次从助手装的真目录迁过去, 只留当前和上一版
func TestSwapVersioned(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "rmkit-cn", "space")
	h := &Handler{Reg: &Registry{BuiltinDir: filepath.Join(root, "apps")}}
	for _, p := range []string{"shell", "kit"} {
		_ = os.MkdirAll(filepath.Join(root, p), 0o755)
		_ = os.WriteFile(filepath.Join(root, p, "v"), []byte("old"), 0o644)
	}
	_ = os.WriteFile(h.versionFile(), []byte("v1.0.12\n"), 0o644)

	stage := func(v string) string {
		d := filepath.Join(tmp, "stage-"+v)
		for _, p := range []string{"shell", "kit"} {
			_ = os.MkdirAll(filepath.Join(d, p), 0o755)
			_ = os.WriteFile(filepath.Join(d, p, "v"), []byte(v), 0o644)
		}
		return d
	}
	read := func(p string) string { b, _ := os.ReadFile(filepath.Join(root, p, "v")); return string(b) }

	if err := h.swapVersioned(stage("1.1.0"), "v1.1.0"); err != nil {
		t.Fatal(err)
	}
	if read("shell") != "1.1.0" || read("kit") != "1.1.0" || read("shell.prev") != "old" {
		t.Fatalf("第一次切换不对: shell=%q kit=%q prev=%q", read("shell"), read("kit"), read("shell.prev"))
	}
	if tg, _ := os.Readlink(filepath.Join(root, "shell")); tg != filepath.Join(root, "rel", "1.1.0", "shell") {
		t.Errorf("shell 应链到 rel/1.1.0: %q", tg)
	}
	_ = os.WriteFile(h.versionFile(), []byte("v1.1.0\n"), 0o644)
	if err := h.swapVersioned(stage("1.2.0"), "v1.2.0"); err != nil {
		t.Fatal(err)
	}
	if read("shell") != "1.2.0" || read("shell.prev") != "1.1.0" {
		t.Fatalf("第二次切换不对: shell=%q prev=%q", read("shell"), read("shell.prev"))
	}
	if _, err := os.Stat(filepath.Join(root, "rel", "1.0.12")); err == nil {
		t.Error("更早的版本应清掉, 只留当前和上一版")
	}
}

func TestVersionCmp(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want int
	}{{"v1.1.0", "v1.0.12", 1}, {"v1.0.9", "v1.0.12", -1}, {"1.1.0", "v1.1.0", 0}, {"v1.1", "v1.1.0", 0}} {
		if got := VersionCmp(c.a, c.b); got != c.want {
			t.Errorf("VersionCmp(%s,%s)=%d 要 %d", c.a, c.b, got, c.want)
		}
	}
}
