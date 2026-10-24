package space

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestThemes(t *testing.T) {
	tmp := t.TempDir()
	root := filepath.Join(tmp, "rmkit-cn", "space")
	r := &Registry{BuiltinDir: filepath.Join(root, "apps"), UserDir: filepath.Join(tmp, "share", "space", "apps"), DataDir: filepath.Join(tmp, "d"), Arch: "aarch64"}
	h := &Handler{Reg: r}
	_ = os.MkdirAll(filepath.Join(root, "themes", "default"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "themes", "default", "theme.json"), []byte(`{"id":"default","name":"默认","version":"1"}`), 0o644)

	// 商店主题包: 清单 theme 指向包里的目录, 装好链进用户主题目录
	zr := makeZip(t, map[string]string{
		"manifest.json":    `{"id":"theme-contrast","name":"高对比","version":"1.0","theme":"theme"}`,
		"theme/theme.json": `{"id":"theme-contrast","name":"高对比","version":"1","colors":{"line":"#000000"},"border":3,"slots":{"hero":"Hero.qml"}}`,
		"theme/Hero.qml":   "import QtQuick\nItem {}\n",
	})
	if _, err := r.InstallZip(zr, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := h.ActiveTheme(); got != "default" {
		t.Fatalf("没选过应是默认, 得到 %s", got)
	}
	put := httptest.NewRecorder()
	h.themeSet(put, httptest.NewRequest("PUT", "/space/theme", strings.NewReader(`{"id":"theme-contrast"}`)))
	if put.Code != http.StatusOK {
		t.Fatalf("切换失败: %d %s", put.Code, put.Body)
	}
	rec := httptest.NewRecorder()
	h.themeActive(rec, httptest.NewRequest("GET", "/space/theme", nil))
	var out struct {
		ID     string
		Border int
		Colors map[string]string
		Slots  map[string]string
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	if out.ID != "theme-contrast" || out.Border != 3 || out.Colors["line"] != "#000000" || !strings.HasPrefix(out.Slots["hero"], "file://") || !strings.Contains(out.Slots["hero"], "?v=") {
		t.Fatalf("当前主题不对: %s", rec.Body)
	}
	// 主题删了回默认
	if err := r.Uninstall("theme-contrast", nil); err != nil {
		t.Fatal(err)
	}
	if got := h.ActiveTheme(); got != "default" {
		t.Errorf("主题删了应回默认, 得到 %s", got)
	}
	// 坏的 slots 路径拒装
	bad := makeZip(t, map[string]string{
		"manifest.json":    `{"id":"theme-bad","name":"x","version":"1","theme":"theme"}`,
		"theme/theme.json": `{"id":"theme-bad","name":"x","version":"1","slots":{"hero":"../evil.qml"}}`,
	})
	if _, err := r.InstallZip(bad, nil, nil); err == nil {
		t.Error("slots 指出目录外应拒装")
	}
}
