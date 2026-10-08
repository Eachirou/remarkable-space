// AI 设置 — 「空间」内置设置项 (从旧高级面板迁出; 配置走 upload-server /ai-config, 与手机端共享)
import QtQuick
import QtQuick.Layouts
import device.ui.controls

Item {
    id: root
    property var space          // 外壳注入的 API 对象 (见 space/README.md)
    anchors.fill: parent

    property string _rmhAiKind: "openai"           // "openai" | "anthropic"
    property string _rmhAiUrl: "https://api.openai.com/v1"
    property string _rmhAiKey: ""
    property string _rmhAiModel: "gpt-4o-mini"
    // Qwen3 等 hybrid thinking 模型用 enable_thinking 控制. 默认开 (跟 Qwen3 默认一致).
    property bool _rmhAiThinking: true
    property string _rmhAiTestStatus: ""
    property bool _rmhAiTesting: false
    readonly property string _rmhAiConfigPath: root.space.baseUrl + "/ai-config"

    Component.onCompleted: { aiLoadConfig(); aiRefreshQr() }

    ColumnLayout {
        anchors.fill: parent
        spacing: 28

        // 类型选择 (单选)
        RowLayout {
            Layout.fillWidth: true
            spacing: 16
            Text {
                text: space.tr("type")
                font.pixelSize: 26
                Layout.preferredWidth: 160
            }
            Repeater {
                model: [
                    { key: "openai",    label: space.tr("openai_compat") },
                    { key: "anthropic", label: "Anthropic" }
                ]
                delegate: Rectangle {
                    required property var modelData
                    Layout.preferredWidth: 240
                    Layout.preferredHeight: 56
                    radius: 6
                    readonly property bool sel: root._rmhAiKind === modelData.key
                    color: sel ? "#eeeeee" : "transparent"
                    border.color: sel ? "#1a0f04" : "#cccccc"
                    border.width: sel ? 2 : 1
                    Text {
                        anchors.centerIn: parent
                        text: parent.modelData.label
                        font.pixelSize: 24
                        font.weight: parent.sel ? Font.Medium : Font.Normal
                    }
                    MouseArea {
                        anchors.fill: parent
                        onClicked: {
                            root._rmhAiKind = parent.modelData.key
                            root.aiApplyKindDefaults(parent.modelData.key)
                        }
                    }
                }
            }
            Item { Layout.fillWidth: true }
        }

        // URL
        RowLayout {
            Layout.fillWidth: true
            spacing: 16
            Text {
                text: "API URL"
                font.pixelSize: 26
                Layout.preferredWidth: 160
            }
            Rectangle {
                Layout.fillWidth: true
                Layout.preferredHeight: 56
                radius: 6
                color: "transparent"
                border.color: _rmhAiUrlInput.activeFocus ? "#1a0f04" : "#cccccc"
                border.width: 1
                TextInput {
                    id: _rmhAiUrlInput
                    anchors.fill: parent
                    anchors.leftMargin: 16
                    anchors.rightMargin: 16
                    verticalAlignment: TextInput.AlignVCenter
                    font.pixelSize: 24
                    text: root._rmhAiUrl
                    selectByMouse: true
                    clip: true
                    onTextChanged: root._rmhAiUrl = text
                }
            }
        }

        // API Key (打码: PasswordEchoOnEdit — 编辑时可见, 失焦自动打码)
        RowLayout {
            Layout.fillWidth: true
            spacing: 16
            Text {
                text: "API Key"
                font.pixelSize: 26
                Layout.preferredWidth: 160
            }
            Rectangle {
                Layout.fillWidth: true
                Layout.preferredHeight: 56
                radius: 6
                color: "transparent"
                border.color: _rmhAiKeyInput.activeFocus ? "#1a0f04" : "#cccccc"
                border.width: 1
                TextInput {
                    id: _rmhAiKeyInput
                    anchors.fill: parent
                    anchors.leftMargin: 16
                    anchors.rightMargin: 16
                    verticalAlignment: TextInput.AlignVCenter
                    font.pixelSize: 24
                    text: root._rmhAiKey
                    echoMode: TextInput.PasswordEchoOnEdit
                    passwordCharacter: "\u2022"
                    selectByMouse: true
                    clip: true
                    onTextChanged: root._rmhAiKey = text
                }
            }
        }

        // 模型
        RowLayout {
            Layout.fillWidth: true
            spacing: 16
            Text {
                text: space.tr("model")
                font.pixelSize: 26
                Layout.preferredWidth: 160
            }
            Rectangle {
                Layout.fillWidth: true
                Layout.preferredHeight: 56
                radius: 6
                color: "transparent"
                border.color: _rmhAiModelInput.activeFocus ? "#1a0f04" : "#cccccc"
                border.width: 1
                TextInput {
                    id: _rmhAiModelInput
                    anchors.fill: parent
                    anchors.leftMargin: 16
                    anchors.rightMargin: 16
                    verticalAlignment: TextInput.AlignVCenter
                    font.pixelSize: 24
                    text: root._rmhAiModel
                    selectByMouse: true
                    clip: true
                    onTextChanged: root._rmhAiModel = text
                }
            }
        }

        // 思考模式 (仅 Qwen3 等 hybrid thinking 模型有效, 关闭可加速响应).
        RowLayout {
            Layout.fillWidth: true
            spacing: 16
            Text {
                // space.tr("thinking")
                text: space.tr("thinking")
                font.pixelSize: 26
                Layout.preferredWidth: 160
            }
            Rectangle {
                Layout.preferredWidth: 120
                Layout.preferredHeight: 56
                radius: height / 2
                color: root._rmhAiThinking ? "#1a0f04" : "#ffffff"
                border.color: "#1a0f04"
                border.width: 1
                Text {
                    anchors.centerIn: parent
                    // space.tr("on") / space.tr("off")
                    text: root._rmhAiThinking ? space.tr("on") : space.tr("off")
                    font.pixelSize: 26
                    color: root._rmhAiThinking ? "#ffffff" : "#1a0f04"
                }
                MouseArea {
                    anchors.fill: parent
                    onClicked: root._rmhAiThinking = !root._rmhAiThinking
                }
            }
            Text {
                Layout.fillWidth: true
                Layout.alignment: Qt.AlignVCenter
                // "仅 Qwen3 等支持的模型有效, 关闭可加速响应"
                text: space.tr("thinking_hint")
                font.pixelSize: 22
                color: "#666666"
                wrapMode: Text.Wrap
            }
        }

        // 操作按钮 + 状态
        RowLayout {
            Layout.fillWidth: true
            Layout.topMargin: 12
            spacing: 16

            IconButton {
                iconSource: "qrc:/ark/icons/checkmark"
                title: space.tr("save")
                onClicked: {
                    root.aiSaveConfig()
                    root._rmhAiTestStatus = space.tr("saved")
                }
            }
            IconButton {
                iconSource: "qrc:/ark/icons/restore"
                title: space.tr("refresh")
                onClicked: {
                    root.aiLoadConfig()
                    root.aiRefreshQr()
                    root._rmhAiTestStatus = space.tr("refreshed")
                }
            }
            IconButton {
                iconSource: "qrc:/ark/icons/wifi_3"
                title: root._rmhAiTesting
                    ? space.tr("testing")
                    : space.tr("test")
                enabled: !root._rmhAiTesting
                onClicked: {
                    root.aiSaveConfig()
                    root.aiTestConnection()
                }
            }
            Text {
                Layout.fillWidth: true
                Layout.alignment: Qt.AlignVCenter
                text: root._rmhAiTestStatus
                font.pixelSize: 22
                color: "#444444"
                elide: Text.ElideRight
            }
        }

        // ─── 手机扫码同步配置 (居中, 简洁) ──────────────────
        Rectangle {
            Layout.fillWidth: true
            Layout.topMargin: 8
            Layout.preferredHeight: 1
            color: "#cccccc"
        }
        ColumnLayout {
            Layout.fillWidth: true
            Layout.topMargin: 12
            Layout.alignment: Qt.AlignHCenter
            spacing: 10

            Image {
                id: _rmhAiQrImage
                Layout.preferredWidth: 220
                Layout.preferredHeight: 220
                Layout.alignment: Qt.AlignHCenter
                cache: false
                fillMode: Image.PreserveAspectFit
                visible: false
            }
            Text {
                id: _rmhAiQrUrl
                Layout.alignment: Qt.AlignHCenter
                font.pixelSize: 22
                color: "#000000"
                visible: _rmhAiQrImage.visible
            }
            Text {
                Layout.alignment: Qt.AlignHCenter
                text: space.tr("qr_hint")
                font.pixelSize: 24
                color: "#000000"
            }
            Text {
                Layout.alignment: Qt.AlignHCenter
                text: space.tr("no_wifi")
                font.pixelSize: 22
                color: "#c62828"
                visible: !_rmhAiQrImage.visible
            }
        }
        Item { Layout.fillHeight: true }
    }

    function aiLoadConfig() {
        var x = new XMLHttpRequest()
        x.open("GET", _rmhAiConfigPath)
        x.onreadystatechange = function() {
            if (x.readyState !== XMLHttpRequest.DONE) return
            try {
                var d = JSON.parse(x.responseText)
                if (typeof d.kind === "string") _rmhAiKind = d.kind
                if (typeof d.url === "string") {
                    _rmhAiUrl = d.url
                    if (typeof _rmhAiUrlInput !== "undefined") _rmhAiUrlInput.text = d.url
                }
                if (typeof d.key === "string") {
                    _rmhAiKey = d.key
                    if (typeof _rmhAiKeyInput !== "undefined") _rmhAiKeyInput.text = d.key
                }
                if (typeof d.model === "string") {
                    _rmhAiModel = d.model
                    if (typeof _rmhAiModelInput !== "undefined") _rmhAiModelInput.text = d.model
                }
                // 后端用 omitempty: nil 不出现 → 保持 QML 默认值 (true).
                if (typeof d.enable_thinking === "boolean") _rmhAiThinking = d.enable_thinking
            } catch (e) {}
        }
        x.send()
    }
    function aiRefreshQr() {
        var x = new XMLHttpRequest()
        x.onreadystatechange = function() {
            if (x.readyState !== 4) return
            if (x.status === 200) {
                try {
                    var info = JSON.parse(x.responseText)
                    if (info.available) {
                        _rmhAiQrUrl.text = info.url
                        _rmhAiQrImage.source = root.space.baseUrl + "/qr.png?focus=ai&t=" + Date.now()
                        _rmhAiQrImage.visible = true
                        return
                    }
                } catch (e) {}
            }
            _rmhAiQrImage.visible = false
        }
        x.open("GET", root.space.baseUrl + "/qr-info?focus=ai")
        x.send()
    }
    function aiSaveConfig() {
        var x = new XMLHttpRequest()
        x.open("PUT", _rmhAiConfigPath)
        x.setRequestHeader("Content-Type", "application/json")
        x.send(JSON.stringify({
            kind: _rmhAiKind,
            url: _rmhAiUrl,
            key: _rmhAiKey,
            model: _rmhAiModel,
            enable_thinking: _rmhAiThinking
        }))
    }
    function aiApplyKindDefaults(kind) {
        if (kind === "openai") {
            if (_rmhAiUrl === "" || _rmhAiUrl.indexOf("anthropic") >= 0) {
                _rmhAiUrl = "https://api.openai.com/v1"
            }
            if (_rmhAiModel === "" || _rmhAiModel.indexOf("claude") >= 0) {
                _rmhAiModel = "gpt-4o-mini"
            }
        } else if (kind === "anthropic") {
            if (_rmhAiUrl === "" || _rmhAiUrl.indexOf("openai") >= 0) {
                _rmhAiUrl = "https://api.anthropic.com/v1"
            }
            if (_rmhAiModel === "" || _rmhAiModel.indexOf("gpt") >= 0) {
                _rmhAiModel = "claude-haiku-4-5"
            }
        }
    }
    function aiTestConnection() {
        if (_rmhAiTesting) return
        _rmhAiTesting = true
        _rmhAiTestStatus = space.tr("testing")
        var x = new XMLHttpRequest()
        var url, headers, body
        if (_rmhAiKind === "anthropic") {
            url = _rmhAiUrl.replace(/\/$/, "") + "/messages"
            body = JSON.stringify({
                model: _rmhAiModel,
                max_tokens: 8,
                messages: [{role: "user", content: "ping"}]
            })
            x.open("POST", url)
            x.setRequestHeader("x-api-key", _rmhAiKey)
            x.setRequestHeader("anthropic-version", "2023-06-01")
            x.setRequestHeader("content-type", "application/json")
        } else {
            url = _rmhAiUrl.replace(/\/$/, "") + "/chat/completions"
            body = JSON.stringify({
                model: _rmhAiModel,
                max_tokens: 8,
                messages: [{role: "user", content: "ping"}]
            })
            x.open("POST", url)
            x.setRequestHeader("Authorization", "Bearer " + _rmhAiKey)
            x.setRequestHeader("content-type", "application/json")
        }
        x.onreadystatechange = function() {
            if (x.readyState !== XMLHttpRequest.DONE) return
            _rmhAiTesting = false
            if (x.status >= 200 && x.status < 300) {
                _rmhAiTestStatus = space.tr("test_ok")
            } else if (x.status === 0) {
                _rmhAiTestStatus = space.tr("test_unreachable")
            } else {
                var msg = "HTTP " + x.status
                try {
                    var err = JSON.parse(x.responseText)
                    if (err.error && err.error.message) msg += ": " + err.error.message
                } catch (e) {}
                _rmhAiTestStatus = msg
            }
        }
        x.send(body)
    }
    function aiMaskKey(k) {
        if (!k || k.length < 8) return k
        return k.substring(0, 4) + "\u2026\u2026" + k.substring(k.length - 4)
    }
    }
