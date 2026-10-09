package xolib

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Mount 挂外壳用的三个接口:
//
//	GET  /space/library/next?wait=25  长连接取活, 没活时等到超时返回 204
//	POST /space/library/done          回报一条结果
//	POST /space/library/bye           外壳关闭 (之后上传一律拒绝)
//	GET  /space/library/status        空间开着没有 / 还有几条没通知 (扫码页用)
func (l *Library) Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /space/library/next", func(w http.ResponseWriter, r *http.Request) {
		wait := 25 * time.Second
		if s, err := strconv.Atoi(r.URL.Query().Get("wait")); err == nil && s >= 0 && s <= 55 {
			wait = time.Duration(s) * time.Second
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
		l.Queue.Done(res)
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
