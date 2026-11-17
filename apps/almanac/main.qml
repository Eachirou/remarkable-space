import QtQuick
import QtQuick.Layouts
import device.ui.controls

Item {
    id: root
    property var space
    anchors.fill: parent
    property date today: new Date()
    property int year: today.getFullYear()
    property string season: ""
    property string nextEvent: ""
    Component.onCompleted: update()
    function update() {
        var d = root.today, month = d.getMonth() + 1, day = d.getDate()
        if ((month === 12 && day >= 21) || month < 3 || (month === 3 && day < 20)) root.season = space.tr("winter")
        else if (month < 6 || (month === 6 && day < 21)) root.season = space.tr("spring")
        else if (month < 9 || (month === 9 && day < 22)) root.season = space.tr("summer")
        else root.season = space.tr("autumn")
        var events = [{m:3,d:20,n:"spring"},{m:6,d:21,n:"summer"},{m:9,d:22,n:"autumn"},{m:12,d:21,n:"winter"}], next = null
        for (var i = 0; i < events.length; ++i) if (events[i].m > month || (events[i].m === month && events[i].d > day)) { next = events[i]; break }
        if (!next) next = events[0]
        root.nextEvent = space.tr("approximate") + " " + next.d + "/" + next.m + " — " + space.tr(next.n)
    }
    ColumnLayout {
        anchors.fill: parent; spacing: 18
        RowLayout { Layout.fillWidth: true; Text { text: space.tr("title"); font.pixelSize: 32; font.weight: Font.Medium }; Item { Layout.fillWidth: true }; IconButton { iconSource: "qrc:/ark/icons/restore"; title: space.tr("today"); onClicked: { root.today = new Date(); root.year = root.today.getFullYear(); root.update() } } }
        Text { text: Qt.formatDate(root.today, Qt.DefaultLocaleLongDate); font.pixelSize: 25 }
        Rectangle { Layout.fillWidth: true; Layout.preferredHeight: 150; color: "#f0f0f0"; radius: 8; ColumnLayout { anchors.fill: parent; anchors.margins: 20; Text { text: space.tr("season"); font.pixelSize: 20 }; Text { text: root.season; font.pixelSize: 42; font.weight: Font.Medium }; Text { text: space.tr("northern"); font.pixelSize: 18 } } }
        Text { text: space.tr("next_event"); font.pixelSize: 25; font.weight: Font.Medium }
        Text { text: root.nextEvent; font.pixelSize: 22; wrapMode: Text.WordWrap }
        Text { text: space.tr("about"); font.pixelSize: 20; wrapMode: Text.WordWrap }
        Item { Layout.fillHeight: true }
    }
}
