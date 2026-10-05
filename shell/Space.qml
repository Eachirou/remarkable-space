// Space.qml — 「空间 (SPACE)」启动台外壳
//
// 职责只有三件事, 别往这里塞功能:
//   1. 首页与标签页: 空间 (小组件 + 我的应用) / 应用 (全部) / 发现 (商店) / 设置 (设置类应用列表)
//      数据全部来自 upload-server 的 /space/* (注册表、商店索引), 外壳不认任何具体应用
//   2. 打开/关闭应用: 按注册表里的 file:// 入口动态创建组件, 退出即销毁; 声明了后台的先拉起后台
//   3. 给应用一个稳定的 API 对象 (space): 基址、目录、标题/边框控制、返回拦截、HTTP 帮手
import QtQuick
import QtQuick.Layouts
import device.ui.controls
import "file:///home/root/rmkit-cn/space/kit" as Kit

Rectangle {
    id: space
    objectName: "_rmkitSpace"
    anchors.fill: parent
    color: "white"
    visible: true
    z: 99999

    readonly property int shellVersion: 1
    readonly property string kitDir: "file:///home/root/rmkit-cn/space/kit"
    property string baseUrl: "http://127.0.0.1:8080"
    // ─── 尺寸基准: 不写死任何机型 ───
    // 以 Paper Pro Move 竖屏短边 954px 为 1.0; Paper Pro (1620x2160) 竖屏约 1.7。横屏时短边不变, 版式靠宽度自适应。
    // 两台机器 DPI 接近 (229 / 264), 所以字号只温和放大 (fs), 边距/卡片高度按 unit 放大, 列数按内容宽度算。
    readonly property real unit: Math.max(0.6, Math.min(width, height) / 954)
    // 纵向另算一套。两台机器高宽比差很多 —— Move 954x1696 (1:1.78), Paper Pro 1620x2160
    // (1:1.33)。尺寸全跟着短边走的话, Paper Pro 上宽度放大 1.70 倍而可用高度只有 1.27 倍,
    // 首页纵向超出约 230px, 要滚一截才看得全。
    // 所以卡片高度、行距、上下留白改用 vpx: 元素该多大还多大 (字号图标仍按 unit), 只压纵向。
    // 下限 0.75 是给横屏留的 —— 横屏高度骤减, 再按比例压就没法看, 那种情况让它滚。
    readonly property real vunit: Math.max(0.75, Math.min(unit, height / 1696))
    readonly property real fs: 1 + (unit - 1) * 0.35
    readonly property bool largeScreen: unit > 1.4
    readonly property int margin: Math.round(64 * unit)
    function px(n) { return Math.round(n * unit) }      // 版式尺寸 (横向与元素)
    function vpx(n) { return Math.round(n * vunit) }   // 纵向尺寸: 卡片高度、行距、上下留白
    function fpx(n) { return Math.round(n * fs) }       // 字号
    function cols(contentWidth, minTile) { return Math.max(2, Math.floor(contentWidth / (minTile * fs))) }

    property string tab: "home"       // home / apps / discover / settings

    // ─── 刷新波形档 (整个空间统一) ───────────────────────────────────
    //   ui      = 原厂 MainView 那档: 快、有彩色、干净, 但色阶少
    //   content = 文档那档: 完整灰阶 + 彩色, 最细腻, 但慢, 进入时会留一层淡残影
    // 外壳整屏挂一个常驻标记, 几何零变化 (EPScreenModeMap 每变一次几何就整屏重合成),
    // 应用跑在 appHost 里自然继承, 所以应用侧不要再自己挂。
    property string screenMode: "ui"
    property var _screenModeItem: null
    function applyScreenMode() {
        if (space._screenModeItem) { space._screenModeItem.destroy(); space._screenModeItem = null }
        try {
            space._screenModeItem = Qt.createQmlObject(
                'import QtQuick; import xofm.libs.epaper; ScreenModeItem { anchors.fill: parent; mode: ScreenModeItem.'
                + (space.screenMode === "content" ? "Content" : "UI") + ' }',
                space, "spaceScreenMode")
        } catch (e) {
            console.warn("[space] ScreenModeItem 不可用, 沿用默认刷新: " + e)
        }
    }
    onScreenModeChanged: space.applyScreenMode()
    // 外壳自己的设置存在应用数据区的保留 id "shell" 下 (后端 id 正则不允许下划线开头)
    function loadShellCfg() {
        request("GET", baseUrl + "/space/apps/shell/data/config", null, function (st, r) {
            if (st === 200 && r && r.screenMode) space.screenMode = r.screenMode
            else space.applyScreenMode()
        })
    }
    function saveShellCfg() {
        request("PUT", baseUrl + "/space/apps/shell/data/config",
                { screenMode: space.screenMode }, null)
    }
    property var apps: []
    property string listError: ""
    property var storeApps: []
    property var recentDoc: null      // 最近在读的文档 (/space/recent, 取自 xochitl 元数据)
    property string storeError: ""
    property bool storeLoaded: false
    property int tzOffset: 480            // 后端给的本地时区偏移 (分钟); 设备系统时区是 UTC
    property bool devMode: false          // 后端 .dev 标记: 打开应用绕过 QML 组件缓存
    property date now: new Date()
    // 本地时间 = 当前时刻按 tzOffset 平移后, 用进程时区 (UTC) 的表示来显示; 若进程时区不是 UTC 也能纠正
    function localNow() { return new Date(now.getTime() + (tzOffset + now.getTimezoneOffset()) * 60000) }

    property var current: null        // 正在打开的应用 (注册表条目)
    property var currentItem: null    // 应用根 Item
    property string appError: ""      // 应用加载失败原因 (给开发者看)
    property string appTitle: ""
    property bool chrome: true        // false = 应用全屏 (自己画导航)
    property string toastText: ""
    property string busyText: ""
    property var confirmApp: null

    // ─── 给应用的 API ─────────────────────────────────────────────────
    QtObject {
        id: api
        readonly property int shellVersion: space.shellVersion
        readonly property string baseUrl: space.baseUrl
        readonly property string kitDir: space.kitDir
        readonly property bool largeScreen: space.largeScreen
        readonly property string screenMode: space.screenMode   // ui / content, 整个空间统一, 应用别自己挂
        readonly property real unit: space.unit           // 版式缩放基准 (Move 竖屏 = 1)
        // 纵向另算一套: Paper Pro 宽是 Move 的 1.70 倍而高只有 1.27 倍, 纵向尺寸
        // 再跟着 unit 走就会把一屏撑爆。卡片高度、上下留白、纵向间距用它。
        readonly property real vunit: space.vunit
        readonly property real fontScale: space.fs        // 字号缩放基准
        readonly property real screenWidth: space.width
        readonly property real screenHeight: space.height
        property string appId: ""
        property string appDir: ""       // 应用目录 (绝对路径)
        property string dataDir: ""      // 应用私有数据目录 (卸载不删)
        property string serviceUrl: ""   // 应用自带后台的 http://127.0.0.1:<port>, 没有则空
        property string title: ""        // 改它 = 改外壳大标题
        property bool chrome: true       // false = 外壳隐藏导航栏和标题, 应用全屏
        property var onBack: null        // function(): 返回 true 表示应用自己消费了返回键

        onTitleChanged: space.appTitle = title
        onChromeChanged: space.chrome = chrome

        function exit() { space.closeApp() }              // 退出应用回到原标签页
        function closeSpace() { space.close() }           // 连启动台一起关
        function toast(msg) { space.showToast(msg) }
        function openApp(id) { space.openById(id) }       // 应用间跳转 (如小组件点开自己的应用)
        function request(method, url, body, cb) { space.request(method, url, body, cb) }
        // 应用把 main.qml 拆成多个文件时, 子文件的 URL 也走这里: 和入口同一个 rev 令牌,
        // 开发模式下改了会自动失效, 没改就命中缓存 (别自己拼 Date.now(), 那会每次都重编)
        function bust(url) { return space.bust(url, space.current ? space.current.rev : 0) }
        function get(path, cb) { space.request("GET", space.baseUrl + path, null, cb) }
        function post(path, body, cb) { space.request("POST", space.baseUrl + path, body, cb) }
        function svcGet(path, cb) { space.request("GET", serviceUrl + path, null, cb) }
        function svcPost(path, body, cb) { space.request("POST", serviceUrl + path, body, cb) }
        function localNow() { return space.localNow() }   // 按设备时区 (见 /space/apps 的 tzOffset) 的当前时间
        // 应用数据: 存到 dataDir/<key>.json (纯 QML 应用的持久化); 小组件要传自己的 app.id
        function dataGet(id, key, cb) { space.request("GET", space.baseUrl + "/space/apps/" + id + "/data/" + key, null, cb) }
        function dataPut(id, key, obj, cb) { space.request("PUT", space.baseUrl + "/space/apps/" + id + "/data/" + key, obj, cb) }
    }

    // ─── 逻辑 ─────────────────────────────────────────────────────────
    function request(method, url, body, cb) {
        var x = new XMLHttpRequest()
        x.onreadystatechange = function() {
            if (x.readyState !== 4) return
            var r = null
            try { r = JSON.parse(x.responseText) } catch (e) { r = null }
            // file:// 读成功时 XHR 的 status 是 0 而不是 200 —— 本地文件没有 HTTP 状态行。
            // 应用读自带的 json 资源 (如天气的 phrases.json) 走的就是 file://, 调用方
            // 一律按 200 判断, 不折算的话那些资源永远读不到, 还只会静默走兜底分支。
            var st = x.status
            if (st === 0 && url.indexOf("file:") === 0 && x.responseText) st = 200
            if (cb) cb(st, r)
        }
        x.open(method, url)
        var hasBody = body !== null && body !== undefined
        if (hasBody) x.setRequestHeader("Content-Type", "application/json")
        x.send(hasBody ? JSON.stringify(body) : null)
    }

    function reload() {
        listError = ""
        request("GET", baseUrl + "/space/apps", null, function(st, r) {
            if (st !== 200 || !r) { listError = "后端未响应 (" + st + ")"; return }
            // 先设开发模式与时区: apps 一赋值就会触发小组件挂载, 那时 devMode 必须已经就位,
            // 否则第一次加载不带免缓存后缀, Qt 会把旧版编译结果缓存住, 之后改 QML 都不生效
            if (r.tzOffset !== undefined) tzOffset = r.tzOffset
            devMode = !!r.dev
            apps = r.apps || []
        })
        loadRecent()
    }

    // 开发模式下给 file:// 加查询串绕开缓存。令牌用的是应用目录的最新改动时间 (后端的 rev),
    // 不是 Date.now() —— 用时钟的话每次打开都是新 URL, 引擎的类型缓存和磁盘缓存同时落空,
    // 每开一次就把整个 main.qml 重编一遍 (RMPPM 上实测 135KB 要 5.2 秒, 占打开耗时的 94%)。
    // 按 rev 之后: 文件没动就一直命中缓存, 一改文件 rev 变了, 下次打开自动重编一次。
    // 只对 file:// 生效: 内置应用的图标是 qrc:/ark/icons/..., 给 qrc URL 加查询串会加载失败
    function bust(url, rev) {
        if (!devMode || !url || url.indexOf("file:") !== 0) return url
        return url + "?t=" + (rev || 0)
    }

    // 最近在读: 后端扫 xochitl 的 .metadata, 60 秒缓存, 这里只管取
    function loadRecent() {
        request("GET", baseUrl + "/space/recent", null, function(st, r) {
            recentDoc = (st === 200 && r && r.doc && r.doc.name) ? r.doc : null
        })
    }

    function loadStore(refresh) {
        storeError = ""
        request("GET", baseUrl + "/space/store" + (refresh ? "?refresh=1" : ""), null, function(st, r) {
            storeLoaded = true
            if (st !== 200 || !r) { storeError = (r && r.error) ? r.error : ("连不上应用商店 (" + st + ")"); storeApps = []; return }
            storeApps = r.apps || []
        })
    }

    function installedVersion(id) {
        for (var i = 0; i < apps.length; i++) if (apps[i].id === id) return apps[i].version || "?"
        return ""
    }

    function installFromStore(item) {
        busyText = "正在安装 " + item.name + "…"
        request("POST", baseUrl + "/space/apps/install-url", { url: item.zip, sha256: item.sha256 || "" }, function(st, r) {
            busyText = ""
            if (st !== 200) { showToast((r && r.error) ? r.error : ("安装失败 (" + st + ")")); return }
            showToast(item.name + " 已安装")
            reload()
        })
    }

    function filterApps(pred) {
        var out = []
        for (var i = 0; i < apps.length; i++) if (pred(apps[i])) out.push(apps[i])
        return out
    }
    // 应用有没有首页小组件 —— 明文发布看 widgetUrl, 编译发布看 module.widget。
    // 四处筛选都走这里, 免得加了新发布形态就漏一处。
    function hasWidget(a) {
        return !!a && (!!a.widgetUrl || !!(a.module && a.module.widget))
    }

    function heroWidgetApp() {
        var l = filterApps(function(a) { return space.hasWidget(a) && a.widget_size === "hero" && !a.error })
        return l.length ? l[0] : null
    }
    function halfWidgetApps() {
        return filterApps(function(a) { return space.hasWidget(a) && a.widget_size !== "hero" && !a.error })
    }
    // "应用" = 非设置类 (设置类只出现在"设置"标签)
    function isSetting(a) { return a.category === "settings" || a.category === "system" }
    function userApps() { return filterApps(function(a) { return !isSetting(a) }) }
    function homeApps() {
        var l = filterApps(function(a) { return !a.error && !isSetting(a) })
        var n = homeGrid.columns * 2   // 首页只放两行, 其余在"应用"页
        return l.length > n ? l.slice(0, n) : l
    }
    function settingsApps() { return filterApps(isSetting) }
    // 声明了 widget_slot 的小组件由外壳指定位置 (如 "plan" → 今日计划那张卡)
    function slotWidgetApp(slot) {
        var l = filterApps(function(a) { return space.hasWidget(a) && a.widget_slot === slot && !a.error })
        return l.length ? l[0] : null
    }
    // 没指定位置的半宽小组件, 统一进「正在进行」
    function plainWidgetApps() {
        return filterApps(function(a) { return space.hasWidget(a) && a.widget_size !== "hero" && !a.widget_slot && !a.error })
    }

    function openById(id) {
        for (var i = 0; i < apps.length; i++) if (apps[i].id === id) { open(apps[i]); return }
        showToast("没有安装 " + id)
    }

    function open(app) {
        if (app.error) { showToast(app.error); return }
        // 有界面就挂界面, 没有才当外部程序。界面有两种形态: 明文的 entryUrl,
        // 或编译发布的 module.main —— 后者包里没有 .qml, 判断时漏掉它就会被误当成
        // "外部程序", 报"这个应用没有入口"。
        if (!app.entryUrl && !(app.module && app.module.main)) { launch(app); return }
        // 开发模式下先把列表拉一遍再开: 免缓存令牌是应用目录的改动时间, 存在列表里。
        // 不重拉的话, 刚 scp 上去的 QML 用的还是进空间那一刻的旧令牌, URL 没变 → 引擎
        // 直接给缓存里的旧版, 人在设备上退出应用再进也没用, 只能退出整个空间重来。
        if (devMode && !app._fresh) {
            request("GET", baseUrl + "/space/apps", null, function(st, r) {
                if (st === 200 && r && r.apps) {
                    apps = r.apps
                    for (var i = 0; i < apps.length; ++i)
                        if (apps[i].id === app.id) {
                            var a = apps[i]
                            a._fresh = true      // 防止递归
                            space.open(a)
                            return
                        }
                }
                app._fresh = true
                space.open(app)
            })
            return
        }
        if (app.service) {
            busyText = "正在启动 " + app.name + " 的后台…"
            request("POST", baseUrl + "/space/apps/" + app.id + "/service/start", null, function(st, r) {
                busyText = ""
                if (st !== 200) { showToast((r && r.error) ? r.error : ("后台启动失败 (" + st + ")")); return }
                mount(app, (r && r.url) ? r.url : (app.serviceUrl || ""))
            })
            return
        }
        mount(app, "")
    }

    // 卸载当前应用: 销毁界面树 + 按需停后台。应用跳应用 (space.openApp) 与退出应用都走它。
    //
    // 原来只有 closeApp 做销毁, mount 直接覆盖 currentItem —— 旧应用那棵树还挂在
    // appHost 上继续显示 (音乐里点蓝牙, 两屏内容叠在一起), 而且 currentItem 一被覆盖
    // 就再没人能销毁它, 一直泄漏到 xochitl 退出。
    function unmount() {
        if (currentItem) { currentItem.destroy(); currentItem = null }
        var app = current
        current = null
        appError = ""
        api.onBack = null
        // keepalive 的后台不停 (音乐跳去蓝牙连耳机时还在放)
        if (app && app.service && !app.service.keepalive)
            request("POST", baseUrl + "/space/apps/" + app.id + "/service/stop", null, null)
    }

    // 为编译发布的应用造一个 Component。
    //
    // 编译之后包里没有 .qml 文件, createComponent 无从下手, 只能靠 import 模块名。
    // 包一层 Component 而不是直接 createQmlObject 出对象, 是为了和明文那条路接口一致 ——
    // 后面都走 createObject(appHost, {space: api}), 让 space 在对象构造时就位。应用的
    // 属性初始化 (如 dir: "file://" + space.appDir) 依赖这一点, 事后再赋值就晚了。
    function moduleComponent(app) {
        var uri = app.module.uri, type = app.module.main
        // 分步打点: 模块出问题时整个界面会卡住, 卡住前没有任何输出的话根本不知道
        // 停在哪一步 —— 是 import 解析、plugin 加载, 还是实例化。
        console.warn("[space] 模块 " + uri + "." + type + " 开始 import")
        try {
            // 用数组 join 拼多行 QML, 不写字符串里的换行转义 —— 之前那版把转义符写成了
            // 真实换行, JS 字符串字面量跨行是畸形的, 结果 Qt 解析时主线程 100% 空转、
            // 界面全卡死, 连 "import 完成" 都打不出来 (实验 2~5 全中)。
            // 第三个参数是解析这段 QML 的基准 URL, 必须合法; 原来传的 "module:<uri>"
            // 是自造 scheme, 一并换成外壳自己的 URL。
            var src = ['import QtQuick', 'import ' + uri,
                       'Component { ' + type + ' { } }'].join('\n')
            var r = Qt.createQmlObject(src, space, Qt.resolvedUrl("Space.qml"))
            console.warn("[space] 模块 " + uri + " import 完成, Component 已就绪")
            return r
        } catch (e) {
            // 最常见的是模块没装到 QML_IMPORT_PATH 指的目录, 或者 .so 架构不对
            return { status: Component.Error,
                     errorString: function () { return "加载模块 " + uri + " 失败: " + e } }
        }
    }

    function mount(app, serviceUrl) {
        unmount()
        api.appId = app.id
        api.appDir = app.dir
        api.dataDir = app.dataDir
        api.serviceUrl = serviceUrl
        api.onBack = null
        api.title = app.name
        api.chrome = true
        appTitle = app.name
        chrome = true
        appError = ""
        current = app
        // 两种发布形态:
        //   明文: entryUrl 指向应用目录里的 .qml, 直接 createComponent
        //   编译: manifest 声明 module, 包里只有 .so —— 用一段即时 QML 去 import 它。
        //        模块装在固定的 qmlmods 目录, 由 xochitl 的 QML_IMPORT_PATH 指过去。
        var c = app.module && app.module.main
                ? moduleComponent(app)
                : Qt.createComponent(bust(app.entryUrl, app.rev))
        if (c.status === Component.Error) {
            appError = c.errorString()
            console.log("[space] 应用 " + app.id + " 加载失败: " + appError)
            return
        }
        console.warn("[space] " + app.id + " 开始实例化")
        var obj = c.createObject(appHost, { "space": api })
        console.warn("[space] " + app.id + " 实例化返回, space 注入=" + (obj && obj.space ? "成功" : "失败"))
        if (!obj) {
            appError = "createObject 失败: " + c.errorString()
            console.log("[space] 应用 " + app.id + " 创建失败: " + c.errorString())
            return
        }
        currentItem = obj
    }

    function closeApp(closing) {
        unmount()
        chrome = true
        appTitle = ""
        // 小组件可能要反映应用里的变化; 但整个启动台正在关闭时不刷 —— 回调会在外壳销毁后才回来
        if (!closing) reload()
    }

    function back() {
        if (!current) return
        if (api.onBack && api.onBack()) return
        closeApp()
    }

    function close() {
        closeApp(true)
        space.visible = false
        space.destroy()
    }

    // 外部程序型应用 (KOReader / 微信读书 / Android): 没有 QML 界面, 点图标即启动
    function launch(app) {
        var l = app.launch
        if (!l) { showToast("这个应用没有入口"); return }
        if (l.confirm && confirmApp !== app) { confirmApp = app; return }
        confirmApp = null
        if (l.type === "appload") {
            var helper = null
            try {
                helper = Qt.createQmlObject(
                    'import QtQuick;\nimport net.asivery.AppLoad;\n' +
                    'QtObject { function go(id){ AppLoadLauncher.launchApplication(id, [], ({}), false) } }',
                    space, "space_appload_launcher")
            } catch (e) { helper = null }
            if (helper) {
                space.visible = false
                helper.go(l.id)
                helper.destroy()
                close()
                return
            }
            if (!l.path) { showToast("appload 未启用"); return }
        }
        // post (或 appload 兜底): 交给 upload-server, 面板先收起来 (重启类操作后面没有界面了)。
        // 只隐藏不销毁: 请求还在路上, 销毁会把发请求的 JS 上下文一起收掉。
        var x = new XMLHttpRequest()
        x.open("POST", baseUrl + l.path)
        x.send()
        space.visible = false
    }

    // 挂载小组件: 与打开应用同一条路径 (createObject 时父项与初始属性都已就位)
    function mountWidget(host, app) {
        if (host.wItem) { host.wItem.destroy(); host.wItem = null }
        var compiled = app && app.module && app.module.widget
        if (!app || (!app.widgetUrl && !compiled)) return
        var c = compiled
                ? moduleComponent({ module: { uri: app.module.uri, main: app.module.widget } })
                : Qt.createComponent(bust(app.widgetUrl, app.rev))
        if (c.status === Component.Error) { console.log("[space] 小组件 " + app.id + ": " + c.errorString()); return }
        var o = c.createObject(host, { "space": api, "app": app })
        if (o) host.wItem = o
        else console.log("[space] 小组件 " + app.id + " 创建失败: " + c.errorString())
    }

    function showToast(msg) { toastText = msg; toastTimer.restart() }
    function tabTitle() {
        if (tab === "apps") return "应用"
        if (tab === "discover") return "发现"
        if (tab === "settings") return "设置"
        return "空间"
    }
    function dateLine() {
        return localNow().toLocaleDateString(Qt.locale("zh_CN"), "M月d日 dddd")
    }

    Timer { id: toastTimer; interval: 4000; repeat: false; onTriggered: space.toastText = "" }
    Timer { interval: 15000; running: space.visible; repeat: true; onTriggered: space.now = new Date() }

    Component.onCompleted: { reload(); loadShellCfg() }
    onTabChanged: { if (tab === "discover" && !storeLoaded) loadStore(false) }

    // ─── 应用内导航栏 (返回 + 标题) ───────────────────────────────────
    Item {
        id: appNav
        visible: space.chrome && space.current !== null
        anchors.top: parent.top
        anchors.left: parent.left
        anchors.right: parent.right
        height: space.vpx(72)
        IconButton {
            anchors.top: parent.top
            anchors.topMargin: 10
            anchors.left: parent.left
            anchors.leftMargin: 10
            iconSource: "qrc:/ark/icons/chevron_left"
            title: "返回"
            onClicked: space.back()
        }
    }
    Text {
        id: appTitleText
        visible: space.chrome && space.current !== null
        anchors.top: appNav.bottom
        anchors.topMargin: space.vpx(56)
        anchors.left: parent.left
        anchors.leftMargin: space.margin
        text: space.current ? (space.appTitle || space.current.name) : ""
        font.pixelSize: space.fpx(56)
        font.weight: Font.Medium
    }

    // ─── 标签页页眉 (大标题 + 副标题 + 齿轮) ──────────────────────────
    Item {
        id: pageHeader
        visible: space.chrome && space.current === null
        anchors.top: parent.top
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.leftMargin: space.margin
        anchors.rightMargin: space.margin
        // 副标题已去掉 (2026-10-03)。高度 = 标题底 + 24:
        // 标题 y 42 + 标题高 ~78 = 120, 再留 24。省下的 72 给首页大卡。
        height: space.vpx(144)
        // 返回只留一个 "<" 并入标题行: 原来单占一行加"返回"二字, 白吃 70px,
        // 又与整页的克制气质不搭。退出空间在设置里也有一条, 这里是一步直达。
        // < 不是主角: 挪到内容边距之外、贴近屏幕左沿, 把内容左边界让给标题,
        // 让"空间"与下面所有卡片的左边对齐。
        Image {
            id: backChevron
            anchors.left: parent.left
            anchors.leftMargin: space.px(16) - space.margin
            anchors.verticalCenter: pageTitle.verticalCenter
            width: space.fpx(34); height: width
            sourceSize.width: width; sourceSize.height: width
            source: space.kitDir + "/icons/caret-right.svg"
            rotation: 180
            fillMode: Image.PreserveAspectFit
            MouseArea { anchors.fill: parent; anchors.margins: -20; onClicked: space.close() }
        }
        Text {
            id: pageTitle
            x: 0
            y: space.vpx(42)
            text: space.tabTitle()
            font.pixelSize: space.fpx(56)
            font.weight: Font.Medium
        }
        Image {
            visible: space.tab !== "home"
            anchors.right: parent.right
            anchors.verticalCenter: pageTitle.verticalCenter
            width: space.fpx(44); height: width
            source: "qrc:/ark/icons/restore"
            fillMode: Image.PreserveAspectFit
            MouseArea {
                anchors.fill: parent; anchors.margins: -16
                onClicked: { space.reload(); if (space.tab === "discover") space.loadStore(true) }
            }
        }
    }

    // 提示条: 4 秒自动消失, 盖在内容区顶部, 不挤动布局
    Rectangle {
        visible: space.toastText !== ""
        z: 5
        anchors.top: content.top
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.leftMargin: space.margin
        anchors.rightMargin: space.margin
        height: toastLabel.implicitHeight + 24
        color: "#000000"
        radius: 8
        Text {
            id: toastLabel
            anchors.fill: parent
            anchors.margins: 12
            text: space.toastText
            font.pixelSize: space.fpx(24)
            color: "white"
            wrapMode: Text.WordWrap
        }
    }

    // ─── 内容区 ───────────────────────────────────────────────────────
    Item {
        id: content
        anchors.top: !space.chrome ? parent.top : (space.current ? appTitleText.bottom : pageHeader.bottom)
        anchors.topMargin: !space.chrome ? 0 : (space.current ? space.vpx(48) : 0)
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: (space.chrome && space.current === null) ? tabBar.top : parent.bottom
        anchors.leftMargin: space.chrome ? space.margin : 0
        anchors.rightMargin: space.chrome ? space.margin : 0
        anchors.bottomMargin: space.chrome ? space.vpx(24) : 0

        // ── 空间 (首页) ──
        Flickable {
            visible: space.current === null && space.tab === "home"
            anchors.fill: parent
            contentWidth: width
            contentHeight: homeCol.height + space.vpx(24)   // 底部留白与块间距一致
            clip: true
            ColumnLayout {
                id: homeCol
                width: parent.width
                spacing: space.vpx(24)

                // 顶部大卡: 日期 + 时间; 装了 hero 小组件 (如天气) 则由它整卡接管
                Rectangle {
                    Layout.fillWidth: true
                    Layout.preferredHeight: space.vpx(562)   // 副标题省下的高度匀给插图
                    radius: 12
                    border.color: "#B4AFA6"
                    border.width: 1
                    color: "white"
                    clip: true
                    Item {
                        anchors.fill: parent
                        visible: space.heroWidgetApp() === null
                        Column {
                            anchors.left: parent.left
                            anchors.leftMargin: space.px(32)
                            anchors.verticalCenter: parent.verticalCenter
                            spacing: 8
                            Text { text: space.dateLine(); font.pixelSize: space.fpx(26); color: "#262626" }
                            Text { text: Qt.formatTime(space.localNow(), "HH:mm"); font.pixelSize: space.fpx(88); font.weight: Font.Light }
                            Text { text: "今天也是安静的一天。"; font.pixelSize: space.fpx(22); color: "#26251F" }
                        }
                        Rectangle {
                            anchors.right: parent.right
                            anchors.rightMargin: space.px(40)
                            anchors.verticalCenter: parent.verticalCenter
                            width: space.px(120); height: width; radius: width / 2
                            color: "#e4e4e4"
                        }
                    }
                    Item {
                        id: heroHost
                        anchors.fill: parent
                        property var wItem: null
                        property var heroApp: space.heroWidgetApp()
                        onHeroAppChanged: space.mountWidget(heroHost, heroApp)
                        Component.onCompleted: space.mountWidget(heroHost, heroApp)
                    }
                }

                // ── 今日计划 / 正在进行: 左右两张卡 (照概念图) ────────────────
                // 左卡交给声明了 widget_slot: "plan" 的应用; 没装就显示引导。
                // 右卡是外壳自带: 最近在读 (/space/recent, 读 xochitl 元数据) + 其余半宽小组件。
                GridLayout {
                    Layout.fillWidth: true
                    columns: 2
                    columnSpacing: space.px(24)
                    rowSpacing: space.vpx(24)

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.preferredHeight: space.vpx(350)
                        radius: 12; border.color: "#B4AFA6"; border.width: 1; color: "white"; clip: true
                        readonly property var planApp: space.slotWidgetApp("plan")

                        Column {
                            anchors.fill: parent; anchors.margins: space.px(22); spacing: space.vpx(12)
                            visible: parent.planApp === null
                            Text { text: "今日计划"; font.pixelSize: space.fpx(28); font.weight: Font.Medium; color: "#000000" }
                            Text {
                                width: parent.width
                                text: "还没装计划应用。到「发现」里安装后，今天要做的事会显示在这里。"
                                font.pixelSize: space.fpx(22); color: "#1F1E1B"; wrapMode: Text.WordWrap
                            }
                        }
                        Item {
                            id: planHost
                            anchors.fill: parent
                            property var wItem: null
                            visible: parent.planApp !== null
                            onVisibleChanged: if (visible) space.mountWidget(planHost, parent.planApp)
                            Component.onCompleted: if (parent.planApp) space.mountWidget(planHost, parent.planApp)
                        }
                        MouseArea {
                            anchors.fill: parent
                            visible: parent.planApp === null
                            onClicked: space.tab = "discover"
                        }
                    }

                    Rectangle {
                        Layout.fillWidth: true
                        Layout.preferredHeight: space.vpx(350)
                        radius: 12; border.color: "#B4AFA6"; border.width: 1; color: "white"; clip: true
                        readonly property bool hasRecent: space.recentDoc !== null
                        readonly property bool hasHalf: space.plainWidgetApps().length > 0

                        Column {
                            anchors.fill: parent; anchors.margins: space.px(22); spacing: space.vpx(10)
                            Text { text: "正在进行"; font.pixelSize: space.fpx(28); font.weight: Font.Medium; color: "#000000" }

                            Item {
                                width: parent.width
                                height: space.vpx(92)
                                visible: parent.parent.hasRecent
                                Image {
                                    id: bookIcon
                                    anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter
                                    width: space.fpx(44); height: width
                                    sourceSize.width: width; sourceSize.height: width
                                    source: space.kitDir + "/icons/book-open.svg"
                                    fillMode: Image.PreserveAspectFit
                                }
                                Column {
                                    anchors.left: bookIcon.right; anchors.leftMargin: space.px(16)
                                    anchors.right: parent.right
                                    anchors.verticalCenter: parent.verticalCenter
                                    spacing: space.vpx(8)
                                    Text {
                                        width: parent.width
                                        text: space.recentDoc ? space.recentDoc.name : ""
                                        font.pixelSize: space.fpx(25); color: "#000000"; elide: Text.ElideRight
                                    }
                                    Text {
                                        text: space.recentDoc && space.recentDoc.pages > 0
                                              ? ("阅读进度 " + space.recentDoc.percent + "%")
                                              : (space.recentDoc ? ("第 " + space.recentDoc.page + " 页") : "")
                                        font.pixelSize: space.fpx(21); color: "#1F1E1B"
                                    }
                                    Rectangle {
                                        width: parent.width; height: 4; radius: 2; color: "#C4BFB6"
                                        visible: !!space.recentDoc && space.recentDoc.pages > 0
                                        Rectangle {
                                            anchors.left: parent.left; anchors.top: parent.top; anchors.bottom: parent.bottom
                                            width: Math.max(4, parent.width * (space.recentDoc ? space.recentDoc.percent : 0) / 100)
                                            radius: 2; color: "#000000"
                                        }
                                    }
                                }
                                MouseArea { anchors.fill: parent; onClicked: space.exit() }
                            }

                            Rectangle {
                                width: parent.width; height: 1; color: "#C4BFB6"
                                visible: parent.parent.hasRecent && parent.parent.hasHalf
                            }

                            Repeater {
                                model: space.plainWidgetApps()
                                delegate: Item {
                                    required property var modelData
                                    width: parent.width
                                    height: space.vpx(92)
                                    property var wItem: null
                                    Component.onCompleted: space.mountWidget(this, modelData)
                                }
                            }
                        }
                    }
                }

                RowLayout {
                    Layout.fillWidth: true
                    Text { text: "我的应用"; font.pixelSize: space.fpx(32); font.weight: Font.Medium }
                    Item { Layout.fillWidth: true }
                    Text {
                        text: "全部应用 ›"
                        font.pixelSize: space.fpx(24)
                        color: "#262626"
                        MouseArea { anchors.fill: parent; anchors.margins: -12; onClicked: space.tab = "apps" }
                    }
                }
                Text {
                    visible: space.listError !== ""
                    text: space.listError
                    font.pixelSize: space.fpx(24)
                    color: "#a00000"
                    wrapMode: Text.WordWrap
                    Layout.fillWidth: true
                }
                Text {
                    visible: space.listError === "" && space.apps.length === 0
                    text: "还没有应用。到「发现」安装，或用手机扫码上传应用包 (zip)。"
                    font.pixelSize: space.fpx(24)
                    color: "#262626"
                    wrapMode: Text.WordWrap
                    Layout.fillWidth: true
                }
                GridLayout {
                    id: homeGrid
                    Layout.fillWidth: true
                    columns: space.cols(width, 200)
                    rowSpacing: space.vpx(24)
                    columnSpacing: space.px(24)
                    Repeater {
                        model: space.homeApps()
                        delegate: Kit.STile {
                            required property var modelData
                            Layout.fillWidth: true
                            Layout.preferredHeight: space.vpx(170)
                            iconSize: space.fpx(62)
                            labelSize: space.fpx(26)
                            iconSource: space.bust(modelData.iconUrl || "", modelData.rev)
                            label: modelData.name
                            onClicked: space.open(modelData)
                        }
                    }
                }
            }
        }

        // ── 应用 (全部) ──
        Flickable {
            visible: space.current === null && space.tab === "apps"
            anchors.fill: parent
            contentWidth: width
            contentHeight: appsGrid.height + space.vpx(24)
            clip: true
            GridLayout {
                id: appsGrid
                width: parent.width
                columns: space.cols(width, 270)
                rowSpacing: space.vpx(24)
                columnSpacing: space.px(24)
                Repeater {
                    model: space.userApps()
                    delegate: Kit.STile {
                        required property var modelData
                        Layout.fillWidth: true
                        Layout.preferredHeight: space.vpx(220)
                        iconSize: space.fpx(76)
                        labelSize: space.fpx(28)
                        iconSource: space.bust(modelData.iconUrl || "", modelData.rev)
                        label: modelData.name
                        dimmed: !!modelData.error
                        onClicked: space.open(modelData)
                    }
                }
            }
        }

        // ── 发现 (应用商店) ──
        Flickable {
            visible: space.current === null && space.tab === "discover"
            anchors.fill: parent
            contentWidth: width
            contentHeight: storeCol.height + space.vpx(24)
            clip: true
            ColumnLayout {
                id: storeCol
                width: parent.width
                spacing: 0
                Rectangle {
                    Layout.fillWidth: true
                    Layout.preferredHeight: space.vpx(150)
                    Layout.bottomMargin: space.vpx(24)
                    radius: 12
                    color: "#f0f0f0"
                    Column {
                        anchors.centerIn: parent
                        spacing: 8
                        Text { anchors.horizontalCenter: parent.horizontalCenter; text: "让好工具"; font.pixelSize: space.fpx(34); font.weight: Font.Medium }
                        Text { anchors.horizontalCenter: parent.horizontalCenter; text: "陪伴更好的你"; font.pixelSize: space.fpx(24); color: "#1A1A18" }
                    }
                }
                Text {
                    visible: !space.storeLoaded
                    text: "正在获取应用列表…"
                    font.pixelSize: space.fpx(24); color: "#1A1A18"
                }
                Text {
                    visible: space.storeError !== ""
                    Layout.fillWidth: true
                    text: space.storeError + "\n应用商店需要联网。也可以用手机扫码上传应用包 (zip) 安装。"
                    font.pixelSize: space.fpx(24); color: "#1A1A18"; wrapMode: Text.WordWrap
                }
                Text {
                    visible: space.storeLoaded && space.storeError === "" && space.storeApps.length === 0
                    text: "商店里还没有应用。"
                    font.pixelSize: space.fpx(24); color: "#1A1A18"
                }
                Repeater {
                    model: space.storeApps
                    delegate: Item {
                        id: storeRow
                        required property var modelData
                        readonly property string installed: space.installedVersion(modelData.id)
                        Layout.fillWidth: true
                        Layout.preferredHeight: space.vpx(120)
                        RowLayout {
                            anchors.fill: parent
                            spacing: space.px(20)
                            Rectangle {
                                Layout.preferredWidth: space.fpx(72); Layout.preferredHeight: space.fpx(72); radius: 12
                                color: "#f0f0f0"
                                Text { anchors.centerIn: parent; text: (storeRow.modelData.name || "?").charAt(0); font.pixelSize: space.fpx(32) }
                            }
                            ColumnLayout {
                                Layout.fillWidth: true
                                spacing: 4
                                Text { text: storeRow.modelData.name || storeRow.modelData.id; font.pixelSize: space.fpx(28); elide: Text.ElideRight; Layout.fillWidth: true }
                                Text {
                                    text: (storeRow.modelData.description || "") + "  v" + (storeRow.modelData.version || "")
                                    font.pixelSize: space.fpx(22); color: "#26251F"; elide: Text.ElideRight; Layout.fillWidth: true
                                }
                            }
                            Kit.SButton {
                                ui: space.fs
                                enabled: storeRow.installed !== storeRow.modelData.version
                                text: storeRow.installed === "" ? "获取" : (storeRow.installed === storeRow.modelData.version ? "已安装" : "更新")
                                primary: storeRow.installed === ""
                                onClicked: space.installFromStore(storeRow.modelData)
                            }
                        }
                        Rectangle { anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom; height: 1; color: "#C4BFB6" }
                    }
                }
            }
        }

        // ── 设置 (设置类应用的行列表) ──
        Flickable {
            visible: space.current === null && space.tab === "settings"
            anchors.fill: parent
            contentWidth: width
            contentHeight: mineCol.height + space.vpx(24)
            clip: true
            ColumnLayout {
                id: mineCol
                width: parent.width
                spacing: 0
                Repeater {
                    model: space.settingsApps()
                    delegate: Kit.SRow {
                        required property var modelData
                        Layout.fillWidth: true
                        ui: space.fs
                        iconSource: space.bust(modelData.iconUrl || "", modelData.rev)
                        text: modelData.name
                        onClicked: space.open(modelData)
                    }
                }
                Kit.SRow {
                    Layout.fillWidth: true
                    ui: space.fs
                    iconSource: space.kitDir + "/icons/sun.svg"
                    text: "刷新效果"
                    detail: space.screenMode === "content" ? "高清" : "快刷"
                    onClicked: {
                        space.screenMode = (space.screenMode === "content") ? "ui" : "content"
                        space.saveShellCfg()
                    }
                }
                Text {
                    Layout.fillWidth: true
                    Layout.leftMargin: 16
                    Layout.rightMargin: 16
                    Layout.topMargin: space.vpx(10)
                    Layout.bottomMargin: space.vpx(16)
                    text: "高清用文档那档波形，插画和灰阶更细腻，代价是每次操作慢一拍、进页面会留一层淡残影；"
                          + "快刷与系统界面同档，干脆利落。整个空间（含所有应用）统一生效。"
                    font.pixelSize: space.fpx(21)
                    color: "#26251F"
                    wrapMode: Text.WordWrap
                }
                Kit.SRow {
                    Layout.fillWidth: true
                    ui: space.fs
                    iconSource: space.kitDir + "/icons/squares-four.svg"
                    text: "关于空间"
                    detail: "外壳 v" + space.shellVersion
                    showArrow: false
                    onClicked: space.showToast("空间 (SPACE) 外壳契约 v" + space.shellVersion + "，应用 " + space.apps.length + " 个")
                }
            }
        }

        // ── 应用宿主: 应用根 Item 会以它为父, 自己 anchors.fill: parent ──
        Item {
            id: appHost
            anchors.fill: parent
            visible: space.current !== null
        }
        Text {
            visible: space.appError !== ""
            anchors.fill: parent
            text: "应用加载失败：\n" + space.appError
            font.pixelSize: space.fpx(24)
            color: "#a00000"
            wrapMode: Text.WrapAnywhere
        }
    }

    // ─── 底部标签栏 ───────────────────────────────────────────────────
    Rectangle {
        id: tabBar
        visible: space.chrome && space.current === null
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.bottom: parent.bottom
        height: space.vpx(120)
        color: "white"
        Rectangle { anchors.top: parent.top; anchors.left: parent.left; anchors.right: parent.right; height: 1; color: "#B4AFA6" }
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: space.margin
            anchors.rightMargin: space.margin
            spacing: 0
            Repeater {
                model: [
                    { key: "home", label: "空间", icon: "house" },
                    { key: "apps", label: "应用", icon: "squares-four" },
                    { key: "discover", label: "发现", icon: "compass" },
                    { key: "settings", label: "设置", icon: "gear" }
                ]
                delegate: Item {
                    id: tabItem
                    required property var modelData
                    readonly property bool active: space.tab === modelData.key
                    Layout.fillWidth: true
                    Layout.fillHeight: true
                    Column {
                        anchors.centerIn: parent
                        spacing: 6
                        Image {
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: space.fpx(40); height: width
                            source: space.kitDir + "/icons/" + tabItem.modelData.icon + ".svg"
                            fillMode: Image.PreserveAspectFit
                            sourceSize.width: width; sourceSize.height: width
                            opacity: tabItem.active ? 1 : 0.55
                        }
                        Text {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: tabItem.modelData.label
                            font.pixelSize: space.fpx(22)
                            font.weight: tabItem.active ? Font.Medium : Font.Normal
                            color: tabItem.active ? "#000000" : "#26251F"
                        }
                        Rectangle {
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: 28; height: 3
                            color: tabItem.active ? "#000000" : "transparent"
                        }
                    }
                    MouseArea {
                        anchors.fill: parent
                        onClicked: space.tab = tabItem.modelData.key
                    }
                }
            }
        }
    }

    // ─── 遮罩: 后台启动中 / 安装中 ─────────────────────────────────────
    Rectangle {
        visible: space.busyText !== ""
        anchors.fill: parent
        z: 10
        color: "#ccffffff"
        MouseArea { anchors.fill: parent }
        Text {
            anchors.centerIn: parent
            text: space.busyText
            font.pixelSize: space.fpx(30)
        }
    }

    // ─── 确认框 (重启进 Android 之类) ────────────────────────────────────
    Rectangle {
        visible: space.confirmApp !== null
        anchors.fill: parent
        z: 10
        color: "#88000000"
        MouseArea { anchors.fill: parent }
        Rectangle {
            anchors.centerIn: parent
            width: Math.min(parent.width - space.px(160), space.px(900))
            height: confirmCol.implicitHeight + space.vpx(96)
            color: "white"
            radius: 12
            ColumnLayout {
                id: confirmCol
                anchors.centerIn: parent
                width: parent.width - 96
                spacing: 40
                Text {
                    Layout.fillWidth: true
                    text: space.confirmApp ? space.confirmApp.launch.confirm : ""
                    font.pixelSize: space.fpx(30)
                    wrapMode: Text.WordWrap
                }
                RowLayout {
                    Layout.alignment: Qt.AlignRight
                    spacing: 24
                    Kit.SButton { ui: space.fs; text: "取消"; onClicked: space.confirmApp = null }
                    Kit.SButton { ui: space.fs; text: "确定"; primary: true; onClicked: space.launch(space.confirmApp) }
                }
            }
        }
    }
}
