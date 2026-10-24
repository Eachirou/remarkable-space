# 空间主题

主题只管外观: 颜色、圆角、描边、图标、默认封面, 以及首页三块区域的替换 QML。功能改动不走主题 (走空间自更新)。
后台见 `internal/space/theme.go`, 外壳见 `shell/Space.qml` 的 `thDefault` / `loadTheme()`。

- 空间自带的主题放这里 (`themes/<id>/`), 随空间安装 / 自更新。
- 商店主题是一个应用: 清单写 `"theme": "theme"` (包里主题目录), 没有 entry, 不进应用格子;
  装好后链进 `~/.local/share/rmkit-cn/space/themes/<应用 id>`。在「个性化 → 主题」里切换。

## theme.json

```json
{
  "id": "contrast", "name": "高对比黑白", "name_en": "High contrast", "version": "1.0.0",
  "min_shell": 3,
  "colors": { "line": "#000000", ... },
  "radius": 4, "border": 2,
  "hero": "hero.jpg",
  "preview": "preview.png",
  "icons": { "gear": "icons/gear.svg" },
  "slots": { "hero": "Hero.qml", "widgets": "Widgets.qml", "apps": "Apps.qml" }
}
```

- `colors` 的键 (没写的用默认值): bg 页面底色, card 卡片/磁贴底色, fill 浅灰底, press 按下,
  line 卡片描边, lineSoft 分隔线/进度槽, lineFaint 更淡的分隔线, text 正文/强调, text2 次要文字,
  textBody 商店页正文, text3 说明/右侧细节, textNote 卡片里的说明, primary 主按钮/提示条底, primaryText 其上文字。
  墨水屏不做深色主题: 没写颜色的文字默认黑色, 底色只能是浅色。
- `hero` 主题自带的封面; 用户自己设了封面时用户的优先。
- `icons` 替换 kit 图标 (键是 kit/icons 里的文件名去掉 .svg)。
- `slots` 替换首页区域: hero 顶部大卡, widgets 今日计划/正在进行两张卡, apps「我的应用」标题和格子。
  替换 QML 的根 Item 要有 `property var space` (外壳本身, 可用 space.th / space.homeApps() / space.open(app) /
  space.dateLine() / space.localNow() / space.st(key) 等), 并给出 implicitHeight; 宽度由外壳定。
- 文件改了 (theme.json 的 mtime 变了) 外壳会换新地址重新加载, 不用重启 xochitl。
