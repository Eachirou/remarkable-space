package xolib

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// 交给 xochitl 里的「空间」外壳去做的操作。现在只有 load, 以后的建文件夹 / 建笔记本 /
// 按 ID 打开也走这里 —— 外壳认识的 op 才会做, 不认识的回报失败。
const OpLoad = "load"

type Job struct {
	Op string `json:"op"`
	ID string `json:"id"`
	// NotBefore: 做失败的活要等一阵再给外壳, 否则「取 → 失败 → 立刻排回 → 立刻再取」空转
	NotBefore time.Time `json:"notBefore,omitzero"`
}

// RetryDelay 失败的活多久之后再试。
const RetryDelay = 30 * time.Second

type Result struct {
	Op    string `json:"op"`
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// ActiveWindow: 外壳最后一次来取活之后这么久内, 算「空间开着」。外壳的长连接 25 秒一轮,
// 取完立刻再连, 所以正常开着时间隔接近 0; 留 35 秒余量兜住网络抖动。
const ActiveWindow = 35 * time.Second

// Queue 是待通知队列 + 外壳在线状态。待通知的条目落盘: 空间在文件写完、通知之前被关掉,
// 或 space-server 中途重启, 下次打开空间照样补上, 不会出现「文件在磁盘上却永远看不到」。
type Queue struct {
	mu       sync.Mutex
	file     string
	pending  []Job
	wake     chan struct{}
	waiters  map[string][]chan Result
	lastSeen time.Time
	polling  int  // 正挂着的长连接数
	closed   bool // 外壳主动说了「我关了」
	now      func() time.Time
}

func NewQueue(file string) *Queue {
	q := &Queue{file: file, wake: make(chan struct{}), waiters: map[string][]chan Result{}, now: time.Now}
	if file != "" {
		if b, err := os.ReadFile(file); err == nil {
			_ = json.Unmarshal(b, &q.pending)
		}
	}
	return q
}

// Enqueue 排一个活, 叫醒正在等的外壳。
func (q *Queue) Enqueue(j Job) {
	q.mu.Lock()
	q.pending = append(q.pending, j)
	q.saveLocked()
	close(q.wake)
	q.wake = make(chan struct{})
	q.mu.Unlock()
}

// Next 给外壳取活: 有能做的就立刻全部拿走, 没有就最多等 wait。拿走即出队 —— 外壳做完回报,
// 失败的由 Done 延后重排。
func (q *Queue) Next(ctx context.Context, wait time.Duration) []Job {
	q.mu.Lock()
	q.closed = false
	q.lastSeen = q.now()
	q.polling++
	defer func() {
		q.mu.Lock()
		q.polling--
		q.lastSeen = q.now()
		q.mu.Unlock()
	}()
	deadline := q.now().Add(wait)
	for {
		now := q.now()
		var ready, later []Job
		var soonest time.Time
		for _, j := range q.pending {
			if j.NotBefore.After(now) {
				later = append(later, j)
				if soonest.IsZero() || j.NotBefore.Before(soonest) {
					soonest = j.NotBefore
				}
			} else {
				ready = append(ready, j)
			}
		}
		if len(ready) > 0 {
			q.pending = later
			q.saveLocked()
			q.mu.Unlock()
			for i := range ready {
				ready[i].NotBefore = time.Time{}
			}
			return ready
		}
		left := deadline.Sub(now)
		if left <= 0 {
			q.mu.Unlock()
			return nil
		}
		if !soonest.IsZero() && soonest.Sub(now) < left {
			left = soonest.Sub(now)
		}
		wake := q.wake
		q.mu.Unlock()
		t := time.NewTimer(left)
		select {
		case <-wake:
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return nil
		}
		t.Stop()
		q.mu.Lock()
	}
}

// Done 收外壳的回报。失败的 load 重新排回去 (下次打开空间再试), 并通知等结果的人。
func (q *Queue) Done(r Result) {
	q.mu.Lock()
	if !r.OK && r.Op == OpLoad {
		q.pending = append(q.pending, Job{Op: r.Op, ID: r.ID, NotBefore: q.now().Add(RetryDelay)})
		q.saveLocked()
	}
	ws := q.waiters[r.ID]
	delete(q.waiters, r.ID)
	q.mu.Unlock()
	for _, w := range ws {
		w <- r
	}
}

// Wait 等某个 id 的回报, 超时返回 false (文件已经在磁盘上, 只是还没出现在文件库里)。
func (q *Queue) Wait(id string, timeout time.Duration) (Result, bool) {
	ch := make(chan Result, 1)
	q.mu.Lock()
	q.waiters[id] = append(q.waiters[id], ch)
	q.mu.Unlock()
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case r := <-ch:
		return r, true
	case <-t.C:
		q.mu.Lock()
		ws := q.waiters[id]
		for i, w := range ws {
			if w == ch {
				q.waiters[id] = append(ws[:i], ws[i+1:]...)
				break
			}
		}
		if len(q.waiters[id]) == 0 {
			delete(q.waiters, id)
		}
		q.mu.Unlock()
		return Result{}, false
	}
}

// Close 外壳关闭时调: 立刻算作「空间关了」, 不必等 ActiveWindow 过期。
func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.mu.Unlock()
}

// Active 空间开着没有: 外壳正挂着长连接, 或刚取过活不久, 且没有主动说关了。
func (q *Queue) Active() bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	return q.polling > 0 || (!q.lastSeen.IsZero() && q.now().Sub(q.lastSeen) < ActiveWindow)
}

// Pending 待通知的条目数 (状态接口用)。
func (q *Queue) Pending() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.pending)
}

func (q *Queue) saveLocked() {
	if q.file == "" {
		return
	}
	b, _ := json.Marshal(q.pending)
	_ = os.MkdirAll(filepath.Dir(q.file), 0o755)
	tmp := q.file + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		_ = os.Rename(tmp, q.file)
	}
}
