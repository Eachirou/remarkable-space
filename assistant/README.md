# 空间助手

Mac / Windows 上的桌面程序，USB 直连 reMarkable，一次把「空间」连同它依赖的底座装好。

纯 Go 的 SSH 直连，没有服务器，不上传任何东西。每条在设备上执行的命令、每个写入的文件
都记在本地审计日志里（界面上有「打开日志目录」）。

## 它装什么

**底座（只在系统升级逼着改时才动）** —— 源码在本仓库 `base/`（原 rmkit-cn）：xovi、qmldiff 注入链与 hashtab、
运行时注入 `.so`、中文汉化 `.qm`、输入法后端。**这部分才是真会把设备弄砖的**：hashtab 和
固件对不上就是 qmldiff panic → xochitl crash → A/B 回滚。所以设备端沿用 install.sh 那套
六阶段防砖脚本，原样执行，一行没改。

**空间** —— 外壳、内置应用、后端 `space-server`（顶替已退役的 upload-server，同样监听
8080）。空间走 `Qt.createComponent` 异步加载，QML 写坏只是面板打不开，不会崩 xochitl。

装完侧栏那一项会变成「空间」——靠 `/home/root/rmkit-cn/space/.primary` 这个标记文件切换，
注入库同时检查它和 `Space.qml` 是否存在，少一个都还是显示「高级」。

顺带还能装 Paper Pro Move 的单槽 Android（和原来一样）。

## 用

```sh
# 1. 先在仓库根出后端二进制
cd .. && make all

# 2. 打安装包 (底座产物取自 ../base/dist, 见 base/README.md)
cd assistant && make bundle

# 3. 构建界面
make build-mac          # 或 make build-win
open build/bin/space-assistant.app
```

## 命令行

同一套内核，没有 GUI 的环境（CI、远程排障）用这个：

```sh
make cli
dist/assistant-cli-darwin-arm64 -host 10.11.99.1 -password xxx probe
dist/assistant-cli-darwin-arm64 ... -bundle dist/space-bundle.zip plan-rmkit   # 预览
dist/assistant-cli-darwin-arm64 ... -bundle dist/space-bundle.zip run-rmkit    # 安装

# 不连设备, 伪造探测结果算落点 —— 改完安装逻辑自查用
dist/assistant-cli-darwin-arm64 -bundle dist/space-bundle.zip -arch armv7l -model rm2 plan-offline
```

## 几个不能动的地方

- **设备数据路径沿用 `~/.local/share/rmkit-cn/`**。字体、锁屏图、已装应用、应用数据都在那，
  换名字就等于让用户数据凭空消失。
- **空间外壳仍落在 `/home/root/rmkit-cn/space/`**。这个路径硬编码在注入库
  （`qml_inject_impl.cpp`）里，改了要重编两个架构的 `.so`。
- **`/etc` 是 tmpfs overlay**，systemd unit 和 wants symlink 必须 upper + ext4 lower 双写，
  只写 `/etc` 重启就没了。`systemctl enable` 建的 symlink 同理，所以脚本里是手工建的。
- **传输用 `--no-same-owner`**。不加的话 tar 会把打包机的 uid（502）还原到设备上，
  `/home/root` 属主一变，sshd 和 xochitl 一起卡死。也**绝不能** `chown -R /home/root`：
  单槽 Android 的系统和 `/data` 也在底下。
- **传完先自检后端二进制能不能执行**再动服务。架构推错只在运行时报 Exec format error，
  等六阶段跑完才发现就晚了——那时旧服务已经 mask，上传/字体/蓝牙/AI 会一起失效。
