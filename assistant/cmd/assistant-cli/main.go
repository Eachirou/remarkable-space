// assistant-cli is the command-line entry point for the desktop assistant's
// deployment engine. It runs the same probe, plan, and deployment flow as the
// GUI in environments without a graphical interface (such as CI or remote diagnostics).
//
//	assistant-cli -host 10.11.99.1 -password xxx probe
//	assistant-cli ... -bundle space-bundle.zip plan-rmkit | run-rmkit | uninstall-rmkit
//	assistant-cli ... -bundle android-rmppm-bundle.zip plan-android | run-android | uninstall-android
//	assistant-cli ... boot-android | return-stock
//
// plan-offline calculates deployment destinations without connecting to a device.
// The -arch, -fw, and -model flags provide simulated probe results for CI and
// validating changes to the deployment logic:
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
	"github.com/boangs/remarkable-space/assistant/internal/i18n"
	"github.com/boangs/remarkable-space/assistant/internal/probe"
	"github.com/boangs/remarkable-space/assistant/internal/sshx"
)

func main() {
	i18n.Set("en")
	host := flag.String("host", "10.11.99.1", "device address")
	password := flag.String("password", os.Getenv("RM_PASSWORD"), "SSH password (or RM_PASSWORD environment variable)")
	bpath := flag.String("bundle", "", "installation bundle ZIP")
	replace := flag.Bool("replace-system", false, "Android: replace an existing Android system")
	removeData := flag.Bool("remove-data", false, "uninstall-android: also delete data in /home")
	offArch := flag.String("arch", "aarch64", "plan-offline: architecture (aarch64 or armv7l)")
	offFW := flag.String("fw", "20260806095513", "plan-offline: firmware version")
	offModel := flag.String("model", "rmppm", "plan-offline: device model (rm2, rmpp, or rmppm)")
	offXovi := flag.Bool("have-xovi", true, "plan-offline: xovi is already installed")
	flag.Parse()
	cmd := flag.Arg(0)
	if cmd == "" {
		flag.Usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log := func(s string) { fmt.Println(s) }

	// plan-offline only calculates a plan and does not open an SSH connection.
	if cmd == "plan-offline" {
		if *bpath == "" {
			die(fmt.Errorf("plan-offline requires -bundle"))
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
		fmt.Printf("Bundle: %s version %s, %d files\n", b.Manifest.Component, b.Manifest.Version, len(b.Manifest.Files))
	}
	need := func() {
		if b == nil {
			die(fmt.Errorf("%s requires -bundle", cmd))
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
		die(fmt.Errorf("unknown command %s", cmd))
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
