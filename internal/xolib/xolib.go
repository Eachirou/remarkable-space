// Package xolib 往 xochitl 的文件库里放东西 (热导入), 不重启 xochitl。
//
// 分两步, 只有第一步在本进程里:
//  1. 把文件写进 xochitl 数据目录: <uuid>.<ext> + .content + .metadata (最后写 metadata)。
//  2. 通知 xochitl 的文件库加载这个 uuid —— 要调它内存里 Library 对象的 requestLoadEntry,
//     外部进程够不着 (xochitl 没有 D-Bus 服务, 也不监听目录), 只能由跑在 xochitl 里的
//     「空间」外壳来调。所以第二步是排进队列, 外壳打开时长连接取走、调完回报。
//
// 因此本包同时记录「空间开着没有」: 外壳在线才接受上传 (Active), 空间关了上传就该关。
//
// 为什么不用 librarian: 它在 3.28 上找不到改了名的 Library 单例, 无限重试漏堆,
// 一周把 xochitl 撑到 1.6GB。这里不往 xochitl 里塞任何原生代码。
package xolib

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// DefaultDir 是设备上 xochitl 的数据目录。
const DefaultDir = "/home/root/.local/share/remarkable/xochitl"

var ErrBadType = errors.New("仅支持 .pdf / .epub")

// Library 是对 xochitl 数据目录的一个写入口, 带着通知队列。
type Library struct {
	Dir   string
	Queue *Queue
	now   func() time.Time
}

// New 建一个写入口; pendingFile 是待通知队列落盘的位置 (空 = 只在内存里, 测试用)。
func New(dir, pendingFile string) *Library {
	return &Library{Dir: dir, Queue: NewQueue(pendingFile), now: time.Now}
}

// Entry 是导入后的条目。
type Entry struct {
	ID          string `json:"uuid"`
	VisibleName string `json:"name"`
	Type        string `json:"type"` // pdf / epub
}

// ImportDocument 把一份 PDF / EPUB 写进文件库并排队通知。parent 为空 = 「我的文件」根目录。
// visibleName 是文件库里显示的名字 (一般是去掉扩展名的原文件名)。
func (l *Library) ImportDocument(r io.Reader, visibleName, ext, parent string) (Entry, error) {
	ext = strings.TrimPrefix(strings.ToLower(ext), ".")
	if ext != "pdf" && ext != "epub" {
		return Entry{}, ErrBadType
	}
	id, err := newUUID()
	if err != nil {
		return Entry{}, err
	}
	if strings.TrimSpace(visibleName) == "" {
		visibleName = "未命名"
	}
	// 顺序: 本体 → .content → .metadata。xochitl 下次扫目录 (或重启) 只认带 .metadata 的条目,
	// metadata 最后落地, 就不会出现「有条目没本体」的半成品。
	if err := l.writeFile(id+"."+ext, r); err != nil {
		return Entry{}, err
	}
	content := map[string]any{"fileType": ext}
	if err := l.writeJSON(id+".content", content); err != nil {
		l.removeAll(id)
		return Entry{}, err
	}
	ms := strconv.FormatInt(l.now().UnixMilli(), 10)
	meta := map[string]any{
		"createdTime": ms, "lastModified": ms, "lastOpened": "0", "lastOpenedPage": 0,
		"new": false, "parent": parent, "pinned": false, "source": "",
		"type": "DocumentType", "visibleName": visibleName,
	}
	if err := l.writeJSON(id+".metadata", meta); err != nil {
		l.removeAll(id)
		return Entry{}, err
	}
	l.Queue.Enqueue(Job{Op: OpLoad, ID: id})
	return Entry{ID: id, VisibleName: visibleName, Type: ext}, nil
}

// writeFile 先写同目录下的临时名再 rename: 断电也不会留下半个文件。
// 临时名以 . 开头且不是 uuid 形状, xochitl 扫目录时不会把它当条目。
func (l *Library) writeFile(name string, r io.Reader) error {
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return err
	}
	tmp := filepath.Join(l.Dir, ".xolib-"+name+".tmp")
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, r); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, filepath.Join(l.Dir, name))
}

func (l *Library) writeJSON(name string, v any) error {
	b, err := json.MarshalIndent(v, "", "    ")
	if err != nil {
		return err
	}
	return l.writeFile(name, strings.NewReader(string(b)))
}

func (l *Library) removeAll(id string) {
	matches, _ := filepath.Glob(filepath.Join(l.Dir, id+".*"))
	for _, m := range matches {
		os.Remove(m)
	}
}

// newUUID 生成 v4 UUID (xochitl 的条目 ID 就是这个格式)。
func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
