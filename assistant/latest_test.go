package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// 第一个地址不通 / 内容不像版本号就换下一个; 都不行返回空 (前端退回用发布标签比)。
func TestLatestVersion(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("v1.1.5\n"))
	}))
	defer ok.Close()
	missing := httptest.NewServer(http.NotFoundHandler())
	defer missing.Close()
	junk := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("<html>运营商劫持页</html>"))
	}))
	defer junk.Close()

	a := &App{}
	if v := a.LatestVersion([]string{missing.URL, junk.URL, ok.URL}, ""); v != "v1.1.5" {
		t.Errorf("得到 %q, 应为 v1.1.5", v)
	}
	if v := a.LatestVersion([]string{missing.URL, junk.URL}, ""); v != "" {
		t.Errorf("都读不到时得到 %q, 应为空", v)
	}
}
