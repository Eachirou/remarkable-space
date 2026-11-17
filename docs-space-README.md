# Space — reMarkable launcher

Space is a personal digital home on reMarkable: a home screen (time, weather, today's plan, and now playing), a
set of apps, and an app catalog. It replaces the rmkit-cn "Advanced" panel as the unified entry point for these
features.

Design goal: **keep the shell small and stable; keep apps independent; install and remove apps without restarting
the interface.**

> **Documentation status:** This document originated while Space was being migrated from rmkit-cn. Some paths,
> server names, and migration notes below are historical. The current implementation is in `shell/`, `apps/`,
> `internal/`, and `cmd/space-server/`; use the root [README](README.md) and source code when details differ.

```text
Sidebar "Space" ──▶ Space.qml ──▶ Home / Apps / Discover / Settings
                         │
                         ├── GET /space/apps ──▶ app registry
                         │                          ├── built-in apps (installer-managed)
                         │                          └── user apps (installed under user data)
                         │
                         └── launch app ──▶ Qt.createComponent(main.qml) ──▶ app root Item (property var space)
                                                └── declared service ──▶ POST /space/apps/{id}/service/start
```

## Layers and boundaries

| Layer | Location (historical or current path) | Responsibility | Does not |
|---|---|---|---|
| Injector | `base/intercept/qml-inject/qml_inject_impl.cpp` | Adds a sidebar entry and chooses the Space shell or legacy panel | Know about individual apps |
| Shell | `shell/Space.qml` | Home and tabs, app creation/destruction, app API | Contain app-specific features |
| Component library | `kit/` | Tiles, buttons, selection controls, list rows, clocks, line icons | Own app state |
| Backend | `internal/space/` | Scan manifests, install/remove apps, manage services, serve store data | Know app-specific behavior |
| Apps | `apps/<id>/` (built-in) or user app directory | Manifest, QML, icon, optional service | Reach into shell internals |

The shell and apps communicate through the app registry (backend to shell) and the `space` API object (shell to
app). An app communicates with its own service through `space.serviceUrl`; the backend starts and stops that
service.

## App package

```text
<id>/
  manifest.json     required
  main.qml          UI entry point (entry)
  icon.svg          icon
  widget.qml        optional home-screen widget
  bin/<service>     optional service executable
  assets/…          optional app assets
```

The manifest definition in `internal/space/manifest.go` is authoritative:

```jsonc
{
  "id": "music",                 // lowercase letters, digits, - or _, 1–32 characters
  "name": "Music",
  "version": "1.0.0",
  "description": "A short description",
  "icon": "icon.svg",            // relative/absolute path or qrc:/ or file: URL
  "entry": "main.qml",           // at least one of entry or launch is required
  "widget": "widget.qml",        // optional home-screen widget
  "widget_size": "half",         // half or hero
  "category": "tool",            // system/settings/reader/tool/game/other
  "order": 10,                   // ordering within a category
  "arch": ["aarch64"],           // supported architectures; empty means all
  "min_shell": 1,                // minimum shell contract version
  "service": {                   // optional background service
    "exec": "bin/music-server", "args": [], "port": 9100, "health": "/health",
    "keepalive": true,           // keep running after the app UI closes
    "autostart": false           // start automatically with the server
  },
  "launch": {                    // optional external app without a QML UI
    "type": "post",              // post or appload
    "path": "/apps/android/launch", "id": "external::koreader",
    "confirm": "Reboot into Android?"
  }
}
```

The service receives `SPACE_APP_ID`, `SPACE_APP_DIR`, `SPACE_DATA_DIR`, `SPACE_PORT`, and `SPACE_BASE_URL`.
Standard output and error are written to `SPACE_DATA_DIR/service.log`. Keep services in the managed process
group; do not fork them into a separate process group.

## App UI contract (shell version 1)

The root is an `Item` with `property var space`, injected by the shell. Use `anchors.fill: parent` to fill the
content area.

```qml
import QtQuick
import "file:///home/root/rmkit-cn/space/kit" as Kit // Optional shared components.

Item {
    id: root
    property var space
    anchors.fill: parent
    Component.onCompleted: space.get("/bt/status", function(status, result) { /* ... */ })
}
```

The `space` object exposes:

| Member | Description |
|---|---|
| `baseUrl` | Backend address, usually `http://127.0.0.1:8080` |
| `serviceUrl` | This app's service address when the manifest declares a port |
| `appId`, `appDir`, `dataDir`, `kitDir` | App identifiers and directories |
| `unit`, `fontScale`, `largeScreen`, `screenWidth`, `screenHeight` | Display scaling values |
| `title` | Set the shell's main title |
| `chrome` | Set to `false` to hide the shell navigation and title |
| `onBack` | Assign a function; return `true` when the app handles the back action |
| `exit()`, `closeSpace()`, `toast(msg)`, `openApp(id)` | Exit, close Space, show a message, or open another app |
| `get/post(path, cb)`, `svcGet/svcPost(path, cb)`, `request(method, url, body, cb)` | HTTP helpers; callback receives `(status, json)` |
| `localNow()` | Current time in the device time zone; do not format `new Date()` directly |
| `dataGet(id, key, cb)`, `dataPut(id, key, obj, cb)` | Persist JSON app data at `dataDir/<key>.json` (up to 1 MB) |

A widget root is an `Item` with `property var space` and `property var app` (the app's registry entry, including
`serviceUrl`, `iconUrl`, and `dataDir`). For widgets, `space.serviceUrl` does not refer to the widget's app; use
`app.serviceUrl`. The shell places widgets in fixed-height cards; open the app with `space.openApp(app.id)`.

### Responsive sizing

The Paper Pro (1620×2160) and Paper Pro Move (954×1696) have similar pixel density but very different widths;
orientation can also change. The shell's sizing approach can be reused by apps:

- `unit = min(width, height) / 954`; multiply margins, card heights, and spacing by it.
- `fontScale = 1 + (unit - 1) × 0.35`; scale text more gently than the rest of the UI.
- Calculate columns from content width: `floor(contentWidth / (minimum tile width × fontScale))`.

## Localization

The shell follows the tablet's system language and exposes the resolved value as `space.lang`. The historical
implementation tried `languageSettings.languageCode`, then a backend language value, then English. The current
supported languages are documented in the root README and shell dictionaries.

An app can include flat JSON dictionaries:

```text
app/
  i18n/
    en.json       required fallback
    de.json
    fr.json
    es.json
    zh-CN.json
    zh-TW.json
    ja.json
    ko.json
```

Keep `_app_name`, `_app_description`, and `_launch_confirm` for localized app metadata:

```json
{ "add_task": "Add task", "done": "Done", "empty_hint": "No tasks yet" }
```

Use `space.tr("empty_hint")` in an app. Widgets should use `space.trApp(app.id, "key")`, because `space.tr`
looks up the currently open app's dictionary. When a translation is missing, the shell falls back to `en.json`,
then to the key itself so missing strings are visible during development.

Do not use `qsTr` for app strings: injected QML does not have the relevant Qt translation table. Keep UI copy in
external JSON so translators can add languages without rebuilding app binaries.

## Backend API

These are the API routes described by the original design. Check the current server routes in
`internal/server/` before relying on any path:

| Method | Path | Purpose |
|---|---|---|
| GET | `/space/apps` | App registry |
| POST | `/space/apps` | Install a ZIP (`file` multipart field or ZIP body) |
| POST | `/space/apps/install-url` | Install from `{"url", "sha256"}` |
| DELETE | `/space/apps/{id}` | Remove a user app; retain its data |
| GET/POST | `/space/apps/{id}/service`, `/service/start`, `/service/stop` | Service status, start, and stop |
| GET/PUT | `/space/apps/{id}/data/{key}` | Read/write app JSON data |
| GET | `/space/store` | App catalog; cached for 10 minutes, `?refresh=1` forces refresh |

The installer is expected to reject path traversal and symlinks, validate before replacing an installed app, and
stop a same-name service before replacing it.

## App catalog

The original design used a static JSON index, generated by app project build scripts:

```json
{"apps": [{"id": "music", "name": "Music", "version": "1.0.0", "description": "...",
           "category": "tool", "arch": ["aarch64"],
           "zip": "https://example.invalid/music-1.0.0.zip", "sha256": "..."}]}
```

The Discover page compares app IDs and versions to show whether an app can be installed, updated, or is already
installed. See [docs-store-key.md](docs-store-key.md) for catalog key configuration.

## Migration and local development notes

The migration notes below describe an earlier project state and are retained for historical context:

1. The shell, backend, and component library were established; legacy panel entries were moved to built-in apps.
2. The `.primary` marker was used to switch the sidebar entry from the legacy panel to Space.
3. Music was used to validate the service/keepalive/widget flow; Weather was the first built-in hero widget.
4. A `.dev` marker bypassed QML component caching during development.

In development mode, QML URL cache-busting may allow changed QML to load without restarting xochitl. JavaScript
imports and image resources can remain cached by Qt and may require an interface restart. The shell displays
`Component.errorString()` when an app fails to load.

The old note `cd upload-server-go && go test ./internal/space/` is obsolete. Use the root README and current
Go module layout to select tests for the active implementation.
