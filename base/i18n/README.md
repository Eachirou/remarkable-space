# 系统界面汉化 (reMarkable_zh_CN.qm)

`reMarkable_zh_CN.ts` 是源, `base/dist/reMarkable_zh_CN.qm` 是产物 (dist 不进仓库, 发版前重新生成):

```sh
/opt/homebrew/opt/qt6/bin/lrelease base/i18n/reMarkable_zh_CN.ts -qm base/dist/reMarkable_zh_CN.qm
```

改译文直接改 .ts (标准 Qt Linguist 格式, 按 context 分组)。固件换了上下文时看二进制里
`translationContext` 后面跟的名字: 3.29 起设置菜单 (General / Wi-Fi / Cloud …) 用 `SettingsPage`,
之前是 `SettingsModel`; 两组都要留着, 老固件还在用。

设备上这份文件在 `/usr/share/remarkable/xochitl/translations/`, 系统升级后由
`fw-upgrade.sh` 从 `/home/root/rmkit-cn/static/` 放回去; 切换系统语言时 Qt 会重新加载, 不用重启。
