// assistant-cli 是桌面助手内核层的命令行入口, 用来在没有 GUI 的环境 (CI / 远程排障)
// 走一遍与 GUI 完全相同的探测 → 计划 → 执行流程。
//
//	assistant-cli -host 10.11.99.1 -password xxx probe
//	assistant-cli ... -bundle space-bundle.zip plan-rmkit | run-rmkit | uninstall-rmkit
//	assistant-cli ... -bundle android-rmppm-bundle.zip plan-android | run-android | uninstall-android
//	assistant-cli ... boot-android | return-stock
//
// plan-offline 不连设备, 用 -arch/-fw/-model 伪造探测结果算落点, 给 CI 与改完
// 安装逻辑后的自查用:
//
//	assistant-cli -bundle space-bundle.zip -arch aarch64 -fw 20260806095513 plan-offline
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/boangs/remarkable-space/assistant/internal/android"
	"github.com/boangs/remarkable-space/assistant/internal/bundle"
	"github.com/boangs/remarkable-space/assistant/internal/deploy"
	"github.com/boangs/remarkable-space/assistant/internal/probe"
	"github.com/boangs/remarkable-space/assistant/internal/sshx"
)

func main() {
	host := flag.String("host", "10.11.99.1", "设备地址")
	password := flag.String("password", os.Getenv("RM_PASSWORD"), "SSH 密码 (或环境变量 RM_PASSWORD)")
	bpath := flag.String("bundle", "", "安装包 zip")
	replace := flag.Bool("replace-system", false, "android: 覆盖已有 Android 系统")
	removeData := flag.Bool("remove-data", false, "uninstall-android: 连同 /home 数据删除")
	offArch := flag.String("arch", "aarch64", "plan-offline: 架构 aarch64 | armv7l")
	offFW := flag.String("fw", "20260806095513", "plan-offline: 固件版本")
	offModel := flag.String("model", "rmppm", "plan-offline: 机型 rm2 | rmpp | rmppm")
	offXovi := flag.Bool("have-xovi", true, "plan-offline: 设备已有 xovi")
	flag.Parse()
	cmd := flag.Arg(0)
	if cmd == "" {
		flag.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log := func(s string) { fmt.Println(s) }

	// plan-offline 只做纯计算, 不开 SSH
	if cmd == "plan-offline" {
		if *bpath == "" {
			die(fmt.Errorf("plan-offline 需要 -bundle"))
		}
		b, err := bundle.Open(*bpath, false)
		die(err)
		defer b.Close()
		p, err := deploy.NewPlan(probe.Info{
			Arch: *offArch, FWVersion: *offFW, ModelKey: *offModel, HaveXovi: *offXovi,
		}, b)
		die(err)
		dump(p)
		return
	}

	c, err := sshx.Dial(ctx, *host, *password, log)
	die(err)
	defer c.Close()
	info, err := probe.Run(ctx, c)
	die(err)

	var b *bundle.Bundle
	if *bpath != "" {
		b, err = bundle.Open(*bpath, true)
		die(err)
		defer b.Close()
		fmt.Printf("安装包: %s 版本 %s, %d 个文件\n", b.Manifest.Component, b.Manifest.Version, len(b.Manifest.Files))
	}
	need := func() {
		if b == nil {
			die(fmt.Errorf("%s 需要 -bundle", cmd))
		}
	}
	switch cmd {
	case "probe":
		dump(info)
	case "plan-rmkit":
		need()
		p, err := deploy.NewPlan(info, b)
		die(err)
		dump(p)
	case "run-rmkit":
		need()
		p, err := deploy.NewPlan(info, b)
		die(err)
		die(deploy.Run(ctx, c, b, p, log))
	case "uninstall-rmkit":
		die(deploy.Uninstall(ctx, c))
	case "plan-android", "run-android":
		need()
		res, err := c.Run(ctx, "[ -e /home/root/android-system/system/bin/init ] && echo yes || echo no")
		die(err)
		p, err := android.NewPlan(info, b, res.Stdout == "yes\n", *replace)
		die(err)
		if cmd == "plan-android" {
			dump(p)
			return
		}
		die(android.Run(ctx, c, b, p, log))
	case "uninstall-android":
		die(android.Uninstall(ctx, c, *removeData))
	case "boot-android":
		die(android.BootAndroid(ctx, c))
	case "return-stock":
		die(android.ReturnToStock(ctx, c))
	default:
		die(fmt.Errorf("未知命令 %s", cmd))
	}
}

func dump(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}

func die(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "✗", err)
		os.Exit(1)
	}
}
