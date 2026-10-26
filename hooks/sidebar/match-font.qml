// 侧栏「空间」入口的字号跟系统菜单项一致。
//
// 入口是底座注入库建的, 字号写死 30; 系统菜单项 (指南、回收站…) 是 28 (rmppm 实测文字宽 56/2 字,
// 入口 60/2 字), 其他机型也未必一样。底座冻着不改, 这里由空间 (能在设备上自更新) 去抄旁边
// 「指南」那一项的字号。本组件零尺寸、不显示, 不占侧栏位置。
import QtQuick

Item {
    width: 0
    height: 0
    visible: false

    function findText(n, d, cls) {
        if (!n || d > 6) return null
        if (String(n).indexOf(cls) === 0 && n.font !== undefined) return n
        var ch = n.children
        if (ch) for (var i = 0; i < ch.length; i++) { var r = findText(ch[i], d + 1, cls); if (r) return r }
        return null
    }
    function apply() {
        var col = parent
        if (!col) return
        var entry = null, ref = null
        for (var i = 0; i < col.children.length; i++) {
            var c = col.children[i]
            if (c.objectName === "advancedItem") entry = c
            else if (!ref && c.objectName === "guides") ref = c
        }
        if (!entry) return
        var mine = findText(entry, 0, "QQuickText")
        var sys = ref ? findText(ref, 0, "TextLabel") : null
        if (!mine) return
        var px = sys && sys.font.pixelSize > 0 ? sys.font.pixelSize : 28
        if (mine.font.pixelSize !== px) mine.font.pixelSize = px
    }
    Component.onCompleted: apply()
    onParentChanged: apply()
}
