package server

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// App 是注册表对外暴露的一条应用记录 (清单 + 解析后的绝对位置)。
// 外壳 (Space.qml) 只消费这个结构, 用其中的 file:// URL 直接加载界面和图标。
type App struct {
	Manifest
	Dir        string `json:"dir"`                  // 应用目录 (绝对路径)
	Builtin    bool   `json:"builtin"`              // true = 随 rmkit-cn 安装的内置应用 (不可卸载)
	Size       int64  `json:"size"`                 // 目录总字节数 (管理页显示用)
	Rev        int64  `json:"rev"`                  // 目录里最新的改动时间 (Unix 毫秒), 外壳拿它做免缓存令牌
	EntryURL   string `json:"entryUrl,omitempty"`   // file:// 入口 QML
	IconURL    string `json:"iconUrl,omitempty"`    // 图标 URL (file:// 或 qrc:/)
	WidgetURL  string `json:"widgetUrl,omitempty"`  // file:// 小组件 QML
	DataDir    string `json:"dataDir"`              // 应用私有数据目录 (卸载时不删)
	ServiceURL string `json:"serviceUrl,omitempty"` // 应用后台的 http://127.0.0.1:<port>
	Error      string `json:"error,omitempty"`      // 清单有问题时仍列出, 把原因给开发者看
}

// Registry 扫描两个应用目录: 内置 (安装器管理) 与用户 (绿色安装), 同 id 时用户目录优先。
type Registry struct {
	BuiltinDir string // /home/root/rmkit-cn/space/apps
	UserDir    string // /home/root/.local/share/rmkit-cn/space/apps
	DataDir    string // /home/root/.local/share/rmkit-cn/space/data
	Arch       string // aarch64 / armv7
}

// List 返回全部应用, 按 类别 → order → 名字 排序。
func (r *Registry) List() []App {
	byID := map[string]App{}
	for _, a := range r.scanDir(r.BuiltinDir, true) {
		byID[a.ID] = a
	}
	for _, a := range r.scanDir(r.UserDir, false) {
		byID[a.ID] = a // 用户目录覆盖同名内置应用 (升级内置应用的绿色通道)
	}
	out := make([]App, 0, len(byID))
	for _, a := range byID {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Error == "") != (out[j].Error == "") {
			return out[i].Error == "" // 坏清单的排最后
		}
		ci, cj := categoryOrder[out[i].Category], categoryOrder[out[j].Category]
		if ci != cj {
			return ci < cj
		}
		if out[i].Order != out[j].Order {
			return out[i].Order < out[j].Order
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Get 按 id 查一条。
func (r *Registry) Get(id string) (App, bool) {
	for _, a := range r.List() {
		if a.ID == id {
			return a, true
		}
	}
	return App{}, false
}

// scanDir 把 dir 下每个含 manifest.json 的子目录变成 App。清单坏了也列出 (带 Error)。
func (r *Registry) scanDir(dir string, builtin bool) []App {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []App
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		appDir := filepath.Join(dir, e.Name())
		if _, err := os.Stat(filepath.Join(appDir, ManifestName)); err != nil {
			continue
		}
		out = append(out, r.build(appDir, e.Name(), builtin))
	}
	return out
}

func (r *Registry) build(appDir, dirName string, builtin bool) App {
	size, rev := dirStat(appDir)
	a := App{Dir: appDir, Builtin: builtin, Size: size, Rev: rev}
	m, err := LoadManifest(appDir)
	if err != nil {
		a.ID, a.Name = dirName, dirName
		a.Error = err.Error()
		return a
	}
	a.Manifest = *m
	if a.ID != dirName {
		a.Error = "目录名 " + dirName + " 与 manifest id " + a.ID + " 不一致"
		a.ID = dirName
	}
	if !m.SupportsArch(r.Arch) && a.Error == "" {
		a.Error = "不支持本机架构 " + r.Arch
	}
	a.DataDir = filepath.Join(r.DataDir, a.ID)
	if m.Entry != "" {
		a.EntryURL = fileURL(filepath.Join(appDir, m.Entry))
	}
	if m.Widget != "" {
		a.WidgetURL = fileURL(filepath.Join(appDir, m.Widget))
	}
	a.IconURL = resolveIcon(appDir, m.Icon)
	if m.Service != nil && m.Service.Port > 0 {
		a.ServiceURL = "http://127.0.0.1:" + itoa(m.Service.Port)
	}
	return a
}

func resolveIcon(appDir, icon string) string {
	switch {
	case icon == "":
		return ""
	case isURL(icon):
		return icon
	case filepath.IsAbs(icon):
		return fileURL(icon)
	default:
		return fileURL(filepath.Join(appDir, icon))
	}
}

func fileURL(p string) string { return "file://" + filepath.ToSlash(p) }

// dirStat 一次遍历同时给出目录总字节数与最新改动时间 (Unix 毫秒)。
//
// rev 是给外壳做免缓存令牌用的: 开发模式下外壳原来给 QML 的 URL 加 ?t=Date.now(),
// 每次打开都是新 URL, Qt 的类型缓存与磁盘缓存同时落空 —— 实测在 RMPPM 上重新
// 编译一个 135KB 的 main.qml 要 5.2 秒, 占了整个打开耗时的 94%。改成按 rev 之后,
// 文件没动就一直命中缓存, 一改文件 rev 变了自动重编一次。
func dirStat(dir string) (size int64, rev int64) {
	_ = filepath.WalkDir(dir, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, e := d.Info()
		if e != nil {
			return nil
		}
		if !d.IsDir() {
			size += info.Size()
		}
		if ms := info.ModTime().UnixMilli(); ms > rev {
			rev = ms // 目录自身的 mtime 也算: 增删文件时它会变
		}
		return nil
	})
	return size, rev
}

func itoa(n int) string { return strconv.Itoa(n) }
