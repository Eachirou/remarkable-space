// mkbundle creates a desktop-assistant bundle from repository artifacts
// (ZIP + manifest.json with a SHA-256 checksum for every file).
//
//	go run ./cmd/mkbundle -component space -repo ../base -space-repo .. -out space-bundle.zip
//	go run ./cmd/mkbundle -component android-rmppm -android-dir <directory> -out android-rmppm-bundle.zip
//
// The Space bundle combines the integration base (xovi, injection, translations,
// and input-method backend) from -repo with the shell, built-in apps, and server
// binaries from -space-repo. The archive stores them under space/ and
// space-dist/ prefixes. Android bundles collect fixed filenames from
// -android-dir (see internal/android.required).
package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/boangs/remarkable-space/assistant/internal/bundle"
)

// Collect these base directories recursively, plus the individual files below.
var rmkitDirs = []string{"dist", "vendor/xovi", "vendor/extensions", "vendor/audio", "qmd-src", "assets/chess", "systemd"}
var rmkitFiles = []string{
	"qmd/pinyin_interceptor.qmd", "qmd/zh_CN.rcc", "qmd/zh_CN/keyboard_layout.json",
	"scripts/version-switcher.sh",
	"installer/reenable.sh", "installer/fw-upgrade.sh", "installer/precheck.sh", "installer/qml-inject-lib.sh", "installer/ota-watch.sh",
	"intercept/qml-inject/adv_panel.qml", "intercept/qml-inject/glyph_ai_button.qml", "intercept/qml-inject/text_ai_button.qml",
	"intercept/qml-inject/pinyin_ime.qml", "intercept/qml-inject/icon_ai.svg",
}

// Only these files from dist/ belong in the bundle (exclude host qmd-tool binaries and backups).
func wantDist(name string) bool {
	base := filepath.Base(name)
	if strings.HasPrefix(base, "qmd-tool-") && base != "qmd-tool-aarch64" && base != "qmd-tool-armv7" {
		return false
	}
	return !strings.Contains(base, ".bak")
}

var androidFiles = []string{"fitImage.ahab-android", "modules.tar.gz", "rm-android-init-ss", "rm-touch-relay",
	"rm-epd-bridge", "rm-native-controls", "propset", "boot-android.sh", "init-wrapper.tmpl.sh",
	"android-kernel-revert.service.tmpl", "udhcpd-usb.conf", "udhcpd-usb1.conf", "rm-wifi-config"}

func main() {
	component := flag.String("component", "space", "bundle component: space or android-rmppm")
	repo := flag.String("repo", "../base", "integration-base directory (base/ in this repository)")
	spaceRepo := flag.String("space-repo", "..", "remarkable-space repository root")
	androidDir := flag.String("android-dir", "", "Android files directory")
	out := flag.String("out", "", "output ZIP path")
	version := flag.String("version", time.Now().Format("20060102-1504"), "bundle version")
	notes := flag.String("notes", "", "release notes")
	flag.Parse()
	if *out == "" {
		*out = *component + "-bundle-" + *version + ".zip"
	}
	files := map[string]string{} // Archive path → local path.
	switch bundle.Component(*component) {
	case bundle.ComponentSpace:
		for _, d := range rmkitDirs {
			root := filepath.Join(*repo, d)
			err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
				if err != nil || e.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(*repo, p)
				rel = filepath.ToSlash(rel)
				if strings.HasPrefix(rel, "dist/") && !wantDist(rel) {
					return nil
				}
				if strings.Contains(rel, ".bak") || strings.HasSuffix(rel, ".DS_Store") {
					return nil
				}
				// The retired upload-server unit is excluded; remarkable-space.service replaces it.
				if rel == "systemd/rmkit-cn-upload.service" {
					return nil
				}
				files[rel] = p
				return nil
			})
			if err != nil {
				die("collect %s: %v", d, err)
			}
		}
		for _, f := range rmkitFiles {
			p := filepath.Join(*repo, f)
			if _, err := os.Stat(p); err == nil {
				files[f] = p
			} else {
				fmt.Fprintf(os.Stderr, "  · skipping missing file %s\n", f)
			}
		}
		// Verify that the Rime binary contains /rime/schema and that the effective
		// IME server exposes /rime/input. Check the binary selected for each
		// architecture; an outdated pure-Go fallback is non-fatal but should be reported.
		_, hasPrebuilt := files["dist/rime-prebuilt.tar.gz"]
		_, hasRuntime := files["dist/rime-runtime-data.tar.gz"]
		hasRime := hasPrebuilt && hasRuntime
		for _, ar := range []struct{ rime, pure string }{{"ime-server-rime-aarch64", "ime-server"}, {"ime-server-rime-armv7", "ime-server-armv7"}} {
			rp, rok := files["dist/"+ar.rime]
			if rok && !fileContains(rp, "/rime/schema") {
				fmt.Fprintf(os.Stderr, "  · dist/%s has no /rime/schema (outdated build); excluded, this architecture will use the pure-Go engine\n", ar.rime)
				delete(files, "dist/"+ar.rime)
				rok = false
			}
			effective := "dist/" + ar.pure
			if rok && hasRime {
				effective = "dist/" + ar.rime
			}
			pp, pok := files[effective]
			if !pok {
				die("missing %s (no deployable input-method backend for this architecture)", effective)
			}
			if !fileContains(pp, "/rime/input") {
				die("%s is outdated (no /rime route); rebuild ime-go first", effective)
			}
			if p, ok := files["dist/"+ar.pure]; ok && !fileContains(p, "/rime/input") {
				fmt.Fprintf(os.Stderr, "  · dist/%s is an outdated pure-Go fallback (no /rime route); consider rebuilding with make -C ime-go\n", ar.pure)
			}
		}
		// Add the Space shell, shared components, built-in apps, server binaries,
		// static page, and vector assets. Device destinations are defined by the
		// deployment planner.
		for _, d := range []string{"shell", "kit", "apps", "hooks", "themes"} {
			root := filepath.Join(*spaceRepo, d)
			err := filepath.WalkDir(root, func(p string, e os.DirEntry, err error) error {
				if err != nil || e.IsDir() {
					return err
				}
				rel, _ := filepath.Rel(*spaceRepo, p)
				rel = filepath.ToSlash(rel)
				if strings.HasSuffix(rel, ".DS_Store") {
					return nil
				}
				files["space/"+rel] = p
				return nil
			})
			if err != nil {
				die("collect Space %s: %v", d, err)
			}
		}
		for zipName, local := range map[string]string{
			"space/static/qr.html": "static/qr.html",
			// The Space server unit is included in the base systemd payload and
			// persisted to both system layers during deployment stage 1.
			"systemd/remarkable-space.service": "systemd/remarkable-space.service",
			// Keep Space's xochitl drop-in separate: it belongs under
			// xochitl.service.d/, not the unit directory. The stage-1 loop only
			// handles .service, .path, and .timer files.
			"space-systemd/zz-space.conf": "systemd/zz-space.conf",
			// The backend drop-in likewise belongs in remarkable-space.service.d/.
			"space-systemd/10-store-api.conf": "systemd/10-store-api.conf",
			// rm-appload provides the framebuffer canvas to apps such as KOReader.
			// vendor/fetch-appload.sh downloads the official package; it is not tracked.
			"third_party/appload/aarch64/appload.so":         "third_party/appload/aarch64/appload.so",
			"third_party/appload/aarch64/qtfb-shim.so":       "third_party/appload/aarch64/shims/qtfb-shim.so",
			"third_party/appload/aarch64/qtfb-shim-32bit.so": "third_party/appload/aarch64/shims/qtfb-shim-32bit.so",
			"third_party/appload/armv7/appload.so":           "third_party/appload/armv7/appload.so",
			"third_party/appload/armv7/qtfb-shim.so":         "third_party/appload/armv7/shims/qtfb-shim.so",
			"third_party/appload/armv7/qtfb-shim-32bit.so":   "third_party/appload/armv7/shims/qtfb-shim-32bit.so",
		} {
			p := filepath.Join(*spaceRepo, local)
			if _, err := os.Stat(p); err != nil {
				if strings.HasPrefix(local, "third_party/appload/") {
					die("missing %s; run vendor/fetch-appload.sh to download the official package first", p)
				}
				die("missing Space file %s", p)
			}
			files[zipName] = p
		}
		for _, bin := range []string{"space-server-aarch64", "space-server-armv7"} {
			p := filepath.Join(*spaceRepo, "dist", bin)
			if _, err := os.Stat(p); err != nil {
				die("missing %s (run make all from the remarkable-space repository first)", p)
			}
			files["space-dist/"+bin] = p
		}
	case bundle.ComponentAndroid:
		if *androidDir == "" {
			die("android-rmppm requires -android-dir")
		}
		for _, f := range append(append([]string{}, androidFiles...), "android-system.tar.gz") {
			p := filepath.Join(*androidDir, f)
			if _, err := os.Stat(p); err != nil {
				if f == "android-system.tar.gz" {
					fmt.Fprintln(os.Stderr, "  · android-system.tar.gz not found; the bundle will omit the Android system (it must already be on the device)")
					continue
				}
				die("missing %s", p)
			}
			files["android/"+f] = p
		}
	default:
		die("unknown component %s", *component)
	}

	zf, err := os.Create(*out)
	if err != nil {
		die("%v", err)
	}
	zw := zip.NewWriter(zf)
	mf := bundle.Manifest{Component: bundle.Component(*component), Version: *version, Created: time.Now().UTC().Format(time.RFC3339), Notes: *notes, Files: map[string]bundle.Entry{}}
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	var total int64
	for _, n := range names {
		src, err := os.Open(files[n])
		if err != nil {
			die("%v", err)
		}
		method := zip.Deflate
		if strings.HasSuffix(n, ".gz") || strings.HasSuffix(n, ".zip") {
			method = zip.Store // Do not recompress files that are already compressed.
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: n, Method: method, Modified: time.Now()})
		if err != nil {
			die("%v", err)
		}
		h := sha256.New()
		size, err := io.Copy(io.MultiWriter(w, h), src)
		_ = src.Close()
		if err != nil {
			die("write %s: %v", n, err)
		}
		mf.Files[n] = bundle.Entry{Size: size, SHA256: hex.EncodeToString(h.Sum(nil))}
		total += size
	}
	var mb bytes.Buffer
	enc := json.NewEncoder(&mb)
	enc.SetIndent("", "  ")
	_ = enc.Encode(mf)
	w, _ := zw.Create(bundle.ManifestName)
	_, _ = w.Write(mb.Bytes())
	if err := zw.Close(); err != nil {
		die("%v", err)
	}
	_ = zf.Close()
	st, _ := os.Stat(*out)
	fmt.Printf("✓ %s: %d files, %.1f MB uncompressed, %.1f MB bundled, version %s\n", *out, len(names), float64(total)/1e6, float64(st.Size())/1e6, *version)
}

func fileContains(p, needle string) bool {
	b, err := os.ReadFile(p)
	return err == nil && bytes.Contains(b, []byte(needle))
}

func die(format string, a ...any) {
	fmt.Fprintf(os.Stderr, "✗ "+format+"\n", a...)
	os.Exit(1)
}
