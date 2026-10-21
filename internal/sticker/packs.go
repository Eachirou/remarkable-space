package sticker

// 贴纸包: stickers/<包>/<贴纸>.rm, 可选 stickers/<包>/pack.json {"name": "...", "name_en": "...", "order": [...]}。
// 缩略图由本机按笔迹画 (面板用), 缓存在 stickers/.thumbs/<包>/<贴纸>@<尺寸>.png, 源文件 mtime 变了重画。

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
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
	for _, e := range ents {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		p := Pack{ID: e.Name(), Name: e.Name()}
		var meta struct {
			Name   string   `json:"name"`
			NameEn string   `json:"name_en"`
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
			packs = append(packs, p)
		}
	}
	return packs, nil
}

// Thumb 贴纸缩略图 (size×size 像素, 透明底), 带缓存
func Thumb(dir, rel string, size int) ([]byte, error) {
	src := filepath.Join(dir, rel)
	st, err := os.Stat(src)
	if err != nil {
		return nil, err
	}
	cache := filepath.Join(dir, ".thumbs", fmt.Sprintf("%s@%d.png", strings.TrimSuffix(rel, ".rm"), size))
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
	img := render(Strokes(sc), size)
	w := &bytesWriter{}
	if err := png.Encode(w, img); err != nil {
		return nil, err
	}
	_ = os.MkdirAll(filepath.Dir(cache), 0o755)
	_ = os.WriteFile(cache, w.b, 0o644)
	return w.b, nil
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
func render(lines []Stroke, size int) *image.NRGBA {
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
