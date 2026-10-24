// STile — 应用磁贴 (图标 + 名字), 首页"我的应用"用小号, "应用"页用中号
import QtQuick
import QtQuick.Layouts

Rectangle {
    id: tile
    property url iconSource
    property string label: ""
    property bool dimmed: false
    property var th: null          // 外壳传 space.th (主题); 不传就是默认样子
    property int iconSize: 48
    property int labelSize: 22
    signal clicked()

    color: mouse.pressed ? (th ? th.press : "#eeeeee") : (th ? th.card : "white")
    border.color: th ? th.line : "#B4AFA6"
    border.width: th ? th.border : 1
    radius: th ? Math.max(0, th.radius - 2) : 10
    opacity: dimmed ? 0.4 : 1

    ColumnLayout {
        anchors.centerIn: parent
        spacing: 10
        Image {
            Layout.alignment: Qt.AlignHCenter
            Layout.preferredWidth: tile.iconSize
            Layout.preferredHeight: tile.iconSize
            source: tile.iconSource
            fillMode: Image.PreserveAspectFit
            sourceSize.width: tile.iconSize
            sourceSize.height: tile.iconSize
        }
        Text {
            Layout.alignment: Qt.AlignHCenter
            Layout.maximumWidth: tile.width - 16
            text: tile.label
            font.pixelSize: tile.labelSize
            elide: Text.ElideRight
            horizontalAlignment: Text.AlignHCenter
        }
    }
    MouseArea {
        id: mouse
        anchors.fill: parent
        onClicked: tile.clicked()
    }
}
