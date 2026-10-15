import './style.css'
import { deviceArt } from './device'
import {
  Connect, Disconnect, Probe, ChooseBundle, DownloadBundle, PlanSpace, RunSpace, UninstallSpace,
  PlanAndroid, RunAndroid, UninstallAndroid, BootAndroid, ReturnToStock, ResetAndroidData, ChooseAPKs, InstallAPKs, Cancel, EmergencyRepair, ExportDiagnostics, OpenLogDir, LogDir, DetectProxy, Confirm, LoadPassword, ForgetPassword, DiagnoseAndroid, Version, RepairPanel, Release,
} from '../wailsjs/go/main/App'
import { EventsOn } from '../wailsjs/runtime/runtime'

// 与 Go 端结构一一对应 (wailsjs/go/models.ts 里也有生成的类型, 这里只声明用到的字段)
type Info = {
  arch: string; fwVersion: string; imgVersion?: string; resolution: string; model: string; modelKey: string
  activeSlot: string; errcntA: number; errcntB: number; secboot: string; hasLSE: boolean
  rootFreeMB: number; homeFreeMB: number; haveXovi: boolean; rmkitInstalled: boolean; rmkitFW: string
  spaceInstalled: boolean; spaceActive: boolean; spaceVersion: string; androidVersion: string
  xochitlActive: boolean; inAndroidMode: boolean; androidInstalled: boolean; initIsWrapper: boolean
}
type BundleInfo = { path: string; component: string; version: string; created: string; notes: string; files: number; bytes: number }
type FileEntry = { device: string; source: string; mode: number; size: number }
type SpacePlan = { arch: string; fwVersion: string; needXovi: boolean; deployRime: boolean; qmlInject: boolean; librarian: boolean; files: FileEntry[]; totalBytes: number; warnings: string[] }
type AndroidPlan = { fwVersion: string; slot: string; hasSystemPkg: boolean; replaceSystem: boolean; systemPresent: boolean; files: string[]; totalBytes: number; warnings: string[]; blockers: string[] }

type Action = 'space' | 'android'

const state = {
  info: null as Info | null,
  // 两种安装包各记各的。以前共用一个, 切到 Android 时会拿装空间用过的包来说
  // "已选的是 space 的包, 不匹配" —— 可用户在 Android 这边根本没选过东西。
  bundles: { space: null as BundleInfo | null, android: null as BundleInfo | null },
  busy: false,
  busyOn: null as Action | null,
  open: { space: false, android: false },
  settings: false,
  replaceSystem: false,
  removeData: false,
  proxy: '',
  source: 'cn' as Source,   // 安装包从哪下: 国内镜像 / GitHub (另一个当后备)
  version: '',
  remember: true,
  password: '',
}

// 安装包两处都有: GitHub Release (仓库公开) 和国内镜像 (GitHub 在国内常下不动)。用户在设置里
// 选先从哪下, 下不动自动换另一处。每版助手写死配套安装包的标签和 sha256 (构建时注入, 见
// Makefile), 下完对不上就不装 —— 国内镜像走明文 http 也不怕被换包。
// Android 包几个月才变一次, 不跟着每版重传, 单独记它放在哪个发布标签下 (androidTag)。
// 自己的服务器 (boangs.com) 留作最后的后备: 旧版助手只认那里。
type Source = 'cn' | 'github'
type Release = { tag: string; spaceSha: string; cnBase: string; androidTag: string; androidSha: string; androidVer: string }
let release: Release = { tag: '', spaceSha: '', cnBase: '', androidTag: '', androidSha: '', androidVer: '' }
const GITHUB = 'https://github.com/boangs/remarkable-space/releases/'
const OWN_SERVER = 'https://boangs.com/space/'
const BUNDLE_NAME: Record<Action, string> = { space: 'space-bundle.zip', android: 'android-rmppm-bundle.zip' }
function bundleURLs(a: Action): string[] {
  const name = BUNDLE_NAME[a]
  const tag = a === 'android' ? release.androidTag : release.tag
  const gh = GITHUB + (tag ? 'download/' + tag + '/' : 'latest/download/') + name
  const cn = release.cnBase ? release.cnBase + (tag || 'latest') + '/' + name : ''
  const order = state.source === 'github' ? [gh, cn] : [cn, gh]
  if (a === 'android') order.push(OWN_SERVER + name)
  return order.filter((u) => u !== '')
}
const bundleSHA = (a: Action) => (a === 'space' ? release.spaceSha : release.androidSha)
const COMPONENT: Record<Action, string> = { space: 'space', android: 'android-rmppm' }
const APPROX: Record<Action, string> = { space: '约 135 MB', android: '约 600 MB' }
const LABEL: Record<Action, string> = { space: '空间', android: 'Android' }

const $ = (sel: string) => document.querySelector(sel) as HTMLElement
const mb = (n: number) => (n / 1e6).toFixed(1) + ' MB'
const num = (n: number) => n.toLocaleString('en-US')
const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' }[c] as string))
// 机器串 (路径、版本哈希、固件戳) 走等宽; 人话永远不走。
const mono = (s: string) => `<span class="mono">${esc(s)}</span>`

// Android 只在 Paper Pro Move 上验证过, 别的机型连这一行都不该出现。
const androidOK = (i: Info | null) => !!i && i.modelKey === 'rmppm'
const CODENAME: Record<string, string> = { rmppm: 'Chiappa', rmpp: 'Ferrari', rm2: 'Zero Sugar' }

const ICON = {
  gear: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7"><circle cx="12" cy="12" r="3.2"/><path d="M19.4 14a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V20a2 2 0 1 1-4 0v-.1a1.6 1.6 0 0 0-1-1.5 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H4a2 2 0 1 1 0-4h.1a1.6 1.6 0 0 0 1.5-1 1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3H10a1.6 1.6 0 0 0 1-1.5V4a2 2 0 1 1 4 0v.1a1.6 1.6 0 0 0 1 1.5 1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8V10a1.6 1.6 0 0 0 1.5 1H20a2 2 0 1 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1Z"/></svg>`,
  info: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.9"><circle cx="12" cy="12" r="9"/><path d="M12 11v5" stroke-linecap="round"/><circle cx="12" cy="7.8" r=".9" fill="currentColor" stroke="none"/></svg>`,
  chev: `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 6 6 6-6 6"/></svg>`,
}

// 说明不往界面上堆: 做成一个 ⓘ, 要看的人悬停自然看得到。
const tip = (text: string) => `<span class="tip" title="${esc(text)}">${ICON.info}</span>`

function render() {
  const i = state.info
  $('#app').innerHTML = `
    <header class="masthead">
      <h1>空间助手</h1>
      ${i ? `<button id="btn-settings" class="icon" title="设置">${ICON.gear}</button>` : ''}
    </header>
    ${i ? renderDevice(i) : renderConnect()}
    ${i ? renderItems(i) : ''}
    ${renderLog()}
    ${state.settings ? renderSettings() : ''}
  `
  bind()
}

// —— 没连设备 ——————————————————————————————————————

function renderConnect() {
  return `
    <section class="stage">
      ${deviceArt({ connected: false, space: false, android: false, inAndroid: false })}
      <div class="stagebody">
        <h2>用 USB 线把设备连上</h2>
        <div class="field">
          <label for="password">SSH 密码</label>
          <input id="password" type="password" value="${esc(state.password)}" placeholder="在设备上查" autocomplete="off">
          <button id="btn-connect" class="go">连接</button>
        </div>
        <p class="note">密码在设备的 设置 → 帮助 → 版权与许可 最底部。${tip('用 USB 线连上后设备默认地址就是 10.11.99.1。Windows 首次连接可能要先装 reMarkable 的 USB 网卡驱动。Android 模式下的密码和 reMarkable 系统的一样。')}</p>
        <label class="opt"><input type="checkbox" id="chk-remember" ${state.remember ? 'checked' : ''}> 记住密码${tip('存进系统钥匙串（Mac 钥匙串 / Windows 凭据管理器），不写进任何文件。')}</label>
        <details class="fallback">
          <summary>连不上</summary>
          <div class="field">
            <label for="host">地址</label>
            <input id="host" value="10.11.99.1">
            <button id="btn-forget" class="flat" ${state.password ? '' : 'disabled'}>忘记已存的密码</button>
          </div>
        </details>
        <details class="fallback">
          <summary>设备起不来</summary>
          <p class="note">卡在开机画面、反复重启时用。填好上面的 SSH 密码，点「紧急修复」，保持 USB 连着并让设备开机或重启；助手会一直尝试连接，一连上就停用中文、空间等全部增强，让设备以原厂状态启动。之后可以用助手重新安装。</p>
          <div class="field">
            <button id="btn-rescue" class="peril" ${state.busy ? 'disabled' : ''}>紧急修复</button>
          </div>
        </details>
      </div>
    </section>
  `
}

// —— 设备 ——————————————————————————————————————

function renderDevice(i: Info) {
  const specs: [string, string][] = [
    ['固件', mono(i.imgVersion ? `${i.imgVersion} (${i.fwVersion})` : (i.fwVersion || '未知'))],
    ['架构', `${mono(i.arch)}${CODENAME[i.modelKey] ? `<span class="dim">（${CODENAME[i.modelKey]}）</span>` : ''}`],
    ['启动槽', i.activeSlot ? `${mono(i.activeSlot)}<span class="dim">　错误计数 ${mono(`a=${i.errcntA} b=${i.errcntB}`)}</span>` : '<span class="dim">该机型无 A/B 槽</span>'],
    ['剩余空间', `系统分区 <span class="qty">${num(i.rootFreeMB)}</span> MB<span class="dim">　/home <span class="qty">${num(i.homeFreeMB)}</span> MB</span>`],
  ]
  return `
    <section class="stage">
      ${deviceArt({ connected: true, space: i.spaceInstalled, android: i.androidInstalled, inAndroid: i.inAndroidMode })}
      <div class="stagebody">
        <h2>${esc(i.model)}</h2>
        <p class="live"><span class="dot"></span>已连接，${i.inAndroidMode ? '当前在 Android' : 'reMarkable 系统' + (i.xochitlActive ? '运行中' : '未运行')}${tip('USB 线直连，不经过服务器，不读取你的笔记。')}</p>
        <dl class="spec">${specs.map(([k, v]) => `<dt>${k}</dt><dd>${v}</dd>`).join('')}</dl>
        <div class="field">
          <button id="btn-probe" class="flat" ${state.busy ? 'disabled' : ''}>重新检测</button>
          <button id="btn-disconnect" class="flat" ${state.busy ? 'disabled' : ''}>断开</button>
          <button id="btn-export-diag" class="flat" ${state.busy ? 'disabled' : ''} title="设备切槽、开不了机、空间突然没了时用：只读收集版本、启动配置和最近几次开机的系统日志，存成一个文件发给开发者。不收集笔记和账号信息。">导出诊断</button>
        </div>
      </div>
    </section>
  `
}

// —— 可安装的东西: 账本式的行, 不是卡片 ——————————————————

function renderItems(i: Info) {
  return `<div class="ledger">${renderItem(i, 'space')}${androidOK(i) ? renderItem(i, 'android') : ''}</div>`
}

function renderItem(i: Info, a: Action) {
  const isSpace = a === 'space'
  const b = state.bundles[a]
  const ready = !!b && b.component === COMPONENT[a]
  const installed = isSpace ? i.spaceInstalled : i.androidInstalled
  const busy = state.busy ? 'disabled' : ''
  // 设备上记的版本 vs 这一版助手配套的版本: 一样就是「已是最新」, 不再一律喊「更新」。
  // 设备上没记版本 (老版助手装的 Android) 就判断不了, 只给「重新安装」, 不说有更新
  const cur = isSpace ? (i.spaceVersion || '') : (i.androidVersion || '')
  const latest = isSpace ? release.tag : release.androidVer
  const upToDate = installed && cur !== '' && latest !== '' && cur === latest
  const unknown = installed && (cur === '' || latest === '')
  const outdated = installed && !upToDate && !unknown
  // 中文动词接英文词要留空格, 不然挤成"更新Android"
  const name = isSpace ? '空间' : ' Android'
  const title = (outdated ? '更新' : installed ? '重新安装' : '安装') + name

  let stateCell: string
  if (upToDate) stateCell = `<span class="on">已安装</span> ${mono(cur)}　已是最新`
  else if (outdated) stateCell = `<span class="on">已安装</span> ${mono(cur)}　可更新到 ${mono(latest)}`
  else if (installed && cur) stateCell = `<span class="on">已安装</span> ${mono(cur)}`
  else if (installed) stateCell = `<span class="on">已安装</span>`
  else stateCell = `<span class="dim">未安装，下载 ${APPROX[a]}</span>`
  // 主按钮: 要装 / 要更新才是显眼的黑底; 已是最新 (或判断不了) 时「重新安装」只是个普通按钮, 用来修复
  const runClass = (installed && !outdated) ? 'flat' : 'go'
  const runText = ready || (installed && !outdated) ? title : '下载并' + (outdated ? '更新' : '安装')

  return `
    <section class="item ${state.busyOn === a ? 'working' : ''}">
      <div class="itemrow">
        <div class="what">
          <h3>${LABEL[a]}</h3>
          <p>${isSpace ? '中文界面、输入法、应用商店' : '双系统启动，开机时选进哪一个'}</p>
        </div>
        <div class="stat">${stateCell}${ready ? `<span class="pending">安装包 ${mono(b!.version)}　${mb(b!.bytes)}</span>` : ''}</div>
        <div class="acts">
          <button class="${runClass} act-run" data-a="${a}" ${busy}>${runText}</button>
          ${installed
            ? `<button class="peril ${isSpace ? 'act-uninstall' : 'act-uninstall-android'}" data-a="${a}" ${busy}>卸载</button>`
            : `<button class="flat act-choose" data-a="${a}" ${busy}>从本地安装</button>`}
        </div>
      </div>
      ${!isSpace && installed ? `<label class="opt"><input type="checkbox" class="chk-replace" ${state.replaceSystem ? 'checked' : ''} ${busy}> 覆盖已有系统${tip('重新解包 Android 系统到 /home，应用数据目录不受影响。')}</label>` : ''}
      <div class="progress ${state.busyOn === a ? '' : 'hidden'}" id="dl-${a}"><div></div></div>
      <button class="disclose act-more" data-a="${a}" aria-expanded="${state.open[a]}">${ICON.chev}详情</button>
      ${state.open[a] ? renderDetail(i, a) : ''}
    </section>
  `
}

function renderDetail(i: Info, a: Action) {
  const busy = state.busy ? 'disabled' : ''
  const ready = !!state.bundles[a] && state.bundles[a]!.component === COMPONENT[a]
  const safety = a === 'space'
    ? '写入前会按你的固件重新生成注入表，全部命中才动系统配置；启动后观察 10 秒，任一步不对自动退回出厂启动。'
    : '出厂内核链接不动，Android 系统与数据都在 /home，重启不切槽。'
  return `
    <div class="detail">
      <div class="grp">
        <h4>安装${tip(safety)}</h4>
        <div class="field">
          <button class="flat act-choose" data-a="${a}" ${busy}>选择本地安装包</button>
          <button class="flat act-plan" data-a="${a}" ${busy || !ready ? 'disabled' : ''}>预览将写入的内容</button>
        </div>
      </div>
      ${a === 'space' ? `
        <div class="grp">
          <h4>维护</h4>
          <div class="field">
            <button id="btn-repair" class="flat" ${busy || !i.rmkitInstalled || i.inAndroidMode ? 'disabled' : ''} title="侧栏入口或中文突然消失时用：清除防砖熔断并重启一次 xochitl">恢复侧栏入口</button>
          </div>
        </div>` : `
        <div class="grp">
          <h4>切换系统</h4>
          <div class="field">
            <button id="btn-boot-android" class="flat" ${busy || !i.androidInstalled || i.inAndroidMode ? 'disabled' : ''}>重启进 Android</button>
            <button id="btn-stock" class="flat" ${busy || !i.inAndroidMode ? 'disabled' : ''}>回 reMarkable 系统</button>
          </div>
        </div>
        <div class="grp">
          <h4>维护</h4>
          <div class="field">
            <button id="btn-apks" class="flat" ${busy || !i.inAndroidMode ? 'disabled' : ''} title="需要设备在 Android 模式">安装 APK</button>
            <button id="btn-diag" class="flat" ${busy}>启动诊断</button>
            <button id="btn-reset-data" class="peril" ${busy || !i.androidInstalled || i.inAndroidMode ? 'disabled' : ''} title="清空 Android 的应用与设置，下次进 Android 重新首次开机">重置数据</button>
          </div>
          <label class="opt"><input type="checkbox" id="chk-removedata" ${state.removeData ? 'checked' : ''} ${busy}> 连数据一起删${tip('卸载时连同 /home 里的 Android 系统与应用数据一起删除，不勾选则只移除启动组件。')}</label>
        </div>`}
      <div class="plan" id="plan-${a}"></div>
    </div>
  `
}

// —— 日志 ——————————————————————————————————————

function renderLog() {
  return `
    <section class="journal">
      <div class="jhead">
        <h4>日志</h4>
        ${state.busy ? '<button id="btn-cancel" class="peril">取消</button>' : ''}
        <button id="btn-logdir" class="flat">打开日志目录</button>
      </div>
      <pre id="log"></pre>
    </section>
  `
}

// —— 设置 ——————————————————————————————————————

function renderSettings() {
  return `
    <div class="scrim" id="scrim">
      <div class="panel" role="dialog" aria-label="设置">
        <h2>设置</h2>
        <div class="field stack">
          <label>下载来源${tip('先从这里下安装包，下不动会自动换另一个。两处是同一个包，下完都会校验。')}</label>
          <div class="seg" role="radiogroup">
            <label class="opt"><input type="radio" name="src" value="cn" ${state.source === 'cn' ? 'checked' : ''}> 国内下载</label>
            <label class="opt"><input type="radio" name="src" value="github" ${state.source === 'github' ? 'checked' : ''}> GitHub 下载</label>
          </div>
        </div>
        <div class="field stack">
          <label for="proxy">下载代理${tip('一般不用填。两处都下不动时才需要，格式如 http://127.0.0.1:7890')}</label>
          <input id="proxy" value="${esc(state.proxy)}" placeholder="一般不用填">
        </div>
        <label class="opt"><input type="checkbox" id="chk-remember2" ${state.remember ? 'checked' : ''}> 记住 SSH 密码${tip('存进系统钥匙串（Mac 钥匙串 / Windows 凭据管理器），不写进任何文件。')}</label>
        <div class="field"><button id="btn-forget2" class="flat">忘记已存的密码</button></div>
        <div class="field right">
          <span class="ver">${esc(state.version)}</span>
          <button id="btn-close-settings" class="go">完成</button>
        </div>
      </div>
    </div>
  `
}

// —— 计划预览 ————————————————————————————————————

function renderSpacePlan(p: SpacePlan) {
  const flags = [
    p.needXovi ? '设备没有 xovi，会先自动部署' : 'xovi 已在',
    p.deployRime ? '整句输入（librime，拼音加五笔86）' : '纯 Go 输入引擎（安装包无 librime）',
    p.qmlInject ? '运行时 QML 注入' : 'qmd 注入',
    p.librarian ? '文件热导入 librarian' : '不装 librarian（rm2）',
  ]
  return `
    <p class="sum">写入 <span class="qty">${(p.files ?? []).length}</span> 个文件，共 ${mb(p.totalBytes)}</p>
    <ul class="flags">${flags.map((f) => `<li>${f}</li>`).join('')}</ul>
    ${(p.warnings ?? []).map((w) => `<p class="warn">${esc(w)}</p>`).join('')}
    <details><summary>文件清单</summary><pre class="files">${(p.files ?? []).map((f) => `${esc(f.device)}  ←  ${esc(f.source)}  (${(f.size / 1024).toFixed(0)} KB)`).join('\n')}</pre></details>
  `
}

function renderAndroidPlan(p: AndroidPlan) {
  return `
    ${(p.blockers ?? []).map((b) => `<p class="stop">${esc(b)}</p>`).join('')}
    ${(p.warnings ?? []).map((w) => `<p class="warn">${esc(w)}</p>`).join('')}
    <p class="sum">写入 <span class="qty">${(p.files ?? []).length}</span> 个文件，共 ${mb(p.totalBytes)}，本槽 ${mono(p.slot)}</p>
    <p class="note">${p.hasSystemPkg ? (p.systemPresent && !p.replaceSystem ? '设备已有 Android 系统，本次不覆盖。' : '包含 Android 系统包，解包到 /home。') : '安装包不含系统包，沿用设备上已有的。'}${tip('/boot 里新增 android 内核（出厂内核链接不动）、/lib/modules 新增一个模块目录、/usr/bin 四个宿主程序、/sbin/init 换成带回落的包装脚本（原链接备份为 /sbin/init.systemd-orig）')}</p>
    <details><summary>文件清单</summary><pre class="files">${(p.files ?? []).map((f) => esc(f)).join('\n')}</pre></details>
  `
}

// —— 杂务 ——————————————————————————————————————

function appendLog(line: string) {
  const el = document.querySelector('#log') as HTMLPreElement | null
  if (!el) return
  const t = new Date().toTimeString().slice(0, 8)
  el.textContent += `${t}  ` + line + '\n'
  el.scrollTop = el.scrollHeight
}

// 整页重绘会把日志 <pre> 一起换掉, 所以每次 render 前后搬一次文本。
function keepLog(fn: () => void) {
  const el = document.querySelector('#log') as HTMLPreElement | null
  const log = el ? el.textContent : ''
  fn()
  const el2 = document.querySelector('#log') as HTMLPreElement | null
  if (el2) { el2.textContent = log; el2.scrollTop = 1e9 }
}

function setBusy(b: boolean, on: Action | null = null) {
  state.busy = b
  state.busyOn = b ? on : null
  keepLog(render)
}

async function guarded(on: Action, fn: () => Promise<void>) {
  if (state.busy) return
  setBusy(true, on)
  try {
    await fn()
  } catch (e) {
    appendLog('✗ ' + String(e))
  } finally {
    try { state.info = (await Probe()) as Info } catch { /* 设备可能已重启 */ }
    setBusy(false)
    // 唯一一处非用户触发的动效: 装完了, 画像像真机那样闪一下重绘。
    const art = document.querySelector('.device')
    if (art) { art.classList.remove('refresh'); void (art as SVGElement).getBoundingClientRect(); art.classList.add('refresh') }
  }
}

// 下载加安装一条龙: 没有包就先下, 有包直接装。
async function runAction(a: Action) {
  const i = state.info as Info
  const have = () => !!state.bundles[a] && state.bundles[a]!.component === COMPONENT[a]
  if (!have()) {
    await guarded(a, async () => {
      try {
        state.bundles[a] = (await DownloadBundle(bundleURLs(a), bundleSHA(a), state.proxy)) as BundleInfo
      } catch (e) {
        // 下不动的时候才提代理。探测到系统代理就告诉他可以去「设置」里填上再试。
        if (!state.proxy) {
          const sys = await DetectProxy().catch(() => '')
          if (sys) appendLog('检测到系统代理 ' + sys + '，若直连下不动，可在设置里填上再试')
        }
        throw e
      }
    })
    if (!have()) return   // 下载失败, 别接着装
  }
  if (a === 'android') {
    const p = (await PlanAndroid(state.replaceSystem).catch((e) => { appendLog('✗ ' + String(e)); return null })) as AndroidPlan | null
    if (!p) return
    if ((p.blockers ?? []).length) { state.open.android = true; keepLog(render); $('#plan-android').innerHTML = renderAndroidPlan(p); return }
  }
  const installed = a === 'space' ? i.spaceInstalled : i.androidInstalled
  const what = a === 'space' ? '「空间」及其注入底座' : '单槽 Android'
  if (!(await Confirm('确认', `确定要在这台设备上${installed ? '更新' : '安装'} ${what} 吗？过程中请不要拔线。`))) return
  guarded(a, async () => { if (a === 'space') await RunSpace(); else await RunAndroid(state.replaceSystem) })
}

function bind() {
  // 连接
  $('#host')?.addEventListener('change', async (e) => {
    const host = (e.target as HTMLInputElement).value.trim()
    const saved = await LoadPassword(host)
    if (saved) { state.password = saved; ($('#password') as HTMLInputElement).value = saved; appendLog('已从钥匙串读到 ' + host + ' 的密码') }
  })
  $('#chk-remember')?.addEventListener('change', (e) => { state.remember = (e.target as HTMLInputElement).checked })
  $('#chk-remember2')?.addEventListener('change', (e) => { state.remember = (e.target as HTMLInputElement).checked })
  $('#btn-forget')?.addEventListener('click', async () => { await ForgetPassword(($('#host') as HTMLInputElement)?.value || '10.11.99.1'); state.password = ''; keepLog(render) })
  $('#btn-forget2')?.addEventListener('click', async () => { await ForgetPassword('10.11.99.1'); state.password = ''; appendLog('已忘记保存的密码') })
  $('#password')?.addEventListener('keydown', (e) => { if ((e as KeyboardEvent).key === 'Enter') $('#btn-connect')?.click() })
  $('#btn-connect')?.addEventListener('click', async () => {
    const host = ($('#host') as HTMLInputElement)?.value || '10.11.99.1'
    const pw = ($('#password') as HTMLInputElement).value
    state.password = pw
    if (!pw) { appendLog('请先填 SSH 密码'); return }
    try {
      state.info = (await Connect(host, pw, state.remember)) as Info
      keepLog(render)
    } catch (e) { appendLog('✗ ' + String(e)) }
  })
  $('#btn-rescue')?.addEventListener('click', async () => {
    const host = ($('#host') as HTMLInputElement)?.value || '10.11.99.1'
    const pw = ($('#password') as HTMLInputElement).value
    state.password = pw
    if (!pw) appendLog('没填 SSH 密码: 设备开机早期多半只能用密码登录，建议先填上')
    if (!(await Confirm('紧急修复', '会停用中文、空间等全部增强，让设备以原厂状态启动（笔记不受影响）。之后可以用助手重新安装。继续？'))) return
    setBusy(true)
    try { await EmergencyRepair(host, pw) } catch (e) { appendLog('✗ ' + String(e)) }
    setBusy(false)
  })
  $('#btn-export-diag')?.addEventListener('click', async () => {
    setBusy(true)
    try { await ExportDiagnostics() } catch (e) { appendLog('✗ ' + String(e)) }
    setBusy(false)
  })
  $('#btn-disconnect')?.addEventListener('click', async () => { await Disconnect(); state.info = null; keepLog(render) })
  $('#btn-probe')?.addEventListener('click', async () => { try { state.info = (await Probe()) as Info; keepLog(render) } catch (e) { appendLog('✗ ' + String(e)) } })

  // 条目
  document.querySelectorAll<HTMLButtonElement>('.act-more').forEach((t) => t.addEventListener('click', () => {
    const a = t.dataset.a as Action
    state.open[a] = !state.open[a]
    keepLog(render)
  }))
  document.querySelectorAll<HTMLButtonElement>('.act-run').forEach((t) => t.addEventListener('click', () => runAction(t.dataset.a as Action)))
  document.querySelectorAll<HTMLButtonElement>('.act-choose').forEach((t) => t.addEventListener('click', async () => {
    const a = t.dataset.a as Action
    try {
      const b = (await ChooseBundle()) as BundleInfo | null
      if (!b) return
      if (b.component !== COMPONENT[a]) { appendLog(`选的是「${b.component}」的安装包，不是${LABEL[a]}的`); return }
      state.bundles[a] = b
      keepLog(render)
    } catch (e) { appendLog('✗ ' + String(e)) }
  }))
  document.querySelectorAll<HTMLButtonElement>('.act-plan').forEach((t) => t.addEventListener('click', async () => {
    const a = t.dataset.a as Action
    const box = $(`#plan-${a}`)
    try {
      box.innerHTML = a === 'space' ? renderSpacePlan((await PlanSpace()) as SpacePlan) : renderAndroidPlan((await PlanAndroid(state.replaceSystem)) as AndroidPlan)
    } catch (e) { box.innerHTML = `<p class="stop">${esc(String(e))}</p>` }
  }))
  document.querySelectorAll<HTMLButtonElement>('.act-uninstall').forEach((t) => t.addEventListener('click', async () => {
    if (await Confirm('确认', '确定卸载空间及其注入底座？设备会恢复出厂启动配置。')) guarded('space', () => UninstallSpace())
  }))
  document.querySelectorAll<HTMLButtonElement>('.act-uninstall-android').forEach((t) => t.addEventListener('click', async () => {
    if (await Confirm('确认', state.removeData ? '确定卸载 Android 并删除 /home 里的系统与数据？' : '确定卸载 Android（保留 /home 数据）？')) guarded('android', () => UninstallAndroid(state.removeData))
  }))
  document.querySelectorAll<HTMLInputElement>('.chk-replace').forEach((t) => t.addEventListener('change', (e) => { state.replaceSystem = (e.target as HTMLInputElement).checked }))
  $('#chk-removedata')?.addEventListener('change', (e) => { state.removeData = (e.target as HTMLInputElement).checked })

  // 详情里的那些
  $('#btn-repair')?.addEventListener('click', () => guarded('space', () => RepairPanel()))
  $('#btn-boot-android')?.addEventListener('click', async () => { if (await Confirm('确认', '设备将重启进入 Android，约 1 到 3 分钟。回来时可在 Android 桌面点“原厂系统”。继续？')) guarded('android', () => BootAndroid()) })
  $('#btn-stock')?.addEventListener('click', () => guarded('android', () => ReturnToStock()))
  $('#btn-diag')?.addEventListener('click', async () => {
    try { const t = await DiagnoseAndroid(); appendLog('—— Android 启动诊断 ——\n' + t + '—— 诊断结束，已存进日志目录，请把该文件发给开发者 ——') } catch (e) { appendLog('✗ ' + String(e)) }
  })
  $('#btn-reset-data')?.addEventListener('click', async () => { if (await Confirm('确认', '清空 Android 的全部应用与设置？下次进 Android 会重新首次开机（约 5 分钟）。')) guarded('android', () => ResetAndroidData()) })
  $('#btn-apks')?.addEventListener('click', async () => {
    let paths: string[] = []
    try { paths = (await ChooseAPKs()) || [] } catch (e) { appendLog('✗ ' + String(e)); return }
    if (!paths.length) return
    guarded('android', () => InstallAPKs(paths))
  })

  // 设置
  $('#btn-settings')?.addEventListener('click', () => { state.settings = true; keepLog(render) })
  $('#btn-close-settings')?.addEventListener('click', () => { state.settings = false; keepLog(render) })
  $('#scrim')?.addEventListener('click', (e) => { if (e.target === $('#scrim')) { state.settings = false; keepLog(render) } })
  document.querySelectorAll('input[name="src"]').forEach((el) => el.addEventListener('change', (e) => {
    state.source = (e.target as HTMLInputElement).value === 'github' ? 'github' : 'cn'
    try { localStorage.setItem('download.source', state.source) } catch { /* 忽略 */ }
  }))
  $('#proxy')?.addEventListener('change', (e) => {
    state.proxy = (e.target as HTMLInputElement).value.trim()
    try { localStorage.setItem('proxy.manual', state.proxy) } catch { /* 忽略 */ }
  })

  $('#btn-cancel')?.addEventListener('click', () => Cancel())
  $('#btn-logdir')?.addEventListener('click', () => { OpenLogDir().catch((e) => appendLog('✗ ' + String(e))) })
}

EventsOn('log', (line: string) => appendLog(line))
EventsOn('progress', (p: { done: number; total: number }) => {
  const a = state.busyOn
  const bar = a ? (document.querySelector(`#dl-${a} div`) as HTMLElement | null) : null
  if (bar && p.total > 0) bar.style.width = Math.min(100, (p.done / p.total) * 100).toFixed(1) + '%'
})

// 代理框默认留空。以前启动时会探测系统代理自动填进去 —— 可各人代理不一样,
// 填错了反而连不上, 不懂的人也不知道那一栏该删。现在只有用户自己填过才记住。
// (key 换过名: 老版本自动探测写进 'proxy' 的值就此失效)
try { state.proxy = localStorage.getItem('proxy.manual') || '' } catch { /* 忽略 */ }
try { state.source = localStorage.getItem('download.source') === 'github' ? 'github' : 'cn' } catch { /* 忽略 */ }
Release().then((r) => { release = r as Release }).catch(() => { /* 开发版: 用默认 */ })
render()
LoadPassword('10.11.99.1').then((pw) => { if (pw) { state.password = pw; keepLog(render); appendLog('已从钥匙串读到保存的密码') } })
LogDir().then((d) => appendLog('日志目录 ' + d))
Version().then((v) => { state.version = v; appendLog('空间助手 ' + v) })
