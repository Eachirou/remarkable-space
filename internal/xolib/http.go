package xolib

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// LoadWait 建条目后等外壳回报「文件库已加载」的时间。等不到不算失败 (文件在盘上, 下次打开空间补上)。
var LoadWait = 8 * time.Second

// Mount 挂外壳用的三个接口:
//
//	GET  /space/library/next?wait=25  长连接取活, 没活时等到超时返回 204
//	POST /space/library/done          回报一条结果
//	POST /space/library/bye           外壳关闭 (之后上传一律拒绝)
//	GET  /space/library/status        空间开着没有 / 还有几条没通知 (扫码页用)
//	POST /space/library/folder        {name, parent} 建文件夹
//	POST /space/library/notebook      {name, parent, template} 建一页的空白笔记本
//
// 建完都会等文件库加载的回报 (最多 LoadWait), 结果里 loaded 说明这次有没有已经出现在文件库。
// 只挂在本机/USB 端口上 —— 局域网白名单 (server/lan.go) 不放它们。
func (l *Library) Mount(mux *http.ServeMux) {
	mux.HandleFunc("POST /space/library/folder", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Name, Parent string }
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil || in.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "要 name"})
			return
		}
		e, err := l.CreateFolder(in.Name, in.Parent)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		res, ok := l.Queue.Wait(e.ID, LoadWait)
		writeJSON(w, http.StatusOK, map[string]any{"entry": e, "loaded": ok && res.OK})
	})
	mux.HandleFunc("POST /space/library/notebook", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Name, Parent, Template string
			Hidden                 bool
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&in); err != nil || in.Name == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "要 name"})
			return
		}
		nb, err := l.CreateNotebook(in.Name, in.Parent, in.Template, in.Hidden)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		res, ok := l.Queue.Wait(nb.ID, LoadWait)
		writeJSON(w, http.StatusOK, map[string]any{"notebook": nb, "loaded": ok && res.OK})
	})
	mux.HandleFunc("GET /space/library/next", func(w http.ResponseWriter, r *http.Request) {
		wait := 25 * time.Second
		if s, err := strconv.Atoi(r.URL.Query().Get("wait")); err == nil && s >= 0 && s <= 55 {
			wait = time.Duration(s) * time.Second
		}
		// 外壳报模式: mode=import 表示这台固件没有 requestLoadEntry, 要走导入器
		switch r.URL.Query().Get("mode") {
		case "import":
			l.SetImporter(true)
		case "load":
			l.SetImporter(false)
		}
		jobs := l.Queue.Next(r.Context(), wait)
		if len(jobs) == 0 {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
	})
	mux.HandleFunc("POST /space/library/done", func(w http.ResponseWriter, r *http.Request) {
		var res Result
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&res); err != nil || res.ID == "" {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "bad result"})
			return
		}
		l.Queue.mu.Lock()
		j, isImport := l.Queue.inflight[res.ID]
		l.Queue.mu.Unlock()
		l.Queue.Done(res)
		if isImport && j.Op == OpImport && res.OK {
			l.doneImport(j)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /space/library/bye", func(w http.ResponseWriter, r *http.Request) {
		l.Queue.Close()
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /space/library/status", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"open": l.Queue.Active(), "pending": l.Queue.Pending()})
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
