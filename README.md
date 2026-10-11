# reMarkable 空间 (SPACE)

reMarkable 上的启动台：首页小组件 + 应用注册表 + 绿色安装 + 应用后台托管。
从 [rmkit-cn](https://github.com/boangs/rmkit) 拆出来单独维护，不再和中文化 / 输入法 / AI 那摊混在一起。

## 下载安装

用「空间助手」装：电脑用 USB 线连上 reMarkable，助手会把空间连同它需要的底座一次装好。

| 助手 | 下载 |
|---|---|
| Mac | [space-assistant-mac.zip](https://github.com/boangs/remarkable-space/releases/latest/download/space-assistant-mac.zip) |
| Windows | [space-assistant-windows.exe](https://github.com/boangs/remarkable-space/releases/latest/download/space-assistant-windows.exe) |

校验值见 [Release](https://github.com/boangs/remarkable-space/releases/latest) 里的 `SHA256SUMS`。

助手装空间时还要下载约 140 MB 的安装包，默认走国内下载，可以在助手的「设置 → 下载来源」里改成 GitHub。
下不动会自动换另一处；下完按助手里写死的校验值核对，对不上不会装。

支持 reMarkable 2、Paper Pro、Paper Pro Move。Mac 首次打开如果提示「无法验证开发者」，
在「系统设置 → 隐私与安全性」里点「仍要打开」。

## 目录

| 这里 | 在 rmkit-cn 里的位置 | 说明 |
|---|---|---|
| `shell/` | `space/shell/` | 外壳 `Space.qml`：标签页、小组件挂载、应用创建销毁、给应用的 `space` API |
| `kit/` | `space/kit/` | 共享 QML 组件（`SRow` / `STile` / `SButton` …）与图标 |
| `apps/` | `space/apps/` | **内置**应用，随安装器部署。只放系统功能入口（设置类）——第三方应用走商店安装，源码在另一个仓库 |
| `server/` | `upload-server-go/internal/space/` | 后端 Go 包：注册表、安装、后台托管、最近在读、天气地图渲染 |
| `data/geo.json` | 随后端部署 | 天气地图的矢量底图（Natural Earth，公共领域，已简化） |

## 内置 vs 第三方

本仓库只放**内置应用** —— 随安装器一起部署到 `/home/root/rmkit-cn/space/apps/`，
多数是系统功能的入口（蓝牙、输入法、上传、个性化、切 Android 等）。

**后续新开发的应用一律走第三方**：独立源码、打包成 zip、通过应用商店分发，
绿色安装到 `~/.local/share/rmkit-cn/space/apps/`，不进这个仓库。
已经这么做的有「音乐」和「计划」。

这样分的好处是外壳和应用彻底解耦：外壳不认任何具体应用，应用也不必跟着外壳的
发布节奏走，各自迭代。

## 设计约定

- **外壳不认任何具体应用**。应用靠 `manifest.json` 自我描述，外壳只读注册表。
- **应用之间互不可见**，只能通过外壳给的 `space` API 说话。
- **墨水屏优先**：大面积浅色会被抖成灰，所以底一律纯白，颜色只用在小面积实心元素上。
- **免缓存令牌**：QML 引擎按 URL 缓存编译结果与位图。开发模式下外壳给 `file://` 加
  `?t=<rev>`（`rev` = 应用目录最新改动时间），文件没动就命中缓存，改了自动失效。
  应用内部拼资源路径要走 `space.bust(url)`，否则换了素材看不见。
- **相对 import 不带令牌**：`.js` 和子 QML 文件改了必须重启 xochitl，所以配色表、
  插画清单这类要频繁调的东西放在 `main.qml` 里或外部 JSON（XHR 读）。

## 应用清单字段

```jsonc
{
  "id": "weather",              // 小写字母数字, 唯一
  "name": "天气",
  "entry": "main.qml",          // 入口; 没有则走 launch
  "widget": "widget.qml",       // 可选, 首页小组件
  "widget_size": "hero|half",
  "widget_slot": "plan",        // 可选, 指定首页位置
  "arch": ["aarch64", "armv7"], // 空 = 都支持
  "service": { "exec": "...", "port": 9100, "keepalive": true }
}
```

## 许可

[Apache License 2.0](LICENSE)。

选它而不是 GPL 是有意的：应用由外壳的 QML 引擎加载，这种"插件"关系在 GPL 下是灰色
地带（FSF 认为共享地址空间的插件属于衍生作品）。Apache-2.0 明确允许闭源扩展，
第三方应用——包括收费的——不必跟着开源。附带的专利授权和商标条款也比 MIT 周全。
