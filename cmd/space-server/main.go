// space-server — 「空间」启动台的后端。
//
// 原来这套代码编译在 rmkit-cn 的 upload-server 里 (internal/space)。拆出来独立成进程,
// 这样空间可以自己迭代、自己发版, 不必跟着 rmkit-cn 走。
//
// 与 rmkit-cn 的关系: 空间里那些"系统功能入口"应用 (蓝牙/输入法/上传/个性化/
// 切 Android/微信读书…) 的启动动作仍然要由 rmkit-cn 的后端执行 —— 它掌握 systemd、
// rootdev、xovi 这些东西。所以本进程把自己不认识的 /apps/* 请求原样转发过去,
// 转发目标由 -proxy 指定。空间自己的接口全在 /space/* 下, 不会撞。
package main

import (
	"flag"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"runtime"

	"github.com/boangs/remarkable-space/server"
)

func deviceArch() string {
	switch runtime.GOARCH {
	case "arm64":
		return "aarch64"
	case "arm":
		return "armv7"
	}
	return runtime.GOARCH
}

func main() {
	home := os.Getenv("HOME")
	if home == "" {
		home = "/home/root"
	}
	var (
		listen   = flag.String("listen", "127.0.0.1:8081", "监听地址")
		root     = flag.String("root", "/home/root/rmkit-cn/space", "内置应用所在目录 (下含 apps/)")
		userDir  = flag.String("user-apps", filepath.Join(home, ".local/share/rmkit-cn/space/apps"), "绿色安装的应用目录")
		dataDir  = flag.String("data", filepath.Join(home, ".local/share/rmkit-cn/space/data"), "应用私有数据目录")
		baseURL  = flag.String("base-url", "", "给应用的 HTTP 基址, 空=用监听地址")
		store    = flag.String("store", "", "应用商店索引 URL, 空=不提供商店")
		proxyURL = flag.String("proxy", "http://127.0.0.1:8080", "不认识的 /apps/* 转发到哪 (rmkit-cn 后端)")
	)
	flag.Parse()

	h := server.New(filepath.Join(*root, "apps"), *userDir, *dataDir, deviceArch(), *baseURL)
	h.StoreURL = *store

	mux := http.NewServeMux()
	h.Mount(mux)

	// 系统功能入口的启动动作由 rmkit-cn 执行 (它管 systemd / rootdev / xovi)
	if *proxyURL != "" {
		if u, err := url.Parse(*proxyURL); err == nil {
			rp := httputil.NewSingleHostReverseProxy(u)
			mux.Handle("/apps/", rp)
		}
	}

	h.AutoStart() // 清单里声明了 autostart 的应用后台

	log.Printf("space-server 监听 %s  内置=%s  用户=%s", *listen, *root, *userDir)
	if err := http.ListenAndServe(*listen, mux); err != nil {
		log.Fatal(err)
	}
}
