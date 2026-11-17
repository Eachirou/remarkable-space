# System UI translations (`reMarkable_*.qm`)

`reMarkable_zh_CN.ts`, `reMarkable_zh_TW.ts`, `reMarkable_ja.ts`, and `reMarkable_ko.ts` are source files.
The corresponding compiled `.qm` files in `base/dist/` are generated artifacts; `dist/` is not tracked, so
regenerate the files before a release. The stock system already includes en, de, fr, and es.

```sh
for lang in zh_CN zh_TW ja ko; do
  /opt/homebrew/opt/qt6/bin/lrelease "base/i18n/reMarkable_${lang}.ts" -qm "base/dist/reMarkable_${lang}.qm"
done
```

Edit translations in the `.ts` files (standard Qt Linguist format, grouped by context). **Context behavior
observed on firmware 3.29 (2026-10-20):** QML `qsTr` contexts use the filename. The Settings page headings
General, Cloud, Security, Display, and Accessibility use the `General`, `Cloud`, `Security`, `Display`, and
`Accessibility` contexts (along with existing `Account`, `Battery`, `Storage`, `Help`, and `Wifi` contexts).
C++ `tr()` contexts use the class name, including its namespace. Keep contexts used by older firmware, such as
`SettingsModel`; add contexts rather than deleting them. Qt reloads translations and recalculates `qsTr` when the
system language changes, so a restart is not required.

On the device, translation files are stored in `/usr/share/remarkable/xochitl/translations/`. After a system
update, the legacy `fw-upgrade.sh` restores them from `/home/root/rmkit-cn/static/`. Qt reloads them when the
system language changes; restarting is not required.
