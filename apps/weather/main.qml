// 天气 — 「空间」内置应用。整屏接管 (space.chrome = false), 自带顶栏与底部标签。
// 版式按概念图: 首页(插画大卡) / 预报(逐小时·未来7天) / 地图(降水回波回放) / 城市 / 设置。
// 尺寸不写死: 一律走 space.unit (版式) 与 space.fontScale (字号), Move 与 Paper Pro 通用。
import QtQuick
import QtQuick.Layouts
import "weather.js" as W

Item {
    id: root
    property var space
    anchors.fill: parent

    readonly property string dir: "file://" + space.appDir + "/assets/"
    // 图片按 URL 缓存解码结果: 同名换内容不会重新读盘, 换了素材得重启 xochitl 才看得见。
    // 所以资源 URL 一律走这里带上外壳的 rev 令牌 (文件没动照常命中缓存, 动了自动失效)。
    function img(path) {
        var u = root.dir + path
        return (space && space.bust) ? space.bust(u) : u
    }
    readonly property real fs: space.fontScale
    readonly property real un: space.unit
    // 纵向单独一套 —— Paper Pro 宽是 Move 的 1.70 倍, 高却只有 1.27 倍。顶栏、温度行
    // 这些纵向位置再按 un 放大, 顶部就白占掉一大截 (实测 header 吃了 442px), 把插画上面
    // 那条白带挤没, 太阳只好缩成一小团。纵向位置与高度一律用 vu。
    readonly property real vn: space.vunit !== undefined ? space.vunit : space.unit
    function f(n) { return Math.round(n * root.fs) }
    function u(n) { return Math.round(n * root.un) }
    function vu(n) { return Math.round(n * root.vn) }
    // Paper Pro (宽屏) 的纵向预算格外紧: 画按屏宽等比铺开, 宽 1.70 倍就高 1.70 倍,
    // 而屏幕只高 1.27 倍。顶栏这些固定开销在它上面得比 vu 再收一档, 否则文字会被
    // 推到画的天空上。Move 上一律走原值, 版式不动。
    readonly property bool wide: space.largeScreen === true

    // ── 整屏常驻一个刷新波形标记 ──────────────────────────────────────────
// 症状: 进出本应用时, 只有插画那一块被整块闪黑重刷, 四周走的是另一种波形,
    // 于是两者交界处被刷出一道撕裂的硬边。原因不在图片边缘够不够柔 (直边、毛边、
    // 模糊都试过), 而在于屏幕把这块彩色照片和周围的白底文字分给了不同的刷新波形。
    // 办法: 整屏一种波形就没有交界可撕 —— 这件事现在由「空间」外壳统一做
    // (设置 → 刷新效果: 高清 / 快刷), 应用跑在 appHost 里自然继承, 不要再自己挂:
    // 两个 ScreenModeItem 叠在一起, 后挂的那个不一定赢。

    // 墨水屏没有背光, 浅色底要靠抖动铺, 看起来就发灰。一律纯白, 层次交给描边。
    readonly property color paper: "#FFFFFF"
    readonly property color card: "#FFFFFF"
    readonly property color line: "#B4AFA6"   // 原 #D8D5CE 亮度 85%, 墨水屏上几乎看不见
    readonly property color ink: "#000000"   // 关键信息一律纯黑, 墨水屏上深灰会往灰里塌
    readonly property color ink2: "#1F1E1B"   // 这块屏显示出来本就发灰, 次要文字只比正文浅一点点就够
    readonly property color ink3: "#4A4842"   // 只给"未选中"这类需要明显弱化的状态

    property var cfg: null            // {cities:[…], current, unit, refresh}
    property var cache: ({})          // cityKey → wx
    property var wx: null           // 当前城市的天气
    property string tab: "home"       // home | forecast | life | city | setting
    property string fcTab: "hourly"   // hourly | daily
    property string status: ""
    property bool busy: false
    property var results: []          // 城市检索结果

    function city() { return (cfg && cfg.cities.length) ? cfg.cities[Math.min(cfg.current, cfg.cities.length - 1)] : W.DEFAULT_CITY }
    function unit() { return cfg ? cfg.unit : "c" }
    function t(c) { return W.toUnit(c, unit()) }          // 摄氏 → 显示值
    function icon(code, day) { return root.img(W.codeIcon(code, day) + ".svg") }

    // 首页那一句: 句子在 assets/phrases.json, 按天气分组, 同一天内不变。
    // 用 XHR 读而不是 import —— 相对 import 不带 ?t= 令牌, 改了要重启 xochitl;
    // XHR 每次真读盘, 改完重开应用就生效, 你可以自己去编辑那个文件。
    property var phrases: null
    function loadPhrases() {
        var u = root.img("phrases.json")
        root.space.request("GET", u, null, function (st, r) {
            // file:// 读成功时 status 是 0 不是 200, 所以只看有没有解析出东西
            if (r) root.phrases = r
            else console.warn("[weather] phrases 读不到 (st=" + st + ") " + u)
        })
    }
    // 分组判断放在这里而不是 weather.js: .js 是相对 import, 不带 ?t= 令牌,
    // 改了必须重启 xochitl 才生效。放 QML 里重开应用就行。
    // isSnowy / isRainy 是 weather.js 里早就有的函数, 直接复用。
    function adviceKind(d) {
        if (!d) return "clear"
        if (d.code >= 95) return "storm"
        if (W.isSnowy(d.code)) return "snow"
        if (W.isRainy(d.code)) return "rain"
        if (d.code === 45 || d.code === 48) return "fog"
        if (d.aqi > 150) return "haze"
        if (d.temp >= 32) return "hot"
        if (d.temp <= 0) return "cold"
        if (d.code <= 1) return "clear"
        return "cloudy"
    }
    // 今日一句。写成绑定而不是函数, 有两个原因:
    //   · 函数里引用的属性不进依赖图 —— phrases 是异步读进来的, 等它到了, 挂在
    //     text: todayLine() 上的绑定不会重算, 界面永远停在"还没读到"那一版。
    //   · 日期取 space.localNow() (设备时区), 原来写的 root.now 这个属性压根不存在,
    //     对 undefined 取 getFullYear 直接抛异常, 整条绑定求值失败 → 那行空白。
    readonly property string adviceLine: {
        if (!root.wx) return ""
        var k = root.adviceKind(root.wx)
        var l = (root.phrases && root.phrases[k]) ? root.phrases[k] : null
        if (!l || !l.length) return root.wx.advice || ""   // 兜底: 老版整句
        var d = root.space.localNow()
        var seed = d.getFullYear() * 400 + d.getMonth() * 32 + d.getDate()
        return l[seed % l.length]
    }

    // ─── 数据 ───
    // 本地缓存读完才算"可以画了"。在此之前首页整块不显示:
    // 否则先画一次没数据的空壳, 缓存回来再重排一次, 墨水屏就白闪一遍。
    property bool ready: false
    function load() {
        space.dataGet(space.appId, "config", function(st, r) {
            cfg = (st === 200 && r && r.cities && r.cities.length) ? r : W.DEFAULT_CONFIG
            space.dataGet(space.appId, "cache", function(st2, r2) {
                cache = (st2 === 200 && r2) ? r2 : ({})
                show()
                ready = true
                if (!wx || Date.now() - wx.at > cfg.refresh * 1000) refresh()
            })
        })
    }
    function show() {
        var c = cache[W.cityKey(city())]
        wx = (c && c.temp !== undefined) ? c : null
    }
    // 提交配置: 换一个新对象, 否则 QML 认为 var 属性没变, 界面不刷新
    function commit() { cfg = Object.assign({}, cfg, { cities: cfg.cities.slice() }) }
    function saveCfg(cb) { space.dataPut(space.appId, "config", cfg, cb) }
    function refresh() {
        if (busy || !cfg) return
        busy = true; status = "正在获取 " + city().name + " 的天气…"
        W.fetchCity(space, city(), function(d, e) {
            busy = false
            if (!d) { status = e + (wx ? "，下面是上次的数据" : ""); return }
            status = ""
            var c = cache; c[W.cityKey(city())] = d; cache = c; wx = d
            space.dataPut(space.appId, "cache", cache, null)
        })
    }
    function selectCity(i) {
        cfg.current = i; commit()
        saveCfg(null); show(); tab = "home"
        if (!wx || Date.now() - wx.at > cfg.refresh * 1000) refresh()
    }
    function addCity(c) {
        for (var i = 0; i < cfg.cities.length; i++)
            if (W.cityKey(cfg.cities[i]) === W.cityKey(c)) { selectCity(i); return }
        cfg.cities.push(c); cfg.current = cfg.cities.length - 1; commit()
        saveCfg(null); results = []; searchInput.text = ""; show(); refresh()
    }
    function removeCity(i) {
        if (cfg.cities.length <= 1) { status = "至少保留一个城市"; return }
        cfg.cities.splice(i, 1)
        if (cfg.current >= cfg.cities.length) cfg.current = cfg.cities.length - 1
        commit(); saveCfg(null); show()
    }
    function search() {
        var q = searchInput.text.trim()
        if (q === "") return
        busy = true; status = "正在查找 " + q + "…"
        W.geocode(space, q, function(list, e) { busy = false; results = list; status = e })
    }

    Component.onCompleted: {
        root.loadPhrases()
        space.chrome = false
        space.onBack = function() { if (root.tab !== "home") { root.tab = "home"; return true } return false }
        load()
    }
    // 注意: 不要在 Component.onDestruction 里清 space 上的共享状态 (onBack / chrome / title)。
    // QML 的 destroy() 是延后执行的, 应用跳应用时旧应用的 onDestruction 会在新应用
    // Component.onCompleted 之后才跑, 把新应用刚设好的东西抹掉。外壳在 unmount() 里
    // 已经统一复位, 应用不用管。
    Timer {   // 到点自动刷新 (设置里可调)
        interval: 60000; repeat: true; running: root.visible
        onTriggered: if (root.cfg && root.wx && Date.now() - root.wx.at > root.cfg.refresh * 1000) root.refresh()
    }

    Rectangle { anchors.fill: parent; color: root.paper }

    // ═══ 顶栏 ═══
    Item {
        id: header
        anchors.top: parent.top; anchors.left: parent.left; anchors.right: parent.right
        height: root.wide ? root.vu(190) : root.vu(260)

        Image {
            id: backBtn
            x: root.u(28); y: root.wide ? root.vu(46) : root.vu(60)
            width: root.f(34); height: width; sourceSize.width: width; sourceSize.height: width
            source: root.img("i-back.svg"); fillMode: Image.PreserveAspectFit
            MouseArea { anchors.fill: parent; anchors.margins: -root.u(18); onClicked: root.tab === "home" ? space.exit() : root.tab = "home" }
        }
        Text {
            anchors.horizontalCenter: parent.horizontalCenter
            y: root.wide ? root.vu(48) : root.vu(64)
            text: root.tab === "home" ? "" : (root.tab === "forecast" ? "预报" : root.tab === "map" ? "天气地图" : root.tab === "city" ? "城市管理" : "设置")
            font.pixelSize: root.f(30); font.weight: Font.Medium; color: root.ink
        }
        // 首页顶栏: 城市 + 日期居中 (其它页在中间显示页名, 右上角显示城市)
        Column {
            // 宽度必须显式给满: 子项又按父宽居中、父宽又由子项撑开的话,
            // 两边互相依赖, 居中会算偏。
            visible: root.tab === "home"
            anchors.left: parent.left; anchors.right: parent.right
            y: root.wide ? root.vu(78) : root.vu(118)
            spacing: 2
            // 城市名自己居中, 定位图标挂在它左边。
            // 图标若和文字一起算进居中范围, 文字就会被推得偏右, 看着不齐。
            Item {
                width: parent.width; height: cityName.height
                Text {
                    id: cityName
                    anchors.horizontalCenter: parent.horizontalCenter
                    text: root.city().name; font.pixelSize: root.f(54)
                    font.weight: Font.Medium; color: root.ink
                }
                Image {
                    anchors.right: cityName.left; anchors.rightMargin: root.u(10)
                    anchors.verticalCenter: cityName.verticalCenter
                    width: root.f(40); height: width; sourceSize.width: width; sourceSize.height: width
                    // 实心版 (Phosphor map-pin-fill): 顶栏这颗要压得住加粗的城市名;
                    // 底部"城市"页签仍用描边版 i-pin.svg, 与其它页签统一。
                    source: root.img("i-pin-fill.svg"); fillMode: Image.PreserveAspectFit
                }
            }
            Text {
                anchors.horizontalCenter: parent.horizontalCenter
                text: root.wx ? (root.wx.date.substring(5, 7) + "月" + root.wx.date.substring(8, 10) + "日 " + W.weekday(root.wx.date)) : ""
                font.pixelSize: root.f(34); color: root.ink2
            }
        }
        Text {
            visible: root.tab !== "home" && root.tab !== "city" && root.tab !== "setting"
            anchors.right: refreshBtn.left; anchors.rightMargin: root.u(20); y: root.u(64)
            text: root.city().name; font.pixelSize: root.f(24); color: root.ink2
        }
        Image {
            id: refreshBtn
            anchors.right: parent.right; anchors.rightMargin: root.u(28); y: root.u(58)
            width: root.f(32); height: width; sourceSize.width: width; sourceSize.height: width
            source: root.img(root.tab === "city" ? "i-plus.svg" : "i-refresh.svg"); fillMode: Image.PreserveAspectFit
            opacity: root.busy ? 0.35 : 1
            MouseArea {
                anchors.fill: parent; anchors.margins: -root.u(18)
                onClicked: { if (root.tab === "city") searchInput.forceActiveFocus(); else root.refresh() }
            }
        }
    }

    // 状态提示条 (4 秒自动消失)
    Rectangle {
        id: toast
        visible: root.status !== ""
        z: 5
        anchors.top: header.bottom; anchors.left: parent.left; anchors.right: parent.right
        anchors.leftMargin: root.u(28); anchors.rightMargin: root.u(28)
        height: toastText.implicitHeight + root.u(16)
        color: "#2B2B2B"; radius: 8
        Text { id: toastText; anchors.fill: parent; anchors.margins: root.u(8)
               text: root.status; color: "white"; font.pixelSize: root.f(21); wrapMode: Text.WordWrap }
        Timer { interval: 4000; repeat: false; running: root.status !== "" && !root.busy; onTriggered: root.status = "" }
    }

    // ═══ 内容区 ═══
    Item {
        id: content
        anchors.top: header.bottom; anchors.bottom: tabbar.top
        anchors.left: parent.left; anchors.right: parent.right
        anchors.leftMargin: root.u(28); anchors.rightMargin: root.u(28)

        // ─────── 首页 (版式按概念图: 插画整幅铺底, 温度与文字压在天空上) ───────
        Item {
            visible: root.tab === "home" && root.ready
            anchors.fill: parent

            // 插画: 从内容区顶部一直铺到速览行, 顶部对齐 (画的上半是天, 正好垫文字)
            Item {
                id: sceneBox
                anchors.top: parent.top
                anchors.left: parent.left; anchors.right: parent.right
                anchors.bottom: statRow.top; anchors.bottomMargin: root.vu(14)
                clip: true
                Image {
                    id: sceneImg
                    // 真素材都在 scenes/ 下; 用路径判断, 不受候选链更新时序影响
                    readonly property bool useFallback: String(source).indexOf("/scenes/") < 0
                    readonly property real sw: sourceSize.width > 0 ? sourceSize.width : 1
                    readonly property real sh: sourceSize.height > 0 ? sourceSize.height : 1
                    // 素材是 2:1 的横幅: 整幅贴着底边画, 按原比例不裁不拉,
                    // 上面空出来的白底正好垫大号温度 —— 两者不重叠, 所以不需要
                    // 任何渐隐或蒙版 (老素材是竖幅铺底、文字压在画上才需要)。
                    width: parent.width
                    // 宽屏上按原比例算出来的高度会吃掉整块 (1620 宽 → 786 高), 文字只好
                    // 压到画的天空里。给个占比上限, Stretch 下等于轻微压扁 —— 实测 5%
                    // 左右, 看不出来, 却把画顶边压低了近百像素, 文字和太阳都有地方了。
                    height: useFallback ? parent.height
                            : (root.wide ? Math.min(Math.round(parent.width * sh / sw),
                                                    Math.round(parent.height * 0.60))
                                         : Math.round(parent.width * sh / sw))
                    x: 0
                    y: useFallback ? 0 : Math.max(0, parent.height - height)
                    fillMode: useFallback ? Image.PreserveAspectFit : Image.Stretch
                    // 选图顺序由 sceneChain 给出: 本季本款 → 本季近似 → 邻近季节 → 矢量插画
                    // 天气没回来时直接给空链 (不依赖 weather.js 的新版本:
                    // .js 按 URL 缓存, 不重启 xochitl 不生效)
                    property var chain: root.wx ? W.sceneChain(root.wx, root.space.localNow()) : []
                    property int step: 0
                    // 天气没回来时不给 source: 空着不画, 避免先上占位图再换真图多刷一次
                    visible: status === Image.Ready
                    source: chain.length ? root.img(chain[0]) : ""
                    onChainChanged: { step = 0; source = chain.length ? root.img(chain[0]) : "" }
                    onStatusChanged: {
                        if (status !== Image.Error) return
                        if (step + 1 >= chain.length) return
                        step += 1
                        source = root.img(chain[step])
                    }
                }

            }

            // 天气配图: 当背景使 —— z: -1 压在文字下层, 且不参与任何布局。
            // 这一点很关键: 原来它是 RowLayout 的子项, 于是和温度那列抢宽度,
            // 挤起来要么图被收窄要么文字被切, 还得手算"体感那行右边缘在 x≈466"
            // 这种魔法数字。当背景之后, 想多大就多大, 谁也不碍着谁。
            // 素材按 sceneKind 的取值命名: w-art-<kind>.jpg, 缺的 (fog/hail) 留空不画。
            Image {
                readonly property string kind: root.wx ? W.sceneKind(root.wx.code) : ""
                readonly property var haveArt: ["clear", "cloudy", "rain", "snow", "storm"]
                z: -1
                // 这张图 z:-1 压在最底层, 而山水画 (sceneImg) 贴着 sceneBox 底边、按屏宽
                // 等比放大。屏越宽画越高, 吃掉的纵向空间越多 —— Paper Pro 上画高 701 而
                // Move 上只有 413, 固定边距的太阳下半就被画盖掉了 (实测只剩 54%)。
                // 所以按"画上面那条白带"来放: 贴着画的顶边往上摆, 高度不超过白带。
                readonly property real band: sceneBox.height - (sceneImg.visible ? sceneImg.height : 0)
                readonly property real topGap: kind === "clear" ? root.vu(28) : root.vu(22)
                // 白带有多少就用多少 (不超过设计尺寸): 屏宽时画高、白带窄, 自动缩;
                // 纵向宽裕时就按设计值铺开, 不会一直缩在角上。
                // 尺寸按字号比例 (f) 而不是版式比例 (u): 它旁边就是大号温度, 两者的相对
                // 大小才是眼睛看到的东西。u 在 Paper Pro 上是 1.70 倍而 f 只有 1.245 倍,
                // 按 u 放大太阳就比温度大出一圈 —— 两台看上去是两个设计。
                readonly property real artH: kind === "clear"
                        ? Math.max(root.f(90), Math.min(root.f(215), band - topGap - root.vu(14)))
                        : Math.max(root.f(120), Math.min(root.f(310), band - topGap - root.vu(10)))
                anchors.top: sceneBox.top
                anchors.topMargin: topGap
                anchors.right: parent.right
                anchors.rightMargin: kind === "clear" ? root.u(64) : root.u(20)
                // 框按图形定, 不是一刀切 520: PreserveAspectFit 会把图在框里居中,
                // 太阳是正方的只占 246 宽, 在 520 的框里左右各空 137 —— 看着就离右边远。
                // 给它一个贴合的窄框, 它才真正靠右。扁的那几张本来就能撑满 520。
                // 宽度跟着高度走: 太阳是正方的, 宽框钉死 230 的话 PreserveAspectFit 会
                // 被宽度卡住 —— 白带再宽它也长不大 (实测白带 573 而太阳只有 391)。
                width: kind === "clear" ? Math.max(root.f(230), artH) : root.f(520)
                height: artH
                // day !== false 而不是 day === true: 离线时读的 last.json 可能是
                // 老格式 (没存 day 字段), 那种情况下按白天处理, 总比不显示强。
                visible: !!root.wx && root.wx.day !== false
                         && source != "" && status === Image.Ready
                source: haveArt.indexOf(kind) >= 0
                        ? root.img("w-art-" + kind + ".jpg") : ""
                sourceSize.width: 800; sourceSize.height: 800
                fillMode: Image.PreserveAspectFit
            }

            // 温度 + 天气文字
            RowLayout {
                // 上移 5mm: 屏幕 229 DPI, 1mm ≈ 9px, 所以 88 - 45 = 43
                anchors.top: sceneBox.top; anchors.topMargin: root.wide ? root.vu(8) : root.vu(43)
                anchors.left: parent.left; anchors.right: parent.right
                ColumnLayout {
                    Layout.leftMargin: root.u(70)
                    spacing: 2
                    RowLayout {
                        spacing: root.u(8)
                        Text {
                            text: root.wx ? String(root.t(root.wx.temp)) : "--"
                            font.pixelSize: root.f(210); font.weight: Font.Light; color: root.ink
                        }
                        Text {
                            Layout.alignment: Qt.AlignTop; Layout.topMargin: root.f(42)
                            text: W.unitSign(root.unit()); font.pixelSize: root.f(46); color: root.ink
                        }
                    }
                    Text {
                        text: root.wx ? root.wx.text : (root.busy ? "获取中…" : "暂无数据")
                        font.pixelSize: root.f(48); font.weight: Font.Bold; color: root.ink
                    }
                    Text {
                        visible: !!root.wx
                        text: root.wx ? ("体感 " + root.t(root.wx.feels) + "°   ↑ " + root.t(root.wx.hi) + "°  ↓ " + root.t(root.wx.lo) + "°") : ""
                        font.pixelSize: root.f(34); color: root.ink
                    }
                }
                Item { Layout.fillWidth: true }
            }

            // 三项速览
            Rectangle {
                id: statRow
                anchors.left: parent.left; anchors.right: parent.right
                anchors.bottom: adviceRule.top; anchors.bottomMargin: root.vu(20)
                height: root.vu(150)
                color: "transparent"
                RowLayout {
                    anchors.fill: parent; anchors.topMargin: root.u(16); anchors.bottomMargin: root.u(6)
                    spacing: 0
                    Repeater {
                        model: [
                            { ic: "i-wind", label: root.wx ? W.windDir(root.wx.windDeg) : "风", val: root.wx ? (W.windLevel(root.wx.wind) + " 级") : "--" },
                            { ic: "i-humidity", label: "湿度", val: root.wx ? (root.wx.humidity + "%") : "--" },
                            { ic: "i-leaf", label: "空气质量", val: (root.wx && root.wx.aqi !== null) ? (W.aqiText(root.wx.aqi) + " " + root.wx.aqi) : "--" }
                        ]
                        delegate: Item {
                            required property var modelData
                            required property int index
                            Layout.fillWidth: true; Layout.fillHeight: true
                            Row {
                                anchors.centerIn: parent; spacing: root.u(24)
                                Image {
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.f(56); height: width; sourceSize.width: width; sourceSize.height: width
                                    source: root.img(modelData.ic + ".svg"); fillMode: Image.PreserveAspectFit
                                }
                                Column {
                                    spacing: 2
                                    Text { text: modelData.label; font.pixelSize: root.f(30); color: root.ink }
                                    Text { text: modelData.val; font.pixelSize: root.f(32); font.weight: Font.Medium; color: root.ink }
                                }
                            }
                            Rectangle {
                                visible: index > 0
                                anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter
                                width: 1; height: parent.height * 0.62; color: root.line
                            }
                        }
                    }
                }
            }

            // 书页式: 插画下面不压线, 只在最后这句建议上面压一条, 像注释的分界
            Rectangle {
                id: adviceRule
                anchors.left: parent.left; anchors.right: parent.right
                anchors.bottom: adviceText.top; anchors.bottomMargin: root.u(20)
                height: 1; color: root.line
            }

            Text {
                id: adviceText
                anchors.left: parent.left; anchors.right: parent.right
                anchors.bottom: parent.bottom; anchors.bottomMargin: root.vu(20)
                // 高度写死一行: 文字空着时高度归零会把上面那条细线拽下去,
                // 数据回来又弹回去, 等于多刷一次屏。
                height: root.f(36) * 1.5
                verticalAlignment: Text.AlignVCenter
                text: root.adviceLine
                font.pixelSize: root.f(36); color: root.ink; wrapMode: Text.WordWrap
            }
        }

        // ─────── 预报 ───────
        Item {
            visible: root.tab === "forecast"
            anchors.fill: parent

            Row {   // 段控
                id: seg
                anchors.top: parent.top; anchors.topMargin: root.u(10)
                anchors.horizontalCenter: parent.horizontalCenter
                spacing: root.u(10)
                Repeater {
                    model: [{ k: "hourly", n: "逐小时预报" }, { k: "daily", n: "未来 7 天" }]
                    delegate: Rectangle {
                        required property var modelData
                        readonly property bool sel: root.fcTab === modelData.k
                        width: root.f(250); height: root.f(72); radius: height / 2
                        color: sel ? root.ink : "transparent"
                        border.color: sel ? root.ink : root.line; border.width: 1
                        Text { anchors.centerIn: parent; text: modelData.n; font.pixelSize: root.f(30)
                               color: parent.sel ? "white" : root.ink2 }
                        MouseArea { anchors.fill: parent; onClicked: root.fcTab = modelData.k }
                    }
                }
            }
            Text {
                id: fcDate
                anchors.top: seg.bottom; anchors.topMargin: root.u(14)
                anchors.horizontalCenter: parent.horizontalCenter
                visible: root.fcTab === "hourly" && !!root.wx
                text: root.wx ? (root.wx.date.substring(5, 7) + "月" + root.wx.date.substring(8, 10) + "日 " + W.weekday(root.wx.date)) : ""
                font.pixelSize: root.f(28); color: root.ink2
            }
            Rectangle {
                anchors.top: fcDate.visible ? fcDate.bottom : seg.bottom
                anchors.topMargin: root.u(12)
                anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom
                anchors.bottomMargin: root.u(10)
                color: "transparent"; clip: true

                // 逐小时
                Flickable {
                    id: hourFlick
                    visible: root.fcTab === "hourly"
                    anchors.fill: parent; anchors.margins: root.u(6)
                    contentWidth: width; contentHeight: hourCol.height; clip: true
                    Column {
                        id: hourCol
                        width: parent.width
                        Repeater {
                            model: root.wx ? root.wx.hours : []
                            delegate: Item {
                                required property var modelData
                                required property int index
                                width: hourCol.width; height: root.u(104)
                                Text { x: root.u(24); anchors.verticalCenter: parent.verticalCenter
                                       text: modelData.time; font.pixelSize: root.f(32); color: root.ink }
                                Image {
                                    anchors.centerIn: parent
                                    width: root.f(54); height: width; sourceSize.width: 110; sourceSize.height: 110
                                    source: root.icon(modelData.code, modelData.day); fillMode: Image.PreserveAspectFit
                                }
                                Text {
                                    anchors.right: parent.right; anchors.rightMargin: root.u(24); anchors.verticalCenter: parent.verticalCenter
                                    text: root.t(modelData.temp) + "°"; font.pixelSize: root.f(34); color: root.ink
                                }
                                Rectangle { anchors.bottom: parent.bottom; anchors.left: parent.left; anchors.right: parent.right
                                            anchors.leftMargin: root.u(20); anchors.rightMargin: root.u(20)
                                            height: 1; color: root.line; visible: index < (root.wx ? root.wx.hours.length - 1 : 0) }
                            }
                        }
                    }
                }

                // 未来 7 天
                Flickable {
                    id: dayFlick
                    visible: root.fcTab === "daily"
                    anchors.fill: parent; anchors.margins: root.u(6)
                    contentWidth: width; contentHeight: dayCol.height; clip: true
                    Column {
                        id: dayCol
                        width: parent.width
                        Repeater {
                            model: root.wx ? root.wx.days : []
                            delegate: Item {
                                required property var modelData
                                required property int index
                                width: dayCol.width
                                // 七行按可用高度均分, 页面不留空白
                                height: {
                                    var n = root.wx && root.wx.days ? root.wx.days.length : 0
                                    return n > 0 ? Math.floor(dayFlick.height / n) : root.u(104)
                                }
                                Column {
                                    x: root.u(24); anchors.verticalCenter: parent.verticalCenter; spacing: root.u(4)
                                    Text { text: index === 0 ? "今天" : W.weekday(modelData.date); font.pixelSize: root.f(34); color: root.ink }
                                    Text { text: W.mmdd(modelData.date); font.pixelSize: root.f(26); color: root.ink2 }
                                }
                                Image {
                                    anchors.horizontalCenter: parent.horizontalCenter; anchors.horizontalCenterOffset: -root.u(40)
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.f(62); height: width; sourceSize.width: 124; sourceSize.height: 124
                                    source: root.icon(modelData.code, true); fillMode: Image.PreserveAspectFit
                                }
                                Column {
                                    anchors.right: parent.right; anchors.rightMargin: root.u(24)
                                    anchors.verticalCenter: parent.verticalCenter
                                    spacing: root.u(4)
                                    Text { anchors.right: parent.right
                                           text: root.t(modelData.lo) + "° / " + root.t(modelData.hi) + "°"
                                           font.pixelSize: root.f(34); color: root.ink }
                                    Text { anchors.right: parent.right; text: W.codeText(modelData.code)
                                           font.pixelSize: root.f(28); color: root.ink }
                                }
                                Rectangle { anchors.bottom: parent.bottom; anchors.left: parent.left; anchors.right: parent.right
                                            anchors.leftMargin: root.u(20); anchors.rightMargin: root.u(20)
                                            height: 1; color: root.line; visible: index < (root.wx ? root.wx.days.length - 1 : 0) }
                            }
                        }
                    }
                }
            }
        }

        // ─────── 地图 (降水回波 + 过去两小时回放) ───────
        //
        // 整张图由后端合成 (/space/weather/map): 底图瓦片拼好、叠上雷达、按墨水屏调色,
        // 前端只拿一张 JPEG。理由见 upload-server-go/internal/space/weathermap.go 的注释:
        // 墨水屏每多一次局部刷新就多一次闪烁, 一张整图只刷一次。
        //
        // 时间轴是"回放过去两小时", 不是预报 —— RainViewer 免费接口在中国区
        // nowcast 帧数为 0, 只有过去 13 帧 (每 10 分钟一帧)。
        Item {
            id: mapPage
            visible: root.tab === "map"
            anchors.fill: parent

            property string layer: "rain"    // rain | aqi
            property var cities: []          // [{n,x,y,r}] 视口内的城市, 中文名在这边画
            property var hours: []           // [{idx, label}] 从当前整点起的逐小时
            property int sel: 0              // 选中第几个小时 (0 = 现在)
            readonly property var cur: (sel >= 0 && sel < hours.length) ? hours[sel] : null

            function load() {
                if (hours.length) return
                root.space.get("/space/weather/map/hours?z=8&lat=" + root.city().lat
                               + "&lon=" + root.city().lon, function (st, r) {
                    if (st !== 200 || !r || !r.hours || !r.hours.length) return
                    mapPage.hours = r.hours
                    mapPage.sel = 0
                })
                mapPage.loadCities()
            }
            function loadCities() {
                if (mapBox.width <= 0) return
                root.space.get("/space/weather/map/cities?z=8&lat=" + root.city().lat
                               + "&lon=" + root.city().lon
                               + "&w=" + Math.round(mapImg.width) + "&h=" + Math.round(mapImg.height),
                               function (st, r) {
                                   if (st === 200 && r && r.cities) mapPage.cities = r.cities
                               })
            }
            onVisibleChanged: if (visible) load()
            // 换了城市要重新取 (网格是按城市中心拉的)
            Connections {
                target: root
                function onCfgChanged() { mapPage.hours = []; if (mapPage.visible) mapPage.load() }
            }

            function stamp(h) {
                if (!h) return ""
                return (mapPage.sel === 0 ? "现在  " : "") + h.label
            }

            // 层级标签 (照原型)。只放有数据源的两层 —— 温度和风场还没接,
            // 摆上去点了没反应不如不摆。
            Row {
                id: layerTabs
                anchors.top: parent.top; anchors.topMargin: root.u(4)
                anchors.left: parent.left
                spacing: root.u(40)
                Repeater {
                    model: [{ k: "rain", n: "降水" }, { k: "aqi", n: "空气质量" }]
                    delegate: Item {
                        required property var modelData
                        readonly property bool sel: mapPage.layer === modelData.k
                        width: lt.width; height: root.f(56)
                        Text {
                            id: lt
                            anchors.horizontalCenter: parent.horizontalCenter
                            anchors.top: parent.top
                            text: modelData.n
                            font.pixelSize: root.f(30)
                            font.weight: parent.sel ? Font.Bold : Font.Normal
                            color: root.ink
                        }
                        Rectangle {
                            anchors.bottom: parent.bottom
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: lt.width; height: 3
                            visible: parent.sel; color: root.ink
                        }
                        MouseArea { anchors.fill: parent; anchors.margins: -root.u(14)
                                    onClicked: mapPage.layer = modelData.k }
                    }
                }
            }

            Rectangle {
                id: mapBox
                anchors.top: layerTabs.bottom; anchors.topMargin: root.u(10)
                anchors.left: parent.left; anchors.right: parent.right
                anchors.bottom: legendRow.top; anchors.bottomMargin: root.u(18)
                color: "#FAF9F6"
                border.color: root.line; border.width: 1
                clip: true

                Image {
                    id: mapImg
                    anchors.fill: parent
                    anchors.margins: 1
                    fillMode: Image.PreserveAspectCrop
                    cache: false            // 每帧都是不同 URL, 缓存只会白占内存
                    asynchronous: true
                    source: (mapPage.cur && width > 0)
                            ? (root.space.baseUrl + "/space/weather/map"
                               + "?lat=" + root.city().lat + "&lon=" + root.city().lon
                               + "&z=8&w=" + Math.round(width) + "&h=" + Math.round(height)
                               + "&layer=" + mapPage.layer + "&hour=" + mapPage.cur.idx)
                            : ""
                }
                Text {
                    anchors.centerIn: parent
                    visible: mapImg.status !== Image.Ready
                    text: mapPage.hours.length === 0
                          ? (mapPage.layer === "aqi" ? "正在取空气质量…" : "正在取降水预报…")
                          : "正在上墨…"
                    font.pixelSize: root.f(26); color: root.ink2
                }
                // 城市点与中文名。位置由后端按同一套墨卡托投影算好 (像素坐标),
                // 这边只管画 —— Go 渲染中日韩文字要带字体和 opentype 依赖, 不划算,
                // 而且 QML 画出来的文字在墨水屏上更清楚。
                Repeater {
                    model: mapImg.status === Image.Ready ? mapPage.cities : []
                    delegate: Item {
                        required property var modelData
                        x: modelData.x; y: modelData.y
                        Rectangle {
                            anchors.centerIn: parent
                            width: root.f(9); height: width; radius: width / 2
                            color: root.ink
                        }
                        Text {
                            anchors.horizontalCenter: parent.horizontalCenter
                            anchors.top: parent.verticalCenter; anchors.topMargin: root.u(9)
                            text: modelData.n
                            font.pixelSize: root.f(modelData.r <= 4 ? 25 : 21)
                            font.weight: modelData.r <= 4 ? Font.Bold : Font.Normal
                            color: root.ink
                        }
                    }
                }

                // 当前城市的位置标记: 视口是以它为中心合成的, 所以就在正中
                Rectangle {
                    anchors.centerIn: parent
                    visible: mapImg.status === Image.Ready
                    width: root.f(18); height: width; radius: width / 2
                    color: "transparent"; border.color: root.ink; border.width: 3
                    Rectangle {
                        anchors.centerIn: parent
                        width: parent.width * 0.42; height: width; radius: width / 2
                        color: root.ink
                    }
                }
                // 当前城市的名字由上面的 Repeater 画 (它一定在视口中心), 这里不重复
            }

            // 图例: 弱 → 强
            RowLayout {
                id: legendRow
                anchors.left: parent.left; anchors.right: parent.right
                anchors.bottom: playRow.top; anchors.bottomMargin: root.u(18)
                spacing: root.u(14)
                Text { text: mapPage.layer === "aqi" ? "优" : "弱"
                       font.pixelSize: root.f(24); color: root.ink2 }
                Row {
                    Layout.fillWidth: true
                    Layout.preferredHeight: root.u(18)
                    Repeater {
                        model: 7
                        delegate: Rectangle {
                            required property int index
                            width: legendRow.width / 9
                            height: root.u(18)
                            // 和后端 renderField 的色阶一致 (改一边必须改另一边,
                            // 否则图例和图上的浓淡对不上):
                            //   降水 #A9C6F0 → #1F5FC0   空气质量 #C4D6BA → #965648
                            readonly property real t: index / 6
                            color: mapPage.layer === "aqi"
                                   ? Qt.rgba((196 + (150 - 196) * t) / 255,
                                             (214 + ( 86 - 214) * t) / 255,
                                             (186 + ( 72 - 186) * t) / 255, 1)
                                   : Qt.rgba((169 + ( 31 - 169) * t) / 255,
                                             (198 + ( 95 - 198) * t) / 255,
                                             (240 + (192 - 240) * t) / 255, 1)
                        }
                    }
                }
                Text { text: mapPage.layer === "aqi" ? "重污染" : "强"
                       font.pixelSize: root.f(24); color: root.ink2 }
            }

            // 播放条
            Item {
                id: playRow
                anchors.left: parent.left; anchors.right: parent.right
                anchors.bottom: parent.bottom; anchors.bottomMargin: root.u(10)
                height: root.u(150)

                // 时刻: 点下面的刻度切换。不做自动播放 —— 每帧都要重新合成一张图,
                // 墨水屏跟不上, 而且冷帧要等好几秒, 播起来是一顿一顿的。
                Text {
                    anchors.left: parent.left
                    anchors.top: parent.top
                    text: mapPage.stamp(mapPage.cur)
                    font.pixelSize: root.f(34); font.weight: Font.Bold; color: root.ink
                }

                // 刻度: 这批帧覆盖过去两小时, 所以轴是 -2h … 现在
                Item {
                    id: ticks
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.top: parent.top; anchors.topMargin: root.u(62)
                    height: root.u(56)

                    Rectangle {
                        anchors.left: parent.left; anchors.right: parent.right
                        anchors.verticalCenter: parent.verticalCenter
                        height: 2; color: root.line
                    }
                    Repeater {
                        model: mapPage.hours.length
                        delegate: Rectangle {
                            required property int index
                            readonly property bool sel: index === mapPage.sel
                            x: mapPage.hours.length > 1
                               ? index * (parent.width - width) / (mapPage.hours.length - 1) : 0
                            anchors.verticalCenter: parent.verticalCenter
                            width: sel ? root.f(20) : root.f(10)
                            height: width; radius: width / 2
                            color: sel ? root.ink : root.line
                            MouseArea {
                                anchors.fill: parent; anchors.margins: -root.u(20)
                                onClicked: mapPage.sel = index
                            }
                        }
                    }
                }
                Row {
                    anchors.left: parent.left; anchors.right: parent.right
                    anchors.bottom: parent.bottom
                    // 刻度文字从真实小时里取, 不写死 —— 轴是向前的预报, 共 13 小时
                    Repeater {
                        model: 5
                        delegate: Text {
                            required property int index
                            readonly property int k: Math.round(index * (mapPage.hours.length - 1) / 4)
                            width: playRow.width / 5
                            horizontalAlignment: index === 0 ? Text.AlignLeft
                                               : index === 4 ? Text.AlignRight : Text.AlignHCenter
                            text: mapPage.hours.length
                                  ? (index === 0 ? "现在" : mapPage.hours[k].label) : ""
                            font.pixelSize: root.f(22); color: root.ink2
                        }
                    }
                }
            }
        }

        // ─────── 城市管理 ───────
        Flickable {
            visible: root.tab === "city"
            anchors.fill: parent
            contentWidth: width; contentHeight: cityCol.height + root.u(20); clip: true
            ColumnLayout {
                id: cityCol
                width: parent.width
                spacing: root.u(12)

                RowLayout {
                    Layout.fillWidth: true; Layout.topMargin: root.u(10)
                    spacing: root.u(10)
                    Rectangle {
                        Layout.fillWidth: true; Layout.preferredHeight: root.f(60)
                        radius: 10; color: root.card
                        border.color: searchInput.activeFocus ? root.ink : root.line; border.width: 1
                        TextInput {
                            id: searchInput
                            anchors.fill: parent; anchors.leftMargin: root.u(16); anchors.rightMargin: root.u(16)
                            verticalAlignment: TextInput.AlignVCenter
                            font.pixelSize: root.f(23); color: root.ink
                            selectByMouse: true; clip: true
                            onAccepted: root.search()
                        }
                        Text {
                            anchors.left: parent.left; anchors.leftMargin: root.u(16); anchors.verticalCenter: parent.verticalCenter
                            visible: searchInput.text === "" && !searchInput.activeFocus
                            text: "输入城市名，如 北京"; font.pixelSize: root.f(23); color: "#7A766C"
                        }
                    }
                    Rectangle {
                        Layout.preferredWidth: root.f(120); Layout.preferredHeight: root.f(60)
                        radius: 10; color: root.ink
                        Text { anchors.centerIn: parent; text: "搜索"; font.pixelSize: root.f(23); color: "white" }
                        MouseArea { anchors.fill: parent; onClicked: root.search() }
                    }
                }

                // 检索结果
                Column {
                    Layout.fillWidth: true
                    visible: root.results.length > 0
                    Repeater {
                        model: root.results
                        delegate: Rectangle {
                            required property var modelData
                            width: cityCol.width; height: root.u(88)
                            color: "transparent"
                            Text {
                                x: root.u(16); anchors.verticalCenter: parent.verticalCenter
                                text: modelData.name + "   " + modelData.admin + (modelData.country && modelData.country !== "中国" ? " · " + modelData.country : "")
                                font.pixelSize: root.f(24); color: root.ink
                            }
                            Text { anchors.right: parent.right; anchors.rightMargin: root.u(16); anchors.verticalCenter: parent.verticalCenter
                                   text: "添加"; font.pixelSize: root.f(22); color: root.ink2 }
                            Rectangle { anchors.bottom: parent.bottom; anchors.left: parent.left; anchors.right: parent.right; height: 1; color: root.line }
                            MouseArea { anchors.fill: parent; onClicked: root.addCity(modelData) }
                        }
                    }
                }

                Text { text: "我的城市"; font.pixelSize: root.f(24); font.weight: Font.Medium; color: root.ink; Layout.topMargin: root.u(6) }
                Column {
                    Layout.fillWidth: true
                    Repeater {
                        model: root.cfg ? root.cfg.cities : []
                        delegate: Rectangle {
                            required property var modelData
                            required property int index
                            readonly property var cd: root.cache[W.cityKey(modelData)]
                            width: cityCol.width; height: root.u(112)
                            radius: 12
                            color: index === root.cfg.current ? "#F0EEE8" : root.card
                            border.color: root.line; border.width: 1
                            Column {
                                x: root.u(20); anchors.verticalCenter: parent.verticalCenter; spacing: 4
                                Row {
                                    spacing: root.u(8)
                                    Text { text: modelData.name; font.pixelSize: root.f(26); font.weight: Font.Medium; color: root.ink }
                                    Image {
                                        visible: index === root.cfg.current
                                        anchors.verticalCenter: parent.verticalCenter
                                        width: root.f(20); height: width; sourceSize.width: width; sourceSize.height: width
                                        source: root.img("i-star-fill.svg"); fillMode: Image.PreserveAspectFit
                                    }
                                }
                                Text { text: modelData.admin || ""; font.pixelSize: root.f(19); color: root.ink2 }
                            }
                            Row {
                                anchors.right: parent.right; anchors.rightMargin: root.u(20); anchors.verticalCenter: parent.verticalCenter
                                spacing: root.u(16)
                                Text {
                                    anchors.verticalCenter: parent.verticalCenter
                                    text: cd ? (root.t(cd.lo) + "° / " + root.t(cd.hi) + "°") : "—"
                                    font.pixelSize: root.f(23); color: root.ink2
                                }
                                Image {
                                    anchors.verticalCenter: parent.verticalCenter
                                    width: root.f(38); height: width; sourceSize.width: 76; sourceSize.height: 76
                                    source: cd ? root.icon(cd.code, true) : root.img("w-cloud.svg")
                                    fillMode: Image.PreserveAspectFit; opacity: cd ? 1 : 0.3
                                }
                                Image {
                                    anchors.verticalCenter: parent.verticalCenter
                                    visible: root.cfg.cities.length > 1
                                    width: root.f(28); height: width; sourceSize.width: width; sourceSize.height: width
                                    source: root.img("i-trash.svg"); fillMode: Image.PreserveAspectFit; opacity: 0.8
                                    MouseArea { anchors.fill: parent; anchors.margins: -root.u(12); onClicked: root.removeCity(index) }
                                }
                            }
                            MouseArea {
                                anchors.fill: parent; anchors.rightMargin: root.u(70)
                                onClicked: root.selectCity(index)
                            }
                        }
                    }
                }

                Text { text: "常用城市"; font.pixelSize: root.f(24); font.weight: Font.Medium; color: root.ink; Layout.topMargin: root.u(8) }
                Flow {
                    Layout.fillWidth: true
                    spacing: root.u(10)
                    Repeater {
                        model: W.COMMON_CITIES
                        delegate: Rectangle {
                            required property var modelData
                            width: root.f(112); height: root.f(58); radius: height / 2
                            color: root.card; border.color: root.line; border.width: 1
                            Text { anchors.centerIn: parent; text: modelData.name; font.pixelSize: root.f(22); color: root.ink }
                            MouseArea { anchors.fill: parent; onClicked: root.addCity(modelData) }
                        }
                    }
                }
            }
        }

        // ─────── 设置 ───────
        Flickable {
            visible: root.tab === "setting"
            anchors.fill: parent
            contentWidth: width; contentHeight: setCol.height + root.u(20); clip: true
            ColumnLayout {
                id: setCol
                width: parent.width
                spacing: 0

                Item { Layout.preferredHeight: root.u(10); Layout.fillWidth: true }

                // 温度单位
                Item {
                    Layout.fillWidth: true; Layout.preferredHeight: root.u(104)
                    Text { x: root.u(6); anchors.verticalCenter: parent.verticalCenter; text: "温度单位"
                           font.pixelSize: root.f(25); color: root.ink }
                    Row {
                        anchors.right: parent.right; anchors.rightMargin: root.u(6); anchors.verticalCenter: parent.verticalCenter
                        spacing: root.u(8)
                        Repeater {
                            model: [{ k: "c", n: "°C" }, { k: "f", n: "°F" }]
                            delegate: Rectangle {
                                required property var modelData
                                readonly property bool sel: root.unit() === modelData.k
                                width: root.f(84); height: root.f(54); radius: height / 2
                                color: sel ? root.ink : "transparent"; border.color: sel ? root.ink : root.line; border.width: 1
                                Text { anchors.centerIn: parent; text: modelData.n; font.pixelSize: root.f(22)
                                       color: parent.sel ? "white" : root.ink2 }
                                MouseArea { anchors.fill: parent; onClicked: { root.cfg.unit = modelData.k; root.commit(); root.saveCfg(null) } }
                            }
                        }
                    }
                    Rectangle { anchors.bottom: parent.bottom; anchors.left: parent.left; anchors.right: parent.right; height: 1; color: root.line }
                }
                // 更新频率
                Item {
                    Layout.fillWidth: true; Layout.preferredHeight: root.u(104)
                    Text { x: root.u(6); anchors.verticalCenter: parent.verticalCenter; text: "更新频率"
                           font.pixelSize: root.f(25); color: root.ink }
                    Row {
                        anchors.right: parent.right; anchors.rightMargin: root.u(6); anchors.verticalCenter: parent.verticalCenter
                        spacing: root.u(8)
                        Repeater {
                            model: [{ s: 1800, n: "30 分钟" }, { s: 3600, n: "1 小时" }, { s: 10800, n: "3 小时" }]
                            delegate: Rectangle {
                                required property var modelData
                                readonly property bool sel: root.cfg && root.cfg.refresh === modelData.s
                                width: root.f(112); height: root.f(54); radius: height / 2
                                color: sel ? root.ink : "transparent"; border.color: sel ? root.ink : root.line; border.width: 1
                                Text { anchors.centerIn: parent; text: modelData.n; font.pixelSize: root.f(21)
                                       color: parent.sel ? "white" : root.ink2 }
                                MouseArea { anchors.fill: parent; onClicked: { root.cfg.refresh = modelData.s; root.commit(); root.saveCfg(null) } }
                            }
                        }
                    }
                    Rectangle { anchors.bottom: parent.bottom; anchors.left: parent.left; anchors.right: parent.right; height: 1; color: root.line }
                }
                // 只读信息行
                Repeater {
                    model: [
                        { n: "数据来源", v: "Open-Meteo" },
                        { n: "当前城市", v: root.city().name },
                        { n: "最近更新", v: root.wx ? Qt.formatDateTime(new Date(root.wx.at), "MM-dd HH:mm") : "还没有数据" }
                    ]
                    delegate: Item {
                        required property var modelData
                        Layout.fillWidth: true; Layout.preferredHeight: root.u(104)
                        Text { x: root.u(6); anchors.verticalCenter: parent.verticalCenter; text: modelData.n
                               font.pixelSize: root.f(25); color: root.ink }
                        Text { anchors.right: parent.right; anchors.rightMargin: root.u(6); anchors.verticalCenter: parent.verticalCenter
                               text: modelData.v; font.pixelSize: root.f(23); color: root.ink2 }
                        Rectangle { anchors.bottom: parent.bottom; anchors.left: parent.left; anchors.right: parent.right; height: 1; color: root.line }
                    }
                }
                Text {
                    Layout.fillWidth: true; Layout.topMargin: root.u(20)
                    text: "天气数据来自 Open-Meteo（open-meteo.com），空气质量为 US AQI 口径。\n设备联网时按上面的频率更新，离线时显示最近一次结果。"
                    font.pixelSize: root.f(20); color: root.ink2; wrapMode: Text.WordWrap
                }
            }
        }
    }

    // ═══ 底部标签栏 ═══
    Rectangle {
        id: tabbar
        anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom
        height: root.u(136)
        color: root.paper
        Rectangle { anchors.top: parent.top; anchors.left: parent.left; anchors.right: parent.right; height: 1; color: root.line }
        RowLayout {
            anchors.fill: parent
            anchors.leftMargin: root.u(20); anchors.rightMargin: root.u(20)
            spacing: 0
            Repeater {
                model: [
                    { k: "home", n: "首页", ic: "house", kit: true },
                    { k: "forecast", n: "预报", ic: "i-cloudtab", kit: false },
                    { k: "map", n: "地图", ic: "i-map", kit: false },
                    { k: "city", n: "城市", ic: "i-pin", kit: false },
                    { k: "setting", n: "设置", ic: "gear", kit: true }
                ]
                delegate: Item {
                    id: tabItem
                    required property var modelData
                    readonly property bool sel: root.tab === modelData.k
                    Layout.fillWidth: true; Layout.fillHeight: true
                    Column {
                        anchors.centerIn: parent; spacing: root.u(6)
                        Image {
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: root.f(46); height: width; sourceSize.width: width; sourceSize.height: width
                            source: (tabItem.modelData.kit ? space.kitDir + "/icons/" : root.dir) + tabItem.modelData.ic + ".svg"
                            fillMode: Image.PreserveAspectFit
                            opacity: tabItem.sel ? 1 : 0.72
                        }
                        Text {
                            anchors.horizontalCenter: parent.horizontalCenter
                            text: tabItem.modelData.n; font.pixelSize: root.f(26)
                            font.weight: tabItem.sel ? Font.Bold : Font.Normal
                            color: tabItem.sel ? root.ink : root.ink3
                        }
                        Rectangle {
                            anchors.horizontalCenter: parent.horizontalCenter
                            width: root.f(32); height: 3
                            color: tabItem.sel ? root.ink : "transparent"
                        }
                    }
                    MouseArea { anchors.fill: parent; onClicked: root.tab = tabItem.modelData.k }
                }
            }
        }
    }
}
