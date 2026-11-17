import QtQuick
import QtQuick.Layouts

Item {
    id: root
    property var space
    implicitHeight: 180
    property var tasks: ({})
    Component.onCompleted: refresh()
    function refresh() {
        space.dataGet("planner", "tasks", function(st, value) {
            root.tasks = st === 200 && value ? value : ({})
        })
    }
    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 18
        Text { text: space.trApp("planner", "today"); font.pixelSize: 25; font.weight: Font.Medium }
        Text {
            Layout.fillWidth: true
            text: {
                var list = root.tasks.tasks || []
                var open = 0
                for (var i = 0; i < list.length; ++i) if (!list[i].done) open++
                return space.trApp("planner", "open_tasks", { n: open })
            }
            font.pixelSize: 22
            wrapMode: Text.WordWrap
        }
    }
}
