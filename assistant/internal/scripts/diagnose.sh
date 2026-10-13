# 导出诊断: 只读收集"为什么起不来 / 为什么切槽 / 为什么空间没了"要看的东西。
# 绝不改设备上的任何东西 (另一个槽只读挂载)。不碰笔记、不读 xochitl.conf (里面有账号凭证)。
# 输出里带 token / password 字样的行由助手在电脑端再遮一遍。
# 注意 3.29 起 BusyBox 1.37 不认 head -5 这种简写, 一律写 head -n / tail -n。
P=/sys/devices/platform/lpgpr
R=/home/root/rmkit-cn
S=/etc/systemd/system
sec() { echo; echo "======== $* ========"; }
kv() { printf '%-22s %s\n' "$1" "$2"; }

sec 基本
kv 时间 "$(date -u '+%Y-%m-%d %H:%M:%S UTC')"
kv 已开机 "$(cut -d. -f1 /proc/uptime) 秒"
kv 机型 "$(tr -d '\0' < /sys/firmware/devicetree/base/model 2>/dev/null)"
kv 固件 "$(cat /etc/version 2>/dev/null)"
kv 系统版本 "$(grep '^IMG_VERSION=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')"
kv 底层系统 "$(grep '^VERSION=' /etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')"
kv 内核 "$(uname -r)"
kv 当前槽 "$(cat $P/root_part 2>/dev/null) ($(rootdev 2>/dev/null))"
kv 另一个槽 "$(rootdev --inactive 2>/dev/null)"
kv 错误计数 "a=$(cat $P/roota_errcnt 2>/dev/null) b=$(cat $P/rootb_errcnt 2>/dev/null)"
kv 系统更新状态 "swu_status=$(cat $P/swu_status 2>/dev/null) swu_applied=$(cat $P/swu_applied 2>/dev/null)"
kv 空间剩余 "/: $(df -kP / | awk 'END{print $4}')KB  /home: $(df -kP /home | awk 'END{print $4}')KB"
kv /home "$(mountpoint -q /home && echo 已挂载 || echo 未挂载)"

sec 当前槽的 xochitl drop-in
ls -la $S/xochitl.service.d/ 2>&1
for f in $S/xochitl.service.d/*; do [ -f "$f" ] && { echo "--- $f"; cat "$f"; }; done
sec 当前槽的空间服务
ls -la $S/remarkable-space.service $S/remarkable-space.service.d/ 2>&1
ls -la $S/multi-user.target.wants/ 2>&1 | grep -iE "space|rmkit"

sec "另一个槽 (只读挂载)"
INACTIVE=$(rootdev --inactive 2>/dev/null)
if [ -n "$INACTIVE" ] && [ -b "$INACTIVE" ]; then
    BUSY=""
    for fd in /proc/[0-9]*/fd/*; do
        [ "$(readlink "$fd" 2>/dev/null)" = "$INACTIVE" ] && { BUSY=1; break; }
    done
    M=/tmp/space-diag-inactive
    if [ -n "$BUSY" ]; then
        echo "正在被写入 (系统更新中), 跳过"
    elif mkdir -p $M && mount -o ro "$INACTIVE" $M 2>/dev/null; then
        kv 固件 "$(cat $M/etc/version 2>/dev/null)"
        kv 系统版本 "$(grep '^IMG_VERSION=' $M/etc/os-release 2>/dev/null | cut -d= -f2 | tr -d '"')"
        ls -la $M$S/xochitl.service.d/ 2>&1
        for f in $M$S/xochitl.service.d/*; do [ -f "$f" ] && { echo "--- $f"; cat "$f"; }; done
        ls -la $M$S/remarkable-space.service 2>&1
        umount $M
    else
        echo "挂不上"
    fi
    rmdir $M 2>/dev/null
else
    echo "该机型没有 A/B 槽"
fi

sec 底座 / 空间状态
kv 注入状态 "$(cat $R/inject-status.json 2>/dev/null || echo 无)"
kv 熔断 "$(cat $R/.fuse_tripped 2>/dev/null || echo 无)"
kv "600 秒内启动次数" "$(awk -v n=$(date +%s) 'n-$1<600' $R/.starts 2>/dev/null | wc -l)"
kv 安装时固件 "$(cat $R/.last_fw_version 2>/dev/null || echo 无)"
kv hashtab固件 "$(cat /home/root/xovi/exthome/qt-resource-rebuilder/hashtab.fw_version 2>/dev/null || echo 无)"
kv 空间版本 "$(cat $R/.space_version 2>/dev/null || echo 无)"
kv 已挂注入 "$(ls $R/active 2>/dev/null | tr '\n' ' ')"
kv xovi "$([ -f /home/root/xovi/xovi.so ] && echo 在 || echo 不在)"
kv xovi扩展 "$(ls /home/root/xovi/extensions.d 2>/dev/null | tr '\n' ' ')"
kv 隔离的qmd "$(ls $R/quarantine 2>/dev/null | tr '\n' ' ')"
kv 紧急修复备份 "$(ls /home/root/rmkit-rescue 2>/dev/null | tr '\n' ' ')"
kv 空间后端单元备份 "$(ls $R/etc/space-systemd 2>/dev/null | tr '\n' ' ')"

sec 服务
for u in xochitl remarkable-space rmkit-cn-ime-http rmkit-cn-ota.timer; do
    kv "$u" "$(systemctl is-active $u 2>/dev/null) / 重启 $(systemctl show -p NRestarts --value $u 2>/dev/null) 次"
done
echo "--- 失败的单元"
systemctl --failed --no-legend --no-pager 2>/dev/null

sec 开机记录
journalctl --list-boots --no-pager 2>/dev/null | tail -n 12

# 最近几次开机: 关键行 + 每次开机的最后几十行 (看它是怎么结束的: 正常关机 / 看门狗 / 紧急模式)
KEY='xochitl|watchdog|emergency|Failed|failed|error|panic|segfault|SIGILL|SIGSEGV|core|Killed|precheck|fw-upgrade|rmkit|xovi|qmldiff|home.mount|lpgpr|errcnt|sanity|start request repeated|start-limit|Rebooting|reboot|Reached target|Dependency'
for b in 0 -1 -2 -3 -4 -5; do
    journalctl -b $b --no-pager -n 1 >/dev/null 2>&1 || continue
    sec "第 $b 次开机 (0 = 本次): 关键行"
    journalctl -b $b --no-pager -o short-monotonic 2>/dev/null | grep -E "$KEY" | grep -v "qt.network\|eventcache\|rm.http" | tail -n 120
    sec "第 $b 次开机: 结尾"
    journalctl -b $b --no-pager -o short-monotonic 2>/dev/null | tail -n 40
done

sec 本次固件适配日志
cat /tmp/fw-upgrade.log 2>/dev/null | tail -n 60 || echo 无
