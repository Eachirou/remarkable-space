# Integration base (base/)

The base layer integrates Space with the reMarkable system. It contains xovi, the qmldiff injection chain and
hashtabs, runtime injection libraries, added system-language resources, keyboard layouts, an input-method
backend, and post-update validation and recovery scripts. **This layer carries the device-boot risk.** Change it
only when a system update requires it.

> **Legacy implementation:** This base was migrated from [boangs/rmkit](https://github.com/boangs/rmkit)
> (`0f17826`) on 2026-10-20. The legacy device paths and installer described here are retained for compatibility;
> use Space Assistant and the root README for the current supported installation workflow.

Directory layout follows the original repository so its relative script paths remain valid. Device paths also
remain unchanged (`/home/root/rmkit-cn/...` and `~/.local/share/rmkit-cn/...`) to preserve existing user data.

| Directory | Contents |
|---|---|
| `intercept/` | `ime_hook.so` and runtime `qml-inject/` (thin hook plus Qt-linked library) |
| `qmd-src/`, `qmd/` | qmldiff patch source and keyboard layout resources |
| `installer/` | Legacy device-side installation, precheck, firmware-update, and OTA scripts |
| `systemd/` | xochitl drop-ins and OTA/input-method services |
| `ime-go/` | Go input-method backend (fallback engine for RM2) |
| `tools/` | `qmd-tool`, `create_rcc.go`, cross-compilation scripts, hashtab samples |
| `vendor/` | Prebuilt xovi components, extensions, and audio packages |
| `scripts/` | Release checks and translation-merging tools |
| `docs/` | Architecture, development, device matrix, and upgrade procedure |

`installer/install.sh` is a **legacy CLI installer retained for reference**. Do not run it as the current
installation workflow: it still attempts to install the retired upload-server, which is not part of this
repository. Use the desktop assistant in `assistant/`.

The procedures in `base/docs/` and older installer-specific notes also describe the predecessor workflow. Treat
them as historical references; they have not been validated as current Space Assistant instructions.

Build artifacts are written to `dist/` and are not tracked. The assistant bundle build consumes them:
`cd assistant && make bundle`. The librime source tree in `third_party/` (about 430 MB) is also local-only.

Components not migrated from the original rmkit repository include Android dual-boot (`android/`), the retired
desktop assistant (`desktop/`, replaced by `assistant/`), the old `upload-server-go/` backend (replaced by
`space-server`), `legacy/`, firmware QML exports (`qml-dump/`), and the original 43 MB IME corpus
(`ime-go/tools/build_dict/cpd.txt`). The compiled dictionary is in `ime-go/pinyin/dict/`.

Third-party code and licenses are listed in [NOTICE.md](NOTICE.md) and `third-party-licenses/`.
