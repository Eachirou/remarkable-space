package space

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// 应用从编译发布 (module) 改回明文 (entry) 后, 新清单里没有 URI 了,
// 得靠这两条线索找到旧版留在模块目录里的那份, 否则 import 还会解析到旧模块。
func TestStaleModuleNames(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "qml", "Legacy"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := staleModuleNames(dir, "plan")
	for _, want := range []string{"Legacy", "Plan"} {
		if !slices.Contains(got, want) {
			t.Errorf("staleModuleNames = %v, 缺 %q", got, want)
		}
	}
	if got := staleModuleNames(t.TempDir(), ""); len(got) != 0 {
		t.Errorf("空 id 且无 qml/ 时应为空, 得到 %v", got)
	}
}
