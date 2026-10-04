// 计划 — 数据模型与纯函数 (.pragma library: 不持有任何界面状态)
.pragma library

// ─── 分类 ──────────────────────────────────────────────────────────
// 墨水屏上大块浅色会发灰, 所以底色只用在小小的标签上, 文字一律纯黑。
// tint 用在小标签底色, accent 用在进度条与圆点这类实心小面积:
// 这是彩色墨水屏, 实心色块能真的显出颜色, 淡淡一层反而被抖动洗成灰。
// 彩色墨水屏 (Kaleido) 的色彩靠滤色阵列合成, 饱和度本来就掉一大截, 面积越小掉得越多。
// 所以这里的颜色比概念图更饱和一档: 屏上看到的才接近概念图的观感。
// 淡色只留给标签底 (面积小但成片), 实心元素一律用深而饱和的色。
var CATS = [
    { k: "work",   n: "工作", tint: "#A9C6F0", accent: "#1F5FC0" },
    { k: "life",   n: "个人", tint: "#F2B6A3", accent: "#C2472A" },
    { k: "hobby",  n: "兴趣", tint: "#CBB2F0", accent: "#6B31C0" }
]
var ACCENT = "#2F7A3A"
function catName(k) {
    for (var i = 0; i < CATS.length; ++i) if (CATS[i].k === k) return CATS[i].n
    return ""
}

// 收集箱条目的种类
var KINDS = [
    { k: "idea",  n: "想法", icon: "lightbulb",      tint: "#EFD98A", accent: "#8A6A00" },
    { k: "todo",  n: "待办", icon: "list-checks",    tint: "#F2B6A3", accent: "#C2472A" },
    { k: "file",  n: "资料", icon: "file-text",      tint: "#A9C6F0", accent: "#1F5FC0" },
    { k: "spark", n: "灵感", icon: "pencil-simple",  tint: "#CBB2F0", accent: "#6B31C0" }
]
function kindOf(k) {
    for (var i = 0; i < KINDS.length; ++i) if (KINDS[i].k === k) return KINDS[i]
    return KINDS[0]
}

// ─── 日期 ──────────────────────────────────────────────────────────
function pad(n) { return n < 10 ? "0" + n : String(n) }
function ymd(d) { return d.getFullYear() + "-" + pad(d.getMonth() + 1) + "-" + pad(d.getDate()) }
function mdText(s) {          // "2026-09-24" → "9月24日"
    if (!s) return ""
    var p = s.split("-")
    return parseInt(p[1], 10) + "月" + parseInt(p[2], 10) + "日"
}
var WEEK = ["星期日", "星期一", "星期二", "星期三", "星期四", "星期五", "星期六"]
function weekText(d) { return WEEK[d.getDay()] }

// ─── 默认数据 ──────────────────────────────────────────────────────
// 第一次打开给几条示例, 让人一眼看懂这四个页面各是什么; 删光也不会再生成。
function seed(today) {
    return {
        tasks: [
            { id: "t1", date: today, time: "08:30", title: "晨间散步",   note: "运动 · 30 分钟",   cat: "life",  done: true  },
            { id: "t2", date: today, time: "10:00", title: "数据治理方案", note: "进行中 · 1/3",     cat: "work",  done: false },
            { id: "t3", date: today, time: "14:00", title: "产品原型设计", note: "设计天气 App 首页", cat: "work",  done: false },
            { id: "t4", date: today, time: "19:30", title: "阅读 / 写作",  note: "整理笔记 · 1 小时", cat: "life",  done: false },
            { id: "t5", date: today, time: "21:30", title: "音乐创作",    note: "完成一首新歌的编曲", cat: "hobby", done: false }
        ],
        inbox: [
            { id: "i1", kind: "idea", text: "天气 App 可以增加空气质量模块，支持多城市切换。", at: "", done: false },
            { id: "i2", kind: "todo", text: "整理数据治理方案文档，发给领导评审。",           at: "", done: false }
        ]
    }
}

// ─── 统计 ──────────────────────────────────────────────────────────
function dayTasks(tasks, date) {
    var out = []
    for (var i = 0; i < tasks.length; ++i) if (tasks[i].date === date) out.push(tasks[i])
    out.sort(function (a, b) { return (a.time || "") < (b.time || "") ? -1 : 1 })
    return out
}
function doneCount(list) {
    var n = 0
    for (var i = 0; i < list.length; ++i) if (list[i].done) n++
    return n
}
function percent(done, total) { return total > 0 ? Math.round(done * 100 / total) : 0 }

// 哪几天有事 (日历上的小圆点): 返回 {"2026-09-24": ["work","life"], …}
function taskDots(tasks) {
    var m = {}
    for (var i = 0; i < tasks.length; ++i) {
        var t = tasks[i]
        if (!m[t.date]) m[t.date] = []
        if (m[t.date].indexOf(t.cat) < 0 && m[t.date].length < 3) m[t.date].push(t.cat)
    }
    return m
}
function filterBy(list, field, value) {
    if (!value) return list
    var out = []
    for (var i = 0; i < list.length; ++i) if (list[i][field] === value) out.push(list[i])
    return out
}
function countBy(list, field, value) { return filterBy(list, field, value).length }

// 日历的三种范围: 月 / 周 / 日
function rangeDays(anchorStr, mode, weekStart) {
    var d = new Date(anchorStr.replace(/-/g, "/"))
    if (mode === "day") return [anchorStr]
    if (mode === "week") {
        var off = (d.getDay() - (weekStart === 0 ? 0 : 1) + 7) % 7
        var s0 = addDays(d, -off), out = []
        for (var i = 0; i < 7; ++i) out.push(ymd(addDays(s0, i)))
        return out
    }
    var y = d.getFullYear(), m = d.getMonth(), n = daysInMonth(y, m), o = []
    for (var k = 1; k <= n; ++k) o.push(y + "-" + pad(m + 1) + "-" + pad(k))
    return o
}
// 周视图用: 把一段日期摊平成 [日期头, 任务, 任务, 日期头, ...], 一个 Repeater 就能画
function rangeSections(tasks, days) {
    var out = []
    for (var i = 0; i < days.length; ++i) {
        var l = dayTasks(tasks, days[i])
        if (!l.length) continue
        out.push({ sec: true, date: days[i] })
        for (var j = 0; j < l.length; ++j) out.push({ sec: false, t: l[j] })
    }
    return out
}

function newId(prefix) { return prefix + Date.now().toString(36) + Math.floor(Math.random() * 1000) }
