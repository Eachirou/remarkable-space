package space

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// 语言解析。
//
// 外壳跑在 xochitl 进程里, 能直接读 languageSettings.languageCode —— 那是实时值,
// 最准。这里是它读不到时的后备, 以及给没有 QML 上下文的场合 (商店、后台服务) 用。
//
// 顺序: xochitl 正在用的翻译文件 → 空间自己的设置 (用户显式选过) → xochitl 的
// Language 键 → Keyboard 键 → 系统 LANG → en。
//
// 实测 languageSettings 在外壳的上下文里拿不到 (live 恒为空), xochitl.conf 也不记
// 界面语言。最可靠的是看 xochitl 进程映射了哪个 reMarkable_xx.qm: Qt 加载翻译时
// mmap 整个文件, 系统设置里一切语言, 旧的解除映射、新的映上, 不用重启 (2026-10-20
// rmppm 实测 en → zh_CN)。读得到它, 空间就跟着系统走, 自己的语言设置不再起作用。
//
// 注意 xochitl.conf 里的 Language 键只有用户在系统设置里切换过才会写入; 从没切过的
// 设备上它不存在, 所以不能把"读不到"当成"用户要英文"——还得往下猜。

// SupportedLangs 是空间自带词典的语言。应用可以带更多, 外壳按同样的规则回落。
var SupportedLangs = []string{"en", "zh-CN"}

const DefaultLang = "en"

// NormalizeLang 把各种写法归一到词典文件名: zh_CN / zh-Hans / zh → zh-CN。
// 认不出来的回落到 en, 不要原样返回 —— 那会让外壳去找一个不存在的词典。
func NormalizeLang(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return DefaultLang
	}
	// 砍掉 .UTF-8 / @euro 之类的修饰
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	s = strings.ReplaceAll(s, "_", "-")
	low := strings.ToLower(s)
	switch {
	case strings.HasPrefix(low, "zh-hant"), strings.HasPrefix(low, "zh-tw"),
		strings.HasPrefix(low, "zh-hk"), strings.HasPrefix(low, "zh-mo"):
		// 繁体暂时没有词典, 先给简体 —— 比掉到英文贴近
		return "zh-CN"
	case strings.HasPrefix(low, "zh"):
		return "zh-CN"
	}
	// 其余按 BCP 47 取主语言段, 有词典就用, 没有回落
	base := low
	if i := strings.Index(base, "-"); i > 0 {
		base = base[:i]
	}
	for _, sup := range SupportedLangs {
		if strings.ToLower(sup) == low || strings.ToLower(sup) == base {
			return sup
		}
	}
	return DefaultLang
}

var confKeyRe = regexp.MustCompile(`(?m)^([A-Za-z]+)=(.*)$`)

// readXochitlKey 从 xochitl.conf 的 [General] 段读一个键。读不到返回空串。
// 只读, 绝不写 —— 那是 reMarkable 自己的配置。
func readXochitlKey(path, key string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	// [General] 是文件头一段; 碰到下一个 [Section] 就停, 免得读到同名键
	text := string(b)
	if i := strings.Index(text, "\n["); i > 0 {
		text = text[:i]
	}
	for _, m := range confKeyRe.FindAllStringSubmatch(text, -1) {
		if strings.EqualFold(m[1], key) {
			return strings.TrimSpace(m[2])
		}
	}
	return ""
}

// qmRe 从 maps 的路径里抠语言码: .../translations/reMarkable_zh_CN.qm → zh_CN
var qmRe = regexp.MustCompile(`/translations/reMarkable_([A-Za-z_-]+)\.qm$`)

// SystemLang 返回 xochitl 当前加载的界面语言, 找不到 (没在跑、固件换了文件名) 返回空串。
func SystemLang() string {
	pid := xochitlPID()
	if pid == "" {
		return ""
	}
	return langFromMaps("/proc/" + pid + "/maps")
}

func langFromMaps(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		if m := qmRe.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
			return NormalizeLang(m[1])
		}
	}
	return ""
}

// xochitlPID 按 /proc/*/comm 找 xochitl (BusyBox pidof 不在 Go 里用)。
func xochitlPID() string {
	ents, err := os.ReadDir("/proc")
	if err != nil {
		return ""
	}
	for _, e := range ents {
		n := e.Name()
		if n[0] < '0' || n[0] > '9' {
			continue
		}
		if b, err := os.ReadFile("/proc/" + n + "/comm"); err == nil && strings.TrimSpace(string(b)) == "xochitl" {
			return n
		}
	}
	return ""
}

// ResolveLang 按优先级定当前语言。pref 是空间自己的设置 (用户在空间里显式选过),
// 为空表示"跟随系统"。
func ResolveLang(pref, xochitlConf string) string {
	if p := strings.TrimSpace(pref); p != "" && !strings.EqualFold(p, "auto") {
		return NormalizeLang(p)
	}
	// 用户在 reMarkable 系统设置里选过语言
	if v := readXochitlKey(xochitlConf, "Language"); v != "" {
		return NormalizeLang(v)
	}
	// 没选过。键盘语言是下一个最好的线索: 装了中文输入法的人多半要中文界面
	if v := readXochitlKey(xochitlConf, "Keyboard"); v != "" {
		return NormalizeLang(v)
	}
	for _, env := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(env); v != "" && !strings.EqualFold(v, "C") && !strings.EqualFold(v, "POSIX") {
			return NormalizeLang(v)
		}
	}
	return DefaultLang
}

// XochitlConf 是 reMarkable 自己的配置, 只读。
const XochitlConf = "/home/root/.config/remarkable/xochitl.conf"

// CurrentLang 读外壳配置里的语言偏好 (shell/config.json 的 lang), 再按优先级定稿。
// 外壳自己能读 languageSettings.languageCode, 比这里准; 这个值是给它兜底的,
// 也给商店和后台服务这类没有 QML 上下文的地方用。
// LangPref 只读用户在空间里显式选的那个, 没选过就是空串。
// 外壳要靠它分清"用户选的"和"后端猜的"——猜的不能压过 xochitl 的实时值。
func (h *Handler) LangPref() string {
	if b, err := os.ReadFile(filepath.Join(h.Reg.DataDir, "shell", "config.json")); err == nil {
		var cfg struct {
			Lang string `json:"lang"`
		}
		if json.Unmarshal(b, &cfg) == nil && !strings.EqualFold(strings.TrimSpace(cfg.Lang), "auto") {
			return strings.TrimSpace(cfg.Lang)
		}
	}
	return ""
}

func (h *Handler) CurrentLang() string {
	if l := SystemLang(); l != "" {
		return l
	}
	var pref string
	if b, err := os.ReadFile(filepath.Join(h.Reg.DataDir, "shell", "config.json")); err == nil {
		var cfg struct {
			Lang string `json:"lang"`
		}
		if json.Unmarshal(b, &cfg) == nil {
			pref = cfg.Lang
		}
	}
	return ResolveLang(pref, XochitlConf)
}
