package server

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sync"

	"image"
	"image/color"
)

// 自绘水墨底图。
//
// 为什么不再用瓦片: 瓦片是别人渲染好的位图, 我们只能调明暗, 画什么完全管不着 ——
// 橙色高速、G15/S3213 的编号、密密麻麻的区县名, 一个都去不掉, 和这个应用通篇的
// 水墨风格格不入。另外高德那个瓦片接口不是公开 API, 发应用商店有授权隐患。
//
// 数据是 Natural Earth (公共领域), 预先简化好随应用打包, 不联网:
//   海岸线 ne_50m_coastline (裁到中国范围)   89 段 / 9755 点
//   省界   ne_10m_admin_1_states_provinces   99 段 / 11930 点
//   城市   ne_10m_populated_places 带 NAME_ZH 400 个, 按 SCALERANK 分级
// 简化用 Douglas-Peucker, 容差 0.012° (约 1.3km), 整包 407KB。
//
// 只画线不填色: 白纸 + 墨线, 降水的水洗压在上面, 正好是宣纸上作画的关系。

type geoData struct {
	Coast  [][][2]float64 `json:"coast"`
	Admin  [][][2]float64 `json:"admin"`
	River  [][][2]float64 `json:"river"` // 内陆没有海岸线, 全靠水系和省界撑起骨架
	Lake   [][][2]float64 `json:"lake"`
	Cities []geoCity      `json:"cities"`
}

type geoCity struct {
	N string  `json:"n"`
	X float64 `json:"x"`
	Y float64 `json:"y"`
	R int     `json:"r"` // SCALERANK, 越小越重要
}

var (
	geoOnce sync.Once
	geoVal  *geoData
)

func geo() *geoData {
	geoOnce.Do(func() {
		for _, p := range geoPaths() {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			var g geoData
			if json.Unmarshal(b, &g) == nil && len(g.Coast) > 0 {
				geoVal = &g
				return
			}
		}
	})
	return geoVal
}

func geoPaths() []string {
	var out []string
	if exe, err := os.Executable(); err == nil {
		out = append(out, filepath.Join(filepath.Dir(exe), "geo.json"))
	}
	return append(out, "/home/root/rmkit-cn/upload-server/geo.json", "geo.json")
}

// dab 在 (x,y) 点一笔: 半径 r 的软圆, 按覆盖率混色。
// 比直接画直线多花一点, 但边缘是柔的, 像笔尖压上去而不是像素阶梯。
func dab(dst *image.RGBA, x, y, r float64, col color.RGBA, alpha float64) {
	x0, x1 := int(math.Floor(x-r-1)), int(math.Ceil(x+r+1))
	y0, y1 := int(math.Floor(y-r-1)), int(math.Ceil(y+r+1))
	b := dst.Bounds()
	for py := y0; py <= y1; py++ {
		if py < b.Min.Y || py >= b.Max.Y {
			continue
		}
		for px := x0; px <= x1; px++ {
			if px < b.Min.X || px >= b.Max.X {
				continue
			}
			d := math.Hypot(float64(px)+0.5-x, float64(py)+0.5-y)
			cov := r + 0.5 - d
			if cov <= 0 {
				continue
			}
			if cov > 1 {
				cov = 1
			}
			a := cov * alpha
			old := dst.RGBAAt(px, py)
			dst.SetRGBA(px, py, color.RGBA{
				R: uint8(float64(old.R)*(1-a) + float64(col.R)*a),
				G: uint8(float64(old.G)*(1-a) + float64(col.G)*a),
				B: uint8(float64(old.B)*(1-a) + float64(col.B)*a),
				A: 255,
			})
		}
	}
}

func inkLine(dst *image.RGBA, x0, y0, x1, y1, w float64, col color.RGBA, alpha float64) {
	d := math.Hypot(x1-x0, y1-y0)
	n := int(d/0.5) + 1
	for i := 0; i <= n; i++ {
		t := float64(i) / float64(n)
		dab(dst, x0+(x1-x0)*t, y0+(y1-y0)*t, w/2, col, alpha)
	}
}

// drawInkBase 把底图画到画布上。left/top 是视口左上角在该缩放级下的全局像素坐标。
func drawInkBase(dst *image.RGBA, z int, left, top float64, vw, vh int) bool {
	g := geo()
	if g == nil {
		return false
	}
	toPx := func(lon, lat float64) (float64, float64) {
		x, y := lonLatToPixel(lon, lat, z)
		return x - left, y - top
	}
	// 画一组折线; 先按包围盒粗筛, 全国的数据不筛会白跑很多段
	draw := func(lines [][][2]float64, w float64, col color.RGBA, alpha float64) {
		for _, ln := range lines {
			minx, miny := math.Inf(1), math.Inf(1)
			maxx, maxy := math.Inf(-1), math.Inf(-1)
			pts := make([][2]float64, len(ln))
			for i, p := range ln {
				px, py := toPx(p[0], p[1])
				pts[i] = [2]float64{px, py}
				minx, maxx = math.Min(minx, px), math.Max(maxx, px)
				miny, maxy = math.Min(miny, py), math.Max(maxy, py)
			}
			if maxx < -8 || minx > float64(vw)+8 || maxy < -8 || miny > float64(vh)+8 {
				continue
			}
			for i := 1; i < len(pts); i++ {
				inkLine(dst, pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1], w, col, alpha)
			}
		}
	}
	// 由淡到浓: 河流最淡 (只是肌理) → 省界 → 湖岸 → 海岸线最浓
	draw(g.River, 1.0, color.RGBA{130, 150, 165, 255}, 0.55)
	draw(g.Lake, 1.2, color.RGBA{110, 135, 155, 255}, 0.7)
	draw(g.Admin, 1.1, color.RGBA{140, 136, 128, 255}, 0.8)
	draw(g.Coast, 1.8, color.RGBA{86, 82, 76, 255}, 0.9)
	return true
}

// visibleCities 返回落在视口内的城市及其像素位置, 由前端去画中文名
// (Go 这边渲染中日韩文字要带字体和 opentype 依赖, 不如交给 QML, 文字还更清楚)。
func visibleCities(z int, left, top float64, vw, vh int, limit int) []map[string]any {
	g := geo()
	if g == nil {
		return nil
	}
	// 缩放越大放越多: z8 只放省会级, z10 连地级市都放
	// Natural Earth 的中国城市只有 400 个 (约地级), 放得太严内陆会一片空白
	maxRank := 5
	if z >= 8 {
		maxRank = 8
	}
	if z >= 10 {
		maxRank = 10
	}
	out := []map[string]any{}
	for _, c := range g.Cities {
		if c.R > maxRank {
			continue
		}
		x, y := lonLatToPixel(c.X, c.Y, z)
		px, py := x-left, y-top
		if px < 10 || py < 10 || px > float64(vw)-10 || py > float64(vh)-10 {
			continue
		}
		out = append(out, map[string]any{"n": c.N, "x": math.Round(px), "y": math.Round(py), "r": c.R})
		if len(out) >= limit {
			break
		}
	}
	return out
}
