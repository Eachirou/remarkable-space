# 紧急修复: 让设备以原厂状态启动。设备开机途中一连上就执行, 所以要快、要幂等、
# 每一步失败都不能挡住后面的步骤 (不用 set -e)。不删用户数据, 摘下来的东西都留着备份。
#
# 为什么两个槽都要处理: 底座 drop-in 的 [Unit] 里有 After=home.mount, 冷启动时 /home
# 加密盘解密慢, xochitl 被推迟, 触发 reMarkable 自带的启动看门狗 —— 重启三次后切槽。
# 如果另一个槽也被 ota-watch 注入过同样的 drop-in, 切过去照样起不来, 设备就在两个槽之间
# 来回转。所以 /home 里的熔断不够 (它管不到启动顺序), 必须把两个槽 rootfs 下层的
# drop-in 都摘掉。
TS=$(date +%Y%m%d-%H%M%S)
m() { if [ "$UI_LANG" = en ]; then printf '%s' "$2"; else printf '%s' "$1"; fi; }
say() { echo "[$(m 紧急修复 rescue)] $(m "$1" "$2")"; }

if [ ! -f /etc/version ]; then
    say "这不是 reMarkable 系统 (可能在 Android 模式), 不处理" "Not the reMarkable system (Android mode?), nothing to do"
    exit 0
fi
say "固件 $(cat /etc/version 2>/dev/null), 开始" "firmware $(cat /etc/version 2>/dev/null), starting"

# ── 1. 两个槽的 rootfs: 停用 xochitl 的 drop-in (最关键, 先做) ─────────────
# 改名而不是删除: 不以 .conf 结尾 systemd 就不读, 想恢复改回来即可。
disable_dropins() { # $1 = 某个 rootfs 的 etc 目录
    D="$1/systemd/system/xochitl.service.d"
    [ -d "$D" ] || return 0
    for f in "$D"/zz-rmkit-cn.conf "$D"/zz-space.conf; do
        [ -f "$f" ] || continue
        mv -f "$f" "$f.rescue-$TS" && say "已停用 $f" "disabled $f"
    done
}

mount -o remount,rw / 2>/dev/null
# 当前槽: /etc 是 overlay (上层 tmpfs, 重启即失), 真正持久的是下层 —— 两边都改。
disable_dropins /etc
MNT=/tmp/space-rescue-root
mkdir -p "$MNT"
if mount --bind / "$MNT" 2>/dev/null; then
    mount -o remount,rw,bind "$MNT" 2>/dev/null
    disable_dropins "$MNT/etc"
    sync
    umount -l "$MNT" 2>/dev/null
else
    say "当前槽下层挂不上, 跳过 (本次开机的 /etc 已处理)" "cannot mount the current slot's lower layer, skipping (this boot's /etc is done)"
fi

# 另一个槽 (只有 Paper Pro 系列有 rootdev)
if command -v rootdev >/dev/null 2>&1; then
    INACTIVE=$(rootdev --inactive 2>/dev/null)
    # 系统更新正往那个分区写时别去挂。不能用 pidof swupdate 判断 —— 它是常驻进程,
    # 平时就在跑; 要看有没有进程正打开着这个分区。
    BUSY=""
    for fd in /proc/[0-9]*/fd/*; do
        [ "$(readlink "$fd" 2>/dev/null)" = "$INACTIVE" ] && { BUSY=1; break; }
    done
    if [ -n "$BUSY" ]; then
        say "另一个槽正在被写入 (系统更新中), 不去碰它" "the other slot is being written (system update), leaving it alone"
    elif [ -n "$INACTIVE" ] && [ -b "$INACTIVE" ]; then
        M2=/tmp/space-rescue-inactive
        mkdir -p "$M2"
        if mount "$INACTIVE" "$M2" 2>/dev/null; then
            disable_dropins "$M2/etc"
            sync
            umount "$M2" 2>/dev/null
            say "另一个槽 ($INACTIVE) 已处理" "other slot ($INACTIVE) done"
        else
            say "另一个槽 ($INACTIVE) 挂不上, 跳过" "cannot mount the other slot ($INACTIVE), skipping"
        fi
        rmdir "$M2" 2>/dev/null
    fi
fi
rmdir "$MNT" 2>/dev/null

# ── 2. /home (两个槽共用): 拉熔断、摘注入、搬走 xovi 扩展 ──────────────────
# 开机早期 /home 可能还没挂好 (加密盘解密中), 等一会儿; 等不到也不要紧, 第 1 步已经够设备起来。
i=0
while [ $i -lt 30 ] && ! [ -d /home/root/rmkit-cn ] && ! [ -d /home/root/xovi ]; do
    mountpoint -q /home 2>/dev/null && break
    sleep 1; i=$((i + 1))
done
if [ -d /home/root ]; then
    BK=/home/root/rmkit-rescue/$TS
    RMKIT=/home/root/rmkit-cn
    if [ -d "$RMKIT" ]; then
        # 熔断: 底座预检看到它就一直原生启动, 直到用助手重新安装
        echo "rescue@$TS" > "$RMKIT/.fuse_tripped"
        rm -f "$RMKIT"/active/*.so 2>/dev/null
        say "已拉熔断并摘下注入" "fuse tripped and injection removed"
    fi
    rm -f /home/root/xovi/exthome/qt-resource-rebuilder/*.qmd 2>/dev/null
    EXT=/home/root/xovi/extensions.d
    if [ -d "$EXT" ] && [ -n "$(ls -A "$EXT" 2>/dev/null)" ]; then
        mkdir -p "$BK/extensions.d"
        mv -f "$EXT"/* "$BK/extensions.d/" 2>/dev/null && say "xovi 扩展已搬到 $BK/extensions.d" "xovi extensions moved to $BK/extensions.d"
    fi
    sync
else
    say "/home 还没挂好, 跳过这一步" "/home is not mounted yet, skipping this step"
fi

# ── 3. 让改动对这次开机生效 ───────────────────────────────────────────────
systemctl daemon-reload 2>/dev/null
STATE=$(systemctl is-active xochitl 2>/dev/null)
if [ "$STATE" = "active" ]; then
    say "界面正在运行, 改动下次开机生效" "the UI is running; changes take effect on next boot"
else
    # 起不来 / 在崩溃重启中: 清掉失败计数, 按原厂配置再起一次
    systemctl reset-failed xochitl 2>/dev/null
    systemctl start --no-block xochitl 2>/dev/null
    say "界面状态 $STATE, 已按原厂配置重新启动" "UI state $STATE, restarted with stock configuration"
fi
say "完成。设备现在以原厂状态启动; 要恢复中文和空间, 等设备正常开机后用助手重新安装" "Done. The device now boots stock; to get Space back, reinstall with the assistant once it boots normally"
