import QtQuick
import QtQuick.Layouts
import device.ui.controls
import "../../kit" as Kit

Item {
    id: root
    property var space
    anchors.fill: parent
    property var defaultUrls: ["https://www.lemonde.fr/rss/tag/actualites.xml", "https://www.tagesschau.de/xml/rss2", "https://feeds.bbci.co.uk/news/rss.xml"]
    property var urls: []
    property var articles: []
    property string message: ""
    property bool busy: false
    Component.onCompleted: loadConfig()

    function loadConfig() {
        space.dataGet("news", "config", function(st, cfg) {
            root.urls = st === 200 && cfg && cfg.urls && cfg.urls.length ? cfg.urls : root.defaultUrls
            urlsInput.text = root.urls.join("\n")
            root.refresh()
        })
    }
    function saveConfig() {
        var values = urlsInput.text.split(/\s*\n\s*/).filter(function(v) { return v.trim() !== "" })
        root.urls = values.length ? values : root.defaultUrls
        urlsInput.text = root.urls.join("\n")
        space.dataPut("news", "config", { urls: root.urls }, function(st, _) {
            root.message = st === 200 ? space.tr("saved") : space.tr("save_failed")
            if (st === 200) root.refresh()
        })
    }
    function clean(value) {
        return value.replace(/<!\[CDATA\[([\s\S]*?)\]\]>/g, "$1").replace(/<[^>]+>/g, "").replace(/&amp;/g, "&").replace(/&lt;/g, "<").replace(/&gt;/g, ">").replace(/&quot;/g, '"').replace(/&#39;/g, "'").trim()
    }
    function parseFeed(xml) {
        var result = [], blocks = xml.match(/<item\b[\s\S]*?<\/item>/gi) || xml.match(/<entry\b[\s\S]*?<\/entry>/gi) || []
        for (var i = 0; i < blocks.length && result.length < 20; ++i) {
            var block = blocks[i]
            var title = block.match(/<title[^>]*>([\s\S]*?)<\/title>/i)
            var link = block.match(/<link[^>]*>([\s\S]*?)<\/link>/i) || block.match(/<link[^>]*href=["']([^"']+)["']/i)
            var date = block.match(/<(pubDate|published|updated|dc:date)[^>]*>([\s\S]*?)<\/(pubDate|published|updated|dc:date)>/i)
            if (title && link) result.push({ title: clean(title[1]), link: clean(link[1]), date: date ? clean(date[2]) : "", feed: root.urls[0] })
        }
        return result
    }
    function refresh() {
        root.articles = []; root.message = ""; root.busy = true
        fetchFeed(0)
    }
    function fetchFeed(index) {
        if (index >= root.urls.length) { root.busy = false; if (!root.articles.length) root.message = space.tr("feed_failed"); return }
        var url = root.urls[index].trim(), x = new XMLHttpRequest()
        x.onreadystatechange = function() {
            if (x.readyState !== XMLHttpRequest.DONE) return
            if (x.status === 200) root.articles = root.articles.concat(parseFeed(x.responseText))
            fetchFeed(index + 1)
        }
        x.open("GET", url); x.send()
    }

    Flickable {
        anchors.fill: parent; contentWidth: width; contentHeight: column.height + 30; clip: true
        ColumnLayout {
            id: column; width: parent.width; spacing: 14
            RowLayout { Layout.fillWidth: true; Text { text: space.tr("title"); font.pixelSize: 32; font.weight: Font.Medium }; Item { Layout.fillWidth: true }; IconButton { iconSource: "qrc:/ark/icons/restore"; title: space.tr("refresh"); enabled: !root.busy; onClicked: root.refresh() } }
            Text { visible: root.message !== ""; text: root.message; font.pixelSize: 20; wrapMode: Text.WordWrap }
            Text { text: root.busy ? space.tr("loading") : space.tr("articles"); font.pixelSize: 25; font.weight: Font.Medium }
            Repeater {
                model: root.articles
                delegate: Rectangle {
                    Layout.fillWidth: true; height: articleColumn.height + 20; color: "#f0f0f0"; radius: 8
                    ColumnLayout { id: articleColumn; anchors.left: parent.left; anchors.right: parent.right; anchors.top: parent.top; anchors.margins: 12; spacing: 5
                        Kit.SButton { Layout.fillWidth: true; text: modelData.title; th: space.th; ui: space.fs; onClicked: Qt.openUrlExternally(modelData.link) }
                        Text { Layout.fillWidth: true; text: modelData.date || space.tr("no_date"); font.pixelSize: 17; color: "#555555" }
                        Text { Layout.fillWidth: true; text: modelData.link; font.pixelSize: 16; elide: Text.ElideRight }
                    }
                }
            }
            Text { text: space.tr("settings"); font.pixelSize: 25; font.weight: Font.Medium }
            Text { text: space.tr("urls_hint"); font.pixelSize: 18; wrapMode: Text.WordWrap }
            TextEdit { id: urlsInput; Layout.fillWidth: true; Layout.preferredHeight: 120; font.pixelSize: 19; wrapMode: TextEdit.Wrap; selectByMouse: true; color: "#202020" }
            Kit.SButton { text: space.tr("save"); th: space.th; ui: space.fs; primary: true; enabled: !root.busy; onClicked: root.saveConfig() }
        }
    }
}
