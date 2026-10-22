// 笔记里的「贴纸」面板。注入库 (qml_inject_impl.cpp doInjectDocAddons) 在打开笔记时挂到 DocumentView 上,
// 文件 mtime 变了会自动重建 (改面板不用重启 xochitl)。
//
// 用法: 右上角往左下斜滑 → 面板 (按贴纸包分页签) → 点一个贴纸 → 用笔在页面上点一下 → 贴纸按原生笔迹
// 描在那里; 要移动缩放由用户自己用选择工具圈。描是后台用虚拟笔逐笔写的 (/space/sticker/stroke), 每笔之前把笔的颜色/粗细
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
    property int tab: 0                 // 当前页签 (贴纸包下标), 面板关了也记着
    property int page: 0                // 当前页 (墨水屏翻页不滚动)
    readonly property int perPage: 16   // 4 列 × 4 行
    readonly property var curStickers: packs.length ? packs[Math.min(tab, packs.length - 1)].stickers : []
    readonly property int pageCount: Math.max(1, Math.ceil(curStickers.length / perPage))
    function turn(d) { var p = page + d; if (p >= 0 && p < pageCount) page = p }
    property bool zh: true
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
        xhr("GET", "/space/sticker/list", null, function (st, r) {
            if (st !== 200 || !r || !r.packs) { root.hint = "贴纸读不到 / Stickers unavailable"; return }
            root.zh = (r.lang || "zh").indexOf("zh") === 0
            var keep = root.packs.length && root.tab < root.packs.length ? root.packs[root.tab].id : ""
            root.packs = r.packs
            var t = 0
            for (var i = 0; i < r.packs.length; i++) if (r.packs[i].id === keep) t = i
            root.tab = t
            if (root.page >= root.pageCount) root.page = 0
            root.open = true; root.hint = ""
        })
    }
    function choose(rel) {
        open = false
        pending = rel
        hint = ""
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
        var h = penHandler()
        if (!h) { log("place: no handler"); hint = "这里不能贴 / Not a notebook page"; return }
        busy = true
        // 笔型/粗细改工具栏的「主笔设置」(PenSetting), 顺着系统绑定传到处理器 (dsv.strokeHandler)。
        // 颜色不能这样走: 主笔的调色板里没有 9 (自定义 ARGB), 工具栏在落笔时会把它纠正回黑色 (2026-10-21 实测)。
        // 所以颜色直接写处理器的 lineColor/lineArgbCode, 描完用 Qt.binding 接回「当前选中那支笔」——
        // 实测处理器这两个值跟随 primaryPen / secondaryPen / selectionPen 中被选中的那支。
        // 不接回的后果: 之后用户在工具栏换笔换色都不生效, 重开笔记也不恢复, 只能重启 xochitl。
        var saved = { t: pen.tool, w: pen.thickness }
        try { tb.selectPen("primary") } catch (e) {}
        var at = { op: "begin", sticker: rel, cx: x, cy: y, size: stickerSize }
        post("/space/sticker/stroke", at, function (st, r) {
            if (st !== 200 || !r) { root.busy = false; root.hint = "贴失败 / Failed"; return }
            var meta = r.strokes, i = 0, retries = 0
            function restore() {
                pen.tool = saved.t; pen.thickness = saved.w
                function cur() {
                    var n = tb.selectedTool ? String(tb.selectedTool.objectName) : ""
                    if (n.indexOf("secondary") === 0) return tb.secondaryPen
                    if (n.indexOf("selection") === 0) return tb.selectionPen
                    return tb.primaryPen
                }
                h.lineColor = Qt.binding(function () { return cur().color })
                h.lineArgbCode = Qt.binding(function () { return cur().colorCode })
                if (h.lineColor !== cur().color) log("restore: binding mismatch " + h.lineColor + " vs " + cur().color)
            }
            function next() {
                if (i >= meta.length) { finish(); return }
                var m = meta[i]
                pen.tool = 17
                if (m.argb !== undefined) { h.lineArgbCode = m.argb; h.lineColor = 9 }   // 自定义颜色: 9 + ARGB
                else { h.lineColor = root.xoColor(m.color); h.lineArgbCode = 0xFF000000 }
                pen.thickness = m.thickness !== undefined ? m.thickness : Math.max(1, m.width / 1.8)   // 边缘线: 针管笔标准档
                root.post("/space/sticker/stroke", { op: "draw", index: i }, function (st2) {
                    if (st2 === 200) { i++; retries = 0; next(); return }
                    if (st2 === 409 && retries < 3) { retries++; next(); return }
                    root.post("/space/sticker/stroke", { op: "end" }, function () { restore(); root.busy = false; root.hint = "贴了一半，笔一直没移开 / Interrupted by the pen" })
                })
            }
            function finish() {
                // 只管画出来; 选中/移动/缩放交给用户自己用选择工具 (2026-10-21 用户定)
                root.post("/space/sticker/stroke", { op: "end" }, function () {
                    restore(); root.busy = false; root.hint = ""
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

    // 面板: 右上角的对话框, 点外面关掉。版式 (2026-10-22 用户定): L 形分类 —— 左上角交叉处是「常用」,
    // 其余分类一半排顶部、一半排左侧, 全部一屏放下不用滚; 右下是 4×4 格子, 底部页码 1 2 3 直接跳页。
    MouseArea { anchors.fill: parent; visible: root.open; onClicked: root.cancel(); z: 20 }
    Rectangle {
        id: panel
        visible: root.open; z: 21
        readonly property real cell: 116 * u
        readonly property real gap: 12 * u
        readonly property real pad: 18 * u
        readonly property real side: 132 * u                     // 左列宽 = 顶行高 (交叉处是正方形)
        readonly property real gridW: 4 * cell + 3 * gap
        readonly property real gridH: 4 * cell + 3 * gap
        readonly property int topN: Math.min(4, Math.ceil((root.packs.length - 1) / 2))
        readonly property int leftN: Math.max(0, root.packs.length - 1 - topN)
        anchors.top: parent.top; anchors.right: parent.right; anchors.topMargin: 128 * u; anchors.rightMargin: 16 * u
        width: pad + side + gap + gridW + pad
        height: pad + side + gap + gridH + pager.height + pad
        radius: 18 * u; color: "#F7F6F2"; border.width: 2; border.color: "#BDBAB2"
        MouseArea { anchors.fill: parent }   // 挡住底下的关闭层

        component Tab: Rectangle {
            property int idx: 0
            property bool cur: idx === root.tab
            readonly property var pk: root.packs[idx] || {}
            radius: 14 * u; color: cur ? "#C9E0AE" : "transparent"   // 选中: 「计划」底部页签同款淡绿
            Column {
                anchors.centerIn: parent; spacing: 4 * u
                Image {
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: 50 * u; height: 50 * u; fillMode: Image.PreserveAspectFit; asynchronous: true; cache: false
                    source: pk.icon ? root.base + "/space/sticker/thumb?size=96&rel=" + encodeURIComponent(pk.icon) : ""
                }
                Text {
                    anchors.horizontalCenter: parent.horizontalCenter
                    width: panel.side - 8 * u; horizontalAlignment: Text.AlignHCenter; elide: Text.ElideRight
                    text: (!root.zh && pk.name_en) ? pk.name_en : (pk.name || "")
                    font.pixelSize: 22 * u; color: "#000"
                }
            }
            MouseArea { anchors.fill: parent; onClicked: { root.tab = parent.idx; root.page = 0 } }
        }

        // 交叉处: 第一个包 (常用)
        Tab { x: panel.pad; y: panel.pad; width: panel.side; height: panel.side; idx: 0; visible: root.packs.length > 0 }
        // 顶行
        Repeater {
            model: panel.topN
            Tab {
                idx: 1 + index
                x: panel.pad + panel.side + panel.gap + index * (panel.gridW / panel.topN); y: panel.pad
                width: panel.gridW / panel.topN; height: panel.side
            }
        }
        // 左列
        Repeater {
            model: panel.leftN
            Tab {
                idx: 1 + panel.topN + index
                readonly property real h: (panel.gridH + panel.gap + pager.height) / Math.max(4, panel.leftN)
                x: panel.pad; y: panel.pad + panel.side + panel.gap + index * h
                width: panel.side; height: h
            }
        }
        // 分隔线
        Rectangle { x: panel.pad + panel.side + panel.gap / 2; y: panel.pad + panel.side + panel.gap / 2; width: panel.gridW + panel.gap / 2; height: 2; color: "#CFCCC4" }
        Rectangle { x: panel.pad + panel.side + panel.gap / 2; y: panel.pad + panel.side + panel.gap / 2; width: 2; height: panel.gridH + pager.height + panel.gap / 2; color: "#CFCCC4" }

        // 4×4 格子; 左右划翻页
        Item {
            id: gridBox
            x: panel.pad + panel.side + panel.gap; y: panel.pad + panel.side + panel.gap
            width: panel.gridW; height: panel.gridH
            property real pressX: 0
            MouseArea {   // 空白处也能划
                anchors.fill: parent
                onPressed: (e) => { gridBox.pressX = e.x }
                onReleased: (e) => { var dx = e.x - gridBox.pressX; if (Math.abs(dx) > 60 * u) root.turn(dx < 0 ? 1 : -1) }
            }
            Grid {
                columns: 4; spacing: panel.gap
                Repeater {
                    model: root.curStickers.slice(root.page * root.perPage, (root.page + 1) * root.perPage)
                    Rectangle {
                        width: panel.cell; height: panel.cell; radius: 12 * u
                        color: tap.pressed ? "#E2DFD8" : "#FFFFFF"; border.width: 2; border.color: "#D8D5CE"
                        Image {
                            anchors.fill: parent; anchors.margins: 12 * u
                            source: root.base + "/space/sticker/thumb?rel=" + encodeURIComponent(modelData.rel) + "&size=160"
                            fillMode: Image.PreserveAspectFit; asynchronous: true; cache: false
                        }
                        MouseArea {
                            id: tap; anchors.fill: parent
                            property real px: 0
                            onPressed: (e) => { px = e.x }
                            onReleased: (e) => {
                                var dx = e.x - px
                                if (Math.abs(dx) > 60 * u) root.turn(dx < 0 ? 1 : -1)
                                else if (Math.abs(dx) < 20 * u) root.choose(modelData.rel)
                            }
                        }
                    }
                }
            }
        }

        // 页码 1 2 3 …: 点哪页跳哪页; 只有一页也显示「1」, 面板大小固定, 切分类时墨水屏不用大面积重刷
        Item {
            id: pager
            x: panel.pad + panel.side + panel.gap; y: panel.pad + panel.side + panel.gap + panel.gridH
            width: panel.gridW; height: 82 * u
            Row {
                anchors.horizontalCenter: parent.horizontalCenter; anchors.bottom: parent.bottom
                spacing: 12 * u
                Repeater {
                    model: root.pageCount
                    Rectangle {
                        property bool cur: index === root.page
                        width: 60 * u; height: 60 * u; radius: 12 * u
                        color: cur ? "#C9E0AE" : "#FFF"
                        border.width: 2; border.color: cur ? "#C9E0AE" : "#CFCCC4"
                        Text { anchors.centerIn: parent; text: index + 1; font.pixelSize: 26 * u; color: "#000" }
                        MouseArea { anchors.fill: parent; onClicked: root.page = index }
                    }
                }
            }
        }
    }
}
