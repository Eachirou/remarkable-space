# qml-inject — runtime QML injection

> **Legacy base-layer documentation:** This describes the injector migrated from rmkit-cn. The base-layer
> installer is retained for reference and is not the supported Space installation workflow. Use Space Assistant
> and the repository root README for current installation instructions.

The injector was designed to reduce dependence on qmldiff patches. qmldiff patches compiled QML resources and
requires a firmware-specific hashtab; a stale hash can panic and crash xochitl. Runtime injection uses
`LD_PRELOAD` instrumentation to obtain the live `QQmlEngine::rootContext()` and add nodes to the scene tree
without reading hashtabs or modifying system binaries.

An alternative is to patch xochitl's `DT_NEEDED` entries and load a shim as a normal dependency. That is more
reliable but requires repatching the executable after each OTA. This implementation avoids modifying system
files.

## Migrated features

The following features were migrated from qmd to runtime injection and tested on RMPPM 3.28 / Qt 6.10.3:

| Feature | Injection point | Former qmd |
|---|---|---|
| Advanced sidebar entry and panel | `ColumnLayout` under `Sidebar`, above Guide | `advanced_panel.qmd` |
| Simplified/Traditional Chinese, Japanese, and Korean in the system language list | `SelectionComponent` (`objectName=SystemLanguageDialog`) | `language_zh_cn.qmd` |
| AI button in the handwriting-selection toolbar | `SelectionContextualMenu` under `SceneSelectionHandler` | `glyph_selection_ai.qmd` |
| AI button in the text-selection menu | `SelectionContextualMenu` under `TextSelectionMenu` | `ai_text_button.qmd` |
| Pinyin candidate UI (`pinyin_ime.qml`) | Main window `contentItem` | `pinyin_interceptor.qmd` |

No active feature remained on the qmldiff injection path at the time this document was written. The
`zh_CN.rcc` keyboard resource uses rcc replacement and is independent of qmd and hashtabs. The Pinyin UI's
timer replacement is described in the header of `pinyin_ime.qml` (a C++ 250 ms `imeTick` heartbeat plus
`onTextChanged`).

## Architecture: thin hook and Qt-linked library

```text
xochitl starts
  └─ LD_PRELOAD: active/qml_inject.so         thin hook, no Qt linkage
       ├─ intercept QQmlEngine::rootContext() and capture the engine pointer
       └─ worker thread: wait 2 s → dlopen qml_inject_impl.so → pw_inject(engine)
            └─ Qt-linked library: invokeMethod on the GUI thread
                 └─ QTimer scans once per second and injects idempotently
```

The split is required because a Qt-linked library loaded through `LD_PRELOAD` can initialize Qt before xochitl's
own static initializers and crash at startup. The thin hook does not link Qt. After Qt is running, it loads the
Qt-linked library with `dlopen`, similar to a Qt plugin.

The injector scans once per second because most target nodes do not exist at startup: `Sidebar` appears only in
the file-list view, and language dialogs and selection toolbars are created on demand and destroyed again. It
uses `objectName` to avoid duplicate injection. Since the scene tree can contain thousands of nodes, scans use
targeted `findByClass` lookups rather than traversing the entire tree.

## Build

Build on a remote Linux machine; macOS does not have the chiappa SDK:

```sh
scp -r intercept/qml-inject user@build-host:/tmp/
ssh user@build-host 'cd /tmp/qml-inject && make'
scp 'user@build-host:/tmp/qml-inject/qml_inject*-aarch64.so' dist/
```

Keep the artifact names expected by `install.sh`: `qml_inject-aarch64.so` and
`qml_inject_impl-aarch64.so`. Renaming them can cause the legacy installer to silently skip deployment.

The documented build produces aarch64 artifacts only. RM2 (armv7) does not have these artifacts; the legacy
`precheck.sh` selects the qmd path for that device.

## Deployment and safety notes

- The legacy `install.sh` deploys the library only when the artifact exists and includes it in `LD_PRELOAD`
  only on supported architectures. The implementation and QML resources are installed under
  `/home/root/rmkit-cn/bin/`; the library uses absolute `file:///home/root/rmkit-cn/bin/*.qml` URLs.
- The legacy `precheck.sh` treats runtime injection and qmldiff as separate paths. A missing hashtab or invalid
  qmd disables qmldiff while runtime injection remains enabled. A crash-loop fuse disables both paths.
  Run `bash installer/test-precheck.sh` after changing that logic.
- Avoid testing unverified injection changes through a persistent systemd restart. The historical safe test was
  a one-off xochitl launch without writing a drop-in, so a crash would not accumulate persistent restart counts:

  ```sh
  systemctl stop xochitl
  env ... LD_PRELOAD=...xovi.so:...ime_hook.so:...qml_inject.so \
    /usr/bin/xochitl --system >/tmp/x.log 2>&1 &
  # Inspect the result, then stop the temporary process and start the service again.
  ```

- Do not combine a file change with an immediate xochitl restart. Several historical upgrade incidents followed
  that pattern.

## Debugging

- On the device, `touch /tmp/rmkit-dump` asks the next scan to write the scene tree to
  `/tmp/rmkit-dump.txt`, then removes the marker.
- Keep stderr unbuffered. Redirected output may be block-buffered and lost on a segmentation fault; the
  Qt-linked library uses `setvbuf(stderr, 0, _IONBF, 0)` in its constructor.

## Files

| File | Purpose |
|---|---|
| `thin_hook.cpp` | Thin hook without Qt linkage; exports only `rootContext` |
| `qml_inject_impl.cpp` | Qt-linked library, injection routines, and one-second scan; exports `pw_inject` |
| `adv_panel.qml` | Advanced panel QML extracted from the former qmd patch |
| `glyph_ai_button.qml`, `text_ai_button.qml` | AI buttons for handwriting and text selection |
| `icon_ai.svg` | Advanced-panel icon |
| `qml_inject.cpp` | Deprecated monolithic version; do not build or preload |
| `noqt_probe.cpp` | Historical probe; `make probe` builds it separately |
