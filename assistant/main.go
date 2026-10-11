// 空间助手: 在 Mac/Windows 上通过 USB 直连 reMarkable, 检测机型/固件后安全地安装
// rmkit-cn (中文化/输入法/高级面板) 或 RMPPM 单槽 Android。
//
// 隐私原则: 没有服务器, 没有遥测; 只连用户给的地址; 只读系统信息, 不碰用户文档;
// 每条远端命令和写入的文件都记在本机审计日志里。
package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

// 由构建时 -ldflags "-X main.version=... -X main.buildTime=..." 注入 (见 Makefile)
var (
	version   = "dev"
	buildTime = ""
	// 这一版助手配套的空间安装包: 发布标签 (GitHub Release 与国内镜像的目录都按它分),
	// 安装包的 sha256 (下载后必须对上, 国内镜像走明文 http 也不怕被换包),
	// 国内镜像地址 (放在不进仓库的 .cn-mirror 里, 源码公开也不写死)
	releaseTag = ""
	spaceSHA   = ""
	cnMirror   = ""
)

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:            "空间助手",
		Width:            1040,
		Height:           800,
		MinWidth:         760,
		MinHeight:        600,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 250, G: 250, B: 248, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
	})
	if err != nil {
		println("Error:", err.Error())
	}
}
