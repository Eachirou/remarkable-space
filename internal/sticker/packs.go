package sticker

// 贴纸包: stickers/<包>/<贴纸>.rm, 可选 stickers/<包>/pack.json
// {"name": "装饰", "name_en": "Decor", "icon": "flower", "sort": 10, "order": ["贴纸名", ...]}。
// 缩略图由本机按笔迹画 (面板用), 缓存在 stickers/.thumbs/<包>/<贴纸>@<尺寸>.png, 源文件 mtime 变了重画。

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	rmscene "github.com/alexgorbatchev/go-rmscene"
)

type Sticker struct {
	Pack  string `json:"pack"`
	Name  string `json:"name"`
	Rel   string `json:"rel"` // 相对 stickers 目录: 包/名字.rm
	Lines int    `json:"lines"`
}

type Pack struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	NameEn   string    `json:"name_en"`
	Icon     string    `json:"icon"` // 页签图标: 包里某个贴纸的 rel (pack.json 的 icon 填贴纸名, 默认第一个)
	Stickers []Sticker `json:"stickers"`
}

// List 列出所有包
func List(dir string) ([]Pack, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var packs []Pack
	var sorts []int
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := Pack{ID: e.Name(), Name: e.Name()}
		var meta struct {
			Name   string   `json:"name"`
			NameEn string   `json:"name_en"`
			Icon   string   `json:"icon"`
			Sort   int      `json:"sort"`
			Order  []string `json:"order"`
		}
		if b, err := os.ReadFile(filepath.Join(dir, e.Name(), "pack.json")); err == nil && json.Unmarshal(b, &meta) == nil {
			if meta.Name != "" {
				p.Name = meta.Name
			}
			p.NameEn = meta.NameEn
		}
		files, _ := os.ReadDir(filepath.Join(dir, e.Name()))
		for _, f := range files {
			if f.IsDir() || filepath.Ext(f.Name()) != ".rm" {
				continue
			}
			name := strings.TrimSuffix(f.Name(), ".rm")
			p.Stickers = append(p.Stickers, Sticker{Pack: e.Name(), Name: name, Rel: e.Name() + "/" + f.Name()})
		}
		order := map[string]int{}
		for i, n := range meta.Order {
			order[n] = i
		}
		sort.SliceStable(p.Stickers, func(i, j int) bool {
			a, ia := order[p.Stickers[i].Name]
			b, ib := order[p.Stickers[j].Name]
			if ia != ib {
				return ia
			}
			if ia && ib && a != b {
				return a < b
			}
			return p.Stickers[i].Name < p.Stickers[j].Name
		})
		if len(p.Stickers) > 0 {
			p.Icon = p.Stickers[0].Rel
			for _, st := range p.Stickers {
				if meta.Icon != "" && st.Name == meta.Icon {
					p.Icon = st.Rel
				}
			}
			packs = append(packs, p)
			sorts = append(sorts, meta.Sort)
		}
	}
	// 页签顺序: pack.json 的 sort 小的在前, 一样就按目录名
	idx := make([]int, len(packs))
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool {
		if sorts[idx[a]] != sorts[idx[b]] {
			return sorts[idx[a]] < sorts[idx[b]]
		}
		return packs[idx[a]].ID < packs[idx[b]].ID
	})
	out := make([]Pack, len(packs))
	for i, j := range idx {
		out[i] = packs[j]
	}
	return out, nil
}

// Thumb 贴纸缩略图 (size×size 像素, 透明底), 带缓存。ink 非空时所有笔画改用这个颜色 (选中页签上的白色图标)。
// rel 为 "@star" 时画内置的五角星 (「常用」页签图标)。
func Thumb(dir, rel string, size int, ink *color.RGBA) ([]byte, error) {
	if rel == "@star" {
		return encodePNG(render(starStrokes(), size, ink))
	}
	src := filepath.Join(dir, rel)
	st, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	tag := ""
	if ink != nil {
		tag = fmt.Sprintf("-%02x%02x%02x", ink.R, ink.G, ink.B)
	}
	cache := filepath.Join(dir, ".thumbs", fmt.Sprintf("%s@%d%s.png", strings.TrimSuffix(rel, ".rm"), size, tag))
	if cs, err := os.Stat(cache); err == nil && !cs.ModTime().Before(st.ModTime()) {
		return os.ReadFile(cache)
	}
	b, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	sc, err := rmscene.Parse(b)
	if err != nil {
		return nil, err
	}
	b, err = encodePNG(render(Strokes(sc), size, ink))
	if err != nil {
		return nil, err
	}
	_ = os.MkdirAll(filepath.Dir(cache), 0o755)
	_ = os.WriteFile(cache, b, 0o644)
	return b, nil
}

func encodePNG(img image.Image) ([]byte, error) {
	w := &bytesWriter{}
	if err := png.Encode(w, img); err != nil {
		return nil, err
	}
	return w.b, nil
}

// starStrokes 内置五角星, 贴纸坐标 (中心原点, 边长 300), 线宽同手绘图标
func starStrokes() []Stroke {
	var pts []rmscene.Point
	for i := 0; i <= 10; i++ {
		r := 110.0
		if i%2 == 1 {
			r = 46
		}
		a := -math.Pi/2 + float64(i)*math.Pi/5
		pts = append(pts, rmscene.Point{X: float32(r * math.Cos(a)), Y: float32(r*math.Sin(a)) + 10, Width: 44})
	}
	return []Stroke{{Line: &rmscene.Line{Points: pts}}}
}

type bytesWriter struct{ b []byte }

func (w *bytesWriter) Write(p []byte) (int, error) { w.b = append(w.b, p...); return len(p), nil }

// 设备调色板 → 缩略图颜色 (与落到页面上的一致)
func penColor(c rmscene.PenColor) color.RGBA {
	switch int(c) {
	case 1, 8:
		return color.RGBA{128, 128, 128, 255}
	case 6, 11:
		return color.RGBA{0, 60, 200, 255}
	case 7, 12:
		return color.RGBA{190, 40, 40, 255}
	case 4, 10:
		return color.RGBA{30, 150, 60, 255}
	case 3, 13:
		return color.RGBA{200, 160, 0, 255}
	}
	return color.RGBA{0, 0, 0, 255}
}

// render 贴纸文件坐标以中心为原点、边长 300; 缩略图按 size 缩放, 用圆点沿折线盖出粗线
func render(lines []Stroke, size int, ink *color.RGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	k := float64(size) / 300
	for _, ln := range lines {
		if len(ln.Points) == 0 {
			continue
		}
		col := penColor(ln.Color)
		if ln.HasRGB {
			col = color.RGBA{uint8(ln.ARGB >> 16), uint8(ln.ARGB >> 8), uint8(ln.ARGB), 255}
		}
		if ink != nil {
			col = *ink
		}
		w := float64(ln.Points[0].Width) / 4 * k
		if w < 1.5 {
			w = 1.5
		}
		r := w / 2
		var px, py float64
		for i, p := range ln.Points {
			x, y := float64(p.X)*k+float64(size)/2, float64(p.Y)*k+float64(size)/2
			if i == 0 {
				dot(img, x, y, r, col)
			} else {
				d := math.Hypot(x-px, y-py)
				steps := int(d/(r*0.6)) + 1
				for s := 1; s <= steps; s++ {
					t := float64(s) / float64(steps)
					dot(img, px+(x-px)*t, py+(y-py)*t, r, col)
				}
			}
			px, py = x, y
		}
	}
	return img
}

func dot(img *image.NRGBA, cx, cy, r float64, col color.RGBA) {
	x0, x1 := int(math.Floor(cx-r-1)), int(math.Ceil(cx+r+1))
	y0, y1 := int(math.Floor(cy-r-1)), int(math.Ceil(cy+r+1))
	b := img.Bounds()
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if x < b.Min.X || y < b.Min.Y || x >= b.Max.X || y >= b.Max.Y {
				continue
			}
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			a := r + 0.5 - d // 边缘 1px 抗锯齿
			if a <= 0 {
				continue
			}
			if a > 1 {
				a = 1
			}
			o := img.PixOffset(x, y)
			old := float64(img.Pix[o+3]) / 255
			na := a + old*(1-a)
			img.Pix[o] = uint8((float64(col.R)*a + float64(img.Pix[o])*old*(1-a)) / math.Max(na, 1e-6))
			img.Pix[o+1] = uint8((float64(col.G)*a + float64(img.Pix[o+1])*old*(1-a)) / math.Max(na, 1e-6))
			img.Pix[o+2] = uint8((float64(col.B)*a + float64(img.Pix[o+2])*old*(1-a)) / math.Max(na, 1e-6))
			img.Pix[o+3] = uint8(na * 255)
		}
	}
}

// ImageFile 「图片」贴法用的 PNG (边长 px, 透明底), 返回设备上的绝对路径。
// 包里有同名 .png (原图) 就裁掉透明边后缩小; 没有就按笔迹渲染。缓存在 stickers/.images/<包>/<名>@<px>.png。
func ImageFile(dir, rel string, px int) (string, error) {
	src := filepath.Join(dir, rel)
	st, err := os.Stat(src)
	if err != nil {
		return "", err
	}
	out := filepath.Join(dir, ".images", fmt.Sprintf("%s@%dk.png", strings.TrimSuffix(rel, ".rm"), px))
	orig := strings.TrimSuffix(src, ".rm") + ".png"
	newest := st.ModTime()
	if os_, err := os.Stat(orig); err == nil && os_.ModTime().After(newest) {
		newest = os_.ModTime()
	}
	if cs, err := os.Stat(out); err == nil && !cs.ModTime().Before(newest) {
		return out, nil
	}
	var img image.Image
	if f, err := os.Open(orig); err == nil {
		im, _, derr := image.Decode(f)
		f.Close()
		if derr == nil {
			img = fitTransparent(im, px)
		}
	}
	if img == nil {
		b, err := os.ReadFile(src)
		if err != nil {
			return "", err
		}
		sc, err := rmscene.Parse(b)
		if err != nil {
			return "", err
		}
		img = render(Strokes(sc), px, nil)
	}
	b, err := encodePNG(inkToBlack(img))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(out, b, 0o644); err != nil {
		return "", err
	}
	return out, nil
}

// fitTransparent 裁掉四周透明边, 等比缩到 px×px 以内 (面积平均, 不发虚), 居中放进 px×px 透明画布
func fitTransparent(im image.Image, px int) *image.NRGBA {
	b := im.Bounds()
	x0, y0, x1, y1 := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := im.At(x, y).RGBA(); a > 0x0800 {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x+1), max(y1, y+1)
			}
		}
	}
	if x1 <= x0 || y1 <= y0 {
		x0, y0, x1, y1 = b.Min.X, b.Min.Y, b.Max.X, b.Max.Y
	}
	w, h := x1-x0, y1-y0
	scale := float64(px) / float64(max(w, h))
	tw, th := max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
	dst := image.NewNRGBA(image.Rect(0, 0, px, px))
	ox, oy := (px-tw)/2, (px-th)/2
	for ty := 0; ty < th; ty++ {
		sy0, sy1 := y0+int(float64(ty)/scale), y0+int(float64(ty+1)/scale)
		for tx := 0; tx < tw; tx++ {
			sx0, sx1 := x0+int(float64(tx)/scale), x0+int(float64(tx+1)/scale)
			var r, g, bb, a, n float64
			for sy := sy0; sy < max(sy1, sy0+1); sy++ {
				for sx := sx0; sx < max(sx1, sx0+1); sx++ {
					cr, cg, cb, ca := im.At(sx, sy).RGBA()
					r += float64(cr)
					g += float64(cg)
					bb += float64(cb)
					a += float64(ca)
					n++
				}
			}
			if a == 0 {
				continue
			}
			// 预乘 alpha 平均后还原
			o := dst.PixOffset(ox+tx, oy+ty)
			dst.Pix[o] = uint8(r / a * 255)
			dst.Pix[o+1] = uint8(g / a * 255)
			dst.Pix[o+2] = uint8(bb / a * 255)
			dst.Pix[o+3] = uint8(a / n / 257)
		}
	}
	return dst
}

// inkToBlack 把深灰描边压成纯黑、边缘抗锯齿加实: 原图描边是 #4B4640 一类的深灰, 缩到 176 像素后边缘又是半透明,
// 墨水屏上看就是灰线 (2026-10-22 用户反馈)。只动低饱和的深色像素, 彩色块不受影响。
func inkToBlack(im image.Image) *image.NRGBA {
	b := im.Bounds()
	dst := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			if c.A > 0 {
				lum := 0.299*float64(c.R) + 0.587*float64(c.G) + 0.114*float64(c.B)
				mx, mn := max(c.R, c.G, c.B), min(c.R, c.G, c.B)
				if lum < 150 && int(mx)-int(mn) < 70 {
					c.R, c.G, c.B = 0, 0, 0
					c.A = uint8(min(255, int(c.A)*3/2))
				}
			}
			dst.SetNRGBA(x, y, c)
		}
	}
	return dst
}
