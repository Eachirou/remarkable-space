package server

// RMPP/RMPPM 截图 via /proc/{pid}/mem
// 算法参考: /Users/xurx/ghostwriter/src/screenshot.rs
//
// 关键步骤:
//   1. 找 /proc/pid/maps 里 card0 之后跟着大匿名映射（6~8MB）的那行
//   2. 从该匿名映射起始地址用 ghostwriter 算法找帧指针
//   3. 读 960×1696×4 字节 RGBA 数据

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

func xochitlPID() (int, error) {
	out, err := exec.Command("pidof", "xochitl").Output()
	if err != nil {
		return 0, fmt.Errorf("pidof: %w", err)
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) == 0 {
		return 0, fmt.Errorf("xochitl not running")
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("parse pid: %w", err)
	}
	return pid, nil
}

// fbGeometry 按机型给帧缓冲的可见宽高。3.28 起 /sys/class/graphics/fb0 不存在, 只能靠设备树型号认。
// 行跨距不在这里定: 同一机型不同固件会变 (Ferrari 3.27 实测 1632, 3.29 实测 1620), 写死哪个都会在
// 另一个固件上整幅图斜着撕开或干脆找不到帧 —— 改为从 glibc 块大小反推 (见 frameStride)。
func fbGeometry() (visW, h int) {
	b, err := os.ReadFile("/proc/device-tree/model")
	if err == nil {
		switch m := string(bytes.TrimRight(b, "\x00")); {
		case strings.Contains(m, "Chiappa"): // Paper Pro Move
			return 960, 1696
		case strings.Contains(m, "Ferrari"): // Paper Pro
			return 1620, 2160
		}
	}
	return 960, 1696
}

// frameStride 由 malloc 块的大小反推行跨距: 帧是 stride×h×4 字节一次 malloc 出来的, 大块走 mmap,
// 块大小 = 向上取整到页的 (数据 + 16 字节块头), 除以 h×4 取整就是 stride (页的零头远小于一行)。
// 跨距只允许比可见宽多不到 64 像素 (对齐填充), 不在这个范围就不是帧。
func frameStride(chunkLen uint64, visW, h int) (int, bool) {
	st := int(chunkLen / uint64(h*4))
	if st < visW || st >= visW+64 {
		return 0, false
	}
	return st, true
}

// frameScore 验内容: 真正的屏幕相邻两行几乎一样 (大片空白、横向笔画), 纹理/字体图集/窗口表面
// 的相邻行差异大。取前 240 行抽样算相邻行平均字节差, 越小越像画面; 顺带给出白像素 (0xff) 占比,
// 几乎没有白的块 (<5%) 也不会是墨水屏画面。
func frameScore(f *os.File, off int64, stride, rows int) (score float64, white float64, err error) {
	buf := make([]byte, stride*4*(rows+1))
	if _, err = io.ReadFull(io.NewSectionReader(f, off, int64(len(buf))), buf); err != nil {
		return 0, 0, err
	}
	rowB := stride * 4
	var sum, n, ff float64
	for y := 0; y < rows; y++ {
		a, b := buf[y*rowB:(y+1)*rowB], buf[(y+1)*rowB:(y+2)*rowB]
		for i := 0; i < rowB; i += 7 {
			d := int(a[i]) - int(b[i])
			if d < 0 {
				d = -d
			}
			sum += float64(d)
			n++
			if a[i] == 0xff {
				ff++
			}
		}
	}
	return sum / n, ff / n, nil
}

func rmppScreenshot() (image.Image, error) {
	if b, err := os.ReadFile("/proc/device-tree/model"); err == nil && strings.Contains(string(b), "reMarkable 2") {
		return rm2Screenshot()
	}
	fbW, fbH := fbGeometry()
	frameMin := uint64(fbW * fbH * 4)

	pid, err := xochitlPID()
	if err != nil {
		return nil, err
	}

	maps, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, fmt.Errorf("read maps: %w", err)
	}
	// 帧缓冲是 glibc 大块 malloc 出来的 mmap, 在**某一条** card0 映射之后不远处 (card0 有几十条 rw-s 映射散在
	// maps 里; Chiappa 3.29 实测在下一行, Ferrari 3.29 实测在第 9 行, 中间夹着别的匿名映射)。每条 card0 向后
	// 看 12 行收候选, 大小够一帧、不超过 64 MB 的都算; 真假靠块大小反推跨距 + frameScore 验内容, 不靠位置猜。
	type cand struct {
		start, size uint64
	}
	var cands []cand
	seen := map[uint64]bool{}
	lines := strings.Split(string(maps), "\n")
	for i, line := range lines {
		if !strings.Contains(line, "/dev/dri/card0") {
			continue
		}
		for d := 1; d <= 12 && i+d < len(lines); d++ {
			parts := strings.Fields(lines[i+d])
			if len(parts) != 5 || !strings.HasPrefix(parts[1], "rw") { // 有文件名 / 保护页: 跳过, 继续往后看
				continue
			}
			rng := strings.Split(parts[0], "-")
			if len(rng) != 2 {
				continue
			}
			s, _ := strconv.ParseUint(rng[0], 16, 64)
			e, _ := strconv.ParseUint(rng[1], 16, 64)
			if sz := e - s; sz >= frameMin && sz < 64<<20 && !seen[s] {
				seen[s] = true
				cands = append(cands, cand{s, sz})
			}
		}
	}
	if len(cands) == 0 {
		return nil, fmt.Errorf("no framebuffer anonymous mapping found after card0 (需 >= %d 字节)", frameMin)
	}

	f, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return nil, fmt.Errorf("open mem: %w", err)
	}
	defer f.Close()

	// Ghostwriter 帧指针搜索: 沿 glibc 块头链 (+8 处的 length) 往后走, 找第一块 ≥ 一帧的, 连块大小一起给出
	findFrame := func(anonStart uint64) (int64, uint64, error) {
		var offset, length uint64 = 0, 2
		for i := 0; i < 50; i++ {
			offset += length - 2
			var hdr [8]byte
			if _, err := f.ReadAt(hdr[:], int64(anonStart+offset+8)); err != nil {
				return 0, 0, fmt.Errorf("read hdr at iter %d offset %d: %w", i, offset, err)
			}
			length = uint64(binary.LittleEndian.Uint32(hdr[:4]))
			if length < 2 {
				return 0, 0, fmt.Errorf("invalid length %d at offset %d", length, offset)
			}
			if length >= frameMin {
				return int64(anonStart + offset), length, nil
			}
		}
		return 0, 0, fmt.Errorf("frame pointer not found")
	}
	var bestOff int64
	bestStride := 0
	bestScore := 1e9
	var tried []string
	for _, c := range cands {
		off, length, err := findFrame(c.start)
		if err != nil {
			tried = append(tried, fmt.Sprintf("0x%x: %v", c.start, err))
			continue
		}
		st, ok := frameStride(length, fbW, fbH)
		if !ok {
			tried = append(tried, fmt.Sprintf("0x%x: 块 %d 字节不像一帧", c.start, length))
			continue
		}
		score, white, err := frameScore(f, off+16, st, 240)
		if err != nil {
			tried = append(tried, fmt.Sprintf("0x%x: %v", c.start, err))
			continue
		}
		tried = append(tried, fmt.Sprintf("0x%x: stride=%d score=%.2f white=%.0f%%", c.start, st, score, white*100))
		if white < 0.05 || score >= bestScore {
			continue
		}
		bestOff, bestStride, bestScore = off, st, score
	}
	if bestStride == 0 {
		return nil, fmt.Errorf("%d 个候选都不是帧缓冲: %s", len(cands), strings.Join(tried, "; "))
	}

	// 读帧缓冲 (跳过 16 字节块头)
	buf := make([]byte, bestStride*fbH*4)
	if _, err := f.ReadAt(buf, bestOff+16); err != nil && err != io.EOF {
		return nil, fmt.Errorf("read frame: %w", err)
	}

	// 按 stride 走行, 只取每行左边 fbW 列 —— 右边那几列是对齐填充, 不是画面
	img := image.NewNRGBA(image.Rect(0, 0, fbW, fbH))
	for y := 0; y < fbH; y++ {
		for x := 0; x < fbW; x++ {
			off := (y*bestStride + x) * 4
			img.SetNRGBA(x, y, color.NRGBA{
				R: buf[off], G: buf[off+1], B: buf[off+2], A: buf[off+3],
			})
		}
	}
	return img, nil
}

// rm2Screenshot reMarkable 2 (3.29 实测): 没有 card0, /dev/fb0 是 SWTCON 的波形口 (260×23936) 不是画面。
// 画面是 xochitl 里一张 1404×1872 的 32 位 BGRA QImage, malloc 出来的大块, 放在某个 rw-p 匿名映射
// 里 (不一定在映射开头)。32 位 glibc 的块头: +4 是 size (低 3 位是标志), 数据从 +8 开始。
//
// 同尺寸的 QImage 不止一张 (要闻的版面图也是 1404×1872, 解码后一模一样大), 2026-10-20 实测
// 4 块候选里只有一块是屏幕: 它在 /dev/fb0 映射**之后**的第一块匿名映射里 (开屏时紧跟 fb0 分配的),
// 也是地址最高的那块 (mmap 从高往低长, 最早分配的最高); 其余是后来解码的图片, 地址更低。
// 所以只扫 fb0 之后的映射, 取第一块; 找不到 fb0 时退回地址最高的候选。
// 老固件 (rm2fb 时代) 是 16 位 RGB565, 一并认, 按块大小区分; 3.28 未实测。
func rm2Screenshot() (image.Image, error) {
	const w, h = 1404, 1872
	const frame32, frame16 = w * h * 4, w * h * 2
	pid, err := xochitlPID()
	if err != nil {
		return nil, err
	}
	maps, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, fmt.Errorf("read maps: %w", err)
	}
	f, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return nil, fmt.Errorf("open mem: %w", err)
	}
	defer f.Close()

	var fbEnd uint64
	type region struct{ start, end uint64 }
	var regions []region
	for _, line := range strings.Split(string(maps), "\n") {
		parts := strings.Fields(line)
		if len(parts) >= 6 && parts[5] == "/dev/fb0" {
			rng := strings.Split(parts[0], "-")
			fbEnd, _ = strconv.ParseUint(rng[1], 16, 64)
			continue
		}
		if len(parts) != 5 || parts[1] != "rw-p" {
			continue
		}
		rng := strings.Split(parts[0], "-")
		if len(rng) != 2 {
			continue
		}
		s, _ := strconv.ParseUint(rng[0], 16, 64)
		e, _ := strconv.ParseUint(rng[1], 16, 64)
		if e-s >= frame16+8 {
			regions = append(regions, region{s, e})
		}
	}
	var hdr [8]byte
	var found uint64
	bpp := 0
	for _, r := range regions {
		if fbEnd != 0 && r.start < fbEnd {
			continue // fb0 之前的都是后来的图片缓存
		}
		for page := r.start; page+8+frame16 <= r.end; page += 4096 {
			if _, err := f.ReadAt(hdr[:], int64(page)); err != nil {
				break
			}
			sz := uint64(binary.LittleEndian.Uint32(hdr[4:8])) &^ 7
			switch {
			case sz >= frame32 && sz <= frame32+1<<20 && page+8+frame32 <= r.end:
				found, bpp = page+8, 4
			case sz >= frame16 && sz <= frame16+1<<20:
				found, bpp = page+8, 2
			default:
				continue
			}
			if fbEnd != 0 {
				break // fb0 之后的第一块就是屏幕
			}
			// 没有 fb0 可参照: 继续扫, 留地址最高的
		}
		if found != 0 && fbEnd != 0 {
			break
		}
	}
	if found == 0 {
		return nil, fmt.Errorf("rm2: 没找到 1404×1872 的帧 (xochitl pid %d, fb0 end %x)", pid, fbEnd)
	}
	buf := make([]byte, w*h*bpp)
	if _, err := f.ReadAt(buf, int64(found)); err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if bpp == 4 {
				off := (y*w + x) * 4
				img.SetNRGBA(x, y, color.NRGBA{R: buf[off+2], G: buf[off+1], B: buf[off], A: 255})
			} else {
				v := binary.LittleEndian.Uint16(buf[(y*w+x)*2:])
				img.SetNRGBA(x, y, color.NRGBA{R: uint8(v>>11) << 3, G: uint8(v>>5&0x3f) << 2, B: uint8(v&0x1f) << 3, A: 255})
			}
		}
	}
	return img, nil
}
