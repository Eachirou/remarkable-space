import QtQuick
import QtQuick.Layouts

Item {
    id: root
    property var space
    implicitHeight: 300
    property string location: "Paris"
    property var weather: ({})
    Component.onCompleted: refresh()
    function refresh() {
        space.dataGet("weather", "config", function(st, cfg) {
            if (st === 200 && cfg) root.location = cfg.location || root.location
            var x = new XMLHttpRequest()
            x.onreadystatechange = function() {
                if (x.readyState !== XMLHttpRequest.DONE || x.status !== 200) return
                try {
                    var d = JSON.parse(x.responseText), c = d.current_condition[0]
                    root.weather = { temp: c.temp_C, description: c.weatherDesc[0].value }
                } catch (e) {}
            }
            x.open("GET", "https://wttr.in/" + encodeURIComponent(root.location) + "?format=j1")
            x.send()
        })
    }
    ColumnLayout {
        anchors.fill: parent
        anchors.margins: 22
        Text { text: root.location; font.pixelSize: 28; font.weight: Font.Medium }
        Text { text: root.weather.temp ? root.weather.temp + " °C" : space.trApp("weather", "loading"); font.pixelSize: 54 }
        Text { text: root.weather.description || space.trApp("weather", "weather_hint"); font.pixelSize: 22 }
    }
}
