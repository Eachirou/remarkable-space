// 笔记里的「贴纸」面板。注入库 (qml_inject_impl.cpp doInjectDocAddons) 在打开笔记时挂到 DocumentView 上,
// 文件 mtime 变了会自动重建 (改面板不用重启 xochitl)。
//
// 用法: 右上角往左下斜滑 → 面板 → 点一个贴纸 → 用笔在页面上点一下 → 贴纸按原生笔迹描在那里并处于
// 选中状态 (可拖可缩放)。描是后台用虚拟笔逐笔写的 (/space/sticker/stroke), 每笔之前把笔的颜色/粗细
// 改成贴纸那一笔的, 描完恢复用户的笔; 真笔靠近屏幕时后台会停下来等。
//
// 运行时注入的组件里 Timer 不触发, 所有"等一会"都借后台 (events 长轮询 / stroke wait)。
import QtQuick

Item {
    id: root
    property var docView: null          // 注入库给的 DocumentView
    property var clip: null             // 系统剪贴板单例 (暂未用)
    anchors.fill: parent

    readonly property string base: "http://127.0.0.1:8080"
    readonly property real u: Math.max(0.6, Math.min(width, height) / 954)   // 同空间外壳的缩放基准
    property bool open: false
    property bool busy: false
    property string hint: ""            // 顶部提示条 (空 = 不显示)
    property var packs: []
    property string pending: ""         // 选中的贴纸 (rel), 等笔点
    property real stickerSize: 200      // 贴纸落到屏幕上的边长 (像素)
    property bool polling: false

    function log(s) { console.warn("[sticker] " + s) }
    function xhr(method, path, body, done) {
        var x = new XMLHttpRequest()
        x.onreadystatechange = function () {
            if (x.readyState !== XMLHttpRequest.DONE) return
            var r = null
            try { r = JSON.parse(x.responseText) } catch (e) {}
            done(x.status, r)
        }
        x.open(method, base + path)
        if (body !== null) x.setRequestHeader("Content-Type", "application/json")
        x.send(body === null ? undefined : JSON.stringify(body))
    }
    function post(path, body, done) { xhr("POST", path, body, done) }

    // ---- 事件长轮询 ----
    function poll() {
        if (polling) return
        polling = true
        xhr("GET", "/space/sticker/events?wait=25", null, function (st, r) {
            polling = false
            if (st !== 200 || !r) { post("/space/sticker/stroke", { op: "wait" }, function () { root.poll() }); return }
            if (r.type === "gesture" && !busy && !pending) root.show()
            else if (r.type === "tap" && pending) root.place(r.x, r.y)
            root.poll()
        })
    }
    Component.onCompleted: poll()

    // ---- 面板 ----
    function show() {
        if (packs.length === 0) {
            xhr("GET", "/space/sticker/list", null, function (st, r) {
                if (st === 200 && r && r.packs) { root.packs = r.packs; root.open = true; root.hint = "" }
                else root.hint = "贴纸读不到 / Stickers unavailable"
            })
        } else { open = true; hint = "" }
    }
    function choose(rel) {
        open = false
        pending = rel
        hint = "用笔在页面上点一下，贴纸落在那里 / Tap with the pen where the sticker should go"
        post("/space/sticker/arm", { on: true }, function () {})
    }
    function cancel() {
        open = false
        if (pending) { pending = ""; post("/space/sticker/arm", { on: false }, function () {}) }
        hint = ""
    }

    // ---- 描 ----
    function visibleDSV() {
        var out = []; walk(docView, 0, out); return out.length ? out[0] : null
    }
    function walk(n, depth, out) {
        if (!n || depth > 40) return
        if (String(n).indexOf("DeviceSceneView") === 0 && n.visible) out.push(n)
        var ch = n.children
        if (ch) for (var i = 0; i < ch.length; i++) walk(ch[i], depth + 1, out)
    }
    function penHandler() {
        var dsv = visibleDSV()
        if (dsv && dsv.strokeHandler) return dsv.strokeHandler
        var out = []; findAll(docView, 0, "PenInputSurface", out)
        for (var i = 0; i < out.length; i++) if (out[i].visible && out[i].handler) return out[i].handler
        return null
    }
    function findAll(n, depth, key, out) {
        if (!n || depth > 40) return
        if (String(n).indexOf(key) === 0) out.push(n)
        var ch = n.children
        if (ch) for (var i = 0; i < ch.length; i++) findAll(ch[i], depth + 1, key, out)
    }
    function xoColor(c) { return c === 4 ? 10 : c }   // rmscene GREEN(4)=荧光绿, 笔要用 Green(10)

    function place(x, y) {
        var rel = pending; pending = ""
        if (busy) { log("place: busy"); return }
        var dsv = visibleDSV(); var tb = dsv ? dsv.toolbar : null
        var pen = tb ? tb.primaryPen : null       // PenSetting {tool, color, colorCode, thickness}
        if (!tb || !pen) { log("place: no page/toolbar"); hint = "这里不能贴 / Not a notebook page"; return }
        busy = true
        hint = "正在贴，请把笔移开 / Placing, keep the pen away"
        // 只改工具栏的「主笔设置」, 让它顺着系统自己的绑定传到笔迹处理器 (dsv.strokeHandler)。
        // 2026-10-21 教训: 直接给处理器的 lineTool/lineColor 赋值会切断绑定, 之后用户在工具栏选笔也不再生效,
        // 重开笔记都不恢复 (处理器对象常驻), 只有重启 xochitl 才行。
        var saved = { t: pen.tool, c: pen.color, cc: pen.colorCode, w: pen.thickness }
        try { tb.selectPen("primary") } catch (e) {}
        var at = { op: "begin", sticker: rel, cx: x, cy: y, size: stickerSize }
        post("/space/sticker/stroke", at, function (st, r) {
            if (st !== 200 || !r) { root.busy = false; root.hint = "贴失败 / Failed"; return }
            var meta = r.strokes, i = 0, retries = 0
            function restore() { pen.tool = saved.t; pen.color = saved.c; pen.colorCode = saved.cc; pen.thickness = saved.w }
            function next() {
                if (i >= meta.length) { finish(); return }
                var m = meta[i]
                pen.tool = 17
                if (m.argb !== undefined) { pen.color = 9; pen.colorCode = m.argb }   // 自定义颜色: 9 + ARGB
                else pen.color = root.xoColor(m.color)
                pen.thickness = Math.max(1, m.width / 1.8)
                root.post("/space/sticker/stroke", { op: "draw", index: i }, function (st2) {
                    if (st2 === 200) { i++; retries = 0; next(); return }
                    if (st2 === 409 && retries < 3) { retries++; root.hint = "请把笔移开 / Move the pen away"; next(); return }
                    root.post("/space/sticker/stroke", { op: "end" }, function () { restore(); root.busy = false; root.hint = "贴了一半，笔一直没移开 / Interrupted by the pen" })
                })
            }
            function finish() {
                root.post("/space/sticker/stroke", { op: "end" }, function () {
                    restore()
                    try { tb.selectSelection() } catch (e) {}
                    root.post("/space/sticker/stroke", { op: "wait" }, function () {
                        root.post("/space/sticker/draw", { cx: x, cy: y, size: root.stickerSize, lasso: true }, function () {
                            root.busy = false; root.hint = ""
                        })
                    })
                })
            }
            next()
        })
    }

    // 提示条
    Rectangle {
        visible: root.hint !== ""
        anchors.horizontalCenter: parent.horizontalCenter; y: 130 * u
        width: Math.min(parent.width - 80 * u, hintText.implicitWidth + 48 * u); height: 72 * u; radius: 12 * u
        color: "#F2EFE8"; border.width: 2; border.color: "#000"; z: 10
        Text { id: hintText; anchors.centerIn: parent; text: root.hint; font.pixelSize: 24 * u; color: "#000"; width: parent.width - 32 * u; wrapMode: Text.Wrap; horizontalAlignment: Text.AlignHCenter }
        MouseArea { anchors.fill: parent; onClicked: root.cancel() }
    }

    // 面板: 右上角展开, 点外面关掉
    MouseArea { anchors.fill: parent; visible: root.open; onClicked: root.cancel(); z: 20 }
    Rectangle {
        id: panel
        visible: root.open; z: 21
        anchors.top: parent.top; anchors.right: parent.right; anchors.margins: 16 * u
        width: Math.min(parent.width - 32 * u, 4 * cell + 5 * gap); height: Math.min(parent.height * 0.7, grid.contentHeight + 2 * gap)
        radius: 16 * u; color: "#FFFFFF"; border.width: 2; border.color: "#000"
        readonly property real cell: 150 * u
        readonly property real gap: 14 * u
        MouseArea { anchors.fill: parent }   // 挡住底下的关闭层
        Flickable {
            id: grid
            anchors.fill: parent; anchors.margins: panel.gap; clip: true
            contentHeight: col.height
            Column {
                id: col
                width: grid.width; spacing: panel.gap
                Repeater {
                    model: root.packs
                    Column {
                        width: col.width; spacing: panel.gap * 0.6
                        property var pack: modelData
                        Text { text: pack.name; font.pixelSize: 22 * u; color: "#000"; visible: root.packs.length > 1 }
                        Flow {
                            width: parent.width; spacing: panel.gap
                            Repeater {
                                model: pack.stickers
                                Rectangle {
                                    width: panel.cell; height: panel.cell; radius: 10 * u
                                    color: tap.pressed ? "#DDD" : "#F7F5F0"; border.width: 1; border.color: "#BBB"
                                    Image {
                                        anchors.fill: parent; anchors.margins: 10 * u
                                        source: root.base + "/space/sticker/thumb?rel=" + encodeURIComponent(modelData.rel) + "&size=160"
                                        fillMode: Image.PreserveAspectFit; asynchronous: true; cache: false
                                    }
                                    MouseArea { id: tap; anchors.fill: parent; onClicked: root.choose(modelData.rel) }
                                }
                            }
                        }
                    }
                }
            }
        }
    }
}
