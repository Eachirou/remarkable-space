import QtQuick
import QtQuick.Layouts
import device.ui.controls
import "../../kit" as Kit

Item {
    id: root
    property var space
    anchors.fill: parent
    property string bridgeUrl: ""
    property string token: ""
    property string message: ""
    property var stations: []
    Component.onCompleted: loadConfig()
    function request(method, path, body, cb) {
        var x = new XMLHttpRequest()
        x.onreadystatechange = function() {
            if (x.readyState !== XMLHttpRequest.DONE) return
            var value = null
            try { value = JSON.parse(x.responseText) } catch (e) {}
            cb(x.status, value)
        }
        x.open(method, root.bridgeUrl + path)
        x.setRequestHeader("Authorization", "Bearer " + root.token)
        if (body !== null) x.setRequestHeader("Content-Type", "application/json")
        x.send(body === null ? null : JSON.stringify(body))
    }
    function loadConfig() {
        space.dataGet("radio", "config", function(st, cfg) {
            if (st === 200 && cfg) { root.bridgeUrl = cfg.bridgeUrl || ""; root.token = cfg.token || "" }
            if (root.bridgeUrl) refresh()
        })
    }
    function saveConfig() {
        while (root.bridgeUrl.endsWith("/")) root.bridgeUrl = root.bridgeUrl.slice(0, -1)
        space.dataPut("radio", "config", { bridgeUrl: root.bridgeUrl, token: root.token }, function(st, _) {
            root.message = st === 200 ? space.tr("saved") : space.tr("save_failed")
            if (st === 200) root.refresh()
        })
    }
    function refresh() {
        root.request("GET", "/api/radio/stations", null, function(st, value) {
            if (st !== 200 || !value) { root.message = space.tr("bridge_unavailable"); return }
            root.stations = value
        })
    }
    function play(station) {
        root.request("POST", "/api/radio/play", station, function(st, value) {
            root.message = st === 200 ? space.tr("playing") : (value && value.error ? value.error : space.tr("play_failed"))
        })
    }
    ColumnLayout {
        anchors.fill: parent
        spacing: 16
        RowLayout {
            Layout.fillWidth: true
            Text { text: space.tr("title"); font.pixelSize: 32; font.weight: Font.Medium }
            Item { Layout.fillWidth: true }
            IconButton { iconSource: "qrc:/ark/icons/restore"; title: space.tr("refresh"); onClicked: root.refresh() }
        }
        Text { visible: root.message !== ""; text: root.message; font.pixelSize: 20; wrapMode: Text.WordWrap }
        Text { text: space.tr("stations"); font.pixelSize: 25; font.weight: Font.Medium }
        Repeater {
            model: root.stations
            delegate: Kit.SButton { text: modelData.name; th: space.th; ui: space.fs; onClicked: root.play(modelData) }
        }
        Text { text: space.tr("settings"); font.pixelSize: 25; font.weight: Font.Medium }
        TextInput { Layout.fillWidth: true; text: root.bridgeUrl; font.pixelSize: 21; onTextChanged: root.bridgeUrl = text }
        TextInput { Layout.fillWidth: true; text: root.token; font.pixelSize: 21; echoMode: TextInput.Password; onTextChanged: root.token = text }
        Kit.SButton { text: space.tr("save"); th: space.th; ui: space.fs; primary: true; onClicked: root.saveConfig() }
    }
}
