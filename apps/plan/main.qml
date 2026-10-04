// 计划 — 「空间」第三方应用。整屏接管 (space.chrome = false), 自带顶栏与底部标签。
// 版式严格照概念图: 今天 / 我的计划 / 日历 / 收集箱 / 设置。
// 概念图里每个页头与每条目都有水墨插画: 这里都留了位置 (assets/header.jpg 与条目的
// illus 字段), 素材没到位时什么都不画, 放进去即生效, 不摆占位方块。
// 数据全部离线, 走外壳的 dataGet/dataPut 存在设备上。
// 尺寸不写死: 一律走 space.unit (版式) 与 space.fontScale (字号), Move 与 Paper Pro 通用。
import QtQuick
import QtQuick.Layouts
import "plan.js" as P

Item {
    id: root
    property double _t0: Date.now()   // 建树开始 (属性按声明顺序初始化, 这是最早的一刻)
    property var space
    anchors.fill: parent

    readonly property string dir: "file://" + space.appDir + "/assets/"
    // 图片按 URL 缓存解码结果: 同名换内容不会重新读盘, 换了插画/图标得重启 xochitl 才看得见。
    // 所以资源 URL 一律走这里带上外壳的 rev 令牌 (文件没动就照常命中缓存, 动了自动失效)。
    function img(path) {
        var u = root.dir + path
        return (space && space.bust) ? space.bust(u) : u
    }
    readonly property real fs: space.fontScale
    readonly property real un: space.unit
    function f(n) { return Math.round(n * root.fs) }
    function u(n) { return Math.round(n * root.un) }

    // 彩色墨水屏: 实心小面积用彩色 (进度条/圆点/选中), 大面积仍是纯白,
    // 文字一律纯黑, 次要文字只比正文浅一点点。
    // 底一律纯白 (墨水屏铺大面积浅色要靠抖动, 近看发灰发脏), 颜色只用在元素上:
    // 进度条、圆点、选中态、分类标签 —— 这些是实心小面积, 彩屏上能真的显出颜色。
    readonly property color paper:  "#FFFFFF"
    readonly property color card:   "#FFFFFF"
    readonly property color ink:    "#000000"
    readonly property color ink2:   "#1F1E1B"
    readonly property color line:   "#CCCCC9"
    // 配色取概念图的"色相", 但彩度必须比概念图高一大截 ——
    // 概念图是普通屏上的 sRGB 渲染图, 那套 10~20% 彩度的淡色 (#CFD6E1 / #E6D5CD / #81998C)
    // 在 Gallery 3 面板上会被直接抖成灰, 看不出是颜色。实测结论: 色相照抄, 彩度拉到
    // 50% 上下、明度压到中间档, 面板才真的出色。别再照着概念图的 RGB 直接填。
    // 主色 #8FB46A 是在设备上实机比过九档选出来的 —— Gallery 3 的绿是整块屏最弱的一环,
    // 实测绿笔只到 #6e7860, 对白 (#a8aaa7) 归一化约 #A6B492, 比这更暗的绿只能掺黑墨,
    // 所以 #357F52 那种 35% 亮度的看着就是黑的。这支亮度 70%、偏黄, 屏上才真的是绿。
    // 代价: 白字压在它上面对比只有 2:1, 所以实心块上的文字和勾一律用黑。
    readonly property color accent: "#8FB46A"
    readonly property color barFill: "#8FB46A"  // 进度条实心用同一支绿
    readonly property color barTrack: "#D8D7D2" // 槽本来就该是灰, 保持概念图的值
    readonly property color tabSel: "#C9E0AE"   // 底部选中页签的底: 主色再淡一档

    readonly property var catPalette: ({
        work:  { tint: "#AFC9EC", accent: "#2F6BC4" },
        life:  { tint: "#EFBCA9", accent: "#C2502E" },
        hobby: { tint: "#C9B5EC", accent: "#7540C0" }
    })
    function tintOf(k)   { return root.catPalette[k] ? root.catPalette[k].tint   : "#C9E0AE" }
    function accentOf(k) { return root.catPalette[k] ? root.catPalette[k].accent : root.accent }

    // 收集箱的四种种类色。原表在 plan.js 里, 但 .js 改了要重启 xochitl, 所以在这里覆盖。
    readonly property var kindPalette: ({
        idea:  { tint: "#EFD98A", accent: "#8A6A00" },
        todo:  { tint: "#EFBCA9", accent: "#C2502E" },
        file:  { tint: "#AFC9EC", accent: "#2F6BC4" },
        spark: { tint: "#C9B5EC", accent: "#7540C0" }
    })
    function kindTint(k)   { return root.kindPalette[k] ? root.kindPalette[k].tint   : root.tintOf("") }
    function kindAccent(k) { return root.kindPalette[k] ? root.kindPalette[k].accent : root.accent }

    // 条目没自己挑配图时按分类给一张, 概念图里每条都有画。
    // illus === "none" 是显式"不要", 和"没挑过"区分开。
    // 每个分类给几张候选, 按 id 散开 —— 概念图里每条都是不同的画, 一个分类只配一张
    // 会让同类条目看着像复制粘贴。挑中哪张只跟 id 有关, 所以同一条每次都一样。
    readonly property var catIllus: ({
        work:  ["illus/laptop.jpg", "illus/clipboard.jpg"],
        life:  ["illus/books.jpg", "illus/trees.jpg", "illus/house.jpg"],
        hobby: ["illus/music.jpg", "illus/guitar.jpg", "illus/headphones.jpg"]
    })
    function illusOf(it) {
        if (!it) return ""
        if (it.illus === "none") return ""
        if (it.illus) return it.illus
        var l = root.catIllus[it.cat]
        if (!l || !l.length) return ""
        var id = String(it.id || ""), h = 0
        for (var i = 0; i < id.length; ++i) h = (h + id.charCodeAt(i) * (i + 1)) % 9973
        return l[h % l.length]
    }


    // ═══ 内联组件 ══════════════════════════════════════════════════
    // 原来放在 ui/*.qml。相对 import 解析时不带 ?t= 令牌, 改一次样式就得重启 xochitl,
    // 调版式时太难受, 所以搬进来 —— 内联组件跟着 main.qml 一起走 rev 令牌。
    // 注意: 内联组件访问不到外层的 id (root), 所以它们必须自带全部参数。

    // Chip — 概念图里那种小圆角标签 / 筛选片。概念图里标签没有描边, 一律淡色底深色字;
    // 选中不反白 (墨水屏上黑块太抢眼), 只加一圈主色描边并把字加粗。
    component Chip: Rectangle {
        id: chip
        property string text: ""
        property color tint: "#CCD6CE"
        property color ink: "#000000"
        property bool selected: false
        property color accent: "#8FB46A"
        property real fs: 24
        property real pad: 30
        signal clicked()

        width: label.width + pad
        height: Math.round(fs * 2.1)
        radius: Math.round(height * 0.26)
        color: tint
        border.color: selected ? accent : "transparent"
        border.width: selected ? 2 : 0

        Text {
            id: label
            anchors.centerIn: parent
            text: chip.text
            font.pixelSize: chip.fs
            font.weight: chip.selected ? Font.Bold : Font.Normal
            color: chip.ink
        }
        MouseArea { anchors.fill: parent; onClicked: chip.clicked() }
    }

    // Bar — 进度条 (槽 + 实心)
    component Bar: Rectangle {
        id: bar
        property int value: 0            // 0-100
        property color fill: "#8FB46A"
        height: 12
        radius: height / 2
        color: "#D8D7D2"                 // 槽色取自概念图
        Rectangle {
            anchors.left: parent.left; anchors.top: parent.top; anchors.bottom: parent.bottom
            width: Math.max(parent.height, parent.width * Math.max(0, Math.min(100, bar.value)) / 100)
            radius: parent.radius
            color: bar.fill
        }
    }

    // Rule — 一条细分隔线
    component Rule: Rectangle { height: 1; color: "#CCCCC9" }

    // Thumb — 条目缩略图, 没素材就什么都不画 (留白), 不摆占位方块
    component Thumb: Item {
        id: t
        property string source: ""
        property real radius: 10        // 留着不用: 现在不裁切也就没有圆角可切
        // PreserveAspectFit 而不是 Crop —— 插画是按自然边界从概念图裁出来的,
        // 长宽比各不相同 (192x92 到 130x138 都有), 再按框裁一次会把内容切掉。
        Image {
            anchors.fill: parent
            source: t.source
            fillMode: Image.PreserveAspectFit
            sourceSize.width: 320; sourceSize.height: 320
            visible: status === Image.Ready
        }
    }

    // Segment — 分段控件 (日历的 月/周/日, 计划的 进行中/准备开始/已完成 共用)
    component Segment: Rectangle {
        id: seg
        property var items: []            // [{ k, n }, …]
        property string current: ""
        property color accent: "#8FB46A"
        property color ink: "#000000"
        property color line: "#CCCCC9"
        property real fs: 27
        signal picked(string key)

        implicitHeight: Math.round(fs * 2.3)
        radius: Math.round(height * 0.3)
        color: "transparent"
        border.color: line; border.width: 1

        Row {
            anchors.fill: parent
            Repeater {
                model: seg.items
                delegate: Rectangle {
                    id: cell
                    required property var modelData
                    readonly property bool sel: seg.current === modelData.k
                    width: seg.width / Math.max(1, seg.items.length)
                    height: seg.height
                    radius: seg.radius
                    color: cell.sel ? seg.accent : "transparent"
                    Text {
                        anchors.centerIn: parent
                        text: cell.modelData.n
                        font.pixelSize: seg.fs
                        font.weight: cell.sel ? Font.Bold : Font.Normal
                        color: seg.ink    // 内联组件访问不到外层 root; 选中态也用黑字 (底是浅绿)
                    }
                    MouseArea { anchors.fill: parent; onClicked: seg.picked(cell.modelData.k) }
                }
            }
        }
    }

    // 条目配图。素材是从概念图里裁出来的那套水彩, 放在 assets/illus/。
    // 清单写在这里而不是 plan.js: 相对 import 解析时不带 ?t= 令牌, .js 改了得重启 xochitl。
    readonly property var illusList: [
        { k: "",                     n: "按分类" },
        { k: "none",                 n: "不要" },
        { k: "illus/trees.jpg",      n: "树林" },
        { k: "illus/laptop.jpg",     n: "电脑" },
        { k: "illus/clipboard.jpg",  n: "图板" },
        { k: "illus/books.jpg",      n: "书" },
        { k: "illus/music.jpg",      n: "音乐" },
        { k: "illus/ginkgo.jpg",     n: "银杏" },
        { k: "illus/mountain.jpg",   n: "山水" },
        { k: "illus/lake.jpg",       n: "湖山" },
        { k: "illus/house.jpg",      n: "屋舍" },
        { k: "illus/headphones.jpg", n: "耳机" },
        { k: "illus/guitar.jpg",     n: "吉他" }
    ]

    property string tab: "today"
    property var tasks: []
    property var inbox: []
    property var cfg: ({ weekStart: 1, defaultView: "today" })
    property bool ready: false
    property date now: space ? space.localNow() : new Date()
    property string today: P.ymd(now)
    property string pickDate: P.ymd(now)
    property int calYear: now.getFullYear()
    property int calMonth: now.getMonth()
    property string calMode: "month"       // month / week / day
    property string todayCat: ""
    property string inboxKind: ""

    // 刷新波形档不在这里设: 「空间」外壳整屏挂了一个常驻 ScreenModeItem,
    // 应用跑在 appHost 里自然继承 (设置 → 刷新效果: 高清 / 快刷)。
    // 应用自己再挂一个会跟外壳那个叠在一起, 后挂的不一定赢。

    Timer { interval: 30000; repeat: true; running: root.visible; onTriggered: root.now = space.localNow() }

    // ═══ 数据 ═══════════════════════════════════════════════════════
    function load() {
        space.dataGet(space.appId, "data", function (st, r) {
            if (!root) return
            if (st === 200 && r && r.tasks) {
                root.tasks = r.tasks || []
                root.inbox = r.inbox || []
                root.cfg = r.cfg || root.cfg
            } else {
                var sd = P.seed(root.today)
                root.tasks = sd.tasks; root.inbox = sd.inbox
                root.save()
            }
            if (root.cfg.defaultView) root.tab = root.cfg.defaultView
            root.ready = true
            console.warn("[plan] 数据到位 " + (Date.now() - root._t0) + "ms")
        })
    }
    function save() {
        space.dataPut(space.appId, "data",
                      { tasks: root.tasks, inbox: root.inbox, cfg: root.cfg },
                      function (st) {
                          if (st !== 200) {
                              console.warn("[plan] 保存失败 status=" + st)
                              space.toast("保存失败 (" + st + ")")
                          }
                      })
    }
    // QML 的 property var 读出来是浅拷贝: 改元素 (打勾) 能生效, 因为元素对象共享;
    // 但 push/unshift/splice 改的是那份临时拷贝, 写完就丢。所以增删一律
    // "取副本 → 改副本 → 整个赋回去", 别在 root.xxx 上原地改长度。
    function bump(name) { root[name] = root[name].slice() }
    function setCfg(k, v) { var c = root.cfg; c[k] = v; root.cfg = c; root.save() }

    function toggleTask(id) {
        var l = root.tasks.slice()
        for (var i = 0; i < l.length; ++i)
            if (l[i].id === id) { l[i].done = !l[i].done; break }
        root.tasks = l; root.save()
    }
    function toggleInbox(id) {
        var l = root.inbox.slice()
        for (var i = 0; i < l.length; ++i)
            if (l[i].id === id) { l[i].done = !l[i].done; break }
        root.inbox = l; root.save()
    }
    function removeFrom(name, id) {
        var l = root[name], out = []
        for (var i = 0; i < l.length; ++i) if (l[i].id !== id) out.push(l[i])
        root[name] = out; root.save()
    }
    function addTask(title, time, cat, date) {
        var l = root.tasks.slice()
        l.push({ id: P.newId("t"), date: date || root.pickDate, time: time || "",
                 title: title, note: "", cat: cat || "work", illus: "", done: false })
        root.tasks = l; root.save()
    }
    function addInbox(text, kind) {
        var l = root.inbox.slice()
        l.unshift({ id: P.newId("i"), kind: kind || "idea", text: text,
                    at: Qt.formatDateTime(space.localNow(), "MM月dd日 HH:mm"), done: false })
        root.inbox = l; root.save()
    }
    // 改一条: 同样走副本, 并且连元素对象一起换新的, 免得外面还拿着旧引用
    function patchIn(name, id, patch) {
        var l = root[name].slice()
        for (var i = 0; i < l.length; ++i)
            if (l[i].id === id) {
                var o = {}, k
                for (k in l[i]) o[k] = l[i][k]
                for (k in patch) o[k] = patch[k]
                l[i] = o
                break
            }
        root[name] = l; root.save()
    }
    function updateTask(id, patch) { root.patchIn("tasks", id, patch) }
    function updateInbox(id, patch) { root.patchIn("inbox", id, patch) }

    // inputMask "99:99" 的输入框, text 在空着时返回的是 ":" 而不是 "  :  "
    // (占位空格被剥掉), 填一半是 "8 :" 这类。所以不比字符串, 一律看能不能
    // 解析成 HH:MM —— 解析不出来就当没填, 免得一个光秃秃的冒号被存进去。
    function normTime(v) {
        var m = /(\d{1,2}):(\d{2})/.exec(v || "")
        if (!m) return ""
        var h = parseInt(m[1], 10), mi = parseInt(m[2], 10)
        if (isNaN(h) || isNaN(mi) || h > 23 || mi > 59) return ""
        return P.pad(h) + ":" + P.pad(mi)
    }

    // 收起虚拟键盘。不编辑的时候它一直杵在那儿, 是因为输入框还抓着焦点 ——
    // 光调 hide() 不摘焦点, 下一拍又会被顶出来。
    // 另: Qt.inputMethod.hide() 即使键盘没显示也会触发一次 IM 状态变化,
    // 会把 xochitl 的主工具栏复位成显示, 所以必须先判 visible (老坑)。
    function dropFocus() {
        root.forceActiveFocus()
        if (Qt.inputMethod.visible) Qt.inputMethod.hide()
    }

    // "2026-10-01" 前后挪 n 天
    function shiftDate(ds, n) {
        var d = new Date(ds.replace(/-/g, "/"))
        d.setDate(d.getDate() + n)
        return P.ymd(d)
    }
    // 日历翻页: 月视图翻月, 周视图翻周, 日视图翻天 —— 原来三种模式下
    // 箭头都只改月份, 周/日视图点了跟没点一样。
    function calStep(n) {
        if (root.calMode === "month") {
            var m = root.calMonth + n, y = root.calYear
            while (m < 0) { m += 12; y-- }
            while (m > 11) { m -= 12; y++ }
            root.calYear = y; root.calMonth = m
            var d = parseInt(root.pickDate.substring(8), 10)
            root.pickDate = y + "-" + P.pad(m + 1) + "-" + P.pad(Math.min(d, P.daysInMonth(y, m)))
        } else {
            root.pickDate = root.shiftDate(root.pickDate, root.calMode === "week" ? 7 * n : n)
            root.syncCalTo(root.pickDate)
        }
    }
    function syncCalTo(ds) {
        root.calYear = parseInt(ds.substring(0, 4), 10)
        root.calMonth = parseInt(ds.substring(5, 7), 10) - 1
    }
    function goToday() {
        root.pickDate = root.today
        root.syncCalTo(root.today)
    }

    Component.onCompleted: {
        console.warn("[plan] 建树完成 " + (Date.now() - root._t0) + "ms")
        space.chrome = false
        space.onBack = function () {
            if (confirmBox.visible) { confirmBox.visible = false; confirmBox.act = null; return true }
            if (edit.visible) { edit.close(); return true }
            if (triage.visible) { triage.close(); return true }
            if (sheet.visible) { sheet.close(); return true }
            if (root.tab !== "today") { root.tab = "today"; return true }
            return false
        }
        root.load()
    }
    // 注意: 不要在 Component.onDestruction 里清 space 上的共享状态 (onBack / chrome / title)。
    // QML 的 destroy() 是延后执行的, 应用跳应用时旧应用的 onDestruction 会在新应用
    // Component.onCompleted 之后才跑, 把新应用刚设好的东西抹掉。外壳在 unmount() 里
    // 已经统一复位, 应用不用管。

    Rectangle { anchors.fill: parent; color: root.paper; z: -2 }

    // 山水是整页的固定背景: 五个页签共用同一张、同一个位置, 内容压在它上面。
    // 取自概念图 —— 纸底已用色阶提成纯白, 原图左边下边本来就是白纸,
    // 所以不做羽化也没有边界; 右边贴屏幕边缘。
    Image {
        id: headerArt
        z: -1
        anchors.right: parent.right
        // 位置算过账 (页头 238): 图从 32 起, 按 1000x304 的比例, 宽 0.74 时高约 215,
        // 底边落在 247, 与下面的内容压 9 像素 —— 既是背景, 又不糊住内容。
        anchors.top: parent.top; anchors.topMargin: root.u(32)
        width: parent.width * 0.74
        // 高度按图的真实比例算。注意不能读 sourceSize: 显式设了 sourceSize.width 之后
        // 它的 height 返回 0, 会让图回落到一个很小的默认高度, 等比缩放把宽度也一起缩掉。
        // implicitWidth/implicitHeight 才是解码后的真实尺寸。
        height: implicitWidth > 0
                ? Math.round(width * implicitHeight / implicitWidth) : root.u(300)
        // 五页共用同一张、同一个位置: 它只是固定背景, 不该跟着页面变
        source: root.img("header.jpg")
        fillMode: Image.PreserveAspectFit
        sourceSize.width: 1200
        visible: status === Image.Ready
        onStatusChanged: if (status === Image.Ready)
            console.warn("[plan] 页头图就绪 " + (Date.now() - root._t0) + "ms")
    }

    // ═══ 页头 ═══════════════════════════════════════════════════════
    Item {
        id: header
        anchors.top: parent.top; anchors.left: parent.left; anchors.right: parent.right
        height: root.u(238)

        Image {
            x: root.u(12)
            anchors.verticalCenter: bigTitle.verticalCenter
            width: root.f(34); height: width
            sourceSize.width: width; sourceSize.height: width
            source: root.img("caret-left.svg")
            fillMode: Image.PreserveAspectFit
            MouseArea { anchors.fill: parent; anchors.margins: -root.u(20); onClicked: space.exit() }
        }
        Text {
            id: dateLine
            x: root.u(56); y: root.u(44)
            text: root.tab === "cal" ? (root.calYear + "年  " + (root.calMonth + 1) + "月")
                                     : (P.mdText(root.today) + "  " + P.weekText(root.now))
            font.pixelSize: root.f(32); color: root.ink2
        }
        Text {
            id: bigTitle
            x: root.u(56)
            anchors.top: dateLine.bottom; anchors.topMargin: root.u(6)
            text: root.tab === "today" ? "今天"
                : root.tab === "cal" ? "日历" : root.tab === "inbox" ? "收集箱" : "设置"
            font.pixelSize: root.f(84); font.weight: Font.Bold; color: root.ink
        }
    }

    // ═══ 内容区 ═════════════════════════════════════════════════════
    Item {
        id: content
        anchors.top: header.bottom
        anchors.bottom: tabbar.top
        anchors.left: parent.left; anchors.right: parent.right
        anchors.leftMargin: root.u(56); anchors.rightMargin: root.u(56)
        visible: root.ready

        // ─────────────── 今天 ───────────────
        Flickable {
            id: todayPage
            visible: root.tab === "today"
            anchors.fill: parent
            contentWidth: width; contentHeight: todayCol.height + root.u(28); clip: true

            readonly property var dayList: P.dayTasks(root.tasks, root.today)
            readonly property var shown: P.filterBy(dayList, "cat", root.todayCat)
            readonly property int dayDone: P.doneCount(dayList)
            readonly property int dayPct: P.percent(dayDone, dayList.length)
            readonly property int firstUndone: {
                for (var i = 0; i < shown.length; ++i) if (!shown[i].done) return i
                return -1
            }

            Column {
                id: todayCol
                width: parent.width
                spacing: root.u(18)

                // ① 今日进度
                Rectangle {
                    width: parent.width; height: root.u(168)
                    radius: 18; color: root.card; border.color: root.line; border.width: 1
                    Column {
                        anchors.left: parent.left; anchors.leftMargin: root.u(26)
                        anchors.right: progSep.left; anchors.rightMargin: root.u(20)
                        anchors.verticalCenter: parent.verticalCenter
                        spacing: root.u(10)
                        Text { text: "今日进度"; font.pixelSize: root.f(34)
                               font.weight: Font.Bold; color: root.ink }
                        Text {
                            text: todayPage.dayDone + "/" + todayPage.dayList.length + " 完成"
                            font.pixelSize: root.f(25); color: root.ink2
                        }
                        Row {
                            width: parent.width; spacing: root.u(16)
                            Bar {
                                anchors.verticalCenter: parent.verticalCenter
                                width: parent.width - root.f(96) - root.u(16)
                                height: root.u(18)
                                fill: root.barFill
                                value: todayPage.dayPct
                            }
                            Text {
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(96); horizontalAlignment: Text.AlignRight
                                text: todayPage.dayPct + "%"
                                font.pixelSize: root.f(32); font.weight: Font.Bold; color: root.ink
                            }
                        }
                    }
                    Rectangle {
                        id: progSep
                        anchors.right: progThumb.left; anchors.rightMargin: root.u(20)
                        anchors.verticalCenter: parent.verticalCenter
                        width: 1; height: parent.height * 0.56
                        color: root.line
                    }
                    Thumb {
                        id: progThumb
                        anchors.right: progArrow.left; anchors.rightMargin: root.u(18)
                        anchors.verticalCenter: parent.verticalCenter
                        width: root.u(110); height: width
                        source: root.img("illus/ginkgo.jpg")
                    }
                    Image {
                        id: progArrow
                        anchors.right: parent.right; anchors.rightMargin: root.u(22)
                        anchors.verticalCenter: parent.verticalCenter
                        width: root.f(28); height: width
                        sourceSize.width: width; sourceSize.height: width
                        source: root.img("caret-right.svg"); fillMode: Image.PreserveAspectFit
                    }
                    MouseArea { anchors.fill: parent; onClicked: { root.goToday(); root.tab = "cal" } }
                }

                // ② 今日计划 + 分类筛选
                Item {
                    width: parent.width; height: root.f(58)
                    Text {
                        anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter
                        text: "今日计划"; font.pixelSize: root.f(36)
                        font.weight: Font.Bold; color: root.ink
                    }
                    Row {
                        anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter
                        spacing: root.u(10)
                        Repeater {
                            model: [{ k: "", n: "全部" }].concat(P.CATS)
                            delegate: Chip {
                                required property var modelData
                                text: modelData.n
                                fs: root.f(23); pad: root.u(26)
                                tint: modelData.k === "" ? root.tintOf("") : root.tintOf(modelData.k)
                                accent: modelData.k === "" ? root.accent : root.accentOf(modelData.k)
                                selected: root.todayCat === modelData.k
                                onClicked: root.todayCat = modelData.k
                            }
                        }
                    }
                }

                // ③ 时间轴
                // 必须套一层零间距的 Column: 外层 todayCol 的 spacing 作用在每两个子项之间,
                // 任务行也是子项, 于是每行之间留 18px 空档 —— 那条连续的竖线就断在那里。
                Column {
                    width: parent.width
                    spacing: 0
                Repeater {
                    model: todayPage.shown
                    delegate: Item {
                        id: taskRow
                        required property var modelData
                        required property int index
                        width: todayCol.width
                        height: root.u(140)       // 概念图行距 131px, 这里略放宽给整幅插画留地方

                        Text {
                            id: tTime
                            anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter
                            width: root.f(92)
                            text: taskRow.modelData.time || "—"
                            font.pixelSize: root.f(26); color: root.ink
                        }
                        // 竖线: 整行高、压在圆点下层, 所以整列是连续的一根 ——
                        // 概念图里它从圆圈背后穿过去, 圆圈是白底实心所以挡住中间那截。
                        // 首尾两行各收一半, 线头正好停在圆心上。
                        Rectangle {
                            z: -1
                            anchors.horizontalCenter: tDot.horizontalCenter
                            anchors.top: parent.top
                            anchors.topMargin: taskRow.index === 0 ? parent.height / 2 : 0
                            anchors.bottom: parent.bottom
                            anchors.bottomMargin: taskRow.index === todayPage.shown.length - 1
                                                  ? parent.height / 2 : 0
                            width: 3
                            color: root.ink
                        }
                        Rectangle {
                            id: tDot
                            anchors.left: tTime.right; anchors.leftMargin: root.u(8)
                            anchors.verticalCenter: parent.verticalCenter
                            // 已完成 = 主色实心打勾; 当前这件 = 分类色实心; 之后的 = 白底黑边空心
                            readonly property bool current:
                                !taskRow.modelData.done && taskRow.index === todayPage.firstUndone
                            width: root.f(34); height: width; radius: width / 2
                            // 空心那档必须是不透明白底, 不能 transparent —— 否则背后的竖线会从圈里穿出来
                            color: taskRow.modelData.done ? root.accent
                                 : (current ? root.accentOf(taskRow.modelData.cat) : root.paper)
                            border.color: taskRow.modelData.done ? root.accent
                                        : (current ? root.accentOf(taskRow.modelData.cat) : root.ink)
                            border.width: 3
                            Image {
                                anchors.centerIn: parent
                                visible: taskRow.modelData.done
                                width: parent.width * 0.6; height: width
                                sourceSize.width: 40; sourceSize.height: 40
                                source: root.img("check.svg")
                                fillMode: Image.PreserveAspectFit
                            }
                            MouseArea { anchors.fill: parent; anchors.margins: -root.u(18)
                                        onClicked: root.toggleTask(taskRow.modelData.id) }
                        }
                        Column {
                            anchors.left: tDot.right; anchors.leftMargin: root.u(20)
                            anchors.right: tChip.left; anchors.rightMargin: root.u(12)
                            anchors.verticalCenter: parent.verticalCenter
                            spacing: root.u(5)
                            Text {
                                width: parent.width
                                text: taskRow.modelData.title
                                font.pixelSize: root.f(33); font.weight: Font.Bold
                                color: root.ink; elide: Text.ElideRight
                                font.strikeout: taskRow.modelData.done
                            }
                            Text {
                                width: parent.width
                                visible: taskRow.modelData.note !== ""
                                text: taskRow.modelData.note
                                font.pixelSize: root.f(24); color: root.ink2; elide: Text.ElideRight
                            }
                        }
                        Chip {
                            id: tChip
                            anchors.right: tThumb.left; anchors.rightMargin: root.u(40)
                            anchors.verticalCenter: parent.verticalCenter
                            text: P.catName(taskRow.modelData.cat)
                            fs: root.f(23); pad: root.u(24)
                            tint: root.tintOf(taskRow.modelData.cat)
                        }
                        Thumb {
                            id: tThumb
                            anchors.right: tArrow.left
                            anchors.rightMargin: root.illusOf(taskRow.modelData) ? root.u(12) : 0
                            anchors.verticalCenter: parent.verticalCenter
                            width: root.illusOf(taskRow.modelData) ? root.u(200) : 0
                            height: root.u(118)
                            source: root.illusOf(taskRow.modelData)
                                    ? root.img(root.illusOf(taskRow.modelData)) : ""
                        }
                        Image {
                            id: tArrow
                            anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter
                            width: root.f(26); height: width
                            sourceSize.width: width; sourceSize.height: width
                            source: root.img("caret-right.svg"); fillMode: Image.PreserveAspectFit
                            opacity: 0.6
                        }
                        // 从圆点右侧起: 竖线现在是连续的, 横线再从最左边拉过来会把它切断
                        Rectangle {
                            anchors.bottom: parent.bottom
                            anchors.left: tDot.right; anchors.leftMargin: root.u(20)
                            anchors.right: parent.right
                            height: 1; color: root.line
                            visible: taskRow.index < todayPage.shown.length - 1
                        }
                        MouseArea {
                            anchors.fill: parent; anchors.leftMargin: root.f(150); z: -1
                            onClicked: edit.open(taskRow.modelData)
                            onPressAndHold: {
                                var id = taskRow.modelData.id
                                confirmBox.ask("删掉这件事？删了就找不回来了。", function () {
                                    root.removeFrom("tasks", id)
                                })
                            }
                        }
                    }
                }
                }

                Text {
                    width: parent.width
                    visible: todayPage.shown.length === 0
                    // 种子数据是安装当天的, 第二天打开"今天"就空了 —— 不说一声
                    // 会以为数据丢了。这里把别处还有多少件也讲清楚。
                    text: {
                        if (root.todayCat !== "") return "这个分类今天没有安排。"
                        var other = root.tasks.length - todayPage.dayList.length
                        return other > 0
                            ? ("今天还没有安排。点右下角的加号添一件；别的日子还有 "
                               + other + " 件，去「日历」翻。")
                            : "今天还没有安排。点右下角的加号添一件。"
                    }
                    font.pixelSize: root.f(26); color: root.ink2; wrapMode: Text.WordWrap
                }

            }
        }

        // ─────────────── 日历 ───────────────
        Item {
            id: calPage
            visible: root.tab === "cal"
            anchors.fill: parent
            readonly property var dots: P.taskDots(root.tasks)
            // 月视图与日视图: 下面列的是"选中那一天"; 周视图: 列整周并按天分段。
            // 之前月视图取的是整月, 所以点哪天列表都一样。
            readonly property var rangeList: P.dayTasks(root.tasks, root.pickDate)
            readonly property var weekList: P.rangeSections(
                root.tasks, P.rangeDays(root.pickDate, "week", root.cfg.weekStart))

            Segment {
                id: seg
                anchors.top: parent.top; anchors.left: parent.left
                width: root.f(330)
                fs: root.f(27)
                items: [{ k: "month", n: "月" }, { k: "week", n: "周" }, { k: "day", n: "日" }]
                current: root.calMode
                accent: root.accent; ink: root.ink; line: root.line
                onPicked: function (k) { root.calMode = k }
            }

            Row {
                anchors.right: parent.right; anchors.verticalCenter: seg.verticalCenter
                spacing: root.u(18)
                Image {
                    anchors.verticalCenter: parent.verticalCenter
                    width: root.f(28); height: width
                    sourceSize.width: width; sourceSize.height: width
                    source: root.img("caret-left.svg"); fillMode: Image.PreserveAspectFit
                    MouseArea { anchors.fill: parent; anchors.margins: -root.u(16)
                                onClicked: root.calStep(-1) }
                }
                Text {
                    anchors.verticalCenter: parent.verticalCenter
                    text: {
                        if (root.calMode === "month") return root.calYear + "年 " + (root.calMonth + 1) + "月"
                        if (root.calMode === "day") return P.mdText(root.pickDate)
                        var w = P.rangeDays(root.pickDate, "week", root.cfg.weekStart)
                        return P.mdText(w[0]) + " – " + P.mdText(w[6])
                    }
                    font.pixelSize: root.f(27); color: root.ink
                }
                Image {
                    anchors.verticalCenter: parent.verticalCenter
                    width: root.f(28); height: width
                    sourceSize.width: width; sourceSize.height: width
                    source: root.img("caret-right.svg"); fillMode: Image.PreserveAspectFit
                    MouseArea { anchors.fill: parent; anchors.margins: -root.u(16)
                                onClicked: root.calStep(1) }
                }
            }

            Rectangle {
                id: calCard
                anchors.top: seg.bottom; anchors.topMargin: root.u(20)
                anchors.left: parent.left; anchors.right: parent.right
                height: root.u(96) + root.u(92) * 6
                radius: 18; color: root.card; border.color: root.line; border.width: 1
                visible: root.calMode === "month"

                Grid {
                    id: weekHead
                    anchors.top: parent.top; anchors.topMargin: root.u(20)
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.leftMargin: root.u(10); anchors.rightMargin: root.u(10)
                    columns: 7
                    Repeater {
                        model: root.cfg.weekStart === 0
                               ? ["日", "一", "二", "三", "四", "五", "六"]
                               : ["一", "二", "三", "四", "五", "六", "日"]
                        delegate: Item {
                            required property var modelData
                            width: weekHead.width / 7; height: root.f(46)
                            Text { anchors.centerIn: parent; text: modelData
                                   font.pixelSize: root.f(24); color: root.ink2 }
                        }
                    }
                }
                Grid {
                    id: calGrid
                    anchors.top: weekHead.bottom; anchors.topMargin: root.u(4)
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.leftMargin: root.u(10); anchors.rightMargin: root.u(10)
                    columns: 7
                    Repeater {
                        model: P.monthGrid(root.calYear, root.calMonth, root.cfg.weekStart)
                        delegate: Item {
                            id: dayCell
                            required property var modelData
                            readonly property string ds: modelData.y + "-" + P.pad(modelData.m + 1) + "-" + P.pad(modelData.d)
                            readonly property bool isSel: ds === root.pickDate
                            readonly property bool isToday: ds === root.today
                            width: calGrid.width / 7; height: root.u(92)
                            Rectangle {
                                anchors.centerIn: dayNum
                                width: root.f(58); height: width; radius: width / 2
                                color: dayCell.isSel ? root.accent : "transparent"
                                border.color: (!dayCell.isSel && dayCell.isToday) ? root.accent : "transparent"
                                border.width: 2
                            }
                            Text {
                                id: dayNum
                                anchors.horizontalCenter: parent.horizontalCenter
                                y: root.u(12)
                                text: dayCell.modelData.d
                                font.pixelSize: root.f(28)
                                color: dayCell.isSel ? root.ink
                                     : (dayCell.modelData.inMonth ? root.ink : root.line)
                            }
                            Row {
                                anchors.horizontalCenter: parent.horizontalCenter
                                anchors.top: dayNum.bottom; anchors.topMargin: root.u(12)
                                spacing: root.u(6)
                                Repeater {
                                    model: calPage.dots[dayCell.ds] || []
                                    delegate: Rectangle {
                                        required property var modelData
                                        width: root.f(11); height: width; radius: width / 2
                                        color: root.accentOf(modelData)
                                    }
                                }
                            }
                            MouseArea {
                                anchors.fill: parent
                                onClicked: { root.pickDate = dayCell.ds; root.syncCalTo(dayCell.ds) }
                            }
                        }
                    }
                }
            }

            // 周视图的七天日期条: 月视图有整月网格, 日视图只看一天, 唯独周视图
            // 原来什么都不显示, 和日视图看不出区别。
            Rectangle {
                id: weekStrip
                anchors.top: seg.bottom; anchors.topMargin: root.u(20)
                anchors.left: parent.left; anchors.right: parent.right
                height: root.u(140)
                radius: 14; color: root.card; border.color: root.line; border.width: 1
                visible: root.calMode === "week"
                Row {
                    anchors.fill: parent; anchors.margins: root.u(8)
                    Repeater {
                        model: P.rangeDays(root.pickDate, "week", root.cfg.weekStart)
                        delegate: Item {
                            id: wCell
                            required property var modelData
                            readonly property bool isSel: modelData === root.pickDate
                            readonly property bool isToday: modelData === root.today
                            width: (weekStrip.width - root.u(16)) / 7
                            height: parent.height
                            Text {
                                id: wDow
                                anchors.horizontalCenter: parent.horizontalCenter
                                y: root.u(10)
                                text: P.weekText(new Date(wCell.modelData.replace(/-/g, "/"))).substring(2)
                                font.pixelSize: root.f(23); color: root.ink2
                            }
                            Rectangle {
                                anchors.centerIn: wNum
                                width: root.f(56); height: width; radius: width / 2
                                color: wCell.isSel ? root.accent : "transparent"
                                border.color: (!wCell.isSel && wCell.isToday) ? root.accent : "transparent"
                                border.width: 2
                            }
                            Text {
                                id: wNum
                                anchors.horizontalCenter: parent.horizontalCenter
                                anchors.top: wDow.bottom; anchors.topMargin: root.u(10)
                                text: parseInt(wCell.modelData.substring(8), 10)
                                font.pixelSize: root.f(30)
                                color: wCell.isSel ? root.ink : root.ink
                            }
                            Row {
                                anchors.horizontalCenter: parent.horizontalCenter
                                anchors.top: wNum.bottom; anchors.topMargin: root.u(8)
                                spacing: root.u(5)
                                Repeater {
                                    model: calPage.dots[wCell.modelData] || []
                                    delegate: Rectangle {
                                        required property var modelData
                                        width: root.f(10); height: width; radius: width / 2
                                        color: root.accentOf(modelData)
                                    }
                                }
                            }
                            MouseArea { anchors.fill: parent; onClicked: root.pickDate = wCell.modelData }
                        }
                    }
                }
            }

            Item {
                id: dayHead
                anchors.top: root.calMode === "month" ? calCard.bottom
                           : root.calMode === "week" ? weekStrip.bottom : seg.bottom
                anchors.topMargin: root.u(24)
                anchors.left: parent.left; anchors.right: parent.right
                height: root.f(58)
                Row {
                    anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter
                    spacing: root.u(14)
                    Text {
                        anchors.bottom: parent.bottom
                        text: root.calMode === "week" ? "本周" : P.mdText(root.pickDate)
                        font.pixelSize: root.f(36); font.weight: Font.Bold; color: root.ink
                    }
                    Text {
                        anchors.bottom: parent.bottom; anchors.bottomMargin: root.u(4)
                        text: P.weekText(new Date(root.pickDate.replace(/-/g, "/")))
                        font.pixelSize: root.f(26); color: root.ink2
                    }
                }
                Chip {
                    anchors.right: parent.right; anchors.verticalCenter: parent.verticalCenter
                    text: "今天"; fs: root.f(24); pad: root.u(28);                     tint: root.tintOf(""); accent: root.accent
                    selected: root.pickDate === root.today
                    onClicked: root.goToday()
                }
            }

            Flickable {
                anchors.top: dayHead.bottom; anchors.topMargin: root.u(14)
                anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom
                contentWidth: width; contentHeight: dayCol.height + root.u(28); clip: true
                Column {
                    id: dayCol
                    width: parent.width
                    spacing: root.u(14)
                    Repeater {
                        model: root.calMode === "week" ? calPage.weekList : calPage.rangeList
                        delegate: Item {
                            id: calRow
                            required property var modelData
                            // 周视图的模型里混着"日期头"与"任务"两种条目
                            readonly property bool isSec: modelData.sec === true
                            readonly property var task: isSec ? null
                                                     : (modelData.t !== undefined ? modelData.t : modelData)
                            width: dayCol.width
                            height: isSec ? root.u(64) : root.u(112)

                            Text {
                                anchors.left: parent.left; anchors.bottom: parent.bottom
                                anchors.bottomMargin: root.u(10)
                                visible: calRow.isSec
                                text: calRow.isSec
                                      ? (P.mdText(calRow.modelData.date) + "  "
                                         + P.weekText(new Date(calRow.modelData.date.replace(/-/g, "/"))))
                                      : ""
                                font.pixelSize: root.f(26); font.weight: Font.Bold; color: root.ink
                            }
                            Text {
                                id: cTime
                                visible: !calRow.isSec
                                anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter
                                width: root.f(92)
                                text: calRow.task ? (calRow.task.time || "—") : ""
                                font.pixelSize: root.f(26); color: root.ink2
                            }
                            Rectangle {
                                id: cDot
                                visible: !calRow.isSec
                                anchors.left: cTime.right; anchors.leftMargin: root.u(8)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(30); height: width; radius: width / 2
                                // 未完成 = 分类色实心点; 已完成 = 主色实心 + 白勾, 和"今天"页一致
                                color: !calRow.task ? "transparent"
                                     : (calRow.task.done ? root.accent : root.accentOf(calRow.task.cat))
                                Image {
                                    anchors.centerIn: parent
                                    visible: calRow.task ? calRow.task.done : false
                                    width: parent.width * 0.6; height: width
                                    sourceSize.width: 40; sourceSize.height: 40
                                    source: root.img("check.svg")
                                    fillMode: Image.PreserveAspectFit
                                }
                                MouseArea {
                                    anchors.fill: parent; anchors.margins: -root.u(16)
                                    onClicked: if (calRow.task) root.toggleTask(calRow.task.id)
                                }
                            }
                            Rectangle {
                                visible: !calRow.isSec
                                anchors.left: cDot.right; anchors.leftMargin: root.u(18)
                                anchors.right: parent.right
                                anchors.verticalCenter: parent.verticalCenter
                                height: root.u(100); radius: 12
                                color: calRow.task ? root.tintOf(calRow.task.cat) : "transparent"
                                Column {
                                    anchors.left: parent.left; anchors.leftMargin: root.u(22)
                                    anchors.right: cThumb.left; anchors.rightMargin: root.u(14)
                                    anchors.verticalCenter: parent.verticalCenter
                                    spacing: root.u(4)
                                    Text {
                                        width: parent.width
                                        text: calRow.task ? calRow.task.title : ""
                                        font.pixelSize: root.f(30); font.weight: Font.Bold
                                        color: root.ink; elide: Text.ElideRight
                                        font.strikeout: calRow.task ? calRow.task.done : false
                                    }
                                    Text {
                                        width: parent.width
                                        visible: calRow.task ? calRow.task.note !== "" : false
                                        text: calRow.task ? calRow.task.note : ""
                                        font.pixelSize: root.f(23); color: root.ink2; elide: Text.ElideRight
                                    }
                                }
                                Thumb {
                                    id: cThumb
                                    anchors.right: parent.right
                                    anchors.rightMargin: root.illusOf(calRow.task) ? root.u(16) : 0
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.illusOf(calRow.task) ? root.u(96) : 0
                                    height: root.u(72)
                                    source: root.illusOf(calRow.task)
                                            ? root.img(root.illusOf(calRow.task)) : ""
                                }
                                MouseArea {
                                    anchors.fill: parent
                                    onClicked: if (calRow.task) edit.open(calRow.task)
                                    onPressAndHold: {
                                        if (!calRow.task) return
                                        var id = calRow.task.id
                                        confirmBox.ask("删掉这件事？删了就找不回来了。", function () {
                                            root.removeFrom("tasks", id)
                                        })
                                    }
                                }
                            }
                        }
                    }
                    Text {
                        width: dayCol.width
                        visible: root.calMode === "week" ? calPage.weekList.length === 0
                                                          : calPage.rangeList.length === 0
                        text: root.calMode === "week" ? "这一周还没有安排。" : "这一天还没有安排。"
                        font.pixelSize: root.f(26); color: root.ink2
                    }
                }
            }
        }

        // ─────────────── 收集箱 ───────────────
        Item {
            id: inboxPage
            visible: root.tab === "inbox"
            anchors.fill: parent
            readonly property var shown: P.filterBy(root.inbox, "kind", root.inboxKind)

            Item {
                id: inboxFilter
                anchors.top: parent.top; anchors.topMargin: root.u(2)
                anchors.left: parent.left; anchors.right: parent.right
                height: root.f(54)
                Row {
                    anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter
                    spacing: root.u(12)
                    Repeater {
                        model: [{ k: "", n: "全部" }].concat(P.KINDS)
                        delegate: Chip {
                            required property var modelData
                            readonly property int cnt: modelData.k === "" ? root.inbox.length
                                                     : P.countBy(root.inbox, "kind", modelData.k)
                            text: modelData.n + (cnt > 0 ? " " + cnt : "")
                            fs: root.f(24); pad: root.u(26)
                            tint: root.kindTint(modelData.k)
                            accent: root.kindAccent(modelData.k)
                            selected: root.inboxKind === modelData.k
                            onClicked: root.inboxKind = modelData.k
                        }
                    }
                }
            }

            Flickable {
                anchors.top: inboxFilter.bottom; anchors.topMargin: root.u(16)
                anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom
                contentWidth: width; contentHeight: inboxCol.height + root.u(28); clip: true
                Column {
                    id: inboxCol
                    width: parent.width
                    spacing: root.u(14)
                    Repeater {
                        model: inboxPage.shown
                        delegate: Rectangle {
                            id: iCard
                            required property var modelData
                            width: inboxCol.width
                            height: Math.max(root.u(150), iText.implicitHeight + root.u(96))
                            radius: 18; color: root.card; border.color: root.line; border.width: 1

                            Rectangle {
                                id: iBox
                                anchors.left: parent.left; anchors.leftMargin: root.u(22)
                                anchors.top: parent.top; anchors.topMargin: root.u(26)
                                width: root.f(34); height: width; radius: 6
                                color: iCard.modelData.done ? root.accent : "transparent"
                                border.color: iCard.modelData.done ? root.accent : root.ink
                                border.width: 2
                                Image {
                                    anchors.centerIn: parent
                                    visible: iCard.modelData.done
                                    width: parent.width * 0.62; height: width
                                    sourceSize.width: 40; sourceSize.height: 40
                                    source: root.img("check.svg"); fillMode: Image.PreserveAspectFit
                                }
                                MouseArea { anchors.fill: parent; anchors.margins: -root.u(16)
                                            onClicked: root.toggleInbox(iCard.modelData.id) }
                            }
                            Row {
                                id: iHead
                                anchors.left: iBox.right; anchors.leftMargin: root.u(18)
                                anchors.top: parent.top; anchors.topMargin: root.u(22)
                                spacing: root.u(12)
                                Image {
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.f(30); height: width
                                    sourceSize.width: width; sourceSize.height: width
                                    source: root.img(P.kindOf)(iCard.modelData.kind).icon + ".svg"
                                    fillMode: Image.PreserveAspectFit
                                }
                                Chip {
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: P.kindOf(iCard.modelData.kind).n
                                    fs: root.f(22); pad: root.u(22)
                                    tint: root.kindTint(iCard.modelData.kind)
                                }
                                Text {
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: iCard.modelData.at
                                    font.pixelSize: root.f(23); color: root.ink2
                                }
                            }
                            Image {
                                anchors.right: parent.right; anchors.rightMargin: root.u(22)
                                anchors.verticalCenter: iHead.verticalCenter
                                width: root.f(30); height: width
                                sourceSize.width: width; sourceSize.height: width
                                source: root.img("dots-three.svg"); fillMode: Image.PreserveAspectFit
                                // 点它打开整理面板: 转成任务 / 转成项目 / 删除。
                                // 原来点一下直接删, 太容易误伤 (删了还不知道去哪了)。
                                MouseArea {
                                    anchors.fill: parent; anchors.margins: -root.u(16)
                                    onClicked: triage.open(iCard.modelData)
                                }
                            }
                            Text {
                                id: iText
                                anchors.left: iBox.right; anchors.leftMargin: root.u(18)
                                anchors.right: parent.right; anchors.rightMargin: root.u(22)
                                anchors.top: iHead.bottom; anchors.topMargin: root.u(12)
                                text: iCard.modelData.text
                                font.pixelSize: root.f(27); color: root.ink; wrapMode: Text.WordWrap
                                font.strikeout: iCard.modelData.done
                            }
                            MouseArea {
                                anchors.fill: parent; z: -1
                                onPressAndHold: {
                                    root.removeFrom("inbox", iCard.modelData.id)
                                }
                            }
                        }
                    }
                    Text {
                        width: inboxCol.width
                        visible: inboxPage.shown.length === 0
                        text: "收集箱是空的。想到什么先扔进来，回头再整理。"
                        font.pixelSize: root.f(26); color: root.ink2; wrapMode: Text.WordWrap
                    }
                }
            }
        }

        // ─────────────── 设置 ───────────────
        Flickable {
            visible: root.tab === "set"
            anchors.fill: parent
            contentWidth: width; contentHeight: setCol.height + root.u(28); clip: true
            Column {
                id: setCol
                width: parent.width
                spacing: root.u(20)

                Rectangle {
                    width: parent.width; height: root.u(104) * 2 + 1
                    radius: 18; color: root.card; border.color: root.line; border.width: 1
                    Column {
                        anchors.fill: parent
                        Item {
                            width: parent.width; height: root.u(104)
                            Rectangle {
                                id: icDefault
                                anchors.left: parent.left; anchors.leftMargin: root.u(22)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(56); height: width; radius: 12
                                color: root.tintOf("work")
                                Image {
                                    anchors.centerIn: parent
                                    width: parent.width * 0.6; height: width
                                    sourceSize.width: 60; sourceSize.height: 60
                                    source: root.img("calendar-check.svg"); fillMode: Image.PreserveAspectFit
                                }
                            }
                            Text {
                                anchors.left: icDefault.right; anchors.leftMargin: root.u(20)
                                anchors.verticalCenter: parent.verticalCenter
                                text: "默认视图"; font.pixelSize: root.f(29)
                                font.weight: Font.Medium; color: root.ink
                            }
                            Text {
                                anchors.right: dvArrow.left; anchors.rightMargin: root.u(12)
                                anchors.verticalCenter: parent.verticalCenter
                                text: root.cfg.defaultView === "cal" ? "日历"
                                    : root.cfg.defaultView === "inbox" ? "收集箱" : "今天"
                                font.pixelSize: root.f(26); color: root.ink2
                            }
                            Image {
                                id: dvArrow
                                anchors.right: parent.right; anchors.rightMargin: root.u(22)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(26); height: width
                                sourceSize.width: width; sourceSize.height: width
                                source: root.img("caret-right.svg"); fillMode: Image.PreserveAspectFit
                            }
                            MouseArea {
                                anchors.fill: parent
                                onClicked: {
                                    var order = ["today", "cal", "inbox"], i = 0
                                    for (var k = 0; k < order.length; ++k)
                                        if (order[k] === root.cfg.defaultView) i = k
                                    root.setCfg("defaultView", order[(i + 1) % order.length])
                                }
                            }
                        }
                        Rule { x: root.u(22); width: parent.width - root.u(44) }
                        Item {
                            width: parent.width; height: root.u(104)
                            Rectangle {
                                id: icWeek
                                anchors.left: parent.left; anchors.leftMargin: root.u(22)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(56); height: width; radius: 12
                                color: root.tintOf("hobby")
                                Image {
                                    anchors.centerIn: parent
                                    width: parent.width * 0.6; height: width
                                    sourceSize.width: 60; sourceSize.height: 60
                                    source: root.img("calendar-dots.svg"); fillMode: Image.PreserveAspectFit
                                }
                            }
                            Text {
                                anchors.left: icWeek.right; anchors.leftMargin: root.u(20)
                                anchors.verticalCenter: parent.verticalCenter
                                text: "周起始日"; font.pixelSize: root.f(29)
                                font.weight: Font.Medium; color: root.ink
                            }
                            Text {
                                anchors.right: wsArrow.left; anchors.rightMargin: root.u(12)
                                anchors.verticalCenter: parent.verticalCenter
                                text: root.cfg.weekStart === 0 ? "星期日" : "星期一"
                                font.pixelSize: root.f(26); color: root.ink2
                            }
                            Image {
                                id: wsArrow
                                anchors.right: parent.right; anchors.rightMargin: root.u(22)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(26); height: width
                                sourceSize.width: width; sourceSize.height: width
                                source: root.img("caret-right.svg"); fillMode: Image.PreserveAspectFit
                            }
                            MouseArea {
                                anchors.fill: parent
                                onClicked: root.setCfg("weekStart", root.cfg.weekStart === 0 ? 1 : 0)
                            }
                        }
                    }
                }
                Rectangle {
                    width: parent.width; height: root.u(104)
                    radius: 18; color: root.card; border.color: root.line; border.width: 1
                    Column {
                        anchors.fill: parent
                        Item {
                            width: parent.width; height: root.u(104)
                            Rectangle {
                                id: icClear
                                anchors.left: parent.left; anchors.leftMargin: root.u(22)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(56); height: width; radius: 12
                                color: root.tintOf("life")
                                Image {
                                    anchors.centerIn: parent
                                    width: parent.width * 0.6; height: width
                                    sourceSize.width: 60; sourceSize.height: 60
                                    source: root.img("trash.svg"); fillMode: Image.PreserveAspectFit
                                }
                            }
                            Text {
                                anchors.left: icClear.right; anchors.leftMargin: root.u(20)
                                anchors.verticalCenter: parent.verticalCenter
                                text: "清除已完成"; font.pixelSize: root.f(29)
                                font.weight: Font.Medium; color: root.ink
                            }
                            Image {
                                anchors.right: parent.right; anchors.rightMargin: root.u(22)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(26); height: width
                                sourceSize.width: width; sourceSize.height: width
                                source: root.img("caret-right.svg"); fillMode: Image.PreserveAspectFit
                            }
                            MouseArea {
                                anchors.fill: parent
                                onClicked: confirmBox.ask("把所有打过勾的任务和收集箱条目清掉？", function () {
                                    var t = [], b = [], i
                                    for (i = 0; i < root.tasks.length; ++i)
                                        if (!root.tasks[i].done) t.push(root.tasks[i])
                                    for (i = 0; i < root.inbox.length; ++i)
                                        if (!root.inbox[i].done) b.push(root.inbox[i])
                                    root.tasks = t; root.inbox = b; root.save()
                                })
                            }
                        }
                    }
                }

                Rectangle {
                    width: parent.width; height: root.u(104)
                    radius: 18; color: root.card; border.color: root.line; border.width: 1
                    Rectangle {
                        id: icAbout
                        anchors.left: parent.left; anchors.leftMargin: root.u(22)
                        anchors.verticalCenter: parent.verticalCenter
                        width: root.f(56); height: width; radius: 12
                        color: root.tintOf("")
                        Image {
                            anchors.centerIn: parent
                            width: parent.width * 0.6; height: width
                            sourceSize.width: 60; sourceSize.height: 60
                            source: root.img("clipboard-text.svg"); fillMode: Image.PreserveAspectFit
                        }
                    }
                    Text {
                        anchors.left: icAbout.right; anchors.leftMargin: root.u(20)
                        anchors.verticalCenter: parent.verticalCenter
                        text: "关于"; font.pixelSize: root.f(29)
                        font.weight: Font.Medium; color: root.ink
                    }
                    Text {
                        anchors.right: parent.right; anchors.rightMargin: root.u(22)
                        anchors.verticalCenter: parent.verticalCenter
                        text: "计划 v1.0.0"; font.pixelSize: root.f(26); color: root.ink2
                    }
                }
            }
        }
    }

    // ═══ 记一条 (收集箱) ═══════════════════════════════════════════
    // 任务和项目的"新建"已经并进各自的编辑屏 (同一套版式, 顶上那行小字区分
    // 是"新建"还是"编辑"), 这里只剩收集箱这一种, 排版照编辑屏来。
    Rectangle {
        id: sheet
        anchors.fill: parent
        color: root.paper
        visible: false
        z: 22
        property string pickKind: "idea"
        // 校验不弹提示条: 标题那条下划线加粗变主色就够了, 墨水屏上这比闪一下黑条清楚
        property bool warn: false

        function open() {
            noteInput.text = ""
            sheet.warn = false
            sheet.pickKind = "idea"
            sheet.visible = true
            noteInput.forceActiveFocus()
        }
        function close() { sheet.visible = false; root.dropFocus() }
        function commit() {
            var t = noteInput.text.replace(/^\s+|\s+$/g, "")
            if (t === "") { sheet.warn = true; noteInput.forceActiveFocus(); return }
            root.addInbox(t, sheet.pickKind)
            root.inboxKind = ""          // 别让筛选把刚记的这条挡在外面
            sheet.close()
        }

        MouseArea { anchors.fill: parent; onClicked: root.dropFocus() }

        Image {
            z: -1
            anchors.right: parent.right
            anchors.top: parent.top; anchors.topMargin: root.u(32)
            width: parent.width * 0.74
            height: implicitWidth > 0
                    ? Math.round(width * implicitHeight / implicitWidth) : root.u(300)
            source: root.img("header.jpg")
            fillMode: Image.PreserveAspectFit
            sourceSize.width: 1200
            visible: status === Image.Ready
        }
        // 页头固定不滚: 大标题 (名字本身) 始终在, 返回箭头和它垂直居中对齐 ——
        // 和主页头 `<` 对齐"今天 / 收集箱"是同一个关系。
        Item {
            id: nHead
            anchors.top: parent.top
            anchors.left: parent.left; anchors.right: parent.right
            anchors.leftMargin: root.u(56); anchors.rightMargin: root.u(56)
            height: root.u(48) + nCtx.height + root.u(22) + root.f(96)

            Text {
                id: nCtx
                anchors.top: parent.top; anchors.topMargin: root.u(48)
                anchors.left: parent.left
                text: sheet.warn ? "新建 · 收集箱    还没写内容" : "新建 · 收集箱"
                font.pixelSize: root.f(30)
                color: sheet.warn ? root.accent : root.ink2
            }
            Item {
                id: nTitleBox
                anchors.top: nCtx.bottom; anchors.topMargin: root.u(22)
                anchors.left: parent.left; anchors.right: parent.right
                height: root.f(96)

                // 画在页边距外侧 (主页头的箭头在 x=12, 这里内容从 56 起, 所以回退 44)
                Image {
                    x: -root.u(44)
                    anchors.verticalCenter: noteInput.verticalCenter
                    width: root.f(34); height: width
                    sourceSize.width: width; sourceSize.height: width
                    source: root.img("caret-left.svg"); fillMode: Image.PreserveAspectFit
                    MouseArea { anchors.fill: parent; anchors.margins: -root.u(20)
                                onClicked: sheet.close() }
                }
                TextInput {
                    id: noteInput
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.bottom: parent.bottom; anchors.bottomMargin: root.u(14)
                    font.pixelSize: root.f(52); font.weight: Font.Bold; color: root.ink
                    clip: true
                    onAccepted: sheet.commit()
                    onTextChanged: if (text !== "") sheet.warn = false
                    Text {
                        anchors.fill: parent; verticalAlignment: Text.AlignVCenter
                        visible: noteInput.text === ""
                        text: "想到了什么"
                        font.pixelSize: root.f(52); font.weight: Font.Bold; color: root.line
                    }
                }
                Rectangle {
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    height: sheet.warn ? 3 : 1
                    color: sheet.warn ? root.accent : root.line
                }
            }
        }

        Flickable {
            anchors.top: nHead.bottom; anchors.topMargin: root.u(26)
            anchors.left: parent.left; anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.leftMargin: root.u(56); anchors.rightMargin: root.u(56)
            anchors.bottomMargin: root.u(28)
            contentWidth: width; contentHeight: nCol.height + root.u(40); clip: true

            MouseArea { anchors.fill: parent; z: -1; onClicked: root.dropFocus() }

            Column {
                id: nCol
                width: parent.width
                spacing: root.u(22)

                Row {
                    spacing: root.u(12)
                    Repeater {
                        model: P.KINDS
                        delegate: Chip {
                            required property var modelData
                            text: modelData.n
                            fs: root.f(25); pad: root.u(32)
                            tint: root.kindTint(modelData.k); accent: root.kindAccent(modelData.k)
                            selected: sheet.pickKind === modelData.k
                            onClicked: sheet.pickKind = modelData.k
                        }
                    }
                }
                Rectangle {
                    width: parent.width; height: root.f(92); radius: 16
                    color: root.accent
                    Text { anchors.centerIn: parent; text: "记下"
                           font.pixelSize: root.f(30); color: root.ink }
                    MouseArea { anchors.fill: parent; onClicked: sheet.commit() }
                }
            }
        }
    }

    // ═══ 收集箱: 改这一条 / 转走 / 删掉 ════════════════════════════
    // 原来这条只能转走或删掉, 文字写错了没救。排版跟两个编辑屏一套。
    Rectangle {
        id: triage
        anchors.fill: parent
        color: root.paper
        visible: false
        z: 22
        property var item: null
        property string pickKind: "idea"
        property string pickCat: "work"
        property bool warn: false

        function open(it) {
            if (!it) return
            triage.item = it
            tText.text = it.text || ""
            triage.pickKind = it.kind || "idea"
            triage.pickCat = "work"
            triage.warn = false
            triage.visible = true
            root.dropFocus()
        }
        function close() { triage.visible = false; root.dropFocus() }
        function text() { return tText.text.replace(/^\s+|\s+$/g, "") }
        function save() {
            if (!triage.item) return
            if (triage.text() === "") { triage.warn = true; tText.forceActiveFocus(); return }
            root.updateInbox(triage.item.id, { text: triage.text(), kind: triage.pickKind })
            triage.close()
        }
        // 转走时用的是框里当前的文字, 顺手改的那几个字不会丢
        function toTask() {
            if (!triage.item || triage.text() === "") { triage.warn = true; return }
            root.addTask(triage.text(), "", triage.pickCat, root.today)
            root.removeFrom("inbox", triage.item.id)
            root.todayCat = ""
            triage.close()
        }
        function drop() {
            if (!triage.item) return
            root.removeFrom("inbox", triage.item.id)
            triage.close()
        }

        MouseArea { anchors.fill: parent; onClicked: root.dropFocus() }

        Image {
            z: -1
            anchors.right: parent.right
            anchors.top: parent.top; anchors.topMargin: root.u(32)
            width: parent.width * 0.74
            height: implicitWidth > 0
                    ? Math.round(width * implicitHeight / implicitWidth) : root.u(300)
            source: root.img("header.jpg")
            fillMode: Image.PreserveAspectFit
            sourceSize.width: 1200
            visible: status === Image.Ready
        }

        Item {
            id: tHead
            anchors.top: parent.top
            anchors.left: parent.left; anchors.right: parent.right
            anchors.leftMargin: root.u(56); anchors.rightMargin: root.u(56)
            height: root.u(48) + tCtx.height + root.u(22) + root.f(96)

            Text {
                id: tCtx
                anchors.top: parent.top; anchors.topMargin: root.u(48)
                anchors.left: parent.left
                text: triage.warn ? "收集箱 · 这一条    内容不能空着" : "收集箱 · 这一条"
                font.pixelSize: root.f(30)
                color: triage.warn ? root.accent : root.ink2
            }
            Item {
                anchors.top: tCtx.bottom; anchors.topMargin: root.u(22)
                anchors.left: parent.left; anchors.right: parent.right
                height: root.f(96)

                Image {
                    x: -root.u(44)
                    anchors.verticalCenter: tText.verticalCenter
                    width: root.f(34); height: width
                    sourceSize.width: width; sourceSize.height: width
                    source: root.img("caret-left.svg"); fillMode: Image.PreserveAspectFit
                    MouseArea { anchors.fill: parent; anchors.margins: -root.u(20)
                                onClicked: triage.close() }
                }
                TextInput {
                    id: tText
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.bottom: parent.bottom; anchors.bottomMargin: root.u(14)
                    font.pixelSize: root.f(46); font.weight: Font.Bold; color: root.ink
                    clip: true
                    onAccepted: triage.save()
                    onTextChanged: if (text !== "") triage.warn = false
                }
                Rectangle {
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    height: triage.warn ? 3 : 1
                    color: triage.warn ? root.accent : root.line
                }
            }
        }

        Flickable {
            anchors.top: tHead.bottom; anchors.topMargin: root.u(26)
            anchors.left: parent.left; anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.leftMargin: root.u(56); anchors.rightMargin: root.u(56)
            anchors.bottomMargin: root.u(28)
            contentWidth: width; contentHeight: tCol.height + root.u(40); clip: true

            MouseArea { anchors.fill: parent; z: -1; onClicked: root.dropFocus() }

            Column {
                id: tCol
                width: parent.width
                spacing: root.u(22)

                Row {
                    spacing: root.u(12)
                    Repeater {
                        model: P.KINDS
                        delegate: Chip {
                            required property var modelData
                            text: modelData.n
                            fs: root.f(25); pad: root.u(32)
                            tint: root.kindTint(modelData.k); accent: root.kindAccent(modelData.k)
                            selected: triage.pickKind === modelData.k
                            onClicked: triage.pickKind = modelData.k
                        }
                    }
                }

                Rectangle {
                    width: parent.width; height: root.f(92); radius: 16
                    color: root.accent
                    Text { anchors.centerIn: parent; text: "保存"
                           font.pixelSize: root.f(30); color: root.ink }
                    MouseArea { anchors.fill: parent; onClicked: triage.save() }
                }

                Rule { width: parent.width }

                Text { text: "整理到别处"; font.pixelSize: root.f(28)
                       font.weight: Font.Bold; color: root.ink }
                Text { text: "放过去之后，收集箱里这一条就消掉了。"
                       font.pixelSize: root.f(24); color: root.ink2 }

                Row {
                    spacing: root.u(12)
                    Repeater {
                        model: P.CATS
                        delegate: Chip {
                            required property var modelData
                            text: modelData.n
                            fs: root.f(25); pad: root.u(32)
                            tint: root.tintOf(modelData.k); accent: root.accentOf(modelData.k)
                            selected: triage.pickCat === modelData.k
                            onClicked: triage.pickCat = modelData.k
                        }
                    }
                }

                Rectangle {
                    width: parent.width; height: root.f(88); radius: 14
                    color: root.card; border.color: root.ink; border.width: 2
                    Text { anchors.centerIn: parent; text: "放进今天的任务"
                           font.pixelSize: root.f(29); color: root.ink }
                    MouseArea { anchors.fill: parent; onClicked: triage.toTask() }
                }
                Text {
                    anchors.horizontalCenter: parent.horizontalCenter
                    text: "删掉这条"
                    font.pixelSize: root.f(26); color: root.ink2
                    MouseArea {
                        anchors.fill: parent; anchors.margins: -root.u(20)
                        onClicked: confirmBox.ask("删掉这一条？删了就找不回来了。", triage.drop)
                    }
                }
            }
        }
    }

    // ═══ 编辑一件事 ════════════════════════════════════════════════
    // 排版跟着应用其它页面走, 不摞表单: 左上角返回箭头 + 小字上下文 + 大号标题
    // (标题本身就是输入框, 像在纸上写), 其余参数收进一张分隔线卡片, 和"设置"页同款。
    // 山水也照旧露出来 —— 整屏纯白盖住它, 这一屏就不像同一个应用了。
    Rectangle {
        id: edit
        anchors.fill: parent
        color: root.paper
        visible: false
        z: 22
        property var item: null          // null = 新建
        property bool isNew: edit.item === null
        property string pickCat: "work"
        property string pickDate: ""
        property bool pickDone: false
        property string pickIllus: ""
        property bool warn: false

        function openNew(date) {
            edit.warn = false
            edit.item = null
            eTitle.text = ""; eTime.text = ""; eNote.text = ""
            edit.pickCat = "work"
            edit.pickIllus = ""
            edit.pickDate = date || root.today
            edit.pickDone = false
            edit.visible = true
            eTitle.forceActiveFocus()     // 新建才自动弹键盘; 编辑是来改参数的, 不弹
        }
        function open(t) {
            if (!t) return
            edit.item = t
            eTitle.text = t.title || ""
            eTime.text = t.time || ""
            eNote.text = t.note || ""
            edit.pickCat = t.cat || "work"
            edit.pickIllus = t.illus || ""
            edit.pickDate = t.date
            edit.pickDone = t.done === true
            edit.warn = false
            edit.visible = true
            root.dropFocus()
        }
        function close() { edit.visible = false; root.dropFocus() }
        function commit() {
            var ti = eTitle.text.replace(/^\s+|\s+$/g, "")
            if (ti === "") { edit.warn = true; eTitle.forceActiveFocus(); return }
            var tm = root.normTime(eTime.text)
            var nt = eNote.text.replace(/^\s+|\s+$/g, "")
            if (edit.isNew) {
                root.addTask(ti, tm, edit.pickCat, edit.pickDate)
                // addTask 刚追加在末尾, 把备注和配图补上去
                var l = root.tasks
                root.updateTask(l[l.length - 1].id, { note: nt, illus: edit.pickIllus })
                // 别让筛选 / 当前看的日子把刚建的这条挡在外面
                root.todayCat = ""
                if (root.tab === "cal") { root.pickDate = edit.pickDate; root.syncCalTo(edit.pickDate) }
            } else {
                root.updateTask(edit.item.id, { title: ti, time: tm, note: nt,
                                                cat: edit.pickCat, date: edit.pickDate,
                                                illus: edit.pickIllus, done: edit.pickDone })
            }
            edit.close()
        }

        MouseArea { anchors.fill: parent; onClicked: root.dropFocus() }

        Image {
            z: -1
            anchors.right: parent.right
            anchors.top: parent.top; anchors.topMargin: root.u(32)
            width: parent.width * 0.74
            height: implicitWidth > 0
                    ? Math.round(width * implicitHeight / implicitWidth) : root.u(300)
            source: root.img("header.jpg")
            fillMode: Image.PreserveAspectFit
            sourceSize.width: 1200
            visible: status === Image.Ready
        }

        // 页头固定不滚: 大标题 (名字本身) 始终在, 返回箭头和它垂直居中对齐 ——
        // 和主页头 `<` 对齐"今天 / 收集箱"是同一个关系。
        Item {
            id: eHead
            anchors.top: parent.top
            anchors.left: parent.left; anchors.right: parent.right
            anchors.leftMargin: root.u(56); anchors.rightMargin: root.u(56)
            height: root.u(48) + eCtx.height + root.u(22) + root.f(96)

            Text {
                id: eCtx
                anchors.top: parent.top; anchors.topMargin: root.u(48)
                anchors.left: parent.left
                text: (edit.isNew ? "新建 · 任务" : "编辑 · 任务")
                      + (edit.warn ? "    还没写标题" : "")
                font.pixelSize: root.f(30)
                color: edit.warn ? root.accent : root.ink2
            }
            Item {
                id: eTitleBox
                anchors.top: eCtx.bottom; anchors.topMargin: root.u(22)
                anchors.left: parent.left; anchors.right: parent.right
                height: root.f(96)

                // 画在页边距外侧 (主页头的箭头在 x=12, 这里内容从 56 起, 所以回退 44)
                Image {
                    x: -root.u(44)
                    anchors.verticalCenter: eTitle.verticalCenter
                    width: root.f(34); height: width
                    sourceSize.width: width; sourceSize.height: width
                    source: root.img("caret-left.svg"); fillMode: Image.PreserveAspectFit
                    MouseArea { anchors.fill: parent; anchors.margins: -root.u(20)
                                onClicked: edit.close() }
                }
                TextInput {
                    id: eTitle
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.bottom: parent.bottom; anchors.bottomMargin: root.u(14)
                    font.pixelSize: root.f(58); font.weight: Font.Bold; color: root.ink
                    clip: true
                    onAccepted: edit.commit()
                    onTextChanged: if (text !== "") edit.warn = false
                    Text {
                        anchors.fill: parent; verticalAlignment: Text.AlignVCenter
                        visible: eTitle.text === ""
                        text: "要做什么"
                        font.pixelSize: root.f(58); font.weight: Font.Bold; color: root.line
                    }
                }
                Rectangle {
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    height: edit.warn ? 3 : 1
                    color: edit.warn ? root.accent : root.line
                }
            }
        }

        Flickable {
            anchors.top: eHead.bottom; anchors.topMargin: root.u(26)
            anchors.left: parent.left; anchors.right: parent.right
            anchors.bottom: parent.bottom
            anchors.leftMargin: root.u(56); anchors.rightMargin: root.u(56)
            anchors.bottomMargin: root.u(28)
            contentWidth: width; contentHeight: eCol.height + root.u(40); clip: true

            MouseArea { anchors.fill: parent; z: -1; onClicked: root.dropFocus() }

            Column {
                id: eCol
                width: parent.width
                spacing: root.u(22)


                Row {
                    spacing: root.u(12)
                    Repeater {
                        model: P.CATS
                        delegate: Chip {
                            required property var modelData
                            text: modelData.n
                            fs: root.f(25); pad: root.u(32)
                            tint: root.tintOf(modelData.k); accent: root.accentOf(modelData.k)
                            selected: edit.pickCat === modelData.k
                            onClicked: edit.pickCat = modelData.k
                        }
                    }
                }

                Text { text: "配图"; font.pixelSize: root.f(26); color: root.ink2 }
                Flickable {
                    width: parent.width; height: root.u(118)
                    contentWidth: eIllusRow.width; contentHeight: height
                    flickableDirection: Flickable.HorizontalFlick
                    clip: true
                    Row {
                        id: eIllusRow
                        spacing: root.u(14)
                        Repeater {
                            model: root.illusList
                            delegate: Rectangle {
                                required property var modelData
                                width: root.u(112); height: root.u(112)
                                radius: 14
                                color: root.card
                                border.color: edit.pickIllus === modelData.k ? root.accent : root.line
                                border.width: edit.pickIllus === modelData.k ? 3 : 1
                                Thumb {
                                    anchors.fill: parent; anchors.margins: 4
                                    radius: 11
                                    source: (modelData.k && modelData.k !== "none")
                                            ? root.img(modelData.k) : ""
                                }
                                Text {
                                    anchors.centerIn: parent
                                    visible: modelData.k === "" || modelData.k === "none"
                                    text: modelData.n
                                    font.pixelSize: root.f(23); color: root.ink2
                                    horizontalAlignment: Text.AlignHCenter
                                }
                                MouseArea {
                                    anchors.fill: parent
                                    onClicked: edit.pickIllus = modelData.k
                                }
                            }
                        }
                    }
                }

                Rectangle {
                    width: parent.width
                    height: root.u(104) * 4 + 3
                    radius: 18; color: root.card; border.color: root.line; border.width: 1
                    Column {
                        anchors.fill: parent

                        // 时间
                        Item {
                            width: parent.width; height: root.u(104)
                            Text {
                                id: eTimeLbl
                                anchors.left: parent.left; anchors.leftMargin: root.u(26)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(150)
                                text: "时间"; font.pixelSize: root.f(27); color: root.ink2
                            }
                            TextInput {
                                id: eTime
                                anchors.left: eTimeLbl.right
                                anchors.right: parent.right; anchors.rightMargin: root.u(26)
                                anchors.verticalCenter: parent.verticalCenter
                                height: root.u(60)
                                verticalAlignment: TextInput.AlignVCenter
                                font.pixelSize: root.f(29); color: root.ink
                                inputMask: "99:99"
                                Text {
                                    anchors.fill: parent; verticalAlignment: Text.AlignVCenter
                                    visible: eTime.text.replace(/[^0-9]/g, "") === ""
                                    text: "全天"; font.pixelSize: root.f(27); color: root.line
                                }
                            }
                        }
                        Rule { x: root.u(26); width: parent.width - root.u(52) }

                        // 日期
                        Item {
                            width: parent.width; height: root.u(104)
                            Text {
                                anchors.left: parent.left; anchors.leftMargin: root.u(26)
                                anchors.verticalCenter: parent.verticalCenter
                                text: "日期"; font.pixelSize: root.f(27); color: root.ink2
                            }
                            Row {
                                anchors.right: parent.right; anchors.rightMargin: root.u(26)
                                anchors.verticalCenter: parent.verticalCenter
                                spacing: root.u(16)
                                Chip {
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: "今天"; fs: root.f(23); pad: root.u(26)
                                    tint: root.tintOf(""); accent: root.accent
                                    selected: edit.pickDate === root.today
                                    onClicked: edit.pickDate = root.today
                                }
                                Image {
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.f(28); height: width
                                    sourceSize.width: width; sourceSize.height: width
                                    source: root.img("caret-left.svg"); fillMode: Image.PreserveAspectFit
                                    MouseArea { anchors.fill: parent; anchors.margins: -root.u(16)
                                                onClicked: edit.pickDate = root.shiftDate(edit.pickDate, -1) }
                                }
                                Text {
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.f(230); horizontalAlignment: Text.AlignHCenter
                                    text: edit.pickDate
                                          ? (P.mdText(edit.pickDate) + " "
                                             + P.weekText(new Date(edit.pickDate.replace(/-/g, "/"))).substring(2)) : ""
                                    font.pixelSize: root.f(28); color: root.ink
                                }
                                Image {
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.f(28); height: width
                                    sourceSize.width: width; sourceSize.height: width
                                    source: root.img("caret-right.svg"); fillMode: Image.PreserveAspectFit
                                    MouseArea { anchors.fill: parent; anchors.margins: -root.u(16)
                                                onClicked: edit.pickDate = root.shiftDate(edit.pickDate, 1) }
                                }
                            }
                        }
                        Rule { x: root.u(26); width: parent.width - root.u(52) }

                        // 备注
                        Item {
                            width: parent.width; height: root.u(104)
                            Text {
                                id: eNoteLbl
                                anchors.left: parent.left; anchors.leftMargin: root.u(26)
                                anchors.verticalCenter: parent.verticalCenter
                                width: root.f(150)
                                text: "备注"; font.pixelSize: root.f(27); color: root.ink2
                            }
                            TextInput {
                                id: eNote
                                anchors.left: eNoteLbl.right
                                anchors.right: parent.right; anchors.rightMargin: root.u(26)
                                anchors.verticalCenter: parent.verticalCenter
                                height: root.u(60)
                                verticalAlignment: TextInput.AlignVCenter
                                font.pixelSize: root.f(27); color: root.ink
                                clip: true
                                Text {
                                    anchors.fill: parent; verticalAlignment: Text.AlignVCenter
                                    visible: eNote.text === ""
                                    text: "可不填"; font.pixelSize: root.f(27); color: root.line
                                }
                            }
                        }
                        Rule { x: root.u(26); width: parent.width - root.u(52) }

                        // 完成
                        Item {
                            width: parent.width; height: root.u(104)
                            Text {
                                anchors.left: parent.left; anchors.leftMargin: root.u(26)
                                anchors.verticalCenter: parent.verticalCenter
                                text: "状态"; font.pixelSize: root.f(27); color: root.ink2
                            }
                            Chip {
                                anchors.right: parent.right; anchors.rightMargin: root.u(26)
                                anchors.verticalCenter: parent.verticalCenter
                                text: edit.pickDone ? "已完成" : "未完成"
                                fs: root.f(25); pad: root.u(36)
                                tint: root.tintOf(""); accent: root.accent
                                selected: edit.pickDone
                                onClicked: edit.pickDone = !edit.pickDone
                            }
                        }
                    }
                }

                Rectangle {
                    width: parent.width; height: root.f(92); radius: 16
                    color: root.accent
                    Text { anchors.centerIn: parent; text: edit.isNew ? "添加" : "保存"
                           font.pixelSize: root.f(30); color: root.ink }
                    MouseArea { anchors.fill: parent; onClicked: edit.commit() }
                }
                // 删除是安静的一行字, 不跟"保存"抢视觉重量
                Text {
                    anchors.horizontalCenter: parent.horizontalCenter
                    visible: !edit.isNew
                    text: "删掉这件事"
                    font.pixelSize: root.f(26); color: root.ink2
                    MouseArea {
                        anchors.fill: parent; anchors.margins: -root.u(20)
                        onClicked: {
                            var id = edit.item ? edit.item.id : ""
                            confirmBox.ask("删掉这件事？删了就找不回来了。", function () {
                                root.removeFrom("tasks", id)
                                edit.close()
                            })
                        }
                    }
                }
            }
        }
    }

    // ═══ 二次确认 (删除 / 清空这类不可逆动作) ═══════════════════════
    Rectangle {
        id: confirmBox
        anchors.fill: parent
        color: root.paper
        visible: false
        z: 25
        property string msg: ""
        property var act: null

        function ask(m, fn) { confirmBox.msg = m; confirmBox.act = fn; confirmBox.visible = true }
        function yes() {
            var fn = confirmBox.act
            confirmBox.visible = false; confirmBox.act = null
            if (fn) fn()
        }

        MouseArea { anchors.fill: parent }

        Column {
            anchors.centerIn: parent
            width: parent.width - root.u(112)
            spacing: root.u(30)
            Text {
                width: parent.width
                text: confirmBox.msg
                font.pixelSize: root.f(32); color: root.ink
                wrapMode: Text.WordWrap; horizontalAlignment: Text.AlignHCenter
            }
            Row {
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: root.u(20)
                Rectangle {
                    width: root.f(230); height: root.f(80); radius: 14
                    color: root.accent
                    Text { anchors.centerIn: parent; text: "确定"
                           font.pixelSize: root.f(28); color: root.ink }
                    MouseArea { anchors.fill: parent; onClicked: confirmBox.yes() }
                }
                Rectangle {
                    width: root.f(230); height: root.f(80); radius: 14
                    color: root.card; border.color: root.line; border.width: 1
                    Text { anchors.centerIn: parent; text: "再想想"
                           font.pixelSize: root.f(28); color: root.ink2 }
                    MouseArea { anchors.fill: parent
                                onClicked: { confirmBox.visible = false; confirmBox.act = null } }
                }
            }
        }
    }

    // 加号
    Rectangle {
        anchors.right: parent.right; anchors.rightMargin: root.u(36)
        anchors.bottom: tabbar.top; anchors.bottomMargin: root.u(28)
        width: root.u(116); height: width; radius: width / 2
        color: root.accent
        visible: root.ready && !sheet.visible && !triage.visible && !edit.visible
                 && !confirmBox.visible && root.tab !== "set"
        z: 10
        Rectangle {
            anchors.centerIn: parent
            width: parent.width * 0.34; height: root.u(6); radius: 3; color: root.ink
        }
        Rectangle {
            anchors.centerIn: parent
            width: root.u(6); height: parent.width * 0.34; radius: 3; color: root.ink
        }
        MouseArea {
            anchors.fill: parent
            onClicked: {
                if (root.tab === "inbox") sheet.open()
                else edit.openNew(root.tab === "cal" ? root.pickDate : root.today)
            }
        }
    }

    // ═══ 底部标签栏 ═════════════════════════════════════════════════
    Rectangle {
        id: tabbar
        anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom
        height: root.u(140)
        color: root.paper
        Rectangle { anchors.top: parent.top; anchors.left: parent.left; anchors.right: parent.right
                    height: 1; color: root.line }
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: root.u(16); anchors.rightMargin: root.u(16)
            spacing: 0
            Repeater {
                model: [
                    { k: "today", n: "今天",   ic: "calendar-check" },
                    { k: "cal",   n: "日历",   ic: "calendar-dots" },
                    { k: "inbox", n: "收集箱", ic: "tray" },
                    { k: "set",   n: "设置",   ic: "gear" }
                ]
                delegate: Item {
                    id: tabItem
                    required property var modelData
                    readonly property bool sel: root.tab === modelData.k
                    Layout.fillWidth: true; Layout.fillHeight: true
                    Rectangle {
                        anchors.centerIn: tabCol
                        // 要把图标和文字整个罩住: 图标 44 + 间距 6 + 文字 ~32 + 上下各 14
                        width: Math.max(root.u(140), tabCol.width + root.u(36))
                        height: tabCol.height + root.u(28)
                        radius: 16
                        color: tabItem.sel ? root.tabSel : "transparent"
                    }
                    Column {
                        id: tabCol
                        anchors.centerIn: parent; spacing: root.u(6)
                        Image {
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: root.f(44); height: width
                            sourceSize.width: width; sourceSize.height: width
                            source: root.img(tabItem.modelData.ic) + ".svg"
                            fillMode: Image.PreserveAspectFit
                        }
                        Text {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: tabItem.modelData.n
                            font.pixelSize: root.f(23)
                            font.weight: tabItem.sel ? Font.Bold : Font.Normal
                            color: root.ink
                        }
                    }
                    MouseArea { anchors.fill: parent
                                onClicked: { root.dropFocus(); root.tab = tabItem.modelData.k } }
                }
            }
        }
    }
}
