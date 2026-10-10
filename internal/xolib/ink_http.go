package xolib

import (
	"encoding/json"
	"net/http"
)

// Mount 挂手写画布的接口 (只在本机/USB 端口; 局域网白名单不放):
//
//	POST   /space/ink/load     {app, key, x, y}  把正本拷进载体, 返回载体 ID (外壳随后后台打开它);
//	                                 x,y 是画布此刻在屏幕上的位置 (正本存画布坐标, 载体存屏幕坐标)
//	POST   /space/ink/save     {app, key}  外壳关掉载体后调: 拷回正本
//	GET    /space/ink/drawing?app=&key=[&live=1]  能直接画的折线 (live=1 读载体里正在写的)
//	DELETE /space/ink?app=&key=            删掉这块画布
func (k *Ink) Mount(mux *http.ServeMux) {
	type req struct {
		App, Key string
		X, Y     float32
	}
	read := func(w http.ResponseWriter, r *http.Request) (req, bool) {
		var in req
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "要 {app, key}"})
			return in, false
		}
		return in, true
	}
	fail := func(w http.ResponseWriter, err error) {
		code := http.StatusInternalServerError
		if err == ErrBadKey {
			code = http.StatusBadRequest
		}
		writeJSON(w, code, map[string]any{"error": err.Error()})
	}
	mux.HandleFunc("POST /space/ink/load", func(w http.ResponseWriter, r *http.Request) {
		in, ok := read(w, r)
		if !ok {
			return
		}
		s, err := k.Load(in.App, in.Key, in.X, in.Y)
		if err != nil {
			fail(w, err)
			return
		}
		if s.Created {
			// 新建的载体要等外壳把它加载进文件库, 不然外壳拿不到文档对象
			k.Lib.Queue.Wait(s.CarrierID, LoadWait)
		}
		writeJSON(w, http.StatusOK, s)
	})
	mux.HandleFunc("POST /space/ink/save", func(w http.ResponseWriter, r *http.Request) {
		in, ok := read(w, r)
		if !ok {
			return
		}
		if err := k.Save(in.App, in.Key); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
	mux.HandleFunc("GET /space/ink/drawing", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		d, err := k.Strokes(q.Get("app"), q.Get("key"), q.Get("live") == "1")
		if err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	})
	mux.HandleFunc("DELETE /space/ink", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if err := k.Delete(q.Get("app"), q.Get("key")); err != nil {
			fail(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	})
}
