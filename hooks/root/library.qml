// 文件库桥 (常驻): 挂在注入库的 root 挂点上, 开机就在, 不依赖空间面板开着。
//
// space-server (internal/xolib) 把「让文件库看见新文件 / 建文件夹 / 建笔记本」排成活,
// 这里长连接取活、调 xochitl 内存里的对象做完、回报结果。外部进程够不着这些对象, 只能由
// 跑在 xochitl 里的 QML 来调。以前这段在外壳 Space.qml 里, 空间没打开时手写画布 (lib/ink)
// 建不了载体笔记; 外壳那份仍在 (两边谁先取到谁做), 它的长连接还负责「空间开着」这个状态。
//
// 取活带 role=bridge: 这条长连接不算空间开着 (扫码上传只在空间打开时开)。
// 运行时注入的组件里 Timer 不触发: 后台没起来时的退避靠注入库每秒递增的 hookTick。
import QtQuick

Item {
    id: bridge
    property var host: null      // 注入库给的主窗口顶层
    property int hookTick: 0     // 注入库每秒 +1
    width: 0
    height: 0
    visible: false

    readonly property string baseUrl: "http://127.0.0.1:8080"
    property var xoLibrary: null
    property var xoImporterHolder: null
    property var xoCtrlHolder: null
    property string libMode: ""
    property var libXhr: null
    property int libFails: 0
    property int retryAt: 0      // 失败后到哪个 hookTick 再连 (0 = 不在等)

    Component.onCompleted: libPoll()
    Component.onDestruction: { if (libXhr) { try { libXhr.abort() } catch (e) {} libXhr = null } }
    onHookTickChanged: { if (retryAt > 0 && hookTick >= retryAt) { retryAt = 0; libPoll() } }

    // 文件库对象不是 QML 单例, 只能从界面树上拿 —— 文件库界面里 CalendarActions 的 .library
    function findXoLibrary() {
        if (xoLibrary) return xoLibrary
        var top = host || bridge
        while (top.parent) top = top.parent
        var stack = [top], seen = 0
        while (stack.length && seen < 30000) {
            var n = stack.pop(); seen++
            if (String(n).indexOf("CalendarActions_") === 0 && n.library) { xoLibrary = n.library; return xoLibrary }
            var ch = n.children
            if (ch) for (var k = 0; k < ch.length; ++k) stack.push(ch[k])
        }
        return null
    }
    // 3.29 起的系统导入器 (USB 网页传文件用的那个)
    function xoImporter() {
        if (!xoImporterHolder) {
            try {
                xoImporterHolder = Qt.createQmlObject('import QtQml; import xofm.libs.library; QtObject { property var v: DocumentImporter }', bridge, "xoImporter")
            } catch (e) {
                return null
            }
        }
        var v = xoImporterHolder.v
        return (v && typeof v.importFromUrls === "function") ? v : null
    }
    // 3.29 起建文件夹 / 笔记本: LibraryController.createCollection / createDocument
    function xoController() {
        if (!xoCtrlHolder) {
            try {
                xoCtrlHolder = Qt.createQmlObject('import QtQml; import xofm.libs.library; QtObject { property var v: LibraryController }', bridge, "xoCtrl")
            } catch (e) {
                return null
            }
        }
        return xoCtrlHolder.v || null
    }
    function detectLibMode() {
        var lib = findXoLibrary()
        if (!lib) return ""   // 文件库界面还没建出来: 先不报模式, 下一轮再判 (3.28 也有导入器, 不能拿它当判据)
        if (typeof lib.requestLoadEntry === "function") return "load"
        return xoImporter() ? "import" : "load"
    }
    function localFileUrl(path) {
        return "file://" + path.split("/").map(encodeURIComponent).join("/")
    }

    // 有活立刻返回, 没活等 25 秒回 204 再连。后台没起来时按 1/2/4…秒退避, 最多 60 秒。
    function libPoll() {
        var x = new XMLHttpRequest()
        libXhr = x
        x.onreadystatechange = function () {
            if (x.readyState !== 4 || bridge.libXhr !== x) return
            bridge.libXhr = null
            if (x.status === 200) {
                bridge.libFails = 0
                var r = null
                try { r = JSON.parse(x.responseText) } catch (e) { r = null }
                var jobs = (r && r.jobs) ? r.jobs : []
                for (var i = 0; i < jobs.length; ++i) bridge.libHandle(jobs[i])
                bridge.libPoll()
            } else if (x.status === 204) {
                bridge.libFails = 0
                bridge.libPoll()
            } else {
                bridge.libFails++
                bridge.retryAt = bridge.hookTick + Math.min(60, Math.pow(2, Math.min(bridge.libFails, 6)))
            }
        }
        if (!libMode) libMode = detectLibMode()
        x.open("GET", baseUrl + "/space/library/next?role=bridge&wait=25" + (libMode ? "&mode=" + libMode : ""))
        x.send()
    }
    function libHandle(job) {
        var out = { op: job.op, id: job.id, ok: false }
        if (job.op === "import") {
            var imp = xoImporter()
            if (!imp) out.error = "找不到导入器 (DocumentImporter)"
            else try { imp.importFromUrls([localFileUrl(job.path)], job.parent || ""); out.ok = true } catch (e) { out.error = String(e) }
        } else if (job.op === "folder" || job.op === "notebook") {
            var ctrl = xoController()
            if (!ctrl) out.error = "找不到 LibraryController"
            else try {
                var nid = job.op === "folder" ? ctrl.createCollection(job.parent || "", job.name)
                                              : ctrl.createDocument(job.parent || "", job.name)
                out.newId = nid ? String(nid) : ""
                out.ok = out.newId !== ""
                if (!out.ok) out.error = "没有返回新条目"
            } catch (e) { out.error = String(e) }
        } else if (job.op === "load") {
            var lib = findXoLibrary()
            if (!lib) out.error = "找不到文件库对象 (CalendarActions.library)"
            else try { lib.requestLoadEntry(job.id); out.ok = true } catch (e) { out.error = String(e) }
        } else out.error = "不认识的操作: " + job.op
        console.warn("[space-bridge] 文件库 " + job.op + " " + job.id + " → " + (out.ok ? "ok" : out.error))
        var x = new XMLHttpRequest()
        x.open("POST", baseUrl + "/space/library/done")
        x.setRequestHeader("Content-Type", "application/json")
        x.send(JSON.stringify(out))
    }
}
