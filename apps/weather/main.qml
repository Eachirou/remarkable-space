import QtQuick
import QtQuick.Layouts
import device.ui.controls
import "../../kit" as Kit

Item {
    id: root
    property var space
    anchors.fill: parent
    property string location: "Paris"
    property string endpoint: "https://wttr.in"
    property string message: ""
    property var weather: ({})
    property bool busy: false
    Component.onCompleted: loadConfig()

    function loadConfig() {
        space.dataGet("weather", "config", function(st, cfg) {
            if (st === 200 && cfg) {
                root.location = cfg.location || root.location
                root.endpoint = cfg.endpoint || root.endpoint
            }
            root.refresh()
        })
    }
    function saveConfig() {
        space.dataPut("weather", "config", { location: root.location, endpoint: root.endpoint }, function(st, _) {
            root.message = st === 200 ? space.tr("saved") : space.tr("save_failed")
            if (st === 200) root.refresh()
        })
    }
    function refresh() {
        if (!root.location || !root.endpoint) return
        root.busy = true
        var x = new XMLHttpRequest()
        x.onreadystatechange = function() {
            if (x.readyState !== XMLHttpRequest.DONE) return
            root.busy = false
            if (x.status !== 200) { root.message = space.tr("weather_failed"); return }
            try {
                var data = JSON.parse(x.responseText)
                var current = data.current_condition && data.current_condition[0]
                if (!current) throw "missing current weather"
                root.weather = { temp: current.temp_C, feels: current.FeelsLikeC,
                    description: current.weatherDesc && current.weatherDesc[0] ? current.weatherDesc[0].value : "",
                    updated: new Date().toLocaleString() }
                root.message = ""
            } catch (e) { root.message = space.tr("weather_invalid") }
        }
        x.open("GET", root.endpoint.replace(/\/+$/, "") + "/" + encodeURIComponent(root.location) + "?format=j1")
        x.send()
    }

    ColumnLayout {
        anchors.fill: parent
        spacing: 18
        RowLayout {
            Layout.fillWidth: true
            Text { text: space.tr("title"); font.pixelSize: 32; font.weight: Font.Medium }
            Item { Layout.fillWidth: true }
            IconButton { iconSource: "qrc:/ark/icons/restore"; title: space.tr("refresh"); enabled: !root.busy; onClicked: root.refresh() }
        }
        Text { visible: root.message !== ""; text: root.message; font.pixelSize: 21; wrapMode: Text.WordWrap }
        Rectangle {
            Layout.fillWidth: true
            Layout.preferredHeight: 230
            color: "#f0f0f0"
            radius: 8
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 22
                Text { text: root.location; font.pixelSize: 28; font.weight: Font.Medium }
                Text { text: root.weather.temp ? root.weather.temp + " °C" : space.tr("no_data"); font.pixelSize: 58 }
                Text { text: root.weather.description || ""; font.pixelSize: 24 }
                Text { text: root.weather.feels ? space.tr("feels") + " " + root.weather.feels + " °C" : ""; font.pixelSize: 20 }
            }
        }
        Text { text: space.tr("settings"); font.pixelSize: 26; font.weight: Font.Medium }
        Text { text: space.tr("location"); font.pixelSize: 20 }
        TextInput { Layout.fillWidth: true; text: root.location; font.pixelSize: 23; onTextChanged: root.location = text }
        Text { text: space.tr("endpoint"); font.pixelSize: 20 }
        TextInput { Layout.fillWidth: true; text: root.endpoint; font.pixelSize: 20; onTextChanged: root.endpoint = text }
        Kit.SButton { text: space.tr("save"); th: space.th; ui: space.fs; primary: true; enabled: !root.busy; onClicked: root.saveConfig() }
    }
}
