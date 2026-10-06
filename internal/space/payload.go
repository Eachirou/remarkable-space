package space

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

// 有些应用的"本体"是别人发布的 —— 比如微信读书那个 41 MB 的官方包。我们不能也不该
// 重新分发它 (版权, 以及跟着人家升级的负担), 所以商店里只放一份几百字节的描述加上
// 我们自己写的安装脚本, 让**设备直接从官方地址下载**。
//
// 边界说清楚: 脚本以 root 跑, 所以它必须是我们自己写的、随包一起审计的。从网上抓一个
// 脚本下来执行是另一回事, 这里不做。下载物用 sha256 锁死 —— 对方换了包当场就知道,
// 而不是装上之后才发现。

// 下载 41 MB 不能落在 /tmp: 那是 tmpfs, 占的是内存。设备总共 2 GB, 解压时还要再占
// 一份, 够把 xochitl 挤到 low on memory。放 /home 下的隐藏目录, 那是 ext4。
const payloadWorkDir = ".install-work"

// runInstall 执行应用的 install 段: 下载 → 校验 → 跑脚本。
// appDir 是应用自己的目录 (脚本和 manifest 都在里头)。
func (r *Registry) runInstall(appDir string, m *Manifest, hdr http.Header) error {
	if m.Install == nil || (len(m.Install.Fetch) == 0 && m.Install.Script == "") {
		return nil
	}
	work := filepath.Join(r.UserDir, payloadWorkDir, m.ID)
	_ = os.RemoveAll(work)
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	// 装完就清: 这些是几十 MB 的中间物, 留着只会占地方
	defer os.RemoveAll(filepath.Join(r.UserDir, payloadWorkDir))

	for i, f := range m.Install.Fetch {
		name := f.To
		if name == "" {
			name = path.Base(strings.Split(f.URL, "?")[0])
		}
		setProgress(func(p *Progress) {
			p.Phase = "fetching"
			p.Got, p.Total = 0, 0
			p.Name = fmt.Sprintf("%s (本体 %d/%d)", m.Name, i+1, len(m.Install.Fetch))
		})
		if err := fetchVerified(f.URL, filepath.Join(work, name), f.SHA256, hdr); err != nil {
			return fmt.Errorf("取 %s 失败: %w", name, err)
		}
	}

	if m.Install.Script == "" {
		return nil
	}
	setProgress(func(p *Progress) { p.Phase = "installing"; p.Name = m.Name })
	return runAppScript(appDir, m.Install.Script, work, m.ID)
}

// runUninstall 在删应用目录之前跑卸载脚本, 把本体一并收掉。
// 脚本失败不挡卸载 —— 用户点了删除就该删掉, 顶多是本体残留在设备上,
// 总比"删不掉"强 (那会让人反复点, 以为是界面坏了)。
func (r *Registry) runUninstall(appDir string, m *Manifest) {
	if m.Install == nil || m.Install.Uninstall == "" {
		return
	}
	work := filepath.Join(r.UserDir, payloadWorkDir, m.ID+"-uninst")
	_ = os.MkdirAll(work, 0o755)
	defer os.RemoveAll(work)
	if err := runAppScript(appDir, m.Install.Uninstall, work, m.ID); err != nil {
		logStore("%s 的卸载脚本出错 (应用仍会删除): %v", m.ID, err)
	}
}

// runAppScript 跑应用包里自带的脚本。
// 工作目录是放下载物的临时目录, SPACE_APP_DIR 指向应用自己的目录。
func runAppScript(appDir, rel, work, id string) error {
	script := filepath.Join(appDir, filepath.Clean(rel))
	// 再挡一道: manifest 校验过是相对路径了, 但那份 manifest 是从包里读的,
	// 这里按实际落地的路径核一遍, 别让它跑出应用目录。
	if !strings.HasPrefix(script, filepath.Clean(appDir)+string(os.PathSeparator)) {
		return fmt.Errorf("脚本 %q 不在应用目录内", rel)
	}
	if _, err := os.Stat(script); err != nil {
		return fmt.Errorf("找不到脚本 %s: %w", rel, err)
	}
	// 装一个几十 MB 的本体可能要解压、拷贝、改权限, 给足时间但别无限等
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", script)
	cmd.Dir = work
	cmd.Env = append(os.Environ(),
		"SPACE_APP_ID="+id,
		"SPACE_APP_DIR="+appDir,
		"HOME=/home/root",
	)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		logStore("%s 安装脚本输出:\n%s", id, strings.TrimRight(string(out), "\n"))
	}
	if err != nil {
		// 把脚本最后几行带给用户 —— 只说"退出码 1"他没法judge是网络还是空间不够
		return fmt.Errorf("%w%s", err, tailLines(string(out), 3))
	}
	return nil
}

// fetchVerified 下载并核对 sha256。校验不过就删掉, 绝不留下一个"可能是对的"文件。
func fetchVerified(url, dst, want string, hdr http.Header) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// 商店自己的凭证只发给商店。本体多半在别人的 CDN 上, 别把票带过去。
	if isOurStore(url, hdr) {
		for k, vs := range hdr {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}
	}
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	f, err := os.Create(dst + ".part")
	if err != nil {
		return err
	}
	h := sha256.New()
	cr := &countingReader{r: io.TeeReader(resp.Body, h)}
	setProgress(func(p *Progress) { p.Total = resp.ContentLength })
	_, err = io.Copy(f, cr)
	cerr := f.Close()
	if err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(dst + ".part")
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, want) {
		_ = os.Remove(dst + ".part")
		return fmt.Errorf("校验不符 (期望 %s…, 实际 %s…) —— 对方可能换了包, 别装", want[:12], got[:12])
	}
	return os.Rename(dst+".part", dst)
}

// isOurStore 判断这个地址是不是我们自己的商店。只有它才配带上票。
func isOurStore(url string, hdr http.Header) bool {
	if len(hdr) == 0 {
		return false
	}
	for _, p := range storeHosts {
		if strings.HasPrefix(url, p) {
			return true
		}
	}
	return false
}

// storeHosts 由后端在启动时按 StoreAPI 填。为空时任何外部下载都不带凭证 —— 这是安全的默认。
var storeHosts []string

func tailLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		return ""
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return ": " + strings.Join(lines, " / ")
}
