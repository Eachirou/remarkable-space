# reMarkable 空间 (SPACE)

reMarkable 上的启动台：首页小组件 + 应用注册表 + 绿色安装 + 应用后台托管。
从 [rmkit-cn](https://github.com/boangs/rmkit) 拆出来单独维护，不再和中文化 / 输入法 / AI 那摊混在一起。

## 目录

| 这里 | 在 rmkit-cn 里的位置 | 说明 |
|---|---|---|
| `shell/` | `space/shell/` | 外壳 `Space.qml`：标签页、小组件挂载、应用创建销毁、给应用的 `space` API |
| `kit/` | `space/kit/` | 共享 QML 组件（`SRow` / `STile` / `SButton` …）与图标 |
| `apps/` | `space/apps/` | 内置应用。每个目录一个 `manifest.json` + `main.qml`（可选 `widget.qml`） |
| `server/` | `upload-server-go/internal/space/` | 后端 Go 包：注册表、安装、后台托管、最近在读、天气地图渲染 |
| `data/geo.json` | 随后端部署 | 天气地图的矢量底图（Natural Earth，公共领域，已简化） |

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
