// 天气 — 「空间」首页整宽大卡 (hero 小组件)
// 版式参考概念图首页: 日期 / 城市 / 大温度 / 天气 / 高低温 / 一句话 + 右侧插画与天气图标。
import QtQuick

import "weather.js" as W

Item {
    id: w
    property var space
    property var app            // 本应用的注册表条目 (id / appDir / iconUrl)
    anchors.fill: parent

    readonly property string dir: app ? "file://" + app.dir + "/assets/" : ""
    // 同 main.qml: 带上外壳的 rev 令牌, 否则同名换素材要重启 xochitl 才生效
    function img(path) {
        var u = w.dir + path
        return (w.space && w.space.bust) ? w.space.bust(u) : u
    }
    readonly property real fs: space ? space.fontScale : 1
    readonly property real un: space ? space.unit : 1
    function f(n) { return Math.round(n * w.fs) }
    function u(n) { return Math.round(n * w.un) }

    property var cfg: null
    property var wx: null
    property string err: ""
    property date now: space ? space.localNow() : new Date()

    Timer { interval: 30000; running: w.visible; repeat: true; onTriggered: w.now = space.localNow() }

    function city() { return (cfg && cfg.cities && cfg.cities.length) ? cfg.cities[Math.min(cfg.current, cfg.cities.length - 1)] : W.DEFAULT_CITY }
    function t(c) { return W.toUnit(c, cfg ? cfg.unit : "c") }

    function load() {
        space.dataGet(app.id, "config", function(st, r) {
            cfg = (st === 200 && r && r.cities && r.cities.length) ? r : W.DEFAULT_CONFIG
            space.dataGet(app.id, "cache", function(st2, r2) {
                var c = (st2 === 200 && r2) ? r2[W.cityKey(city())] : null
                if (c && c.temp !== undefined) wx = c
                if (!wx || Date.now() - wx.at > cfg.refresh * 1000) refresh()
            })
        })
    }
    function refresh() {
        W.fetchCity(space, city(), function(d, e) {
            if (!d) { err = e; return }
            err = ""; wx = d
            space.dataGet(app.id, "cache", function(st, r) {
                var all = (st === 200 && r) ? r : ({})
                all[W.cityKey(city())] = d
                space.dataPut(app.id, "cache", all, null)
            })
        })
    }

    // 首页那一句: 句子在 assets/phrases.json, 按天气分组, 同一天内不变。
    // 用 XHR 读而不是 import —— 相对 import 不带 ?t= 令牌, 改了要重启 xochitl;
    // XHR 每次真读盘, 改完重开应用就生效, 你可以自己去编辑那个文件。
    property var phrases: null
    function loadPhrases() {
        space.request("GET", w.img("phrases.json"), null, function (st, r) {
            if (st === 200 && r) w.phrases = r
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
    function todayLine() {
        var k = w.wx ? w.adviceKind(w.wx) : ""
        if (!w.phrases || !k || !w.phrases[k] || !w.phrases[k].length)
            return w.wx ? w.wx.advice : ""      // 兜底: 老版整句
        var l = w.phrases[k]
        var d = w.now
        var seed = d.getFullYear() * 400 + d.getMonth() * 32 + d.getDate()
        return l[seed % l.length]
    }
    Component.onCompleted: { load(); loadPhrases() }

    // ── 插画: 整幅铺在卡片下缘, 不裁上端 ──────────────────────────
    // 换成 2:1 原稿之后顶部本来就是留白, 所以直接用整图、按 2:1 贴底画,
    // 上面那片白正好垫文字。原来用的是 *-band.jpg (下部 45% 的横幅) ——
    // 那是为竖构图老素材做的: 竖图塞进扁带子只能取到天空一片白, 才单独出了横幅。
    // 现在不需要了, 整图比横幅多出山脊和天际线, 层次更完整。
    Item {
        id: band
        z: -1
        anchors.left: parent.left; anchors.right: parent.right; anchors.bottom: parent.bottom
        height: Math.min(parent.height, Math.round(parent.width / 2))
        clip: true
        Image {
            anchors.fill: parent
            fillMode: Image.PreserveAspectCrop
            sourceSize.width: 1100
            property var chain: w.wx ? W.sceneChain(w.wx, w.now) : []
            property int step: 0
            source: chain.length ? w.img(chain[0]) : ""
            visible: status === Image.Ready
            onChainChanged: { step = 0; source = chain.length ? w.img(chain[0]) : "" }
            onStatusChanged: {
                if (status !== Image.Error) return
                if (step + 1 >= chain.length) return
                step += 1
                source = w.img(chain[step])
            }
        }
    }

    // 横幅是方角、卡片是圆角, 下面两角会露出来 (见上一版实拍)。
    // 设备上没有 QtQuick.Effects 做遮罩, QtQuick.Shapes 实测也没画出来,
    // 改用 Canvas: 只把"方角减去那道弧"的一小块填白, 四边不留白, 图与描边贴合。
    Component {
        id: cornerCut
        Canvas {
            property bool atRight: false
            width: 14; height: 14
            onPaint: {
                var ctx = getContext("2d")
                ctx.reset()
                ctx.fillStyle = "white"
                ctx.beginPath()
                if (atRight) {
                    ctx.moveTo(width, 0)
                    ctx.lineTo(width, height)
                    ctx.lineTo(0, height)
                    ctx.arc(0, 0, width, Math.PI / 2, 0, true)
                } else {
                    ctx.moveTo(0, 0)
                    ctx.lineTo(0, height)
                    ctx.lineTo(width, height)
                    ctx.arc(width, 0, width, Math.PI / 2, Math.PI, false)
                }
                ctx.closePath()
                ctx.fill()
            }
        }
    }
    Loader {
        sourceComponent: cornerCut
        anchors.left: parent.left; anchors.bottom: parent.bottom
    }
    Loader {
        sourceComponent: cornerCut
        anchors.right: parent.right; anchors.bottom: parent.bottom
        onLoaded: { item.atRight = true; item.requestPaint() }
    }
    // 卡片自己的细描边被横幅盖住了, 补回来
    Rectangle {
        anchors.fill: parent
        color: "transparent"; radius: 12
        border.color: "#B4AFA6"; border.width: 1
    }

    // ── 两栏三行, 行与行横向对齐 ──────────────────────────────────
    // 四项辅助文字 (日期 / 一句话 / 城市 / 天气) 统一 f(26)、常规字重、纯黑。
    // 原来靠 Medium / Bold 分主次, 但反射屏上字重差异会被读成深浅差异,
    // 看着就像有的字是灰的 —— 层次交给字号 (92 / 52 的大字) 就够了。
    // 左右各自用 Column 排的话, 两边字号不同 (时间 92 / 温度 52+图标) 行高就不同,
    // 第二行往后越错越多。改成先定三行的框, 左右内容各自塞进同一个框里垂直居中,
    // 于是 日期↔太原、时间↔温度、一句话↔天气 一定是齐的。
    Item {
        id: rows
        anchors.left: parent.left; anchors.right: parent.right
        anchors.top: parent.top; anchors.topMargin: w.u(10)
        height: r1.height + r2.height + r3.height

        readonly property real lpad: w.u(32)                 // 左栏左边距
        readonly property real dx: Math.round(width * 0.52)  // 中缝位置
        readonly property real rpad: dx + w.u(34)            // 右栏起点

        Rectangle {   // 中缝细竖线
            x: rows.dx
            y: w.u(6); height: rows.height - w.u(12)
            width: 1; color: "#CCCCC9"
        }

        // 行 1: 日期 ↔ 城市
        Item {
            id: r1
            width: parent.width; height: w.f(46)
            Text {
                x: rows.lpad; width: rows.dx - rows.lpad
                anchors.verticalCenter: parent.verticalCenter
                horizontalAlignment: Text.AlignHCenter
                text: w.now.toLocaleDateString(Qt.locale("zh_CN"), "M月d日 dddd")
                font.pixelSize: w.f(26); color: "#000000"
            }
            Row {
                x: rows.rpad
                anchors.verticalCenter: parent.verticalCenter
                spacing: w.u(8)
                Image {
                    anchors.verticalCenter: parent.verticalCenter
                    width: w.f(26); height: width
                    sourceSize.width: width; sourceSize.height: width
                    source: w.img("i-pin-fill.svg"); fillMode: Image.PreserveAspectFit
                }
                Text {
                    anchors.verticalCenter: parent.verticalCenter
                    text: w.city().name
                    font.pixelSize: w.f(26); color: "#000000"
                }
            }
        }

        // 行 2: 时间 ↔ 天气图标 + 温度
        Item {
            id: r2
            anchors.top: r1.bottom
            width: parent.width; height: w.f(100)
            Text {
                x: rows.lpad; width: rows.dx - rows.lpad
                anchors.verticalCenter: parent.verticalCenter
                horizontalAlignment: Text.AlignHCenter
                text: Qt.formatTime(w.now, "HH:mm")
                font.pixelSize: w.f(92); font.weight: Font.Light; color: "#000000"
            }
            Row {
                x: rows.rpad
                anchors.verticalCenter: parent.verticalCenter
                spacing: w.u(10)
                Image {
                    anchors.verticalCenter: parent.verticalCenter
                    width: w.u(76); height: width
                    sourceSize.width: 160; sourceSize.height: 160
                    source: w.wx ? w.img(W.codeIcon(w.wx.code, w.wx.day) + ".svg")
                                 : w.img("w-sun.svg")
                    fillMode: Image.PreserveAspectFit
                }
                Text {
                    anchors.verticalCenter: parent.verticalCenter
                    text: (w.wx ? String(w.t(w.wx.temp)) : "--")
                          + W.unitSign(w.cfg ? w.cfg.unit : "c")
                    font.pixelSize: w.f(52); font.weight: Font.Light; color: "#000000"
                }
            }
        }

        // 行 3: 一句话 ↔ 天气文字 + 高低温
        Item {
            id: r3
            anchors.top: r2.bottom
            width: parent.width; height: w.f(44)
            Text {
                x: rows.lpad; width: rows.dx - rows.lpad
                anchors.verticalCenter: parent.verticalCenter
                horizontalAlignment: Text.AlignHCenter
                text: w.wx ? w.todayLine() : (w.err !== "" ? w.err + "，点这里重试" : "")
                font.pixelSize: w.f(26); color: "#000000"; elide: Text.ElideRight
            }
            Row {
                x: rows.rpad
                anchors.verticalCenter: parent.verticalCenter
                spacing: w.u(16)
                Text {
                    anchors.baseline: hilo.baseline
                    text: w.wx ? w.wx.text : (w.err !== "" ? "暂无天气数据" : "获取中…")
                    font.pixelSize: w.f(26); color: "#000000"
                }
                Text {
                    id: hilo
                    visible: !!w.wx
                    text: w.wx ? ("↑ " + w.t(w.wx.hi) + "°  ↓ " + w.t(w.wx.lo) + "°") : ""
                    font.pixelSize: w.f(26); color: "#000000"
                }
            }
        }
    }

    MouseArea {
        anchors.fill: parent
        onClicked: { if (!w.wx && w.cfg) w.refresh(); else space.openApp(app.id) }
    }
}
