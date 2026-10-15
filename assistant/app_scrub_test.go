package main

import (
	"strings"
	"testing"
)

// 日志会被用户截图发出来求助, 分发地址不该跟着走。net/http 的错误自带整条 URL,
// 所以打进日志前必须过一道 scrubURL。
func TestScrubURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{`Get "https://example.com/space/space-bundle.zip": dial tcp 1.2.3.4:443: i/o timeout`,
			`Get "space-bundle.zip": dial tcp 1.2.3.4:443: i/o timeout`},
		// 指向目录: 连路径片段都不留
		{`Get "https://example.com/space/": EOF`, `Get "服务器": EOF`},
		{`Get "https://example.com": EOF`, `Get "服务器": EOF`},
		// 带 query 的要把 token 之类一并丢掉
		{`Get "http://x.cn/a/b.zip?token=abc123": 404`, `Get "b.zip": 404`},
		// 代理地址是用户自己填的, 留着才好排错
		{`proxyconnect tcp: dial tcp 127.0.0.1:7890: connection refused`,
			`proxyconnect tcp: dial tcp 127.0.0.1:7890: connection refused`},
		// 没有地址的错误原样不动
		{`sha256 不符, 安装包已损坏`, `sha256 不符, 安装包已损坏`},
	}
	for _, c := range cases {
		if got := scrubURL(c.in); got != c.want {
			t.Errorf("scrubURL(%q)\n 得到 %q\n 应为 %q", c.in, got, c.want)
		}
	}
}

// DownloadBundle 失败时返回给前端的错误会被原样 appendLog。这条路径以前漏过:
// 日志行过滤了, 返回值没过滤, 等于白过滤。
func TestDownloadBundleErrorHasNoURL(t *testing.T) {
	a := &App{}
	_, err := a.DownloadBundle([]string{"https://example.invalid/space/space-bundle.zip"}, "")
	if err == nil {
		t.Fatal("指向不存在的主机, 应当报错")
	}
	if strings.Contains(err.Error(), "example.invalid") || strings.Contains(err.Error(), "https://") {
		t.Errorf("返回给前端的错误里还有地址: %q", err.Error())
	}
}
