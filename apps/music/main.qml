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
    property string pairingCode: ""
    property string message: ""
    property var playback: ({})
    property bool busy: false

    Component.onCompleted: loadConfig()

    function request(method, path, body, cb) {
        var x = new XMLHttpRequest()
        var finished = false
        x.timeout = 12000
        x.onreadystatechange = function() {
            if (x.readyState !== XMLHttpRequest.DONE || finished) return
            finished = true
            var value = null
            try { value = JSON.parse(x.responseText) } catch (e) {}
            cb(x.status, value)
        }
        x.onerror = function() {
            if (finished) return
            finished = true
            cb(0, { error: space.tr("bridge_unavailable") })
        }
        x.ontimeout = x.onerror
        x.open(method, root.bridgeUrl + path)
        x.setRequestHeader("Authorization", "Bearer " + root.token)
        if (body !== null) x.setRequestHeader("Content-Type", "application/json")
        x.send(body === null ? null : JSON.stringify(body))
    }

    function loadConfig() {
        space.dataGet("music", "config", function(st, cfg) {
            if (st === 200 && cfg) {
                root.bridgeUrl = cfg.bridgeUrl || ""
                root.token = cfg.token || ""
            }
            if (root.bridgeUrl !== "") root.refresh()
        })
    }

    function validBridgeUrl() {
        return root.bridgeUrl.indexOf("http://") === 0 || root.bridgeUrl.indexOf("https://") === 0
    }

    function pair() {
        if (!validBridgeUrl()) {
            root.message = space.tr("invalid_url")
            return
        }
        var code = root.pairingCode.trim()
        if (!/^[0-9]{8}$/.test(code)) {
            root.message = space.tr("invalid_pairing_code")
            return
        }
        while (root.bridgeUrl.endsWith("/")) root.bridgeUrl = root.bridgeUrl.slice(0, -1)
        root.busy = true
        var x = new XMLHttpRequest()
        var finished = false
        x.timeout = 12000
        x.onreadystatechange = function() {
            if (x.readyState !== XMLHttpRequest.DONE || finished) return
            finished = true
            root.busy = false
            var value = null
            try { value = JSON.parse(x.responseText) } catch (e) {}
            if (x.status !== 200 || !value || !value.token) {
                if (x.status === 401) root.message = space.tr("invalid_pairing_code")
                else if (x.status === 410) root.message = space.tr("pairing_expired")
                else if (x.status === 429) root.message = space.tr("pairing_attempts_exceeded")
                else root.message = value && value.error ? value.error : space.tr("pair_failed")
                return
            }
            root.token = value.token
            space.dataPut("music", "config", { bridgeUrl: root.bridgeUrl, token: root.token }, function(st, _) {
                if (st !== 200) {
                    root.message = space.tr("save_failed")
                    return
                }
                root.pairingCode = ""
                root.message = space.tr("paired")
                root.refresh()
            })
        }
        x.onerror = function() {
            if (finished) return
            finished = true
            root.busy = false
            root.message = space.tr("bridge_unavailable")
        }
        x.ontimeout = x.onerror
        x.open("POST", root.bridgeUrl + "/api/pair")
        x.setRequestHeader("Content-Type", "application/json")
        x.send(JSON.stringify({ code: code }))
    }

    function refresh() {
        root.busy = true
        root.request("GET", "/api/status", null, function(st, value) {
            root.busy = false
            if (st !== 200 || !value) {
                if (value && value.error) {
                    root.message = value.error === "invalid bridge token" ? space.tr("invalid_token") : value.error
                } else {
                    root.message = space.tr("bridge_unavailable")
                }
                return
            }
            root.playback = value
            root.message = ""
        })
    }

    function control(action) {
        if (root.bridgeUrl === "" || root.token === "") {
            root.message = space.tr("configure_first")
            return
        }
        root.busy = true
        root.request("POST", "/api/control", { action: action }, function(st, value) {
            root.busy = false
            if (st !== 200) {
                root.message = value && value.error ? value.error : space.tr("control_failed")
                return
            }
            root.refresh()
        })
    }

    ColumnLayout {
        anchors.left: parent.left
        anchors.right: parent.right
        anchors.top: parent.top
        spacing: 18

        RowLayout {
            Layout.fillWidth: true
            Text { text: space.tr("title"); font.pixelSize: 32; font.weight: Font.Medium }
            Item { Layout.fillWidth: true }
            IconButton {
                iconSource: "qrc:/ark/icons/restore"
                title: space.tr("refresh")
                enabled: !root.busy && root.bridgeUrl !== ""
                onClicked: root.refresh()
            }
        }

        Text {
            Layout.fillWidth: true
            visible: root.message !== ""
            text: root.message
            font.pixelSize: 22
            wrapMode: Text.WordWrap
        }

        Rectangle {
            Layout.fillWidth: true
            Layout.preferredHeight: 150
            color: "#f0f0f0"
            radius: 8
            ColumnLayout {
                anchors.fill: parent
                anchors.margins: 20
                Text {
                    Layout.fillWidth: true
                    text: root.playback.track || space.tr("nothing_playing")
                    font.pixelSize: 28
                    elide: Text.ElideRight
                }
                Text {
                    Layout.fillWidth: true
                    text: root.playback.artist ? root.playback.artist + " — " + (root.playback.album || "") : space.tr("connect_hint")
                    font.pixelSize: 22
                    elide: Text.ElideRight
                }
                Text {
                    Layout.fillWidth: true
                    text: root.playback.device ? space.tr("device") + ": " + root.playback.device : ""
                    font.pixelSize: 20
                }
            }
        }

        RowLayout {
            Layout.fillWidth: true
            spacing: 12
            Kit.SButton { text: space.tr("previous"); th: space.th; ui: space.fs; enabled: !root.busy; onClicked: root.control("previous") }
            Kit.SButton { text: root.playback.playing ? space.tr("pause") : space.tr("play"); primary: true; th: space.th; ui: space.fs; enabled: !root.busy; onClicked: root.control(root.playback.playing ? "pause" : "play") }
            Kit.SButton { text: space.tr("next"); th: space.th; ui: space.fs; enabled: !root.busy; onClicked: root.control("next") }
        }

        Text { text: space.tr("settings"); font.pixelSize: 26; font.weight: Font.Medium }
        Text { text: space.tr("bridge_url"); font.pixelSize: 20 }
        TextInput {
            Layout.fillWidth: true
            text: root.bridgeUrl
            font.pixelSize: 22
            selectByMouse: true
            onTextChanged: root.bridgeUrl = text
        }
        Text {
            Layout.fillWidth: true
            text: space.tr("bridge_url_hint")
            font.pixelSize: 17
            wrapMode: Text.WordWrap
        }
        Text { text: space.tr("pairing_code"); font.pixelSize: 20 }
        TextInput {
            Layout.fillWidth: true
            text: root.pairingCode
            font.pixelSize: 22
            inputMethodHints: Qt.ImhDigitsOnly
            maximumLength: 8
            selectByMouse: true
            onTextChanged: root.pairingCode = text
        }
        Text {
            Layout.fillWidth: true
            text: space.tr("pairing_hint")
            font.pixelSize: 17
            wrapMode: Text.WordWrap
        }
        Kit.SButton {
            text: space.tr("pair")
            th: space.th
            ui: space.fs
            primary: true
            enabled: !root.busy
            onClicked: root.pair()
        }
    }
}
