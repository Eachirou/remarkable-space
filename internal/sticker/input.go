package sticker

// 贴纸的输入侧: 只读监听触屏与笔的 evdev (不拦截, 系统照常收到同一份事件)。
//   - 触屏: 认「右上角往左下斜滑」(2026-10-21 rmppm 实测参数: 起点右 22%/顶 20% 内, 25°–65°, 长 ≥0.3,
//     单指, 笔不在感应范围; 系统快捷面板只认竖直下滑, 翻页 8°/180°, 都不会误认)。
//   - 笔: 面板选好贴纸后等用户「点一下」定落点; 描贴纸期间监视真笔 —— 真笔和虚拟笔写的是同一个
//     设备, 真笔一靠近坐标就混在一起线条乱飞, 所以看到不是我们写的坐标就暂停, 等真笔离开再继续。

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// Event 发给面板的事件: gesture (斜滑) / tap (笔点, 屏幕像素坐标)
type Event struct {
	Type string  `json:"type"`
	X    float32 `json:"x,omitempty"`
	Y    float32 `json:"y,omitempty"`
	At   int64   `json:"at"`
}

// Input 一份常驻的监听
type Input struct {
	mu       sync.Mutex
	subs     map[chan Event]struct{}
	penNear  bool
	armed    bool // 等笔点落点
	foreign  time.Time
	penDev   string
	touchDev string
	screenW  float32
	screenH  float32
	inputW   float32
	inputH   float32
	written  [64][2]int32 // 虚拟笔最近写过的坐标环 (区分真笔)
	wpos     int
}

var absInfoReq = func(code uint) uintptr { return uintptr(0x80184540 + code) } // EVIOCGABS

type absInfo struct{ Value, Min, Max, Fuzz, Flat, Res int32 }

func absRange(f *os.File, code uint) absInfo {
	var a absInfo
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), absInfoReq(code), uintptr(unsafe.Pointer(&a)))
	return a
}

// findInput 按设备名找 /dev/input/eventN
func findInput(sub string) string {
	for i := 0; i < 12; i++ {
		b, err := os.ReadFile(fmt.Sprintf("/sys/class/input/event%d/device/name", i))
		if err == nil && strings.Contains(strings.ToLower(string(b)), sub) {
			return fmt.Sprintf("/dev/input/event%d", i)
		}
	}
	return ""
}

// NewInput 起监听; screenW/H 是屏幕像素, inputW/H 是笔设备坐标范围 (落点换算用)
func NewInput(screenW, screenH, inputW, inputH int) *Input {
	in := &Input{subs: map[chan Event]struct{}{}, screenW: float32(screenW), screenH: float32(screenH),
		inputW: float32(inputW), inputH: float32(inputH)}
	in.penDev = findInput("marker")
	if in.penDev == "" {
		in.penDev = findInput("pen")
	}
	in.touchDev = findInput("touch")
	if in.penDev != "" {
		go in.penLoop()
	}
	if in.touchDev != "" {
		go in.touchLoop()
	}
	return in
}

func (in *Input) Subscribe() chan Event {
	ch := make(chan Event, 4)
	in.mu.Lock()
	in.subs[ch] = struct{}{}
	in.mu.Unlock()
	return ch
}

func (in *Input) Unsubscribe(ch chan Event) {
	in.mu.Lock()
	delete(in.subs, ch)
	in.mu.Unlock()
}

func (in *Input) publish(ev Event) {
	ev.At = time.Now().UnixMilli()
	in.mu.Lock()
	for ch := range in.subs {
		select {
		case ch <- ev:
		default:
		}
	}
	in.mu.Unlock()
}

// Arm 开始等笔点 (面板选了贴纸之后); Disarm 取消
func (in *Input) Arm(on bool) {
	in.mu.Lock()
	in.armed = on
	in.mu.Unlock()
}

// NoteWritten 虚拟笔每写一个坐标就登记, 监听里据此分辨真笔
func (in *Input) NoteWritten(x, y int32) {
	in.mu.Lock()
	in.written[in.wpos%len(in.written)] = [2]int32{x, y}
	in.wpos++
	in.mu.Unlock()
}

func (in *Input) isOurs(x, y int32) bool {
	for _, w := range in.written {
		if w[0] == x && w[1] == y {
			return true
		}
	}
	return false
}

// ForeignQuiet 真笔是否已离开 (最近 quiet 内没有真笔坐标)
func (in *Input) ForeignQuiet(quiet time.Duration) bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return time.Since(in.foreign) > quiet
}

func (in *Input) PenNear() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	return in.penNear
}

const (
	evSyn, evKey, evAbs  = 0, 1, 3
	btnToolPen           = 0x140
	btnTouch             = 0x14a
	absX, absY           = 0x00, 0x01
	absMTSlot            = 0x2f
	absMTTrackingID      = 0x39
	absMTPosX, absMTPosY = 0x35, 0x36
)

func readEvent(f *os.File, buf []byte) (typ, code uint16, val int32, ok bool) {
	if _, err := f.Read(buf); err != nil {
		return 0, 0, 0, false
	}
	return binary.LittleEndian.Uint16(buf[16:]), binary.LittleEndian.Uint16(buf[18:]), int32(binary.LittleEndian.Uint32(buf[20:])), true
}

// penLoop: 真笔检测 + 点一下
func (in *Input) penLoop() {
	for {
		f, err := os.Open(in.penDev)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		xr, yr := absRange(f, absX), absRange(f, absY)
		if xr.Max > 0 {
			in.inputW, in.inputH = float32(xr.Max), float32(yr.Max)
		}
		buf := make([]byte, 24)
		var x, y int32 = -1, -1
		var sx, sy int32
		touching := false
		var downAt time.Time
		moved := float64(0)
		for {
			typ, code, val, ok := readEvent(f, buf)
			if !ok {
				break
			}
			switch {
			case typ == evKey && code == btnToolPen:
				in.mu.Lock()
				in.penNear = val != 0
				in.mu.Unlock()
			case typ == evAbs && code == absX:
				x = val
			case typ == evAbs && code == absY:
				y = val
			case typ == evKey && code == btnTouch:
				if val != 0 {
					touching, downAt, sx, sy, moved = true, time.Now(), x, y, 0
				} else if touching {
					touching = false
					in.mu.Lock()
					armed := in.armed
					in.mu.Unlock()
					if armed && time.Since(downAt) < 500*time.Millisecond && moved < 0.03*float64(in.inputW) && !in.isOurs(sx, sy) {
						in.Arm(false)
						in.publish(Event{Type: "tap", X: float32(sx) / in.inputW * in.screenW, Y: float32(sy) / in.inputH * in.screenH})
					}
				}
			case typ == evSyn && code == 0:
				if x >= 0 && y >= 0 && !in.isOurs(x, y) {
					in.mu.Lock()
					in.foreign = time.Now()
					in.mu.Unlock()
				}
				if touching {
					if d := math.Hypot(float64(x-sx), float64(y-sy)); d > moved {
						moved = d
					}
				}
			}
		}
		f.Close()
		time.Sleep(time.Second)
	}
}

// touchLoop: 右上角斜滑
func (in *Input) touchLoop() {
	for {
		f, err := os.Open(in.touchDev)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		xr, yr := absRange(f, absMTPosX), absRange(f, absMTPosY)
		if xr.Max <= 0 || yr.Max <= 0 {
			f.Close()
			return
		}
		buf := make([]byte, 24)
		slot := 0
		active := map[int]bool{}
		cur := map[int]*[2]float64{}
		var start, last [2]float64
		tracking, penDuring := false, false
		maxF := 0
		for {
			typ, code, val, ok := readEvent(f, buf)
			if !ok {
				break
			}
			switch {
			case typ == evAbs && code == absMTSlot:
				slot = int(val)
			case typ == evAbs && code == absMTTrackingID:
				if val >= 0 {
					active[slot] = true
					cur[slot] = &[2]float64{-1, -1}
				} else {
					delete(active, slot)
				}
			case typ == evAbs && code == absMTPosX:
				if p := cur[slot]; p != nil {
					p[0] = float64(val-xr.Min) / float64(xr.Max-xr.Min)
				}
			case typ == evAbs && code == absMTPosY:
				if p := cur[slot]; p != nil {
					p[1] = float64(val-yr.Min) / float64(yr.Max-yr.Min)
				}
			case typ == evSyn && code == 0:
				n := len(active)
				if n > 0 && !tracking {
					tracking, maxF, penDuring = true, 0, false
					for s := range active {
						if p := cur[s]; p != nil {
							start = *p
						}
					}
				}
				if n > maxF {
					maxF = n
				}
				if in.PenNear() {
					penDuring = true
				}
				for s := range active {
					if p := cur[s]; p != nil && p[0] >= 0 {
						last = *p
					}
				}
				if n == 0 && tracking {
					tracking = false
					dx, dy := last[0]-start[0], last[1]-start[1]
					l := math.Hypot(dx, dy)
					ang := math.Atan2(dy, -dx) * 180 / math.Pi
					if maxF == 1 && !penDuring && start[0] > 0.78 && start[1] < 0.20 && dx < 0 && dy > 0 && ang >= 25 && ang <= 65 && l >= 0.3 {
						in.publish(Event{Type: "gesture"})
					}
				}
			}
		}
		f.Close()
		time.Sleep(time.Second)
	}
}
