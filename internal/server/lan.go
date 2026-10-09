package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// 扫码上传的对外端口: 只在「空间」开着时在 Wi-Fi 等局域网地址上监听, 空间一关端口就关掉,
// 手机那边直接连不上 (不是看到一句提示)。
//
// 平时的 8080 只听本机 127.0.0.1 —— 笔记本里的 AI 按钮、空间外壳、应用后台都在设备上,
// 用不着对局域网开放; 以前整个 8080 对同网段开着, 谁都能读 AI 配置、装应用。
// USB 网段 (usb0, 10.11.99.x) 例外, 一直开着: 只有插线的电脑够得着, 开发和排障靠它。
//
// 开关全在本进程里做, 不碰 systemd / /etc —— 那里的每次写入都有变砖风险 (见 issue #4)。

// lanAllowed: 对外端口只放手机扫码页用得到的接口, 其余一律 404。
func lanAllowed(r *http.Request) bool {
	p := r.URL.Path
	switch {
	case p == "/" || p == "/qr":
		return r.Method == http.MethodGet
	case p == "/ai-config":
		return r.Method == http.MethodGet || r.Method == http.MethodPut
	case p == "/documents":
		return r.Method == http.MethodPost
	case p == "/space/apps":
		return r.Method == http.MethodGet || r.Method == http.MethodPost
	case strings.HasPrefix(p, "/space/apps/"):
		// 只有删除整个应用: /space/apps/{id}; 应用数据、后台启停这些子路径不对外
		return r.Method == http.MethodDelete && !strings.Contains(strings.TrimPrefix(p, "/space/apps/"), "/")
	}
	for _, base := range []string{"/fonts", "/screens", "/hero", "/music"} {
		if p == base {
			return r.Method == http.MethodGet || r.Method == http.MethodPost
		}
		if strings.HasPrefix(p, base+"/") {
			rest := strings.TrimPrefix(p, base+"/")
			if strings.Contains(rest, "/") {
				return false // /fonts/x/apply、/screens/x/preview 这类是设备上用的
			}
			return r.Method == http.MethodDelete || (r.Method == http.MethodGet && base == "/hero")
		}
	}
	return false
}

func lanFilter(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !lanAllowed(r) {
			http.NotFound(w, r)
			return
		}
		h.ServeHTTP(w, r)
	})
}

// isUSBAddr: reMarkable 的 USB 网卡固定在 10.11.99.0/24。
func isUSBAddr(ip net.IP) bool {
	ip4 := ip.To4()
	return ip4 != nil && ip4[0] == 10 && ip4[1] == 11 && ip4[2] == 99
}

// localAddrs 列出本机 IPv4 地址: usb 是 USB 网段的, lan 是其余非回环的 (Wi-Fi)。
func localAddrs() (usb, lan []string) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, nil
	}
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok || ipn.IP.To4() == nil {
				continue
			}
			if isUSBAddr(ipn.IP) {
				usb = append(usb, ipn.IP.String())
			} else {
				lan = append(lan, ipn.IP.String())
			}
		}
	}
	return usb, lan
}

// listeners 管一组按地址开关的监听 (USB 常开 / 局域网随空间开关)。
type listeners struct {
	mu   sync.Mutex
	port int
	h    http.Handler
	open map[string]*http.Server
	tag  string
}

func newListeners(port int, h http.Handler, tag string) *listeners {
	return &listeners{port: port, h: h, open: map[string]*http.Server{}, tag: tag}
}

// sync 让正在监听的地址正好等于 want: 多的关掉, 缺的打开。Wi-Fi 换了地址也靠它跟上。
func (l *listeners) sync(want []string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	keep := map[string]bool{}
	for _, ip := range want {
		keep[ip] = true
		if _, ok := l.open[ip]; ok {
			continue
		}
		addr := net.JoinHostPort(ip, fmt.Sprint(l.port))
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[%s] 监听 %s 失败: %v\n", l.tag, addr, err)
			continue
		}
		srv := &http.Server{Handler: l.h, ReadHeaderTimeout: 10 * time.Second}
		l.open[ip] = srv
		go srv.Serve(ln)
		fmt.Fprintf(os.Stderr, "[%s] 开: %s\n", l.tag, addr)
	}
	for ip, srv := range l.open {
		if keep[ip] {
			continue
		}
		// 正在传的文件给 5 秒收尾, 之后强制断开
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = srv.Shutdown(ctx)
		cancel()
		delete(l.open, ip)
		fmt.Fprintf(os.Stderr, "[%s] 关: %s\n", l.tag, ip)
	}
}

func (l *listeners) addrs() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]string, 0, len(l.open))
	for ip := range l.open {
		out = append(out, ip)
	}
	return out
}

// ServeExternal 跑 USB 常开端口与局域网随空间开关的端口, 不返回。
// 每秒对一次: 空间开没开 (外壳长连接在不在)、本机现在有哪些地址。
// full 是和本机端口同一个路由 (s.Routes() 的结果), 别再建一份。
func (s *Server) ServeExternal(port int, full http.Handler) {
	usbL := newListeners(port, full, "usb")
	s.lan = newListeners(port, lanFilter(full), "扫码上传")
	for {
		usb, lan := localAddrs()
		usbL.sync(usb)
		if s.lib.Queue.Active() {
			s.lan.sync(lan)
		} else {
			s.lan.sync(nil)
		}
		time.Sleep(time.Second)
	}
}
