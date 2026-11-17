import QtQuick
import QtQuick.Layouts
import device.ui.controls
import "../../kit" as Kit

Item {
    id: root
    property var space
    anchors.fill: parent
    property string message: ""
    ListModel { id: tasks }

    Component.onCompleted: load()

    function load() {
        space.dataGet("planner", "tasks", function(st, value) {
            tasks.clear()
            if (st !== 200 || !value || !value.tasks) return
            for (var i = 0; i < value.tasks.length; ++i) tasks.append(value.tasks[i])
        })
    }
    function save() {
        var values = []
        for (var i = 0; i < tasks.count; ++i) values.push(tasks.get(i))
        space.dataPut("planner", "tasks", { tasks: values }, function(st, _) {
            root.message = st === 200 ? space.tr("saved") : space.tr("save_failed")
        })
    }
    function addTask() {
        var value = newTask.text.trim()
        if (!value) return
        tasks.append({ text: value, done: false })
        newTask.text = ""
        save()
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 16
        RowLayout {
            Layout.fillWidth: true
            Text { text: space.tr("title"); font.pixelSize: 32; font.weight: Font.Medium }
            Item { Layout.fillWidth: true }
            IconButton { iconSource: "qrc:/ark/icons/restore"; title: space.tr("refresh"); onClicked: load() }
        }
        Text { visible: root.message !== ""; text: root.message; font.pixelSize: 20 }
        RowLayout {
            Layout.fillWidth: true
            TextInput {
                id: newTask
                Layout.fillWidth: true
                font.pixelSize: 24
                placeholderText: space.tr("new_task")
                onAccepted: root.addTask()
            }
            Kit.SButton { text: space.tr("add"); th: space.th; ui: space.fs; primary: true; onClicked: root.addTask() }
        }
        ListView {
            Layout.fillWidth: true
            Layout.fillHeight: true
            clip: true
            model: tasks
            delegate: Rectangle {
                width: ListView.view.width
                height: 64
                color: "transparent"
                border.color: "#cccccc"
                RowLayout {
                    anchors.fill: parent
                    anchors.margins: 10
                    Text {
                        text: model.done ? "✓" : "○"
                        font.pixelSize: 28
                        MouseArea {
                            anchors.fill: parent
                            anchors.margins: -8
                            onClicked: { tasks.setProperty(index, "done", !model.done); root.save() }
                        }
                    }
                    Text { Layout.fillWidth: true; text: model.text; font.pixelSize: 23; font.strikeout: model.done }
                    IconButton {
                        iconSource: "qrc:/ark/icons/trashcan"
                        title: space.tr("delete")
                        onClicked: { tasks.remove(index); root.save() }
                    }
                }
            }
        }
    }
}
