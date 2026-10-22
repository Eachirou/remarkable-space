package server

// 笔记里的「贴纸」: 一个贴纸一个 .rm 笔迹文件 (stickers/<包>/<名>.rm, 坐标以中心为原点、边长 300)。
// 落到页面上的办法是**虚拟笔描** (2026-10-21 定): .rm 文件进不了 xochitl 的剪贴板/笔迹对象,
// 只有正在显示的页面才克隆得出笔迹 (后台打开的页面克隆为空, 社区 glossa / xovi-content-inserter
// 也都停在虚拟笔这一步)。面板 (笔记里的 QML, 注入库挂上) 的流程:
//   斜滑 (events 长轮询收到 gesture) → 选贴纸 → arm → 用户用笔点一下 (events 收到 tap) →
//   stroke begin/draw…/end (面板逐笔改笔的颜色粗细) → 切选择工具 → draw lasso 套索选中。

import (
	"encoding/json"
	"fmt"
	"image/color"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	rmscene "github.com/alexgorbatchev/go-rmscene"
	"github.com/boangs/remarkable-space/internal/handwriting"
	"github.com/boangs/remarkable-space/internal/sticker"
)

const stickerDir = "/home/root/.local/share/rmkit-cn/space/stickers"

var (
	stickerMu     sync.Mutex
	stickerSess   *handwriting.Session
	stickerGroups [][][][2]float32 // 按颜色/粗细分组的笔画 (屏幕坐标)
	stickerInput  *sticker.Input
	stickerOnce   sync.Once
)

func (s *Server) stickerIn() *sticker.Input {
	stickerOnce.Do(func() {
		stickerInput = sticker.NewInput(handwriting.DetectProfile())
	})
	return stickerInput
}

func cleanRel(rel string) (string, bool) {
	rel = filepath.Clean(rel)
	if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) || filepath.Ext(rel) != ".rm" {
		return "", false
	}
	return rel, true
}

// GET /space/sticker/list → {packs:[{id,name,name_en,stickers:[{pack,name,rel}]}], lang}
func (s *Server) stickerList(w http.ResponseWriter, r *http.Request) {
	packs, err := sticker.List(stickerDir)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if packs == nil {
		packs = []sticker.Pack{}
	}
	// 「常用」放最前, 始终显示: 最近贴过的在前, 不满一页用默认的补齐 (只列还存在的贴纸)
	have := map[string]sticker.Sticker{}
	for _, p := range packs {
		for _, st := range p.Stickers {
			have[st.Rel] = st
		}
	}
	var recent []sticker.Sticker
	seen := map[string]bool{}
	for _, rel := range append(loadRecent(), recentDefaults...) {
		if st, ok := have[rel]; ok && !seen[rel] && len(recent) < recentMax {
			recent = append(recent, st)
			seen[rel] = true
		}
	}
	if len(recent) > 0 {
		packs = append([]sticker.Pack{{ID: "@recent", Name: "常用", NameEn: "Recent", Icon: "@star", Stickers: recent}}, packs...)
	}
	lang := "zh"
	if s.space != nil {
		lang = s.space.CurrentLang()
	}
	writeJSON(w, http.StatusOK, map[string]any{"packs": packs, "lang": lang, "mode": loadPrefs().Mode})
}

// GET /space/sticker/thumb?rel=包/名.rm&size=160[&ink=ffffff] → PNG (rel=@star 为「常用」页签图标)
func (s *Server) stickerThumb(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	rel := q.Get("rel")
	if rel != "@star" {
		var ok bool
		if rel, ok = cleanRel(rel); !ok {
			httpError(w, http.StatusBadRequest, "贴纸路径不对")
			return
		}
	}
	size, _ := strconv.Atoi(q.Get("size"))
	if size < 32 || size > 512 {
		size = 160
	}
	var ink *color.RGBA
	if v, err := strconv.ParseUint(q.Get("ink"), 16, 32); err == nil && len(q.Get("ink")) == 6 {
		ink = &color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}
	}
	b, err := sticker.Thumb(stickerDir, rel, size, ink)
	if err != nil {
		httpError(w, http.StatusNotFound, "贴纸不存在")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(b)
}

// GET /space/sticker/image?rel=包/名.rm → {"path": 设备上的 PNG 绝对路径}
// 「图片」贴法用: 面板拿到路径后调 xochitl 的 SceneController.insertImageFileAsSceneItem(QUrl, QPointF) 插进当前页。
// 包里有同名 .png (原图) 就用原图缩小, 没有就用笔迹渲染; 缓存在 stickers/.images/。
func (s *Server) stickerImage(w http.ResponseWriter, r *http.Request) {
	rel, ok := cleanRel(r.URL.Query().Get("rel"))
	if !ok {
		httpError(w, http.StatusBadRequest, "贴纸路径不对")
		return
	}
	p, err := sticker.ImageFile(stickerDir, rel, stickerImagePx)
	if err != nil {
		httpError(w, http.StatusNotFound, "贴纸不存在")
		return
	}
	noteRecent(rel)
	writeJSON(w, http.StatusOK, map[string]any{"path": p})
}

// 图片贴纸的边长 (像素)。xochitl 插图按 1 像素 = 1 页面单位, 默认缩放下约 1.16 屏幕像素,
// 所以 176 像素落到屏幕上约 200 像素, 和「绘制」的贴纸一样大。
const stickerImagePx = 176

// 面板偏好 (贴法 image/draw), 存 stickers/.prefs.json
type stickerPrefs struct {
	Mode string `json:"mode"`
}

func loadPrefs() stickerPrefs {
	p := stickerPrefs{Mode: "draw"}
	if b, err := os.ReadFile(filepath.Join(stickerDir, ".prefs.json")); err == nil {
		_ = json.Unmarshal(b, &p)
	}
	if p.Mode != "image" {
		p.Mode = "draw"
	}
	return p
}

// POST /space/sticker/prefs {"mode":"image"|"draw"}
func (s *Server) stickerPrefsSet(w http.ResponseWriter, r *http.Request) {
	var in stickerPrefs
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || (in.Mode != "image" && in.Mode != "draw") {
		httpError(w, http.StatusBadRequest, "mode 只能是 image 或 draw")
		return
	}
	b, _ := json.Marshal(in)
	if err := os.WriteFile(filepath.Join(stickerDir, ".prefs.json"), b, 0o644); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// 「常用」: 最近贴过的贴纸 (最多一页 12 个), 存在 stickers/.recent.json
const recentMax = 12

// 还没贴过时「常用」先放这些 (各分类挑几个最常用的)
var recentDefaults = []string{
	"mood/happy.rm", "mood/sun.rm", "deco/star.rm", "deco/heart.rm",
	"nature/flower.rm", "nature/butterfly.rm", "plan/checklist.rm", "plan/calendar.rm",
	"label/washi-tape.rm", "label/banner.rm", "life/tea.rm", "date/week-1.rm",
}

func recentPath() string { return filepath.Join(stickerDir, ".recent.json") }

func loadRecent() []string {
	var out []string
	if b, err := os.ReadFile(recentPath()); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func noteRecent(rel string) {
	list := []string{rel}
	for _, r := range loadRecent() {
		if r != rel && len(list) < recentMax {
			list = append(list, r)
		}
	}
	if b, err := json.Marshal(list); err == nil {
		_ = os.WriteFile(recentPath(), b, 0o644)
	}
}

// GET /space/sticker/events?wait=25 长轮询: 斜滑 → {type:"gesture"}; 笔点 (arm 之后) → {type:"tap",x,y};
// 超时 → {type:"none"}。面板用 XHR 循环调用 (运行时注入的组件里 Timer 不触发, 只能靠它)。
func (s *Server) stickerEvents(w http.ResponseWriter, r *http.Request) {
	in := s.stickerIn()
	wait, _ := strconv.Atoi(r.URL.Query().Get("wait"))
	if wait <= 0 || wait > 60 {
		wait = 25
	}
	ch := in.Subscribe()
	defer in.Unsubscribe(ch)
	select {
	case ev := <-ch:
		writeJSON(w, http.StatusOK, ev)
	case <-time.After(time.Duration(wait) * time.Second):
		writeJSON(w, http.StatusOK, sticker.Event{Type: "none"})
	case <-r.Context().Done():
	}
}

// POST /space/sticker/arm {"on":true} 开始/取消等笔点
func (s *Server) stickerArm(w http.ResponseWriter, r *http.Request) {
	var in struct {
		On bool `json:"on"`
	}
	_ = json.NewDecoder(r.Body).Decode(&in)
	s.stickerIn().Arm(in.On)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// POST /space/sticker/draw {cx, cy, size, lasso:true}: 画一圈套索 (调用方先切到选择工具)
func (s *Server) drawSticker(w http.ResponseWriter, r *http.Request) {
	var in struct {
		CX    float32 `json:"cx"`
		CY    float32 `json:"cy"`
		Size  float32 `json:"size"`
		Lasso bool    `json:"lasso"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	if in.Size <= 0 {
		in.Size = 200
	}
	var ring [][2]float32
	rx := in.Size * 0.62
	n := 72
	for i := 0; i <= n; i++ {
		a := float64(i) / float64(n) * 2 * math.Pi
		ring = append(ring, [2]float32{in.CX + rx*float32(math.Cos(a)), in.CY + rx*float32(math.Sin(a))})
	}
	sess, err := s.newSession()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sess.Delay = time.Millisecond // 套索圈也用 1ms 一点
	defer sess.End()
	if err := sess.Stroke(ring); err != nil {
		httpError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) newSession() (*handwriting.Session, error) {
	in := s.stickerIn()
	sess, err := handwriting.Begin(handwriting.DetectProfile())
	if err != nil {
		return nil, err
	}
	sess.OnWrite = in.NoteWritten
	sess.WaitClear = func() bool {
		// 真笔在屏幕附近 (最近 300ms 有不是我们写的坐标) 就等, 最多等 8 秒
		deadline := time.Now().Add(8 * time.Second)
		for !in.ForeignQuiet(300 * time.Millisecond) {
			if time.Now().After(deadline) {
				return false
			}
			time.Sleep(50 * time.Millisecond)
		}
		return true
	}
	return sess, nil
}

// 逐笔描贴纸 (面板在笔与笔之间改笔的颜色/粗细):
//
//	POST /space/sticker/stroke {op:"begin", sticker, cx, cy, size} → {strokes:[{color, argb?, tool, width, count}]}
//	                                                                 (strokes 是按颜色/粗细分的组)
//	POST /space/sticker/stroke {op:"draw", index}                   → 描第 index 组 (count 笔)
//	POST /space/sticker/stroke {op:"wait"}                          → 等 300ms (给界面切工具)
//	POST /space/sticker/stroke {op:"end"}
func (s *Server) strokeSticker(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Op      string  `json:"op"`
		Sticker string  `json:"sticker"`
		CX      float32 `json:"cx"`
		CY      float32 `json:"cy"`
		Size    float32 `json:"size"`
		Index   int     `json:"index"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	stickerMu.Lock()
	defer stickerMu.Unlock()
	switch in.Op {
	case "begin":
		if stickerSess != nil {
			stickerSess.End()
			stickerSess = nil
		}
		rel, ok := cleanRel(in.Sticker)
		if !ok {
			httpError(w, http.StatusBadRequest, "贴纸路径不对")
			return
		}
		b, err := os.ReadFile(filepath.Join(stickerDir, rel))
		if err != nil {
			httpError(w, http.StatusNotFound, "贴纸不存在")
			return
		}
		noteRecent(rel)
		sc, err := rmscene.Parse(b)
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if in.Size <= 0 {
			in.Size = 200
		}
		k := in.Size / 300
		grayscale := handwriting.DetectProfile().ScreenW == handwriting.ProfileRM2.ScreenW // rm2 黑白屏
		// 按 (颜色, 粗细) 分组, 同组的笔一次描完: 面板每换一次笔只需一个来回 (原来逐笔来回, 杯子 8 笔要 5 秒)
		type group struct {
			key   string
			meta  map[string]any
			paths [][][2]float32
		}
		var groups []*group
		byKey := map[string]*group{}
		for _, ln := range sticker.Strokes(sc) {
			st := densify(ln.Points, in.CX, in.CY, k)
			wpx := 0.0
			if len(ln.Points) > 0 {
				wpx = float64(ln.Points[0].Width) / 4 * float64(k)
			}
			m := map[string]any{"color": int(ln.Color), "tool": int(ln.Tool), "width": wpx}
			// 整数档位 (1/2/3 = 针管笔细/中/粗) 原样交给笔, 不随贴纸大小缩放
			if ts := ln.ThicknessScale; ts == math.Trunc(ts) && ts >= 1 && ts <= 3 {
				m["thickness"] = ts
			}
			if ln.HasRGB {
				if grayscale {
					// rm2 不认自定义颜色 (color 9 + RGB 画成黑), 按亮度落到黑/灰
					r, g, b := float64(ln.ARGB>>16&0xff), float64(ln.ARGB>>8&0xff), float64(ln.ARGB&0xff)
					m["color"] = 0
					if 0.3*r+0.59*g+0.11*b >= 100 {
						m["color"] = 1
					}
				} else {
					m["argb"] = ln.ARGB
				}
			}
			key := fmt.Sprintf("%v/%v/%.2f/%v", m["color"], m["argb"], wpx, m["thickness"])
			g := byKey[key]
			if g == nil {
				g = &group{key: key, meta: m}
				byKey[key] = g
				groups = append(groups, g)
			}
			g.paths = append(g.paths, st)
		}
		stickerGroups = nil
		var meta []map[string]any
		for _, g := range groups {
			g.meta["count"] = len(g.paths)
			meta = append(meta, g.meta)
			stickerGroups = append(stickerGroups, g.paths)
		}
		sess, err := s.newSession()
		if err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
		sess.Delay = 2 * time.Millisecond // 1 像素一个事件, 2ms 一点 ≈ 500 像素/秒, 接近真人写字; 1ms 时急弯处会甩出去 (2026-10-22)
		stickerSess = sess
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "strokes": meta})
	case "draw":
		if stickerSess == nil || in.Index < 0 || in.Index >= len(stickerGroups) {
			httpError(w, http.StatusBadRequest, "没有进行中的贴纸或序号不对")
			return
		}
		t0 := time.Now()
		w0, d0, p0 := stickerSess.WaitTotal, stickerSess.DrawTotal, stickerSess.Points
		for _, st := range stickerGroups[in.Index] {
			if err := stickerSess.Stroke(st); err != nil {
				httpError(w, http.StatusConflict, err.Error())
				return
			}
		}
		wt, st := stickerSess.PenStats()
		log.Printf("sticker: 第 %d 组 %d 笔 %d 点, 总 %s (等笔 %s, 写点 %s, %.2f ms/点; 累计 write %s sleep %s)", in.Index, len(stickerGroups[in.Index]),
			stickerSess.Points-p0, time.Since(t0).Round(time.Millisecond), (stickerSess.WaitTotal - w0).Round(time.Millisecond),
			(stickerSess.DrawTotal - d0).Round(time.Millisecond), float64((stickerSess.DrawTotal-d0).Microseconds())/1000/float64(max(1, stickerSess.Points-p0)),
			wt.Round(time.Millisecond), st.Round(time.Millisecond))
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "wait":
		time.Sleep(300 * time.Millisecond)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	case "end":
		if stickerSess != nil {
			stickerSess.End()
			stickerSess = nil
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		httpError(w, http.StatusBadRequest, "op 不对")
	}
}

// densify 贴纸笔画换成屏幕坐标, 相邻点间距控制在约 1 像素。2026-10-21 实测 (同一个圆四种发法):
// 系统存点与发点节奏/密度无关 (都约每 5 像素存一点), 但每个事件跳 3 像素时笔迹引擎按"快速移动"
// 预测, 圆被压扁、起笔处鼓包; 每事件 1 像素 (2ms 或 5ms 都行) 画出来才是正圆。
// 太疏 (闭合路径最后那一段直接连回起点) 系统还会把瞬移当噪声丢掉, 实测爱心缺一截。
func densify(pts []rmscene.Point, cx, cy, k float32) [][2]float32 {
	// 步距: 曲线 1 像素, 涂色的水平扫描行 2 像素 (再宽就太快, 行尾刹不住涂出线外, 2026-10-22 实测)
	const spacing, straightSpacing = 1.0, 2.0
	src := make([][2]float32, 0, len(pts))
	for _, p := range pts {
		src = append(src, [2]float32{cx + p.X*k, cy + p.Y*k})
	}
	straight := func(i int) bool {
		if i <= 0 || i >= len(src)-1 {
			return false
		}
		a, b, c := src[i-1], src[i], src[i+1]
		// 只认「纯水平」(涂色的扫描行): 轮廓线上近似直的段放宽步距照样会被笔迹预测拉歪
		const eps = 0.01
		return math.Abs(float64(b[1]-a[1])) < eps && math.Abs(float64(c[1]-b[1])) < eps && b[0] != a[0] && c[0] != b[0]
	}
	// 急转弯 (方向变化超过 60°, 比如扫描行掉头、薯条尖) 原地多停几拍, 让笔迹引擎先刹住再转
	sharp := func(i int) bool {
		if i <= 0 || i >= len(src)-1 {
			return false
		}
		a, b, c := src[i-1], src[i], src[i+1]
		x1, y1, x2, y2 := float64(b[0]-a[0]), float64(b[1]-a[1]), float64(c[0]-b[0]), float64(c[1]-b[1])
		n := math.Hypot(x1, y1) * math.Hypot(x2, y2)
		return n > 0 && (x1*x2+y1*y2)/n < 0.5
	}
	var out [][2]float32
	for i, q := range src {
		if n := len(out); n > 0 {
			a := out[n-1]
			dx, dy := q[0]-a[0], q[1]-a[1]
			d := float32(math.Hypot(float64(dx), float64(dy)))
			sp := float32(spacing)
			if straight(i) || straight(i-1) {
				sp = straightSpacing
			}
			if d < sp*0.7 && i < len(src)-1 && !sharp(i) {
				continue
			}
			for steps, j := int(d/sp+0.5), 1; j < steps; j++ {
				f := float32(j) / float32(steps)
				out = append(out, [2]float32{a[0] + dx*f, a[1] + dy*f})
			}
		}
		out = append(out, q)
		if sharp(i) {
			out = append(out, q, q, q)
		}
	}
	return out
}
