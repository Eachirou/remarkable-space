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

// fbGeometry 按机型给帧缓冲几何: 可见宽高, 外加每行的真实跨距 (stride)。
// 两台 Paper Pro 的屏不一样大, 写死一个另一台就读出错位的图 —— 算法本身通用,
// 只有几何是机型相关的。3.28 起 /sys/class/graphics/fb0 不存在, 只能靠设备树型号认。
//
// Paper Pro 的 1620 在帧里补齐到 32 的倍数 = 1632, 按 1620 读会每行偏 12 像素,
// 整幅图斜着撕开。stride 是 2026-10-04 实测出来的: 把帧按若干候选宽度切行, 算相邻
// 行的平均差异, 1632 是 5.87 而 1620 是 20.00, 其余候选都在 14 以上。
func fbGeometry() (visW, h, stride int) {
	b, err := os.ReadFile("/proc/device-tree/model")
	if err == nil {
		switch m := string(bytes.TrimRight(b, "\x00")); {
		case strings.Contains(m, "Chiappa"): // Paper Pro Move
			return 960, 1696, 960
		case strings.Contains(m, "Ferrari"): // Paper Pro
			return 1620, 2160, 1632
		}
	}
	return 960, 1696, 960
}

func rmppScreenshot() (image.Image, error) {
	if b, err := os.ReadFile("/proc/device-tree/model"); err == nil && strings.Contains(string(b), "reMarkable 2") {
		return rm2Screenshot()
	}
	fbW, fbH, fbStride := fbGeometry()
	frameSize := uint64(fbStride * fbH * 4)

	pid, err := xochitlPID()
	if err != nil {
		return nil, err
	}

	// 找 card0 之后大匿名映射的起始地址（~6.5MB）
	maps, err := os.ReadFile(fmt.Sprintf("/proc/%d/maps", pid))
	if err != nil {
		return nil, fmt.Errorf("read maps: %w", err)
	}
	var anonStart uint64
	lines := strings.Split(string(maps), "\n")
	for i, line := range lines {
		if !strings.Contains(line, "/dev/dri/card0") {
			continue
		}
		if i+1 >= len(lines) {
			continue
		}
		next := lines[i+1]
		parts := strings.Fields(next)
		if len(parts) != 5 { // 匿名映射没有 pathname 字段
			continue
		}
		rng := strings.Split(parts[0], "-")
		if len(rng) != 2 {
			continue
		}
		s, _ := strconv.ParseUint(rng[0], 16, 64)
		e, _ := strconv.ParseUint(rng[1], 16, 64)
		// 尺寸窗口原本写死 6MB~8MB, 实测某次固件/旋转后这块映射变成 8,146,944
		// 字节, 刚好卡在窗口外 → 找不到帧缓冲 → 截图降级成纯文字 prompt, 于是
		// AI 一本正经地解释起帧缓冲报错。定位真正靠的是"紧邻 card0 的匿名映射"
		// (实测全局唯一), 尺寸只需保证放得下一帧, 上界放宽即可。
		if sz := e - s; sz >= frameSize && sz < 64<<20 {
			anonStart = s
		}
	}
	if anonStart == 0 {
		return nil, fmt.Errorf("no framebuffer anonymous mapping found after card0 (需 >= %d 字节)", frameSize)
	}

	// Ghostwriter 帧指针搜索算法
	f, err := os.Open(fmt.Sprintf("/proc/%d/mem", pid))
	if err != nil {
		return nil, fmt.Errorf("open mem: %w", err)
	}
	defer f.Close()

	var offset, length uint64 = 0, 2
	var frameOff int64
	for i := 0; i < 50; i++ {
		offset += length - 2
		if _, err := f.Seek(int64(anonStart+offset+8), io.SeekStart); err != nil {
			return nil, fmt.Errorf("seek at iter %d: %w", i, err)
		}
		var hdr [8]byte
		if _, err := io.ReadFull(f, hdr[:]); err != nil {
			return nil, fmt.Errorf("read hdr at iter %d offset %d: %w", i, offset, err)
		}
		length = uint64(binary.LittleEndian.Uint32(hdr[:4]))
		if length < 2 {
			return nil, fmt.Errorf("invalid length %d at offset %d", length, offset)
		}
		if length >= frameSize {
			frameOff = int64(anonStart + offset)
			break
		}
	}
	if frameOff == 0 {
		return nil, fmt.Errorf("frame pointer not found (anonStart=0x%x)", anonStart)
	}

	// 读帧缓冲
	if _, err := f.Seek(frameOff, io.SeekStart); err != nil {
		return nil, fmt.Errorf("seek frame: %w", err)
	}
	buf := make([]byte, fbStride*fbH*4)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, fmt.Errorf("read frame: %w", err)
	}

	// 按 stride 走行, 只取每行左边 fbW 列 —— 右边那几列是对齐填充, 不是画面
	img := image.NewNRGBA(image.Rect(0, 0, fbW, fbH))
	for y := 0; y < fbH; y++ {
		for x := 0; x < fbW; x++ {
			off := (y*fbStride + x) * 4
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
// 逐页扫所有放得下一帧的匿名映射, 找 size&^7 正好比一帧大一点的那一页 (2026-10-20 实测
// size=10514424, 帧 10513152, 多出的 1272 是 Qt 的对齐余量)。
func rm2Screenshot() (image.Image, error) {
	const w, h = 1404, 1872
	const frameSize = w * h * 4
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
	var hdr [8]byte
	for _, line := range strings.Split(string(maps), "\n") {
		parts := strings.Fields(line)
		if len(parts) != 5 || parts[1] != "rw-p" {
			continue
		}
		rng := strings.Split(parts[0], "-")
		if len(rng) != 2 {
			continue
		}
		start, _ := strconv.ParseUint(rng[0], 16, 64)
		end, _ := strconv.ParseUint(rng[1], 16, 64)
		if end-start < frameSize+8 {
			continue
		}
		for page := start; page+8+frameSize <= end; page += 4096 {
			if _, err := f.ReadAt(hdr[:], int64(page)); err != nil {
				break
			}
			sz := uint64(binary.LittleEndian.Uint32(hdr[4:8])) &^ 7
			if sz < frameSize || sz > frameSize+1<<20 {
				continue
			}
			buf := make([]byte, frameSize)
			if _, err := f.ReadAt(buf, int64(page+8)); err != nil {
				return nil, fmt.Errorf("read frame: %w", err)
			}
			img := image.NewNRGBA(image.Rect(0, 0, w, h))
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					off := (y*w + x) * 4
					img.SetNRGBA(x, y, color.NRGBA{R: buf[off+2], G: buf[off+1], B: buf[off], A: 255})
				}
			}
			return img, nil
		}
	}
	return nil, fmt.Errorf("rm2: 没找到 1404×1872 的帧 (xochitl pid %d)", pid)
}
