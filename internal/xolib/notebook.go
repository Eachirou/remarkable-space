package xolib

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// ErrShellClosed 导入器模式下建文件夹 / 笔记本要外壳来做, 空间没开或没回报。
var ErrShellClosed = errors.New("空间没有打开, 建不了")

// createViaShell 导入器模式 (3.29 起): 让外壳调 LibraryController 建, 等它回报系统给的 uuid。
func (l *Library) createViaShell(op, name, parent string) (string, error) {
	key, err := newUUID()
	if err != nil {
		return "", err
	}
	l.Queue.Enqueue(Job{Op: op, ID: key, Name: name, Parent: parent})
	res, ok := l.Queue.Wait(key, LoadWait)
	if !ok {
		l.Queue.Drop(key)
		return "", ErrShellClosed
	}
	if !res.OK || res.NewID == "" {
		return "", fmt.Errorf("系统没建成: %s", res.Error)
	}
	return res.NewID, nil
}

// firstPageID 读系统建好的笔记本的第一页 ID。.content 是系统异步写的, 等一会儿。
func (l *Library) firstPageID(id string) (string, error) {
	var c struct {
		CPages struct {
			Pages []struct{ ID string } `json:"pages"`
		} `json:"cPages"`
	}
	for i := 0; i < 20; i++ {
		if b, err := os.ReadFile(filepath.Join(l.Dir, id+".content")); err == nil &&
			json.Unmarshal(b, &c) == nil && len(c.CPages.Pages) > 0 && c.CPages.Pages[0].ID != "" {
			return c.CPages.Pages[0].ID, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("笔记本 %s 建好了但读不到页", id)
}

// CreateFolder 建一个文件夹 (文件库里的 CollectionType 条目), parent 为空 = 根目录。
// 文件夹在磁盘上只是两份 JSON, 不是目录; 写完排队让文件库加载。
func (l *Library) CreateFolder(name, parent string) (Entry, error) {
	if l.Queue.Importer() {
		id, err := l.createViaShell(OpFolder, name, parent)
		if err != nil {
			return Entry{}, err
		}
		return Entry{ID: id, VisibleName: name, Type: "folder", loaded: true}, nil
	}
	id, err := newUUID()
	if err != nil {
		return Entry{}, err
	}
	if err := l.writeJSON(id+".content", map[string]any{"tags": []any{}}); err != nil {
		return Entry{}, err
	}
	ms := strconv.FormatInt(l.now().UnixMilli(), 10)
	meta := map[string]any{
		"createdTime": ms, "lastModified": ms, "parent": parent, "pinned": false,
		"type": "CollectionType", "visibleName": name,
	}
	if err := l.writeJSON(id+".metadata", meta); err != nil {
		l.removeAll(id)
		return Entry{}, err
	}
	l.Queue.Enqueue(Job{Op: OpLoad, ID: id})
	return Entry{ID: id, VisibleName: name, Type: "folder"}, nil
}

// Notebook 是新建的笔记本: ID 与第一页的页 ID (笔迹写进 <ID>/<页ID>.rm)。
type Notebook struct {
	Entry
	PageID string `json:"pageId"`
}

// CreateNotebook 建一本只有一页的空白笔记本。字段照原生笔记本的 .content 写 (3.28 实测),
// 只保留必需的: 页列表 cPages、文件类型、格式版本、方向、页数。笔迹文件不预建 ——
// 页面上第一次落笔时 xochitl 自己生成 <ID>/<页ID>.rm。
// template 是页面模板名 ("Blank" 空白, "P Lines small" 等), 空 = Blank。
//
// hidden: 写 "deleted": true。注意 3.28 不认它 (照样显示, 3.5 起删除改用 .tombstone);
// 想藏起来要 parent = "trash" —— 回收站里的笔记本照样能在后台打开写入 (手写载体就这么用,
// 见应用仓库 lib/ink)。
//
// 导入器模式 (3.29 起) 由系统建: 模板用系统默认 (template 不生效), hidden 直接建进回收站。
func (l *Library) CreateNotebook(name, parent, template string, hidden bool) (Notebook, error) {
	if l.Queue.Importer() {
		if hidden {
			parent = "trash"
		}
		id, err := l.createViaShell(OpNotebook, name, parent)
		if err != nil {
			return Notebook{}, err
		}
		pid, err := l.firstPageID(id)
		if err != nil {
			return Notebook{}, err
		}
		return Notebook{Entry: Entry{ID: id, VisibleName: name, Type: "notebook", loaded: true}, PageID: pid}, nil
	}
	id, err := newUUID()
	if err != nil {
		return Notebook{}, err
	}
	pid, err := newUUID()
	if err != nil {
		return Notebook{}, err
	}
	if template == "" {
		template = "Blank"
	}
	content := map[string]any{
		"cPages": map[string]any{
			"lastOpened": map[string]any{"timestamp": "0:0", "value": ""},
			"original":   map[string]any{"timestamp": "0:0", "value": -1},
			"pages": []any{map[string]any{
				"id":       pid,
				"idx":      map[string]any{"timestamp": "1:2", "value": "ba"},
				"template": map[string]any{"timestamp": "1:1", "value": template},
			}},
		},
		"coverPageNumber":  -1,
		"documentMetadata": map[string]any{},
		"extraMetadata":    map[string]any{},
		"fileType":         "notebook",
		"formatVersion":    2,
		"orientation":      "portrait",
		"pageCount":        1,
		"pageTags":         []any{},
		"tags":             []any{},
	}
	if err := l.writeJSON(id+".content", content); err != nil {
		return Notebook{}, err
	}
	ms := strconv.FormatInt(l.now().UnixMilli(), 10)
	meta := map[string]any{
		"createdTime": ms, "lastModified": ms, "lastOpened": "0", "lastOpenedPage": 0,
		"new": false, "parent": parent, "pinned": false, "source": "",
		"type": "DocumentType", "visibleName": name,
	}
	if hidden {
		meta["deleted"] = true
	}
	if err := l.writeJSON(id+".metadata", meta); err != nil {
		l.removeAll(id)
		return Notebook{}, err
	}
	l.Queue.Enqueue(Job{Op: OpLoad, ID: id})
	return Notebook{Entry: Entry{ID: id, VisibleName: name, Type: "notebook"}, PageID: pid}, nil
}

// String 方便日志
func (n Notebook) String() string { return fmt.Sprintf("%s(%s) 页 %s", n.VisibleName, n.ID, n.PageID) }
