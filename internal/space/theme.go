package space

// 主题 (v1.1.0 起): 只管外观 —— 颜色、圆角、描边、图标、默认封面, 以及首页三块 (顶部大卡 / 小组件区 /
// 应用格子) 的替换 QML。功能改动不走主题, 走自更新 (selfupdate.go)。
//
// 主题目录里一个 theme.json (格式见 themes/default/theme.json 的注释版 themes/README.md):
//   空间自带的在 <空间目录>/themes/<id>/;
//   商店装的主题是个应用, 清单 theme 字段指向包里的主题目录, 安装器链进 ~/.local/share/rmkit-cn/space/themes/<应用 id>。
// 当前主题记在 ~/.local/share/rmkit-cn/space/theme-active (一行 id); 主题被删了就回默认。
//
// 外壳拿 GET /space/theme 的结果直接用: 文件都换成 file:// 地址并带 ?v=<theme.json 的 mtime>
// (xochitl 按地址缓存图片和 QML, 主题更新了地址得变)。

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultTheme 空间自带、和 v1.0 外观一致的主题
const DefaultTheme = "default"

// ThemeSlots 主题能替换的首页区域
var ThemeSlots = map[string]bool{"hero": true, "widgets": true, "apps": true}

type themeFile struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	NameEn   string            `json:"name_en,omitempty"`
	Version  string            `json:"version"`
	MinShell int               `json:"min_shell,omitempty"`
	Colors   map[string]string `json:"colors,omitempty"`
	Radius   *int              `json:"radius,omitempty"`
	Border   *int              `json:"border,omitempty"`
	Hero     string            `json:"hero,omitempty"`    // 默认封面图 (相对主题目录)
	Preview  string            `json:"preview,omitempty"` // 个性化页里的预览图
	Icons    map[string]string `json:"icons,omitempty"`   // kit 图标名 → 主题里的 svg
	Slots    map[string]string `json:"slots,omitempty"`   // hero / widgets / apps → 主题里的 .qml
}

func (h *Handler) themeDirs() []string {
	return []string{filepath.Join(h.spaceRoot(), "themes"), filepath.Join(filepath.Dir(h.Reg.UserDir), "themes")}
}
func (h *Handler) themeActiveFile() string {
	return filepath.Join(filepath.Dir(h.Reg.UserDir), "theme-active")
}

// loadTheme 读一个主题目录; 校验字段, 文件路径都得在目录里
func loadTheme(dir string) (*themeFile, error) {
	b, err := os.ReadFile(filepath.Join(dir, "theme.json"))
	if err != nil {
		return nil, err
	}
	var t themeFile
	if err := json.Unmarshal(b, &t); err != nil {
		return nil, fmt.Errorf("theme.json 解析不了: %w", err)
	}
	if t.MinShell > ShellVersion {
		return nil, fmt.Errorf("需要外壳契约版本 %d", t.MinShell)
	}
	for k, f := range t.Slots {
		if !ThemeSlots[k] || !isRelativeInside(f) || filepath.Ext(f) != ".qml" {
			return nil, fmt.Errorf("slots.%s 不对: %q", k, f)
		}
	}
	for k, f := range t.Icons {
		if !isRelativeInside(f) {
			return nil, fmt.Errorf("icons.%s 不对: %q", k, f)
		}
	}
	for _, f := range []string{t.Hero, t.Preview} {
		if f != "" && !isRelativeInside(f) {
			return nil, fmt.Errorf("文件路径不对: %q", f)
		}
	}
	return &t, nil
}

type themeEntry struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	NameEn  string `json:"name_en,omitempty"`
	Version string `json:"version"`
	Preview string `json:"preview,omitempty"`
	Builtin bool   `json:"builtin"`
	dir     string
	t       *themeFile
}

// themes 列出能用的主题 (同 id 时商店装的覆盖自带的); 坏了的跳过
func (h *Handler) themes() map[string]*themeEntry {
	out := map[string]*themeEntry{}
	for i, root := range h.themeDirs() {
		ents, _ := os.ReadDir(root)
		for _, e := range ents {
			dir := filepath.Join(root, e.Name())
			if st, err := os.Stat(dir); err != nil || !st.IsDir() { // 商店主题是链接, 跟过去看
				continue
			}
			t, err := loadTheme(dir)
			if err != nil {
				continue
			}
			v := themeVer(dir)
			en := &themeEntry{ID: e.Name(), Name: t.Name, NameEn: t.NameEn, Version: t.Version, Builtin: i == 0, dir: dir, t: t}
			if t.Preview != "" {
				en.Preview = fileURL(filepath.Join(dir, t.Preview)) + "?v=" + v
			}
			out[en.ID] = en
		}
	}
	return out
}

func themeVer(dir string) string {
	st, err := os.Stat(filepath.Join(dir, "theme.json"))
	if err != nil {
		return "0"
	}
	return fmt.Sprint(st.ModTime().Unix())
}

// ActiveTheme 当前主题 id (记录的主题不在了就回默认)
func (h *Handler) ActiveTheme() string {
	b, _ := os.ReadFile(h.themeActiveFile())
	id := strings.TrimSpace(string(b))
	if id == "" {
		return DefaultTheme
	}
	if _, ok := h.themes()[id]; !ok {
		return DefaultTheme
	}
	return id
}

// GET /space/themes
func (h *Handler) themeList(w http.ResponseWriter, r *http.Request) {
	all := h.themes()
	list := make([]*themeEntry, 0, len(all))
	for _, t := range all {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Builtin != list[j].Builtin {
			return list[i].Builtin
		}
		return list[i].ID < list[j].ID
	})
	writeJSON(w, http.StatusOK, map[string]any{"active": h.ActiveTheme(), "themes": list})
}

// GET /space/theme → 当前主题, 文件都换成地址
func (h *Handler) themeActive(w http.ResponseWriter, r *http.Request) {
	id := h.ActiveTheme()
	t, ok := h.themes()[id]
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"id": DefaultTheme}) // 连默认主题都没有: 外壳用自己的内置值
		return
	}
	v := "?v=" + themeVer(t.dir)
	url := func(f string) string { return fileURL(filepath.Join(t.dir, f)) + v }
	out := map[string]any{"id": t.ID, "name": t.Name, "version": t.t.Version, "colors": t.t.Colors}
	if t.t.Radius != nil {
		out["radius"] = *t.t.Radius
	}
	if t.t.Border != nil {
		out["border"] = *t.t.Border
	}
	if t.t.Hero != "" {
		out["hero"] = url(t.t.Hero)
	}
	icons := map[string]string{}
	for k, f := range t.t.Icons {
		icons[k] = url(f)
	}
	slots := map[string]string{}
	for k, f := range t.t.Slots {
		slots[k] = url(f)
	}
	out["icons"], out["slots"] = icons, slots
	writeJSON(w, http.StatusOK, out)
}

// PUT /space/theme {"id": "..."}
func (h *Handler) themeSet(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.ID == "" {
		fail(w, http.StatusBadRequest, "需要 {\"id\": ...}")
		return
	}
	if _, ok := h.themes()[in.ID]; !ok {
		fail(w, http.StatusNotFound, "没有这个主题")
		return
	}
	if err := os.WriteFile(h.themeActiveFile(), []byte(in.ID+"\n"), 0o644); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"active": in.ID})
}

// linkTheme 商店主题应用装好后, 把包里的主题目录链进用户主题目录 (名字 = 应用 id)
func (r *Registry) linkTheme(appDir string, m *Manifest) error {
	src := filepath.Join(appDir, m.Theme)
	if _, err := loadTheme(src); err != nil {
		return fmt.Errorf("主题包不对: %w", err)
	}
	dir := filepath.Join(filepath.Dir(r.UserDir), "themes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, m.ID)
	if st, err := os.Lstat(dst); err == nil && st.Mode()&os.ModeSymlink == 0 {
		return errors.New("主题目录里已有同名的非链接目录")
	}
	_ = os.Remove(dst)
	return os.Symlink(src, dst)
}

// unlinkTheme 卸载时摘掉指向 appDir 的主题链接
func (r *Registry) unlinkTheme(appDir string) {
	dir := filepath.Join(filepath.Dir(r.UserDir), "themes")
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		if t, err := os.Readlink(p); err == nil && strings.HasPrefix(t, appDir+string(filepath.Separator)) {
			_ = os.Remove(p)
		}
	}
}
