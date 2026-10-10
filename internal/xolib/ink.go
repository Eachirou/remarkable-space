package xolib

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	rmscene "github.com/alexgorbatchev/go-rmscene"
)

// 手写画布: 借 xochitl 的原生算法把笔迹存成 .rm, 正本放在应用自己的数据目录里。
//
// xochitl 只会把笔画写进「一本打开着的笔记本的某一页」, 没有单独存一笔的接口 (3.28 实测,
// ShapesOverlay.saveStroke 只存形状手势)。所以用一本「载体」笔记本:
//   - 载体放在回收站 (parent = "trash"): 「我的文件」里看不见, 照样能在后台打开、写入 (实测)。
//   - 正本在 <应用数据>/ink/<key>.rm。要写哪块画布, 就把它的 .rm 拷进载体那一页 (Load),
//     外壳在后台打开载体、原生描线器写字、抬笔交给 sceneController.addDrawingLine;
//     写完外壳关掉载体, 再拷回正本 (Save)。关掉后换文件再开, xochitl 读的是盘上的新文件 (实测)。
//   - 回收站被清空、载体没了就重建 —— 正本不在里面, 不受影响。
//   - 写到一半空间崩了: 下次 Load 前先把载体里没拷回的那份存回原处 (session 记着是谁的)。
//
// 3.28 里文件库藏不住条目: deleted:true 照样显示, 父目录指向不存在的 ID 显示在根目录,
// 名字以 . 开头的文件夹也照样显示 —— 只有回收站可用。

const carrierName = "空间画板 (手写载体, 可清空)"

var keyRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// Ink 管载体与各应用的笔迹正本。
type Ink struct {
	Lib     *Library
	DataDir string // 空间应用数据根目录: <DataDir>/<app>/ink/<key>.rm
	State   string // 载体与当前会话的状态文件
}

type inkState struct {
	CarrierID string `json:"carrierId"`
	PageID    string `json:"pageId"`
	// 当前拷进载体、还没拷回的画布 (空 = 没有)
	App string `json:"app,omitempty"`
	Key string `json:"key,omitempty"`
	// 这次会话里画布在屏幕上的位置: 载体里存屏幕坐标 (= 原生笔输入层给的), 正本存画布坐标
	OX float32 `json:"ox,omitempty"`
	OY float32 `json:"oy,omitempty"`
}

// Session 是 Load 的结果: 外壳拿 CarrierID 去 entryForId / openDocument。
type Session struct {
	CarrierID string `json:"carrierId"`
	PageID    string `json:"pageId"`
	Created   bool   `json:"created"` // 载体是这次新建的 (外壳要等文件库加载完再打开)
}

var ErrBadKey = errors.New("app / key 只能是字母数字和 . _ -, 最长 64")

func (k *Ink) check(app, key string) error {
	if !keyRe.MatchString(app) || !keyRe.MatchString(key) {
		return ErrBadKey
	}
	return nil
}

func (k *Ink) inkFile(app, key string) string {
	return filepath.Join(k.DataDir, app, "ink", key+".rm")
}

func (k *Ink) pageFile(st inkState) string {
	return filepath.Join(k.Lib.Dir, st.CarrierID, st.PageID+".rm")
}

func (k *Ink) load() inkState {
	var st inkState
	if b, err := os.ReadFile(k.State); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

func (k *Ink) save(st inkState) error {
	b, _ := json.MarshalIndent(st, "", "  ")
	if err := os.MkdirAll(filepath.Dir(k.State), 0o755); err != nil {
		return err
	}
	return writeAtomic(k.State, b)
}

// carrier 拿到可用的载体, 没有 (从没建过 / 回收站被清空) 就建一本。
func (k *Ink) carrier(st *inkState) (created bool, err error) {
	if st.CarrierID != "" {
		if b, err := os.ReadFile(filepath.Join(k.Lib.Dir, st.CarrierID+".metadata")); err == nil {
			var m map[string]any
			if json.Unmarshal(b, &m) == nil && m["parent"] == "trash" {
				return false, nil
			}
			// 被人从回收站恢复到「我的文件」了: 不再当载体用 (那是用户的东西了), 另建一本。
			// 换之前把上次没拷回的那份存回原主
			if st.App != "" {
				if err := k.copyBack(*st, k.pageFile(*st)); err != nil {
					return false, err
				}
			}
		}
	}
	nb, err := k.Lib.CreateNotebook(carrierName, "trash", "Blank", false)
	if err != nil {
		return false, err
	}
	st.CarrierID, st.PageID = nb.ID, nb.PageID
	st.App, st.Key = "", ""
	return true, nil
}

// Load 把某块画布的正本拷进载体。调用前外壳必须已经关掉载体 (DocumentView.close)。
// (ox, oy) 是画布此刻在屏幕上的位置: 正本存的是画布坐标, 拷进载体时加上它, 变成屏幕坐标,
// 和原生笔输入层给的坐标一致 (橡皮、撤销都靠这个对上)。
func (k *Ink) Load(app, key string, ox, oy float32) (Session, error) {
	if err := k.check(app, key); err != nil {
		return Session{}, err
	}
	st := k.load()
	created, err := k.carrier(&st)
	if err != nil {
		return Session{}, err
	}
	page := k.pageFile(st)
	// 上次写到一半没拷回 (空间崩了 / 被关): 先存回原主, 不然下面一拷就覆盖没了
	if !created && st.App != "" && (st.App != app || st.Key != key) {
		if err := k.copyBack(st, page); err != nil {
			return Session{}, fmt.Errorf("存回上次没保存的 %s/%s 失败: %w", st.App, st.Key, err)
		}
	}
	// 同一块画布上次没存就被打断 (空间关了 / 崩了): 载体里的比正本新, 直接接着用, 不拿旧正本覆盖
	if !created && st.App == app && st.Key == key {
		if st.OX != ox || st.OY != oy {
			if b, err := os.ReadFile(page); err == nil {
				if b, err = Shift(b, ox-st.OX, oy-st.OY); err != nil {
					return Session{}, err
				}
				if err := writeAtomic(page, b); err != nil {
					return Session{}, err
				}
			}
			st.OX, st.OY = ox, oy
			if err := k.save(st); err != nil {
				return Session{}, err
			}
		}
		return Session{CarrierID: st.CarrierID, PageID: st.PageID}, nil
	}
	if err := os.MkdirAll(filepath.Dir(page), 0o755); err != nil {
		return Session{}, err
	}
	src := k.inkFile(app, key)
	if b, err := os.ReadFile(src); err == nil {
		if b, err = Shift(b, ox, oy); err != nil {
			return Session{}, fmt.Errorf("换成屏幕坐标失败: %w", err)
		}
		if err := writeAtomic(page, b); err != nil {
			return Session{}, err
		}
	} else if errors.Is(err, os.ErrNotExist) {
		// 新画布: 载体那一页清空, xochitl 打开时就是白纸
		if err := os.Remove(page); err != nil && !errors.Is(err, os.ErrNotExist) {
			return Session{}, err
		}
	} else {
		return Session{}, err
	}
	st.App, st.Key, st.OX, st.OY = app, key, ox, oy
	if err := k.save(st); err != nil {
		return Session{}, err
	}
	return Session{CarrierID: st.CarrierID, PageID: st.PageID, Created: created}, nil
}

// Save 把载体那一页拷回正本。调用前外壳必须已经关掉载体; 这里再等文件写稳 (xochitl 关文档后
// 后台还会收尾写一下)。
func (k *Ink) Save(app, key string) error {
	if err := k.check(app, key); err != nil {
		return err
	}
	st := k.load()
	if st.App != app || st.Key != key {
		return fmt.Errorf("载体里现在是 %s/%s, 不是 %s/%s", st.App, st.Key, app, key)
	}
	page := k.pageFile(st)
	waitStable(page, 400*time.Millisecond, 3*time.Second)
	if err := k.copyBack(st, page); err != nil {
		return err
	}
	st.App, st.Key = "", ""
	return k.save(st)
}

func (k *Ink) copyBack(st inkState, page string) error {
	b, err := os.ReadFile(page)
	if errors.Is(err, os.ErrNotExist) {
		return nil // 一笔没写: 正本保持原样 (新画布就还是没有)
	}
	if err != nil {
		return err
	}
	// 载体里是屏幕坐标, 正本存画布坐标
	if b, err = Shift(b, -st.OX, -st.OY); err != nil {
		return fmt.Errorf("换回画布坐标失败: %w", err)
	}
	dst := k.inkFile(st.App, st.Key)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return writeAtomic(dst, b)
}

// Delete 删掉某块画布的正本 (应用里"清空这一页")。
func (k *Ink) Delete(app, key string) error {
	if err := k.check(app, key); err != nil {
		return err
	}
	err := os.Remove(k.inkFile(app, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// ─── 给界面画的数据 ───────────────────────────────────────────────

// Run 是一段能直接画的折线: 同色、同宽、同透明度。坐标就是写字时画布上的坐标 (像素)。
type Run struct {
	Color   string    `json:"c"`
	Opacity float64   `json:"o"`
	Width   float64   `json:"w"`
	Cap     string    `json:"cap"` // round / square
	Points  []float64 `json:"p"`   // x0,y0,x1,y1,…
}

// Drawing 是一块画布的全部笔迹 (荧光笔在前, 画在墨迹下面)。
type Drawing struct {
	Strokes int        `json:"strokes"`
	Runs    []Run      `json:"runs"`
	Bounds  [4]float64 `json:"bounds"` // minX, minY, maxX, maxY (空画布全 0)
}

// Strokes 读一块画布: live=true 读载体里正在写的那一页 (写字时刷新干墨用), 否则读正本。
func (k *Ink) Strokes(app, key string, live bool) (Drawing, error) {
	if err := k.check(app, key); err != nil {
		return Drawing{}, err
	}
	path := k.inkFile(app, key)
	var dx, dy float32
	if live {
		st := k.load()
		if st.App == app && st.Key == key && st.CarrierID != "" {
			path = k.pageFile(st)
			dx, dy = -st.OX, -st.OY // 载体里是屏幕坐标
		}
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Drawing{Runs: []Run{}}, nil
	}
	if err != nil {
		return Drawing{}, err
	}
	if b, err = Shift(b, dx, dy); err != nil {
		return Drawing{}, err
	}
	return Render(b)
}

// 荧光笔/涂色笔在 go-rmscene 的 SVG 里用的是点 (pt), 这里换回屏幕像素
const ptToPx = 1 / rmscene.ScaleFactor

// Render 把 .rm 解析成能画的折线。宽度、颜色、透明度用 go-rmscene 的各笔模型
// (按 xochitl 存下的逐点宽度/压力/速度/方向算), 和它的 SVG 输出一致。
func Render(rm []byte) (Drawing, error) {
	sc, err := rmscene.Parse(rm)
	if err != nil {
		return Drawing{}, err
	}
	d := Drawing{Runs: []Run{}}
	first := true
	grow := func(x, y float64) {
		if first {
			d.Bounds = [4]float64{x, y, x, y}
			first = false
			return
		}
		d.Bounds[0], d.Bounds[1] = min(d.Bounds[0], x), min(d.Bounds[1], y)
		d.Bounds[2], d.Bounds[3] = max(d.Bounds[2], x), max(d.Bounds[3], y)
	}
	var ink []Run
	for _, l := range sc.Lines() {
		if len(l.Points) == 0 || l.Tool == rmscene.PenToolEraser || l.Tool == rmscene.PenToolEraserArea {
			continue
		}
		d.Strokes++
		if l.Tool == rmscene.PenToolHighlighter1 || l.Tool == rmscene.PenToolHighlighter2 || l.Tool == rmscene.PenToolShader {
			c := rmscene.GetColor(l.Color)
			w, o := rmscene.DefaultHighlighterWidth, rmscene.DefaultHighlighterOpacity
			if l.Tool == rmscene.PenToolShader {
				w = 16
			}
			r := Run{Color: c.Hex(), Opacity: o, Width: w * ptToPx, Cap: "square"}
			for _, p := range l.Points {
				r.Points = append(r.Points, float64(p.X), float64(p.Y))
				grow(float64(p.X), float64(p.Y))
			}
			d.Runs = append(d.Runs, r)
			continue
		}
		pen := rmscene.CreatePen(l.Tool, l.Color, l.ThicknessScale)
		seg := pen.SegmentLength()
		if seg <= 0 {
			seg = 1000
		}
		var cur *Run
		lastW := 0.0
		lastX, lastY := 0.0, 0.0
		for i, p := range l.Points {
			x, y := float64(p.X), float64(p.Y)
			grow(x, y)
			if i%seg == 0 {
				sp, dir, wd, pr := float64(p.Speed), float64(p.Direction), float64(p.Width), float64(p.Pressure)
				w := pen.SegmentWidth(sp, dir, wd, pr, lastW)
				r := Run{
					Color:   pen.SegmentColor(sp, dir, wd, pr, lastW),
					Opacity: pen.SegmentOpacity(sp, dir, wd, pr, lastW),
					Width:   w, Cap: pen.StrokeLinecap(),
				}
				if i > 0 {
					r.Points = append(r.Points, lastX, lastY) // 接上前一段, 不留缝
				}
				ink = append(ink, r)
				cur = &ink[len(ink)-1]
				lastW = w
			}
			cur.Points = append(cur.Points, x, y)
			lastX, lastY = x, y
		}
	}
	d.Runs = append(d.Runs, ink...)
	return d, nil
}

// ─── 小工具 ──────────────────────────────────────────────────────

func writeAtomic(path string, b []byte) error {
	tmp := path + ".xolib.tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// waitStable 等文件大小与修改时间在 quiet 时间内不再变 (最多等 limit)。文件不存在直接返回。
func waitStable(path string, quiet, limit time.Duration) {
	deadline := time.Now().Add(limit)
	var lastSize int64 = -1
	var lastMod time.Time
	stableSince := time.Now()
	for time.Now().Before(deadline) {
		fi, err := os.Stat(path)
		if err != nil {
			return
		}
		if fi.Size() != lastSize || !fi.ModTime().Equal(lastMod) {
			lastSize, lastMod, stableSince = fi.Size(), fi.ModTime(), time.Now()
		} else if time.Since(stableSince) >= quiet {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}
