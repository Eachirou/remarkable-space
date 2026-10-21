//go:build linux

package handwriting

import (
	"syscall"
	"time"
)

// preciseSleep 毫秒级精确停顿。设备内核没有高精度定时器 (2026-10-21 rmppm 实测 time.Sleep 与
// nanosleep 设 1ms 都要 5-6ms, 按时钟节拍取整), 虚拟笔每点一次就慢了 6 倍。短于 3ms 的停顿改为
// 自旋等待 (一张贴纸几百个点, 自旋总共不到 1 秒), 更长的仍用 nanosleep。
func preciseSleep(d time.Duration) {
	if d <= 0 {
		return
	}
	if d < 3*time.Millisecond {
		t0 := time.Now()
		for time.Since(t0) < d {
		}
		return
	}
	ts := syscall.NsecToTimespec(d.Nanoseconds())
	_ = syscall.Nanosleep(&ts, nil)
}
