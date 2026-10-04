package space

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

// 天气地图: 服务端把底图瓦片拼好、叠上雷达回波、调成墨水屏能看的色调, 只给前端一张图。
//
// 为什么在服务端合成, 而不是让 QML 自己拉十几块瓦片:
//   - 墨水屏每多一次局部刷新就多一次闪烁, 一张整图只刷一次
//   - 瓦片要缓存、要重试、要带 User-Agent, 这些在 QML 的 XHR 里都不好做
//     (商店索引当初也是为同样的理由改成后端代拉的)
//   - 调色必须在合成前做: 雷达的饱和彩虹色直接丢到 Gallery 3 上会糊成一团
//
// 数据源 (都在设备上实测过可达, 2026-10-03):
//
//	底图  高德 style=7  —— 常规底图, 国内直连快 (OSM 官方瓦片在国内超时)。
//	                     试过 style=8, 那是给卫星图打底的"影像路网"叠加层, 底色是深的,
//	                     单看一块瓦片看不出来, 合成整屏才发现是黑的。
//	雷达  RainViewer   —— 免费无 key; 中国有覆盖 (深圳雷雨时 z6 瓦片回波占 12.9%)
//	                     注意: 该区域 nowcast 帧数为 0, 只有过去 ~2 小时的 13 帧,
//	                     所以时间轴是"回放过去", 不是"预报未来"。
const (
	tileSize      = 256
	mapCacheTTL   = 20 * time.Minute // 瓦片在盘上留多久
	framesTTL     = 5 * time.Minute  // 雷达帧索引缓存
	fetchTimeout  = 15 * time.Second
	maxMapPixels  = 1600 * 1600 // 防止被参数撑爆内存
	radarAlphaMax = 0.78        // 回波最浓时的不透明度
	radarMaxZoom  = 7           // RainViewer 免费雷达的上限; 再高返回 "Zoom Level Not Supported" 占位图
)

var mapClient = &http.Client{Timeout: fetchTimeout}

// ─── 瓦片抓取与磁盘缓存 ────────────────────────────────────────────

func mapCacheDir() string {
	d := filepath.Join(os.TempDir(), "rmkit-weathermap")
	_ = os.MkdirAll(d, 0o755)
	return d
}

// fetchTile 取一块瓦片, 命中磁盘缓存就不走网络。
// key 用来做文件名, 调用方保证它只含安全字符。
func fetchTile(url, key string) (image.Image, error) {
	p := filepath.Join(mapCacheDir(), key)
	if st, err := os.Stat(p); err == nil && time.Since(st.ModTime()) < mapCacheTTL {
		if f, err := os.Open(p); err == nil {
			defer f.Close()
			if img, _, err := image.Decode(f); err == nil {
				return img, nil
			}
		}
	}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	// 高德和 RainViewer 都会按 UA 限流; 带个老实的标识, 别伪装成浏览器
	req.Header.Set("User-Agent", "rmkit-cn/1.0 (reMarkable personal device)")
	resp, err := mapClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("瓦片 %s 返回 %d", url, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	_ = os.WriteFile(p, b, 0o644)
	img, _, err := image.Decode(newByteReader(b))
	return img, err
}

type byteReader struct {
	b []byte
	i int
}

func newByteReader(b []byte) *byteReader { return &byteReader{b: b} }
func (r *byteReader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}

// ─── 墨卡托换算 ────────────────────────────────────────────────────

// lonLatToPixel 返回在该缩放级下的全局像素坐标 (左上角为原点)。
func lonLatToPixel(lon, lat float64, z int) (float64, float64) {
	n := math.Exp2(float64(z)) * tileSize
	x := (lon + 180) / 360 * n
	s := math.Sin(lat * math.Pi / 180)
	y := (0.5 - math.Log((1+s)/(1-s))/(4*math.Pi)) * n
	return x, y
}

// ─── 调色 ──────────────────────────────────────────────────────────

// washBase 调底图的明暗: 暗的压到更黑 (地名、路网、边界), 亮的往白里提 (大片底色)。
//
// 两版教训都写在这儿:
//  1. 第一版直接压成灰度 —— "墨点要留给回波"是过度修正, 整页成了黑白打印。
//     Gallery 3 显示中等彩度的实色没问题, 显不出来的是低彩度淡色。
//  2. 第二版改成 0.30 + 0.70*v 保留了颜色, 但那是无差别线性提亮:
//     纯黑的地名被一起提到 0.30 (#4D 中灰), 在反射屏上就看不清了。
//
// 现在按亮度走分段曲线, 并且按 目标亮度/原亮度 的比例去缩放 RGB 三个通道 ——
// 这样色相和彩度都保住, 只有明暗被重新分配。
func washBase(src image.Image) *image.RGBA {
	b := src.Bounds()
	out := image.NewRGBA(b)
	clamp := func(v float64) uint8 {
		if v <= 0 {
			return 0
		}
		if v >= 1 {
			return 255
		}
		return uint8(math.Round(v * 255))
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r0, g0, b0, _ := src.At(x, y).RGBA()
			r, g, bl := float64(r0)/65535, float64(g0)/65535, float64(b0)/65535
			lum := 0.299*r + 0.587*g + 0.114*bl
			var target float64
			if lum < 0.55 {
				target = lum * 0.42 // 地名/路网/边界: 压到接近纯黑
			} else {
				target = 0.62 + (lum-0.55)/0.45*0.38 // 底色: 往纸白提
			}
			if lum < 1e-4 {
				out.Set(x, y, color.RGBA{0, 0, 0, 255})
				continue
			}
			k := target / lum
			out.Set(x, y, color.RGBA{clamp(r * k), clamp(g * k), clamp(bl * k), 255})
		}
	}
	return out
}

// radarInk 把 RainViewer 的彩虹回波换成单一蓝灰的浓淡。
// 原始配色是为背光屏设计的 (绿→黄→红), 在 Gallery 3 上红绿都往黑里塌,
// 强弱反而分不出来。改成"越强越深的蓝灰", 和原型里那张图一致。
// scale: 一个源像素画成 scale×scale 的块。雷达最高只到 z=7 (再高返回
// "Zoom Level Not Supported" 占位图), 底图却要用 z=8 的省级范围, 所以雷达
// 在 z=7 取、按倍数放大叠上去。回波本来就是团状的, 放大 2 倍看不出马赛克。
func radarInk(src image.Image, dst *image.RGBA, offX, offY, scale int) {
	b := src.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			// 用原色的亮度反推强度: RainViewer 的色阶从浅蓝到深红, 越强越暗越饱和
			mx := math.Max(float64(r), math.Max(float64(g), float64(bl))) / 65535
			mn := math.Min(float64(r), math.Min(float64(g), float64(bl))) / 65535
			sat := 0.0
			if mx > 0 {
				sat = (mx - mn) / mx
			}
			// 强度 = 饱和度为主, 暗度为辅; 0~1
			inten := math.Min(1, sat*0.75+(1-mx)*0.5)
			if inten < 0.06 {
				continue
			}
			px, py := offX+(x-b.Min.X)*scale, offY+(y-b.Min.Y)*scale
			// 蓝色阶: 弱 #A9C6F0 → 强 #1F5FC0。
			// 这两个端点是在"计划"应用里实机验证过能显色的那支蓝, 不是拍脑袋调的;
			// 低彩度的蓝灰在这块屏上会被抖成灰, 强弱也就分不出来了。
			cr := uint8(169 - 138*inten)
			cg := uint8(198 - 103*inten)
			cb := uint8(240 - 48*inten)
			al := (float64(a) / 65535) * radarAlphaMax * (0.45 + 0.55*inten)
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					qx, qy := px+dx, py+dy
					if !(image.Point{qx, qy}.In(dst.Bounds())) {
						continue
					}
					old := dst.RGBAAt(qx, qy)
					dst.SetRGBA(qx, qy, color.RGBA{
						R: uint8(float64(old.R)*(1-al) + float64(cr)*al),
						G: uint8(float64(old.G)*(1-al) + float64(cg)*al),
						B: uint8(float64(old.B)*(1-al) + float64(cb)*al),
						A: 255,
					})
				}
			}
		}
	}
}

// ─── 雷达帧索引 ────────────────────────────────────────────────────

type radarFrame struct {
	Time int64  `json:"time"`
	Path string `json:"path"`
}

type rvIndex struct {
	Host  string `json:"host"`
	Radar struct {
		Past    []radarFrame `json:"past"`
		Nowcast []radarFrame `json:"nowcast"`
	} `json:"radar"`
}

var (
	framesMu   sync.Mutex
	framesAt   time.Time
	framesBody *rvIndex
)

func radarIndex() (*rvIndex, error) {
	framesMu.Lock()
	defer framesMu.Unlock()
	if framesBody != nil && time.Since(framesAt) < framesTTL {
		return framesBody, nil
	}
	req, _ := http.NewRequest("GET", "https://api.rainviewer.com/public/weather-maps.json", nil)
	req.Header.Set("User-Agent", "rmkit-cn/1.0 (reMarkable personal device)")
	resp, err := mapClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var idx rvIndex
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&idx); err != nil {
		return nil, err
	}
	framesBody, framesAt = &idx, time.Now()
	return framesBody, nil
}

// weatherMapFrames 返回可用的雷达帧 (过去 + 预报, 预报在中国通常为空)。
func (h *Handler) weatherMapFrames(w http.ResponseWriter, r *http.Request) {
	idx, err := radarIndex()
	if err != nil {
		fail(w, http.StatusBadGateway, "取雷达帧索引失败: "+err.Error())
		return
	}
	all := append(append([]radarFrame{}, idx.Radar.Past...), idx.Radar.Nowcast...)
	writeJSON(w, http.StatusOK, map[string]any{
		"frames":  all,
		"past":    len(idx.Radar.Past),
		"nowcast": len(idx.Radar.Nowcast),
	})
}

// ─── 合成 ──────────────────────────────────────────────────────────

// weatherMap 按中心点与视口尺寸合成一张地图。
//
//	GET /space/weather/map?lat=&lon=&z=&w=&h=&frame=/v2/radar/xxxx
//
// frame 省略时只画底图。
func (h *Handler) weatherMap(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat := atof(q.Get("lat"), 39.9)
	lon := atof(q.Get("lon"), 116.4)
	z := atoiDef(q.Get("z"), 7)
	vw := atoiDef(q.Get("w"), 900)
	vh := atoiDef(q.Get("h"), 700)
	if z < 3 {
		z = 3
	}
	if z > 10 {
		z = 10 // 再放大瓦片数量翻倍, 设备上不划算
	}
	if vw*vh > maxMapPixels {
		fail(w, http.StatusBadRequest, "视口太大")
		return
	}

	cx, cy := lonLatToPixel(lon, lat, z)
	left, top := cx-float64(vw)/2, cy-float64(vh)/2
	tx0, ty0 := int(math.Floor(left/tileSize)), int(math.Floor(top/tileSize))
	tx1, ty1 := int(math.Floor((left+float64(vw))/tileSize)), int(math.Floor((top+float64(vh))/tileSize))
	nTiles := int(math.Exp2(float64(z)))

	canvas := image.NewRGBA(image.Rect(0, 0, vw, vh))
	draw.Draw(canvas, canvas.Bounds(), image.NewUniform(color.RGBA{252, 251, 247, 255}), image.Point{}, draw.Src)

	// 底图: 优先自绘墨线 (见 inkmap.go)。矢量数据缺失时才退回高德瓦片 ——
	// 瓦片能跑但风格不对, 只当兜底。
	if !drawInkBase(canvas, z, left, top, vw, vh) {
		for ty := ty0; ty <= ty1; ty++ {
			for tx := tx0; tx <= tx1; tx++ {
				if ty < 0 || ty >= nTiles {
					continue
				}
				wx := ((tx % nTiles) + nTiles) % nTiles
				url := fmt.Sprintf("https://wprd01.is.autonavi.com/appmaptile?lang=zh_cn&size=1&style=7&x=%d&y=%d&z=%d", wx, ty, z)
				img, err := fetchTile(url, fmt.Sprintf("base-%d-%d-%d.png", z, wx, ty))
				if err != nil {
					continue
				}
				ox := int(math.Round(float64(tx)*tileSize - left))
				oy := int(math.Round(float64(ty)*tileSize - top))
				draw.Draw(canvas, image.Rect(ox, oy, ox+tileSize, oy+tileSize),
					washBase(img), image.Point{}, draw.Over)
			}
		}
	}

	// 数据层 (自绘水墨): layer=rain|aqi, hour = 从当前整点起的第几小时
	if ly := q.Get("layer"); ly == "rain" || ly == "aqi" {
		if g, err := fetchRainGrid(lat, lon, z, ly); err == nil && len(g.vals) > 0 {
			hr := g.startIndex() + atoiDef(q.Get("hour"), 0)
			if hr >= 0 && hr < len(g.vals[0]) {
				renderField(canvas, g, hr, z, left, top, vw, vh, ly)
			}
		}
	}

	// 雷达: 最高只到 radarMaxZoom, 更高的级别在 radarMaxZoom 上取再放大
	if frame := q.Get("frame"); frame != "" && isSafeFramePath(frame) {
		rz := z
		if rz > radarMaxZoom {
			rz = radarMaxZoom
		}
		scale := int(math.Exp2(float64(z - rz))) // 一个雷达像素画成 scale×scale
		rn := int(math.Exp2(float64(rz)))
		// 视口换算到 rz 的像素坐标系
		rLeft, rTop := left/float64(scale), top/float64(scale)
		rRight, rBottom := rLeft+float64(vw)/float64(scale), rTop+float64(vh)/float64(scale)
		rx0, ry0 := int(math.Floor(rLeft/tileSize)), int(math.Floor(rTop/tileSize))
		rx1, ry1 := int(math.Floor(rRight/tileSize)), int(math.Floor(rBottom/tileSize))
		for ty := ry0; ty <= ry1; ty++ {
			for tx := rx0; tx <= rx1; tx++ {
				if ty < 0 || ty >= rn {
					continue
				}
				wx := ((tx % rn) + rn) % rn
				url := fmt.Sprintf("https://tilecache.rainviewer.com%s/256/%d/%d/%d/4/1_1.png", frame, rz, wx, ty)
				img, err := fetchTile(url, fmt.Sprintf("radar-%s-%d-%d-%d.png", sanitizeKey(frame), rz, wx, ty))
				if err != nil {
					continue
				}
				ox := int(math.Round((float64(tx)*tileSize - rLeft) * float64(scale)))
				oy := int(math.Round((float64(ty)*tileSize - rTop) * float64(scale)))
				radarInk(img, canvas, ox, oy, scale)
			}
		}
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache")
	_ = jpeg.Encode(w, canvas, &jpeg.Options{Quality: 88})
}

// isSafeFramePath 只接受 RainViewer 索引里那种路径, 防止被当成任意 URL 拼接。
func isSafeFramePath(p string) bool {
	if len(p) < 2 || len(p) > 64 || p[0] != '/' {
		return false
	}
	for _, c := range p {
		if !(c == '/' || c == '-' || c == '_' ||
			(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return true
}

func sanitizeKey(s string) string {
	out := make([]rune, 0, len(s))
	for _, c := range s {
		if c == '/' {
			continue
		}
		out = append(out, c)
	}
	return string(out)
}

func atof(s string, def float64) float64 {
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return def
	}
	return v
}

func atoiDef(s string, def int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}

// ═══ 降水预报层 (自绘水墨) ══════════════════════════════════════════
//
// 为什么不用雷达瓦片:
//  1. RainViewer 免费接口在中国区没有 nowcast, 只有过去 ~2 小时的回放。
//     "雨带往哪走"要人自己判断, 远不如直接告诉你几点会下。
//  2. 雷达瓦片是别人渲染好的写实像素图, 我们改不了它的笔触。这个应用通篇是
//     水墨风, 一块照片质感的回波图嵌进来格格不入。
//
// 改成: 用本来就在用的 Open-Meteo 取一片网格点的逐小时降水, 自己插值、自己上墨。
// 这样时间轴是真的向前 (原型那条 现在→2h→4h→8h 才成立), 笔触也归我们管。
const (
	rainCols   = 13
	rainRows   = 9
	rainSpan   = 4.5 // 经度跨度 (度), 约等于一个省
	rainGridTT = 30 * time.Minute
	fieldDiv   = 4 // 场在 1/4 分辨率上算, 再双线性放大; 117 点 × 50 万像素直接算太慢
)

type rainGrid struct {
	lats, lons []float64
	times      []string
	vals       [][]float64 // [点][小时]
	at         time.Time
}

var (
	rainMu    sync.Mutex
	rainCache = map[string]*rainGrid{}
)

// z 决定网格跨度: 视野小了还用 4.5° 的网格, 落在画面里的点只有三四个,
// 插出来就是一团糊。跨度随缩放对半收, 点数不变, 于是分辨率跟着提高
// (z=8 约 37km 一个点, z=9 约 19km, 接近预报模式本身的格距)。
// layerSpec: 一个数据层用哪个接口、取哪个小时字段。
// 降水和空气质量的网格、插值、上墨完全同构, 只有数据源和色阶不同。
type layerSpec struct{ host, field string }

var layerSpecs = map[string]layerSpec{
	"rain": {"https://api.open-meteo.com/v1/forecast", "precipitation"},
	"aqi":  {"https://air-quality-api.open-meteo.com/v1/air-quality", "us_aqi"},
}

func fetchRainGrid(lat, lon float64, z int, layer string) (*rainGrid, error) {
	spec, ok := layerSpecs[layer]
	if !ok {
		spec, layer = layerSpecs["rain"], "rain"
	}
	span := rainSpan / math.Exp2(float64(z-8))
	key := fmt.Sprintf("%s,%.1f,%.1f,%d", layer, lat, lon, z)
	rainMu.Lock()
	if g, ok := rainCache[key]; ok && time.Since(g.at) < rainGridTT {
		rainMu.Unlock()
		return g, nil
	}
	rainMu.Unlock()

	spanLat := span * 2 / 3
	var lats, lons []string
	g := &rainGrid{}
	for r := 0; r < rainRows; r++ {
		for c := 0; c < rainCols; c++ {
			la := lat + spanLat/2 - spanLat*float64(r)/float64(rainRows-1)
			lo := lon - span/2 + span*float64(c)/float64(rainCols-1)
			lats = append(lats, strconv.FormatFloat(la, 'f', 3, 64))
			lons = append(lons, strconv.FormatFloat(lo, 'f', 3, 64))
			g.lats = append(g.lats, la)
			g.lons = append(g.lons, lo)
		}
	}
	url := spec.host + "?latitude=" + joinComma(lats) +
		"&longitude=" + joinComma(lons) +
		"&hourly=" + spec.field + "&forecast_days=2&timezone=Asia%2FShanghai"
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "rmkit-cn/1.0 (reMarkable personal device)")
	resp, err := mapClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var raw []struct {
		Hourly map[string]json.RawMessage `json:"hourly"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&raw); err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("预报网格为空")
	}
	if err := json.Unmarshal(raw[0].Hourly["time"], &g.times); err != nil {
		return nil, err
	}
	for i := range raw {
		var rawv []*float64 // 空气质量接口偶尔给 null
		if json.Unmarshal(raw[i].Hourly[spec.field], &rawv) != nil {
			return nil, fmt.Errorf("字段 %s 解析失败", spec.field)
		}
		v := make([]float64, 0, len(rawv))
		for _, q := range rawv {
			if q == nil {
				v = append(v, 0)
			} else {
				v = append(v, *q)
			}
		}
		g.vals = append(g.vals, v)
	}
	g.at = time.Now()
	rainMu.Lock()
	rainCache[key] = g
	rainMu.Unlock()
	return g, nil
}

func joinComma(a []string) string {
	out := ""
	for i, s := range a {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

// pixelToLonLat 是 lonLatToPixel 的逆运算。
func pixelToLonLat(px, py float64, z int) (float64, float64) {
	n := math.Exp2(float64(z)) * tileSize
	lon := px/n*360 - 180
	lat := math.Atan(math.Sinh(math.Pi*(1-2*py/n))) * 180 / math.Pi
	return lon, lat
}

// valueNoise 低频噪声: 让墨迹边缘不规则。纯数学插值出来的场边缘太圆太匀,
// 一眼就是计算机画的; 掺一点噪声才有洇开的样子。
func valueNoise(x, y, seed float64) float64 {
	s := math.Sin(x*12.9898+y*78.233+seed) * 43758.5453
	return s - math.Floor(s)
}

func smoothNoise(x, y float64) float64 {
	xi, yi := math.Floor(x), math.Floor(y)
	xf, yf := x-xi, y-yi
	u := xf * xf * (3 - 2*xf)
	v := yf * yf * (3 - 2*yf)
	a := valueNoise(xi, yi, 0)
	b := valueNoise(xi+1, yi, 0)
	c := valueNoise(xi, yi+1, 0)
	d := valueNoise(xi+1, yi+1, 0)
	return (a*(1-u)+b*u)*(1-v) + (c*(1-u)+d*u)*v
}

// renderRain 把某一小时的降水画成几层水墨。
//
// 水墨的两个特征, 都在这里做:
//   - 分层: 不是连续渐变, 而是几道浓淡分明的"水洗", 像一遍遍上墨
//   - 边缘积墨: 水彩/水墨干了之后边缘会比中间深一圈, 这是最像的一点
//
// startIndex 找"当前整点"在 times 里的下标。设备系统时区是 UTC, 而接口返回的是
// 北京时间的墙上钟点, 直接拿 time.Now() 去比会差 8 小时。
func (g *rainGrid) startIndex() int {
	_, off := localTimezone()
	loc := time.FixedZone("local", off*60)
	now := time.Now().In(loc)
	for i, t := range g.times {
		if ts, err := time.ParseInLocation("2006-01-02T15:04", t, loc); err == nil {
			if ts.Add(time.Hour).After(now) {
				return i
			}
		}
	}
	return 0
}

func renderField(canvas *image.RGBA, g *rainGrid, hour int, z int, left, top float64, vw, vh int, layer string) {
	fw, fh := vw/fieldDiv+2, vh/fieldDiv+2
	field := make([]float64, fw*fh)
	for j := 0; j < fh; j++ {
		for i := 0; i < fw; i++ {
			lon, lat := pixelToLonLat(left+float64(i*fieldDiv), top+float64(j*fieldDiv), z)
			// 反距离加权: 2 次幂, 够平滑又不会把远处的雨拖过来
			var num, den float64
			for p := range g.lats {
				dla, dlo := lat-g.lats[p], lon-g.lons[p]
				d2 := dla*dla + dlo*dlo
				if d2 < 1e-6 {
					num, den = g.vals[p][hour], 1
					break
				}
				w := 1 / (d2 * d2)
				num += w * g.vals[p][hour]
				den += w
			}
			v := 0.0
			if den > 0 {
				v = num / den
			}
			// 掺噪声: 幅度随强度走, 没雨的地方不要凭空长出墨点
			n := smoothNoise(float64(i)/7.0, float64(j)/7.0)*2 - 1
			v *= 1 + 0.35*n
			field[j*fw+i] = v
		}
	}

	// 分档与墨色按层走。
	// 降水: mm/h, 毛毛雨 → 暴雨, 蓝。
	// 空气质量: US AQI 国标分档 (50/100/150/200/300), 淡青 → 赭 ——
	//   用蓝会和降水撞, 而且"污染"配蓝不符合直觉。
	levels := []float64{0.1, 0.6, 2.0, 5.0, 12.0}
	inkStep, inkCap := 0.17, 0.82
	rampLo := [3]float64{169, 198, 240} // #A9C6F0
	rampHi := [3]float64{31, 95, 192}   // #1F5FC0
	if layer == "aqi" {
		levels = []float64{50, 100, 150, 200, 300}
		inkStep, inkCap = 0.15, 0.74
		rampLo = [3]float64{196, 214, 186} // #C4D6BA 优
		rampHi = [3]float64{150, 86, 72}   // #965648 重污染
	}
	at := func(fx, fy float64) float64 { // 双线性取样
		i, j := int(fx), int(fy)
		if i < 0 {
			i = 0
		}
		if j < 0 {
			j = 0
		}
		if i >= fw-1 {
			i = fw - 2
		}
		if j >= fh-1 {
			j = fh - 2
		}
		tx, ty := fx-float64(i), fy-float64(j)
		a := field[j*fw+i]
		b := field[j*fw+i+1]
		c := field[(j+1)*fw+i]
		d := field[(j+1)*fw+i+1]
		return (a*(1-tx)+b*tx)*(1-ty) + (c*(1-tx)+d*tx)*ty
	}

	for y := 0; y < vh; y++ {
		for x := 0; x < vw; x++ {
			v := at(float64(x)/fieldDiv, float64(y)/fieldDiv)
			if v < levels[0] {
				continue
			}
			ink := 0.0
			for li, lv := range levels {
				if v < lv {
					break
				}
				ink += inkStep // 每多压过一道, 墨就重一层
				// 边缘积墨: 刚好压在这道线上的那一圈, 再加一点
				band := lv * 0.18
				if math.Abs(v-lv) < band {
					ink += 0.14 * (1 - math.Abs(v-lv)/band)
				}
				_ = li
			}
			if ink > inkCap {
				ink = inkCap
			}
			// 墨色: 淡处偏灰蓝, 浓处是饱和的靛青 (两端都在设备上验过能显色)
			t := math.Min(1, ink/inkCap)
			cr := rampLo[0] + (rampHi[0]-rampLo[0])*t
			cg := rampLo[1] + (rampHi[1]-rampLo[1])*t
			cb := rampLo[2] + (rampHi[2]-rampLo[2])*t
			old := canvas.RGBAAt(x, y)
			canvas.SetRGBA(x, y, color.RGBA{
				R: uint8(float64(old.R)*(1-ink) + cr*ink),
				G: uint8(float64(old.G)*(1-ink) + cg*ink),
				B: uint8(float64(old.B)*(1-ink) + cb*ink),
				A: 255,
			})
		}
	}
}

// weatherRainHours 给前端时间轴用: 从当前整点起的逐小时刻度。
func (h *Handler) weatherRainHours(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	g, err := fetchRainGrid(atof(q.Get("lat"), 39.9), atof(q.Get("lon"), 116.4), atoiDef(q.Get("z"), 8), "rain")
	if err != nil {
		fail(w, http.StatusBadGateway, "取预报网格失败: "+err.Error())
		return
	}
	start := g.startIndex()
	type hourItem struct {
		Idx   int    `json:"idx"`
		Label string `json:"label"`
	}
	out := []hourItem{}
	for i := start; i < len(g.times) && len(out) < 13; i++ {
		lbl := g.times[i]
		if len(lbl) >= 16 {
			lbl = lbl[11:16]
		}
		out = append(out, hourItem{Idx: i - start, Label: lbl}) // 相对偏移, 降水与空气质量通用
	}
	writeJSON(w, http.StatusOK, map[string]any{"hours": out})
}

// weatherMapCities 返回视口内的城市及像素位置, 中文名交给前端画。
func (h *Handler) weatherMapCities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	lat, lon := atof(q.Get("lat"), 39.9), atof(q.Get("lon"), 116.4)
	z := atoiDef(q.Get("z"), 9)
	vw, vh := atoiDef(q.Get("w"), 900), atoiDef(q.Get("h"), 700)
	cx, cy := lonLatToPixel(lon, lat, z)
	writeJSON(w, http.StatusOK, map[string]any{
		"cities": visibleCities(z, cx-float64(vw)/2, cy-float64(vh)/2, vw, vh, 24),
	})
}
