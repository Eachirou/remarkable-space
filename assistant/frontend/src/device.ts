// 设备画像。
//
// 这是整个界面唯一的"重"元素: 按真机 3:4 比例画出来, 屏幕里的内容跟着安装状态变 ——
// 装了「空间」, 侧栏就多出一条并点亮; 装了 Android, 机身后面浮出第二层轮廓 (一台设备
// 两套系统); 进了 Android 模式, 屏幕整个换成应用网格。
//
// 用户正在改的就是这块屏幕, 所以让它直接显示改成了什么样, 比任何一段说明都快。

export type Shape = {
  connected: boolean
  space: boolean
  android: boolean
  inAndroid: boolean
}

const doc = (x: number, y: number) => `
  <rect class="d-doc" x="${x}" y="${y}" width="30" height="39" rx="2"/>
  <line class="d-line" x1="${x + 5}" y1="${y + 28}" x2="${x + 20}" y2="${y + 28}"/>
  <line class="d-line" x1="${x + 5}" y1="${y + 33}" x2="${x + 14}" y2="${y + 33}"/>`

const navItem = (y: number, w: number, on = false) =>
  `<rect class="d-nav${on ? ' on' : ''}" x="20" y="${y}" width="${w}" height="3.4" rx="1.7"/>`

const tile = (x: number, y: number) => `<rect class="d-tile" x="${x}" y="${y}" width="13" height="13" rx="3"/>`

export function deviceArt(s: Shape): string {
  let screen = ''

  if (!s.connected) {
    // 没连上: 屏幕空着, 虚线表示"还不知道里面是什么"
    screen = `<rect class="d-empty" x="13" y="19" width="104" height="140" rx="2"/>`
  } else if (s.inAndroid) {
    // Android 模式: 应用网格
    const tiles: string[] = []
    for (let r = 0; r < 5; r++) for (let c = 0; c < 4; c++) tiles.push(tile(20 + c * 20, 30 + r * 21))
    screen = tiles.join('')
  } else {
    // reMarkable 系统: 左边侧栏 + 右边文稿。装了「空间」就多一条并点亮。
    const nav = [navItem(32, 16), navItem(41, 12), navItem(50, 14)]
    if (s.space) nav.push(navItem(59, 15, true))
    const docs = [doc(46, 28), doc(82, 28), doc(46, 76), doc(82, 76), doc(46, 124), doc(82, 124)]
    screen = `<line class="d-rule" x1="40" y1="19" x2="40" y2="159"/>${nav.join('')}${docs.join('')}`
  }

  return `
  <svg class="device" viewBox="0 0 146 190" role="img" aria-label="设备">
    ${s.android ? '<rect class="d-ghost" x="18" y="4" width="124" height="166" rx="9"/>' : ''}
    <rect class="d-body" x="4" y="14" width="124" height="166" rx="9"/>
    <rect class="d-screen" x="13" y="19" width="104" height="140" rx="2"/>
    ${screen}
    <circle class="d-dot" cx="66" cy="169" r="2.6"/>
  </svg>`
}
