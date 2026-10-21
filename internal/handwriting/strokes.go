package handwriting

import (
	"fmt"
	"time"
)

// DrawStrokes 用虚拟笔把若干笔画 (屏幕像素坐标) 依次写到屏幕上, 用的是当前笔。
// 全程保持 BTN_TOOL_PEN=1 (见 hoverEnter 注释), 笔间只抬笔。
func DrawStrokes(profile DeviceProfile, strokes [][][2]float32) error {
	pen, err := NewPen(profile)
	if err != nil {
		return err
	}
	defer pen.Close()
	_ = pen.hoverEnter()
	defer pen.hoverLeave()
	time.Sleep(150 * time.Millisecond)
	for _, st := range strokes {
		if len(st) < 2 {
			continue
		}
		if err := pen.gotoXY(int(st[0][0]), int(st[0][1])); err != nil {
			return err
		}
		if err := pen.penDown(); err != nil {
			return err
		}
		time.Sleep(15 * time.Millisecond) // 让笔锋引擎先认到落笔, 头几个点不丢
		for _, p := range st[1:] {
			if err := pen.gotoXY(int(p[0]), int(p[1])); err != nil {
				return err
			}
		}
		// 收笔: 系统要等后面的采样点才把最后一段落成笔迹, 直接抬笔会吞掉末尾 (实测爱心缺一截)。
		// 在终点原地多报几次, 再停一下才抬。
		last := st[len(st)-1]
		for i := 0; i < 4; i++ {
			if err := pen.gotoXY(int(last[0]), int(last[1])); err != nil {
				return err
			}
		}
		time.Sleep(25 * time.Millisecond)
		if err := pen.penUp(); err != nil {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

// Session 一次贴纸描写期间一直保持笔悬停, 笔画之间由调用方改笔的颜色/粗细。
// OnWrite 每写一个坐标回调一次 (给监听器登记, 分辨真笔); WaitClear 每笔开始前调用,
// 返回 false 表示真笔一直不走, 放弃这次描写。
type Session struct {
	pen       *Pen
	OnWrite   func(x, y int32)
	WaitClear func() bool
	Delay     time.Duration // 每个坐标之间的停顿 (0 = 默认 5ms)
	// 统计 (贴纸调速用): 等真笔离开累计、写点累计、点数
	WaitTotal time.Duration
	DrawTotal time.Duration
	Points    int
}

func Begin(profile DeviceProfile) (*Session, error) {
	pen, err := NewPen(profile)
	if err != nil {
		return nil, err
	}
	_ = pen.hoverEnter()
	time.Sleep(150 * time.Millisecond)
	return &Session{pen: pen}, nil
}

func (s *Session) Stroke(st [][2]float32) error {
	if len(st) < 2 {
		return nil
	}
	t0 := time.Now()
	if s.WaitClear != nil && !s.WaitClear() {
		return fmt.Errorf("笔还在屏幕附近, 这笔没描")
	}
	s.WaitTotal += time.Since(t0)
	t1 := time.Now()
	defer func() { s.DrawTotal += time.Since(t1); s.Points += len(st) }()
	pen := s.pen
	pen.onWrite = s.OnWrite
	pen.delay = s.Delay
	if err := pen.gotoXYF(st[0][0], st[0][1]); err != nil {
		return err
	}
	if err := pen.penDown(); err != nil {
		return err
	}
	time.Sleep(15 * time.Millisecond)
	for _, p := range st[1:] {
		if err := pen.gotoXYF(p[0], p[1]); err != nil {
			return err
		}
	}
	last := st[len(st)-1]
	for i := 0; i < 4; i++ {
		if err := pen.gotoXYF(last[0], last[1]); err != nil {
			return err
		}
	}
	time.Sleep(25 * time.Millisecond)
	return pen.penUp()
}

// PenStats 累计的 uinput 写入时间与停顿时间
func (s *Session) PenStats() (write, sleep time.Duration) { return s.pen.WriteTotal, s.pen.SleepTotal }

func (s *Session) End() {
	_ = s.pen.hoverLeave()
	_ = s.pen.Close()
}
