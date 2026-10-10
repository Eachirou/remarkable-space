package xolib

import (
	"fmt"
	"strconv"
)

// CreateFolder 建一个文件夹 (文件库里的 CollectionType 条目), parent 为空 = 根目录。
// 文件夹在磁盘上只是两份 JSON, 不是目录; 写完排队让文件库加载。
func (l *Library) CreateFolder(name, parent string) (Entry, error) {
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
func (l *Library) CreateNotebook(name, parent, template string, hidden bool) (Notebook, error) {
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
