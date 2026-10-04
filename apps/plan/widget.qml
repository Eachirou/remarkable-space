// 今日计划 — 「空间」首页左卡 (清单里声明 widget_slot: "plan", 外壳据此放位置)
// 只读今天的任务, 打勾就地生效; 点空白处进应用。
import QtQuick
import "plan.js" as P

Item {
    id: w
    property var space          // 外壳 API
    property var app            // 本应用的注册表条目 (id / dir / dataDir)
    anchors.fill: parent

    readonly property string dir: app ? "file://" + app.dir + "/assets/" : ""
    readonly property real fs: space ? space.fontScale : 1
    readonly property real un: space ? space.unit : 1
    function f(n) { return Math.round(n * w.fs) }
    function u(n) { return Math.round(n * w.un) }

    readonly property color ink:  "#000000"
    readonly property color ink2: "#1F1E1B"
    readonly property color line: "#CCCCC9"
    readonly property color soft: "#D8D7D2"

    property var tasks: []
    property bool ready: false
    readonly property string today: space ? P.ymd(space.localNow()) : ""
    readonly property var list: P.dayTasks(tasks, today)
    readonly property int done: P.doneCount(list)

    // 首页每次进来都会重挂小组件, 上一份实例可能已经销毁而它的请求还在路上,
    // 回调里的 w 这时是 null, 不判断就抛 "Value is null" 并中断。
    function load() {
        space.dataGet(app.id, "data", function (st, r) {
            if (!w) return
            w.tasks = (st === 200 && r && r.tasks) ? r.tasks : []
            w.ready = true
        })
    }
    function toggle(id) {
        for (var i = 0; i < w.tasks.length; ++i)
            if (w.tasks[i].id === id) { w.tasks[i].done = !w.tasks[i].done; break }
        w.tasks = w.tasks.slice()
        space.dataGet(app.id, "data", function (st, r) {
            if (!w) return
            var d = (st === 200 && r) ? r : ({})
            d.tasks = w.tasks
            space.dataPut(app.id, "data", d, null)
        })
    }
    Component.onCompleted: load()
    // 回到首页时重新取一次: 在应用里改过的东西, 退出来要能看到
    onVisibleChanged: if (visible && ready) load()

    Column {
        anchors.fill: parent
        anchors.margins: w.u(22)
        spacing: w.u(10)
        visible: w.ready

        Row {
            width: parent.width
            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: "今日计划"; font.pixelSize: w.f(28); font.weight: Font.Bold; color: w.ink
            }
            Item { width: w.u(14); height: 1 }
            Text {
                anchors.verticalCenter: parent.verticalCenter
                text: w.list.length > 0 ? (w.done + "/" + w.list.length) : ""
                font.pixelSize: w.f(24); color: w.ink2
            }
        }

        Rectangle {
            width: parent.width; height: w.u(10); radius: height / 2; color: w.soft
            visible: w.list.length > 0
            Rectangle {
                anchors.left: parent.left; anchors.top: parent.top; anchors.bottom: parent.bottom
                width: Math.max(height, parent.width * P.percent(w.done, w.list.length) / 100)
                radius: height / 2; color: "#8FB46A"
            }
        }

        Repeater {
            model: w.list.slice(0, 4)
            delegate: Item {
                required property var modelData
                width: parent.width
                height: w.u(54)
                Rectangle {
                    id: box
                    anchors.left: parent.left; anchors.verticalCenter: parent.verticalCenter
                    width: w.f(28); height: width; radius: width / 2
                    color: modelData.done ? "#8FB46A" : "transparent"
                    border.color: modelData.done ? "#8FB46A" : w.ink; border.width: 2
                    Image {
                        anchors.centerIn: parent
                        visible: modelData.done
                        width: parent.width * 0.6; height: width
                        sourceSize.width: 40; sourceSize.height: 40
                        source: w.dir + "check.svg"; fillMode: Image.PreserveAspectFit
                    }
                    MouseArea { anchors.fill: parent; anchors.margins: -w.u(12)
                                onClicked: w.toggle(modelData.id) }
                }
                Text {
                    anchors.left: box.right; anchors.leftMargin: w.u(14)
                    anchors.right: parent.right
                    anchors.verticalCenter: parent.verticalCenter
                    text: modelData.title
                    font.pixelSize: w.f(25); color: w.ink
                    elide: Text.ElideRight
                    font.strikeout: modelData.done
                }
            }
        }

        Text {
            width: parent.width
            visible: w.list.length === 0
            text: "今天还没有安排。点这里去添一件。"
            font.pixelSize: w.f(23); color: w.ink2; wrapMode: Text.WordWrap
        }
        Text {
            width: parent.width
            visible: w.list.length > 4
            text: "还有 " + (w.list.length - 4) + " 件…"
            font.pixelSize: w.f(22); color: w.ink2
        }
    }

    // 空白处点开应用 (打勾的圆圈在上层, 不会被它吃掉)
    MouseArea {
        anchors.fill: parent
        z: -1
        onClicked: space.openApp(app.id)
    }
}
