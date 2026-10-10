// InkPad — 手写画布 (空间共享组件)。笔迹由 xochitl 原生算法生成矢量、存成 .rm, 和原生笔记本一样。
//
// 用法:
//   import "file:///home/root/rmkit-cn/space/kit" as Kit
//   Kit.InkPad { id: ink; anchors.fill: …; space: root.space; app: "plan"; key: "2026-10-09" }
//   ink.start()   // 开始写 (几百毫秒后笔才可用, 看 ready)
//   ink.stop()    // 写完 (存盘, 也会在组件销毁时自动做)
//   ink.undo()
// 不在写的时候, 它就是一张只读的画 (从正本读出来自己画)。
//
// 原理 (详见 internal/xolib/ink.go): xochitl 只往「一本打开着的笔记本」里写笔画, 所以用一本
// 放在回收站里的载体笔记本。start: 后台把这块画布的正本拷进载体 → 后台打开载体 → 在画布上
// 盖一层原生笔输入层 (PenInputSurface + PenInputLineHandler, 笔尖跟手) → 抬笔把这一笔交给
// 载体的 sceneController.addDrawingLine, 由 xochitl 原生算法写进 .rm。stop: 关掉载体 → 拷回正本。
//
// 原生描线直接写屏, 绕过 Qt: 这块区域里任何 Qt 重绘都会盖掉它。所以每次抬笔后都从载体
// 读回这一笔 (已经是原生算好的矢量) 画进下面的画布, 屏上留下的是画布里的这份。
//
// 这些原生类型 (com.remarkable / 文件库对象) 只在 xochitl 进程里有, Mac 上预览不了。
import QtQuick
import com.remarkable

Item {
    id: pad

    property var space
    property string app: ""
    property string key: ""

    // 笔: xochitl 的 Line::Tool 编号 (实测) —— 21 书法笔, 15 圆珠笔, 14 铅笔, 12 画笔,
    // 16 马克笔, 17 针管笔, 13 自动铅笔, 18 荧光笔
    property int tool: 15
    property real thickness: 1
    property color paper: "transparent"

    readonly property bool writing: ses !== null       // 在写 (载体开着)
    property bool ready: false                          // 笔可以落了
    property string error: ""
    signal saved()                                      // stop 存完

    property var ses: null      // {carrierId, pageId, ctrl, dv, handler, surface, strokes}
    property var runs: []       // 已经画在画布上的折线
    property int painted: 0     // 画布上已经画了几段 (增量画)
    property bool fullRepaint: true
    property point origin: Qt.point(0, 0)   // 画布在屏幕上的位置 (笔输入层坐标换算用)

    readonly property string base: space ? space.baseUrl : ""

    // ─── 原生对象 ─────────────────────────────────────────────────
    function findNode(prefix, test) {
        var top = pad
        while (top.parent) top = top.parent
        var stack = [top], seen = 0
        while (stack.length && seen < 30000) {
            var n = stack.pop(); seen++
            if (String(n).indexOf(prefix) === 0 && (!test || test(n))) return n
            var ch = n.children
            if (ch) for (var k = 0; k < ch.length; ++k) stack.push(ch[k])
        }
        return null
    }
    property var _dv: null
    property var _lib: null
    property var _mgr: null
    function natives() {
        if (!_dv) _dv = findNode("DocumentView_")
        if (!_lib) { var ca = findNode("CalendarActions_", function (n) { return !!n.library }); _lib = ca ? ca.library : null }
        // 笔输入管理器是全局共享的, 从 xochitl 现成的笔输入层上借
        if (!_mgr) { var s = findNode("PenInputSurface", function (n) { return !!n.manager }); _mgr = s ? s.manager : null }
        return _dv && _lib && _mgr
    }

    function req(method, path, body, cb) {
        var x = new XMLHttpRequest()
        x.onreadystatechange = function () {
            if (x.readyState !== 4) return
            var r = null
            try { r = JSON.parse(x.responseText) } catch (e) { r = null }
            if (cb) cb(x.status, r)
        }
        x.open(method, pad.base + path)
        if (body !== null && body !== undefined) x.setRequestHeader("Content-Type", "application/json")
        x.send(body !== null && body !== undefined ? JSON.stringify(body) : null)
    }
    function fail(msg) {
        pad.error = msg
        console.warn("[InkPad] " + msg)
        if (pad.space && pad.space.toast) pad.space.toast(msg)
    }

    // ─── 只读显示 ─────────────────────────────────────────────────
    function refresh(live) {
        if (!pad.app || !pad.key || !pad.base) return
        req("GET", "/space/ink/drawing?app=" + pad.app + "&key=" + encodeURIComponent(pad.key) + (live ? "&live=1" : ""),
            null, function (st, r) {
                if (!pad || st !== 200 || !r) return
                pad.applyRuns(r.runs || [], r.strokes || 0)
            })
    }
    // 只多了几段就增量画, 否则整张重画 (擦除 / 撤销 / 荧光笔插在中间)
    function applyRuns(runs, strokes) {
        var old = pad.runs
        var incremental = !pad.fullRepaint && runs.length >= old.length && old.length > 0
        if (incremental) {
            for (var i = 0; i < old.length; ++i)
                if (runs[i].p.length !== old[i].p.length || runs[i].c !== old[i].c) { incremental = false; break }
        }
        pad.runs = runs
        if (!incremental) { pad.fullRepaint = true; pad.painted = 0 }
        canvas.requestPaint()
        if (pad.ses) pad.ses.strokes = strokes
    }

    Rectangle { anchors.fill: parent; color: pad.paper }
    Canvas {
        id: canvas
        anchors.fill: parent
        // 不用 Canvas.Immediate: xochitl 的软件渲染下它画了也不上屏 (本机 offscreen 能画, 设备上空白)
        onPaint: {
            var ctx = getContext("2d")
            if (pad.fullRepaint) { ctx.clearRect(0, 0, width, height); pad.painted = 0; pad.fullRepaint = false }
            ctx.lineJoin = "round"
            for (var i = pad.painted; i < pad.runs.length; ++i) {
                var r = pad.runs[i], p = r.p
                if (p.length < 2) continue
                ctx.globalAlpha = r.o
                ctx.strokeStyle = r.c
                ctx.lineWidth = r.w
                ctx.lineCap = r.cap === "square" ? "square" : "round"
                ctx.beginPath()
                ctx.moveTo(p[0], p[1])
                if (p.length === 2) ctx.lineTo(p[0] + 0.01, p[1])
                for (var j = 2; j < p.length; j += 2) ctx.lineTo(p[j], p[j + 1])
                ctx.stroke()
            }
            ctx.globalAlpha = 1
            pad.painted = pad.runs.length
        }
    }

    // ─── 开始 / 结束写 ────────────────────────────────────────────
    function start() {
        if (pad.ses || !pad.app || !pad.key) return
        pad.error = ""
        if (!natives()) { fail("找不到 xochitl 的文档视图 / 文件库 / 笔输入管理器"); return }
        // 后台若开着别的文档 (上一块画布的载体) 先关掉, 载体换文件前必须关着
        try { if (_dv.document) _dv.close() } catch (e) {}
        // 原生笔输入层给的是整块屏幕的坐标 (实测 1:1, 原点在屏幕左上角), 画布可能在屏幕任何位置。
        // 把画布此刻在屏幕上的位置交给后端: 正本存画布坐标, 拷进载体时换成屏幕坐标, 存回时再换回来
        // (笔输入层的 setTransform 要 QTransform, QML 造不出来, 只能在后端换)。
        var o = pad.mapToItem(null, 0, 0)
        pad.origin = Qt.point(o.x, o.y)
        req("POST", "/space/ink/load", { app: pad.app, key: pad.key, x: o.x, y: o.y }, function (st, r) {
            if (!pad) return
            if (st !== 200 || !r || !r.carrierId) { pad.fail("载体准备失败 (" + st + ")"); return }
            pad.ses = { carrierId: r.carrierId, pageId: r.pageId, app: pad.app, key: pad.key,
                        ctrl: null, handler: null, surface: null, strokes: 0 }
            step.n = 0; step.phase = "entry"; step.start()
        })
    }
    // 等文件库能查到载体 → 打开 → 等场景控制器就绪 → 盖笔输入层
    Timer {
        id: step; interval: 150; repeat: false
        property int n: 0
        property string phase: ""
        property var wrapper: null
        onTriggered: {
            var s = pad.ses
            if (!s) return
            if (++n > 40) {
                var dd = pad._dv ? pad._dv.document : null, cc = pad._dv ? pad._dv.sceneController : null
                console.warn("[InkPad] 超时现场: document=" + dd + " id=" + (dd ? String(dd.id) : "") + " 期望=" + s.carrierId
                             + " controller=" + cc + " pageId=" + (cc ? String(cc.pageId) : "") + " 期望页=" + s.pageId)
                pad.fail("打开载体超时 (" + phase + ")"); pad.abort(); return
            }
            if (phase === "entry") {
                var w = null
                try { w = pad._lib.entryForId(s.carrierId) } catch (e) { w = null }
                if (!w) { step.start(); return }
                wrapper = w
                try { pad._dv.openDocument(w) } catch (e2) { pad.fail("打开载体失败: " + e2); pad.abort(); return }
                phase = "scene"; n = 0; step.start()
            } else if (phase === "scene") {
                var d = pad._dv.document, c = pad._dv.sceneController
                // 文档 ID / 页 ID 是 xochitl 自己的 entry::Id 类型, 不是 JS 字符串 —— 必须先转成字符串再比
                if (!d || String(d.id) !== s.carrierId || !c || String(c.pageId) !== s.pageId) { step.start(); return }
                s.ctrl = c
                // 不能马上盖: xochitl 自己的笔输入层在文档加载完后才登记, 会把笔抢回去
                // (探针里隔了几秒才盖所以能写)。等一会再盖, 盖完再登记一次。
                phase = "settle"; n = 0; step.interval = 800; step.start()
            } else if (phase === "settle") {
                step.interval = 150
                pad.attachPen()
            }
        }
    }
    // 载体在后台真的开着, 它的页面显示层 (DeviceSceneViewport) 收到新笔画会绕过 Qt 直接刷屏,
    // 把我们这层的湿墨抹掉 —— 写字期间让它暂停刷屏, 屏上只留原生描线器画的那份 (= 探针的效果)
    function viewport() {
        return pad.findNode("DeviceSceneViewport", function (n) { return n.visible && !!n.tileManager })
    }
    function holdViewport(on) {
        var v = viewport()
        if (!v) { console.warn("[InkPad] 没找到载体的页面显示层"); return }
        try { v.blockingUpdates = on } catch (e) { console.warn("[InkPad] 设 blockingUpdates 失败 " + e) }
        console.warn("[InkPad] 载体页面显示层 " + (on ? "暂停刷屏" : "恢复刷屏") + ": " + v)
    }
    function attachPen() {
        var s = pad.ses
        pad.holdViewport(true)
        s.handler = handlerComp.createObject(pad, { lineTool: pad.tool, lineThickness: pad.thickness })
        s.surface = surfaceComp.createObject(pad)
        s.surface.handler = s.handler
        s.surface.manager = pad._mgr
        s.handler.stroke.connect(pad.onStroke)
        reclaim.start()
        pad.ready = true
        pad.refresh(true)
        console.warn("[InkPad] " + pad.app + "/" + pad.key + " 可以写了")
    }
    // 盖上之后再登记一次笔输入管理器, 确保笔落到这一层而不是 xochitl 自己那层
    Timer {
        id: reclaim; interval: 300; repeat: false
        onTriggered: {
            var s = pad.ses
            if (!s || !s.surface) return
            try { s.surface.manager = null; s.surface.manager = pad._mgr } catch (e) {}
            console.warn("[InkPad] 重新登记笔输入层: " + s.surface + " " + s.surface.width + "x" + s.surface.height
                         + " 屏上位置=(" + Math.round(pad.origin.x) + "," + Math.round(pad.origin.y) + ")")
        }
    }
    function onStroke(line) {
        var s = pad.ses
        var b = line.boundingRect
        console.warn("[InkPad] 收到一笔 笔型=" + line.tool + " 点数=" + line.pointCount + " 橡皮=" + line.isEraserTool
                     + " 画布上范围=(" + Math.round(b.x) + "," + Math.round(b.y) + " " + Math.round(b.width) + "x" + Math.round(b.height) + ")")
        if (!s || !s.ctrl) return
        try {
            if (line.isEraserTool) s.ctrl.eraseWithLine(line)
            else s.ctrl.addDrawingLine(line)
        } catch (e) { pad.fail("写入笔画失败: " + e); return }
        console.warn("[InkPad] 已交给载体, 内容范围=" + s.ctrl.itemsBoundingRect)
        if (s) s.strokes++
    }
    function undo() {
        if (!pad.ses || !pad.ses.ctrl) return
        try { if (pad.ses.ctrl.undoAvailable) pad.ses.ctrl.undo() } catch (e) {}
    }
    // 调试: 撤掉再盖一次笔输入层 (对照探针 —— 探针是载体开了一阵后手动盖的)
    function reattach() {
        if (!pad.ses || !pad.ses.ctrl) { console.warn("[InkPad] reattach: 载体没开着"); return }
        detachPen()
        attachPen()
    }
    function penState() {
        var s = pad.ses
        if (!s || !s.handler) return "没有描线器"
        var t = "?"
        try { t = s.handler.timeSincePenUp() } catch (e) { t = "<" + e + ">" }
        return "timeSincePenUp=" + t + " surface=" + s.surface + " 可见=" + (s.surface ? s.surface.visible : "")
             + " manager=" + (s.surface ? s.surface.manager : "") + " dv.visible=" + (pad._dv ? pad._dv.visible : "")
    }
    function detachPen() {
        var s = pad.ses
        if (!s) return
        if (s.surface) {
            try { s.surface.manager = null } catch (e) {}
            try { s.surface.handler = null } catch (e2) {}
            s.surface.destroy(); s.surface = null
        }
        if (s.handler) { s.handler.destroy(); s.handler = null }
        pad.holdViewport(false)
        pad.ready = false
    }
    function abort() {
        detachPen()
        try { if (pad._dv && pad._dv.document) pad._dv.close() } catch (e) {}
        pad.ses = null
    }
    // 写完: 撤掉笔输入层 → 关载体 → 等一下让 xochitl 收尾写盘 → 拷回正本
    function stop() {
        if (!pad.ses) return
        // 用会话里记的那块画布: 换页时 key 已经变了, 载体里装的还是上一块
        var app = pad.ses.app, key = pad.ses.key
        detachPen()
        try { if (pad._dv && pad._dv.document) pad._dv.close() } catch (e) {}
        pad.ses = null
        req("POST", "/space/ink/save", { app: app, key: key }, function (st, r) {
            if (!pad) return
            if (st !== 200) pad.fail("保存手写失败 (" + st + ")")
            pad.fullRepaint = true
            pad.refresh(false)
            pad.saved()
        })
    }

    onToolChanged: if (pad.ses && pad.ses.handler) pad.ses.handler.lineTool = pad.tool
    onThicknessChanged: if (pad.ses && pad.ses.handler) pad.ses.handler.lineThickness = pad.thickness
    onKeyChanged: { if (pad.ses) stop(); pad.fullRepaint = true; pad.runs = []; canvas.requestPaint(); refresh(false) }
    Component.onCompleted: refresh(false)
    // 应用被关掉时还在写: 尽量存一下。来不及也不丢 —— 下次打开时后端会先把载体里没存的拷回去
    Component.onDestruction: stop()

    Component { id: handlerComp; PenInputLineHandler {} }
    Component { id: surfaceComp; PenInputSurface { anchors.fill: parent } }
}
